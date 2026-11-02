// Package db opens database connections and runs statements for Rowan.
//
// Each connection has two pools. The read pool is read-only at the database
// level: PostgreSQL sessions start with default_transaction_read_only, MySQL
// sessions with transaction_read_only, and SQLite connections with
// query_only. The write pool opens only when a caller asks for it, and the
// caller must check the unlock state first.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/ssh"

	"rowan/internal/config"
)

// Engine labels.
var EngineLabel = map[string]string{"postgres": "PostgreSQL", "mysql": "MySQL", "sqlite": "SQLite"}

// Manager owns the open connections.
type Manager struct {
	mu    sync.Mutex
	conns map[string]*Conn
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{conns: map[string]*Conn{}} }

// Conn returns the connection for cfg. When the saved settings changed since
// the last call, it closes the old pools first.
func (m *Manager) Conn(cfg config.Connection) *Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[cfg.ID]; ok {
		if sameTarget(c.cfg, cfg) {
			c.mu.Lock()
			c.cfg = cfg
			c.mu.Unlock()
			return c
		}
		go c.Close()
	}
	c := &Conn{cfg: cfg, tables: map[string]*Table{}, samples: map[string]*sampleSet{}}
	m.conns[cfg.ID] = c
	return c
}

// Close closes one connection.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	c := m.conns[id]
	delete(m.conns, id)
	m.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

// CloseWriters closes the write pool of one connection.
func (m *Manager) CloseWriters(id string) {
	m.mu.Lock()
	c := m.conns[id]
	m.mu.Unlock()
	if c != nil {
		c.closeWriter()
	}
}

// CloseAll closes every connection.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	list := make([]*Conn, 0, len(m.conns))
	for _, c := range m.conns {
		list = append(list, c)
	}
	m.conns = map[string]*Conn{}
	m.mu.Unlock()
	for _, c := range list {
		c.Close()
	}
}

func sameTarget(a, b config.Connection) bool {
	a.Name, b.Name = "", ""
	a.Color, b.Color = "", ""
	a.Production, b.Production = false, false
	a.Agent, b.Agent = config.Policy{}, config.Policy{}
	return reflect.DeepEqual(a, b)
}

// Conn is one database target with its pools and caches.
type Conn struct {
	mu      sync.Mutex
	cfg     config.Connection
	pw      *string // password override for connection tests
	ro, rw  *sql.DB
	tunnel  *ssh.Client
	version string

	cacheMu sync.Mutex
	schema  *Schema
	columns *ColumnSet
	tables  map[string]*Table
	samples map[string]*sampleSet
}

// Config returns the saved settings.
func (c *Conn) Config() config.Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// Engine returns postgres, mysql or sqlite.
func (c *Conn) Engine() string { return c.Config().Engine }

// HashComments reports whether '#' starts a comment in this dialect.
func (c *Conn) HashComments() bool { return c.Engine() == "mysql" }

// Close closes both pools and the tunnel.
func (c *Conn) Close() {
	c.mu.Lock()
	ro, rw, t := c.ro, c.rw, c.tunnel
	c.ro, c.rw, c.tunnel = nil, nil, nil
	c.mu.Unlock()
	if ro != nil {
		ro.Close()
	}
	if rw != nil {
		rw.Close()
	}
	if t != nil {
		t.Close()
	}
}

func (c *Conn) closeWriter() {
	c.mu.Lock()
	rw := c.rw
	c.rw = nil
	c.mu.Unlock()
	if rw != nil {
		rw.Close()
	}
}

// Reader returns the read-only pool. It opens the pool on first use.
func (c *Conn) Reader(ctx context.Context) (*sql.DB, error) { return c.pool(ctx, false) }

// Writer returns the read-write pool. The caller must check the unlock state.
func (c *Conn) Writer(ctx context.Context) (*sql.DB, error) { return c.pool(ctx, true) }

func (c *Conn) pool(ctx context.Context, write bool) (*sql.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if write && c.rw != nil {
		return c.rw, nil
	}
	if !write && c.ro != nil {
		return c.ro, nil
	}
	db, err := c.openLocked(ctx, write)
	if err != nil {
		return nil, err
	}
	if write {
		c.rw = db
	} else {
		c.ro = db
	}
	return db, nil
}

