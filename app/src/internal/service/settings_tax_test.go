package service_test

import (
	"math"
	"testing"

	"brewhouse/internal/service"
)

func TestTaxForABV_SwedishBeerFormula(t *testing.T) {
	_, users, _, _, settings, _, _ := testDB(t)
	if err := users.EnsureDemoUser(); err != nil {
		t.Fatalf("demo: %v", err)
	}
	u, err := users.Authenticate("demo", "demo")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	admin := service.Actor{UserID: u.ID, Role: service.RoleAdmin}

	cfg, err := settings.GetAlcoholTaxConfig()
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.RateSEK != 2.28 || cfg.FreeMaxABV != 2.8 || cfg.Discount != 1.0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	sek, err := settings.TaxForABV(2.8)
	if err != nil {
		t.Fatalf("tax 2.8: %v", err)
	}
	if sek != 0 {
		t.Fatalf("expected 0 at 2.8%%, got %v", sek)
	}

	sek, err = settings.TaxForABV(5.0)
	if err != nil {
		t.Fatalf("tax 5.0: %v", err)
	}
	if math.Abs(sek-11.40) > 1e-9 {
		t.Fatalf("expected 11.40 at 5%% full rate, got %v", sek)
	}

	if _, err := settings.UpdateAlcoholTaxConfig(admin, 2.28, 2.8, 0.5); err != nil {
		t.Fatalf("update discount: %v", err)
	}
	sek, err = settings.TaxForABV(5.0)
	if err != nil {
		t.Fatalf("tax 5.0 discounted: %v", err)
	}
	if math.Abs(sek-5.70) > 1e-9 {
		t.Fatalf("expected 5.70 at 5%% with 50%% discount, got %v", sek)
	}
}
