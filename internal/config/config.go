// Package config reads and writes ~/.config/rowan/config.toml.
package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Config is the full content of config.toml.
type Config struct {
	App         App          `toml:"app" json:"app"`
	Safety      Safety       `toml:"safety" json:"safety"`
	MCP         MCP          `toml:"mcp" json:"mcp"`
	Connections []Connection `toml:"connections" json:"connections"`
	Snippets    []Snippet    `toml:"snippets" json:"snippets"`
}

// App holds display settings.
type App struct {
	Theme    string `toml:"theme" json:"theme"`        // system, light or dark
	Layout   string `toml:"layout" json:"layout"`      // command or workspace
	Accent   string `toml:"accent" json:"accent"`      // violet, blue, green or teal
	Density  string `toml:"density" json:"density"`    // compact or balanced
	RowLimit int    `toml:"row_limit" json:"rowLimit"` // rows fetched per query in the app
}

// Safety holds the write protection settings.
type Safety struct {
	RelockAfter             string `toml:"relock_after" json:"relockAfter"` // such as "15m", or "relaunch"
	ConfirmProductionUnlock bool   `toml:"confirm_production_unlock" json:"confirmProductionUnlock"`
	ConfirmDestructive      bool   `toml:"confirm_destructive" json:"confirmDestructive"`
	StatementTimeout        string `toml:"statement_timeout" json:"statementTimeout"`
}

// MCP holds the agent server settings.
type MCP struct {
	Enabled        bool     `toml:"enabled" json:"enabled"`
	HTTPPort       int      `toml:"http_port" json:"httpPort"`
	Tools          []string `toml:"tools" json:"tools"`
	BlockedClients []string `toml:"blocked_clients" json:"blockedClients"`
}

// Connection is one saved database.
type Connection struct {
	ID          string `toml:"id" json:"id"`
	Name        string `toml:"name" json:"name"`
	Engine      string `toml:"engine" json:"engine"` // postgres, mysql or sqlite
	Host        string `toml:"host,omitempty" json:"host"`
	Port        int    `toml:"port,omitzero" json:"port"`
	Database    string `toml:"database,omitempty" json:"database"`
	User        string `toml:"user,omitempty" json:"user"`
	Password    string `toml:"password,omitempty" json:"-"` // "keychain:rowan/<id>" or "file:<id>"
	HasPassword bool   `toml:"-" json:"hasPassword"`
	File        string `toml:"file,omitempty" json:"file"`
	SSLMode     string `toml:"ssl_mode,omitempty" json:"sslMode"`
	SSLRootCert string `toml:"ssl_root_cert,omitempty" json:"sslRootCert"`
	Color       string `toml:"color" json:"color"`
	Production  bool   `toml:"production" json:"production"`
	SSH         SSH    `toml:"ssh" json:"ssh"`
	Agent       Policy `toml:"mcp" json:"mcp"`
}

// SSH describes an optional bastion tunnel.
type SSH struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Host    string `toml:"host,omitempty" json:"host"`
	Port    int    `toml:"port,omitzero" json:"port"`
	User    string `toml:"user,omitempty" json:"user"`
	Key     string `toml:"key,omitempty" json:"key"`
}

// Policy controls what agents can do on one connection.
type Policy struct {
	Exposed       bool     `toml:"exposed" json:"exposed"`
	Access        string   `toml:"access" json:"access"` // read-only or read-write
	ApproveWrites bool     `toml:"approve_writes" json:"approveWrites"`
	RowLimit      int      `toml:"row_limit" json:"rowLimit"`
	Tables        []string `toml:"tables,omitempty" json:"tables"` // empty means all tables
}

// Snippet is a saved query.
type Snippet struct {
	ID   string `toml:"id" json:"id"`
	Name string `toml:"name" json:"name"`
	SQL  string `toml:"sql" json:"sql"`
}

// AllTools lists the MCP tools in display order.
var AllTools = []string{"list_connections", "list_tables", "describe_table", "query", "explain", "execute"}

// Default returns the configuration that a first launch writes.
func Default() Config {
	return Config{
		App:    App{Theme: "system", Layout: "command", Accent: "violet", Density: "balanced", RowLimit: 1000},
		Safety: Safety{RelockAfter: "15m", ConfirmProductionUnlock: true, ConfirmDestructive: true, StatementTimeout: "60s"},
		MCP:    MCP{Enabled: true, HTTPPort: 7421, Tools: append([]string(nil), AllTools...)},
	}
}

// DefaultPolicy is the policy of a connection that has none.
func DefaultPolicy() Policy {
	return Policy{Exposed: false, Access: "read-only", ApproveWrites: true, RowLimit: 500}
}

