package service

import (
	"database/sql"
	"fmt"
	"time"
)

// ScheduleService books brewery equipment days and fermentation tanks.
type ScheduleService struct {
	db     *sql.DB
	access *AccessService
}

// NewScheduleService creates a ScheduleService.
func NewScheduleService(db *sql.DB, access *AccessService) *ScheduleService {
	return &ScheduleService{db: db, access: access}
}

// BookRequest holds schedule booking parameters.
type BookRequest struct {
	RecipeID  int64  `json:"recipe_id"`
	Date      string `json:"date"` // YYYY-MM-DD brew day
	TankID    int64  `json:"tank_id"`
	TankDays  int    `json:"tank_days"` // inclusive days tank is occupied from Date
}

// IsDateAvailable reports whether the main brewery equipment is free on date.
func (s *ScheduleService) IsDateAvailable(date string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM brewery_bookings WHERE booked_date = ?`, date).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// IsTankAvailable reports whether a tank is free for [start, end] inclusive.
func (s *ScheduleService) IsTankAvailable(tankID int64, start, end string, excludeRecipeID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tank_bookings
		 WHERE tank_id = ?
		   AND recipe_id != ?
		   AND start_date <= ?
		   AND end_date >= ?`,
		tankID, excludeRecipeID, end, start,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// Book reserves brewery day + tank for a recipe and sets status to scheduled.
func (s *ScheduleService) Book(actor Actor, req BookRequest) error {
	if req.Date == "" || req.TankID == 0 || req.RecipeID == 0 {
		return fmt.Errorf("recipe_id, date, and tank_id required")
	}
	if req.TankDays < 1 {
		req.TankDays = 14
	}

	var breweryID int64
	var status string
	err := s.db.QueryRow(`SELECT brewery_id, status FROM recipes WHERE id = ?`, req.RecipeID).
		Scan(&breweryID, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.access.RequireBreweryAccess(actor, breweryID); err != nil {
		return err
	}
	if status != StatusCreated && status != StatusScheduled {
		return ErrInvalidStatus
	}

	start := req.Date
	endTime, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return fmt.Errorf("invalid date: %w", err)
	}
	end := endTime.AddDate(0, 0, req.TankDays-1).Format("2006-01-02")

	ok, err := s.IsDateAvailable(req.Date)
	if err != nil {
		return err
	}
	if !ok {
		// Allow rebooking same recipe's existing date
		var existingRecipe int64
		err := s.db.QueryRow(`SELECT recipe_id FROM brewery_bookings WHERE booked_date = ?`, req.Date).Scan(&existingRecipe)
		if err != nil || existingRecipe != req.RecipeID {
			return ErrConflict
		}
	}

	ok, err = s.IsTankAvailable(req.TankID, start, end, req.RecipeID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConflict
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec(`DELETE FROM brewery_bookings WHERE recipe_id = ?`, req.RecipeID)
	_, _ = tx.Exec(`DELETE FROM tank_bookings WHERE recipe_id = ?`, req.RecipeID)

	_, err = tx.Exec(
		`INSERT INTO brewery_bookings (recipe_id, booked_date) VALUES (?, ?)`,
		req.RecipeID, req.Date,
	)
	if err != nil {
		return fmt.Errorf("brewery booking: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO tank_bookings (recipe_id, tank_id, start_date, end_date) VALUES (?, ?, ?, ?)`,
		req.RecipeID, req.TankID, start, end,
	)
	if err != nil {
		return fmt.Errorf("tank booking: %w", err)
	}
	_, err = tx.Exec(
		`UPDATE recipes SET booked_date = ?, tank_id = ?, status = ? WHERE id = ?`,
		req.Date, req.TankID, StatusScheduled, req.RecipeID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListBookings returns brewery bookings in a date range.
func (s *ScheduleService) ListBookings(from, to string) ([]map[string]any, error) {
	rows, err := s.db.Query(
		`SELECT b.booked_date, COALESCE(tb.end_date, b.booked_date), br.name, r.name, COALESCE(t.name, '')
		 FROM brewery_bookings b
		 INNER JOIN recipes r ON r.id = b.recipe_id
		 INNER JOIN breweries br ON br.id = r.brewery_id
		 LEFT JOIN tank_bookings tb ON tb.recipe_id = b.recipe_id
		 LEFT JOIN fermentation_tanks t ON t.id = tb.tank_id
		 WHERE b.booked_date >= ? AND b.booked_date <= ?
		 ORDER BY b.booked_date`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var date, endDate, breweryName, recipeName, tankName string
		if err := rows.Scan(&date, &endDate, &breweryName, &recipeName, &tankName); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"date":          date,
			"end_date":      endDate,
			"brewery_name":  breweryName,
			"name":          recipeName,
			"tank_name":     tankName,
		})
	}
	return out, rows.Err()
}
