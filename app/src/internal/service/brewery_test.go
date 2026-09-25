package service_test

import (
	"errors"
	"testing"

	"brewhouse/internal/service"
)

func TestBrewery_UpdateFields(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	u, admin := ensureAdminUser(t, users)

	created, err := breweries.Create(admin, "Old Name", "Old Contact", "old@ex.com", "111", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := breweries.Update(admin, created.ID, "New Name", "New Contact", "new@ex.com", "222", &u.ID)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "New Name" || updated.ContactName != "New Contact" ||
		updated.ContactEmail != "new@ex.com" || updated.ContactPhone != "222" {
		t.Fatalf("unexpected brewery fields: %+v", updated)
	}
	members, err := breweries.ListMembers(admin, created.ID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	found := false
	for _, m := range members {
		if m.UserID == u.ID && m.Role == service.RoleBreweryAdmin {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected brewery_admin membership for user %d, got %+v", u.ID, members)
	}
}

func TestBrewery_DeleteBlockedWhenDelivered(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Delivered Brewery", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV-DEL", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Batch", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2031-01-01", TankID: tank.ID, TankDays: 5}); err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := recipes.SetBrewday(admin, id, 1.048, 100); err != nil {
		t.Fatalf("brewday: %v", err)
	}
	routines, err := settings.ListHygieneRoutines()
	if err != nil || len(routines) == 0 {
		t.Fatalf("routines: %v", err)
	}
	for _, rt := range routines {
		if _, err := recipes.CompleteHygiene(admin, id, rt.ID); err != nil {
			t.Fatalf("hygiene: %v", err)
		}
	}
	if _, err := recipes.SetDelivery(admin, id, 1.010, 90, 55, defaultMultID); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if _, err := recipes.Deliver(admin, id); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	err = breweries.Delete(admin, brewery.ID)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected ErrConflict when delivered batches exist, got %v", err)
	}
}

func TestBrewery_DeleteAllowedWithoutDelivered(t *testing.T) {
	_, users, breweries, inventory, _, recipes, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Empty Brewery", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale2", Unit: "kg", Qty: 20, CostPrice: 10})
	if _, err := recipes.Create(admin, brewery.ID, "Undelivered", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 2},
	}); err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	if err := breweries.Delete(admin, brewery.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := breweries.Get(admin, brewery.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}
