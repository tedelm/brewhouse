package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecipeService manages recipe-as-batch lifecycle through create/delete and lookups.
type RecipeService struct {
	db        *sql.DB
	access    *AccessService
	inventory *InventoryService
	settings  *SettingsService
}

// NewRecipeService creates a RecipeService.
func NewRecipeService(db *sql.DB, access *AccessService, inventory *InventoryService, settings *SettingsService) *RecipeService {
	return &RecipeService{db: db, access: access, inventory: inventory, settings: settings}
}

// List returns recipes for a brewery (or all accessible if breweryID is 0 for admin).
// When includeHidden is false, only active recipes are returned.
func (s *RecipeService) List(actor Actor, breweryID int64, includeHidden bool) ([]Recipe, error) {
	activeFilter := ""
	activeFilterAliased := ""
	if !includeHidden {
		activeFilter = ` AND active = 1`
		activeFilterAliased = ` AND r.active = 1`
	}
	var out []Recipe
	var err error
	if breweryID > 0 {
		if err := s.access.RequireBreweryAccess(actor, breweryID); err != nil {
			return nil, err
		}
		out, err = s.queryRecipes(
			`SELECT `+recipeColumns+` FROM recipes WHERE brewery_id = ?`+activeFilter+` ORDER BY id DESC`,
			breweryID,
		)
	} else if actor.IsAdmin() {
		where := ""
		if !includeHidden {
			where = ` WHERE active = 1`
		}
		out, err = s.queryRecipes(`SELECT ` + recipeColumns + ` FROM recipes` + where + ` ORDER BY id DESC`)
	} else {
		out, err = s.queryRecipes(
			`SELECT `+recipeColumnsAliased+` FROM recipes r
			 INNER JOIN brewery_members m ON m.brewery_id = r.brewery_id
			 WHERE m.user_id = ?`+activeFilterAliased+`
			 ORDER BY r.id DESC`,
			actor.UserID,
		)
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachBreweryNames(out); err != nil {
		return nil, err
	}
	if err := s.attachIngredientsStatus(out); err != nil {
		return nil, err
	}
	return out, nil
}

const recipeColumns = `id, brewery_id, name, status, booked_date, tank_id, og, fg, brew_volume, delivery_volume, cost, tax, net, created_by, created_at, delivered_at, active`

const recipeColumnsAliased = `r.id, r.brewery_id, r.name, r.status, r.booked_date, r.tank_id, r.og, r.fg, r.brew_volume, r.delivery_volume, r.cost, r.tax, r.net, r.created_by, r.created_at, r.delivered_at, r.active`

func (s *RecipeService) queryRecipes(query string, args ...any) ([]Recipe, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	defer rows.Close()
	var out []Recipe
	for rows.Next() {
		r, err := scanRecipe(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *RecipeService) attachBreweryNames(recipes []Recipe) error {
	if len(recipes) == 0 {
		return nil
	}
	ids := map[int64]struct{}{}
	for _, r := range recipes {
		ids[r.BreweryID] = struct{}{}
	}
	names := map[int64]string{}
	for id := range ids {
		var name string
		err := s.db.QueryRow(`SELECT name FROM breweries WHERE id = ?`, id).Scan(&name)
		if err != nil {
			return fmt.Errorf("brewery name: %w", err)
		}
		names[id] = name
	}
	for i := range recipes {
		recipes[i].BreweryName = names[recipes[i].BreweryID]
	}
	return nil
}

func (s *RecipeService) attachIngredientsStatus(recipes []Recipe) error {
	for i := range recipes {
		ings, err := s.loadIngredients(recipes[i].ID)
		if err != nil {
			return err
		}
		recipes[i].Ingredients = ings
		recipes[i].IngredientStatus = IngredientStatusForRecipe(recipes[i].Status, ings)
	}
	return nil
}

// IngredientStatusForRecipe derives fulfillment from recipe batch status and lines.
// Brewday and later always report completed (ingredients already used).
func IngredientStatusForRecipe(status string, ings []RecipeIngredient) string {
	switch status {
	case StatusBrewday, StatusHygieneDone, StatusReadyForDelivery, StatusDelivered:
		return IngredientStatusCompleted
	}
	return IngredientStatusFor(ings)
}

// IngredientStatusFor derives aggregate fulfillment from recipe ingredient lines.
func IngredientStatusFor(ings []RecipeIngredient) string {
	hasShort := false
	anyCheckedOut := false
	for _, ing := range ings {
		if ing.CheckedOut > 0 {
			anyCheckedOut = true
		}
		if ing.CheckedOut < ing.Qty {
			hasShort = true
		}
	}
	if !hasShort {
		return IngredientStatusOK
	}
	if anyCheckedOut {
		return IngredientStatusPartial
	}
	return IngredientStatusShort
}

// IngredientLineStatus derives fulfillment for a single ingredient line.
func IngredientLineStatus(qty, checkedOut float64) string {
	if checkedOut >= qty {
		return IngredientStatusOK
	}
	if checkedOut > 0 {
		return IngredientStatusPartial
	}
	return IngredientStatusShort
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecipe(row scannable) (*Recipe, error) {
	r := &Recipe{}
	var bookedDate, deliveredAt sql.NullString
	var tankID, createdBy sql.NullInt64
	var og, fg, brewVol, delVol, cost, tax, net sql.NullFloat64
	var active int
	err := row.Scan(
		&r.ID, &r.BreweryID, &r.Name, &r.Status,
		&bookedDate, &tankID, &og, &fg, &brewVol, &delVol, &cost, &tax, &net,
		&createdBy, &r.CreatedAt, &deliveredAt, &active,
	)
	if err != nil {
		return nil, err
	}
	r.Active = active != 0
	if bookedDate.Valid {
		r.BookedDate = &bookedDate.String
	}
	if tankID.Valid {
		v := tankID.Int64
		r.TankID = &v
	}
	if og.Valid {
		v := og.Float64
		r.OG = &v
	}
	if fg.Valid {
		v := fg.Float64
		r.FG = &v
	}
	if brewVol.Valid {
		v := brewVol.Float64
		r.BrewVolume = &v
	}
	if delVol.Valid {
		v := delVol.Float64
		r.DeliveryVolume = &v
	}
	if cost.Valid {
		v := cost.Float64
		r.Cost = &v
	}
	if tax.Valid {
		v := tax.Float64
		r.Tax = &v
	}
	if net.Valid {
		v := net.Float64
		r.Net = &v
	}
	if createdBy.Valid {
		v := createdBy.Int64
		r.CreatedBy = &v
	}
	if deliveredAt.Valid {
		r.DeliveredAt = &deliveredAt.String
	}
	return r, nil
}

// Get returns a recipe with ingredients.
func (s *RecipeService) Get(actor Actor, id int64) (*Recipe, error) {
	row := s.db.QueryRow(`SELECT `+recipeColumns+` FROM recipes WHERE id = ?`, id)
	r, err := scanRecipe(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get recipe: %w", err)
	}
	if err := s.access.RequireBreweryAccess(actor, r.BreweryID); err != nil {
		return nil, err
	}
	ings, err := s.loadIngredients(id)
	if err != nil {
		return nil, err
	}
	r.Ingredients = ings
	r.IngredientStatus = IngredientStatusForRecipe(r.Status, ings)
	return r, nil
}

func (s *RecipeService) loadIngredients(recipeID int64) ([]RecipeIngredient, error) {
	rows, err := s.db.Query(
		`SELECT ri.id, ri.recipe_id, ri.inventory_item_id, i.name, i.category, ri.qty, ri.unit, ri.checked_out, ri.cost_price
		 FROM recipe_ingredients ri
		 INNER JOIN inventory_items i ON i.id = ri.inventory_item_id
		 WHERE ri.recipe_id = ?`,
		recipeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecipeIngredient
	for rows.Next() {
		var ing RecipeIngredient
		if err := rows.Scan(
			&ing.ID, &ing.RecipeID, &ing.InventoryItemID, &ing.ItemName, &ing.Category,
			&ing.Qty, &ing.Unit, &ing.CheckedOut, &ing.CostPrice,
		); err != nil {
			return nil, err
		}
		out = append(out, ing)
	}
	return out, rows.Err()
}

// CreateResult is returned from Create/Update.
type CreateResult struct {
	Recipe     *Recipe          `json:"recipe,omitempty"`
	Shortfalls []StockShortfall `json:"shortfalls,omitempty"`
	OrderID    int64            `json:"order_id,omitempty"`
}

// Create checks out available inventory and creates a recipe. Missing amounts go to a planning order.
func (s *RecipeService) Create(actor Actor, breweryID int64, name string, ingredients []IngredientInput) (*CreateResult, error) {
	if err := s.access.RequireBreweryAccess(actor, breweryID); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if len(ingredients) == 0 {
		return nil, fmt.Errorf("at least one ingredient required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(
		`INSERT INTO recipes (brewery_id, name, status, created_by, created_at, active) VALUES (?, ?, ?, ?, ?, 1)`,
		breweryID, name, StatusCreated, actor.UserID, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert recipe: %w", err)
	}
	recipeID, _ := res.LastInsertId()

	shortfalls, err := s.checkoutIngredients(tx, actor, recipeID, breweryID, ingredients, nil)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	recipe, err := s.Get(actor, recipeID)
	if err != nil {
		return nil, err
	}
	result := &CreateResult{Recipe: recipe, Shortfalls: shortfalls}
	if len(shortfalls) > 0 {
		order, err := s.inventory.SyncRecipeWishlist(actor, recipeID, breweryID, "From recipe create", shortfalls)
		if err != nil {
			return result, fmt.Errorf("recipe created but wishlist failed: %w", err)
		}
		if order != nil {
			result.OrderID = order.ID
		}
	}
	return result, nil
}

// Update restores prior checkout, then checks out new ingredients (partial OK) and updates the name.
func (s *RecipeService) Update(actor Actor, id int64, name string, ingredients []IngredientInput) (*CreateResult, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusCreated && recipe.Status != StatusScheduled {
		return nil, ErrInvalidStatus
	}
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if len(ingredients) == 0 {
		return nil, fmt.Errorf("at least one ingredient required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, ing := range recipe.Ingredients {
		if ing.CheckedOut <= 0 {
			continue
		}
		var qtyBefore float64
		err := tx.QueryRow(`SELECT qty FROM inventory_items WHERE id = ?`, ing.InventoryItemID).Scan(&qtyBefore)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(
			`UPDATE inventory_items SET qty = qty + ? WHERE id = ?`,
			ing.CheckedOut, ing.InventoryItemID,
		)
		if err != nil {
			return nil, err
		}
		itemID := ing.InventoryItemID
		qtyAfter := qtyBefore + ing.CheckedOut
		summary := fmt.Sprintf(
			"recipe restore +%s (qty: %s → %s)",
			formatLogQty(ing.CheckedOut), formatLogQty(qtyBefore), formatLogQty(qtyAfter),
		)
		if err := s.inventory.appendItemLog(tx, &itemID, ing.ItemName, actor, summary); err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(`DELETE FROM recipe_ingredients WHERE recipe_id = ?`, id)
	if err != nil {
		return nil, err
	}

	shortfalls, err := s.checkoutIngredients(tx, actor, id, recipe.BreweryID, ingredients, nil)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`UPDATE recipes SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return nil, fmt.Errorf("update recipe name: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	updated, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	result := &CreateResult{Recipe: updated, Shortfalls: shortfalls}
	order, err := s.inventory.SyncRecipeWishlist(actor, id, recipe.BreweryID, "From recipe edit", shortfalls)
	if err != nil {
		return result, fmt.Errorf("recipe updated but wishlist failed: %w", err)
	}
	if order != nil {
		result.OrderID = order.ID
	}
	return result, nil
}

// checkoutIngredients deducts available stock and inserts BOM lines (full requested qty).
// Returns shortfalls for amounts not checked out. needQty is in inventory item units.
func (s *RecipeService) checkoutIngredients(tx *sql.Tx, actor Actor, recipeID, breweryID int64, ingredients []IngredientInput, _ map[int64]float64) ([]StockShortfall, error) {
	var shortfalls []StockShortfall
	var breweryPtr *int64
	if breweryID > 0 {
		breweryPtr = &breweryID
	}
	for _, in := range ingredients {
		if in.Qty <= 0 {
			return nil, fmt.Errorf("ingredient qty must be positive")
		}
		unit := defaultUnit(in.Unit)
		var itemID int64
		var stockQty, costPrice float64
		var itemUnit, itemName, itemCategory string
		err := tx.QueryRow(
			`SELECT id, qty, cost_price, unit, name, category FROM inventory_items WHERE id = ?`,
			in.InventoryItemID,
		).Scan(&itemID, &stockQty, &costPrice, &itemUnit, &itemName, &itemCategory)
		if err != nil {
			return nil, err
		}
		need, err := ConvertQty(in.Qty, unit, defaultUnit(itemUnit))
		if err != nil {
			return nil, err
		}
		take := need
		if take > stockQty {
			take = stockQty
		}
		if take < 0 {
			take = 0
		}
		if take > 0 {
			_, err = tx.Exec(
				`UPDATE inventory_items SET qty = qty - ? WHERE id = ? AND qty >= ?`,
				take, itemID, take,
			)
			if err != nil {
				return nil, err
			}
			qtyAfter := stockQty - take
			summary := fmt.Sprintf(
				"recipe checkout −%s (qty: %s → %s)",
				formatLogQty(take), formatLogQty(stockQty), formatLogQty(qtyAfter),
			)
			if err := s.inventory.appendItemLog(tx, &itemID, itemName, actor, summary); err != nil {
				return nil, err
			}
		}
		_, err = tx.Exec(
			`INSERT INTO recipe_ingredients (recipe_id, inventory_item_id, qty, unit, checked_out, cost_price)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			recipeID, itemID, in.Qty, unit, take, costPrice,
		)
		if err != nil {
			return nil, err
		}
		missing := need - take
		if missing > 0.0000001 {
			shortfalls = append(shortfalls, StockShortfall{
				InventoryItemID: itemID,
				Name:            itemName,
				Category:        itemCategory,
				Requested:       need,
				Available:       take,
				Missing:         missing,
				BreweryID:       breweryPtr,
			})
		}
	}
	return shortfalls, nil
}

// Delete restores checked-out inventory and removes a recipe if status allows.
func (s *RecipeService) Delete(actor Actor, id int64) error {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return err
	}
	if recipe.Status != StatusCreated && recipe.Status != StatusScheduled {
		return ErrInvalidStatus
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	ings, err := s.loadIngredients(id)
	if err != nil {
		return err
	}
	for _, ing := range ings {
		if ing.CheckedOut <= 0 {
			continue
		}
		var qtyBefore float64
		err := tx.QueryRow(`SELECT qty FROM inventory_items WHERE id = ?`, ing.InventoryItemID).Scan(&qtyBefore)
		if err != nil {
			return err
		}
		_, err = tx.Exec(
			`UPDATE inventory_items SET qty = qty + ? WHERE id = ?`,
			ing.CheckedOut, ing.InventoryItemID,
		)
		if err != nil {
			return err
		}
		itemID := ing.InventoryItemID
		qtyAfter := qtyBefore + ing.CheckedOut
		summary := fmt.Sprintf(
			"recipe restore +%s (qty: %s → %s)",
			formatLogQty(ing.CheckedOut), formatLogQty(qtyBefore), formatLogQty(qtyAfter),
		)
		if err := s.inventory.appendItemLog(tx, &itemID, ing.ItemName, actor, summary); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM brewery_bookings WHERE recipe_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM tank_bookings WHERE recipe_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM recipes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SetActive hides or unhides a delivered recipe (soft-hide; no inventory changes).
func (s *RecipeService) SetActive(actor Actor, id int64, active bool) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusDelivered {
		return nil, ErrInvalidStatus
	}
	val := 0
	if active {
		val = 1
	}
	_, err = s.db.Exec(`UPDATE recipes SET active = ? WHERE id = ?`, val, id)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// SetBrewday records OG and brew volume and advances status to brewday.
func (s *RecipeService) SetBrewday(actor Actor, id int64, og, brewVolume float64) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusScheduled && recipe.Status != StatusBrewday {
		return nil, ErrInvalidStatus
	}
	if recipe.BookedDate == nil {
		return nil, ErrInvalidStatus
	}
	_, err = s.db.Exec(
		`UPDATE recipes SET og = ?, brew_volume = ?, status = ? WHERE id = ?`,
		og, brewVolume, StatusBrewday, id,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// RevokeBrewday undoes brewday recording: back to scheduled, clears OG and brew volume.
func (s *RecipeService) RevokeBrewday(actor Actor, id int64) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusBrewday {
		return nil, ErrInvalidStatus
	}
	_, err = s.db.Exec(
		`UPDATE recipes SET status = ?, og = NULL, brew_volume = NULL WHERE id = ?`,
		StatusScheduled, id,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// CompleteHygiene marks routines complete; when all done, advances to hygiene_done.
func (s *RecipeService) CompleteHygiene(actor Actor, recipeID, routineID int64) (*Recipe, error) {
	recipe, err := s.Get(actor, recipeID)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusBrewday && recipe.Status != StatusHygieneDone {
		return nil, ErrInvalidStatus
	}
	if recipe.OG == nil {
		return nil, ErrInvalidStatus
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO recipe_hygiene_checks (recipe_id, routine_id, completed_at) VALUES (?, ?, ?)
		 ON CONFLICT(recipe_id, routine_id) DO UPDATE SET completed_at = excluded.completed_at`,
		recipeID, routineID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("hygiene check: %w", err)
	}

	var total, done int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hygiene_routines`).Scan(&total); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM recipe_hygiene_checks WHERE recipe_id = ?`, recipeID,
	).Scan(&done); err != nil {
		return nil, err
	}
	if total > 0 && done >= total {
		_, err = s.db.Exec(`UPDATE recipes SET status = ? WHERE id = ?`, StatusHygieneDone, recipeID)
		if err != nil {
			return nil, err
		}
	}
	return s.Get(actor, recipeID)
}

// CompleteAllHygiene marks every hygiene routine complete and advances to hygiene_done.
func (s *RecipeService) CompleteAllHygiene(actor Actor, recipeID int64) (*Recipe, error) {
	recipe, err := s.Get(actor, recipeID)
	if err != nil {
		return nil, err
	}
	if recipe.Status == StatusHygieneDone {
		return recipe, nil
	}
	if recipe.Status != StatusBrewday {
		return nil, ErrInvalidStatus
	}
	if recipe.OG == nil {
		return nil, ErrInvalidStatus
	}
	routines, err := s.settings.ListHygieneRoutines()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, rt := range routines {
		_, err = tx.Exec(
			`INSERT INTO recipe_hygiene_checks (recipe_id, routine_id, completed_at) VALUES (?, ?, ?)
			 ON CONFLICT(recipe_id, routine_id) DO UPDATE SET completed_at = excluded.completed_at`,
			recipeID, rt.ID, now,
		)
		if err != nil {
			return nil, fmt.Errorf("hygiene check: %w", err)
		}
	}
	_, err = tx.Exec(`UPDATE recipes SET status = ? WHERE id = ?`, StatusHygieneDone, recipeID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(actor, recipeID)
}

// RevokeHygiene undoes hygiene completion: back to brewday and clears hygiene checks.
func (s *RecipeService) RevokeHygiene(actor Actor, recipeID int64) (*Recipe, error) {
	recipe, err := s.Get(actor, recipeID)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusHygieneDone {
		return nil, ErrInvalidStatus
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM recipe_hygiene_checks WHERE recipe_id = ?`, recipeID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE recipes SET status = ? WHERE id = ?`, StatusBrewday, recipeID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(actor, recipeID)
}

// HygieneStatus returns which routines are complete for a recipe.
func (s *RecipeService) HygieneStatus(actor Actor, recipeID int64) ([]HygieneRoutine, map[int64]bool, error) {
	recipe, err := s.Get(actor, recipeID)
	if err != nil {
		return nil, nil, err
	}
	_ = recipe
	routines, err := s.settings.ListHygieneRoutines()
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.Query(`SELECT routine_id FROM recipe_hygiene_checks WHERE recipe_id = ?`, recipeID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	done := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		done[id] = true
	}
	return routines, done, rows.Err()
}

// SetDelivery computes tax/cost/net from FG, delivery volume, beer net SEK/L, and multiplier.
// Net = beerNetSEKPerLiter × multiplier × volume. Tax is fixed alcohol tax (never multiplied, not included in net).
func (s *RecipeService) SetDelivery(actor Actor, id int64, fg, deliveryVolume, beerNetSEKPerLiter float64, multiplierID int64) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusHygieneDone && recipe.Status != StatusReadyForDelivery {
		return nil, ErrInvalidStatus
	}
	if recipe.OG == nil {
		return nil, fmt.Errorf("og required before delivery")
	}
	if beerNetSEKPerLiter < 0 {
		return nil, fmt.Errorf("beer net SEK per liter must be non-negative")
	}
	if deliveryVolume <= 0 {
		return nil, fmt.Errorf("delivery volume must be positive")
	}

	mult, err := s.settings.GetMultiplier(multiplierID)
	if err != nil {
		return nil, err
	}
	if !mult.Active {
		return nil, fmt.Errorf("multiplier is disabled")
	}

	abv := ABVFromSG(*recipe.OG, fg)
	sekPerLiter, err := s.settings.TaxForABV(abv)
	if err != nil {
		return nil, err
	}
	tax := deliveryVolume * sekPerLiter

	ings, err := s.loadIngredients(id)
	if err != nil {
		return nil, err
	}
	var cost float64
	for _, ing := range ings {
		cost += ing.Qty * ing.CostPrice
	}
	net := beerNetSEKPerLiter * mult.Multiplier * deliveryVolume

	_, err = s.db.Exec(
		`UPDATE recipes SET fg = ?, delivery_volume = ?, cost = ?, tax = ?, net = ?, status = ? WHERE id = ?`,
		fg, deliveryVolume, cost, tax, net, StatusReadyForDelivery, id,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// Deliver marks a ready recipe as delivered to the pub.
func (s *RecipeService) Deliver(actor Actor, id int64) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusReadyForDelivery {
		return nil, ErrInvalidStatus
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`UPDATE recipes SET status = ?, delivered_at = ? WHERE id = ?`,
		StatusDelivered, now, id,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// RevokeDelivery undoes a delivered batch: back to hygiene_done and clears delivery fields.
func (s *RecipeService) RevokeDelivery(actor Actor, id int64) (*Recipe, error) {
	recipe, err := s.Get(actor, id)
	if err != nil {
		return nil, err
	}
	if recipe.Status != StatusDelivered {
		return nil, ErrInvalidStatus
	}
	_, err = s.db.Exec(
		`UPDATE recipes SET status = ?, fg = NULL, delivery_volume = NULL, cost = NULL, tax = NULL, net = NULL, delivered_at = NULL, active = 1 WHERE id = ?`,
		StatusHygieneDone, id,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(actor, id)
}

// ABVFromSG estimates ABV from original and final gravity (standard formula).
func ABVFromSG(og, fg float64) float64 {
	return (og - fg) * 131.25
}

// IngredientCostSum returns total ingredient cost for a recipe (exported for tests).
func IngredientCostSum(ings []RecipeIngredient) float64 {
	var cost float64
	for _, ing := range ings {
		cost += ing.Qty * ing.CostPrice
	}
	return cost
}
