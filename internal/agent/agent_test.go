package agent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"rowan/internal/config"
	"rowan/internal/db"
)

type client struct {
	t     *testing.T
	url   string
	token string
	sid   string
}

func (c *client) post(body string, origin string) (*http.Response, map[string]any) {
	req, _ := http.NewRequest(http.MethodPost, c.url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.sid != "" {
		req.Header.Set("Mcp-Session-Id", c.sid)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func (c *client) call(tool string, args map[string]string) (string, bool) {
	a, _ := json.Marshal(args)
	_, out := c.post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+string(a)+`}}`, "")
	res := out["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	return text, res["isError"] == true
}

func TestAgent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROWAN_CONFIG_DIR", dir)
	path := filepath.Join(dir, "s.sqlite")
	if err := db.CreateSampleFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	store, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	err = store.Update(func(c *config.Config) error {
		c.MCP.HTTPPort = port
		c.Connections = []config.Connection{{ID: "s", Name: "shop", Engine: "sqlite", File: path,
			Agent: config.Policy{Exposed: true, Access: "read-write", ApproveWrites: true, RowLimit: 5, Tables: []string{"orders", "customers"}}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dbm := db.NewManager()
	defer dbm.CloseAll()
	var mu sync.Mutex
	events := map[string]int{}
	a := New(store, dbm, func(ev string, _ any) { mu.Lock(); events[ev]++; mu.Unlock() })
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	c := &client{t: t, url: "http://127.0.0.1:" + strconv.Itoa(port) + "/mcp", token: a.Token()}
	resp, _ := c.post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"claude-ai"}}}`, "")
	c.sid = resp.Header.Get("Mcp-Session-Id")
	if c.sid == "" {
		t.Fatal("no session id")
	}
	if resp, _ := c.post(`{}`, "http://example.com"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origin: %d", resp.StatusCode)
	}
	bad := *c
	bad.token = "nope"
	if resp, _ := bad.post(`{}`, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token: %d", resp.StatusCode)
	}

	if txt, isErr := c.call("query", map[string]string{"connection": "shop", "sql": "SELECT * FROM orders"}); isErr || !strings.Contains(txt, `"truncated": true`) {
		t.Fatalf("query: %v %s", isErr, txt)
	}
	if txt, isErr := c.call("query", map[string]string{"connection": "shop", "sql": "SELECT * FROM invoices"}); !isErr || !strings.Contains(txt, "allowlist") {
		t.Fatalf("hidden table: %v %s", isErr, txt)
	}
	if _, isErr := c.call("query", map[string]string{"connection": "shop", "sql": "DELETE FROM orders"}); !isErr {
		t.Fatal("query should refuse writes")
	}
	if txt, _ := c.call("list_tables", map[string]string{"connection": "shop"}); strings.Contains(txt, "invoices") {
		t.Fatal("list_tables shows a hidden table")
	}
	if txt, _ := c.call("list_tables", map[string]string{"connection": "shop", "filter": "ORD"}); !strings.Contains(txt, `"orders"`) || strings.Contains(txt, "customers") {
		t.Fatalf("list_tables filter: %s", txt)
	}

	done := make(chan string, 1)
	go func() {
		txt, _ := c.call("execute", map[string]string{"connection": "shop", "sql": "UPDATE orders SET status = 'refunded' WHERE id = 10400", "reason": "test"})
		done <- txt
	}()
	var items []Item
	for i := 0; i < 50 && len(items) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		items = a.Queue.List()
	}
	if len(items) != 1 || items[0].Impact != "Updates 1 row in orders" {
		t.Fatalf("queue: %+v", items)
	}
	a.Decide([]string{items[0].ID}, true)
	select {
	case txt := <-done:
		if !strings.Contains(txt, "approved") || !strings.Contains(txt, "UPDATE 1") {
			t.Fatalf("execute: %s", txt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execute did not return")
	}
	if n := len(a.Log.Recent(100)); n != 6 {
		t.Fatalf("log entries: %d", n)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "activity.jsonl")); err != nil || bytes.Count(b, []byte("\n")) != 6 {
		t.Fatalf("activity.jsonl: %v", err)
	}
}

func TestListTablesCap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROWAN_CONFIG_DIR", dir)
	path := filepath.Join(dir, "big.sqlite")
	w, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 230 {
		if _, err := w.Exec(fmt.Sprintf("CREATE TABLE t_%03d (id INTEGER PRIMARY KEY)", i)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	store, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Update(func(c *config.Config) error {
		c.MCP.HTTPPort = port
		c.Connections = []config.Connection{{ID: "b", Name: "big", Engine: "sqlite", File: path, Agent: config.Policy{Exposed: true, Access: "read-only", RowLimit: 5}}}
		return nil
	})
	dbm := db.NewManager()
	defer dbm.CloseAll()
	a := New(store, dbm, func(string, any) {})
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	c := &client{t: t, url: "http://127.0.0.1:" + strconv.Itoa(port) + "/mcp", token: a.Token()}
	resp, _ := c.post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}`, "")
	c.sid = resp.Header.Get("Mcp-Session-Id")

	var out struct {
		Tables []map[string]any `json:"tables"`
		Total  int              `json:"total"`
		Note   string           `json:"note"`
	}
	txt, _ := c.call("list_tables", map[string]string{"connection": "big"})
	if err := json.Unmarshal([]byte(txt), &out); err != nil || len(out.Tables) != maxListTables || out.Total != 230 || out.Note == "" {
		t.Fatalf("cap: %d tables, total %d, note %q, %v", len(out.Tables), out.Total, out.Note, err)
	}
	out.Tables, out.Total = nil, 0
	txt, _ = c.call("list_tables", map[string]string{"connection": "big", "filter": "t_22"})
	if err := json.Unmarshal([]byte(txt), &out); err != nil || len(out.Tables) != 10 || out.Total != 0 {
		t.Fatalf("filter: %d tables, total %d, %v", len(out.Tables), out.Total, err)
	}
}
