package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	demoUsername = "demo"
	demoPassword = "demo"
	demoEmail    = "demo@brewhouse.local"
	demoRole     = RoleAdmin
)

// ErrInvalidCredentials is returned when username/password do not match.
var ErrInvalidCredentials = errors.New("invalid credentials")

// UserService provides user persistence and authentication.
type UserService struct {
	db *sql.DB
}

// NewUserService creates a UserService backed by db.
func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

func validUserRole(role string) bool {
	switch role {
	case RoleAdmin, RoleSuperuser, RoleUser:
		return true
	default:
		return false
	}
}

func scanActive(v int64) bool {
	return v != 0
}

func activeInt(active bool) int {
	if active {
		return 1
	}
	return 0
}

// EnsureDemoUser inserts the demo/demo admin user if it does not already exist.
func (s *UserService) EnsureDemoUser() error {
	var existingID int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, demoUsername).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup demo user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO users (username, password_hash, email, role, active, created_at) VALUES (?, ?, ?, ?, 1, ?)`,
		demoUsername,
		string(hash),
		demoEmail,
		demoRole,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert demo user: %w", err)
	}
	return nil
}

// Authenticate verifies username and password and returns the user on success.
func (s *UserService) Authenticate(username, password string) (*User, error) {
	user := &User{}
	var active int64
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, email, role, active FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	user.Active = scanActive(active)
	if !user.Active {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// List returns all users.
func (s *UserService) List() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, email, role, active, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var active int64
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &active, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Active = scanActive(active)
		out = append(out, u)
	}
	return out, rows.Err()
}

// Get returns a user by id.
func (s *UserService) Get(id int64) (*User, error) {
	u := &User{}
	var active int64
	err := s.db.QueryRow(
		`SELECT id, username, email, role, active, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &active, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.Active = scanActive(active)
	return u, nil
}

// Create inserts a user with a bcrypt-hashed password.
func (s *UserService) Create(username, password, email, role string) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	if !validUserRole(role) {
		return nil, fmt.Errorf("role must be user, superuser, or admin")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, email, role, active, created_at) VALUES (?, ?, ?, ?, 1, ?)`,
		username, string(hash), email, role, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("user id: %w", err)
	}
	return &User{ID: id, Username: username, Email: email, Role: role, Active: true, CreatedAt: createdAt}, nil
}

// Update changes username, email, and/or role. Password is updated when non-empty.
func (s *UserService) Update(id int64, username, password, email, role string) (*User, error) {
	if !validUserRole(role) {
		return nil, fmt.Errorf("role must be user, superuser, or admin")
	}
	existing, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if username == "" {
		username = existing.Username
	}
	if email == "" {
		email = existing.Email
	}

	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		_, err = s.db.Exec(
			`UPDATE users SET username = ?, password_hash = ?, email = ?, role = ? WHERE id = ?`,
			username, string(hash), email, role, id,
		)
		if err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
	} else {
		_, err = s.db.Exec(
			`UPDATE users SET username = ?, email = ?, role = ? WHERE id = ?`,
			username, email, role, id,
		)
		if err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
	}
	return s.Get(id)
}

// UpdateProfile updates the user's email and optionally password (self-service).
func (s *UserService) UpdateProfile(id int64, email, password string) (*User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		_, err = s.db.Exec(
			`UPDATE users SET email = ?, password_hash = ? WHERE id = ?`,
			email, string(hash), id,
		)
		if err != nil {
			return nil, fmt.Errorf("update profile: %w", err)
		}
	} else {
		_, err := s.db.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, id)
		if err != nil {
			return nil, fmt.Errorf("update profile: %w", err)
		}
	}
	return s.Get(id)
}

// SetActive sets whether a user account is active.
func (s *UserService) SetActive(id int64, active bool) (*User, error) {
	res, err := s.db.Exec(`UPDATE users SET active = ? WHERE id = ?`, activeInt(active), id)
	if err != nil {
		return nil, fmt.Errorf("set active: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(id)
}

// Delete removes a user by id.
func (s *UserService) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MembershipRoleForCreate maps a user role to the brewery membership role when attaching a brewery.
func MembershipRoleForCreate(userRole string) string {
	if userRole == RoleAdmin {
		return RoleBreweryAdmin
	}
	return userRole
}
