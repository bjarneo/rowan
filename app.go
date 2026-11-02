package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"rowan/internal/agent"
	"rowan/internal/config"
	"rowan/internal/db"
	"rowan/internal/search"
	"rowan/internal/sqlx"
	"rowan/internal/state"
)

// App is the backend that the frontend calls through Wails bindings.
type App struct {
	// wctx is the Wails context. Wails sets it in OnStartup, which runs in a
	// goroutine at the same time as the webview loads.
	wctx  atomic.Pointer[context.Context]
	cfg   *config.Store
	st    *state.Store
	dbm   *db.Manager
	agent *agent.Agent

	lockMu  sync.Mutex
	unlocks map[string]time.Time // zero time: until relaunch

	runMu sync.Mutex
	runs  map[string]context.CancelFunc

	prevMu     sync.Mutex
	prevCancel context.CancelFunc

	results *resultCache

	firstRun bool
	notice   string
	stop     chan struct{}
	ready    chan struct{} // closed when prepare is done. Bootstrap waits for it.

	emitHook func(event string, data any) // set by the dev server
}

// NewApp loads the config and state. The rest of the setup runs in prepare,
// in the background, while the window and the webview start.
func NewApp() *App {
	cfg, created, err := config.Load()
	a := &App{
		st:      state.Load(),
		dbm:     db.NewManager(),
		unlocks: map[string]time.Time{},
		runs:    map[string]context.CancelFunc{},
		results: newResultCache(24),
		stop:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	if err != nil {
		// Keep running with defaults so the user can see the error.
		a.notice = err.Error() + ". Rowan uses default settings until you fix the file."
		cfg = &config.Store{}
		_ = cfg.Update(func(c *config.Config) error { *c = config.Default(); return nil })
	}
	a.cfg = cfg
	a.firstRun = created
	a.agent = agent.New(a.cfg, a.dbm, a.emit)
	go a.prepare()
	return a
}

// prepare creates the sample database on first run and starts the MCP
// server. Fields that it sets are safe to read after a.ready is closed.
func (a *App) prepare() {
	defer close(a.ready)
	if a.firstRun {
		a.createSample()
	}
	if a.cfg.Get().MCP.Enabled {
		if err := a.agent.Start(); err != nil {
			a.notice = "The MCP server did not start: " + err.Error()
		}
	}
	go a.relockLoop()
}

func (a *App) startup(ctx context.Context) {
	a.wctx.Store(&ctx)
}

// ctx returns the Wails context, or nil before OnStartup.
func (a *App) ctx() context.Context {
	if c := a.wctx.Load(); c != nil {
		return *c
	}
	return nil
}

func (a *App) shutdown(context.Context) {
	<-a.ready
	close(a.stop)
	a.agent.Close()
	_ = a.st.Flush()
	a.dbm.CloseAll()
}

func (a *App) emit(event string, data any) {
	if a.emitHook != nil {
		a.emitHook(event, data)
		return
	}
	if c := a.ctx(); c != nil {
		wr.EventsEmit(c, event, data)
	}
}

func (a *App) toast(msg string) { a.emit("toast", msg) }

// createSample writes a sample SQLite database on first launch so the app
// has something to open.
func (a *App) createSample() {
	path := filepath.Join(config.Dir(), "sample.sqlite")
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		a.notice = err.Error()
		return
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := db.CreateSampleFile(ctx, path); err != nil {
			a.notice = "Rowan could not create the sample database: " + err.Error()
			return
		}
	}
	err := a.cfg.Update(func(c *config.Config) error {
		c.Connections = append(c.Connections, config.Connection{
			ID: "sample", Name: "sample.sqlite", Engine: "sqlite", File: config.DisplayPath(path), Color: "green",
			Agent: config.Policy{Exposed: true, Access: "read-write", ApproveWrites: true, RowLimit: 500},
		})
		c.Snippets = append(c.Snippets,
			config.Snippet{ID: config.NewID("s"), Name: "Refunded orders", SQL: "SELECT id, customer_id, status, total_cents, placed_at\nFROM orders\nWHERE status = 'refunded'\nORDER BY placed_at DESC;"},
			config.Snippet{ID: config.NewID("s"), Name: "Paying customers by country", SQL: "SELECT country, count(*) AS customers, sum(mrr_cents) / 100 AS mrr\nFROM customers\nWHERE plan <> 'free'\nGROUP BY country\nORDER BY mrr DESC;"},
			config.Snippet{ID: config.NewID("s"), Name: "Unpaid invoices", SQL: "SELECT number, order_id, issued_at\nFROM invoices\nWHERE paid_at IS NULL\nORDER BY issued_at;"},
			config.Snippet{ID: config.NewID("s"), Name: "Refund an order", SQL: "UPDATE orders\nSET status = 'refunded'\nWHERE id = 10442;"},
		)
		return nil
	})
	if err != nil {
		a.notice = err.Error()
	}
	a.st.SetLastConn("sample")
}