func (c *Conn) openLocked(ctx context.Context, write bool) (*sql.DB, error) {
	cfg := c.cfg
	var pw string
	var err error
	if c.pw != nil {
		pw = *c.pw
	} else if pw, err = config.Password(cfg.Password); err != nil {
		return nil, err
	}
	var dial func(ctx context.Context, network, addr string) (net.Conn, error)
	if cfg.SSH.Enabled && cfg.Engine != "sqlite" {
		if c.tunnel == nil {
			t, err := dialSSH(ctx, cfg.SSH)
			if err != nil {
				return nil, err
			}
			c.tunnel = t
		}
		t := c.tunnel
		dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return t.DialContext(ctx, "tcp", addr)
		}
	}
	var db *sql.DB
	switch cfg.Engine {
	case "postgres":
		db, err = openPostgres(cfg, pw, write, dial)
	case "mysql":
		db, err = openMySQL(ctx, cfg, pw, write, dial, "transaction_read_only")
	case "sqlite":
		db, err = openSQLite(cfg, write)
	default:
		err = fmt.Errorf("unknown engine %q", cfg.Engine)
	}
	if err != nil {
		return nil, err
	}
	if write {
		db.SetMaxOpenConns(2)
	} else {
		db.SetMaxOpenConns(6)
	}
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		db.Close()
		if cfg.Engine == "mysql" && !write && strings.Contains(err.Error(), "transaction_read_only") {
			// MariaDB before 11.1 names the variable tx_read_only.
			db, err2 := openMySQL(ctx, cfg, pw, write, dial, "tx_read_only")
			if err2 == nil {
				if err2 = db.PingContext(pctx); err2 == nil {
					db.SetMaxOpenConns(6)
					db.SetMaxIdleConns(2)
					return db, nil
				}
				db.Close()
			}
		}
		return nil, friendly(err)
	}
	return db, nil
}

func openPostgres(cfg config.Connection, pw string, write bool, dial func(context.Context, string, string) (net.Conn, error)) (*sql.DB, error) {
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(orDefault(cfg.Host, "localhost"), strconv.Itoa(orPort(cfg.Port, 5432))), Path: "/" + cfg.Database}
	if cfg.User != "" {
		if pw != "" {
			u.User = url.UserPassword(cfg.User, pw)
		} else {
			u.User = url.User(cfg.User)
		}
	}
	q := url.Values{}
	q.Set("sslmode", orDefault(cfg.SSLMode, "prefer"))
	if cfg.SSLRootCert != "" {
		q.Set("sslrootcert", config.Expand(cfg.SSLRootCert))
	}
	q.Set("application_name", "rowan")
	q.Set("connect_timeout", "10")
	u.RawQuery = q.Encode()
	pc, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, err
	}
	if !write {
		pc.RuntimeParams["default_transaction_read_only"] = "on"
	}
	if dial != nil {
		pc.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) { return dial(ctx, network, addr) }
		// Resolve names on the far side of the tunnel.
		pc.LookupFunc = func(ctx context.Context, host string) ([]string, error) { return []string{host}, nil }
	}
	return stdlib.OpenDB(*pc), nil
}

func openMySQL(_ context.Context, cfg config.Connection, pw string, write bool, dial func(context.Context, string, string) (net.Conn, error), roVar string) (*sql.DB, error) {
	mc := mysql.NewConfig()
	mc.User = cfg.User
	mc.Passwd = pw
	mc.Net = "tcp"
	mc.Addr = net.JoinHostPort(orDefault(cfg.Host, "localhost"), strconv.Itoa(orPort(cfg.Port, 3306)))
	mc.DBName = cfg.Database
	mc.Timeout = 10 * time.Second
	mc.ParseTime = false
	mc.Params = map[string]string{}
	if !write {
		mc.Params[roVar] = "1"
	}
	switch cfg.SSLMode {
	case "disabled":
		mc.TLSConfig = "false"
	case "required":
		mc.TLSConfig = "skip-verify"
	case "verify-ca", "verify-identity":
		mc.TLSConfig = "true"
	default:
		mc.TLSConfig = "preferred"
	}
	if dial != nil {
		name := "rowan-ssh-" + cfg.ID
		mysql.RegisterDialContext(name, func(ctx context.Context, addr string) (net.Conn, error) { return dial(ctx, "tcp", addr) })
		mc.Net = name
	}
	conn, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}

func openSQLite(cfg config.Connection, write bool) (*sql.DB, error) {
	path := config.Expand(cfg.File)
	if path == "" {
		return nil, errors.New("choose a database file first")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("can't open %s: the file does not exist", cfg.File)
	}
	esc := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path)
	dsn := "file:" + esc + "?_busy_timeout=5000&_foreign_keys=1"
	if write {
		dsn += "&mode=rw"
	} else {
		dsn += "&mode=ro&_query_only=1"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if write {
		db.SetMaxOpenConns(1)
	}
	return db, nil
}

