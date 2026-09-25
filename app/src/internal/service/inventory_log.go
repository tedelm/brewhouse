package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const inventoryLogMaxRows = 100

// ListItemLogs returns change-history rows for an inventory item (newest first).
// limit defaults to 5 and is clamped to inventoryLogMaxRows; hasMore is true when more rows exist after this page.
func (s *InventoryService) ListItemLogs(itemID int64, limit, offset int) ([]InventoryLogEntry, bool, error) {
	if _, err := s.Get(itemID); err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > inventoryLogMaxRows {
		limit = inventoryLogMaxRows
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(
		`SELECT id, inventory_item_id, item_name, created_at, username, email, summary
		 FROM inventory_item_logs
		 WHERE inventory_item_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ? OFFSET ?`,
		itemID, limit+1, offset,
	)
	if err != nil {
		return nil, false, fmt.Errorf("list inventory logs: %w", err)
	}
	defer rows.Close()

	var out []InventoryLogEntry
	for rows.Next() {
		var e InventoryLogEntry
		var itemIDNull sql.NullInt64
		if err := rows.Scan(
			&e.ID, &itemIDNull, &e.ItemName, &e.CreatedAt, &e.Username, &e.Email, &e.Summary,
		); err != nil {
			return nil, false, err
		}
		if itemIDNull.Valid {
			id := itemIDNull.Int64
			e.InventoryItemID = &id
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// appendItemLog inserts a change-history row and trims to inventoryLogMaxRows per item.
func (s *InventoryService) appendItemLog(db execQuerier, itemID *int64, itemName string, actor Actor, summary string) error {
	if summary == "" {
		return nil
	}
	username, email := "", ""
	if actor.UserID > 0 {
		err := db.QueryRow(
			`SELECT username, email FROM users WHERE id = ?`, actor.UserID,
		).Scan(&username, &email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("resolve log actor: %w", err)
		}
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	var idArg any
	if itemID != nil {
		idArg = *itemID
	}
	_, err := db.Exec(
		`INSERT INTO inventory_item_logs (inventory_item_id, item_name, created_at, username, email, summary)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		idArg, itemName, createdAt, username, email, summary,
	)
	if err != nil {
		return fmt.Errorf("insert inventory log: %w", err)
	}
	if itemID == nil {
		return nil
	}
	_, err = db.Exec(
		`DELETE FROM inventory_item_logs
		 WHERE inventory_item_id = ?
		   AND id NOT IN (
		     SELECT id FROM (
		       SELECT id FROM inventory_item_logs
		       WHERE inventory_item_id = ?
		       ORDER BY created_at DESC, id DESC
		       LIMIT ?
		     )
		   )`,
		*itemID, *itemID, inventoryLogMaxRows,
	)
	if err != nil {
		return fmt.Errorf("trim inventory log: %w", err)
	}
	return nil
}

func formatLogQty(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func inventoryCreateSummary(in InventoryItem) string {
	parts := []string{
		"qty=" + formatLogQty(in.Qty),
		"unit=" + in.Unit,
		"cost_price=" + formatLogQty(in.CostPrice),
	}
	if in.Producer != "" {
		parts = append(parts, "producer="+in.Producer)
	}
	if in.ItemType != "" {
		parts = append(parts, "item_type="+in.ItemType)
	}
	return "created item (" + strings.Join(parts, ", ") + ")"
}

func inventoryUpdateSummary(before, after InventoryItem) string {
	var parts []string
	if before.Name != after.Name {
		parts = append(parts, fmt.Sprintf("name: %q → %q", before.Name, after.Name))
	}
	if before.Unit != after.Unit {
		parts = append(parts, fmt.Sprintf("unit: %q → %q", before.Unit, after.Unit))
	}
	if before.Qty != after.Qty {
		parts = append(parts, fmt.Sprintf("qty: %s → %s", formatLogQty(before.Qty), formatLogQty(after.Qty)))
	}
	if before.CostPrice != after.CostPrice {
		parts = append(parts, fmt.Sprintf("cost_price: %s → %s", formatLogQty(before.CostPrice), formatLogQty(after.CostPrice)))
	}
	if before.Producer != after.Producer {
		parts = append(parts, fmt.Sprintf("producer: %q → %q", before.Producer, after.Producer))
	}
	if before.ItemType != after.ItemType {
		parts = append(parts, fmt.Sprintf("item_type: %q → %q", before.ItemType, after.ItemType))
	}
	if before.MinEBC != after.MinEBC {
		parts = append(parts, fmt.Sprintf("min_ebc: %s → %s", formatLogQty(before.MinEBC), formatLogQty(after.MinEBC)))
	}
	if before.MaxEBC != after.MaxEBC {
		parts = append(parts, fmt.Sprintf("max_ebc: %s → %s", formatLogQty(before.MaxEBC), formatLogQty(after.MaxEBC)))
	}
	if before.Link != after.Link {
		parts = append(parts, fmt.Sprintf("link: %q → %q", before.Link, after.Link))
	}
	return strings.Join(parts, "; ")
}