// ---- Bootstrap ----

// Bootstrap is everything the UI needs on load.
type Bootstrap struct {
	Config     config.Config    `json:"config"`
	State      state.State      `json:"state"`
	Locks      map[string]int64 `json:"locks"`
	Queue      []agent.Item     `json:"queue"`
	Log        []agent.Entry    `json:"log"`
	MCP        agent.Status     `json:"mcp"`
	ConfigPath string           `json:"configPath"`
	Platform   string           `json:"platform"`
	FirstRun   bool             `json:"firstRun"`
	Notice     string           `json:"notice"`
}

// Bootstrap returns the initial state.
func (a *App) Bootstrap() Bootstrap {
	<-a.ready
	return Bootstrap{
		Config:     a.cfg.Get(),
		State:      a.st.Get(),
		Locks:      a.locks(),
		Queue:      a.agent.Queue.List(),
		Log:        a.agent.Log.Recent(300),
		MCP:        a.agent.Status(),
		ConfigPath: config.DisplayPath(config.Path()),
		Platform:   runtime.GOOS,
		FirstRun:   a.firstRun,
		Notice:     a.notice,
	}
}

// ---- Settings and connections ----

// SaveSettings stores the app and safety settings.
func (a *App) SaveSettings(app config.App, safety config.Safety) (config.Config, error) {
	err := a.cfg.Update(func(c *config.Config) error {
		c.App = app
		c.Safety = safety
		return nil
	})
	return a.cfg.Get(), err
}

// ConnForm is the connection editor content.
type ConnForm struct {
	config.Connection
	Password        string `json:"password"`
	PasswordChanged bool   `json:"passwordChanged"`
}

// SaveConnection creates or updates a connection.
func (a *App) SaveConnection(f ConnForm) (config.Connection, error) {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return config.Connection{}, errors.New("give the connection a name")
	}
	cfg := a.cfg.Get()
	isNew := f.ID == ""
	if isNew {
		f.ID = config.Slug(f.Name)
		for i := 2; slices.ContainsFunc(cfg.Connections, func(c config.Connection) bool { return c.ID == f.ID }); i++ {
			f.ID = fmt.Sprintf("%s-%d", config.Slug(f.Name), i)
		}
	}
	c := f.Connection
	var old config.Connection
	for _, x := range cfg.Connections {
		if x.ID == c.ID {
			old = x
		}
	}
	c.Password = old.Password
	if !isNew {
		c.Agent = old.Agent
		c.Agent.Exposed = f.Agent.Exposed
	} else {
		pol := config.DefaultPolicy()
		pol.Exposed = f.Agent.Exposed
		c.Agent = pol
	}
	if f.PasswordChanged || isNew {
		ref, fallback, err := config.SetPassword(c.ID, f.Password)
		if err != nil {
			return config.Connection{}, err
		}
		c.Password = ref
		if fallback {
			a.toast("The system keychain is not available. Rowan saved the password in credentials.json with mode 0600.")
		}
	}
	if c.Engine == "sqlite" {
		c.Host, c.Port, c.User, c.Database, c.SSLMode, c.SSH = "", 0, "", "", "", config.SSH{}
	}
	err := a.cfg.Update(func(cc *config.Config) error {
		for i := range cc.Connections {
			if cc.Connections[i].ID == c.ID {
				cc.Connections[i] = c
				return nil
			}
		}
		cc.Connections = append(cc.Connections, c)
		return nil
	})
	if err != nil {
		return config.Connection{}, err
	}
	saved, _ := a.cfg.Connection(c.ID)
	a.dbm.Conn(saved) // closes old pools when the target changed
	saved.HasPassword = saved.Password != ""
	return saved, nil
}

