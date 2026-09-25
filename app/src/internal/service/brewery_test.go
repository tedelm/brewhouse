package service_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"brewhouse/internal/service"
)

func TestBrewery_UpdateFields(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	u, admin := ensureAdminUser(t, users)

	created, err := breweries.Create(admin, "Old Name", "Old Contact", "old@ex.com", "111", "https://instagram.com/old", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := breweries.Update(admin, created.ID, "New Name", "New Contact", "new@ex.com", "222", "https://instagram.com/new", &u.ID)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "New Name" || updated.ContactName != "New Contact" ||
		updated.ContactEmail != "new@ex.com" || updated.ContactPhone != "222" ||
		updated.Instagram != "https://instagram.com/new" {
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
	brewery, err := breweries.Create(admin, "Delivered Brewery", "", "", "", "", nil)
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
	brewery, err := breweries.Create(admin, "Empty Brewery", "", "", "", "", nil)
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

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestBrewery_Logo_BreweryAdminCanSetAndClear(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Logo Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	manager, err := users.Create("logoadmin", "pass", "logoadmin@test.local", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, manager.ID, service.RoleBreweryAdmin); err != nil {
		t.Fatalf("add brewery_admin: %v", err)
	}
	actor := service.Actor{UserID: manager.ID, Role: service.RoleUser}
	data := testPNG(t, 100, 100)

	if err := breweries.SetLogo(actor, brewery.ID, "image/png", data); err != nil {
		t.Fatalf("set logo: %v", err)
	}
	ok, err := breweries.LogoConfigured(brewery.ID)
	if err != nil || !ok {
		t.Fatalf("expected logo configured, ok=%v err=%v", ok, err)
	}
	got, b, ok, err := breweries.GetLogo(brewery.ID)
	if err != nil || !ok || got != "image/png" || len(b) == 0 {
		t.Fatalf("get logo: ct=%q ok=%v len=%d err=%v", got, ok, len(b), err)
	}
	listed, err := breweries.Get(actor, brewery.ID)
	if err != nil {
		t.Fatalf("get brewery: %v", err)
	}
	if !listed.CanManage || !listed.LogoConfigured {
		t.Fatalf("expected can_manage and logo_configured, got %+v", listed)
	}
	if err := breweries.ClearLogo(actor, brewery.ID); err != nil {
		t.Fatalf("clear logo: %v", err)
	}
	ok, err = breweries.LogoConfigured(brewery.ID)
	if err != nil || ok {
		t.Fatalf("expected logo cleared, ok=%v err=%v", ok, err)
	}
}

func TestBrewery_Logo_PlainMemberForbidden(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Member Logo Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	member, err := users.Create("logomember", "pass", "logomember@test.local", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, member.ID, service.RoleUser); err != nil {
		t.Fatalf("add member: %v", err)
	}
	actor := service.Actor{UserID: member.ID, Role: service.RoleUser}
	err = breweries.SetLogo(actor, brewery.ID, "image/png", testPNG(t, 50, 50))
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	err = breweries.ClearLogo(actor, brewery.ID)
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden on clear, got %v", err)
	}
}

func TestBrewery_Logo_RejectsOversizedDimensions(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Big Dim Logo Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	err = breweries.SetLogo(admin, brewery.ID, "image/png", testPNG(t, 301, 100))
	if err == nil {
		t.Fatal("expected dimension error")
	}
}

func TestBrewery_Logo_RejectsOversizedBytes(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Big Bytes Logo Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	// Valid tiny PNG header/format check runs after size; pad past 1 MB with junk that fails decode
	// unless we pass size check first. Size is checked before decode.
	big := make([]byte, (1<<20)+1)
	copy(big, testPNG(t, 10, 10))
	err = breweries.SetLogo(admin, brewery.ID, "image/png", big)
	if err == nil {
		t.Fatal("expected size error")
	}
}

func TestBrewery_Logo_CascadesOnDelete(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)
	brewery, err := breweries.Create(admin, "Cascade Logo Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	if err := breweries.SetLogo(admin, brewery.ID, "image/png", testPNG(t, 32, 32)); err != nil {
		t.Fatalf("set logo: %v", err)
	}
	if err := breweries.Delete(admin, brewery.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, _, ok, err := breweries.GetLogo(brewery.ID)
	if err != nil {
		t.Fatalf("get logo after delete: %v", err)
	}
	if ok {
		t.Fatal("expected logo gone after brewery delete")
	}
}

func TestBrewery_Instagram_ValidAndInvalid(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	created, err := breweries.Create(admin, "IG Brewery", "", "", "", "https://www.instagram.com/brewhouse", nil)
	if err != nil {
		t.Fatalf("create with instagram: %v", err)
	}
	if created.Instagram != "https://www.instagram.com/brewhouse" {
		t.Fatalf("instagram=%q", created.Instagram)
	}

	invalid := []string{"@handle", "instagram.com/x", "ftp://instagram.com/x", "not a url"}
	for _, ig := range invalid {
		if _, err := breweries.Update(admin, created.ID, "IG Brewery", "", "", "", ig, nil); err == nil {
			t.Fatalf("expected invalid instagram %q", ig)
		} else if !strings.Contains(err.Error(), "instagram") {
			t.Fatalf("expected instagram error for %q, got %v", ig, err)
		}
	}

	cleared, err := breweries.Update(admin, created.ID, "IG Brewery", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("clear instagram: %v", err)
	}
	if cleared.Instagram != "" {
		t.Fatalf("expected empty instagram, got %q", cleared.Instagram)
	}
}
