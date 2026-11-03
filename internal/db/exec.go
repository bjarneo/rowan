package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"rowan/internal/sqlx"
)

// Column describes one result column.
type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Numeric bool   `json:"numeric"`
}

// Result is the outcome of a script. Rows hold display strings, or nil for NULL.
type Result struct {
	Columns    []Column `json:"columns"`
	Rows       Rows     `json:"rows"`
	Truncated  bool     `json:"truncated"`
	Affected   int64    `json:"affected"`
	Message    string   `json:"message"`
	Statements int      `json:"statements"`
	Ms         float64  `json:"ms"`
}

// maxCell caps the length of one cell sent to the UI.
const maxCell = 20000

// Run executes statements in order on one session. When write is true, it
// uses the write pool and wraps the script in a transaction, unless the
// script manages its own transaction. The result holds the rows of the last
// statement that returned rows.
func (c *Conn) Run(ctx context.Context, stmts []string, write bool, limit int) (*Result, error) {
	if limit <= 0 {
		limit = 1000
	}
	var pool *sql.DB
	var err error
	if write {
		pool, err = c.Writer(ctx)
	} else {
		pool, err = c.Reader(ctx)
	}
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, friendly(err)
	}
	engine := c.Engine()
	hash := engine == "mysql"
	infos := make([]sqlx.Info, len(stmts))
	ownTx, session := false, false
	for i, s := range stmts {
		infos[i] = sqlx.Classify(s, hash)
		switch infos[i].Verb {
		case "BEGIN", "START", "COMMIT", "ROLLBACK", "END", "SAVEPOINT", "RELEASE":
			ownTx = true
		case "SET", "RESET", "USE", "PRAGMA":
			session = true
		}
	}
	defer func() {
		// A script that changed session state or managed its own transaction
		// may leave the connection in a state the next query must not see,
		// such as an open transaction. Close it instead of returning it to the pool.
		if ownTx || session {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}()
	start := time.Now()
	res := &Result{Statements: len(stmts)}
	inTx := false
	if write && !ownTx {
		begin := map[string]string{"postgres": "BEGIN READ WRITE", "mysql": "START TRANSACTION", "sqlite": "BEGIN IMMEDIATE"}[engine]
		if _, err := conn.ExecContext(ctx, begin); err != nil {
			return nil, friendly(err)
		}
		inTx = true
	}
	fail := func(i int, err error) (*Result, error) {
		if inTx {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
		if len(stmts) > 1 {
			return nil, fmt.Errorf("statement %d: %w", i+1, friendly(err))
		}
		return nil, friendly(err)
	}
	var msgs []string
	for i, s := range stmts {
		info := infos[i]
		last := i == len(stmts)-1
		if info.Rows {
			q, stop := bound(engine, s, limit)
			cols, rows, trunc, err := query(ctx, conn, q, limit, last && stop)
			if err != nil {
				return fail(i, err)
			}
			res.Columns, res.Rows, res.Truncated = cols, rows, trunc
			if info.Write {
				res.Affected += int64(len(rows))
			}
			continue
		}
		r, err := conn.ExecContext(ctx, s)
		if err != nil {
			return fail(i, err)
		}
		n, _ := r.RowsAffected()
		switch info.Verb {
		case "INSERT", "UPDATE", "DELETE", "REPLACE", "MERGE":
			res.Affected += n
			msgs = append(msgs, fmt.Sprintf("%s %d", info.Verb, n))
		default:
			msgs = append(msgs, info.Verb)
		}
	}
	if inTx {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return fail(len(stmts)-1, err)
		}
	}
	res.Ms = float64(time.Since(start).Microseconds()) / 1000
	res.Message = strings.Join(compact(msgs), " · ")
	if write {
		c.Invalidate()
	}
	return res, nil
}

func compact(msgs []string) []string {
	if len(msgs) <= 3 {
		return msgs
	}
	return append(msgs[:2], fmt.Sprintf("%d more", len(msgs)-2))
}

// bound caps a plain query at limit+1 rows on the server. The planner then
// plans for the rows that Rowan reads, with a top-N sort or an index scan,
// and the server stops on its own, so the connection stays in the pool. The
// extra row tells Rowan that the result has more rows. stop reports whether
// Rowan must still cancel the statement when more rows arrive.
//
// PostgreSQL gets an outer LIMIT. The line breaks keep a trailing comment
// inside the subquery. MySQL gets a sql_select_limit hint, because a derived
// table there rejects duplicate column names. A LIMIT clause in the query
// takes precedence over the hint, so Rowan still cancels in that case.
func bound(engine, stmt string, limit int) (q string, stop bool) {
	switch engine {
	case "postgres":
		if sqlx.Bounded(stmt, false) {
			return "SELECT * FROM (\n" + stmt + "\n) rowan_q LIMIT " + strconv.Itoa(limit+1), false
		}
	case "mysql":
		if sqlx.Bounded(stmt, true) {
			if at := sqlx.HintAt(stmt, true); at >= 0 {
				return stmt[:at] + " /*+ SET_VAR(sql_select_limit = " + strconv.Itoa(limit+1) + ") */" + stmt[at:], sqlx.TopLimit(stmt, true)
			}
		}
	}
	return stmt, true
}

