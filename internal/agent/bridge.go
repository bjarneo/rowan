package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"rowan/internal/config"
)

// Bridge forwards MCP messages from stdin to the running Rowan app and
// writes the replies to stdout. Clients that only speak stdio, such as
// Claude Desktop, start it with "rowan mcp --stdio".
//
// When the app is not running, the bridge still answers initialize and
// tools/list, and every tool call returns an error that tells the user to
// open Rowan.
func Bridge(in io.Reader, out io.Writer) error {
	store, _, err := config.Load()
	if err != nil {
		return err
	}
	cfg := store.Get()
	token, err := config.MCPToken()
	if err != nil {
		return err
	}
	b := &bridge{
		url:   "http://127.0.0.1:" + strconv.Itoa(cfg.MCP.HTTPPort) + "/mcp",
		token: token,
		out:   out,
		store: store,
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		msg := append([]byte(nil), line...)
		var m rpcMsg
		_ = json.Unmarshal(msg, &m)
		if m.Method == "initialize" {
			b.initialize(msg, m)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.forward(msg, m)
		}()
	}
	wg.Wait()
	b.closeSession()
	return sc.Err()
}

type bridge struct {
	url, token string
	out        io.Writer
	store      *config.Store

	mu      sync.Mutex
	outMu   sync.Mutex
	session string
	init    []byte // the client's initialize request, replayed when the app starts later
}

func (b *bridge) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.outMu.Lock()
	defer b.outMu.Unlock()
	b.out.Write(append(data, '\n'))
}

func (b *bridge) writeRaw(data []byte) {
	b.outMu.Lock()
	defer b.outMu.Unlock()
	b.out.Write(append(bytes.TrimSpace(data), '\n'))
}

func (b *bridge) post(body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, b.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("X-Rowan-Transport", "stdio")
	b.mu.Lock()
	if b.session != "" {
		req.Header.Set("Mcp-Session-Id", b.session)
	}
	b.mu.Unlock()
	return http.DefaultClient.Do(req)
}

func (b *bridge) initialize(raw []byte, m rpcMsg) {
	b.mu.Lock()
	b.init = raw
	b.mu.Unlock()
	if body, ok := b.connect(); ok {
		b.writeRaw(body)
		return
	}
	// The app is closed. Answer locally so the client starts.
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(m.Params, &p)
	v := protocolVersions[0]
	for _, x := range protocolVersions {
		if x == p.ProtocolVersion {
			v = x
		}
	}
	b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Result: map[string]any{
		"protocolVersion": v,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "rowan", "title": "Rowan", "version": Version},
		"instructions":    "Rowan is not running. Tool calls fail until the user opens the Rowan app.",
	}})
}

// connect sends the saved initialize request and stores the session id.
func (b *bridge) connect() ([]byte, bool) {
	b.mu.Lock()
	initMsg := b.init
	b.session = ""
	b.mu.Unlock()
	if initMsg == nil {
		return nil, false
	}
	resp, err := b.post(initMsg)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "rowan: initialize failed: %s %s\n", resp.Status, strings.TrimSpace(string(body)))
		return nil, false
	}
	b.mu.Lock()
	b.session = resp.Header.Get("Mcp-Session-Id")
	b.mu.Unlock()
	// Tell the server the handshake is done.
	if r, err := b.post([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); err == nil {
		r.Body.Close()
	}
	return body, true
}

func (b *bridge) forward(raw []byte, m rpcMsg) {
	isRequest := len(m.ID) > 0 && string(m.ID) != "null"
	b.mu.Lock()
	hasSession := b.session != ""
	b.mu.Unlock()
	if !hasSession {
		if _, ok := b.connect(); !ok {
			b.offline(m, isRequest)
			return
		}
	}
	resp, err := b.post(raw)
	if err == nil && resp.StatusCode == http.StatusNotFound {
		// The app restarted and forgot the session. Start a new one.
		resp.Body.Close()
		if _, ok := b.connect(); ok {
			resp, err = b.post(raw)
		} else {
			err = ErrNotRunning
		}
	}
	if err != nil {
		b.mu.Lock()
		b.session = ""
		b.mu.Unlock()
		b.offline(m, isRequest)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !isRequest {
		return
	}
	if resp.StatusCode != http.StatusOK {
		b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Error: &rpcErr{-32000, fmt.Sprintf("Rowan answered %s: %s", resp.Status, strings.TrimSpace(string(body)))}})
		return
	}
	b.writeRaw(body)
}

// offline answers a request while the app is closed.
func (b *bridge) offline(m rpcMsg, isRequest bool) {
	if !isRequest {
		return
	}
	switch m.Method {
	case "ping":
		b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Result: map[string]any{}})
	case "tools/list":
		a := &Agent{store: b.store}
		b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Result: map[string]any{"tools": a.toolList()}})
	case "tools/call":
		b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Result: failure(ErrNotRunning.Error() + ".")})
	default:
		b.write(rpcResp{JSONRPC: "2.0", ID: m.ID, Error: &rpcErr{-32000, ErrNotRunning.Error()}})
	}
}

func (b *bridge) closeSession() {
	b.mu.Lock()
	sid := b.session
	b.mu.Unlock()
	if sid == "" {
		return
	}
	req, err := http.NewRequest(http.MethodDelete, b.url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Mcp-Session-Id", sid)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
}
