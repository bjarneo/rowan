package db

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"
	"time"
)

// Schema lists the tables of a connection, grouped by schema.
type Schema struct {
	Version string  `json:"version"`
	Groups  []Group `json:"groups"`
}

// Group is one schema (or database, for MySQL).
type Group struct {
	Name   string         `json:"name"`
	Tables []TableSummary `json:"tables"`
}

// TableSummary is one table or view in a list.
type TableSummary struct {
	Schema   string `json:"schema"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // table or view
	Rows     int64  `json:"rows"`
	Estimate bool   `json:"estimate"`
}

// Table holds the structure of one table.
type Table struct {
	Schema       string       `json:"schema"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Columns      []ColumnInfo `json:"columns"`
	Indexes      []Index      `json:"indexes"`
	ReferencedBy []Ref        `json:"referencedBy"`
}

// ColumnInfo describes one table column.
type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
	PK       bool   `json:"pk"`
	Unique   bool   `json:"unique"`
	FK       *Ref   `json:"fk"`
	Numeric  bool   `json:"numeric"`
}

// Ref links a column to a column in another table. For a foreign key, Local
// is the column in this table and Table.Column is the target. For a
// referenced-by entry, Table.Column is the referencing column and Local is
// the column in this table that it points to.
type Ref struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Column string `json:"column"`
	Local  string `json:"local"`
	// Indexed reports, for a referenced-by entry, that an index starts with
	// the referencing column. Without one, a count of the rows that point
	// at a row reads the whole referencing table.
	Indexed bool `json:"indexed,omitempty"`
}

// Index describes one index.
type Index struct {
	Name    string `json:"name"`
	Columns string `json:"columns"`
	Unique  bool   `json:"unique"`
	Primary bool   `json:"primary"`
}

// Column returns the column with this name.
func (t *Table) Column(name string) *ColumnInfo {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// PK returns the single-column primary key, or nil.
func (t *Table) PK() *ColumnInfo {
	var pk *ColumnInfo
	for i := range t.Columns {
		if t.Columns[i].PK {
			if pk != nil {
				return nil
			}
			pk = &t.Columns[i]
		}
	}
	return pk
}

// Invalidate drops the cached schema after a write.
func (c *Conn) Invalidate() {
	c.cacheMu.Lock()
	c.schema = nil
	c.columns = nil
	c.tables = map[string]*Table{}
	c.samples = map[string]*sampleSet{}
	c.cacheMu.Unlock()
}

// Schema returns the table list. It caches the list until refresh is true or
// a write runs.
func (c *Conn) Schema(ctx context.Context, refresh bool) (*Schema, error) {
	c.cacheMu.Lock()
	if c.schema != nil && !refresh {
		s := c.schema
		c.cacheMu.Unlock()
		return s, nil
	}
	c.cacheMu.Unlock()
	if refresh {
		c.Invalidate()
	}
	db, err := c.Reader(ctx)
	if err != nil {
		return nil, err
	}
	ver, err := c.Version(ctx)
	if err != nil {
		return nil, err
	}
	var list []TableSummary
	switch c.Engine() {
	case "postgres":
		list, err = pgTables(ctx, db)
	case "mysql":
		list, err = myTables(ctx, db)
	case "sqlite":
		list, err = liteTables(ctx, db)
	}
	if err != nil {
		return nil, friendly(err)
	}
	c.exactCounts(ctx, list)
	s := &Schema{Version: ver}
	idx := map[string]int{}
	for _, t := range list {
		i, ok := idx[t.Schema]
		if !ok {
			i = len(s.Groups)
			idx[t.Schema] = i
			s.Groups = append(s.Groups, Group{Name: t.Schema})
		}
		s.Groups[i].Tables = append(s.Groups[i].Tables, t)
	}
	sort.SliceStable(s.Groups, func(a, b int) bool {
		return s.Groups[a].Name == "public" && s.Groups[b].Name != "public"
	})
	c.cacheMu.Lock()
	c.schema = s
	c.cacheMu.Unlock()
	return s, nil
}

// exactCounts replaces estimates with exact counts for small tables. It
// stops after two seconds and keeps the estimates it could not replace.
func (c *Conn) exactCounts(ctx context.Context, list []TableSummary) {
	const cap = 200000
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range list {
		t := &list[i]
		if t.Kind != "table" || (t.Estimate && t.Rows > cap) || len(list) > 150 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			n, capped, err := c.Count(ctx, t.Schema, t.Name, "", nil, cap)
			if err == nil {
				t.Rows, t.Estimate = n, capped
			}
		}()
	}
	wg.Wait()
}

