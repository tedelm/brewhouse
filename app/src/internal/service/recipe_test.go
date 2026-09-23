package service_test

import (
	"path/filepath"
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
	if err != service.ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
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
	recipe, err := recipes.SetDelivery(admin, id, 1.010, 90)
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
	if _, err := recipes.SetDelivery(admin, id, 1.010, 90); err != nil {
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

func TestDelivery_MinNetFloor(t *testing.T) {
	_, users, breweries, inventory, settings, recipes, schedule := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, _ := users.Authenticate("demo", "demo")
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}
	if _, err := settings.UpdateBeerPriceConfig(admin, 45); err != nil {
		t.Fatalf("beer price: %v", err)
	}
	brewery, _ := breweries.Create(admin, "B5", "", "", "", nil)
	item, _ := inventory.Create(admin, service.InventoryItem{Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 20, CostPrice: 10})
	tank, _ := settings.CreateTank(admin, "FV3", 500)

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
	recipe, err := recipes.SetDelivery(admin, id, 1.010, 100)
	if err != nil {
		t.Fatalf("delivery: %v", err)
	}
	if recipe.Net == nil || *recipe.Net < 4500 {
		t.Fatalf("expected net >= 4500 (45×100), got %v", recipe.Net)
	}
}
