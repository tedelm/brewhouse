package service

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"brewhouse/internal/database"
)

const inventorySelectCols = `i.id, i.category, i.name, i.unit, i.qty, i.cost_price, i.producer, i.item_type, i.min_ebc, i.max_ebc, i.link, i.pitch_min_g_hl, i.pitch_max_g_hl, i.pack_size_g, i.temp_min_c, i.temp_max_c, i.supplier_id, COALESCE(s.name, ''), COALESCE(s.adjust_percent, 0)`

const inventoryFrom = `inventory_items i LEFT JOIN suppliers s ON s.id = i.supplier_id`

// EffectiveCost returns base cost adjusted by a supplier percentage.
func EffectiveCost(base, adjustPercent float64) float64 {
	return math.Round(base*(1+adjustPercent/100)*1e6) / 1e6
}

// InventoryService manages shared stock and wishlist orders.
type InventoryService struct {
	db     *database.Holder
	access *AccessService
}

// NewInventoryService creates an InventoryService.
func NewInventoryService(db *database.Holder, access *AccessService) *InventoryService {
	return &InventoryService{db: db, access: access}
}

func scanInventoryItem(scanner interface {
	Scan(dest ...any) error
}) (InventoryItem, error) {
	var item InventoryItem
	var supplierID sql.NullInt64
	err := scanner.Scan(
		&item.ID, &item.Category, &item.Name, &item.Unit, &item.Qty, &item.CostPrice,
		&item.Producer, &item.ItemType, &item.MinEBC, &item.MaxEBC, &item.Link,
		&item.PitchMinGHl, &item.PitchMaxGHl, &item.PackSizeG, &item.TempMinC, &item.TempMaxC,
		&supplierID, &item.SupplierName, &item.AdjustPercent,
	)
	if err != nil {
		return item, err
	}
	if supplierID.Valid {
		id := supplierID.Int64
		item.SupplierID = &id
	}
	item.EffectiveCostPrice = EffectiveCost(item.CostPrice, item.AdjustPercent)
	return item, nil
}

func (s *InventoryService) resolveSupplierID(id *int64) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	var exists int64
	err := s.db.QueryRow(`SELECT id FROM suppliers WHERE id = ?`, *id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("supplier not found")
	}
	if err != nil {
		return nil, err
	}
	return id, nil
}

