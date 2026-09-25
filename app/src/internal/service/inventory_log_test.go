package service_test

import (
	"fmt"
	"strings"
	"testing"

	"brewhouse/internal/service"
)

func TestInventoryItemLog_CreateUpdateAndList(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Log Malt", Unit: "kg", Qty: 10, CostPrice: 20,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated := *item
	updated.Qty = 15
	updated.CostPrice = 22
	if _, err := inventory.Update(admin, item.ID, updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	logs, hasMore, err := inventory.ListItemLogs(item.ID, 5, 0)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if hasMore {
		t.Fatalf("expected no more pages for 2 entries")
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 log rows, got %d", len(logs))
	}
	if logs[0].Username != "admin" || logs[0].Email != "admin@brewhouse.local" {
		t.Fatalf("expected admin actor on newest log, got %q / %q", logs[0].Username, logs[0].Email)
	}
	if !strings.Contains(logs[0].Summary, "qty: 10 → 15") {
		t.Fatalf("expected qty diff in summary, got %q", logs[0].Summary)
	}
	if !strings.Contains(logs[1].Summary, "created item") {
		t.Fatalf("expected create summary, got %q", logs[1].Summary)
	}
}

func TestInventoryItemLog_Cap100(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryHops, Name: "Cap Hop", Unit: "g", Qty: 0, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Create already logged 1; add 100 more updates → trim keeps 100 newest.
	for i := 1; i <= 100; i++ {
		next := *item
		next.Qty = float64(i)
		updated, err := inventory.Update(admin, item.ID, next)
		if err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		item = updated
	}

	all, hasMore, err := inventory.ListItemLogs(item.ID, 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if hasMore {
		t.Fatalf("expected at most 100 rows")
	}
	if len(all) != 100 {
		t.Fatalf("expected 100 retained logs, got %d", len(all))
	}
	if !strings.Contains(all[0].Summary, "qty:") || !strings.Contains(all[0].Summary, "→ 100") {
		t.Fatalf("expected newest log for qty 100, got %q", all[0].Summary)
	}
	for _, e := range all {
		if strings.Contains(e.Summary, "created item") {
			t.Fatalf("oldest create log should have been trimmed away")
		}
	}
}

func TestInventoryItemLog_Pagination(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryYeast, Name: "Page Yeast", Unit: "pack", Qty: 0, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 1; i <= 7; i++ {
		next := *item
		next.Qty = float64(i)
		updated, err := inventory.Update(admin, item.ID, next)
		if err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		item = updated
	}

	page1, hasMore, err := inventory.ListItemLogs(item.ID, 5, 0)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if !hasMore {
		t.Fatalf("expected has_more on first page")
	}
	if len(page1) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(page1))
	}

	page2, hasMore, err := inventory.ListItemLogs(item.ID, 5, 5)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if hasMore {
		t.Fatalf("expected no more after offset 5 (8 total)")
	}
	if len(page2) != 3 {
		t.Fatalf("expected 3 remaining rows, got %d", len(page2))
	}
}

func TestInventoryItemLog_RecipeCheckoutAndRestore(t *testing.T) {
	_, users, breweries, inventory, _, recipes, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "Log Brewery", "A", "a@t.com", "", "", nil)
	if err != nil {
		t.Fatalf("brewery: %v", err)
	}
	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Recipe Log Malt", Unit: "kg", Qty: 10, CostPrice: 5,
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}

	result, err := recipes.Create(admin, brewery.ID, "Log Batch", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 3, Unit: "kg"},
	})
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}

	logs, _, err := inventory.ListItemLogs(item.ID, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	foundCheckout := false
	for _, e := range logs {
		if strings.Contains(e.Summary, "recipe checkout") && strings.Contains(e.Summary, "10 → 7") {
			foundCheckout = true
			break
		}
	}
	if !foundCheckout {
		t.Fatalf("expected recipe checkout log, got %#v", summaries(logs))
	}

	if err := recipes.Delete(admin, result.Recipe.ID); err != nil {
		t.Fatalf("delete recipe: %v", err)
	}
	logs, _, err = inventory.ListItemLogs(item.ID, 10, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	foundRestore := false
	for _, e := range logs {
		if strings.Contains(e.Summary, "recipe restore") && strings.Contains(e.Summary, "7 → 10") {
			foundRestore = true
			break
		}
	}
	if !foundRestore {
		t.Fatalf("expected recipe restore log, got %#v", summaries(logs))
	}
}

func summaries(logs []service.InventoryLogEntry) []string {
	out := make([]string, len(logs))
	for i, e := range logs {
		out[i] = fmt.Sprintf("%s", e.Summary)
	}
	return out
}
