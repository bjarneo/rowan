package db

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRowsJSON(t *testing.T) {
	rows := Rows{
		{"plain", nil, "quote \" and \\ slash", "tab\tnew\nline\r", "\x01\x1f", "naïve ✓", "bad \xff byte", "line sep ", ""},
		nil,
		{},
		{int64(7), true},
	}
	got, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal([][]any(rows))
	if string(got) != string(want) {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	var back [][]any
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatal(err)
	}
	if back[0][2] != "quote \" and \\ slash" || back[0][6] != "bad � byte" || !reflect.DeepEqual(back[2], []any{}) {
		t.Fatalf("round trip %#v", back)
	}
	if b, _ := json.Marshal(Rows(nil)); string(b) != "null" {
		t.Fatalf("nil rows: %s", b)
	}
}

func BenchmarkRowsJSON(b *testing.B) {
	rows := make(Rows, 1000)
	for i := range rows {
		row := make([]any, 120)
		for j := range row {
			if j%9 == 0 {
				continue
			}
			row[j] = "12345.678"
		}
		rows[i] = row
	}
	b.Run("fast", func(b *testing.B) {
		for b.Loop() {
			_, _ = json.Marshal(rows)
		}
	})
	b.Run("reflect", func(b *testing.B) {
		for b.Loop() {
			_, _ = json.Marshal([][]any(rows))
		}
	})
}
