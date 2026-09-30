package service

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// inventoryCSVColumns returns export/import columns relevant for the category.
func inventoryCSVColumns(category string) []string {
	base := []string{"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "link"}
	switch category {
	case CategoryMalt:
		return []string{"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "min_ebc", "max_ebc", "link"}
	case CategoryYeast:
		return []string{
			"category", "name", "unit", "qty", "cost_price", "producer", "supplier", "item_type", "link",
			"pitch_min_g_hl", "pitch_max_g_hl", "pack_size_g", "temp_min_c", "temp_max_c",
		}
	default:
		return base
	}
}

// inventoryCSVRequiredColumns are headers required on import (supplier is optional).
func inventoryCSVRequiredColumns(category string) []string {
	cols := inventoryCSVColumns(category)
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		if c == "supplier" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func inventoryCSVRow(item InventoryItem, cols []string) []string {
	out := make([]string, len(cols))
	for i, col := range cols {
		switch col {
		case "category":
			out[i] = item.Category
		case "name":
			out[i] = item.Name
		case "unit":
			out[i] = item.Unit
		case "qty":
			out[i] = formatCSVFloat(item.Qty)
		case "cost_price":
			out[i] = formatCSVFloat(item.CostPrice)
		case "producer":
			out[i] = item.Producer
		case "supplier":
			out[i] = item.SupplierName
		case "item_type":
			out[i] = item.ItemType
		case "min_ebc":
			out[i] = formatCSVFloat(item.MinEBC)
		case "max_ebc":
			out[i] = formatCSVFloat(item.MaxEBC)
		case "link":
			out[i] = item.Link
		case "pitch_min_g_hl":
			out[i] = formatCSVFloat(item.PitchMinGHl)
		case "pitch_max_g_hl":
			out[i] = formatCSVFloat(item.PitchMaxGHl)
		case "pack_size_g":
			out[i] = formatCSVFloat(item.PackSizeG)
		case "temp_min_c":
			out[i] = formatCSVFloat(item.TempMinC)
		case "temp_max_c":
			out[i] = formatCSVFloat(item.TempMaxC)
		}
	}
	return out
}

// ExportInventoryCSV returns inventory items for a category as CSV.
func (s *InventoryService) ExportInventoryCSV(category string) ([]byte, error) {
	if !validCategory(category) {
		return nil, fmt.Errorf("invalid category")
	}
	items, err := s.ListByCategory(category)
	if err != nil {
		return nil, err
	}
	cols := inventoryCSVColumns(category)
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return nil, fmt.Errorf("write inventory csv header: %w", err)
	}
	for _, item := range items {
		if err := w.Write(inventoryCSVRow(item, cols)); err != nil {
			return nil, fmt.Errorf("write inventory csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const orderCSVHeader = "item_name,category,qty,ordered_qty,unit,cost_price,line_cost,brewery_name,link"

// ExportOrderCSV returns one order's lines as CSV.
func (s *InventoryService) ExportOrderCSV(orderID int64) ([]byte, error) {
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(strings.Split(orderCSVHeader, ",")); err != nil {
		return nil, fmt.Errorf("write order csv header: %w", err)
	}
	for _, line := range order.Lines {
		orderedQty := ""
		if line.OrderedQty != nil {
			orderedQty = formatCSVFloat(*line.OrderedQty)
		}
		if err := w.Write([]string{
			line.ItemName,
			line.Category,
			formatCSVFloat(line.Qty),
			orderedQty,
			line.Unit,
			formatCSVFloat(line.CostPrice),
			formatCSVFloat(line.LineCost),
			line.BreweryName,
			line.Link,
		}); err != nil {
			return nil, fmt.Errorf("write order csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportInventoryCSV upserts inventory items for a category from CSV.
// Rows are matched by (category, name, producer). Missing rows are never deleted.
func (s *InventoryService) ImportInventoryCSV(actor Actor, category string, data []byte) (ImportResult, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return ImportResult{}, err
	}
	if !ok {
		return ImportResult{}, ErrForbidden
	}
	if !validCategory(category) {
		return ImportResult{}, fmt.Errorf("invalid category")
	}
	records, err := readCSVRecords(data)
	if err != nil {
		return ImportResult{}, err
	}
	if len(records) == 0 {
		return ImportResult{}, fmt.Errorf("empty csv")
	}
	idx, err := mapCSVHeader(records[0], inventoryCSVRequiredColumns(category))
	if err != nil {
		return ImportResult{}, err
	}

	var result ImportResult
	for i, rec := range records[1:] {
		rowNum := i + 2
		action, err := s.importInventoryRow(actor, category, idx, rec)
		if err != nil {
			result.Failed++
			appendImportError(&result, rowNum, err)
			continue
		}
		switch action {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		}
	}
	return result, nil
}

func (s *InventoryService) importInventoryRow(actor Actor, category string, idx map[string]int, rec []string) (action string, err error) {
	rowCategory := strings.TrimSpace(csvCol(rec, idx, "category"))
	if rowCategory != "" && rowCategory != category {
		return "", fmt.Errorf("category %q does not match import category %q", rowCategory, category)
	}
	name := strings.TrimSpace(csvCol(rec, idx, "name"))
	if name == "" {
		return "", fmt.Errorf("name required")
	}
	unit := strings.TrimSpace(csvCol(rec, idx, "unit"))
	if unit == "" {
		unit = defaultUnitForCategory(category)
	}
	producer := strings.TrimSpace(csvCol(rec, idx, "producer"))
	itemType := strings.TrimSpace(csvCol(rec, idx, "item_type"))
	link := strings.TrimSpace(csvCol(rec, idx, "link"))
	supplierName := strings.TrimSpace(csvCol(rec, idx, "supplier"))

	qty, err := parseCSVFloat(csvCol(rec, idx, "qty"))
	if err != nil {
		return "", fmt.Errorf("qty: %w", err)
	}
	costPrice, err := parseCSVFloat(csvCol(rec, idx, "cost_price"))
	if err != nil {
		return "", fmt.Errorf("cost_price: %w", err)
	}
	minEBC, err := parseCSVFloat(csvCol(rec, idx, "min_ebc"))
	if err != nil {
		return "", fmt.Errorf("min_ebc: %w", err)
	}
	maxEBC, err := parseCSVFloat(csvCol(rec, idx, "max_ebc"))
	if err != nil {
		return "", fmt.Errorf("max_ebc: %w", err)
	}
	pitchMin, err := parseCSVFloat(csvCol(rec, idx, "pitch_min_g_hl"))
	if err != nil {
		return "", fmt.Errorf("pitch_min_g_hl: %w", err)
	}
	pitchMax, err := parseCSVFloat(csvCol(rec, idx, "pitch_max_g_hl"))
	if err != nil {
		return "", fmt.Errorf("pitch_max_g_hl: %w", err)
	}
	packSize, err := parseCSVFloat(csvCol(rec, idx, "pack_size_g"))
	if err != nil {
		return "", fmt.Errorf("pack_size_g: %w", err)
	}
	tempMin, err := parseCSVFloat(csvCol(rec, idx, "temp_min_c"))
	if err != nil {
		return "", fmt.Errorf("temp_min_c: %w", err)
	}
	tempMax, err := parseCSVFloat(csvCol(rec, idx, "temp_max_c"))
	if err != nil {
		return "", fmt.Errorf("temp_max_c: %w", err)
	}

	var supplierID *int64
	_, hasSupplierCol := idx["supplier"]
	if hasSupplierCol {
		if supplierName != "" {
			var id int64
			err := s.db.QueryRow(`SELECT id FROM suppliers WHERE name = ?`, supplierName).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("unknown supplier %q", supplierName)
			}
			if err != nil {
				return "", err
			}
			supplierID = &id
		}
	}

	in := InventoryItem{
		Category:    category,
		Name:        name,
		Unit:        unit,
		Qty:         qty,
		CostPrice:   costPrice,
		Producer:    producer,
		ItemType:    itemType,
		MinEBC:      minEBC,
		MaxEBC:      maxEBC,
		Link:        link,
		PitchMinGHl: pitchMin,
		PitchMaxGHl: pitchMax,
		PackSizeG:   packSize,
		TempMinC:    tempMin,
		TempMaxC:    tempMax,
		SupplierID:  supplierID,
	}

	existing, err := s.getByCategoryNameProducer(category, name, producer)
	if errors.Is(err, ErrNotFound) {
		if _, err := s.Create(actor, in); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}
	if !hasSupplierCol {
		in.SupplierID = existing.SupplierID
	}
	if _, err := s.Update(actor, existing.ID, in); err != nil {
		return "", err
	}
	return "updated", nil
}

func (s *InventoryService) getByCategoryNameProducer(category, name, producer string) (*InventoryItem, error) {
	item, err := scanInventoryItem(s.db.QueryRow(
		`SELECT `+inventorySelectCols+` FROM `+inventoryFrom+` WHERE i.category = ? AND i.name = ? AND i.producer = ?`,
		category, name, producer,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory by key: %w", err)
	}
	return &item, nil
}

func defaultUnitForCategory(category string) string {
	switch category {
	case CategoryHops:
		return "g"
	case CategoryYeast:
		return "pack"
	case CategoryEquipment:
		return "pcs"
	default:
		return "kg"
	}
}

func formatCSVFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func parseCSVFloat(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	return v, nil
}
