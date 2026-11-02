package sqlx

import (
	"strings"
)

// Info describes one statement.
type Info struct {
	Verb        string // first keyword in upper case, such as SELECT
	Write       bool   // the statement can change data, schema or access mode
	Rows        bool   // the statement returns a result set
	Destructive bool   // DROP, TRUNCATE, or DELETE and UPDATE without WHERE
}

var readVerbs = map[string]bool{
	"select": true, "with": true, "show": true, "explain": true, "describe": true,
	"desc": true, "values": true, "table": true, "pragma": true,
}

// sessionVerbs change session state but not data. Rowan allows them on
// read-only connections unless they try to lift the read-only mode.
var sessionVerbs = map[string]bool{
	"set": true, "reset": true, "use": true, "begin": true, "start": true,
	"commit": true, "rollback": true, "end": true, "savepoint": true, "release": true,
}

var writeWords = map[string]bool{
	"insert": true, "update": true, "delete": true, "merge": true, "upsert": true,
	"create": true, "alter": true, "drop": true, "truncate": true, "grant": true,
	"revoke": true, "replace": true, "copy": true, "call": true, "do": true,
	"vacuum": true, "reindex": true, "cluster": true, "lock": true, "attach": true,
	"detach": true, "load": true, "import": true, "refresh": true, "comment": true,
	"rename": true, "optimize": true, "analyze": true, "discard": true,
}

// escalation words lift a read-only session. Rowan treats any statement that
// names one of them as a write.
var escalation = map[string]bool{
	"default_transaction_read_only": true, "transaction_read_only": true,
	"tx_read_only": true, "query_only": true, "session_authorization": true,
	"characteristics": true, "super_read_only": true, "read_only": true,
}

// Classify reports what a statement does. Unknown verbs count as writes.
func Classify(stmt string, hashComments bool) Info {
	toks := Significant(Lex(stmt, hashComments))
	if len(toks) == 0 {
		return Info{}
	}
	// Skip leading parentheses, as in "(SELECT 1) UNION (SELECT 2)".
	i := 0
	for i < len(toks) && toks[i].Kind == Punct && toks[i].Text == "(" {
		i++
	}
	if i >= len(toks) {
		return Info{Verb: "(", Write: true}
	}
	verb := strings.ToLower(toks[i].Text)
	info := Info{Verb: strings.ToUpper(verb)}

	hasWriteWord := false
	for j, t := range toks {
		if t.Kind != Word && t.Kind != Quoted {
			continue
		}
		w := t.Lower()
		if t.Kind == Word && writeWords[w] && j > i {
			hasWriteWord = true
		}
		if escalation[w] {
			info.Write = true
		}
		if w == "read" && j+1 < len(toks) && toks[j+1].Is("write") {
			info.Write = true
		}
	}

	switch {
	case readVerbs[verb]:
		info.Rows = true
		switch verb {
		case "with", "explain":
			// A data-modifying CTE or EXPLAIN ANALYZE of a write runs the write.
			if hasWriteWord {
				info.Write = true
				info.Rows = hasWord(toks, "returning") || verb == "explain"
			}
		case "pragma":
			if hasPunct(toks, "=") {
				info.Write = true
			}
		}
	case sessionVerbs[verb]:
		// Session statements are allowed unless an escalation word set Write.
	default:
		info.Write = true
		info.Rows = hasWord(toks, "returning")
	}

	switch verb {
	case "drop", "truncate":
		info.Destructive = true
	case "delete", "update":
		info.Destructive = !hasTopWord(toks, "where")
	case "alter":
		info.Destructive = hasWord(toks, "drop")
	}
	return info
}

func hasWord(toks []Token, w string) bool {
	for _, t := range toks {
		if t.Is(w) {
			return true
		}
	}
	return false
}

func hasPunct(toks []Token, p string) bool {
	for _, t := range toks {
		if t.Kind == Punct && t.Text == p {
			return true
		}
	}
	return false
}

func hasTopWord(toks []Token, w string) bool {
	d := 0
	for _, t := range toks {
		if t.Kind == Punct {
			if t.Text == "(" {
				d++
			} else if t.Text == ")" {
				d--
			}
			continue
		}
		if d == 0 && t.Is(w) {
			return true
		}
	}
	return false
}