// Dir returns the Rowan config directory. ROWAN_CONFIG_DIR overrides it.
func Dir() string {
	if d := os.Getenv("ROWAN_CONFIG_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "rowan")
}

// Path returns the path of config.toml.
func Path() string { return filepath.Join(Dir(), "config.toml") }

// DisplayPath shortens a path under the home directory to ~/...
func DisplayPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// Expand turns a leading ~ into the home directory.
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// Store guards the config and writes it to disk.
type Store struct {
	mu  sync.RWMutex
	cfg Config
}

// Load reads config.toml. It returns created=true when the file did not exist.
func Load() (*Store, bool, error) {
	s := &Store{cfg: Default()}
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return s, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	cfg := Default()
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		return nil, false, fmt.Errorf("read %s: %w", DisplayPath(Path()), err)
	}
	cfg.normalize()
	s.cfg = cfg
	return s, false, nil
}

func (c *Config) normalize() {
	d := Default()
	if c.App.Theme == "" {
		c.App.Theme = d.App.Theme
	}
	if c.App.Layout != "command" && c.App.Layout != "workspace" {
		c.App.Layout = d.App.Layout
	}
	if c.App.Accent == "" {
		c.App.Accent = d.App.Accent
	}
	if c.App.Density == "" {
		c.App.Density = d.App.Density
	}
	if c.App.RowLimit <= 0 {
		c.App.RowLimit = d.App.RowLimit
	}
	if c.Safety.RelockAfter == "" {
		c.Safety.RelockAfter = d.Safety.RelockAfter
	}
	if c.Safety.StatementTimeout == "" {
		c.Safety.StatementTimeout = d.Safety.StatementTimeout
	}
	if c.MCP.HTTPPort == 0 {
		c.MCP.HTTPPort = d.MCP.HTTPPort
	}
	for i := range c.Connections {
		cn := &c.Connections[i]
		if cn.ID == "" {
			cn.ID = Slug(cn.Name)
		}
		if cn.Agent.Access == "" {
			cn.Agent.Access = "read-only"
		}
		if cn.Agent.RowLimit <= 0 {
			cn.Agent.RowLimit = 500
		}
		if cn.Production {
			cn.Agent.ApproveWrites = true
		}
		if cn.Color == "" {
			cn.Color = "gray"
		}
		if cn.SSH.Port == 0 {
			cn.SSH.Port = 22
		}
	}
	for i := range c.Snippets {
		if c.Snippets[i].ID == "" {
			c.Snippets[i].ID = NewID("s")
		}
	}
}

// Get returns a deep copy of the config.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.clone()
}

func (c Config) clone() Config {
	out := c
	out.MCP.Tools = append([]string(nil), c.MCP.Tools...)
	out.MCP.BlockedClients = append([]string(nil), c.MCP.BlockedClients...)
	out.Connections = make([]Connection, len(c.Connections))
	for i, cn := range c.Connections {
		cn.Agent.Tables = append([]string(nil), cn.Agent.Tables...)
		cn.HasPassword = cn.Password != ""
		out.Connections[i] = cn
	}
	out.Snippets = append([]Snippet(nil), c.Snippets...)
	return out
}

// Connection returns one connection by id.
func (s *Store) Connection(id string) (Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cfg.Connections {
		if c.ID == id {
			c.Agent.Tables = append([]string(nil), c.Agent.Tables...)
			return c, true
		}
	}
	return Connection{}, false
}

// Update changes the config under the lock and writes it to disk.
func (s *Store) Update(fn func(c *Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg.clone()
	if err := fn(&next); err != nil {
		return err
	}
	next.normalize()
	if err := write(next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

// Save writes the current config to disk.
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return write(s.cfg)
}

func write(c Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# " + DisplayPath(Path()) + "\n")
	buf.WriteString("# Written by Rowan. Passwords live in the system keychain or in credentials.json, never in this file.\n\n")
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return err
	}
	return WriteFileAtomic(Path(), buf.Bytes(), 0o600)
}

// WriteFileAtomic writes to a temp file and renames it over path.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Slug makes an id from a name.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimSuffix(b.String(), "-")
	if s == "" {
		s = "conn"
	}
	return s
}

// NewID returns a random id with a prefix.
func NewID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// MCPToken returns the bearer token for the HTTP MCP endpoint. It creates
// the token file on first use.
func MCPToken() (string, error) {
	p := filepath.Join(Dir(), "mcp-token")
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return "", err
	}
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	t := "rwn_" + hex.EncodeToString(b[:])
	if err := WriteFileAtomic(p, []byte(t+"\n"), 0o600); err != nil {
		return "", err
	}
	return t, nil
}
