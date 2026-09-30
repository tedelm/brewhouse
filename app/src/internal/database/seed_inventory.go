package database

import (
	"bytes"
	"database/sql"
	_ "embed"
	"encoding/csv"
	"errors"
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

//go:embed seed/equipment.csv
var equipmentCSV []byte

func seedInventoryCatalogs(db *sql.DB) error {
	if err := seedDefaultSupplier(db); err != nil {
		return err
	}
	if err := seedInventoryExportCSV(db, "malt", maltCSV); err != nil {
		return err
	}
	if err := seedInventoryExportCSV(db, "hops", hopsCSV); err != nil {
		return err
	}
	if err := seedInventoryExportCSV(db, "yeast", yeastCSV); err != nil {
		return err
	}
	if err := seedInventoryExportCSV(db, "misc", miscCSV); err != nil {
		return err
	}
	if err := seedInventoryExportCSV(db, "equipment", equipmentCSV); err != nil {
		return err
	}
	return nil
}

const defaultSupplierName = "mr malt"

func seedDefaultSupplier(db *sql.DB) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO suppliers (name, adjust_percent, active) VALUES (?, 0, 1)`,
		defaultSupplierName,
	)
	return err
}

func supplierIDByName(db *sql.DB, name string) (sql.NullInt64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return sql.NullInt64{}, nil
	}
	var id int64
	err := db.QueryRow(`SELECT id FROM suppliers WHERE name = ?`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, fmt.Errorf("unknown supplier %q", name)
	}
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

// seedInventoryExportCSV inserts rows from inventory export/import CSV when the category is empty.
func seedInventoryExportCSV(db *sql.DB, category string, data []byte) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM inventory_items WHERE category = ?`, category).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	rows, err := parseInventoryExportCSV(data, category)
	if err != nil {
		return fmt.Errorf("parse %s csv: %w", category, err)
	}
	for _, row := range rows {
		supplierID, err := supplierIDByName(db, row.supplier)
		if err != nil {
			return fmt.Errorf("supplier for %s %q: %w", category, row.name, err)
		}
		if _, err := db.Exec(
			`INSERT INTO inventory_items (
				category, name, unit, qty, cost_price, producer, item_type,
				min_ebc, max_ebc, link,
				pitch_min_g_hl, pitch_max_g_hl, pack_size_g, temp_min_c, temp_max_c,
				supplier_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			category, row.name, row.unit, row.qty, row.costPrice, row.producer, row.itemType,
			row.minEBC, row.maxEBC, row.link,
			row.pitchMin, row.pitchMax, row.packSize, row.tempMin, row.tempMax,
			supplierID,
		); err != nil {
			return fmt.Errorf("insert %s %q (%s): %w", category, row.name, row.producer, err)
		}
	}
	return nil
}

type inventoryExportSeedRow struct {
	name, unit, producer, itemType, link, supplier string
	qty, costPrice, minEBC, maxEBC                 float64
	pitchMin, pitchMax, packSize                   float64
	tempMin, tempMax                               float64
}

func defaultUnitForCategory(category string) string {
	switch category {
	case "hops":
		return "g"
	case "yeast":
		return "pack"
	case "equipment":
		return "pcs"
	default:
		return "kg"
	}
}

// inventorySeedColumns returns export/import columns relevant for the category (mirrors service).
func inventorySeedColumns(category string) []string {
	switch category {
	case "malt":
		return []string{"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "min_ebc", "max_ebc", "link"}
	case "yeast":
		return []string{
			"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "link",
			"pitch_min_g_hl", "pitch_max_g_hl", "pack_size_g", "temp_min_c", "temp_max_c",
		}
	default:
		return []string{"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "link"}
	}
}

// parseInventoryExportCSV parses inventory export/import format (comma-separated).
func parseInventoryExportCSV(data []byte, category string) ([]inventoryExportSeedRow, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	r := csv.NewReader(bytes.NewReader(data))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("empty csv")
	}
	idx := map[string]int{}
	for i, h := range records[0] {
		key := strings.ToLower(strings.TrimSpace(h))
		idx[key] = i
	}
	for _, col := range inventorySeedColumns(category) {
		if colIndex(idx, col) < 0 {
			return nil, fmt.Errorf("missing column %s", col)
		}
	}

	get := func(rec []string, key string) string {
		i := colIndex(idx, key)
		if i < 0 || i >= len(rec) {
			return ""
		}
		return cleanCSVField(rec[i])
	}

	defaultUnit := defaultUnitForCategory(category)
	var out []inventoryExportSeedRow
	seen := map[string]bool{}
	for _, rec := range records[1:] {
		if len(rec) == 0 {
			continue
		}
		name := get(rec, "name")
		if name == "" {
			continue
		}
		producer := get(rec, "producer")
		key := strings.ToLower(name) + "\x00" + strings.ToLower(producer)
		if seen[key] {
			continue
		}
		seen[key] = true
		unit := get(rec, "unit")
		if unit == "" {
			unit = defaultUnit
		}
		out = append(out, inventoryExportSeedRow{
			name:      name,
			unit:      unit,
			producer:  producer,
			itemType:  get(rec, "item_type"),
			link:      get(rec, "link"),
			supplier:  get(rec, "supplier"),
			qty:       parseFloatField(get(rec, "qty")),
			costPrice: parseFloatField(get(rec, "cost_price")),
			minEBC:    parseFloatField(get(rec, "min_ebc")),
			maxEBC:    parseFloatField(get(rec, "max_ebc")),
			pitchMin:  parseFloatField(get(rec, "pitch_min_g_hl")),
			pitchMax:  parseFloatField(get(rec, "pitch_max_g_hl")),
			packSize:  parseFloatField(get(rec, "pack_size_g")),
			tempMin:   parseFloatField(get(rec, "temp_min_c")),
			tempMax:   parseFloatField(get(rec, "temp_max_c")),
		})
	}
	return out, nil
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