func pgTables(ctx context.Context, db *sql.DB) ([]TableSummary, error) {
	rows, err := db.QueryContext(ctx, `
SELECT n.nspname, c.relname, c.relkind::text, GREATEST(c.reltuples, -1)::bigint
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p','v','m','f')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%'
  AND NOT c.relispartition
ORDER BY 1, c.relkind IN ('v','m'), 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableSummary
	for rows.Next() {
		var t TableSummary
		var kind string
		if err := rows.Scan(&t.Schema, &t.Name, &kind, &t.Rows); err != nil {
			return nil, err
		}
		t.Kind = "table"
		if kind == "v" || kind == "m" {
			t.Kind = "view"
		}
		t.Estimate = true
		if t.Rows < 0 {
			t.Rows = 0
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func myTables(ctx context.Context, db *sql.DB) ([]TableSummary, error) {
	rows, err := db.QueryContext(ctx, `
SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE, COALESCE(TABLE_ROWS, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
ORDER BY TABLE_TYPE = 'VIEW', TABLE_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableSummary
	for rows.Next() {
		var t TableSummary
		var kind string
		if err := rows.Scan(&t.Schema, &t.Name, &kind, &t.Rows); err != nil {
			return nil, err
		}
		t.Kind = "table"
		if strings.Contains(kind, "VIEW") {
			t.Kind = "view"
		}
		t.Estimate = true
		out = append(out, t)
	}
	return out, rows.Err()
}

func liteTables(ctx context.Context, db *sql.DB) ([]TableSummary, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY type = 'view', name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableSummary
	for rows.Next() {
		var t TableSummary
		if err := rows.Scan(&t.Name, &t.Kind); err != nil {
			return nil, err
		}
		t.Schema = "main"
		t.Estimate = true
		t.Rows = 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// Table returns the structure of one table. It caches the result.
func (c *Conn) Table(ctx context.Context, schema, name string) (*Table, error) {
	key := schema + "." + name
	c.cacheMu.Lock()
	if t, ok := c.tables[key]; ok {
		c.cacheMu.Unlock()
		return t, nil
	}
	c.cacheMu.Unlock()
	db, err := c.Reader(ctx)
	if err != nil {
		return nil, err
	}
	var t *Table
	switch c.Engine() {
	case "postgres":
		t, err = pgTable(ctx, db, schema, name)
	case "mysql":
		t, err = myTable(ctx, db, schema, name)
	case "sqlite":
		t, err = liteTable(ctx, db, name)
	}
	if err != nil {
		return nil, friendly(err)
	}
	for i := range t.Columns {
		t.Columns[i].Numeric = isNumericType(strings.ToUpper(t.Columns[i].Type)) && t.Columns[i].FK == nil
	}
	c.cacheMu.Lock()
	c.tables[key] = t
	c.cacheMu.Unlock()
	return t, nil
}

// FindTable finds a table summary by name, with or without a schema prefix.
func (c *Conn) FindTable(ctx context.Context, name string) (*TableSummary, error) {
	s, err := c.Schema(ctx, false)
	if err != nil {
		return nil, err
	}
	name = strings.ToLower(name)
	schema := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		schema, name = name[:i], name[i+1:]
	}
	for _, g := range s.Groups {
		for i := range g.Tables {
			t := &g.Tables[i]
			if strings.ToLower(t.Name) == name && (schema == "" || strings.ToLower(t.Schema) == schema) {
				return t, nil
			}
		}
	}
	return nil, nil
}

func pgTable(ctx context.Context, db *sql.DB, schema, name string) (*Table, error) {
	t := &Table{Schema: schema, Name: name, Kind: "table"}
	rel := `format('%I.%I', $1::text, $2::text)::regclass`
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT relkind::text FROM pg_class WHERE oid = `+rel, schema, name).Scan(&kind); err != nil {
		return nil, err
	}
	if kind == "v" || kind == "m" {
		t.Kind = "view"
	}
	rows, err := db.QueryContext(ctx, `
SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
       COALESCE(pg_get_expr(d.adbin, d.adrelid), CASE WHEN a.attidentity <> '' THEN 'identity' ELSE '' END)
FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE a.attrelid = `+rel+` AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, schema, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c ColumnInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default); err != nil {
			rows.Close()
			return nil, err
		}
		t.Columns = append(t.Columns, c)
	}
	rows.Close()
	cons, err := db.QueryContext(ctx, `
SELECT c.contype::text, a.attname, COALESCE(nf.nspname, ''), COALESCE(cf.relname, ''), COALESCE(af.attname, ''), cardinality(c.conkey)
FROM pg_constraint c
CROSS JOIN LATERAL unnest(c.conkey, COALESCE(c.confkey, c.conkey)) AS k(attnum, fattnum)
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
LEFT JOIN pg_class cf ON cf.oid = c.confrelid
LEFT JOIN pg_namespace nf ON nf.oid = cf.relnamespace
LEFT JOIN pg_attribute af ON af.attrelid = c.confrelid AND af.attnum = k.fattnum
WHERE c.conrelid = `+rel+` AND c.contype IN ('p','u','f')`, schema, name)
	if err != nil {
		return nil, err
	}
	for cons.Next() {
		var typ, col, fs, ft, fc string
		var n int
		if err := cons.Scan(&typ, &col, &fs, &ft, &fc, &n); err != nil {
			cons.Close()
			return nil, err
		}
		cc := t.Column(col)
		if cc == nil {
			continue
		}
		switch typ {
		case "p":
			cc.PK = true
		case "u":
			cc.Unique = cc.Unique || n == 1
		case "f":
			if n == 1 {
				cc.FK = &Ref{Schema: fs, Table: ft, Column: fc, Local: col}
			}
		}
	}
	cons.Close()
	refs, err := db.QueryContext(ctx, `
SELECT nr.nspname, cr.relname, a.attname, af.attname,
       EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.conrelid AND i.indkey[0] = c.conkey[1])
FROM pg_constraint c
JOIN pg_class cr ON cr.oid = c.conrelid JOIN pg_namespace nr ON nr.oid = cr.relnamespace
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
JOIN pg_attribute af ON af.attrelid = c.confrelid AND af.attnum = c.confkey[1]
WHERE c.confrelid = `+rel+` AND c.contype = 'f' AND cardinality(c.conkey) = 1
ORDER BY 2, 3`, schema, name)
	if err != nil {
		return nil, err
	}
	for refs.Next() {
		var r Ref
		if err := refs.Scan(&r.Schema, &r.Table, &r.Column, &r.Local, &r.Indexed); err != nil {
			refs.Close()
			return nil, err
		}
		t.ReferencedBy = append(t.ReferencedBy, r)
	}
	refs.Close()
	idx, err := db.QueryContext(ctx, `
SELECT ic.relname, i.indisunique, i.indisprimary,
       COALESCE((SELECT string_agg(a.attname, ', ' ORDER BY k.ord) FROM unnest(i.indkey) WITH ORDINALITY k(attnum, ord)
                 JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum), '')
FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
WHERE i.indrelid = `+rel+` ORDER BY i.indisprimary DESC, ic.relname`, schema, name)
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	for idx.Next() {
		var x Index
		if err := idx.Scan(&x.Name, &x.Unique, &x.Primary, &x.Columns); err != nil {
			return nil, err
		}
		t.Indexes = append(t.Indexes, x)
		if x.Unique && !x.Primary && !strings.Contains(x.Columns, ",") {
			if cc := t.Column(x.Columns); cc != nil {
				cc.Unique = true
			}
		}
	}
	return t, idx.Err()
}

func myTable(ctx context.Context, db *sql.DB, schema, name string) (*Table, error) {
	t := &Table{Schema: schema, Name: name, Kind: "table"}
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, schema, name).Scan(&kind); err != nil {
		return nil, err
	}
	if strings.Contains(kind, "VIEW") {
		t.Kind = "view"
	}
	rows, err := db.QueryContext(ctx, `
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE = 'YES', COALESCE(COLUMN_DEFAULT, ''), COLUMN_KEY, EXTRA
FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`, schema, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c ColumnInfo
		var key, extra string
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &key, &extra); err != nil {
			rows.Close()
			return nil, err
		}
		c.PK = key == "PRI"
		c.Unique = key == "UNI"
		if strings.Contains(extra, "auto_increment") {
			c.Default = "auto_increment"
		}
		t.Columns = append(t.Columns, c)
	}
	rows.Close()
	fks, err := db.QueryContext(ctx, `
SELECT k.COLUMN_NAME, k.REFERENCED_TABLE_SCHEMA, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME,
       (SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE k2 WHERE k2.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND k2.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND k2.TABLE_NAME = k.TABLE_NAME)
FROM information_schema.KEY_COLUMN_USAGE k
WHERE k.TABLE_SCHEMA = ? AND k.TABLE_NAME = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL`, schema, name)
	if err != nil {
		return nil, err
	}
	for fks.Next() {
		var col string
		var r Ref
		var n int
		if err := fks.Scan(&col, &r.Schema, &r.Table, &r.Column, &n); err != nil {
			fks.Close()
			return nil, err
		}
		if cc := t.Column(col); cc != nil && n == 1 {
			r.Local = col
			cc.FK = &r
		}
	}
	fks.Close()
	refs, err := db.QueryContext(ctx, `
SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, REFERENCED_COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE REFERENCED_TABLE_SCHEMA = ? AND REFERENCED_TABLE_NAME = ? ORDER BY TABLE_NAME, COLUMN_NAME`, schema, name)
	if err != nil {
		return nil, err
	}
	for refs.Next() {
		// InnoDB keeps an index on every foreign key column.
		r := Ref{Indexed: true}
		if err := refs.Scan(&r.Schema, &r.Table, &r.Column, &r.Local); err != nil {
			refs.Close()
			return nil, err
		}
		t.ReferencedBy = append(t.ReferencedBy, r)
	}
	refs.Close()
	idx, err := db.QueryContext(ctx, `
SELECT INDEX_NAME, NON_UNIQUE = 0, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ')
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
GROUP BY INDEX_NAME, NON_UNIQUE ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME`, schema, name)
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	for idx.Next() {
		var x Index
		if err := idx.Scan(&x.Name, &x.Unique, &x.Columns); err != nil {
			return nil, err
		}
		x.Primary = x.Name == "PRIMARY"
		t.Indexes = append(t.Indexes, x)
	}
	return t, idx.Err()
}