// Identifiers returns every identifier in the statement in lower case. It
// splits qualified names, so "public.orders" gives "public" and "orders".
// Keywords that are not quoted are skipped.
func Identifiers(stmt string, hashComments bool) []string {
	toks := Significant(Lex(stmt, hashComments))
	seen := map[string]bool{}
	var out []string
	for _, t := range toks {
		var name string
		switch t.Kind {
		case Quoted:
			name = t.Lower()
		case Word:
			if Keywords[strings.ToLower(t.Text)] {
				continue
			}
			name = strings.ToLower(t.Text)
		default:
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Target is the table and filter of a simple single-table write.
type Target struct {
	Verb  string // UPDATE or DELETE
	Table string // table text as written, such as public.orders
	Alias string // alias text, such as "o", or ""
	Sets  []Assign
	Where string // WHERE clause text without the keyword, or "" for none
}

// Assign is one "col = expr" pair in an UPDATE.
type Assign struct {
	Column string
	Expr   string
}

// ParseTarget reads simple UPDATE and DELETE statements. It returns false
// for joins, USING, FROM in UPDATE, CTEs and other shapes it can't read.
func ParseTarget(stmt string, hashComments bool) (Target, bool) {
	all := Lex(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";")), hashComments)
	toks := Significant(all)
	if len(toks) < 2 {
		return Target{}, false
	}
	src := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";"))
	top := topLevel(toks)
	find := func(w string, from int) int {
		for _, i := range top {
			if i >= from && toks[i].Is(w) {
				return i
			}
		}
		return -1
	}
	end := len(src)
	for _, w := range []string{"returning", "order", "limit"} {
		if j := find(w, 1); j >= 0 && toks[j].Pos < end {
			end = toks[j].Pos
		}
	}
	tableAt := func(i int) (string, int, bool) {
		if i >= len(toks) || (toks[i].Kind != Word && toks[i].Kind != Quoted) {
			return "", i, false
		}
		name := toks[i].Text
		i++
		for i+1 < len(toks) && toks[i].Kind == Punct && toks[i].Text == "." {
			name += "." + toks[i+1].Text
			i += 2
		}
		return name, i, true
	}
	switch {
	case toks[0].Is("update"):
		if toks[1].Is("only") {
			return Target{}, false
		}
		name, i, ok := tableAt(1)
		if !ok {
			return Target{}, false
		}
		alias, i := aliasAt(toks, i, "set")
		set := find("set", i)
		if set != i || find("from", set) >= 0 {
			return Target{}, false
		}
		where := find("where", set)
		setEnd := end
		if where >= 0 {
			setEnd = toks[where].Pos
		}
		t := Target{Verb: "UPDATE", Table: name, Alias: alias}
		for _, part := range splitTopLevel(src[toks[set].Pos+3 : setEnd]) {
			eq := strings.Index(part, "=")
			if eq < 0 {
				return Target{}, false
			}
			col := strings.TrimSpace(part[:eq])
			if strings.ContainsAny(col, "(,") {
				return Target{}, false
			}
			t.Sets = append(t.Sets, Assign{Column: Unquote(col), Expr: strings.TrimSpace(part[eq+1:])})
		}
		if where >= 0 {
			t.Where = strings.TrimSpace(src[toks[where].Pos+5 : end])
		}
		return t, len(t.Sets) > 0
	case toks[0].Is("delete"):
		if !toks[1].Is("from") {
			return Target{}, false
		}
		name, i, ok := tableAt(2)
		if !ok {
			return Target{}, false
		}
		alias, i := aliasAt(toks, i, "where")
		if find("using", i) >= 0 {
			return Target{}, false
		}
		t := Target{Verb: "DELETE", Table: name, Alias: alias}
		if where := find("where", i); where >= 0 {
			t.Where = strings.TrimSpace(src[toks[where].Pos+5 : end])
		} else if i < len(toks) && !toks[i].Is("returning") && !toks[i].Is("order") && !toks[i].Is("limit") {
			return Target{}, false
		}
		return t, true
	}
	return Target{}, false
}

// aliasAt reads an optional "AS alias" or "alias" at toks[i]. It stops at
// the keyword next.
func aliasAt(toks []Token, i int, next string) (string, int) {
	if i < len(toks) && toks[i].Is("as") && i+1 < len(toks) {
		return toks[i+1].Text, i + 2
	}
	if i < len(toks) && (toks[i].Kind == Quoted || (toks[i].Kind == Word && !Keywords[strings.ToLower(toks[i].Text)])) && !toks[i].Is(next) {
		return toks[i].Text, i + 1
	}
	return "", i
}

