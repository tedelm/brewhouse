package database

import (
	"bytes"
	"database/sql"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

//go:embed seed/malt.csv
var maltCSV []byte

//go:embed seed/hops.csv
var hopsCSV []byte

//go:embed seed/yeast.csv
var yeastCSV []byte

//go:embed seed/misc.csv
var miscCSV []byte

func seedInventoryCatalogs(db *sql.DB) error {
	if err := seedMalt(db); err != nil {
		return err
	}
	if err := seedCategoryCSV(db, "hops", "g", hopsCSV, false); err != nil {
		return err
	}
	if err := seedCategoryCSV(db, "yeast", "pack", yeastCSV, true); err != nil {
		return err
	}
	if err := seedCategoryCSV(db, "misc", "kg", miscCSV, false); err != nil {
		return err
	}
	return nil
}

func seedMalt(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_items WHERE category = 'malt'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	rows, err := parseMaltCSV(maltCSV)
	if err != nil {
		return fmt.Errorf("parse malt csv: %w", err)
	}
	for _, row := range rows {
		if _, err := db.Exec(
			`INSERT INTO inventory_items (category, name, unit, qty, cost_price, producer, item_type, min_ebc, max_ebc, link)
			 VALUES ('malt', ?, 'kg', 0, ?, ?, ?, ?, ?, ?)`,
			row.name, row.cost, row.producer, row.itemType, row.minEBC, row.maxEBC, row.link,
		); err != nil {
			return fmt.Errorf("insert malt %q (%s): %w", row.name, row.producer, err)
		}
	}
	return nil
}

// seedCategoryCSV inserts hops/yeast/misc rows when the category is empty.
func seedCategoryCSV(db *sql.DB, category, unit string, data []byte, joinSubType bool) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_items WHERE category = ?`, category).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	rows, err := parseInventoryCSV(data, joinSubType)
	if err != nil {
		return fmt.Errorf("parse %s csv: %w", category, err)
	}
	for _, row := range rows {
		if _, err := db.Exec(
			`INSERT INTO inventory_items (category, name, unit, qty, cost_price, producer, item_type, min_ebc, max_ebc, link)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?)`,
			category, row.name, unit, row.qty, row.cost, row.producer, row.itemType, row.link,
		); err != nil {
			return fmt.Errorf("insert %s %q: %w", category, row.name, err)
		}
	}
	return nil
}

type inventorySeedRow struct {
	name     string
	producer string
	itemType string
	qty      float64
	cost     float64
	minEBC   float64
	maxEBC   float64
	link     string
}

type maltSeedRow = inventorySeedRow

func parseInventoryCSV(data []byte, joinSubType bool) ([]inventorySeedRow, error) {
	records, idx, err := readSemicolonCSV(data)
	if err != nil {
		return nil, err
	}
	if colIndex(idx, "name") < 0 {
		return nil, fmt.Errorf("missing column name")
	}

	var out []inventorySeedRow
	seen := map[string]bool{}
	for _, rec := range records[1:] {
		if len(rec) == 0 {
			continue
		}
		get := func(keys ...string) string {
			for _, key := range keys {
				if i := colIndex(idx, key); i >= 0 && i < len(rec) {
					return cleanCSVField(rec[i])
				}
			}
			return ""
		}
		name := get("name")
		if name == "" {
			continue
		}
		producer := get("producer")
		itemType := get("type")
		if joinSubType {
			if sub := get("sub_type", "subtype"); sub != "" {
				if itemType != "" {
					itemType = itemType + " / " + sub
				} else {
					itemType = sub
				}
			}
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(producer)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, inventorySeedRow{
			name:     name,
			producer: producer,
			itemType: itemType,
			qty:      parseFloatField(get("inventory_amount", "qty")),
			cost:     parseFloatField(get("cost_price_per_kg", "cost_price")),
			link:     get("link"),
		})
	}
	return out, nil
}

func parseMaltCSV(data []byte) ([]maltSeedRow, error) {
	records, idx, err := readSemicolonCSV(data)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"name", "type", "producer", "min_ebc", "max_ebc", "cost_price_per_kg", "link"} {
		if colIndex(idx, key) < 0 {
			return nil, fmt.Errorf("missing column %s", key)
		}
	}

	best := map[string]maltSeedRow{}
	for _, rec := range records[1:] {
		if len(rec) == 0 {
			continue
		}
		get := func(key string) string {
			i := colIndex(idx, key)
			if i < 0 || i >= len(rec) {
				return ""
			}
			return cleanCSVField(rec[i])
		}
		name := get("name")
		producer := get("producer")
		if name == "" {
			continue
		}
		row := maltSeedRow{
			name:     name,
			producer: producer,
			itemType: get("type"),
			minEBC:   parseFloatField(get("min_ebc")),
			maxEBC:   parseFloatField(get("max_ebc")),
			cost:     parseFloatField(get("cost_price_per_kg")),
			link:     get("link"),
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(producer)
		if prev, ok := best[key]; ok {
			if maltTypeRank(row.itemType) <= maltTypeRank(prev.itemType) {
				continue
			}
		}
		best[key] = row
	}

	out := make([]maltSeedRow, 0, len(best))
	for _, row := range best {
		out = append(out, row)
	}
	return out, nil
}

func readSemicolonCSV(data []byte) ([][]string, map[string]int, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("empty csv")
	}

	idx := map[string]int{}
	for i, h := range records[0] {
		key := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		idx[key] = i
	}
	return records, idx, nil
}

func colIndex(idx map[string]int, name string) int {
	if i, ok := idx[strings.ToLower(name)]; ok {
		return i
	}
	return -1
}

func cleanCSVField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) && r != ' ' {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func parseFloatField(s string) float64 {
	s = cleanCSVField(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// maltTypeRank prefers grain-specific types over generic catalog labels when deduping.
func maltTypeRank(t string) int {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "wheat", "rye", "oats":
		return 3
	case "extract":
		return 2
	case "caramel", "roasted", "basemalt":
		return 1
	default:
		return 0
	}
}