func liteTable(ctx context.Context, db *sql.DB, name string) (*Table, error) {
	t := &Table{Schema: "main", Name: name, Kind: "table"}
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT type FROM sqlite_master WHERE name = ?`, name).Scan(&kind); err != nil {
		return nil, err
	}
	t.Kind = kind
	rows, err := db.QueryContext(ctx, `SELECT name, type, "notnull", COALESCE(dflt_value, ''), pk FROM pragma_table_info(?) ORDER BY cid`, name)
	if err != nil {
		return nil, err
	}
	pks := 0
	for rows.Next() {
		var c ColumnInfo
		var notnull, pk int
		if err := rows.Scan(&c.Name, &c.Type, &notnull, &c.Default, &pk); err != nil {
			rows.Close()
			return nil, err
		}
		c.Nullable = notnull == 0 && pk == 0
		c.PK = pk > 0
		if c.PK {
			pks++
		}
		t.Columns = append(t.Columns, c)
	}
	rows.Close()
	fks, err := db.QueryContext(ctx, `SELECT id, "table", "from", COALESCE("to", '') FROM pragma_foreign_key_list(?)`, name)
	if err != nil {
		return nil, err
	}
	count := map[int]int{}
	type fk struct {
		id          int
		table, from string
		to          string
	}
	var list []fk
	for fks.Next() {
		var f fk
		if err := fks.Scan(&f.id, &f.table, &f.from, &f.to); err != nil {
			fks.Close()
			return nil, err
		}
		count[f.id]++
		list = append(list, f)
	}
	fks.Close()
	for _, f := range list {
		if count[f.id] != 1 {
			continue
		}
		if cc := t.Column(f.from); cc != nil {
			to := f.to
			if to == "" {
				to = "rowid"
			}
			cc.FK = &Ref{Schema: "main", Table: f.table, Column: to, Local: f.from}
		}
	}
	idx, err := db.QueryContext(ctx, `SELECT name, "unique", origin FROM pragma_index_list(?)`, name)
	if err != nil {
		return nil, err
	}
	type ix struct {
		name   string
		unique int
		origin string
	}
	var ixs []ix
	for idx.Next() {
		var x ix
		if err := idx.Scan(&x.name, &x.unique, &x.origin); err != nil {
			idx.Close()
			return nil, err
		}
		ixs = append(ixs, x)
	}
	idx.Close()
	for _, x := range ixs {
		var cols []string
		r, err := db.QueryContext(ctx, `SELECT COALESCE(name, '') FROM pragma_index_info(?) ORDER BY seqno`, x.name)
		if err != nil {
			return nil, err
		}
		for r.Next() {
			var c string
			_ = r.Scan(&c)
			cols = append(cols, c)
		}
		r.Close()
		t.Indexes = append(t.Indexes, Index{Name: x.name, Columns: strings.Join(cols, ", "), Unique: x.unique == 1, Primary: x.origin == "pk"})
		if x.unique == 1 && len(cols) == 1 && x.origin != "pk" {
			if cc := t.Column(cols[0]); cc != nil {
				cc.Unique = true
			}
		}
	}
	if pks == 1 {
		for _, c := range t.Columns {
			if c.PK {
				t.Indexes = append([]Index{{Name: "PRIMARY KEY", Columns: c.Name, Unique: true, Primary: true}}, t.Indexes...)
			}
		}
	}
	// Referenced by: scan the other tables. SQLite has no reverse catalog.
	all, err := db.QueryContext(ctx, `SELECT m.name, f."from", COALESCE(f."to", ''),
       EXISTS (SELECT 1 FROM pragma_index_list(m.name) il, pragma_index_info(il.name) ii WHERE ii.seqno = 0 AND ii.name = f."from")
FROM sqlite_master m, pragma_foreign_key_list(m.name) f WHERE m.type = 'table' AND f."table" = ? COLLATE NOCASE ORDER BY m.name`, name)
	if err != nil {
		return nil, err
	}
	defer all.Close()
	for all.Next() {
		var r Ref
		if err := all.Scan(&r.Table, &r.Column, &r.Local, &r.Indexed); err != nil {
			return nil, err
		}
		r.Schema = "main"
		t.ReferencedBy = append(t.ReferencedBy, r)
	}
	return t, all.Err()
}
