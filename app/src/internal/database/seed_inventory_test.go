package database_test

import (
	"testing"

	"brewhouse/internal/database"
)

func TestOpenSeedsInventoryCatalogs(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var supplierID int64
	if err := db.QueryRow(`SELECT id FROM suppliers WHERE name = ?`, "mr malt").Scan(&supplierID); err != nil {
		t.Fatalf("expected mr malt supplier: %v", err)
	}

	cases := []struct {
		category string
		min      int
	}{
		{"malt", 80},
		{"hops", 50},
		{"yeast", 40},
		{"misc", 40},
		{"equipment", 20},
	}
	for _, tc := range cases {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_items WHERE category=?`, tc.category).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n < tc.min {
			t.Fatalf("%s: expected at least %d rows, got %d", tc.category, tc.min, n)
		}
		var linked int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM inventory_items WHERE category=? AND supplier_id=?`,
			tc.category, supplierID,
		).Scan(&linked); err != nil {
			t.Fatal(err)
		}
		if linked != n {
			t.Fatalf("%s: expected all %d rows linked to mr malt, got %d", tc.category, n, linked)
		}
	}
}
