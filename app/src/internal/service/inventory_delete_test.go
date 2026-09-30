package service_test

import (
	"errors"
	"testing"

	"brewhouse/internal/service"
)

func TestInventoryDelete_AdminUnusedOK(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMisc, Name: "Unused Cleaner", Unit: "pcs", Qty: 0, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := inventory.Delete(admin, item.ID); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
	if _, err := inventory.Get(item.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestInventoryDelete_NonAdminForbidden(t *testing.T) {
	_, users, breweries, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "SU Brew", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	suUser, err := users.Create("invsu", "pass123!", "invsu@test.local", service.RoleSuperuser, service.UserContact{})
	if err != nil {
		t.Fatalf("create superuser: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, suUser.ID, service.RoleSuperuser); err != nil {
		t.Fatalf("add member: %v", err)
	}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryHops, Name: "No Delete Hop", Unit: "g", Qty: 10, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	super := service.Actor{UserID: suUser.ID, Role: service.RoleSuperuser}
	if err := inventory.Delete(super, item.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden for superuser, got %v", err)
	}
	plain := service.Actor{UserID: 99, Role: service.RoleUser}
	if err := inventory.Delete(plain, item.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden for user, got %v", err)
	}
}

func TestInventoryDelete_InUseOnRecipe(t *testing.T) {
	_, users, breweries, inventory, _, recipes, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "Recipe Brew", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Recipe Malt", Unit: "kg", Qty: 20, CostPrice: 5,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := recipes.Create(admin, brewery.ID, "Batch", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 2, Unit: "kg"},
	}); err != nil {
		t.Fatalf("create recipe: %v", err)
	}

	if err := inventory.Delete(admin, item.ID); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected conflict for recipe usage, got %v", err)
	}
}

func TestInventoryDelete_InUseOnOrder(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryYeast, Name: "Order Yeast", Unit: "pack", Qty: 5, CostPrice: 2,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if _, err := inventory.CreateOrder(admin, "wishlist", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 3},
	}); err != nil {
		t.Fatalf("create order: %v", err)
	}

	if err := inventory.Delete(admin, item.ID); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected conflict for order usage, got %v", err)
	}
}
