package db

import (
	"context"
	"database/sql"
	"strconv"
)

// ColumnSet lists the columns of the tables and views on a connection. The
// editor uses it for SQL completion.
type ColumnSet struct {
	Tables []TableColumns `json:"tables"`
	// Truncated is true when the schema has more than maxColumnSet columns.
	// The editor then loads the columns of a table when it needs them.
	Truncated bool `json:"truncated"`
}

// TableColumns holds the column names and types of one table, in table order.
type TableColumns struct {
	Schema  string     `json:"schema"`
	Name    string     `json:"name"`
	Columns []ColBrief `json:"columns"`
}

// ColBrief is one column in a ColumnSet.
type ColBrief struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// maxColumnSet caps the size of a ColumnSet. A schema with 1000 tables of 20
// columns fits.
const maxColumnSet = 20000

// Columns returns the columns of every table and view in one catalog query.
// It caches the result until a write runs or the schema is refreshed.
func (c *Conn) Columns(ctx context.Context) (*ColumnSet, error) {
	c.cacheMu.Lock()
	if s := c.columns; s != nil {
		c.cacheMu.Unlock()
		return s, nil
	}
	c.cacheMu.Unlock()
	db, err := c.Reader(ctx)
	if err != nil {
		return nil, err
	}
	limit := " LIMIT " + strconv.Itoa(maxColumnSet+1)
	var q string
	switch c.Engine() {
	case "postgres":
		// The same relations as pgTables. The default schema comes first, so a
		// truncated set keeps it.
		q = `
SELECT n.nspname, c.relname, a.attname, format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE a.attnum > 0 AND NOT a.attisdropped
  AND c.relkind IN ('r','p','v','m','f')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%'
  AND NOT c.relispartition
ORDER BY n.nspname <> 'public', n.nspname, c.relname, a.attnum` + limit
	case "mysql":
		q = `
SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, COLUMN_TYPE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
ORDER BY TABLE_NAME, ORDINAL_POSITION` + limit
	case "sqlite":
		q = `
SELECT 'main', m.name, p.name, p.type
FROM sqlite_master m JOIN pragma_table_info(m.name) p
WHERE m.type IN ('table','view') AND m.name NOT LIKE 'sqlite_%'
ORDER BY m.name, p.cid` + limit
	}
	s, err := scanColumnSet(ctx, db, q)
	if err != nil {
		return nil, friendly(err)
	}
	c.cacheMu.Lock()
	c.columns = s
	c.cacheMu.Unlock()
	return s, nil
}

func scanColumnSet(ctx context.Context, db *sql.DB, q string) (*ColumnSet, error) {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	s := &ColumnSet{Tables: []TableColumns{}}
	n := 0
	var schema, table string
	var col ColBrief
	for rows.Next() {
		if n == maxColumnSet {
			s.Truncated = true
			break
		}
		if err := rows.Scan(&schema, &table, &col.Name, &col.Type); err != nil {
			return nil, err
		}
		n++
		last := len(s.Tables) - 1
		if last < 0 || s.Tables[last].Schema != schema || s.Tables[last].Name != table {
			s.Tables = append(s.Tables, TableColumns{Schema: schema, Name: table})
			last++
		}
		s.Tables[last].Columns = append(s.Tables[last].Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A truncated set can end in the middle of a table. Drop that table, so
	// the editor loads it in full when it needs it.
	if s.Truncated && len(s.Tables) > 0 {
		s.Tables = s.Tables[:len(s.Tables)-1]
	}
	return s, nil
}
