package service

import (
	"database/sql"
	"errors"
	"fmt"
)

// SettingsService manages tanks, tax tiers, multipliers, and hygiene routines.
type SettingsService struct {
	db     *sql.DB
	access *AccessService
}

// NewSettingsService creates a SettingsService.
func NewSettingsService(db *sql.DB, access *AccessService) *SettingsService {
	return &SettingsService{db: db, access: access}
}

func (s *SettingsService) requireAdmin(actor Actor) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

// --- Fermentation tanks ---

// ListTanks returns all fermentation tanks.
func (s *SettingsService) ListTanks() ([]FermentationTank, error) {
	rows, err := s.db.Query(`SELECT id, name, capacity_liters FROM fermentation_tanks ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tanks: %w", err)
	}
	defer rows.Close()
	var out []FermentationTank
	for rows.Next() {
		var t FermentationTank
		if err := rows.Scan(&t.ID, &t.Name, &t.CapacityLiters); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTank inserts a fermentation tank.
func (s *SettingsService) CreateTank(actor Actor, name string, capacity float64) (*FermentationTank, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`INSERT INTO fermentation_tanks (name, capacity_liters) VALUES (?, ?)`, name, capacity)
	if err != nil {
		return nil, fmt.Errorf("insert tank: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetTank(id)
}

// GetTank returns a tank by id.
func (s *SettingsService) GetTank(id int64) (*FermentationTank, error) {
	t := &FermentationTank{}
	err := s.db.QueryRow(`SELECT id, name, capacity_liters FROM fermentation_tanks WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.CapacityLiters)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTank updates a tank.
func (s *SettingsService) UpdateTank(actor Actor, id int64, name string, capacity float64) (*FermentationTank, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	_, err := s.db.Exec(`UPDATE fermentation_tanks SET name = ?, capacity_liters = ? WHERE id = ?`, name, capacity, id)
	if err != nil {
		return nil, err
	}
	return s.GetTank(id)
}

// DeleteTank removes a tank.
func (s *SettingsService) DeleteTank(actor Actor, id int64) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM fermentation_tanks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Alcohol tax tiers ---

// ListTaxTiers returns tax tiers ordered by min ABV.
func (s *SettingsService) ListTaxTiers() ([]AlcoholTaxTier, error) {
	rows, err := s.db.Query(`SELECT id, min_abv, max_abv, sek_per_liter FROM alcohol_tax_tiers ORDER BY min_abv`)
	if err != nil {
		return nil, fmt.Errorf("list tax tiers: %w", err)
	}
	defer rows.Close()
	var out []AlcoholTaxTier
	for rows.Next() {
		var t AlcoholTaxTier
		if err := rows.Scan(&t.ID, &t.MinABV, &t.MaxABV, &t.SEKPerLiter); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTaxTier inserts a tax tier.
func (s *SettingsService) CreateTaxTier(actor Actor, minABV, maxABV, sekPerLiter float64) (*AlcoholTaxTier, error) {
	ok, err := s.access.CanManageEconomy(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	res, err := s.db.Exec(
		`INSERT INTO alcohol_tax_tiers (min_abv, max_abv, sek_per_liter) VALUES (?, ?, ?)`,
		minABV, maxABV, sekPerLiter,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getTaxTier(id)
}

func (s *SettingsService) getTaxTier(id int64) (*AlcoholTaxTier, error) {
	t := &AlcoholTaxTier{}
	err := s.db.QueryRow(`SELECT id, min_abv, max_abv, sek_per_liter FROM alcohol_tax_tiers WHERE id = ?`, id).
		Scan(&t.ID, &t.MinABV, &t.MaxABV, &t.SEKPerLiter)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTaxTier updates a tax tier.
func (s *SettingsService) UpdateTaxTier(actor Actor, id int64, minABV, maxABV, sekPerLiter float64) (*AlcoholTaxTier, error) {
	ok, err := s.access.CanManageEconomy(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	_, err = s.db.Exec(
		`UPDATE alcohol_tax_tiers SET min_abv = ?, max_abv = ?, sek_per_liter = ? WHERE id = ?`,
		minABV, maxABV, sekPerLiter, id,
	)
	if err != nil {
		return nil, err
	}
	return s.getTaxTier(id)
}

// DeleteTaxTier removes a tax tier.
func (s *SettingsService) DeleteTaxTier(actor Actor, id int64) error {
	ok, err := s.access.CanManageEconomy(actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	res, err := s.db.Exec(`DELETE FROM alcohol_tax_tiers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TaxForABV returns SEK/liter for the given ABV using Swedish beer formula.
func (s *SettingsService) TaxForABV(abv float64) (float64, error) {
	cfg, err := s.GetAlcoholTaxConfig()
	if err != nil {
		return 0, err
	}
	if abv <= cfg.FreeMaxABV {
		return 0, nil
	}
	return abv * cfg.RateSEK * cfg.Discount, nil
}

// GetAlcoholTaxConfig returns the single tax config row (defaults if missing).
func (s *SettingsService) GetAlcoholTaxConfig() (*AlcoholTaxConfig, error) {
	cfg := &AlcoholTaxConfig{}
	err := s.db.QueryRow(
		`SELECT rate_sek, free_max_abv, discount FROM alcohol_tax_config WHERE id = 1`,
	).Scan(&cfg.RateSEK, &cfg.FreeMaxABV, &cfg.Discount)
	if errors.Is(err, sql.ErrNoRows) {
		return &AlcoholTaxConfig{RateSEK: 2.28, FreeMaxABV: 2.8, Discount: 1.0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alcohol tax config: %w", err)
	}
	return cfg, nil
}

// UpdateAlcoholTaxConfig updates the Swedish beer tax formula parameters.
func (s *SettingsService) UpdateAlcoholTaxConfig(actor Actor, rateSEK, freeMaxABV, discount float64) (*AlcoholTaxConfig, error) {
	ok, err := s.access.CanManageEconomy(actor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if rateSEK < 0 || freeMaxABV < 0 {
		return nil, fmt.Errorf("rate and free max ABV must be non-negative")
	}
	if !validTaxDiscount(discount) {
		return nil, fmt.Errorf("discount must be 0.5, 0.6, 0.7, 0.8, 0.9, or 1.0")
	}
	_, err = s.db.Exec(
		`INSERT INTO alcohol_tax_config (id, rate_sek, free_max_abv, discount) VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET rate_sek = excluded.rate_sek, free_max_abv = excluded.free_max_abv, discount = excluded.discount`,
		rateSEK, freeMaxABV, discount,
	)
	if err != nil {
		return nil, err
	}
	return s.GetAlcoholTaxConfig()
}

func validTaxDiscount(d float64) bool {
	switch d {
	case 0.5, 0.6, 0.7, 0.8, 0.9, 1.0:
		return true
	default:
		return false
	}
}

// GetBeerPriceConfig returns min net SEK/L (defaults to 0 if missing).
func (s *SettingsService) GetBeerPriceConfig() (*BeerPriceConfig, error) {
	cfg := &BeerPriceConfig{}
	err := s.db.QueryRow(
		`SELECT min_net_sek_per_liter FROM beer_price_config WHERE id = 1`,
	).Scan(&cfg.MinNetSEKPerLiter)
	if errors.Is(err, sql.ErrNoRows) {
		return &BeerPriceConfig{MinNetSEKPerLiter: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get beer price config: %w", err)
	}
	return cfg, nil
}

// UpdateBeerPriceConfig updates the minimum net SEK per liter.
func (s *SettingsService) UpdateBeerPriceConfig(actor Actor, minNetSEKPerLiter float64) (*BeerPriceConfig, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	if minNetSEKPerLiter < 0 {
		return nil, fmt.Errorf("min net SEK per liter must be non-negative")
	}
	_, err := s.db.Exec(
		`INSERT INTO beer_price_config (id, min_net_sek_per_liter) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET min_net_sek_per_liter = excluded.min_net_sek_per_liter`,
		minNetSEKPerLiter,
	)
	if err != nil {
		return nil, err
	}
	return s.GetBeerPriceConfig()
}

// --- Price multipliers ---

// ListMultipliers returns price multipliers.
func (s *SettingsService) ListMultipliers() ([]PriceMultiplier, error) {
	rows, err := s.db.Query(`SELECT id, name, multiplier FROM price_multipliers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceMultiplier
	for rows.Next() {
		var m PriceMultiplier
		if err := rows.Scan(&m.ID, &m.Name, &m.Multiplier); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DefaultMultiplier returns the default multiplier value (falls back to 1.0).
func (s *SettingsService) DefaultMultiplier() (float64, error) {
	var m float64
	err := s.db.QueryRow(`SELECT multiplier FROM price_multipliers WHERE name = 'default'`).Scan(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return 1.0, nil
	}
	if err != nil {
		return 0, err
	}
	return m, nil
}

// CreateMultiplier inserts a multiplier.
func (s *SettingsService) CreateMultiplier(actor Actor, name string, multiplier float64) (*PriceMultiplier, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`INSERT INTO price_multipliers (name, multiplier) VALUES (?, ?)`, name, multiplier)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getMultiplier(id)
}

func (s *SettingsService) getMultiplier(id int64) (*PriceMultiplier, error) {
	m := &PriceMultiplier{}
	err := s.db.QueryRow(`SELECT id, name, multiplier FROM price_multipliers WHERE id = ?`, id).
		Scan(&m.ID, &m.Name, &m.Multiplier)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateMultiplier updates a multiplier.
func (s *SettingsService) UpdateMultiplier(actor Actor, id int64, name string, multiplier float64) (*PriceMultiplier, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	_, err := s.db.Exec(`UPDATE price_multipliers SET name = ?, multiplier = ? WHERE id = ?`, name, multiplier, id)
	if err != nil {
		return nil, err
	}
	return s.getMultiplier(id)
}

// DeleteMultiplier removes a multiplier.
func (s *SettingsService) DeleteMultiplier(actor Actor, id int64) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM price_multipliers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Hygiene routines ---

// ListHygieneRoutines returns hygiene routines.
func (s *SettingsService) ListHygieneRoutines() ([]HygieneRoutine, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, sort_order FROM hygiene_routines ORDER BY sort_order, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HygieneRoutine
	for rows.Next() {
		var r HygieneRoutine
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateHygieneRoutine inserts a routine.
func (s *SettingsService) CreateHygieneRoutine(actor Actor, name, description string, sortOrder int) (*HygieneRoutine, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO hygiene_routines (name, description, sort_order) VALUES (?, ?, ?)`,
		name, description, sortOrder,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getHygieneRoutine(id)
}

func (s *SettingsService) getHygieneRoutine(id int64) (*HygieneRoutine, error) {
	r := &HygieneRoutine{}
	err := s.db.QueryRow(
		`SELECT id, name, description, sort_order FROM hygiene_routines WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.Description, &r.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// UpdateHygieneRoutine updates a routine.
func (s *SettingsService) UpdateHygieneRoutine(actor Actor, id int64, name, description string, sortOrder int) (*HygieneRoutine, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	_, err := s.db.Exec(
		`UPDATE hygiene_routines SET name = ?, description = ?, sort_order = ? WHERE id = ?`,
		name, description, sortOrder, id,
	)
	if err != nil {
		return nil, err
	}
	return s.getHygieneRoutine(id)
}

// DeleteHygieneRoutine removes a routine.
func (s *SettingsService) DeleteHygieneRoutine(actor Actor, id int64) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM hygiene_routines WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
