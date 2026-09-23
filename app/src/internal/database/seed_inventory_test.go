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

	cases := []struct {
		category string
		min      int
	}{
		{"malt", 80},
		{"hops", 50},
		{"yeast", 40},
		{"misc", 40},
	}
	for _, tc := range cases {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_items WHERE category=?`, tc.category).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n < tc.min {
			t.Fatalf("%s: expected at least %d rows, got %d", tc.category, tc.min, n)
		}
	}
}