// Version returns the server version text, such as "PostgreSQL 16.3".
func (c *Conn) Version(ctx context.Context) (string, error) {
	c.mu.Lock()
	v := c.version
	c.mu.Unlock()
	if v != "" {
		return v, nil
	}
	db, err := c.Reader(ctx)
	if err != nil {
		return "", err
	}
	var raw string
	switch c.Engine() {
	case "postgres":
		err = db.QueryRowContext(ctx, "SHOW server_version").Scan(&raw)
		raw = "PostgreSQL " + strings.Fields(raw + " ")[0]
	case "mysql":
		err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&raw)
		if strings.Contains(strings.ToLower(raw), "mariadb") {
			raw = "MariaDB " + strings.SplitN(raw, "-", 2)[0]
		} else {
			raw = "MySQL " + strings.SplitN(raw, "-", 2)[0]
		}
	case "sqlite":
		err = db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&raw)
		raw = "SQLite " + raw
	}
	if err != nil {
		return "", friendly(err)
	}
	c.mu.Lock()
	c.version = raw
	c.mu.Unlock()
	return raw, nil
}

// Test opens a fresh connection with cfg and closes it again. When password
// is not nil, it replaces the saved password.
func Test(ctx context.Context, cfg config.Connection, password *string) (string, time.Duration, error) {
	start := time.Now()
	c := &Conn{cfg: cfg, pw: password, tables: map[string]*Table{}, samples: map[string]*sampleSet{}}
	defer c.Close()
	v, err := c.Version(ctx)
	return v, time.Since(start), err
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func orPort(p, d int) int {
	if p <= 0 {
		return d
	}
	return p
}

// friendly shortens driver errors for display.
func friendly(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "failed to connect to ")
	if i := strings.Index(msg, "server error ("); i >= 0 {
		msg = msg[i+len("server error ("):]
		msg = strings.TrimSuffix(msg, ")")
	}
	if strings.HasPrefix(msg, "ERROR: ") {
		msg = msg[len("ERROR: "):]
	}
	if i := strings.Index(msg, " (SQLSTATE"); i >= 0 {
		msg = msg[:i]
	}
	if errors.Is(err, context.DeadlineExceeded) {
		msg = "the statement timed out"
	}
	if errors.Is(err, context.Canceled) {
		msg = "the statement was canceled"
	}
	return errors.New(msg)
}

// QuoteIdent quotes a name for the engine when the name needs quotes.
func QuoteIdent(engine, name string) string {
	plain := name != ""
	for i, r := range name {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9')) {
			plain = false
			break
		}
	}
	if engine == "mysql" && plain && !reservedMySQL[name] {
		return name
	}
	if engine != "mysql" && plain && !reservedPG[name] {
		return name
	}
	if engine == "mysql" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// TableRef returns the qualified table name for SQL text. PostgreSQL tables
// in the public schema and all MySQL and SQLite tables stay unqualified.
func TableRef(engine, schema, table string) string {
	if engine == "postgres" && schema != "" && schema != "public" {
		return QuoteIdent(engine, schema) + "." + QuoteIdent(engine, table)
	}
	return QuoteIdent(engine, table)
}

// Placeholder returns the bind marker for argument n, counted from 1.
func Placeholder(engine string, n int) string {
	if engine == "postgres" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

var reservedPG = setOf("all analyse analyze and any array as asc asymmetric both case cast check collate column constraint create current_catalog current_date current_role current_time current_timestamp current_user default deferrable desc distinct do else end except false fetch for foreign from grant group having in initially intersect into lateral leading limit localtime localtimestamp not null offset on only or order placing primary references returning select session_user some symmetric table then to trailing true union unique user using variadic when where window with")
var reservedMySQL = setOf("add all alter analyze and as asc before between bigint binary blob both by call cascade case change char character check collate column condition constraint continue convert create cross current_date current_time current_timestamp current_user cursor database databases declare default delete desc describe distinct div double drop dual each else elseif enclosed escaped exists exit explain false fetch float for force foreign from fulltext grant group having if ignore in index infile inner insert int integer interval into is iterate join key keys kill leading leave left like limit lines load lock long loop match mod modifies natural not null numeric on optimize option or order out outer outfile precision primary procedure purge range read reads real references regexp release rename repeat replace require restrict return revoke right rlike schema schemas select set show smallint spatial sql ssl starting table terminated then to trailing trigger true undo union unique unlock unsigned update usage use using values varchar when where while with write xor")

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}
