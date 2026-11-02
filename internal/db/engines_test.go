package db

import (
	"context"
	"os"
	"strconv"
	"testing"

	"rowan/internal/config"
)

// These tests run when ROWAN_TEST_PG_PORT or ROWAN_TEST_MYSQL_PORT is set.
// They need a server on 127.0.0.1 with user root/postgres and password rowanpw.

func engineConn(t *testing.T, engine string) *Conn {
	env := map[string]string{"postgres": "ROWAN_TEST_PG_PORT", "mysql": "ROWAN_TEST_MYSQL_PORT"}[engine]
	port, _ := strconv.Atoi(os.Getenv(env))
	if port == 0 {
		t.Skip(env + " is not set")
	}
	ref, _, err := config.SetPassword("test-"+engine, "rowanpw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.DeletePassword("test-" + engine) })
	cfg := config.Connection{ID: "test-" + engine, Name: engine, Engine: engine, Host: "127.0.0.1", Port: port, Password: ref, SSLMode: "disable"}
	if engine == "postgres" {
		cfg.User, cfg.Database = "postgres", "acme"
	} else {
		cfg.User, cfg.Database, cfg.SSLMode = "root", "shop", "preferred"
	}
	m := NewManager()
	t.Cleanup(m.CloseAll)
	c := m.Conn(cfg)
	ctx := context.Background()
	w, err := c.Writer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := w.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = 'orders'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		if err := CreateSample(ctx, w, engine); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func testEngine(t *testing.T, engine string) {
	ctx := context.Background()
	c := engineConn(t, engine)
	res, err := c.Run(ctx, []string{"SELECT * FROM orders WHERE status = 'refunded' ORDER BY id"}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 3 || !res.Truncated {
		t.Fatalf("rows %d truncated %v", len(res.Rows), res.Truncated)
	}
	t.Logf("%s row: %v cols: %+v", engine, res.Rows[0], res.Columns)
	if _, err := c.Run(ctx, []string{"UPDATE orders SET status = 'x' WHERE id = 10400"}, false, 1); err == nil {
		t.Fatal("write on the read pool should fail")
	} else {
		t.Logf("%s read-only error: %v", engine, err)
	}
	if _, err := c.Run(ctx, []string{"SELECT * FROM orders WHERE id = 10400 FOR UPDATE"}, false, 1); err == nil && engine == "postgres" {
		t.Fatal("row lock in read-only transaction should fail on postgres")
	}
	res, err = c.Run(ctx, []string{"UPDATE orders SET currency = currency WHERE id = 10400"}, true, 1)
	if err != nil || res.Affected < 0 {
		t.Fatalf("write: %v %+v", err, res)
	}
	s, err := c.Schema(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s schema: %s %+v", engine, s.Version, s.Groups[0].Tables)
	g := s.Groups[0]
	tb, err := c.Table(ctx, g.Name, "orders")
	if err != nil {
		t.Fatal(err)
	}
	if tb.PK() == nil || tb.Column("customer_id").FK == nil || len(tb.ReferencedBy) != 2 || len(tb.Indexes) < 2 {
		t.Fatalf("table %+v", tb)
	}
	vals, err := c.EnumValues(ctx, tb)
	if err != nil || vals["status"]["refunded"] == "" {
		t.Fatalf("enum %v %v", vals, err)
	}
	if engine == "postgres" {
		// An enum column reports its own type name. Search reads its labels
		// from the catalog, so an empty table has them too.
		w, err := c.Writer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{"DROP TABLE IF EXISTS enum_probe", "DROP TYPE IF EXISTS probe_mood", "CREATE TYPE probe_mood AS ENUM ('happy', 'sad')", "CREATE TABLE enum_probe (id int PRIMARY KEY, mood probe_mood)"} {
			if _, err := w.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { _, _ = w.ExecContext(ctx, "DROP TABLE enum_probe; DROP TYPE probe_mood") })
		c.Invalidate()
		pt, err := c.Table(ctx, g.Name, "enum_probe")
		if err != nil {
			t.Fatal(err)
		}
		if vals, err := c.EnumValues(ctx, pt); err != nil || vals["mood"]["sad"] != "sad" {
			t.Fatalf("pg enum %v %v", vals, err)
		}
	}
	cs, err := c.Columns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	for _, tc := range cs.Tables {
		if tc.Schema == g.Name {
			found[tc.Name] = len(tc.Columns)
		}
	}
	if cs.Truncated || found["orders"] != len(tb.Columns) || found["v_monthly_revenue"] == 0 {
		t.Fatalf("column set %v truncated %v", found, cs.Truncated)
	}
	t.Logf("%s column set: %v", engine, found)
}

func TestPostgres(t *testing.T) { testEngine(t, "postgres") }
func TestMySQL(t *testing.T)    { testEngine(t, "mysql") }
