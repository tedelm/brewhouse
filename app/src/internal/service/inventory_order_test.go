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
