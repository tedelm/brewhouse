package service

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
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

func scanTank(scanner interface{ Scan(dest ...any) error }) (*FermentationTank, error) {
	t := &FermentationTank{}
	var active int
	if err := scanner.Scan(&t.ID, &t.Name, &t.CapacityLiters, &active); err != nil {
		return nil, err
	}
	t.Active = active != 0
	return t, nil
}

// ListTanks returns all fermentation tanks.
func (s *SettingsService) ListTanks() ([]FermentationTank, error) {
	return s.listTanks(false)
}

// ListActiveTanks returns tanks with active = 1.
func (s *SettingsService) ListActiveTanks() ([]FermentationTank, error) {
	return s.listTanks(true)
}

func (s *SettingsService) listTanks(activeOnly bool) ([]FermentationTank, error) {
	q := `SELECT id, name, capacity_liters, active FROM fermentation_tanks`
	if activeOnly {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("list tanks: %w", err)
	}
	defer rows.Close()
	var out []FermentationTank
	for rows.Next() {
		t, err := scanTank(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// CreateTank inserts a fermentation tank.
func (s *SettingsService) CreateTank(actor Actor, name string, capacity float64) (*FermentationTank, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO fermentation_tanks (name, capacity_liters, active) VALUES (?, ?, 1)`,
		name, capacity,
	)
	if err != nil {
		return nil, fmt.Errorf("insert tank: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetTank(id)
}

// GetTank returns a tank by id.
func (s *SettingsService) GetTank(id int64) (*FermentationTank, error) {
	t, err := scanTank(s.db.QueryRow(
		`SELECT id, name, capacity_liters, active FROM fermentation_tanks WHERE id = ?`, id,
	))
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

// SetTankActive enables or disables a tank.
func (s *SettingsService) SetTankActive(actor Actor, id int64, active bool) (*FermentationTank, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	val := 0
	if active {
		val = 1
	}
	res, err := s.db.Exec(`UPDATE fermentation_tanks SET active = ? WHERE id = ?`, val, id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTank(id)
}

// DeleteTank removes a tank.
func (s *SettingsService) DeleteTank(actor Actor, id int64) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	var nBookings int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tank_bookings WHERE tank_id = ?`, id).Scan(&nBookings); err != nil {
		return fmt.Errorf("count tank bookings: %w", err)
	}
	if nBookings > 0 {
		return fmt.Errorf("%w: tank has historical bookings; disable it instead", ErrConflict)
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

func scanMultiplier(scanner interface{ Scan(dest ...any) error }) (*PriceMultiplier, error) {
	m := &PriceMultiplier{}
	var active int
	if err := scanner.Scan(&m.ID, &m.Name, &m.Multiplier, &active); err != nil {
		return nil, err
	}
	m.Active = active != 0
	return m, nil
}

// ListMultipliers returns all price multipliers.
func (s *SettingsService) ListMultipliers() ([]PriceMultiplier, error) {
	return s.listMultipliers(false)
}

// ListActiveMultipliers returns multipliers with active = 1.
func (s *SettingsService) ListActiveMultipliers() ([]PriceMultiplier, error) {
	return s.listMultipliers(true)
}

func (s *SettingsService) listMultipliers(activeOnly bool) ([]PriceMultiplier, error) {
	q := `SELECT id, name, multiplier, active FROM price_multipliers`
	if activeOnly {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceMultiplier
	for rows.Next() {
		m, err := scanMultiplier(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
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
	res, err := s.db.Exec(
		`INSERT INTO price_multipliers (name, multiplier, active) VALUES (?, ?, 1)`,
		name, multiplier,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getMultiplier(id)
}

func (s *SettingsService) getMultiplier(id int64) (*PriceMultiplier, error) {
	m, err := scanMultiplier(s.db.QueryRow(
		`SELECT id, name, multiplier, active FROM price_multipliers WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetMultiplier returns a price multiplier by id.
func (s *SettingsService) GetMultiplier(id int64) (*PriceMultiplier, error) {
	return s.getMultiplier(id)
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

// SetMultiplierActive enables or disables a multiplier.
func (s *SettingsService) SetMultiplierActive(actor Actor, id int64, active bool) (*PriceMultiplier, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	val := 0
	if active {
		val = 1
	}
	res, err := s.db.Exec(`UPDATE price_multipliers SET active = ? WHERE id = ?`, val, id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
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

// --- Brand assets (logo / favicon) ---

const (
	maxBrandImageBytes = 500 * 1024
	maxLogoWidth       = 300
	maxLogoHeight      = 300
	maxFaviconWidth    = 16
	maxFaviconHeight   = 16
)

// GetLogo returns the stored logo bytes when configured.
func (s *SettingsService) GetLogo() (contentType string, data []byte, ok bool, err error) {
	return s.getBrandImage("app_logo")
}

// SetLogo stores a custom logo (admin only).
func (s *SettingsService) SetLogo(actor Actor, contentType string, data []byte) error {
	return s.setBrandImage(actor, "app_logo", contentType, data, maxLogoWidth, maxLogoHeight)
}

// ClearLogo removes the custom logo (admin only).
func (s *SettingsService) ClearLogo(actor Actor) error {
	return s.clearBrandImage(actor, "app_logo")
}

// LogoConfigured reports whether a custom logo is stored.
func (s *SettingsService) LogoConfigured() (bool, error) {
	return s.brandImageConfigured("app_logo")
}

// GetFavicon returns the stored favicon bytes when configured.
func (s *SettingsService) GetFavicon() (contentType string, data []byte, ok bool, err error) {
	return s.getBrandImage("app_favicon")
}

// SetFavicon stores a custom favicon (admin only).
func (s *SettingsService) SetFavicon(actor Actor, contentType string, data []byte) error {
	return s.setBrandImage(actor, "app_favicon", contentType, data, maxFaviconWidth, maxFaviconHeight)
}

// ClearFavicon removes the custom favicon (admin only).
func (s *SettingsService) ClearFavicon(actor Actor) error {
	return s.clearBrandImage(actor, "app_favicon")
}

// FaviconConfigured reports whether a custom favicon is stored.
func (s *SettingsService) FaviconConfigured() (bool, error) {
	return s.brandImageConfigured("app_favicon")
}

func (s *SettingsService) getBrandImage(table string) (contentType string, data []byte, ok bool, err error) {
	err = s.db.QueryRow(`SELECT content_type, data FROM `+table+` WHERE id = 1`).Scan(&contentType, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("get %s: %w", table, err)
	}
	return contentType, data, true, nil
}

func (s *SettingsService) brandImageConfigured(table string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE id = 1`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("count %s: %w", table, err)
	}
	return n > 0, nil
}

func (s *SettingsService) setBrandImage(actor Actor, table, contentType string, data []byte, maxW, maxH int) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	if err := validateBrandImage(contentType, data, maxW, maxH); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO `+table+` (id, content_type, data) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET content_type = excluded.content_type, data = excluded.data`,
		contentType, data,
	)
	if err != nil {
		return fmt.Errorf("set %s: %w", table, err)
	}
	return nil
}

func (s *SettingsService) clearBrandImage(actor Actor, table string) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM ` + table + ` WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("clear %s: %w", table, err)
	}
	return nil
}

func validateBrandImage(contentType string, data []byte, maxW, maxH int) error {
	if len(data) == 0 {
		return fmt.Errorf("image is empty")
	}
	if len(data) > maxBrandImageBytes {
		return fmt.Errorf("image must be at most 500 KB")
	}
	switch contentType {
	case "image/png", "image/jpeg", "image/gif":
	default:
		return fmt.Errorf("image must be PNG, JPEG, or GIF")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}
	switch format {
	case "png", "jpeg", "gif":
	default:
		return fmt.Errorf("image must be PNG, JPEG, or GIF")
	}
	expected := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}
	if expected[format] != contentType {
		return fmt.Errorf("content type does not match image data")
	}
	if cfg.Width > maxW || cfg.Height > maxH {
		return fmt.Errorf("image must be at most %dx%d px", maxW, maxH)
	}
	return nil
}

const defaultLogoBgHex = "#6c704a"

// GetLogoBgColor returns the welcome logo backdrop hex color.
func (s *SettingsService) GetLogoBgColor() (string, error) {
	var hex string
	err := s.db.QueryRow(`SELECT logo_bg_hex FROM brand_config WHERE id = 1`).Scan(&hex)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultLogoBgHex, nil
	}
	if err != nil {
		return "", fmt.Errorf("get logo bg color: %w", err)
	}
	if hex == "" {
		return defaultLogoBgHex, nil
	}
	return hex, nil
}

// SetLogoBgColor stores the welcome logo backdrop color (admin only).
// Empty hex resets to the default.
func (s *SettingsService) SetLogoBgColor(actor Actor, hex string) (string, error) {
	if err := s.requireAdmin(actor); err != nil {
		return "", err
	}
	hex = strings.TrimSpace(hex)
	if hex == "" {
		hex = defaultLogoBgHex
	} else {
		normalized, err := normalizeHexColor(hex)
		if err != nil {
			return "", err
		}
		hex = normalized
	}
	_, err := s.db.Exec(
		`INSERT INTO brand_config (id, logo_bg_hex) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET logo_bg_hex = excluded.logo_bg_hex`,
		hex,
	)
	if err != nil {
		return "", fmt.Errorf("set logo bg color: %w", err)
	}
	return hex, nil
}

func normalizeHexColor(hex string) (string, error) {
	if len(hex) == 0 || hex[0] != '#' {
		return "", fmt.Errorf("color must be #RRGGBB")
	}
	body := hex[1:]
	if len(body) != 6 {
		return "", fmt.Errorf("color must be #RRGGBB")
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return "", fmt.Errorf("color must be #RRGGBB")
		}
	}
	return "#" + strings.ToLower(body), nil
}
