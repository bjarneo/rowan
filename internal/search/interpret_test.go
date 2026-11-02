package search

import (
	"testing"

	"rowan/internal/db"
)

func ordersTable() *db.Table {
	return &db.Table{Name: "orders", Columns: []db.ColumnInfo{
		{Name: "id", Type: "bigint", PK: true, Numeric: true},
		{Name: "customer_id", Type: "bigint", FK: &db.Ref{Table: "customers", Column: "id"}},
		{Name: "status", Type: "text"},
		{Name: "total_cents", Type: "integer", Numeric: true},
		{Name: "currency", Type: "char(3)"},
		{Name: "shipped_at", Type: "timestamptz", Nullable: true},
	}}
}

func TestInterpret(t *testing.T) {
	tb := ordersTable()
	enums := map[string]map[string]string{
		"status":   {"refunded": "refunded", "paid": "paid"},
		"currency": {"eur": "EUR", "usd": "USD"},
	}
	in := Interpret(Terms("refunded eur total_cents>5000 shipped_at:null"), tb, enums)
	if len(in.Filters) != 4 || len(in.Unresolved) != 0 {
		t.Fatalf("%+v", in)
	}
	if in.Canonical != "status:refunded currency:EUR total_cents>5000 shipped_at:null" {
		t.Fatalf("canonical %q", in.Canonical)
	}
	where, args := Where("postgres", tb, in.Filters)
	if where != "status = $1 AND currency = $2 AND total_cents > $3 AND shipped_at IS NULL" || len(args) != 3 || args[2] != int64(5000) {
		t.Fatalf("where %q %v", where, args)
	}
	if got := Literal("postgres", tb, in.Filters); got != "status = 'refunded' AND currency = 'EUR' AND total_cents > 5000 AND shipped_at IS NULL" {
		t.Fatalf("literal %q", got)
	}
	in = Interpret(Terms("10442"), tb, nil)
	if len(in.Filters) != 1 || in.Filters[0].Column != "id" {
		t.Fatalf("pk %+v", in)
	}
	in = Interpret(Terms("status shipped nope"), tb, enums)
	if len(in.Filters) != 1 || in.Filters[0].Value != "shipped" || len(in.Unresolved) != 1 {
		t.Fatalf("pair %+v", in)
	}
	if in.Note == "" {
		t.Fatal("expected a note for unresolved words")
	}
}
