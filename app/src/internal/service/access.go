package service

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrForbidden is returned when the actor lacks permission.
var ErrForbidden = errors.New("forbidden")

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned for booking or uniqueness conflicts.
var ErrConflict = errors.New("conflict")

// ErrInsufficientStock is returned when recipe create cannot check out stock.
var ErrInsufficientStock = errors.New("insufficient stock")

// ErrInvalidStatus is returned when a status transition is not allowed.
var ErrInvalidStatus = errors.New("invalid status")

// AccessService resolves brewery membership for authorization.
type AccessService struct {
	db *sql.DB
}

// NewAccessService creates an AccessService.
func NewAccessService(db *sql.DB) *AccessService {
	return &AccessService{db: db}
}

// IsAdmin reports whether the actor has the global admin role.
func (a Actor) IsAdmin() bool {
	return a.Role == RoleAdmin
}

// MembershipRole returns the actor's role in a brewery, or empty if not a member.
func (s *AccessService) MembershipRole(userID, breweryID int64) (string, error) {
	var role string
	err := s.db.QueryRow(
		`SELECT role FROM brewery_members WHERE user_id = ? AND brewery_id = ?`,
		userID, breweryID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("membership role: %w", err)
	}
	return role, nil
}

// CanAccessBrewery reports whether the actor may view brewery data.
func (s *AccessService) CanAccessBrewery(actor Actor, breweryID int64) (bool, error) {
	if actor.IsAdmin() {
		return true, nil
	}
	role, err := s.MembershipRole(actor.UserID, breweryID)
	if err != nil {
		return false, err
	}
	return role != "", nil
}

// CanManageBrewery reports brewery_admin (or global admin) access.
func (s *AccessService) CanManageBrewery(actor Actor, breweryID int64) (bool, error) {
	if actor.IsAdmin() {
		return true, nil
	}
	role, err := s.MembershipRole(actor.UserID, breweryID)
	if err != nil {
		return false, err
	}
	return role == RoleBreweryAdmin, nil
}

// CanManageInventory reports inventory write access (superuser membership or admin).
func (s *AccessService) CanManageInventory(actor Actor) (bool, error) {
	if actor.IsAdmin() {
		return true, nil
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM brewery_members WHERE user_id = ? AND role IN (?, ?)`,
		actor.UserID, RoleSuperuser, RoleBreweryAdmin,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("inventory access: %w", err)
	}
	return n > 0, nil
}

// CanManageEconomy reports economy/tax write access (global admin only).
func (s *AccessService) CanManageEconomy(actor Actor) (bool, error) {
	return actor.IsAdmin(), nil
}

// RequireBreweryAccess returns ErrForbidden if the actor cannot access the brewery.
func (s *AccessService) RequireBreweryAccess(actor Actor, breweryID int64) error {
	ok, err := s.CanAccessBrewery(actor, breweryID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
