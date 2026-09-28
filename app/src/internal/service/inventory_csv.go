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

const inventoryCSVHeader = "category,name,unit,qty,cost_price,producer,item_type,min_ebc,max_ebc,link"

// ExportInventoryCSV returns inventory items for a category as CSV.
func (s *InventoryService) ExportInventoryCSV(category string) ([]byte, error) {
	if !validCategory(category) {
		return nil, fmt.Errorf("invalid category")
	}
	items, err := s.ListByCategory(category)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(strings.Split(inventoryCSVHeader, ",")); err != nil {
		return nil, fmt.Errorf("write inventory csv header: %w", err)
	}
	for _, item := range items {
		if err := w.Write([]string{
			item.Category,
			item.Name,
			item.Unit,
			formatCSVFloat(item.Qty),
			formatCSVFloat(item.CostPrice),
			item.Producer,
			item.ItemType,
			formatCSVFloat(item.MinEBC),
			formatCSVFloat(item.MaxEBC),
			item.Link,
		}); err != nil {
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
	idx, err := mapCSVHeader(records[0], []string{
		"category", "name", "unit", "qty", "cost_price",
		"producer", "item_type", "min_ebc", "max_ebc", "link",
	})
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

	in := InventoryItem{
		Category:  category,
		Name:      name,
		Unit:      unit,
		Qty:       qty,
		CostPrice: costPrice,
		Producer:  producer,
		ItemType:  itemType,
		MinEBC:    minEBC,
		MaxEBC:    maxEBC,
		Link:      link,
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
	if _, err := s.Update(actor, existing.ID, in); err != nil {
		return "", err
	}
	return "updated", nil
}

func (s *InventoryService) getByCategoryNameProducer(category, name, producer string) (*InventoryItem, error) {
	item, err := scanInventoryItem(s.db.QueryRow(
		`SELECT `+inventorySelectCols+` FROM inventory_items WHERE category = ? AND name = ? AND producer = ?`,
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
