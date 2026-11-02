// Package search reads the free text of the command surface. The first word
// picks a table. The other words become filters on that table:
//
//	orders refunded          status = 'refunded' (a value seen in the status column)
//	orders 10442             id = 10442 (a number filters the primary key)
//	orders status:shipped    status = 'shipped'
//	orders total_cents>5000  total_cents > 5000
//	customers email~acme     email contains "acme"
//	orders shipped_at:null   shipped_at IS NULL
package search

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"rowan/internal/db"
)

// Filter is one condition on a column.
type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"` // =, <>, >, <, >=, <=, contains, null, notnull
	Value  string `json:"value"`
	Source string `json:"source"` // the words that produced the filter
	Guess  bool   `json:"guess"`  // true when Rowan matched a bare value to a column
}

// Interp is the reading of the words after the table name.
type Interp struct {
	Filters    []Filter `json:"filters"`
	Unresolved []string `json:"unresolved"`
	Note       string   `json:"note"`
	Canonical  string   `json:"canonical"` // the same filters in column:value form
}

// Terms splits text into words. Double quotes keep spaces in one word.
func Terms(s string) []string {
	var out []string
	var b strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case (r == ' ' || r == '\t') && !inQ:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

var ops = []string{">=", "<=", "!=", "<>", "=", ">", "<", ":", "~"}

// Interpret reads terms against the table. enums maps a column to its
// sampled values (lower case to original) and can be nil.
func Interpret(terms []string, t *db.Table, enums map[string]map[string]string) Interp {
	var in Interp
	col := func(name string) *db.ColumnInfo {
		name = strings.ToLower(name)
		var prefix *db.ColumnInfo
		n := 0
		for i := range t.Columns {
			c := &t.Columns[i]
			lc := strings.ToLower(c.Name)
			if lc == name {
				return c
			}
			if strings.HasPrefix(lc, name) {
				prefix = c
				n++
			}
		}
		if n == 1 {
			return prefix
		}
		return nil
	}
	for i := 0; i < len(terms); i++ {
		term := terms[i]
		// column<op>value
		if c, op, val, ok := splitOp(term, col); ok {
			in.Filters = append(in.Filters, makeFilter(c, op, val, term, false))
			continue
		}
		// column value
		if c := col(term); c != nil && strings.EqualFold(c.Name, term) && i+1 < len(terms) {
			if _, _, _, ok := splitOp(terms[i+1], col); !ok && t.Column(terms[i+1]) == nil {
				in.Filters = append(in.Filters, makeFilter(c, "=", terms[i+1], term+" "+terms[i+1], false))
				i++
				continue
			}
		}
		// A bare number filters the primary key.
		if _, err := strconv.ParseFloat(term, 64); err == nil {
			if pk := t.PK(); pk != nil && pk.Numeric {
				in.Filters = append(in.Filters, Filter{Column: pk.Name, Op: "=", Value: term, Source: term, Guess: true})
				continue
			}
		}
		// A bare word that matches a sampled value.
		if c := guessColumn(term, enums); c != "" {
			in.Filters = append(in.Filters, Filter{Column: c, Op: "=", Value: enums[c][strings.ToLower(term)], Source: term, Guess: true})
			continue
		}
		in.Unresolved = append(in.Unresolved, term)
	}
	var canon []string
	for _, f := range in.Filters {
		canon = append(canon, f.Canonical())
	}
	in.Canonical = strings.Join(canon, " ")
	in.Note = note(in)
	return in
}

func splitOp(term string, col func(string) *db.ColumnInfo) (*db.ColumnInfo, string, string, bool) {
	best := -1
	bestOp := ""
	for _, op := range ops {
		if i := strings.Index(term, op); i > 0 && (best < 0 || i < best || (i == best && len(op) > len(bestOp))) {
			best, bestOp = i, op
		}
	}
	if best < 0 {
		return nil, "", "", false
	}
	c := col(term[:best])
	if c == nil {
		return nil, "", "", false
	}
	return c, bestOp, term[best+len(bestOp):], true
}

func makeFilter(c *db.ColumnInfo, op, val, src string, guess bool) Filter {
	f := Filter{Column: c.Name, Value: val, Source: src, Guess: guess}
	lv := strings.ToLower(val)
	switch op {
	case ":", "=":
		f.Op = "="
		if lv == "null" {
			f.Op = "null"
		} else if lv == "!null" || lv == "notnull" {
			f.Op = "notnull"
		}
	case "!=", "<>":
		f.Op = "<>"
		if lv == "null" {
			f.Op = "notnull"
		}
	case "~":
		f.Op = "contains"
	default:
		f.Op = op
	}
	return f
}

func guessColumn(term string, enums map[string]map[string]string) string {
	lt := strings.ToLower(term)
	var hits []string
	for c, vals := range enums {
		if _, ok := vals[lt]; ok {
			hits = append(hits, c)
		}
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Slice(hits, func(a, b int) bool {
		pa, pb := rank(hits[a]), rank(hits[b])
		if pa != pb {
			return pa < pb
		}
		if len(enums[hits[a]]) != len(enums[hits[b]]) {
			return len(enums[hits[a]]) < len(enums[hits[b]])
		}
		return hits[a] < hits[b]
	})
	return hits[0]
}

func rank(col string) int {
	switch strings.ToLower(col) {
	case "status", "state":
		return 0
	case "type", "kind", "plan", "role", "category":
		return 1
	}
	return 2
}

// Canonical returns the filter in column:value form.
func (f Filter) Canonical() string {
	v := f.Value
	if strings.ContainsAny(v, " \t") {
		v = `"` + v + `"`
	}
	switch f.Op {
	case "=":
		return f.Column + ":" + v
	case "contains":
		return f.Column + "~" + v
	case "null":
		return f.Column + ":null"
	case "notnull":
		return f.Column + "!=null"
	}
	return f.Column + f.Op + v
}

func note(in Interp) string {
	var guesses []string
	for _, f := range in.Filters {
		if f.Guess {
			guesses = append(guesses, fmt.Sprintf("“%s” as a filter on %s", f.Source, f.Column))
		}
	}
	switch {
	case len(in.Unresolved) > 0:
		return fmt.Sprintf("No column has the value “%s” in the first 500 rows. Type column:value to filter.", in.Unresolved[0])
	case len(guesses) > 0:
		return "Read " + strings.Join(guesses, " and ") + ". Press tab to edit it."
	}
	return ""
}

// Where builds a WHERE clause with bind arguments for the engine.
func Where(engine string, t *db.Table, filters []Filter) (string, []any) {
	var parts []string
	var args []any
	for _, f := range filters {
		c := t.Column(f.Column)
		if c == nil {
			continue
		}
		q := db.QuoteIdent(engine, c.Name)
		switch f.Op {
		case "null":
			parts = append(parts, q+" IS NULL")
			continue
		case "notnull":
			parts = append(parts, q+" IS NOT NULL")
			continue
		}
		args = append(args, typed(c, f.Value))
		ph := db.Placeholder(engine, len(args))
		switch f.Op {
		case "contains":
			args[len(args)-1] = "%" + likeEscape(f.Value) + "%"
			switch engine {
			case "postgres":
				parts = append(parts, q+"::text ILIKE "+ph)
			default:
				parts = append(parts, q+" LIKE "+ph)
			}
		default:
			parts = append(parts, q+" "+f.Op+" "+ph)
		}
	}
	return strings.Join(parts, " AND "), args
}

// Literal builds the same WHERE clause as SQL text for display.
func Literal(engine string, t *db.Table, filters []Filter) string {
	var parts []string
	for _, f := range filters {
		c := t.Column(f.Column)
		if c == nil {
			continue
		}
		q := db.QuoteIdent(engine, c.Name)
		switch f.Op {
		case "null":
			parts = append(parts, q+" IS NULL")
		case "notnull":
			parts = append(parts, q+" IS NOT NULL")
		case "contains":
			op := " LIKE "
			if engine == "postgres" {
				op = "::text ILIKE "
			}
			parts = append(parts, q+op+quote("%"+likeEscape(f.Value)+"%"))
		default:
			v := typed(c, f.Value)
			lit := quote(f.Value)
			switch x := v.(type) {
			case int64:
				lit = strconv.FormatInt(x, 10)
			case float64:
				lit = strconv.FormatFloat(x, 'f', -1, 64)
			case bool:
				lit = strconv.FormatBool(x)
			}
			parts = append(parts, q+" "+f.Op+" "+lit)
		}
	}
	return strings.Join(parts, " AND ")
}

func typed(c *db.ColumnInfo, v string) any {
	lt := strings.ToLower(c.Type)
	if strings.Contains(lt, "bool") {
		switch strings.ToLower(v) {
		case "true", "t", "yes", "1":
			return true
		case "false", "f", "no", "0":
			return false
		}
	}
	if c.Numeric || strings.Contains(lt, "int") {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return v
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
