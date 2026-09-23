package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BreweryService manages breweries and memberships.
type BreweryService struct {
	db     *sql.DB
	access *AccessService
}

// NewBreweryService creates a BreweryService.
func NewBreweryService(db *sql.DB, access *AccessService) *BreweryService {
	return &BreweryService{db: db, access: access}
}

// List returns breweries visible to the actor.
func (s *BreweryService) List(actor Actor) ([]Brewery, error) {
	var rows *sql.Rows
	var err error
	if actor.IsAdmin() {
		rows, err = s.db.Query(
			`SELECT id, name, contact_name, contact_email, contact_phone, created_at
			 FROM breweries ORDER BY name`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT b.id, b.name, b.contact_name, b.contact_email, b.contact_phone, b.created_at
			 FROM breweries b
			 INNER JOIN brewery_members m ON m.brewery_id = b.id
			 WHERE m.user_id = ?
			 ORDER BY b.name`,
			actor.UserID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list breweries: %w", err)
	}
	defer rows.Close()

	var out []Brewery
	for rows.Next() {
		var b Brewery
		if err := rows.Scan(&b.ID, &b.Name, &b.ContactName, &b.ContactEmail, &b.ContactPhone, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan brewery: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Get returns a brewery if the actor can access it.
func (s *BreweryService) Get(actor Actor, id int64) (*Brewery, error) {
	if err := s.access.RequireBreweryAccess(actor, id); err != nil {
		return nil, err
	}
	b := &Brewery{}
	err := s.db.QueryRow(
		`SELECT id, name, contact_name, contact_email, contact_phone, created_at FROM breweries WHERE id = ?`,
		id,
	).Scan(&b.ID, &b.Name, &b.ContactName, &b.ContactEmail, &b.ContactPhone, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get brewery: %w", err)
	}
	return b, nil
}

// Create inserts a brewery. Admin only. Optionally assigns brewery_admin.
func (s *BreweryService) Create(actor Actor, name, contactName, contactEmail, contactPhone string, breweryAdminUserID *int64) (*Brewery, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO breweries (name, contact_name, contact_email, contact_phone, created_at) VALUES (?, ?, ?, ?, ?)`,
		name, contactName, contactEmail, contactPhone, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert brewery: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if breweryAdminUserID != nil && *breweryAdminUserID > 0 {
		if err := s.addMemberTx(nil, *breweryAdminUserID, id, RoleBreweryAdmin); err != nil {
			return nil, err
		}
	}
	return s.Get(actor, id)
}

// Update updates brewery contact fields.
func (s *BreweryService) Update(actor Actor, id int64, name, contactName, contactEmail, contactPhone string) (*Brewery, error) {
	ok, err := s.access.CanManageBrewery(actor, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	_, err = s.db.Exec(
		`UPDATE breweries SET name = ?, contact_name = ?, contact_email = ?, contact_phone = ? WHERE id = ?`,
		name, contactName, contactEmail, contactPhone, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update brewery: %w", err)
	}
	return s.Get(actor, id)
}

// Delete removes a brewery. Admin only.
func (s *BreweryService) Delete(actor Actor, id int64) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	res, err := s.db.Exec(`DELETE FROM breweries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete brewery: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMembers returns members of a brewery.
func (s *BreweryService) ListMembers(actor Actor, breweryID int64) ([]BreweryMember, error) {
	if err := s.access.RequireBreweryAccess(actor, breweryID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT m.user_id, u.username, m.brewery_id, m.role
		 FROM brewery_members m
		 INNER JOIN users u ON u.id = m.user_id
		 WHERE m.brewery_id = ?
		 ORDER BY u.username`,
		breweryID,
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var out []BreweryMember
	for rows.Next() {
		var m BreweryMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.BreweryID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMember adds or updates a brewery membership.
func (s *BreweryService) AddMember(actor Actor, breweryID, userID int64, role string) error {
	ok, err := s.access.CanManageBrewery(actor, breweryID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return s.addMemberTx(nil, userID, breweryID, role)
}

func (s *BreweryService) addMemberTx(tx *sql.Tx, userID, breweryID int64, role string) error {
	switch role {
	case RoleUser, RoleSuperuser, RoleBreweryAdmin:
	default:
		return fmt.Errorf("invalid membership role")
	}
	exec := s.db.Exec
	if tx != nil {
		exec = tx.Exec
	}
	_, err := exec(
		`INSERT INTO brewery_members (user_id, brewery_id, role) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, brewery_id) DO UPDATE SET role = excluded.role`,
		userID, breweryID, role,
	)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember removes a membership.
func (s *BreweryService) RemoveMember(actor Actor, breweryID, userID int64) error {
	ok, err := s.access.CanManageBrewery(actor, breweryID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	res, err := s.db.Exec(
		`DELETE FROM brewery_members WHERE brewery_id = ? AND user_id = ?`,
		breweryID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
