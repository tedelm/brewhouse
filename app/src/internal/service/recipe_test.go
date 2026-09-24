package service_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"brewhouse/internal/database"
	"brewhouse/internal/service"
)

func testDB(t *testing.T) (*service.AccessService, *service.UserService, *service.BreweryService, *service.InventoryService, *service.SettingsService, *service.RecipeService, *service.ScheduleService) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	access := service.NewAccessService(db)
	users := service.NewUserService(db)
	breweries := service.NewBreweryService(db, access)
	inventory := service.NewInventoryService(db, access)
	settings := service.NewSettingsService(db, access)
	recipes := service.NewRecipeService(db, access, inventory, settings)
	schedule := service.NewScheduleService(db, access)
	return access, users, breweries, inventory, settings, recipes, schedule
}

func TestRecipe_CreateCheckoutAndDeleteRestoresStock(t *testing.T) {
	_, users, breweries, inventory, _, recipes, _ := testDB(t)
	admin := service.Actor{UserID: 1, Role: service.RoleAdmin}
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo user: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin.UserID = u.ID

	brewery, err := breweries.Create(admin, "Test Brewery", "A", "a@t.com", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	member, err := users.Create("brewer", "pass", "brewer@test.local", service.RoleUser)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, member.ID, service.RoleUser); err != nil {
		t.Fatalf("add member: %v", err)
	}

	item, err := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pilsner", Unit: "kg", Qty: 10, CostPrice: 25})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	actor := service.Actor{UserID: member.ID, Role: ""}
	result, err := recipes.Create(actor, brewery.ID, "IPA Batch", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 4, Unit: "kg"},
	})
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	if result.Recipe == nil {
		t.Fatalf("expected recipe")
	}

	updated, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if updated.Qty != 6 {
		t.Fatalf("expected qty 6 after checkout, got %v", updated.Qty)
	}

	if err := recipes.Delete(actor, result.Recipe.ID); err != nil {
		t.Fatalf("delete recipe: %v", err)
	}
	restored, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if restored.Qty != 10 {
		t.Fatalf("expected qty 10 after restore, got %v", restored.Qty)
	}
}

func TestRecipe_PartialCheckoutAddsPlanningOrder(t *testing.T) {
	_, users, breweries, inventory, _, recipes, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B2", "", "", "", nil)
	item, err := inventory.Create(admin, service.InventoryItem{Category: service.CategoryHops, Name: "Test Cascade Shortfall", Unit: "g", Qty: 50, CostPrice: 1})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	result, err := recipes.Create(admin, brewery.ID, "Hop Bomb", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 100, Unit: "g"},
	})
	if err != nil {
		t.Fatalf("create recipe: %v", err)
	}
	if result.Recipe == nil {
		t.Fatal("expected recipe created")
	}
	if len(result.Shortfalls) != 1 || result.Shortfalls[0].Missing != 50 {
		t.Fatalf("expected shortfall missing 50, got %+v", result.Shortfalls)
	}
	if result.OrderID == 0 {
		t.Fatal("expected planning order id")
	}
	stock, err := inventory.Get(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if stock.Qty != 0 {
		t.Fatalf("expected stock 0 after partial checkout, got %v", stock.Qty)
	}
	order, err := inventory.GetOrder(result.OrderID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if order.Status != service.OrderStatusPlanning {
		t.Fatalf("expected planning, got %s", order.Status)
	}
	if len(order.Lines) != 1 || order.Lines[0].Qty != 50 {
		t.Fatalf("expected order line qty 50, got %+v", order.Lines)
	}
}

func TestABVAndPricing(t *testing.T) {
	abv := service.ABVFromSG(1.050, 1.010)
	if abv < 5.2 || abv > 5.3 {
		t.Fatalf("unexpected abv %v", abv)
	}
	ings := []service.RecipeIngredient{
		{Qty: 2, CostPrice: 10},
		{Qty: 1, CostPrice: 5},
	}
	if got := service.IngredientCostSum(ings); got != 25 {
		t.Fatalf("expected cost 25, got %v", got)
	}
}

func TestSchedule_BookingConflict(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B3", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryYeast, Name: "US-05", Unit: "pack", Qty: 5, CostPrice: 30})
	tank, err := settings.CreateTank(admin, "FV1", 1000)
	if err != nil {
		t.Fatalf("tank: %v", err)
	}

	r1, err := recipes.Create(admin, brewery.ID, "A", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := recipes.Create(admin, brewery.ID, "B", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r2: %v", err)
	}

	err = schedule.Book(admin, service.BookRequest{
		RecipeID: r1.Recipe.ID, Date: "2030-01-15", TankID: tank.ID, TankDays: 7,
	})
	if err != nil {
		t.Fatalf("book1: %v", err)
	}
	err = schedule.Book(admin, service.BookRequest{
		RecipeID: r2.Recipe.ID, Date: "2030-01-15", TankID: tank.ID, TankDays: 7,
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"brew day", "2030-01-15", "B3", "A"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
	}
}

func TestSchedule_TankConflictDetails(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "Conflict Brewery", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryYeast, Name: "US-05", Unit: "pack", Qty: 5, CostPrice: 30})
	tank, err := settings.CreateTank(admin, "FVConflict", 1000)
	if err != nil {
		t.Fatalf("tank: %v", err)
	}
	freeTank, err := settings.CreateTank(admin, "FVFree", 1000)
	if err != nil {
		t.Fatalf("free tank: %v", err)
	}
	_ = freeTank

	r1, err := recipes.Create(admin, brewery.ID, "Occupying Batch", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := recipes.Create(admin, brewery.ID, "Challenger", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r2: %v", err)
	}

	// Tank occupied 2030-07-01 .. 2030-07-14
	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: r1.Recipe.ID, Date: "2030-07-01", TankID: tank.ID, TankDays: 14,
	}); err != nil {
		t.Fatalf("book1: %v", err)
	}
	// Different brewery day but overlapping tank window
	err = schedule.Book(admin, service.BookRequest{
		RecipeID: r2.Recipe.ID, Date: "2030-07-05", TankID: tank.ID, TankDays: 7,
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"FVConflict",
		"Conflict Brewery",
		"Occupying Batch",
		"2030-07-01",
		"2030-07-14",
		"Available fermenters: FVFree",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
	}
}

func TestSchedule_TankConflictNoAlternatives(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "Solo Brewery", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryYeast, Name: "US-05", Unit: "pack", Qty: 5, CostPrice: 30})
	tank, err := settings.CreateTank(admin, "OnlyFV", 1000)
	if err != nil {
		t.Fatalf("tank: %v", err)
	}

	r1, err := recipes.Create(admin, brewery.ID, "First", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := recipes.Create(admin, brewery.ID, "Second", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("r2: %v", err)
	}

	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: r1.Recipe.ID, Date: "2030-08-01", TankID: tank.ID, TankDays: 14,
	}); err != nil {
		t.Fatalf("book1: %v", err)
	}
	err = schedule.Book(admin, service.BookRequest{
		RecipeID: r2.Recipe.ID, Date: "2030-08-05", TankID: tank.ID, TankDays: 7,
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "No other fermenters are free for that period.") {
		t.Fatalf("expected no-alternatives message, got %q", msg)
	}
}

