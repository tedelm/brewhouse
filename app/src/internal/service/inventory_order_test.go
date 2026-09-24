package service_test

import (
	"errors"
	"testing"

	"brewhouse/internal/service"
)

func TestUpdateOrderLineProductLink(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryHops, Name: "Cascade Link", Unit: "g", Qty: 100, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	order, err := inventory.CreateOrder(admin, "link test", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 50},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if len(order.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(order.Lines))
	}
	lineID := order.Lines[0].ID

	url := "https://example.com/cascade"
	updated, err := inventory.UpdateOrderLineProductLink(admin, order.ID, lineID, url)
	if err != nil {
		t.Fatalf("set link: %v", err)
	}
	if updated.Lines[0].Link != url {
		t.Fatalf("expected line link %q, got %q", url, updated.Lines[0].Link)
	}
	got, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Link != url {
		t.Fatalf("expected catalog link %q, got %q", url, got.Link)
	}

	cleared, err := inventory.UpdateOrderLineProductLink(admin, order.ID, lineID, "")
	if err != nil {
		t.Fatalf("clear link: %v", err)
	}
	if cleared.Lines[0].Link != "" {
		t.Fatalf("expected empty link, got %q", cleared.Lines[0].Link)
	}

	_, err = inventory.UpdateOrderLineProductLink(admin, order.ID, lineID+999, url)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing line, got %v", err)
	}
}

func TestOrderPauseBlocksAddLine(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryHops, Name: "Pause Hop", Unit: "g", Qty: 100, CostPrice: 1,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	order, err := inventory.CreateOrder(admin, "pause test", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 10},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	paused, err := inventory.SetOrderStatus(admin, order.ID, service.OrderStatusPaused)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	if paused.Status != service.OrderStatusPaused {
		t.Fatalf("expected paused, got %s", paused.Status)
	}

	_, err = inventory.AddOrderLine(admin, order.ID, item.ID, 5, nil)
	if err == nil {
		t.Fatal("expected add line on paused order to fail")
	}

	resumed, err := inventory.SetOrderStatus(admin, order.ID, service.OrderStatusPlanning)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Status != service.OrderStatusPlanning {
		t.Fatalf("expected planning, got %s", resumed.Status)
	}
	if _, err := inventory.AddOrderLine(admin, order.ID, item.ID, 5, nil); err != nil {
		t.Fatalf("add line after resume: %v", err)
	}
}

func TestOrderPauseRequiresAdmin(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	super := service.Actor{UserID: u.ID, Role: service.RoleSuperuser}

	order, err := inventory.CreateOrder(admin, "pause auth", "", nil)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_, err = inventory.SetOrderStatus(super, order.ID, service.OrderStatusPaused)
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden for non-admin pause, got %v", err)
	}
}

func TestDeleteOrderWithLines(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Delete Malt", Unit: "kg", Qty: 50, CostPrice: 2,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	order, err := inventory.CreateOrder(admin, "delete test", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 20},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if err := inventory.DeleteOrder(admin, order.ID); err != nil {
		t.Fatalf("delete with lines: %v", err)
	}
	_, err = inventory.GetOrder(order.ID)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestOrderedQtyZeroOnComplete(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryYeast, Name: "Zero Yeast", Unit: "pack", Qty: 10, CostPrice: 3,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	order, err := inventory.CreateOrder(admin, "zero qty", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	lineID := order.Lines[0].ID

	updated, err := inventory.UpdateOrderLineOrderedQty(admin, order.ID, lineID, 0)
	if err != nil {
		t.Fatalf("set ordered qty 0: %v", err)
	}
	if updated.Lines[0].OrderedQty == nil || *updated.Lines[0].OrderedQty != 0 {
		t.Fatalf("expected ordered_qty 0, got %v", updated.Lines[0].OrderedQty)
	}

	if _, err := inventory.SetOrderStatus(admin, order.ID, service.OrderStatusOrdered); err != nil {
		t.Fatalf("mark ordered: %v", err)
	}
	if _, err := inventory.SetOrderStatus(admin, order.ID, service.OrderStatusCompleted); err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Qty != 10 {
		t.Fatalf("expected stock unchanged at 10, got %v", got.Qty)
	}
}
