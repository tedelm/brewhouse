package service_test

import (
	"errors"
	"testing"

	"brewhouse/internal/service"
)

func TestEffectiveCost(t *testing.T) {
	cases := []struct {
		name   string
		base   float64
		adjust float64
		want   float64
	}{
		{"zero", 100, 0, 100},
		{"plus10", 100, 10, 110},
		{"minus5", 100, -5, 95},
		{"missing_supplier_like", 42.5, 0, 42.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.EffectiveCost(tc.base, tc.adjust)
			if got != tc.want {
				t.Fatalf("EffectiveCost(%v, %v) = %v, want %v", tc.base, tc.adjust, got, tc.want)
			}
		})
	}
}

func TestSupplier_CRUDAndInventoryEffectiveCost(t *testing.T) {
	_, users, _, inventory, settings, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	nonAdmin := service.Actor{UserID: 99, Role: service.RoleUser}

	if _, err := settings.CreateSupplier(nonAdmin, "x", 10); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden create, got %v", err)
	}

	sup, err := settings.CreateSupplier(admin, "Test Vendor", 10)
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	if !sup.Active || sup.AdjustPercent != 10 {
		t.Fatalf("unexpected supplier %+v", sup)
	}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category:   service.CategoryMalt,
		Name:       "Supplier Malt",
		Unit:       "kg",
		Qty:        10,
		CostPrice:  100,
		SupplierID: &sup.ID,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if item.EffectiveCostPrice != 110 {
		t.Fatalf("expected effective 110, got %v", item.EffectiveCostPrice)
	}
	if item.SupplierName != "Test Vendor" || item.AdjustPercent != 10 {
		t.Fatalf("expected supplier fields, got name=%q adj=%v", item.SupplierName, item.AdjustPercent)
	}

	updated, err := settings.UpdateSupplier(admin, sup.ID, "Test Vendor", 20)
	if err != nil {
		t.Fatalf("update supplier: %v", err)
	}
	if updated.AdjustPercent != 20 {
		t.Fatalf("expected adjust 20, got %v", updated.AdjustPercent)
	}
	got, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.EffectiveCostPrice != 120 {
		t.Fatalf("expected effective 120 after supplier change, got %v", got.EffectiveCostPrice)
	}
	if got.CostPrice != 100 {
		t.Fatalf("base cost should stay 100, got %v", got.CostPrice)
	}
}

func TestSupplier_SnapshotsUseEffectiveCost(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	sup, err := settings.CreateSupplier(admin, "Snap Vendor", 10)
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	item, err := inventory.Create(admin, service.InventoryItem{
		Category:   service.CategoryMalt,
		Name:       "Snap Malt",
		Unit:       "kg",
		Qty:        50,
		CostPrice:  100,
		SupplierID: &sup.ID,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	order, err := inventory.CreateOrder(admin, "snap order", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if len(order.Lines) != 1 || order.Lines[0].CostPrice != 110 {
		t.Fatalf("expected order line cost 110, got %+v", order.Lines)
	}

	brewery, err := breweries.Create(admin, "Snap Brewery", "A", "a@t.com", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	result, err := recipes.Create(admin, brewery.ID, "Snap Recipe", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 2, Unit: "kg"},
	})
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	if result.Recipe == nil || len(result.Recipe.Ingredients) != 1 {
		t.Fatal("expected recipe ingredient")
	}
	if result.Recipe.Ingredients[0].CostPrice != 110 {
		t.Fatalf("expected ingredient cost 110, got %v", result.Recipe.Ingredients[0].CostPrice)
	}

	if _, err := settings.UpdateSupplier(admin, sup.ID, "Snap Vendor", 50); err != nil {
		t.Fatalf("bump supplier percent: %v", err)
	}
	gotRecipe, err := recipes.Get(admin, result.Recipe.ID)
	if err != nil {
		t.Fatalf("get recipe: %v", err)
	}
	if gotRecipe.Ingredients[0].CostPrice != 110 {
		t.Fatalf("snapshot should stay 110 after supplier change, got %v", gotRecipe.Ingredients[0].CostPrice)
	}
	gotOrder, err := inventory.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if gotOrder.Lines[0].CostPrice != 110 {
		t.Fatalf("order line snapshot should stay 110, got %v", gotOrder.Lines[0].CostPrice)
	}
	live, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if live.EffectiveCostPrice != 150 {
		t.Fatalf("live effective should be 150, got %v", live.EffectiveCostPrice)
	}
}