// query reads up to limit rows. When the result has more rows and stop is
// true, it cancels the statement so the server stops work. A canceled MySQL
// statement closes its connection. PostgreSQL gets a cancel request and keeps
// the connection.
func query(ctx context.Context, conn *sql.Conn, stmt string, limit int, stop bool) ([]Column, [][]any, bool, error) {
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rows, err := conn.QueryContext(qctx, stmt)
	if err != nil {
		return nil, nil, false, err
	}
	cols, out, trunc, err := scan(rows, limit, func() {
		if stop {
			cancel()
		}
	})
	rows.Close()
	if err != nil && !trunc {
		return nil, nil, false, err
	}
	return cols, out, trunc, nil
}

func scan(rows *sql.Rows, limit int, stop func()) ([]Column, [][]any, bool, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, false, err
	}
	n := len(types)
	cols := make([]Column, n)
	kinds := make([]string, n)
	for i, t := range types {
		tn := strings.ToUpper(t.DatabaseTypeName())
		kinds[i] = tn
		cols[i] = Column{Name: t.Name(), Type: strings.ToLower(tn), Numeric: isNumericType(tn)}
	}
	vals := make([]any, n)
	ptrs := make([]any, n)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	seenNum := make([]int, n)
	seenAny := make([]int, n)
	out := make([][]any, 0, min(limit, 256))
	trunc := false
	for rows.Next() {
		if len(out) >= limit {
			trunc = true
			stop()
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, false, err
		}
		row := make([]any, n)
		for i, v := range vals {
			if v != nil {
				seenAny[i]++
				switch v.(type) {
				case int64, float64, int32, float32:
					seenNum[i]++
				}
			}
			row[i] = Format(v, kinds[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil && !trunc {
		return nil, nil, false, err
	}
	for i := range cols {
		if !cols[i].Numeric && cols[i].Type == "" && seenAny[i] > 0 && seenNum[i] == seenAny[i] {
			cols[i].Numeric = true
		}
	}
	return cols, out, trunc, nil
}

func isNumericType(t string) bool {
	switch {
	case strings.Contains(t, "INT"), strings.Contains(t, "NUMERIC"), strings.Contains(t, "DECIMAL"),
		strings.Contains(t, "FLOAT"), strings.Contains(t, "DOUBLE"), strings.Contains(t, "REAL"),
		t == "MONEY", t == "OID", strings.Contains(t, "SERIAL"):
		return !strings.Contains(t, "INTERVAL") && !strings.Contains(t, "POINT")
	}
	return false
}

// Format turns a scanned value into a display string, or nil for NULL.
func Format(v any, dbType string) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return capCell(x)
	case []byte:
		if utf8.Valid(x) {
			return capCell(string(x))
		}
		if len(x) > 64 {
			return `\x` + hex.EncodeToString(x[:64]) + "…"
		}
		return `\x` + hex.EncodeToString(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case time.Time:
		return formatTime(x, dbType)
	}
	return capCell(fmt.Sprint(v))
}

func capCell(s string) string {
	if len(s) <= maxCell {
		return s
	}
	cut := maxCell
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func formatTime(t time.Time, dbType string) string {
	switch dbType {
	case "DATE":
		return t.Format("2006-01-02")
	case "TIME":
		return t.Format("15:04:05.999999")
	case "TIMESTAMPTZ":
		u := t.UTC()
		return u.Format("2006-01-02 15:04:05.999999") + "+00"
	}
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 && dbType == "" {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04:05.999999")
}

// Count returns the number of rows in a table that match where, up to cap.
// capped is true when the table has more rows than cap.
func (c *Conn) Count(ctx context.Context, schema, table, where string, args []any, cap int) (int64, bool, error) {
	db, err := c.Reader(ctx)
	if err != nil {
		return 0, false, err
	}
	q := "SELECT count(*) FROM (SELECT 1 FROM " + TableRef(c.Engine(), schema, table)
	if where != "" {
		q += " WHERE " + where
	}
	q += " LIMIT " + strconv.Itoa(cap+1) + ") rowan_count"
	var n int64
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, false, friendly(err)
	}
	if n > int64(cap) {
		return int64(cap), true, nil
	}
	return n, false, nil
}

// Select runs one read-only query with bind arguments.
func (c *Conn) Select(ctx context.Context, q string, args []any, limit int) ([]Column, [][]any, bool, error) {
	db, err := c.Reader(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rows, err := db.QueryContext(qctx, q, args...)
	if err != nil {
		return nil, nil, false, friendly(err)
	}
	defer rows.Close()
	cols, out, trunc, err := scan(rows, limit, cancel)
	if err != nil {
		return nil, nil, false, friendly(err)
	}
	return cols, out, trunc, nil
}
