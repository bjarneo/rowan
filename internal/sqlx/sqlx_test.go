package sqlx

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	src := "-- head\nSELECT 'a;b' FROM t; UPDATE x SET y = $$q;w$$;\n/* only a comment */;\nSELECT 1"
	got := Split(src, false)
	want := []string{"-- head\nSELECT 'a;b' FROM t", "UPDATE x SET y = $$q;w$$", "SELECT 1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Split = %q, want %q", got, want)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		sql         string
		write, dest bool
	}{
		{"SELECT * FROM orders", false, false},
		{"select count(*) from customers where plan <> 'free'", false, false},
		{"WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d", true, false},
		{"WITH a AS (SELECT 1) SELECT * FROM a", false, false},
		{"EXPLAIN SELECT 1", false, false},
		{"EXPLAIN ANALYZE UPDATE t SET a = 1", true, false},
		{"UPDATE orders SET status = 'x' WHERE id = 1", true, false},
		{"UPDATE orders SET status = 'x'", true, true},
		{"DELETE FROM orders", true, true},
		{"DELETE FROM orders WHERE id IN (SELECT id FROM x WHERE y)", true, false},
		{"DROP TABLE x", true, true},
		{"SET search_path = public", false, false},
		{"SET default_transaction_read_only = off", true, false},
		{"SET SESSION CHARACTERISTICS AS TRANSACTION READ WRITE", true, false},
		{"BEGIN READ WRITE", true, false},
		{"BEGIN", false, false},
		{"PRAGMA table_info(notes)", false, false},
		{"PRAGMA query_only = 0", true, false},
		{"VACUUM", true, false},
		{"SELECT 'delete from x'", false, false},
		{"select pg_sleep(1)", false, false},
	}
	for _, c := range cases {
		got := Classify(c.sql, false)
		if got.Write != c.write || got.Destructive != c.dest {
			t.Errorf("Classify(%q) = write %v destructive %v, want %v %v", c.sql, got.Write, got.Destructive, c.write, c.dest)
		}
	}
}

func TestIdentifiers(t *testing.T) {
	got := Identifiers(`SELECT number, paid_at FROM public."Invoices" i JOIN orders o ON o.id = i.order_id`, false)
	want := []string{"number", "paid_at", "public", "invoices", "i", "orders", "o", "id", "order_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Identifiers = %q, want %q", got, want)
	}
}

func TestParseTarget(t *testing.T) {
	tg, ok := ParseTarget("UPDATE orders SET status = 'refunded', total_cents = 0 WHERE id = 10442;", false)
	if !ok || tg.Table != "orders" || tg.Where != "id = 10442" || len(tg.Sets) != 2 || tg.Sets[1].Expr != "0" {
		t.Fatalf("update: %+v %v", tg, ok)
	}
	tg, ok = ParseTarget("UPDATE public.orders o SET status = 'x' WHERE o.id = 1 RETURNING *", false)
	if !ok || tg.Table != "public.orders" || tg.Alias != "o" || tg.Where != "o.id = 1" {
		t.Fatalf("update alias: %+v %v", tg, ok)
	}
	tg, ok = ParseTarget("DELETE FROM order_items WHERE order_id = 10431", false)
	if !ok || tg.Verb != "DELETE" || tg.Where != "order_id = 10431" {
		t.Fatalf("delete: %+v %v", tg, ok)
	}
	if _, ok = ParseTarget("UPDATE a SET x = b.x FROM b WHERE a.id = b.id", false); ok {
		t.Fatal("update from should not parse")
	}
	if _, ok = ParseTarget("INSERT INTO a VALUES (1)", false); ok {
		t.Fatal("insert should not parse")
	}
}

func TestSourceTable(t *testing.T) {
	for sql, want := range map[string]string{
		"SELECT * FROM orders WHERE status = 'refunded' ORDER BY placed_at DESC": "orders",
		"select id from public.orders o limit 5":                                 "public.orders",
		"SELECT * FROM a JOIN b ON a.id = b.a":                                   "",
		"SELECT country, count(*) FROM customers GROUP BY country":               "",
		"SELECT * FROM a, b": "",
	} {
		got, _ := SourceTable(sql, false)
		if got != want {
			t.Errorf("SourceTable(%q) = %q, want %q", sql, got, want)
		}
	}
}

func TestBounded(t *testing.T) {
	cases := map[string]bool{
		"SELECT * FROM orders":                                  true,
		"select 1":                                              true,
		"WITH x AS (SELECT 1) SELECT * FROM x":                  true,
		"VALUES (1), (2)":                                       true,
		"TABLE orders":                                          true,
		"(SELECT 1) UNION (SELECT 2)":                           true,
		"SELECT * FROM orders -- the end":                       true,
		"SELECT 'into' AS word":                                 true,
		"SELECT * INTO copy FROM orders":                        false,
		"SELECT * FROM orders FOR UPDATE":                       false,
		"SELECT * FROM orders FOR NO KEY UPDATE":                false,
		"SELECT * FROM orders FOR SHARE":                        false,
		"WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d": false,
		"EXPLAIN SELECT 1":                                      false,
		"SHOW search_path":                                      false,
		"INSERT INTO t VALUES (1) RETURNING id":                 false,
		"SET default_transaction_read_only = off":               false,
	}
	for sql, want := range cases {
		if got := Bounded(sql, false); got != want {
			t.Errorf("Bounded(%q) = %v, want %v", sql, got, want)
		}
	}
}

func TestHintAt(t *testing.T) {
	cases := map[string]string{
		"SELECT * FROM t":                       "SELECT",
		"  select a FROM t":                     "  select",
		"WITH x AS (SELECT 1) SELECT * FROM x":  "WITH x AS (SELECT 1) SELECT",
		"-- note\nSELECT 1":                     "-- note\nSELECT",
		"(SELECT 1) UNION (SELECT 2)":           "",
		"SELECT /*+ MAX_EXECUTION_TIME(5) */ 1": "",
		"SELECT /* plain */ 1":                  "SELECT",
		"VALUES ROW(1)":                         "",
	}
	for sql, want := range cases {
		at := HintAt(sql, true)
		got := ""
		if at >= 0 {
			got = sql[:at]
		}
		if got != want {
			t.Errorf("HintAt(%q) cuts at %q, want %q", sql, got, want)
		}
	}
}