func TestSchedule_Unbook(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "BUnbook", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryYeast, Name: "US-05", Unit: "pack", Qty: 5, CostPrice: 30})
	tank, err := settings.CreateTank(admin, "FVUnbook", 1000)
	if err != nil {
		t.Fatalf("tank: %v", err)
	}

	r1, err := recipes.Create(admin, brewery.ID, "Unbook Me", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := r1.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: id, Date: "2030-06-01", TankID: tank.ID, TankDays: 7,
	}); err != nil {
		t.Fatalf("book: %v", err)
	}

	if err := schedule.Unbook(admin, id); err != nil {
		t.Fatalf("unbook: %v", err)
	}
	got, err := recipes.Get(admin, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != service.StatusCreated {
		t.Fatalf("expected created, got %s", got.Status)
	}
	if got.BookedDate != nil || got.TankID != nil {
		t.Fatalf("expected cleared booking fields, got date=%v tank=%v", got.BookedDate, got.TankID)
	}

	r2, err := recipes.Create(admin, brewery.ID, "Take Slot", []service.IngredientInput{{InventoryItemID: item.ID, Qty: 1}})
	if err != nil {
		t.Fatalf("create r2: %v", err)
	}
	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: r2.Recipe.ID, Date: "2030-06-01", TankID: tank.ID, TankDays: 7,
	}); err != nil {
		t.Fatalf("rebook freed date: %v", err)
	}

	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: id, Date: "2030-06-10", TankID: tank.ID, TankDays: 7,
	}); err != nil {
		t.Fatalf("rebook first recipe: %v", err)
	}
	if _, err := recipes.SetBrewday(admin, id, 1.050, 100); err != nil {
		t.Fatalf("brewday: %v", err)
	}
	if err := schedule.Unbook(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus after brewday, got %v", err)
	}
}

