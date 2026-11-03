package db

import (
	"encoding/json"
	"unicode/utf8"
)

// Rows holds the cells of a result: display strings, or nil for NULL. It
// encodes to JSON without reflection, because one result can hold more than
// 100,000 cells and the UI waits for the encoding.
type Rows [][]any

// MarshalJSON encodes the rows as an array of arrays.
func (r Rows) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	n := 2
	for _, row := range r {
		n += 3 + 3*len(row)
		for _, v := range row {
			if s, ok := v.(string); ok {
				n += len(s)
			}
		}
	}
	b := make([]byte, 0, n+n/32)
	b = append(b, '[')
	for i, row := range r {
		if i > 0 {
			b = append(b, ',')
		}
		if row == nil {
			b = append(b, "null"...)
			continue
		}
		b = append(b, '[')
		for j, v := range row {
			if j > 0 {
				b = append(b, ',')
			}
			switch x := v.(type) {
			case nil:
				b = append(b, "null"...)
			case string:
				b = appendJSONString(b, x)
			default:
				enc, err := json.Marshal(x)
				if err != nil {
					return nil, err
				}
				b = append(b, enc...)
			}
		}
		b = append(b, ']')
	}
	return append(b, ']'), nil
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string. Like encoding/json, it
// replaces invalid UTF-8 with U+FFFD and escapes U+2028 and U+2029, so the
// result is also safe inside a JavaScript string literal.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '"', '\\':
				b = append(b, '\\', c)
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, `�`...)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}
