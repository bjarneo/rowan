package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"rowan/internal/config"
	"rowan/internal/db"
	"rowan/internal/sqlx"
)

type toolDef struct {
	name, title, desc string
	props             map[string]string
	required          []string
	readOnly          bool
}

var tools = []toolDef{
	{"list_connections", "List connections", "Lists the database connections that the user shared with agents, with engine, access level and row limit.", nil, nil, true},
	{"list_tables", "List tables", "Lists the tables and views that a connection allows, with estimated row counts. A list of more than 200 tables stops at 200 and gives the table count of each schema. Use schema or filter to see the rest.", map[string]string{"connection": "Connection name from list_connections", "schema": "Optional. Only tables in this schema", "filter": "Optional. Only tables whose name contains this text, case-insensitive"}, []string{"connection"}, true},
	{"describe_table", "Describe table", "Returns the columns, types, keys, indexes and references of one table.", map[string]string{"connection": "Connection name", "table": "Table name, optionally schema-qualified"}, []string{"connection", "table"}, true},
	{"query", "Query", "Runs one read-only statement (SELECT, WITH, SHOW). The database session is read-only. Results are capped at the connection's row limit.", map[string]string{"connection": "Connection name", "sql": "One read-only SQL statement"}, []string{"connection", "sql"}, true},
	{"explain", "Explain", "Returns the query plan of one statement without running it.", map[string]string{"connection": "Connection name", "sql": "One SQL statement"}, []string{"connection", "sql"}, true},
	{"execute", "Execute", "Runs one INSERT, UPDATE, DELETE or DDL statement on a connection with read-write access. When approvals are on, the call waits until the user approves or denies the write in Rowan.", map[string]string{"connection": "Connection name", "sql": "One SQL statement that changes data", "reason": "One sentence that tells the user why this write is needed"}, []string{"connection", "sql", "reason"}, false},
}

func (a *Agent) toolList() []any {
	enabled := a.store.Get().MCP.Tools
	var out []any
	for _, t := range tools {
		if !slices.Contains(enabled, t.name) {
			continue
		}
		props := map[string]any{}
		for k, v := range t.props {
			props[k] = map[string]any{"type": "string", "description": v}
		}
		schema := map[string]any{"type": "object", "properties": props}
		if len(t.required) > 0 {
			schema["required"] = t.required
		}
		out = append(out, map[string]any{
			"name": t.name, "title": t.title, "description": t.desc, "inputSchema": schema,
			"annotations": map[string]any{"readOnlyHint": t.readOnly, "destructiveHint": !t.readOnly, "openWorldHint": false},
		})
	}
	return out
}

type toolResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError"`
}

func text(s string) toolResult {
	return toolResult{Content: []map[string]any{{"type": "text", "text": s}}}
}

func failure(s string) toolResult {
	r := text(s)
	r.IsError = true
	return r
}

func jsonText(v any) toolResult {
	b, _ := json.MarshalIndent(v, "", " ")
	return text(string(b))
}

type args struct {
	Connection string `json:"connection"`
	Table      string `json:"table"`
	SQL        string `json:"sql"`
	Reason     string `json:"reason"`
	Schema     string `json:"schema"`
	Filter     string `json:"filter"`
}

// maxListTables caps list_tables, so a large database does not fill the
// context of the agent.
const maxListTables = 200