func TestPipeline_GatesAndRevokeHygiene(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "BPipe", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FVPipe", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Pipeline", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID

	if _, err := recipes.SetBrewday(admin, id, 1.050, 100); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("SetBrewday on created: expected ErrInvalidStatus, got %v", err)
	}
	if _, err := recipes.CompleteAllHygiene(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("CompleteAllHygiene on created: expected ErrInvalidStatus, got %v", err)
	}

	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: id, Date: "2030-08-01", TankID: tank.ID, TankDays: 7,
	}); err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := recipes.CompleteAllHygiene(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("CompleteAllHygiene before SetBrewday: expected ErrInvalidStatus, got %v", err)
	}

	if _, err := recipes.SetBrewday(admin, id, 1.050, 100); err != nil {
		t.Fatalf("SetBrewday: %v", err)
	}
	if _, err := recipes.CompleteAllHygiene(admin, id); err != nil {
		t.Fatalf("CompleteAllHygiene: %v", err)
	}
	got, err := recipes.Get(admin, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != service.StatusHygieneDone {
		t.Fatalf("expected hygiene_done, got %s", got.Status)
	}

	revoked, err := recipes.RevokeHygiene(admin, id)
	if err != nil {
		t.Fatalf("RevokeHygiene: %v", err)
	}
	if revoked.Status != service.StatusBrewday {
		t.Fatalf("expected brewday after revoke, got %s", revoked.Status)
	}
	_, doneMap, err := recipes.HygieneStatus(admin, id)
	if err != nil {
		t.Fatalf("HygieneStatus: %v", err)
	}
	for _, v := range doneMap {
		if v {
			t.Fatal("expected no hygiene checks after revoke")
		}
	}

	if _, err := recipes.CompleteAllHygiene(admin, id); err != nil {
		t.Fatalf("CompleteAllHygiene again: %v", err)
	}
	if _, err := recipes.SetDelivery(admin, id, 1.010, 90, 0, defaultMultID); err != nil {
		t.Fatalf("SetDelivery: %v", err)
	}
	if _, err := recipes.RevokeHygiene(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("RevokeHygiene after ready_for_delivery: expected ErrInvalidStatus, got %v", err)
	}
}

func TestRevokeBrewday(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "BRevBrew", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FVRevBrew", 500)

	created, err := recipes.Create(admin, brewery.ID, "Revoke Brewday", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{
		RecipeID: id, Date: "2030-09-01", TankID: tank.ID, TankDays: 7,
	}); err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := recipes.SetBrewday(admin, id, 1.052, 110); err != nil {
		t.Fatalf("SetBrewday: %v", err)
	}

	revoked, err := recipes.RevokeBrewday(admin, id)
	if err != nil {
		t.Fatalf("RevokeBrewday: %v", err)
	}
	if revoked.Status != service.StatusScheduled {
		t.Fatalf("expected scheduled, got %s", revoked.Status)
	}
	if revoked.OG != nil || revoked.BrewVolume != nil {
		t.Fatalf("expected OG/volume cleared, got og=%v vol=%v", revoked.OG, revoked.BrewVolume)
	}
	if revoked.BookedDate == nil {
		t.Fatal("expected booking date kept")
	}

	if _, err := recipes.SetBrewday(admin, id, 1.050, 100); err != nil {
		t.Fatalf("SetBrewday again: %v", err)
	}
	if _, err := recipes.CompleteAllHygiene(admin, id); err != nil {
		t.Fatalf("CompleteAllHygiene: %v", err)
	}
	if _, err := recipes.RevokeBrewday(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("RevokeBrewday on hygiene_done: expected ErrInvalidStatus, got %v", err)
	}
}

func multiplierIDByName(t *testing.T, settings *service.SettingsService, name string) int64 {
	t.Helper()
	list, err := settings.ListMultipliers()
	if err != nil {
		t.Fatalf("list multipliers: %v", err)
	}
	for _, m := range list {
		if m.Name == name {
			return m.ID
		}
	}
	t.Fatalf("multiplier %q not found", name)
	return 0
}

func TestDelivery_ComputesTaxCostNet(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B4", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV2", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Lager", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2030-02-01", TankID: tank.ID, TankDays: 10}); err != nil {
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
			t.Fatalf("hygiene %d: %v", rt.ID, err)
		}
	}
	recipe, err := recipes.SetDelivery(admin, id, 1.010, 90, 0, defaultMultID)
	if err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if recipe.Status != service.StatusReadyForDelivery {
		t.Fatalf("status %s", recipe.Status)
	}
	if recipe.Cost == nil || *recipe.Cost != 50 {
		t.Fatalf("expected cost 50, got %v", recipe.Cost)
	}
	if recipe.Tax == nil || recipe.Net == nil {
		t.Fatalf("expected tax and net")
	}
	delivered, err := recipes.Deliver(admin, id)
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if delivered.Status != service.StatusDelivered {
		t.Fatalf("expected delivered, got %s", delivered.Status)
	}
}

