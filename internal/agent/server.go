// Package agent runs Rowan's MCP server. Agents such as Claude Desktop and
// Cursor connect over Streamable HTTP on 127.0.0.1, or over stdio through
// "rowan mcp --stdio", which forwards to the same endpoint.
package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"rowan/internal/config"
	"rowan/internal/db"
)

// Version is the server version reported to clients.
const Version = "0.1.0"

var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Session is one connected client.
type Session struct {
	ID        string
	Client    string
	Transport string
	Since     time.Time
	LastSeen  time.Time
	Calls     int
	Closed    bool
}

// ClientInfo is a client row for the UI.
type ClientInfo struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Since     int64  `json:"since"`
	LastSeen  int64  `json:"lastSeen"`
	Calls     int    `json:"calls"`
	Connected bool   `json:"connected"`
	Blocked   bool   `json:"blocked"`
}

// Status is the server state for the UI.
type Status struct {
	Running bool         `json:"running"`
	Port    int          `json:"port"`
	Error   string       `json:"error"`
	Clients []ClientInfo `json:"clients"`
}

// Agent is the MCP server.
type Agent struct {
	store *config.Store
	dbm   *db.Manager
	emit  func(event string, data any)

	Log   *Log
	Queue *Queue

	mu       sync.Mutex
	sessions map[string]*Session
	srv      *http.Server
	port     int
	err      string
	token    string
}

// New creates the server. It does not start listening.
func New(store *config.Store, dbm *db.Manager, emit func(string, any)) *Agent {
	return &Agent{store: store, dbm: dbm, emit: emit, Log: OpenLog(), Queue: &Queue{}, sessions: map[string]*Session{}}
}

// Start listens on 127.0.0.1 at the configured port.
func (a *Agent) Start() error {
	cfg := a.store.Get()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.srv != nil {
		return nil
	}
	tok, err := config.MCPToken()
	if err != nil {
		a.err = err.Error()
		return err
	}
	a.token = tok
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.MCP.HTTPPort)))
	if err != nil {
		a.err = fmt.Sprintf("can't listen on port %d: %v", cfg.MCP.HTTPPort, err)
		return errors.New(a.err)
	}
	a.port = cfg.MCP.HTTPPort
	a.err = ""
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", a.handle)
	a.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go a.srv.Serve(ln)
	return nil
}

// Stop closes the listener and cancels waiting calls.
func (a *Agent) Stop() {
	a.mu.Lock()
	srv := a.srv
	a.srv = nil
	for _, s := range a.sessions {
		s.Closed = true
	}
	a.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		srv.Close()
	}
	for _, it := range a.Queue.List() {
		if x := a.Queue.take(it.ID); x != nil {
			x.decide <- false
		}
	}
	a.emitQueue()
	a.emitClients()
}

// Close stops the server and closes the log.
func (a *Agent) Close() {
	a.Stop()
	a.Log.Close()
}

// Status returns the server state.
func (a *Agent) Status() Status {
	cfg := a.store.Get()
	a.mu.Lock()
	defer a.mu.Unlock()
	st := Status{Running: a.srv != nil, Port: cfg.MCP.HTTPPort, Error: a.err}
	byName := map[string]*ClientInfo{}
	var order []string
	for _, s := range a.sessions {
		ci, ok := byName[s.Client]
		if !ok {
			ci = &ClientInfo{Name: s.Client, Transport: s.Transport}
			byName[s.Client] = ci
			order = append(order, s.Client)
		}
		connected := !s.Closed && st.Running && time.Since(s.LastSeen) < 30*time.Minute
		if connected && (!ci.Connected || s.Since.UnixMilli() < ci.Since) {
			ci.Since = s.Since.UnixMilli()
			ci.Transport = s.Transport
		}
		ci.Connected = ci.Connected || connected
		ci.Calls += s.Calls
		if s.LastSeen.UnixMilli() > ci.LastSeen {
			ci.LastSeen = s.LastSeen.UnixMilli()
		}
	}
	slices.Sort(order)
	for _, n := range order {
		ci := byName[n]
		ci.Blocked = slices.Contains(cfg.MCP.BlockedClients, n)
		st.Clients = append(st.Clients, *ci)
	}
	return st
}

// Token returns the bearer token.
func (a *Agent) Token() string {
	t, _ := config.MCPToken()
	return t
}

func (a *Agent) emitQueue()   { a.emit("queue", a.Queue.List()) }
func (a *Agent) emitClients() { a.emit("clients", a.Status()) }

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