func (a *Agent) call(ctx context.Context, s *Session, name string, raw json.RawMessage) toolResult {
	cfg := a.store.Get()
	var in args
	_ = json.Unmarshal(raw, &in)
	start := time.Now()
	e := Entry{ID: config.NewID("l"), At: start.UnixMilli(), Client: s.Client, Tool: name, SQL: strings.TrimSpace(in.SQL)}
	record := func(status, note string) {
		e.Status, e.Note = status, note
		e.Ms = float64(time.Since(start).Microseconds()) / 1000
		a.Log.Add(e)
		a.emit("log", e)
	}
	if slices.Contains(cfg.MCP.BlockedClients, s.Client) {
		record("blocked", s.Client+" is blocked in Rowan")
		return failure("The user blocked this client in Rowan.")
	}
	if !slices.Contains(cfg.MCP.Tools, name) {
		record("blocked", "The "+name+" tool is turned off")
		return failure("The " + name + " tool is turned off in Rowan.")
	}
	if name == "list_connections" {
		var list []map[string]any
		for _, c := range cfg.Connections {
			if !c.Agent.Exposed {
				continue
			}
			list = append(list, map[string]any{
				"name": c.Name, "engine": db.EngineLabel[c.Engine], "access": c.Agent.Access,
				"approve_writes": c.Agent.ApproveWrites || c.Production, "row_limit": c.Agent.RowLimit, "production": c.Production,
			})
		}
		e.Rows = int64(len(list))
		record("allowed", "")
		if len(list) == 0 {
			return text("The user has not shared any connections with agents. They can share one in Rowan > MCP server.")
		}
		return jsonText(list)
	}
	cn, ok := findConn(cfg, in.Connection)
	if !ok {
		e.Conn = in.Connection
		record("blocked", "Unknown or hidden connection")
		return failure(fmt.Sprintf("No shared connection is named %q. Call list_connections.", in.Connection))
	}
	e.Conn = cn.ID
	if name == "describe_table" {
		e.SQL = in.Table
	}
	pol := cn.Agent
	conn := a.dbm.Conn(cn)
	// Reads time out after 30 seconds. execute can wait for the user, so it
	// only ends when the client goes away.
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	if name == "execute" {
		cancel()
		ctx, cancel = context.WithCancel(parent)
	}
	defer cancel()

	switch name {
	case "list_tables":
		sch, err := conn.Schema(ctx, false)
		if err != nil {
			record("error", err.Error())
			return failure(err.Error())
		}
		list := []map[string]any{}
		counts := map[string]int{}
		filter := strings.ToLower(strings.TrimSpace(in.Filter))
		for _, g := range sch.Groups {
			for _, t := range g.Tables {
				if !allowed(pol, t.Name) || (in.Schema != "" && !strings.EqualFold(t.Schema, in.Schema)) || (filter != "" && !strings.Contains(strings.ToLower(t.Name), filter)) {
					continue
				}
				counts[t.Schema]++
				list = append(list, map[string]any{"schema": t.Schema, "name": t.Name, "kind": t.Kind, "rows": t.Rows, "rows_estimated": t.Estimate})
			}
		}
		e.Rows = int64(len(list))
		out := map[string]any{"connection": cn.Name, "server": sch.Version, "tables": list}
		var notes []string
		if len(pol.Tables) > 0 {
			notes = append(notes, "Tables outside the allowlist are hidden")
		}
		if len(list) > maxListTables {
			e.Rows = maxListTables
			out["tables"] = list[:maxListTables]
			out["total"] = len(list)
			out["tables_per_schema"] = counts
			out["note"] = fmt.Sprintf("This list shows %d of %d tables. Use the schema or filter argument to see the rest.", maxListTables, len(list))
			notes = append(notes, fmt.Sprintf("Showed %d of %d tables", maxListTables, len(list)))
		}
		record("allowed", strings.Join(notes, ". "))
		return jsonText(out)

	case "describe_table":
		if !allowed(pol, lastPart(in.Table)) {
			record("blocked", lastPart(in.Table)+" is not in the allowlist for "+cn.Name)
			return failure(lastPart(in.Table) + " is not in the allowlist for " + cn.Name + ".")
		}
		ts, err := conn.FindTable(ctx, in.Table)
		if err != nil || ts == nil {
			record("error", "Table not found")
			return failure(fmt.Sprintf("Table %q does not exist on %s.", in.Table, cn.Name))
		}
		t, err := conn.Table(ctx, ts.Schema, ts.Name)
		if err != nil {
			record("error", err.Error())
			return failure(err.Error())
		}
		var refs []db.Ref
		for _, r := range t.ReferencedBy {
			if allowed(pol, r.Table) {
				refs = append(refs, r)
			}
		}
		out := *t
		out.ReferencedBy = refs
		e.Rows = int64(len(t.Columns))
		record("allowed", "")
		return jsonText(out)
	}

	// query, explain and execute take one SQL statement.
	stmts := sqlx.Split(in.SQL, cn.Engine == "mysql")
	if len(stmts) != 1 {
		record("blocked", "Sent more than one statement")
		return failure("Send exactly one SQL statement per call.")
	}
	stmt := stmts[0]
	if hidden := hiddenTables(ctx, conn, pol, stmt); hidden != "" {
		record("blocked", hidden+" is not in the allowlist for "+cn.Name)
		return failure(hidden + " is not in the allowlist for " + cn.Name + ".")
	}
	info := sqlx.Classify(stmt, cn.Engine == "mysql")

	switch name {
	case "query":
		if info.Write {
			record("blocked", "query only runs read-only statements")
			return failure("query only runs read-only statements. Use execute for writes.")
		}
		if !info.Rows {
			record("blocked", "query only runs statements that return rows")
			return failure("query only runs statements that return rows, such as SELECT, WITH, SHOW and EXPLAIN.")
		}
		res, err := conn.Run(ctx, []string{stmt}, false, pol.RowLimit)
		if err != nil {
			record("error", err.Error())
			return failure(err.Error())
		}
		e.Rows = int64(len(res.Rows))
		names := make([]string, len(res.Columns))
		for i, c := range res.Columns {
			names[i] = c.Name
		}
		out := map[string]any{"columns": names, "rows": res.Rows, "row_count": len(res.Rows)}
		if res.Truncated {
			note := fmt.Sprintf("Capped at the row limit of %d rows", pol.RowLimit)
			out["truncated"] = true
			out["note"] = note + ". Add a WHERE clause or LIMIT to see other rows."
			record("truncated", note)
		} else {
			record("allowed", "")
		}
		return jsonText(out)

	case "explain":
		if sqlx.Classify(stmt, false).Verb == "EXPLAIN" {
			stmt = strings.TrimSpace(stmt[len("explain"):])
		}
		for _, w := range sqlx.Significant(sqlx.Lex(stmt, cn.Engine == "mysql")) {
			if w.Is("analyze") || w.Is("analyse") {
				record("blocked", "EXPLAIN ANALYZE runs the statement")
				return failure("explain does not run statements, so ANALYZE is not allowed. Use query instead.")
			}
		}
		prefix := map[string]string{"postgres": "EXPLAIN ", "mysql": "EXPLAIN ", "sqlite": "EXPLAIN QUERY PLAN "}[cn.Engine]
		res, err := conn.Run(ctx, []string{prefix + stmt}, false, 500)
		if err != nil {
			record("error", err.Error())
			return failure(err.Error())
		}
		e.Rows = int64(len(res.Rows))
		record("allowed", "")
		var b strings.Builder
		for _, r := range res.Rows {
			for i, v := range r {
				if i > 0 {
					b.WriteString(" | ")
				}
				if v == nil {
					b.WriteString("NULL")
				} else {
					b.WriteString(fmt.Sprint(v))
				}
			}
			b.WriteByte('\n')
		}
		return text(b.String())

	case "execute":
		if pol.Access != "read-write" {
			record("blocked", cn.Name+" is read-only for agents")
			return failure(cn.Name + " is read-only for agents. The user can change this in Rowan > MCP server.")
		}
		if !info.Write {
			record("blocked", "execute only runs writes")
			return failure("execute is for writes. Use query for read-only statements.")
		}
		e.Reason = strings.TrimSpace(in.Reason)
		if pol.ApproveWrites || cn.Production {
			return a.waitForApproval(ctx, cn, conn, stmt, e, start)
		}
		res, err := conn.Run(ctx, []string{stmt}, true, pol.RowLimit)
		if err != nil {
			record("error", err.Error())
			return failure(err.Error())
		}
		e.Rows = res.Affected
		record("approved", "Ran without approval by connection policy")
		return text(resultText(res))
	}
	record("error", "Unknown tool")
	return failure("Unknown tool " + name)
}

