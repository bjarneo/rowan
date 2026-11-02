package db

import (
	"context"
	"strings"
	"time"
)

type sampleSet struct {
	at     time.Time
	values map[string]map[string]string
}

// isTextType reports whether a column type holds text.
func isTextType(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "char") || strings.Contains(t, "text") || strings.Contains(t, "enum") ||
		t == "string" || strings.HasPrefix(t, "set(") || t == "citext" || t == "name"
}

// EnumValues reads the first 500 rows of a table and returns the text
// columns that look like enums: columns with 24 distinct values or fewer.
// The map goes from column to lower-case value to value. Results stay in a
// cache for five minutes.
func (c *Conn) EnumValues(ctx context.Context, t *Table) (map[string]map[string]string, error) {
	key := t.Schema + "." + t.Name
	c.cacheMu.Lock()
	if s, ok := c.samples[key]; ok && time.Since(s.at) < 5*time.Minute {
		c.cacheMu.Unlock()
		return s.values, nil
	}
	c.cacheMu.Unlock()
	engine := c.Engine()
	var cols []string
	var names []string
	for _, col := range t.Columns {
		if isTextType(col.Type) && !col.PK {
			cols = append(cols, QuoteIdent(engine, col.Name))
			names = append(names, col.Name)
		}
	}
	out := map[string]map[string]string{}
	if len(cols) > 0 {
		q := "SELECT " + strings.Join(cols, ", ") + " FROM " + TableRef(engine, t.Schema, t.Name) + " LIMIT 500"
		_, rows, _, err := c.Select(ctx, q, nil, 500)
		if err != nil {
			return nil, err
		}
		for i, name := range names {
			set := map[string]string{}
			for _, r := range rows {
				s, ok := r[i].(string)
				if !ok || s == "" || len(s) > 64 {
					continue
				}
				set[strings.ToLower(s)] = s
				if len(set) > 24 {
					break
				}
			}
			if len(set) > 0 && len(set) <= 24 {
				out[name] = set
			}
		}
	}
	if engine == "postgres" {
		// An enum column has its own type name, so isTextType skips it. The
		// catalog has its exact values.
		if err := pgEnumValues(ctx, c, t, out); err != nil {
			return nil, err
		}
	}
	c.cacheMu.Lock()
	c.samples[key] = &sampleSet{at: time.Now(), values: out}
	c.cacheMu.Unlock()
	return out, nil
}

// pgEnumValues adds the labels of the enum columns of a PostgreSQL table.
func pgEnumValues(ctx context.Context, c *Conn, t *Table, out map[string]map[string]string) error {
	db, err := c.Reader(ctx)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `
SELECT a.attname, e.enumlabel
FROM pg_attribute a
JOIN pg_enum e ON e.enumtypid = a.atttypid
WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum, e.enumsortorder`, QuoteIdent("postgres", t.Schema)+"."+QuoteIdent("postgres", t.Name))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var col, label string
		if err := rows.Scan(&col, &label); err != nil {
			return err
		}
		if out[col] == nil {
			out[col] = map[string]string{}
		}
		out[col][strings.ToLower(label)] = label
	}
	return rows.Err()
}