// ListByCategory returns inventory items for a category.
func (s *InventoryService) ListByCategory(category string) ([]InventoryItem, error) {
	rows, err := s.db.Query(
		`SELECT `+inventorySelectCols+` FROM `+inventoryFrom+` WHERE i.category = ? ORDER BY i.name, i.producer`,
		category,
	)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	var out []InventoryItem
	for rows.Next() {
		item, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListAll returns all inventory items.
func (s *InventoryService) ListAll() ([]InventoryItem, error) {
	rows, err := s.db.Query(
		`SELECT ` + inventorySelectCols + ` FROM ` + inventoryFrom + ` ORDER BY i.category, i.name, i.producer`,
	)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	var out []InventoryItem
	for rows.Next() {
		item, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Get returns one inventory item.
func (s *InventoryService) Get(id int64) (*InventoryItem, error) {
	item, err := scanInventoryItem(s.db.QueryRow(
		`SELECT `+inventorySelectCols+` FROM `+inventoryFrom+` WHERE i.id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory: %w", err)
	}
	return &item, nil
}

// Create adds an inventory item. Requires inventory manage permission.
func (s *InventoryService) Create(actor Actor, in InventoryItem) (*InventoryItem, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if !validCategory(in.Category) {
		return nil, fmt.Errorf("invalid category")
	}
	if in.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	if in.Unit == "" {
		in.Unit = "kg"
	}
	supplierID, err := s.resolveSupplierID(in.SupplierID)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO inventory_items (category, name, unit, qty, cost_price, producer, item_type, min_ebc, max_ebc, link,
			pitch_min_g_hl, pitch_max_g_hl, pack_size_g, temp_min_c, temp_max_c, supplier_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Category, in.Name, in.Unit, in.Qty, in.CostPrice,
		in.Producer, in.ItemType, in.MinEBC, in.MaxEBC, in.Link,
		in.PitchMinGHl, in.PitchMaxGHl, in.PackSizeG, in.TempMinC, in.TempMaxC, supplierID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert inventory: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.appendItemLog(s.db, &id, in.Name, actor, inventoryCreateSummary(in)); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Update updates an inventory item.
func (s *InventoryService) Update(actor Actor, id int64, in InventoryItem) (*InventoryItem, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if in.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	if in.Unit == "" {
		in.Unit = "kg"
	}
	before, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	supplierID, err := s.resolveSupplierID(in.SupplierID)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`UPDATE inventory_items SET name = ?, unit = ?, qty = ?, cost_price = ?,
		 producer = ?, item_type = ?, min_ebc = ?, max_ebc = ?, link = ?,
		 pitch_min_g_hl = ?, pitch_max_g_hl = ?, pack_size_g = ?, temp_min_c = ?, temp_max_c = ?,
		 supplier_id = ?
		 WHERE id = ?`,
		in.Name, in.Unit, in.Qty, in.CostPrice,
		in.Producer, in.ItemType, in.MinEBC, in.MaxEBC, in.Link,
		in.PitchMinGHl, in.PitchMaxGHl, in.PackSizeG, in.TempMinC, in.TempMaxC,
		supplierID, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update inventory: %w", err)
	}
	after, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if summary := inventoryUpdateSummary(*before, *after); summary != "" {
		if err := s.appendItemLog(s.db, &id, after.Name, actor, summary); err != nil {
			return nil, err
		}
	}
	return after, nil
}

// Delete removes an unused inventory item. Admin only; refuses if the item appears
// on any recipe or order line.
func (s *InventoryService) Delete(actor Actor, id int64) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	item, err := s.Get(id)
	if err != nil {
		return err
	}
	var recipeRefs, orderRefs int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM recipe_ingredients WHERE inventory_item_id = ?`, id,
	).Scan(&recipeRefs); err != nil {
		return fmt.Errorf("check recipe usage: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM inventory_order_lines WHERE inventory_item_id = ?`, id,
	).Scan(&orderRefs); err != nil {
		return fmt.Errorf("check order usage: %w", err)
	}
	if recipeRefs > 0 || orderRefs > 0 {
		return fmt.Errorf("inventory item is in use: %w", ErrConflict)
	}
	if err := s.appendItemLog(s.db, &id, item.Name, actor, "deleted item"); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM inventory_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete inventory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckShortfalls returns missing amounts for requested ingredients (no mutation).
func (s *InventoryService) CheckShortfalls(ingredients []IngredientInput) ([]StockShortfall, error) {
	var shortfalls []StockShortfall
	for _, in := range ingredients {
		if in.Qty <= 0 {
			return nil, fmt.Errorf("ingredient qty must be positive")
		}
		item, err := s.Get(in.InventoryItemID)
		if err != nil {
			return nil, err
		}
		if item.Qty < in.Qty {
			shortfalls = append(shortfalls, StockShortfall{
				InventoryItemID: item.ID,
				Name:            item.Name,
				Category:        item.Category,
				Requested:       in.Qty,
				Available:       item.Qty,
				Missing:         in.Qty - item.Qty,
			})
		}
	}
	return shortfalls, nil
}

// CreateOrder creates a planning order, optionally with lines.
func (s *InventoryService) CreateOrder(actor Actor, notes, externalOrderID string, lines []OrderLineInput) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(
		`INSERT INTO inventory_orders (status, notes, external_order_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		OrderStatusPlanning, notes, externalOrderID, actor.UserID, createdAt, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}
	orderID, _ := res.LastInsertId()
	for _, line := range lines {
		if err := s.insertOrderLineTx(tx, orderID, line.InventoryItemID, line.Qty, line.BreweryID, nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetOrder(orderID)
}

// LatestPlanningOrder returns the newest planning order, or ErrNotFound.
func (s *InventoryService) LatestPlanningOrder() (*InventoryOrder, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM inventory_orders WHERE status = ? ORDER BY id DESC LIMIT 1`,
		OrderStatusPlanning,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetOrder(id)
}

// EnsurePlanningOrder returns the latest planning order or creates an empty one.
func (s *InventoryService) EnsurePlanningOrder(actor Actor) (*InventoryOrder, error) {
	order, err := s.LatestPlanningOrder()
	if err == nil {
		return order, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return s.CreateOrder(actor, "", "", nil)
}

// AddOrderLine adds a catalog item to a planning order.
func (s *InventoryService) AddOrderLine(actor Actor, orderID, itemID int64, qty float64, breweryID *int64) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != OrderStatusPlanning {
		return nil, fmt.Errorf("order is not in planning")
	}
	if qty <= 0 {
		return nil, fmt.Errorf("qty must be positive")
	}
	if err := s.insertOrderLineTx(s.db, orderID, itemID, qty, breweryID, nil); err != nil {
		return nil, err
	}
	if err := s.touchOrderUpdatedAt(orderID); err != nil {
		return nil, err
	}
	return s.GetOrder(orderID)
}

type execQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func (s *InventoryService) insertOrderLineTx(db execQuerier, orderID, itemID int64, qty float64, breweryID, recipeID *int64) error {
	if qty <= 0 {
		return fmt.Errorf("qty must be positive")
	}
	item := &InventoryItem{}
	var adjustPercent float64
	err := db.QueryRow(
		`SELECT i.id, i.category, i.name, i.cost_price, COALESCE(s.adjust_percent, 0)
		 FROM inventory_items i
		 LEFT JOIN suppliers s ON s.id = i.supplier_id
		 WHERE i.id = ?`, itemID,
	).Scan(&item.ID, &item.Category, &item.Name, &item.CostPrice, &adjustPercent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get inventory: %w", err)
	}
	effectiveCost := EffectiveCost(item.CostPrice, adjustPercent)
	if breweryID != nil {
		var bid int64
		err = db.QueryRow(`SELECT id FROM breweries WHERE id = ?`, *breweryID).Scan(&bid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get brewery: %w", err)
		}
	}
	// Merge qty into an existing line for the same item+brewery+recipe on this order.
	var existingID int64
	err = db.QueryRow(
		`SELECT id FROM inventory_order_lines
		 WHERE order_id = ? AND inventory_item_id = ?
		   AND ((brewery_id IS NULL AND ? IS NULL) OR brewery_id = ?)
		   AND ((recipe_id IS NULL AND ? IS NULL) OR recipe_id = ?)`,
		orderID, itemID, breweryID, breweryID, recipeID, recipeID,
	).Scan(&existingID)
	if err == nil {
		_, err = db.Exec(`UPDATE inventory_order_lines SET qty = qty + ? WHERE id = ?`, qty, existingID)
		if err != nil {
			return fmt.Errorf("update order line: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO inventory_order_lines (order_id, inventory_item_id, item_name, category, qty, cost_price, brewery_id, recipe_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		orderID, item.ID, item.Name, item.Category, qty, effectiveCost, breweryID, recipeID,
	)
	if err != nil {
		return fmt.Errorf("insert order line: %w", err)
	}
	return nil
}

// UpdateOrder updates notes and/or external order id on a non-completed order.
func (s *InventoryService) UpdateOrder(actor Actor, orderID int64, notes *string, externalOrderID *string) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.Status == OrderStatusCompleted {
		return nil, fmt.Errorf("order is completed")
	}
	if notes == nil && externalOrderID == nil {
		return order, nil
	}
	n := order.Notes
	ext := order.ExternalOrderID
	if notes != nil {
		n = *notes
	}
	if externalOrderID != nil {
		ext = *externalOrderID
	}
	_, err = s.db.Exec(
		`UPDATE inventory_orders SET notes = ?, external_order_id = ?, updated_at = ? WHERE id = ?`,
		n, ext, time.Now().UTC().Format(time.RFC3339), orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update order: %w", err)
	}
	return s.GetOrder(orderID)
}

// UpdateOrderNotes updates notes on a non-completed order.
func (s *InventoryService) UpdateOrderNotes(actor Actor, orderID int64, notes string) (*InventoryOrder, error) {
	return s.UpdateOrder(actor, orderID, &notes, nil)
}

// DeleteOrder deletes a non-completed order (global admin only). Lines cascade.
func (s *InventoryService) DeleteOrder(actor Actor, orderID int64) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return err
	}
	if order.Status == OrderStatusCompleted {
		return fmt.Errorf("cannot delete completed order")
	}
	_, err = s.db.Exec(`DELETE FROM inventory_orders WHERE id = ?`, orderID)
	if err != nil {
		return fmt.Errorf("delete order: %w", err)
	}
	return nil
}

// SetOrderStatus transitions planning↔paused (admin), planning→ordered, or ordered→completed.
func (s *InventoryService) SetOrderStatus(actor Actor, orderID int64, status string) (*InventoryOrder, error) {
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	pauseOrResume := (order.Status == OrderStatusPlanning && status == OrderStatusPaused) ||
		(order.Status == OrderStatusPaused && status == OrderStatusPlanning)
	if pauseOrResume {
		if !actor.IsAdmin() {
			return nil, ErrForbidden
		}
		_, err = s.db.Exec(
			`UPDATE inventory_orders SET status = ?, updated_at = ? WHERE id = ?`,
			status, now, orderID,
		)
		if err != nil {
			return nil, fmt.Errorf("set status: %w", err)
		}
		return s.GetOrder(orderID)
	}

	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	switch {
	case order.Status == OrderStatusPlanning && status == OrderStatusOrdered:
		_, err = s.db.Exec(
			`UPDATE inventory_orders SET status = ?, ordered_at = ?, updated_at = ? WHERE id = ?`,
			OrderStatusOrdered, now, now, orderID,
		)
		if err != nil {
			return nil, fmt.Errorf("set ordered: %w", err)
		}
	case order.Status == OrderStatusOrdered && status == OrderStatusCompleted:
		if err := s.completeOrder(actor, order); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid status transition %s → %s", order.Status, status)
	}
	return s.GetOrder(orderID)
}

func (s *InventoryService) completeOrder(actor Actor, order *InventoryOrder) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, line := range order.Lines {
		if line.InventoryItemID == nil {
			continue
		}
		itemID := *line.InventoryItemID
		receive := line.Qty
		if line.OrderedQty != nil {
			receive = *line.OrderedQty
		}
		var qtyBefore float64
		var itemName string
		err := tx.QueryRow(
			`SELECT qty, name FROM inventory_items WHERE id = ?`, itemID,
		).Scan(&qtyBefore, &itemName)
		if err != nil {
			return fmt.Errorf("load item for receive: %w", err)
		}
		_, err = tx.Exec(
			`UPDATE inventory_items SET qty = qty + ? WHERE id = ?`,
			receive, itemID,
		)
		if err != nil {
			return fmt.Errorf("receive stock: %w", err)
		}
		qtyAfterReceive := qtyBefore + receive
		if receive != 0 {
			summary := fmt.Sprintf(
				"received +%s from order #%d (qty: %s → %s)",
				formatLogQty(receive), order.ID, formatLogQty(qtyBefore), formatLogQty(qtyAfterReceive),
			)
			if err := s.appendItemLog(tx, &itemID, itemName, actor, summary); err != nil {
				return err
			}
		}
		if line.RecipeID == nil || receive <= 0 {
			continue
		}
		var need, checkedOut float64
		err = tx.QueryRow(
			`SELECT qty, checked_out FROM recipe_ingredients WHERE recipe_id = ? AND inventory_item_id = ?`,
			*line.RecipeID, itemID,
		).Scan(&need, &checkedOut)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load recipe ingredient: %w", err)
		}
		missing := need - checkedOut
		if missing <= 0 {
			continue
		}
		allocate := receive
		if allocate > missing {
			allocate = missing
		}
		_, err = tx.Exec(
			`UPDATE recipe_ingredients SET checked_out = checked_out + ? WHERE recipe_id = ? AND inventory_item_id = ?`,
			allocate, *line.RecipeID, itemID,
		)
		if err != nil {
			return fmt.Errorf("allocate to recipe: %w", err)
		}
		_, err = tx.Exec(
			`UPDATE inventory_items SET qty = qty - ? WHERE id = ?`,
			allocate, itemID,
		)
		if err != nil {
			return fmt.Errorf("deduct allocated stock: %w", err)
		}
		qtyAfterAlloc := qtyAfterReceive - allocate
		summary := fmt.Sprintf(
			"recipe allocate −%s for recipe #%d (qty: %s → %s)",
			formatLogQty(allocate), *line.RecipeID, formatLogQty(qtyAfterReceive), formatLogQty(qtyAfterAlloc),
		)
		if err := s.appendItemLog(tx, &itemID, itemName, actor, summary); err != nil {
			return err
		}
	}
	_, err = tx.Exec(
		`UPDATE inventory_orders SET status = ?, updated_at = ? WHERE id = ?`,
		OrderStatusCompleted, time.Now().UTC().Format(time.RFC3339), order.ID,
	)
	if err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	return tx.Commit()
}

// CreateWishlistOrder creates or appends shortfall lines to a planning order.
func (s *InventoryService) CreateWishlistOrder(actor Actor, notes string, lines []StockShortfall) (*InventoryOrder, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("no lines")
	}
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	// Regular users may create planning orders from recipe shortfalls without inventory manage.
	// Admins/superusers always can; others may still create a planning wishlist.
	_ = ok

	order, err := s.LatestPlanningOrder()
	if errors.Is(err, ErrNotFound) {
		createdAt := time.Now().UTC().Format(time.RFC3339)
		if notes == "" {
			notes = "Wishlist"
		}
		res, err := s.db.Exec(
			`INSERT INTO inventory_orders (status, notes, external_order_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			OrderStatusPlanning, notes, "", actor.UserID, createdAt, createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert order: %w", err)
		}
		id, _ := res.LastInsertId()
		order, err = s.GetOrder(id)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	for _, line := range lines {
		qty := line.Missing
		if qty <= 0 {
			continue
		}
		if err := s.insertOrderLineTx(s.db, order.ID, line.InventoryItemID, qty, line.BreweryID, nil); err != nil {
			return nil, err
		}
	}
	if err := s.touchOrderUpdatedAt(order.ID); err != nil {
		return nil, err
	}
	return s.GetOrder(order.ID)
}

// SyncRecipeWishlist replaces this recipe's lines on the latest planning order.
// Empty shortfalls clear prior recipe lines when a planning order exists.
func (s *InventoryService) SyncRecipeWishlist(actor Actor, recipeID, breweryID int64, notes string, lines []StockShortfall) (*InventoryOrder, error) {
	var breweryPtr *int64
	if breweryID > 0 {
		breweryPtr = &breweryID
	}
	recipePtr := &recipeID

	order, err := s.LatestPlanningOrder()
	if errors.Is(err, ErrNotFound) {
		if len(lines) == 0 {
			return nil, nil
		}
		createdAt := time.Now().UTC().Format(time.RFC3339)
		if notes == "" {
			notes = "Wishlist"
		}
		res, err := s.db.Exec(
			`INSERT INTO inventory_orders (status, notes, external_order_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			OrderStatusPlanning, notes, "", actor.UserID, createdAt, createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert order: %w", err)
		}
		id, _ := res.LastInsertId()
		order, err = s.GetOrder(id)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if order.Status != OrderStatusPlanning {
		return nil, fmt.Errorf("order is not in planning")
	}

	_, err = s.db.Exec(
		`DELETE FROM inventory_order_lines WHERE order_id = ? AND recipe_id = ?`,
		order.ID, recipeID,
	)
	if err != nil {
		return nil, fmt.Errorf("clear recipe order lines: %w", err)
	}

	for _, line := range lines {
		qty := line.Missing
		if qty <= 0 {
			continue
		}
		bid := breweryPtr
		if line.BreweryID != nil {
			bid = line.BreweryID
		}
		if err := s.insertOrderLineTx(s.db, order.ID, line.InventoryItemID, qty, bid, recipePtr); err != nil {
			return nil, err
		}
	}
	if err := s.touchOrderUpdatedAt(order.ID); err != nil {
		return nil, err
	}
	return s.GetOrder(order.ID)
}

// ListOrders returns inventory orders newest first.
func (s *InventoryService) ListOrders() ([]InventoryOrder, error) {
	rows, err := s.db.Query(
		`SELECT id, status, notes, COALESCE(external_order_id, ''), COALESCE(created_by, 0), created_at,
		        COALESCE(updated_at, ''), ordered_at
		 FROM inventory_orders ORDER BY id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var out []InventoryOrder
	for rows.Next() {
		var o InventoryOrder
		var orderedAt sql.NullString
		if err := rows.Scan(
			&o.ID, &o.Status, &o.Notes, &o.ExternalOrderID, &o.CreatedBy, &o.CreatedAt,
			&o.UpdatedAt, &orderedAt,
		); err != nil {
			return nil, err
		}
		if orderedAt.Valid && orderedAt.String != "" {
			v := orderedAt.String
			o.OrderedAt = &v
		}
		lines, err := s.orderLines(o.ID)
		if err != nil {
			return nil, err
		}
		o.Lines = lines
		o.Total = orderLinesTotal(lines)
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountOpenOrders returns how many orders are in planning or ordered status.
func (s *InventoryService) CountOpenOrders() (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM inventory_orders WHERE status IN (?, ?)`,
		OrderStatusPlanning, OrderStatusOrdered,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count open orders: %w", err)
	}
	return n, nil
}

// GetOrder returns one order with lines.
func (s *InventoryService) GetOrder(id int64) (*InventoryOrder, error) {
	o := &InventoryOrder{}
	var orderedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, status, notes, COALESCE(external_order_id, ''), COALESCE(created_by, 0), created_at,
		        COALESCE(updated_at, ''), ordered_at
		 FROM inventory_orders WHERE id = ?`, id,
	).Scan(
		&o.ID, &o.Status, &o.Notes, &o.ExternalOrderID, &o.CreatedBy, &o.CreatedAt,
		&o.UpdatedAt, &orderedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if orderedAt.Valid && orderedAt.String != "" {
		v := orderedAt.String
		o.OrderedAt = &v
	}
	lines, err := s.orderLines(id)
	if err != nil {
		return nil, err
	}
	o.Lines = lines
	o.Total = orderLinesTotal(lines)
	return o, nil
}

func orderLinesTotal(lines []InventoryOrderLine) float64 {
	var total float64
	for _, line := range lines {
		total += line.LineCost
	}
	return total
}

func orderLineEffectiveQty(line InventoryOrderLine) float64 {
	if line.OrderedQty != nil {
		return *line.OrderedQty
	}
	return line.Qty
}

func (s *InventoryService) orderLines(orderID int64) ([]InventoryOrderLine, error) {
	rows, err := s.db.Query(
		`SELECT l.id, l.order_id, l.inventory_item_id, l.item_name, l.category, l.qty, l.ordered_qty,
		        COALESCE(l.cost_price, 0), COALESCE(i.unit, ''), COALESCE(i.link, ''),
		        l.brewery_id, COALESCE(b.name, ''), l.recipe_id
		 FROM inventory_order_lines l
		 LEFT JOIN inventory_items i ON i.id = l.inventory_item_id
		 LEFT JOIN breweries b ON b.id = l.brewery_id
		 WHERE l.order_id = ?
		 ORDER BY COALESCE(b.name, ''), l.item_name`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InventoryOrderLine
	for rows.Next() {
		var line InventoryOrderLine
		var itemID, breweryID, recipeID sql.NullInt64
		var orderedQty sql.NullFloat64
		if err := rows.Scan(
			&line.ID, &line.OrderID, &itemID, &line.ItemName, &line.Category, &line.Qty, &orderedQty,
			&line.CostPrice, &line.Unit, &line.Link, &breweryID, &line.BreweryName, &recipeID,
		); err != nil {
			return nil, err
		}
		if itemID.Valid {
			v := itemID.Int64
			line.InventoryItemID = &v
		}
		if orderedQty.Valid {
			v := orderedQty.Float64
			line.OrderedQty = &v
		}
		if breweryID.Valid {
			v := breweryID.Int64
			line.BreweryID = &v
		}
		if recipeID.Valid {
			v := recipeID.Int64
			line.RecipeID = &v
		}
		line.LineCost = line.CostPrice * orderLineEffectiveQty(line)
		out = append(out, line)
	}
	return out, rows.Err()
}

// UpdateOrderLineOrderedQty sets the actual purchase qty on a planning, paused, or ordered line.
func (s *InventoryService) UpdateOrderLineOrderedQty(actor Actor, orderID, lineID int64, orderedQty float64) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if orderedQty < 0 {
		return nil, fmt.Errorf("ordered qty must be zero or positive")
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != OrderStatusPlanning && order.Status != OrderStatusOrdered && order.Status != OrderStatusPaused {
		return nil, fmt.Errorf("order is completed")
	}
	var found bool
	for _, line := range order.Lines {
		if line.ID == lineID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}
	_, err = s.db.Exec(
		`UPDATE inventory_order_lines SET ordered_qty = ? WHERE id = ? AND order_id = ?`,
		orderedQty, lineID, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update ordered qty: %w", err)
	}
	if err := s.touchOrderUpdatedAt(orderID); err != nil {
		return nil, err
	}
	return s.GetOrder(orderID)
}

// UpdateOrderLineCostPrice sets the unit cost on a planning, paused, or ordered line.
func (s *InventoryService) UpdateOrderLineCostPrice(actor Actor, orderID, lineID int64, costPrice float64) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if costPrice < 0 {
		return nil, fmt.Errorf("cost price must be zero or positive")
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != OrderStatusPlanning && order.Status != OrderStatusOrdered && order.Status != OrderStatusPaused {
		return nil, fmt.Errorf("order is completed")
	}
	var found bool
	for _, line := range order.Lines {
		if line.ID == lineID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}
	_, err = s.db.Exec(
		`UPDATE inventory_order_lines SET cost_price = ? WHERE id = ? AND order_id = ?`,
		costPrice, lineID, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update cost price: %w", err)
	}
	if err := s.touchOrderUpdatedAt(orderID); err != nil {
		return nil, err
	}
	return s.GetOrder(orderID)
}

// UpdateOrderLineProductLink sets the catalog product URL for a line's inventory item.
func (s *InventoryService) UpdateOrderLineProductLink(actor Actor, orderID, lineID int64, link string) (*InventoryOrder, error) {
	ok, err := s.access.CanManageInventory(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	order, err := s.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	var itemID *int64
	for _, line := range order.Lines {
		if line.ID == lineID {
			itemID = line.InventoryItemID
			break
		}
	}
	if itemID == nil {
		return nil, ErrNotFound
	}
	before, err := s.Get(*itemID)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`UPDATE inventory_items SET link = ? WHERE id = ?`, link, *itemID)
	if err != nil {
		return nil, fmt.Errorf("update product link: %w", err)
	}
	if before.Link != link {
		summary := fmt.Sprintf("link: %q → %q", before.Link, link)
		if err := s.appendItemLog(s.db, itemID, before.Name, actor, summary); err != nil {
			return nil, err
		}
	}
	return s.GetOrder(orderID)
}

func (s *InventoryService) touchOrderUpdatedAt(orderID int64) error {
	_, err := s.db.Exec(
		`UPDATE inventory_orders SET updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), orderID,
	)
	if err != nil {
		return fmt.Errorf("touch order updated_at: %w", err)
	}
	return nil
}

func validCategory(c string) bool {
	switch c {
	case CategoryMalt, CategoryHops, CategoryYeast, CategoryMisc, CategoryEquipment:
		return true
	default:
		return false
	}
}