func (a *Agent) waitForApproval(ctx context.Context, cn config.Connection, conn *db.Conn, stmt string, e Entry, start time.Time) toolResult {
	it := &Item{ID: config.NewID("q"), At: time.Now().UnixMilli(), Client: e.Client, Conn: cn.ID, Reason: e.Reason, SQL: stmt}
	pctx, pcancel := context.WithTimeout(ctx, 3*time.Second)
	it.Impact, it.Changes = Impact(pctx, conn, stmt)
	pcancel()
	a.Queue.add(it)
	a.emitQueue()
	a.emit("toast", fmt.Sprintf("%s wants to write to %s", e.Client, cn.Name))

	record := func(status, note string) {
		e.Status, e.Note = status, note
		e.At = time.Now().UnixMilli()
		e.Ms = float64(time.Since(start).Microseconds()) / 1000
		a.Log.Add(e)
		a.emit("log", e)
	}
	select {
	case ok := <-it.decide:
		if !ok {
			record("denied", "Denied by you")
			return failure("The user denied this write in Rowan.")
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		res, err := conn.Run(rctx, []string{stmt}, true, cn.Agent.RowLimit)
		if err != nil {
			record("error", "Approved, then failed: "+err.Error())
			return failure("The user approved the write, but it failed: " + err.Error())
		}
		e.Rows = res.Affected
		record("approved", "Approved by you")
		return text("The user approved the write. " + resultText(res))
	case <-ctx.Done():
		if a.Queue.take(it.ID) != nil {
			a.emitQueue()
			record("denied", "The client stopped waiting for approval")
		}
		return failure("The request was canceled before the user decided.")
	}
}

// Decide approves or denies queued writes.
func (a *Agent) Decide(ids []string, approve bool) int {
	n := 0
	for _, id := range ids {
		if it := a.Queue.take(id); it != nil {
			it.decide <- approve
			n++
		}
	}
	a.emitQueue()
	return n
}

func resultText(res *db.Result) string {
	if res.Message != "" {
		return res.Message + "."
	}
	return fmt.Sprintf("%d rows affected.", res.Affected)
}

func findConn(cfg config.Config, name string) (config.Connection, bool) {
	for _, c := range cfg.Connections {
		if c.Agent.Exposed && (strings.EqualFold(c.Name, name) || c.ID == name) {
			return c, true
		}
	}
	return config.Connection{}, false
}

func allowed(p config.Policy, table string) bool {
	if len(p.Tables) == 0 {
		return true
	}
	for _, t := range p.Tables {
		if strings.EqualFold(t, table) {
			return true
		}
	}
	return false
}

func lastPart(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return sqlx.Unquote(s[i+1:])
	}
	return sqlx.Unquote(s)
}