// DeleteConnection removes a connection and its password.
func (a *App) DeleteConnection(id string) error {
	err := a.cfg.Update(func(c *config.Config) error {
		c.Connections = slices.DeleteFunc(c.Connections, func(x config.Connection) bool { return x.ID == id })
		return nil
	})
	if err != nil {
		return err
	}
	_ = config.DeletePassword(id)
	a.dbm.Close(id)
	a.st.ForgetConn(id)
	a.lockMu.Lock()
	delete(a.unlocks, id)
	a.lockMu.Unlock()
	a.emit("locks", a.locks())
	return nil
}

// TestResult is the outcome of a connection test.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// TestConnection connects with the form settings without saving them.
func (a *App) TestConnection(f ConnForm) TestResult {
	c := f.Connection
	if c.ID == "" {
		c.ID = "test-" + config.Slug(c.Name)
	}
	var pw *string
	if old, ok := a.cfg.Connection(c.ID); ok && !f.PasswordChanged {
		c.Password = old.Password
	} else {
		pw = &f.Password
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ver, d, err := db.Test(ctx, c, pw)
	if err != nil {
		return TestResult{Message: err.Error()}
	}
	via := ""
	if c.SSH.Enabled && c.Engine != "sqlite" {
		via = " via " + c.SSH.Host
	}
	return TestResult{OK: true, Message: fmt.Sprintf("Connected · %s · %d ms%s", ver, d.Milliseconds(), via)}
}

// ChooseFile opens a file dialog and returns the path, shortened with ~.
func (a *App) ChooseFile(title string) string {
	c := a.ctx()
	if c == nil {
		return ""
	}
	p, err := wr.OpenFileDialog(c, wr.OpenDialogOptions{Title: title, ShowHiddenFiles: true})
	if err != nil || p == "" {
		return ""
	}
	return config.DisplayPath(p)
}

// ---- Schema ----

func (a *App) conn(id string) (*db.Conn, config.Connection, error) {
	cn, ok := a.cfg.Connection(id)
	if !ok {
		return nil, cn, errors.New("this connection no longer exists")
	}
	return a.dbm.Conn(cn), cn, nil
}

// Schema lists the tables of a connection.
func (a *App) Schema(id string, refresh bool) (*db.Schema, error) {
	c, _, err := a.conn(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.Schema(ctx, refresh)
}

// Table describes one table.
func (a *App) Table(id, schema, name string) (*db.Table, error) {
	c, _, err := a.conn(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return c.Table(ctx, schema, name)
}

// Columns lists the columns of every table, for SQL completion.
func (a *App) Columns(id string) (*db.ColumnSet, error) {
	c, _, err := a.conn(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return c.Columns(ctx)
}

// ---- Running SQL ----

// RunRequest asks to run a script.
type RunRequest struct {
	RunID string `json:"runId"`
	Conn  string `json:"conn"`
	SQL   string `json:"sql"`
	Limit int    `json:"limit"`
	Force bool   `json:"force"` // skip the destructive-statement confirmation
}

// SourceRef names the table that a result came from.
type SourceRef struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
}

// RunResult is the outcome of Run.
type RunResult struct {
	ID         string      `json:"id"`
	Columns    []db.Column `json:"columns"`
	Rows       [][]any     `json:"rows"`
	Truncated  bool        `json:"truncated"`
	Affected   int64       `json:"affected"`
	Message    string      `json:"message"`
	Ms         float64     `json:"ms"`
	Statements int         `json:"statements"`
	Write      bool        `json:"write"`
	Error      string      `json:"error"`
	Locked     bool        `json:"locked"`
	Confirm    string      `json:"confirm"`
	Source     *SourceRef  `json:"source"`
}

func (a *App) timeout() time.Duration {
	d, err := time.ParseDuration(a.cfg.Get().Safety.StatementTimeout)
	if err != nil || d <= 0 {
		return 60 * time.Second
	}
	return d
}

// Run runs a script. Writes need an unlocked connection.
func (a *App) Run(req RunRequest) RunResult {
	c, cn, err := a.conn(req.Conn)
	if err != nil {
		return RunResult{Error: err.Error()}
	}
	hash := cn.Engine == "mysql"
	stmts := sqlx.Split(req.SQL, hash)
	if len(stmts) == 0 {
		return RunResult{Error: "Nothing to run."}
	}
	write, firstWrite, destructive := false, "", ""
	for _, s := range stmts {
		info := sqlx.Classify(s, hash)
		if info.Write {
			write = true
			if firstWrite == "" {
				firstWrite = info.Verb
			}
		}
		if info.Destructive && destructive == "" {
			switch info.Verb {
			case "DELETE", "UPDATE":
				destructive = info.Verb + " without a WHERE clause changes every row in the table. Run it anyway?"
			default:
				destructive = info.Verb + " can’t be undone. Run it anyway?"
			}
		}
	}
	hist := state.HistoryItem{ID: config.NewID("h"), SQL: strings.TrimSpace(req.SQL), Conn: cn.ID, At: time.Now().UnixMilli()}
	if write && !a.unlocked(cn.ID) {
		hist.Status = "blocked"
		a.st.AddHistory(hist)
		return RunResult{Locked: true, Write: true, Error: fmt.Sprintf("%s is read-only. Unlock writes to run %s.", cn.Name, firstWrite)}
	}
	if write && destructive != "" && !req.Force && a.cfg.Get().Safety.ConfirmDestructive {
		return RunResult{Confirm: destructive, Write: true}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = a.cfg.Get().App.RowLimit
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout())
	defer cancel()
	if req.RunID != "" {
		a.runMu.Lock()
		a.runs[req.RunID] = cancel
		a.runMu.Unlock()
		defer func() {
			a.runMu.Lock()
			delete(a.runs, req.RunID)
			a.runMu.Unlock()
		}()
	}
	res, err := c.Run(ctx, stmts, write, limit)
	if err != nil {
		hist.Status = "error"
		a.st.AddHistory(hist)
		return RunResult{Error: err.Error(), Write: write}
	}
	hist.Status, hist.Ms = "ok", res.Ms
	hist.Rows = int64(len(res.Rows))
	if res.Columns == nil {
		hist.Rows = res.Affected
	}
	a.st.AddHistory(hist)
	out := RunResult{
		Columns: res.Columns, Rows: res.Rows, Truncated: res.Truncated, Affected: res.Affected,
		Message: res.Message, Ms: res.Ms, Statements: res.Statements, Write: write,
	}
	if out.Rows == nil && out.Columns != nil {
		out.Rows = [][]any{}
	}
	if len(stmts) == 1 && res.Columns != nil {
		if name, ok := sqlx.SourceTable(stmts[len(stmts)-1], hash); ok {
			sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
			if t, _ := c.FindTable(sctx, sqlx.Unquote(name)); t != nil {
				out.Source = &SourceRef{Schema: t.Schema, Table: t.Name}
			}
			scancel()
		}
	}
	if res.Columns != nil {
		name := "result"
		if out.Source != nil {
			name = out.Source.Table
		}
		out.ID = a.results.put(name, cn.Engine, res)
	}
	return out
}

// Cancel stops a running script.
func (a *App) Cancel(runID string) {
	a.runMu.Lock()
	cancel := a.runs[runID]
	a.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Explain returns the plan of the first statement.
func (a *App) Explain(req RunRequest) RunResult {
	_, cn, err := a.conn(req.Conn)
	if err != nil {
		return RunResult{Error: err.Error()}
	}
	stmts := sqlx.Split(req.SQL, cn.Engine == "mysql")
	if len(stmts) == 0 {
		return RunResult{Error: "Nothing to explain."}
	}
	s := stmts[0]
	if sqlx.Classify(s, false).Verb == "EXPLAIN" {
		s = strings.TrimSpace(s[len("explain"):])
	}
	prefix := map[string]string{"postgres": "EXPLAIN ", "mysql": "EXPLAIN ", "sqlite": "EXPLAIN QUERY PLAN "}[cn.Engine]
	req.SQL = prefix + s
	return a.Run(req)
}

// ---- Command surface preview ----

// PreviewRequest asks for a table preview with filters read from free text.
type PreviewRequest struct {
	Conn   string `json:"conn"`
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Rest   string `json:"rest"` // the words after the table name
	Limit  int    `json:"limit"`
}

// PreviewResult is a table preview.
type PreviewResult struct {
	Schema        string        `json:"schema"`
	Table         string        `json:"table"`
	Columns       []db.Column   `json:"columns"`
	Rows          [][]any       `json:"rows"`
	Interp        search.Interp `json:"interp"`
	Matched       int64         `json:"matched"`
	MatchedCapped bool          `json:"matchedCapped"`
	Total         int64         `json:"total"`
	TotalEstimate bool          `json:"totalEstimate"`
	SQL           string        `json:"sql"`
	Where         string        `json:"where"`
	Error         string        `json:"error"`
	Canceled      bool          `json:"canceled"`
}

func (a *App) previewCtx() (context.Context, context.CancelFunc) {
	a.prevMu.Lock()
	defer a.prevMu.Unlock()
	if a.prevCancel != nil {
		a.prevCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	a.prevCancel = cancel
	return ctx, cancel
}

// Preview reads a table with the filters in req.Rest. A new call cancels the
// one before it.
func (a *App) Preview(req PreviewRequest) PreviewResult {
	ctx, cancel := a.previewCtx()
	defer cancel()
	out := PreviewResult{Schema: req.Schema, Table: req.Table}
	c, cn, err := a.conn(req.Conn)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	t, err := c.Table(ctx, req.Schema, req.Table)
	if err != nil {
		return previewErr(out, ctx, err)
	}
	terms := search.Terms(req.Rest)
	var enums map[string]map[string]string
	if len(terms) > 0 {
		enums, _ = c.EnumValues(ctx, t)
	}
	out.Interp = search.Interpret(terms, t, enums)
	where, args := search.Where(cn.Engine, t, out.Interp.Filters)
	out.Where = search.Literal(cn.Engine, t, out.Interp.Filters)
	ref := db.TableRef(cn.Engine, t.Schema, t.Name)
	out.SQL = "SELECT * FROM " + ref
	q := "SELECT * FROM " + ref
	if where != "" {
		q += " WHERE " + where
		out.SQL += " WHERE " + out.Where
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	var wg sync.WaitGroup
	var cerr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Matched, out.MatchedCapped, cerr = c.Count(ctx, t.Schema, t.Name, where, args, 100000)
	}()
	cols, rows, _, err := c.Select(ctx, q+" LIMIT "+fmt.Sprint(limit), args, limit)
	wg.Wait()
	if err != nil {
		return previewErr(out, ctx, err)
	}
	if cerr != nil {
		out.Matched = -1
	}
	out.Columns, out.Rows = cols, rows
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	if s, err := c.Schema(ctx, false); err == nil {
		for _, g := range s.Groups {
			for _, x := range g.Tables {
				if x.Schema == t.Schema && x.Name == t.Name {
					out.Total, out.TotalEstimate = x.Rows, x.Estimate
				}
			}
		}
	}
	return out
}

func previewErr(out PreviewResult, ctx context.Context, err error) PreviewResult {
	if ctx.Err() == context.Canceled {
		out.Canceled = true
		return out
	}
	out.Error = err.Error()
	return out
}

// PreviewSQL runs a read-only statement for the "Run as SQL" preview. It
// never runs writes.
func (a *App) PreviewSQL(conn, sql string) RunResult {
	ctx, cancel := a.previewCtx()
	defer cancel()
	c, cn, err := a.conn(conn)
	if err != nil {
		return RunResult{Error: err.Error()}
	}
	stmts := sqlx.Split(sql, cn.Engine == "mysql")
	if len(stmts) != 1 {
		return RunResult{Error: "Preview runs one statement. Open it in the editor to run a script."}
	}
	if sqlx.Classify(stmts[0], cn.Engine == "mysql").Write {
		return RunResult{Write: true}
	}
	res, err := c.Run(ctx, stmts, false, 12)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return RunResult{}
		}
		return RunResult{Error: err.Error()}
	}
	return RunResult{Columns: res.Columns, Rows: res.Rows, Truncated: res.Truncated, Ms: res.Ms, Message: res.Message}
}

// ---- Row inspector ----

// InspectRequest asks for the links of one row.
type InspectRequest struct {
	Conn   string         `json:"conn"`
	Schema string         `json:"schema"`
	Table  string         `json:"table"`
	Row    map[string]any `json:"row"`
}

// Link is a foreign key or reverse reference of a row.
type Link struct {
	Column string `json:"column"`
	Table  string `json:"table"`
	Schema string `json:"schema"`
	Label  string `json:"label"`
	Count  int64  `json:"count"`
	Capped bool   `json:"capped"`
	SQL    string `json:"sql"`
}

// Inspection holds the links of one row.
type Inspection struct {
	FKs   map[string]Link `json:"fks"`
	RefBy []Link          `json:"refBy"`
}

var labelCols = []string{"name", "full_name", "display_name", "title", "email", "username", "user_login", "label", "number", "sku", "slug", "post_title"}

// Inspect resolves foreign keys to a label and counts the rows that point
// at this row.
func (a *App) Inspect(req InspectRequest) Inspection {
	out := Inspection{FKs: map[string]Link{}}
	c, cn, err := a.conn(req.Conn)
	if err != nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	t, err := c.Table(ctx, req.Schema, req.Table)
	if err != nil {
		return out
	}
	e := cn.Engine
	var mu sync.Mutex
	var wg sync.WaitGroup
	n := 0
	for _, col := range t.Columns {
		v, ok := req.Row[col.Name]
		if col.FK == nil || !ok || v == nil || n >= 8 {
			continue
		}
		n++
		fk := *col.FK
		val := fmt.Sprint(v)
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := Link{Column: col.Name, Table: fk.Table, Schema: fk.Schema,
				SQL: "SELECT * FROM " + db.TableRef(e, fk.Schema, fk.Table) + " WHERE " + db.QuoteIdent(e, fk.Column) + " = " + sqlLiteral(val) + ";"}
			if rt, err := c.Table(ctx, fk.Schema, fk.Table); err == nil {
				for _, lc := range labelCols {
					if x := rt.Column(lc); x != nil {
						_, rows, _, err := c.Select(ctx, "SELECT "+db.QuoteIdent(e, lc)+" FROM "+db.TableRef(e, fk.Schema, fk.Table)+" WHERE "+db.QuoteIdent(e, fk.Column)+" = "+db.Placeholder(e, 1)+" LIMIT 1", []any{val}, 1)
						if err == nil && len(rows) == 1 && rows[0][0] != nil {
							l.Label = fmt.Sprint(rows[0][0])
						}
						break
					}
				}
			}
			mu.Lock()
			out.FKs[col.Name] = l
			mu.Unlock()
		}()
	}
	refs := t.ReferencedBy
	if len(refs) > 8 {
		refs = refs[:8]
	}
	out.RefBy = make([]Link, 0, len(refs))
	slots := make([]*Link, len(refs))
	for i, r := range refs {
		v, ok := req.Row[r.Local]
		if !ok || v == nil {
			continue
		}
		val := fmt.Sprint(v)
		wg.Add(1)
		go func() {
			defer wg.Done()
			where := db.QuoteIdent(e, r.Column) + " = " + db.Placeholder(e, 1)
			cnt, capped, err := c.Count(ctx, r.Schema, r.Table, where, []any{val}, 1000)
			if err != nil {
				return
			}
			slots[i] = &Link{Column: r.Column, Table: r.Table, Schema: r.Schema, Count: cnt, Capped: capped,
				SQL: "SELECT * FROM " + db.TableRef(e, r.Schema, r.Table) + " WHERE " + db.QuoteIdent(e, r.Column) + " = " + sqlLiteral(val) + ";"}
		}()
	}
	wg.Wait()
	for _, l := range slots {
		if l != nil {
			out.RefBy = append(out.RefBy, *l)
		}
	}
	return out
}

func sqlLiteral(v string) string {
	isNum := v != ""
	for i, r := range v {
		if !(r >= '0' && r <= '9') && !(i == 0 && r == '-') && r != '.' {
			isNum = false
			break
		}
	}
	if isNum {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// ---- Write unlock ----

func (a *App) unlocked(id string) bool {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	until, ok := a.unlocks[id]
	return ok && (until.IsZero() || time.Now().Before(until))
}

func (a *App) locks() map[string]int64 {
	a.lockMu.Lock()
	defer a.lockMu.Unlock()
	out := map[string]int64{}
	for id, until := range a.unlocks {
		if until.IsZero() {
			out[id] = -1
		} else {
			out[id] = until.UnixMilli()
		}
	}
	return out
}

// Unlock allows writes on a connection for minutes, or until relaunch when
// minutes is 0. Production connections need the typed connection name.
func (a *App) Unlock(id string, minutes int, typed string) error {
	cn, ok := a.cfg.Connection(id)
	if !ok {
		return errors.New("this connection no longer exists")
	}
	if cn.Production && a.cfg.Get().Safety.ConfirmProductionUnlock && strings.TrimSpace(typed) != cn.Name {
		return errors.New("type the connection name to unlock writes")
	}
	var until time.Time
	if minutes > 0 {
		until = time.Now().Add(time.Duration(minutes) * time.Minute)
	}
	a.lockMu.Lock()
	a.unlocks[id] = until
	a.lockMu.Unlock()
	a.emit("locks", a.locks())
	return nil
}

// Relock makes a connection read-only again.
func (a *App) Relock(id string) {
	a.lockMu.Lock()
	delete(a.unlocks, id)
	a.lockMu.Unlock()
	a.dbm.CloseWriters(id)
	a.emit("locks", a.locks())
}

func (a *App) relockLoop() {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-a.stop:
			return
		case now := <-tk.C:
			var expired []string
			a.lockMu.Lock()
			for id, until := range a.unlocks {
				if !until.IsZero() && !now.Before(until) {
					delete(a.unlocks, id)
					expired = append(expired, id)
				}
			}
			a.lockMu.Unlock()
			if len(expired) == 0 {
				continue
			}
			var names []string
			for _, id := range expired {
				a.dbm.CloseWriters(id)
				if cn, ok := a.cfg.Connection(id); ok {
					names = append(names, cn.Name)
				}
			}
			a.emit("locks", a.locks())
			a.toast("Writes relocked on " + strings.Join(names, ", "))
		}
	}
}

// ---- MCP ----

// Decide approves or denies queued agent writes.
func (a *App) Decide(ids []string, approve bool) int {
	return a.agent.Decide(ids, approve)
}

// SetPolicy changes the agent policy of a connection.
func (a *App) SetPolicy(id string, p config.Policy) (config.Config, error) {
	err := a.cfg.Update(func(c *config.Config) error {
		for i := range c.Connections {
			if c.Connections[i].ID == id {
				c.Connections[i].Agent = p
				return nil
			}
		}
		return errors.New("this connection no longer exists")
	})
	return a.cfg.Get(), err
}

// SetMCP changes the server settings and starts or stops it.
func (a *App) SetMCP(m config.MCP) (agent.Status, error) {
	old := a.cfg.Get().MCP
	if err := a.cfg.Update(func(c *config.Config) error { c.MCP = m; return nil }); err != nil {
		return a.agent.Status(), err
	}
	if !m.Enabled || old.HTTPPort != m.HTTPPort {
		a.agent.Stop()
	}
	var err error
	if m.Enabled {
		err = a.agent.Start()
	}
	st := a.agent.Status()
	a.emit("clients", st)
	return st, err
}

// McpSetup is a config snippet for one client.
type McpSetup struct {
	File string `json:"file"`
	Code string `json:"code"`
}

// McpSetupFor returns the setup snippet for claude-desktop, claude-code,
// cursor or http.
func (a *App) McpSetupFor(client string) McpSetup {
	exe, _ := os.Executable()
	if exe == "" {
		exe = "rowan"
	}
	port := a.cfg.Get().MCP.HTTPPort
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	tok := a.agent.Token()
	switch client {
	case "claude-code":
		return McpSetup{File: "Terminal", Code: "claude mcp add rowan -- " + shellQuote(exe) + " mcp --stdio"}
	case "cursor":
		return McpSetup{File: "~/.cursor/mcp.json", Code: fmt.Sprintf("{\n  \"mcpServers\": {\n    \"rowan\": {\n      \"url\": %q,\n      \"headers\": { \"Authorization\": \"Bearer %s\" }\n    }\n  }\n}", url, tok)}
	case "http":
		return McpSetup{File: "Streamable HTTP", Code: "POST " + url + "\nAuthorization: Bearer " + tok}
	}
	return McpSetup{File: "claude_desktop_config.json", Code: fmt.Sprintf("{\n  \"mcpServers\": {\n    \"rowan\": {\n      \"command\": %q,\n      \"args\": [\"mcp\", \"--stdio\"]\n    }\n  }\n}", exe)}
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " '\"$`\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ActivityLog returns the newest agent calls.
func (a *App) ActivityLog(n int) []agent.Entry { return a.agent.Log.Recent(n) }

// ---- Snippets, recents, drafts ----

// SaveSnippet adds a saved query.
func (a *App) SaveSnippet(name, sql string) (config.Config, error) {
	err := a.cfg.Update(func(c *config.Config) error {
		c.Snippets = append([]config.Snippet{{ID: config.NewID("s"), Name: strings.TrimSpace(name), SQL: strings.TrimSpace(sql)}}, c.Snippets...)
		return nil
	})
	return a.cfg.Get(), err
}

// DeleteSnippet removes a saved query.
func (a *App) DeleteSnippet(id string) (config.Config, error) {
	err := a.cfg.Update(func(c *config.Config) error {
		c.Snippets = slices.DeleteFunc(c.Snippets, func(s config.Snippet) bool { return s.ID == id })
		return nil
	})
	return a.cfg.Get(), err
}

// AddRecent records an opened item.
func (a *App) AddRecent(r state.Recent) {
	r.At = time.Now().UnixMilli()
	a.st.AddRecent(r)
}

// SetDraft stores the editor text of a connection.
func (a *App) SetDraft(conn, sql string) { a.st.SetDraft(conn, sql) }

// SetLastConn stores the active connection.
func (a *App) SetLastConn(id string) { a.st.SetLastConn(id) }

// History returns the newest runs.
func (a *App) History() []state.HistoryItem { return a.st.Get().History }