// SourceTable returns the table that a simple SELECT reads from, such as
// "SELECT * FROM orders WHERE ...". It returns false for joins, grouping,
// set operations and subqueries in FROM.
func SourceTable(stmt string, hashComments bool) (string, bool) {
	toks := Significant(Lex(stmt, hashComments))
	if len(toks) < 4 || !toks[0].Is("select") {
		return "", false
	}
	top := topLevel(toks)
	from := -1
	for _, i := range top {
		t := toks[i]
		switch {
		case t.Is("from") && from < 0:
			from = i
		case t.Is("join"), t.Is("group"), t.Is("union"), t.Is("intersect"), t.Is("except"), t.Is("distinct"), t.Is("having"):
			return "", false
		case t.Kind == Punct && t.Text == "," && from >= 0:
			return "", false
		}
	}
	if from < 0 || from+1 >= len(toks) {
		return "", false
	}
	i := from + 1
	if toks[i].Kind != Word && toks[i].Kind != Quoted {
		return "", false
	}
	name := toks[i].Text
	i++
	for i+1 < len(toks) && toks[i].Kind == Punct && toks[i].Text == "." {
		name += "." + toks[i+1].Text
		i += 2
	}
	// Aggregates in the select list mean the rows are not table rows.
	for j := 1; j < from; j++ {
		if toks[j].Kind == Word && j+1 < from && toks[j+1].Text == "(" {
			switch strings.ToLower(toks[j].Text) {
			case "count", "sum", "avg", "min", "max", "array_agg", "string_agg", "group_concat", "json_agg":
				return "", false
			}
		}
	}
	return name, true
}

// topLevel returns the indexes of tokens at parenthesis depth zero.
func topLevel(toks []Token) []int {
	out := make([]int, 0, len(toks))
	d := 0
	for i, t := range toks {
		if t.Kind == Punct && t.Text == "(" {
			d++
			continue
		}
		if t.Kind == Punct && t.Text == ")" {
			d--
			continue
		}
		if d == 0 {
			out = append(out, i)
		}
	}
	return out
}

// splitTopLevel splits s at commas that are outside quotes and parentheses.
func splitTopLevel(s string) []string {
	toks := Lex(s, false)
	var out []string
	d, start := 0, 0
	for _, t := range toks {
		if t.Kind != Punct {
			continue
		}
		switch t.Text {
		case "(":
			d++
		case ")":
			d--
		case ",":
			if d == 0 {
				out = append(out, s[start:t.Pos])
				start = t.Pos + 1
			}
		}
	}
	if strings.TrimSpace(s[start:]) != "" {
		out = append(out, s[start:])
	}
	return out
}

// Keywords is the set of SQL words that Identifiers skips.
var Keywords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`select from where and or not null is in as on join left right inner outer full cross
	natural using group by order limit offset having insert into values update set delete create table alter drop
	truncate primary key references default unique index distinct asc desc case when then else end explain analyze
	with recursive returning union all any some like ilike between exists true false begin commit rollback
	fetch first next rows row only over partition window filter lateral cast collate interval array
	count sum avg min max coalesce nullif greatest least extract date time timestamp now current_date
	current_timestamp current_time lower upper length substring trim position
	show describe pragma view materialized if replace ignore duplicate conflict nothing do
	intersect except nulls last escape similar to for share nowait skip locked of`) {
		m[w] = true
	}
	return m
}()

// Insert is a simple INSERT ... VALUES statement.
type Insert struct {
	Table   string
	Columns []string
	Rows    int
	First   []string // expressions of the first tuple
}

// ParseInsert reads "INSERT INTO t (a, b) VALUES (...), (...)". It returns
// false for INSERT ... SELECT and other shapes.
func ParseInsert(stmt string, hashComments bool) (Insert, bool) {
	src := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";"))
	toks := Significant(Lex(src, hashComments))
	if len(toks) < 5 || !toks[0].Is("insert") || !toks[1].Is("into") {
		return Insert{}, false
	}
	i := 2
	if toks[i].Kind != Word && toks[i].Kind != Quoted {
		return Insert{}, false
	}
	ins := Insert{Table: toks[i].Text}
	i++
	for i+1 < len(toks) && toks[i].Text == "." {
		ins.Table += "." + toks[i+1].Text
		i += 2
	}
	if i < len(toks) && toks[i].Text == "(" {
		j := i + 1
		for j < len(toks) && toks[j].Text != ")" {
			if toks[j].Kind == Word || toks[j].Kind == Quoted {
				ins.Columns = append(ins.Columns, Unquote(toks[j].Text))
			}
			j++
		}
		i = j + 1
	}
	if i >= len(toks) || !toks[i].Is("values") {
		return Insert{}, false
	}
	d, start := 0, -1
	for _, t := range toks[i+1:] {
		if t.Kind != Punct {
			continue
		}
		switch t.Text {
		case "(":
			if d == 0 {
				start = t.Pos + 1
			}
			d++
		case ")":
			d--
			if d == 0 {
				ins.Rows++
				if ins.Rows == 1 && start >= 0 {
					for _, p := range splitTopLevel(src[start:t.Pos]) {
						ins.First = append(ins.First, strings.TrimSpace(p))
					}
				}
			}
		}
	}
	return ins, ins.Rows > 0
}