// hiddenTables returns the first table outside the allowlist that the
// statement names, or "". It errs on the side of blocking: a column that
// shares a name with a hidden table also blocks the statement.
func hiddenTables(ctx context.Context, conn *db.Conn, p config.Policy, stmt string) string {
	if len(p.Tables) == 0 {
		return ""
	}
	sch, err := conn.Schema(ctx, false)
	if err != nil {
		return ""
	}
	hidden := map[string]string{}
	for _, g := range sch.Groups {
		for _, t := range g.Tables {
			if !allowed(p, t.Name) {
				hidden[strings.ToLower(t.Name)] = t.Name
			}
		}
	}
	for _, id := range sqlx.Identifiers(stmt, conn.Engine() == "mysql") {
		if n, ok := hidden[id]; ok {
			return n
		}
	}
	return ""
}

// Impact describes what a write would change. It reads the database with
// the read-only pool and never runs the write.
func Impact(ctx context.Context, conn *db.Conn, stmt string) (string, []Change) {
	hash := conn.Engine() == "mysql"
	if ins, ok := sqlx.ParseInsert(stmt, hash); ok {
		var ch []Change
		for i, c := range ins.Columns {
			if i < len(ins.First) {
				ch = append(ch, Change{Column: c, To: unquoteLit(ins.First[i])})
			}
		}
		return fmt.Sprintf("Inserts %d row%s into %s", ins.Rows, plural(ins.Rows), ins.Table), ch
	}
	t, ok := sqlx.ParseTarget(stmt, hash)
	if !ok {
		info := sqlx.Classify(stmt, hash)
		if info.Destructive {
			return "Changes the schema or deletes data. Rowan can't preview it", nil
		}
		return "", nil
	}
	from := t.Table
	if t.Alias != "" {
		from += " " + t.Alias
	}
	where := ""
	if t.Where != "" {
		where = " WHERE " + t.Where
	}
	var n int64
	const cap = 100000
	_, rows, _, err := conn.Select(ctx, "SELECT count(*) FROM (SELECT 1 FROM "+from+where+" LIMIT "+fmt.Sprint(cap+1)+") rowan_impact", nil, 1)
	if err != nil || len(rows) == 0 {
		return "Rowan can't count the rows this changes", nil
	}
	fmt.Sscan(fmt.Sprint(rows[0][0]), &n)
	count := fmt.Sprintf("%d", n)
	if n > cap {
		count = fmt.Sprintf("more than %d", cap)
	}
	if t.Verb == "DELETE" {
		return fmt.Sprintf("Deletes %s row%s from %s", count, plural(int(n)), t.Table), nil
	}
	impact := fmt.Sprintf("Updates %s row%s in %s", count, plural(int(n)), t.Table)
	var cols []string
	for _, s := range t.Sets {
		cols = append(cols, db.QuoteIdent(conn.Engine(), s.Column))
	}
	_, prev, _, err := conn.Select(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM "+from+where+" LIMIT 2", nil, 2)
	if err != nil {
		return impact, nil
	}
	var ch []Change
	for _, r := range prev {
		for i, s := range t.Sets {
			ch = append(ch, Change{Column: s.Column, From: display(r[i]), To: unquoteLit(s.Expr)})
		}
	}
	return impact, ch
}

func display(v any) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprint(v)
}

func unquoteLit(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ErrNotRunning tells the stdio bridge that the app is closed.
var ErrNotRunning = errors.New("Rowan is not running. Open the Rowan app, then try again")
