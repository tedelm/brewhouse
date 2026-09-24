package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens a SQLite database at path and applies migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			active INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS breweries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			contact_name TEXT NOT NULL DEFAULT '',
			contact_email TEXT NOT NULL DEFAULT '',
			contact_phone TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS brewery_members (
			user_id INTEGER NOT NULL,
			brewery_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (user_id, brewery_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (brewery_id) REFERENCES breweries(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS inventory_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			name TEXT NOT NULL,
			unit TEXT NOT NULL DEFAULT 'kg',
			qty REAL NOT NULL DEFAULT 0,
			cost_price REAL NOT NULL DEFAULT 0,
			producer TEXT NOT NULL DEFAULT '',
			item_type TEXT NOT NULL DEFAULT '',
			min_ebc REAL NOT NULL DEFAULT 0,
			max_ebc REAL NOT NULL DEFAULT 0,
			link TEXT NOT NULL DEFAULT '',
			UNIQUE(category, name, producer)
		)`,
		`CREATE TABLE IF NOT EXISTS inventory_orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			status TEXT NOT NULL DEFAULT 'planning',
			notes TEXT NOT NULL DEFAULT '',
			external_order_id TEXT NOT NULL DEFAULT '',
			created_by INTEGER,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT '',
			ordered_at TEXT,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS fermentation_tanks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			capacity_liters REAL NOT NULL DEFAULT 0,
			active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS alcohol_tax_tiers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			min_abv REAL NOT NULL,
			max_abv REAL NOT NULL,
			sek_per_liter REAL NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS alcohol_tax_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			rate_sek REAL NOT NULL,
			free_max_abv REAL NOT NULL,
			discount REAL NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS beer_price_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			min_net_sek_per_liter REAL NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_logo (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			content_type TEXT NOT NULL,
			data BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_favicon (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			content_type TEXT NOT NULL,
			data BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS brand_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			logo_bg_hex TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS price_multipliers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			multiplier REAL NOT NULL DEFAULT 1.0,
			active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS hygiene_routines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS recipes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			brewery_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'created',
			booked_date TEXT,
			tank_id INTEGER,
			og REAL,
			fg REAL,
			brew_volume REAL,
			delivery_volume REAL,
			cost REAL,
			tax REAL,
			net REAL,
			created_by INTEGER,
			created_at TEXT NOT NULL,
			delivered_at TEXT,
			active INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (brewery_id) REFERENCES breweries(id) ON DELETE CASCADE,
			FOREIGN KEY (tank_id) REFERENCES fermentation_tanks(id) ON DELETE SET NULL,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS inventory_order_lines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL,
			inventory_item_id INTEGER,
			item_name TEXT NOT NULL,
			category TEXT NOT NULL,
			qty REAL NOT NULL,
			ordered_qty REAL,
			brewery_id INTEGER,
			recipe_id INTEGER,
			FOREIGN KEY (order_id) REFERENCES inventory_orders(id) ON DELETE CASCADE,
			FOREIGN KEY (inventory_item_id) REFERENCES inventory_items(id) ON DELETE SET NULL,
			FOREIGN KEY (brewery_id) REFERENCES breweries(id) ON DELETE SET NULL,
			FOREIGN KEY (recipe_id) REFERENCES recipes(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_ingredients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recipe_id INTEGER NOT NULL,
			inventory_item_id INTEGER NOT NULL,
			qty REAL NOT NULL,
			unit TEXT NOT NULL DEFAULT '',
			checked_out REAL NOT NULL DEFAULT 0,
			cost_price REAL NOT NULL DEFAULT 0,
			FOREIGN KEY (recipe_id) REFERENCES recipes(id) ON DELETE CASCADE,
			FOREIGN KEY (inventory_item_id) REFERENCES inventory_items(id)
		)`,
		`CREATE TABLE IF NOT EXISTS brewery_bookings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recipe_id INTEGER NOT NULL UNIQUE,
			booked_date TEXT NOT NULL UNIQUE,
			FOREIGN KEY (recipe_id) REFERENCES recipes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS tank_bookings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recipe_id INTEGER NOT NULL UNIQUE,
			tank_id INTEGER NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			FOREIGN KEY (recipe_id) REFERENCES recipes(id) ON DELETE CASCADE,
			FOREIGN KEY (tank_id) REFERENCES fermentation_tanks(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_hygiene_checks (
			recipe_id INTEGER NOT NULL,
			routine_id INTEGER NOT NULL,
			completed_at TEXT NOT NULL,
			PRIMARY KEY (recipe_id, routine_id),
			FOREIGN KEY (recipe_id) REFERENCES recipes(id) ON DELETE CASCADE,
			FOREIGN KEY (routine_id) REFERENCES hygiene_routines(id) ON DELETE CASCADE
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate statement: %w", err)
		}
	}

	if err := ensureUserEmailColumn(db); err != nil {
		return fmt.Errorf("ensure users.email: %w", err)
	}
	if err := ensureUserActiveColumn(db); err != nil {
		return fmt.Errorf("ensure users.active: %w", err)
	}
	if err := ensureInventoryCatalogColumns(db); err != nil {
		return fmt.Errorf("ensure inventory catalog columns: %w", err)
	}
	if err := migrateOrderStatuses(db); err != nil {
		return fmt.Errorf("migrate order statuses: %w", err)
	}
	if err := ensureRecipeIngredientUnitColumns(db); err != nil {
		return fmt.Errorf("ensure recipe ingredient units: %w", err)
	}
	if err := ensureOrderBreweryColumns(db); err != nil {
		return fmt.Errorf("ensure order brewery columns: %w", err)
	}
	if err := ensureHygieneChecklist(db); err != nil {
		return fmt.Errorf("ensure hygiene checklist: %w", err)
	}
	if err := ensureTankAndMultiplierActiveColumns(db); err != nil {
		return fmt.Errorf("ensure tank/multiplier active: %w", err)
	}
	if err := ensureRecipeActiveColumn(db); err != nil {
		return fmt.Errorf("ensure recipes.active: %w", err)
	}

	if err := seedDefaults(db); err != nil {
		return fmt.Errorf("seed defaults: %w", err)
	}
	return nil
}

func ensureTankAndMultiplierActiveColumns(db *sql.DB) error {
	if err := addColumnIfMissing(db, "fermentation_tanks", "active",
		`ALTER TABLE fermentation_tanks ADD COLUMN active INTEGER NOT NULL DEFAULT 1`); err != nil {
		return err
	}
	return addColumnIfMissing(db, "price_multipliers", "active",
		`ALTER TABLE price_multipliers ADD COLUMN active INTEGER NOT NULL DEFAULT 1`)
}

func ensureRecipeActiveColumn(db *sql.DB) error {
	return addColumnIfMissing(db, "recipes", "active",
		`ALTER TABLE recipes ADD COLUMN active INTEGER NOT NULL DEFAULT 1`)
}

func ensureUserEmailColumn(db *sql.DB) error {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'email'`,
	).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''`)
	return err
}

func ensureUserActiveColumn(db *sql.DB) error {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'active'`,
	).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1`)
	return err
}

// ensureInventoryCatalogColumns recreates inventory_items when malt catalog columns are missing
// (SQLite cannot change UNIQUE constraints in place).
func ensureInventoryCatalogColumns(db *sql.DB) error {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('inventory_items') WHERE name = 'producer'`,
	).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	_, err = tx.Exec(`
		CREATE TABLE inventory_items_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			name TEXT NOT NULL,
			unit TEXT NOT NULL DEFAULT 'kg',
			qty REAL NOT NULL DEFAULT 0,
			cost_price REAL NOT NULL DEFAULT 0,
			producer TEXT NOT NULL DEFAULT '',
			item_type TEXT NOT NULL DEFAULT '',
			min_ebc REAL NOT NULL DEFAULT 0,
			max_ebc REAL NOT NULL DEFAULT 0,
			link TEXT NOT NULL DEFAULT '',
			UNIQUE(category, name, producer)
		)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO inventory_items_new (id, category, name, unit, qty, cost_price, producer, item_type, min_ebc, max_ebc, link)
		SELECT id, category, name, unit, qty, cost_price, '', '', 0, 0, '' FROM inventory_items`)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE inventory_items`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE inventory_items_new RENAME TO inventory_items`); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateOrderStatuses(db *sql.DB) error {
	_, err := db.Exec(`UPDATE inventory_orders SET status = 'planning' WHERE status = 'open'`)
	return err
}

func ensureRecipeIngredientUnitColumns(db *sql.DB) error {
	if err := addColumnIfMissing(db, "recipe_ingredients", "unit", `ALTER TABLE recipe_ingredients ADD COLUMN unit TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	hadCheckedOut, err := columnExists(db, "recipe_ingredients", "checked_out")
	if err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "recipe_ingredients", "checked_out", `ALTER TABLE recipe_ingredients ADD COLUMN checked_out REAL NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if !hadCheckedOut {
		// Existing rows were fully checked out at create time.
		_, err = db.Exec(`UPDATE recipe_ingredients SET checked_out = qty`)
		return err
	}
	return nil
}

func ensureOrderBreweryColumns(db *sql.DB) error {
	if err := addColumnIfMissing(db, "inventory_orders", "external_order_id", `ALTER TABLE inventory_orders ADD COLUMN external_order_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "inventory_orders", "updated_at", `ALTER TABLE inventory_orders ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "inventory_orders", "ordered_at", `ALTER TABLE inventory_orders ADD COLUMN ordered_at TEXT`); err != nil {
		return err
	}
	_, _ = db.Exec(`UPDATE inventory_orders SET updated_at = created_at WHERE updated_at = '' OR updated_at IS NULL`)
	if err := addColumnIfMissing(db, "inventory_order_lines", "brewery_id", `ALTER TABLE inventory_order_lines ADD COLUMN brewery_id INTEGER`); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "inventory_order_lines", "ordered_qty", `ALTER TABLE inventory_order_lines ADD COLUMN ordered_qty REAL`); err != nil {
		return err
	}
	return addColumnIfMissing(db, "inventory_order_lines", "recipe_id", `ALTER TABLE inventory_order_lines ADD COLUMN recipe_id INTEGER`)
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('`+table+`') WHERE name = ?`,
		column,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func addColumnIfMissing(db *sql.DB, table, column, alterSQL string) error {
	ok, err := columnExists(db, table, column)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	_, err = db.Exec(alterSQL)
	return err
}

func seedDefaults(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alcohol_tax_config`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		// Swedish beer tax: 0 at/below free_max_abv; else ABV × rate_sek × discount (SEK/L).
		if _, err := db.Exec(
			`INSERT INTO alcohol_tax_config (id, rate_sek, free_max_abv, discount) VALUES (1, 2.28, 2.8, 1.0)`,
		); err != nil {
			return err
		}
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM beer_price_config`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(
			`INSERT INTO beer_price_config (id, min_net_sek_per_liter) VALUES (1, 55)`,
		); err != nil {
			return err
		}
	} else if _, err := db.Exec(
		`UPDATE beer_price_config SET min_net_sek_per_liter = 55 WHERE id = 1 AND min_net_sek_per_liter = 0`,
	); err != nil {
		return err
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM brand_config`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(
			`INSERT INTO brand_config (id, logo_bg_hex) VALUES (1, '#6c704a')`,
		); err != nil {
			return err
		}
	}

	if _, err := db.Exec(
		`DELETE FROM price_multipliers WHERE name IN ('good', 'extra good', 'super')`,
	); err != nil {
		return err
	}
	for _, row := range []struct {
		name string
		mult float64
	}{
		{"default", 1.0},
		{"1.25", 1.25},
		{"1.50", 1.50},
		{"1.75", 1.75},
		{"2.00", 2.00},
		{"2.25", 2.25},
		{"2.50", 2.50},
		{"2.75", 2.75},
		{"3.00", 3.00},
	} {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO price_multipliers (name, multiplier) VALUES (?, ?)`,
			row.name, row.mult,
		); err != nil {
			return err
		}
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM hygiene_routines`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		for _, r := range canonicalHygieneRoutines() {
			if _, err := db.Exec(
				`INSERT INTO hygiene_routines (name, description, sort_order) VALUES (?, ?, ?)`,
				r.name, r.desc, r.order,
			); err != nil {
				return err
			}
		}
	}

	if err := seedInventoryCatalogs(db); err != nil {
		return err
	}

	return nil
}

type hygieneRoutineSeed struct {
	name, desc string
	order      int
}

func canonicalHygieneRoutines() []hygieneRoutineSeed {
	return []hygieneRoutineSeed{
		{"Empty the dish rack of dry dishes and put everything in its proper place.", "Brewing — Before brewing", 1},
		{"Wipe down the dish rack (before brewing).", "Brewing — Before brewing", 2},
		{"Clean the brew systems and the pumps.", "Brewing — After brewing", 3},
		{"Flush the plate heat exchanger and pump (if used) from both directions.", "Brewing — After brewing", 4},
		{"Clean buckets used for milling malt.", "Brewing — After brewing", 5},
		{"Sweep the floor in the malt store and ensure everything is in its proper place.", "Brewing — After brewing", 6},
		{"Wash all other equipment that was used.", "Brewing — After brewing", 7},
		{"Wipe down all surfaces.", "Brewing — After brewing", 8},
		{"Mop the floor with detergent including unused areas (after brewing).", "Brewing — After brewing", 9},
		{"Scrape the floor so no puddles remain.", "Brewing — After brewing", 10},
		{"Take out the trash and put in a new bin bag.", "Brewing — After brewing", 11},
		{"Empty the dish rack of dry dishes and put them in their proper place.", "Kegging / carbonation", 12},
		{"Wipe down the dish rack (kegging).", "Kegging / carbonation", 13},
		{"Clean fermentation buckets that were used.", "Kegging / carbonation", 14},
		{"Mop the floor with detergent including unused areas (kegging).", "Kegging / carbonation", 15},
		{"Clean the hose to the CO₂ cylinder.", "Kegging / carbonation", 16},
		{"Carry any empty CO₂ cylinder out to storage in the garage.", "Kegging / carbonation", 17},
	}
}

func ensureHygieneChecklist(db *sql.DB) error {
	legacy := []string{
		"Clean mash tun",
		"Clean kettle",
		"Sanitize fermenter",
		"Floor and drains",
	}
	for _, name := range legacy {
		if _, err := db.Exec(`DELETE FROM hygiene_routines WHERE name = ?`, name); err != nil {
			return err
		}
	}
	for _, r := range canonicalHygieneRoutines() {
		var id int64
		err := db.QueryRow(
			`SELECT id FROM hygiene_routines WHERE name = ? AND description = ?`,
			r.name, r.desc,
		).Scan(&id)
		if err == nil {
			_, err = db.Exec(
				`UPDATE hygiene_routines SET sort_order = ? WHERE id = ?`,
				r.order, id,
			)
			if err != nil {
				return err
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err := db.Exec(
			`INSERT INTO hygiene_routines (name, description, sort_order) VALUES (?, ?, ?)`,
			r.name, r.desc, r.order,
		); err != nil {
			return err
		}
	}
	return nil
}
