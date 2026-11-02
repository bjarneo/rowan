package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rowan/internal/config"
)

func sampleConn(t *testing.T) *Conn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.sqlite")
	if err := CreateSampleFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	t.Cleanup(m.CloseAll)
	return m.Conn(config.Connection{ID: "s", Name: "sample", Engine: "sqlite", File: path})
}

func TestSQLiteReadOnly(t *testing.T) {
	ctx := context.Background()
	c := sampleConn(t)
	res, err := c.Run(ctx, []string{"SELECT * FROM orders WHERE status = 'refunded'"}, false, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 5 || !res.Truncated || len(res.Columns) != 7 {
		t.Fatalf("rows %d truncated %v cols %d", len(res.Rows), res.Truncated, len(res.Columns))
	}
	if !res.Columns[0].Numeric {
		t.Fatalf("id should be numeric: %+v", res.Columns[0])
	}
	// The read pool must refuse writes even when the caller skips the checks.
	if _, err := c.Run(ctx, []string{"UPDATE orders SET status = 'x' WHERE id = 10400"}, false, 10); err == nil {
		t.Fatal("write on the read pool should fail")
	}
	if _, err := c.Run(ctx, []string{"PRAGMA query_only = 0", "DELETE FROM orders"}, false, 10); err == nil {
		t.Fatal("pragma escape should still fail on the read pool")
	}
	res, err = c.Run(ctx, []string{"UPDATE orders SET status = 'refunded' WHERE id = 10400"}, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Affected != 1 || res.Message != "UPDATE 1" {
		t.Fatalf("affected %d message %q", res.Affected, res.Message)
	}
}

func TestSQLiteIntrospect(t *testing.T) {
	ctx := context.Background()
	c := sampleConn(t)
	s, err := c.Schema(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Groups) != 1 || len(s.Groups[0].Tables) != 6 || !strings.HasPrefix(s.Version, "SQLite") {
		t.Fatalf("schema %+v", s)
	}
	for _, tb := range s.Groups[0].Tables {
		if tb.Name == "orders" && (tb.Rows != 8000 || tb.Estimate) {
			t.Fatalf("orders rows %d estimate %v", tb.Rows, tb.Estimate)
		}
	}
	tb, err := c.Table(ctx, "main", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if tb.PK() == nil || tb.PK().Name != "id" {
		t.Fatalf("pk %+v", tb.PK())
	}
	if fk := tb.Column("customer_id").FK; fk == nil || fk.Table != "customers" || fk.Column != "id" {
		t.Fatalf("fk %+v", fk)
	}
	if len(tb.ReferencedBy) != 2 {
		t.Fatalf("referenced by %+v", tb.ReferencedBy)
	}
	vals, err := c.EnumValues(ctx, tb)
	if err != nil {
		t.Fatal(err)
	}
	if vals["status"]["refunded"] != "refunded" {
		t.Fatalf("enum values %+v", vals)
	}
	n, capped, err := c.Count(ctx, "main", "orders", "status = ?", []any{"refunded"}, 10000)
	if err != nil || n == 0 || capped {
		t.Fatalf("count %d %v %v", n, capped, err)
	}
}

func TestColumns(t *testing.T) {
	ctx := context.Background()
	c := sampleConn(t)
	s, err := c.Columns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Truncated || len(s.Tables) != 6 {
		t.Fatalf("truncated %v tables %d", s.Truncated, len(s.Tables))
	}
	var orders *TableColumns
	for i := range s.Tables {
		if s.Tables[i].Name == "orders" {
			orders = &s.Tables[i]
		}
	}
	if orders == nil || orders.Schema != "main" || len(orders.Columns) != 7 || orders.Columns[0].Name != "id" {
		t.Fatalf("orders %+v", orders)
	}
	// A write drops the cache, so a new column shows up.
	if _, err := c.Run(ctx, []string{"ALTER TABLE orders ADD COLUMN note TEXT"}, true, 1); err != nil {
		t.Fatal(err)
	}
	s, err = c.Columns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range s.Tables {
		if tb.Name == "orders" && (len(tb.Columns) != 8 || tb.Columns[7].Name != "note" || tb.Columns[7].Type != "TEXT") {
			t.Fatalf("orders after alter %+v", tb.Columns)
		}
	}
}

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "rowan-test")
	os.Setenv("ROWAN_CONFIG_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestSessionStateDoesNotLeak(t *testing.T) {
	ctx := context.Background()
	c := sampleConn(t)
	// An open transaction on the read pool must not survive the script.
	if _, err := c.Run(ctx, []string{"BEGIN", "SELECT 1"}, false, 1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := c.Run(ctx, []string{"BEGIN", "SELECT 1", "COMMIT"}, false, 1); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}