func (a *Agent) handle(w http.ResponseWriter, r *http.Request) {
	host, port, _ := net.SplitHostPort(r.Host)
	if (host != "127.0.0.1" && host != "localhost") || port != strconv.Itoa(a.port) {
		http.Error(w, "invalid Host header", http.StatusForbidden)
		return
	}
	// Browsers send Origin. Agents don't. Refusing it blocks web pages and
	// DNS rebinding.
	if r.Header.Get("Origin") != "" {
		http.Error(w, "browser requests are not allowed", http.StatusForbidden)
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), []byte(a.token)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="rowan"`)
		http.Error(w, "missing or wrong bearer token. Copy it from Rowan > MCP server", http.StatusUnauthorized)
		return
	}
	sid := r.Header.Get("Mcp-Session-Id")
	switch r.Method {
	case http.MethodDelete:
		a.mu.Lock()
		if s := a.sessions[sid]; s != nil {
			s.Closed = true
		}
		a.mu.Unlock()
		a.emitClients()
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "this server does not open SSE streams", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	var msgs []rpcMsg
	batch := len(body) > 0 && strings.TrimSpace(string(body))[0] == '['
	if batch {
		err = json.Unmarshal(body, &msgs)
	} else {
		var m rpcMsg
		err = json.Unmarshal(body, &m)
		msgs = []rpcMsg{m}
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "parse error"}})
		return
	}
	var sess *Session
	for _, m := range msgs {
		if m.Method == "initialize" {
			sess = a.newSession(m.Params, r.Header.Get("X-Rowan-Transport"))
			w.Header().Set("Mcp-Session-Id", sess.ID)
		}
	}
	if sess == nil {
		a.mu.Lock()
		sess = a.sessions[sid]
		a.mu.Unlock()
		if sess == nil || sess.Closed {
			http.Error(w, "unknown session. Send initialize first", http.StatusNotFound)
			return
		}
	}
	a.mu.Lock()
	sess.LastSeen = time.Now()
	a.mu.Unlock()

	var out []rpcResp
	for _, m := range msgs {
		if len(m.ID) == 0 || string(m.ID) == "null" {
			continue // notification
		}
		res, rerr := a.dispatch(r.Context(), sess, m)
		resp := rpcResp{JSONRPC: "2.0", ID: m.ID}
		if rerr != nil {
			resp.Error = rerr
		} else {
			resp.Result = res
		}
		out = append(out, resp)
	}
	if len(out) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if batch {
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusOK, out[0])
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *Agent) newSession(params json.RawMessage, transport string) *Session {
	var p struct {
		ClientInfo struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(params, &p)
	if transport == "" {
		transport = "HTTP"
	}
	s := &Session{ID: config.NewID("mcp-"), Client: ClientName(p.ClientInfo.Name), Transport: transport, Since: time.Now(), LastSeen: time.Now()}
	a.mu.Lock()
	a.sessions[s.ID] = s
	// Drop closed sessions of the same client, and sessions that have been
	// idle for a day, so the list stays short.
	for id, old := range a.sessions {
		if id != s.ID && ((old.Client == s.Client && old.Closed) || time.Since(old.LastSeen) > 24*time.Hour) {
			delete(a.sessions, id)
		}
	}
	a.mu.Unlock()
	a.emitClients()
	return s
}

// ClientName maps the clientInfo name to a display name.
func ClientName(raw string) string {
	l := strings.ToLower(raw)
	switch {
	case l == "claude-ai" || strings.Contains(l, "claude desktop"):
		return "Claude Desktop"
	case strings.Contains(l, "claude-code") || strings.Contains(l, "claude code"):
		return "Claude Code"
	case strings.Contains(l, "cursor"):
		return "Cursor"
	case strings.Contains(l, "vscode") || strings.Contains(l, "visual studio code"):
		return "VS Code"
	case strings.Contains(l, "zed"):
		return "Zed"
	case raw == "":
		return "Unknown client"
	}
	return raw
}

func (a *Agent) dispatch(ctx context.Context, s *Session, m rpcMsg) (any, *rpcErr) {
	switch m.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(m.Params, &p)
		v := protocolVersions[0]
		if slices.Contains(protocolVersions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "rowan", "title": "Rowan", "version": Version},
			"instructions":    "Rowan gives access to the databases that the user shared with agents. Call list_connections first. Reads are capped at a row limit. Writes go through execute and can wait for the user to approve them in the Rowan app, so always give a short reason.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": a.toolList()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, &rpcErr{-32602, "invalid params"}
		}
		a.mu.Lock()
		s.Calls++
		a.mu.Unlock()
		defer a.emitClients()
		return a.call(ctx, s, p.Name, p.Arguments), nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	}
	return nil, &rpcErr{-32601, "method not found: " + m.Method}
}
