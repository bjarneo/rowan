// Package sqlx holds a small SQL lexer and the statement checks that Rowan
// uses to keep connections read-only. It does not parse full SQL. It reads
// enough structure to split scripts, classify statements and find the tables
// that a statement names.
package sqlx

import (
	"strings"
)

// Kind is the type of a lexical token.
type Kind uint8

const (
	Word   Kind = iota // keyword or unquoted identifier
	Quoted             // "ident", `ident` or [ident]
	String             // 'text', E'text', $tag$text$tag$
	Number
	Comment
	Space
	Punct
	Param // $1, ?, :name
)

// Token is one lexical unit. Pos is the byte offset in the source.
type Token struct {
	Kind Kind
	Text string
	Pos  int
}

// Lower returns the token text in lower case. For quoted identifiers it
// returns the inner name.
func (t Token) Lower() string {
	if t.Kind == Quoted {
		return strings.ToLower(Unquote(t.Text))
	}
	return strings.ToLower(t.Text)
}

// Is reports whether the token is the word w. The comparison ignores case.
func (t Token) Is(w string) bool {
	return t.Kind == Word && strings.EqualFold(t.Text, w)
}

// Unquote removes identifier quotes.
func Unquote(s string) string {
	if len(s) < 2 {
		return s
	}
	switch s[0] {
	case '"':
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	case '`':
		return strings.ReplaceAll(s[1:len(s)-1], "``", "`")
	case '[':
		return s[1 : len(s)-1]
	}
	return s
}

// Lex splits src into tokens. MySQL-style '#' comments are accepted when
// hashComments is true.
func Lex(src string, hashComments bool) []Token {
	toks := make([]Token, 0, len(src)/3+1)
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		start := i
		switch {
		case c == '-' && i+1 < n && src[i+1] == '-', hashComments && c == '#':
			for i < n && src[i] != '\n' {
				i++
			}
			toks = append(toks, Token{Comment, src[start:i], start})
		case c == '/' && i+1 < n && src[i+1] == '*':
			i += 2
			depth := 1
			for i < n && depth > 0 {
				if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					depth--
					i += 2
				} else if src[i] == '/' && i+1 < n && src[i+1] == '*' {
					depth++
					i += 2
				} else {
					i++
				}
			}
			toks = append(toks, Token{Comment, src[start:i], start})
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			for i < n && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r' || src[i] == '\f') {
				i++
			}
			toks = append(toks, Token{Space, src[start:i], start})
		case c == '\'':
			i = scanQuoted(src, i, '\'')
			toks = append(toks, Token{String, src[start:i], start})
		case (c == 'E' || c == 'e' || c == 'N' || c == 'n' || c == 'X' || c == 'x' || c == 'B' || c == 'b') && i+1 < n && src[i+1] == '\'':
			i = scanQuoted(src, i+1, '\'')
			toks = append(toks, Token{String, src[start:i], start})
		case c == '"':
			i = scanQuoted(src, i, '"')
			toks = append(toks, Token{Quoted, src[start:i], start})
		case c == '`':
			i = scanQuoted(src, i, '`')
			toks = append(toks, Token{Quoted, src[start:i], start})
		case c == '$':
			if end, ok := scanDollar(src, i); ok {
				i = end
				toks = append(toks, Token{String, src[start:i], start})
				break
			}
			i++
			for i < n && isDigit(src[i]) {
				i++
			}
			toks = append(toks, Token{Param, src[start:i], start})
		case c == '?':
			i++
			toks = append(toks, Token{Param, src[start:i], start})
		case isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1])):
			for i < n && (isDigit(src[i]) || src[i] == '.' || src[i] == 'e' || src[i] == 'E' || src[i] == '_') {
				if (src[i] == 'e' || src[i] == 'E') && i+1 < n && (src[i+1] == '-' || src[i+1] == '+') {
					i++
				}
				i++
			}
			toks = append(toks, Token{Number, src[start:i], start})
		case isIdentStart(c):
			for i < n && isIdentPart(src[i]) {
				i++
			}
			toks = append(toks, Token{Word, src[start:i], start})
		default:
			i++
			// Keep multi-byte UTF-8 runes together.
			for i < n && src[i]&0xC0 == 0x80 {
				i++
			}
			toks = append(toks, Token{Punct, src[start:i], start})
		}
	}
	return toks
}

func scanQuoted(src string, i int, q byte) int {
	n := len(src)
	i++
	for i < n {
		if src[i] == '\\' && q == '\'' && i+1 < n {
			i += 2
			continue
		}
		if src[i] == q {
			if i+1 < n && src[i+1] == q {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return n
}

// scanDollar reads a PostgreSQL dollar-quoted string such as $$..$$ or $fn$..$fn$.
func scanDollar(src string, i int) (int, bool) {
	n := len(src)
	j := i + 1
	for j < n && src[j] != '$' && (isIdentPart(src[j]) && !(j == i+1 && isDigit(src[j]))) {
		j++
	}
	if j >= n || src[j] != '$' {
		return 0, false
	}
	tag := src[i : j+1]
	k := strings.Index(src[j+1:], tag)
	if k < 0 {
		return n, true
	}
	return j + 1 + k + len(tag), true
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c|0x20 >= 'a' && c|0x20 <= 'z') || c >= 0x80 }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) || c == '$' }

// Significant returns the tokens without spaces and comments.
func Significant(toks []Token) []Token {
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		if t.Kind != Space && t.Kind != Comment {
			out = append(out, t)
		}
	}
	return out
}

// Split separates a script into statements at top-level semicolons. Empty
// statements and statements that hold only comments are dropped.
func Split(src string, hashComments bool) []string {
	toks := Lex(src, hashComments)
	var out []string
	start := 0
	meaningful := false
	depth := 0
	flush := func(end int) {
		if meaningful {
			if s := strings.TrimSpace(src[start:end]); s != "" {
				out = append(out, s)
			}
		}
		meaningful = false
	}
	for _, t := range toks {
		switch {
		case t.Kind == Punct && t.Text == "(":
			depth++
		case t.Kind == Punct && t.Text == ")":
			if depth > 0 {
				depth--
			}
		case t.Kind == Punct && t.Text == ";" && depth == 0:
			flush(t.Pos)
			start = t.Pos + 1
			continue
		}
		if t.Kind != Space && t.Kind != Comment {
			meaningful = true
		}
	}
	flush(len(src))
	return out
}