func TestDelivered_HideAndList(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "BHide", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FVHide", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Hide Me", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if _, err := recipes.SetActive(admin, id, false); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus before deliver, got %v", err)
	}

	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2030-05-01", TankID: tank.ID, TankDays: 10}); err != nil {
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
			t.Fatalf("hygiene %d: %v", rt.ID, err)
		}
	}
	if _, err := recipes.SetDelivery(admin, id, 1.010, 90, 0, defaultMultID); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if _, err := recipes.Deliver(admin, id); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if err := recipes.Delete(admin, id); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus on delete, got %v", err)
	}

	hidden, err := recipes.SetActive(admin, id, false)
	if err != nil {
		t.Fatalf("hide: %v", err)
	}
	if hidden.Active {
		t.Fatal("expected active false after hide")
	}

	visible, err := recipes.List(admin, brewery.ID, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range visible {
		if r.ID == id {
			t.Fatal("hidden recipe should be omitted from default list")
		}
	}

	all, err := recipes.List(admin, brewery.ID, true)
	if err != nil {
		t.Fatalf("list include hidden: %v", err)
	}
	found := false
	for _, r := range all {
		if r.ID == id {
			found = true
			if r.Active {
				t.Fatal("expected inactive in include_hidden list")
			}
		}
	}
	if !found {
		t.Fatal("expected hidden recipe in include_hidden list")
	}

	shown, err := recipes.SetActive(admin, id, true)
	if err != nil {
		t.Fatalf("unhide: %v", err)
	}
	if !shown.Active {
		t.Fatal("expected active true after unhide")
	}
}

func TestDelivery_Revoke(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B6", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV4", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Revoke Lager", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2030-04-01", TankID: tank.ID, TankDays: 10}); err != nil {
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
			t.Fatalf("hygiene %d: %v", rt.ID, err)
		}
	}
	if _, err := recipes.SetDelivery(admin, id, 1.010, 90, 0, defaultMultID); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if _, err := recipes.Deliver(admin, id); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	revoked, err := recipes.RevokeDelivery(admin, id)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Status != service.StatusHygieneDone {
		t.Fatalf("status %s", revoked.Status)
	}
	if revoked.FG != nil || revoked.DeliveryVolume != nil || revoked.Tax != nil || revoked.Net != nil || revoked.Cost != nil || revoked.DeliveredAt != nil {
		t.Fatalf("expected delivery fields cleared, got fg=%v vol=%v cost=%v tax=%v net=%v delivered_at=%v",
			revoked.FG, revoked.DeliveryVolume, revoked.Cost, revoked.Tax, revoked.Net, revoked.DeliveredAt)
	}
}

func TestDelivery_NetIsBeerNetTimesMult(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B5", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV3", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")

	created, err := recipes.Create(admin, brewery.ID, "Floor Lager", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2030-03-01", TankID: tank.ID, TankDays: 10}); err != nil {
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
			t.Fatalf("hygiene %d: %v", rt.ID, err)
		}
	}
	recipe, err := recipes.SetDelivery(admin, id, 1.010, 100, 45, defaultMultID)
	if err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if recipe.Cost == nil || recipe.Tax == nil || recipe.Net == nil {
		t.Fatalf("expected cost/tax/net, got cost=%v tax=%v net=%v", recipe.Cost, recipe.Tax, recipe.Net)
	}
	want := 45.0 * 1.0 * 100 // beer_net × default mult × volume
	if *recipe.Net != want {
		t.Fatalf("expected net=%v (beer_net×mult×vol), got %v", want, *recipe.Net)
	}
	if *recipe.Tax == 0 {
		t.Fatal("expected nonzero tax stored separately from net")
	}
}

func TestDelivery_HigherMultiplierRaisesNet(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	brewery, _ := breweries.Create(admin, "B7", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 40, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV5", 500)
	defaultMultID := multiplierIDByName(t, settings, "default")
	highMultID := multiplierIDByName(t, settings, "3.00")

	created, err := recipes.Create(admin, brewery.ID, "Mult Lager", []service.IngredientInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.Recipe.ID
	if err := schedule.Book(admin, service.BookRequest{RecipeID: id, Date: "2030-05-01", TankID: tank.ID, TankDays: 10}); err != nil {
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
			t.Fatalf("hygiene %d: %v", rt.ID, err)
		}
	}
	base, err := recipes.SetDelivery(admin, id, 1.010, 100, 55, defaultMultID)
	if err != nil {
		t.Fatalf("delivery default: %v", err)
	}
	raised, err := recipes.SetDelivery(admin, id, 1.010, 100, 55, highMultID)
	if err != nil {
		t.Fatalf("delivery 3.00: %v", err)
	}
	if base.Net == nil || raised.Net == nil {
		t.Fatal("expected nets")
	}
	if *base.Net != 55*1.0*100 {
		t.Fatalf("expected default net 5500, got %v", *base.Net)
	}
	if *raised.Net != 55*3.0*100 {
		t.Fatalf("expected 3.00 net 16500, got %v", *raised.Net)
	}
	if *raised.Net <= *base.Net {
		t.Fatalf("expected 3.00 net > default net, got default=%v high=%v", *base.Net, *raised.Net)
	}
}
