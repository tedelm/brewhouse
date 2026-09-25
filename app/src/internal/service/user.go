package service

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAdminUsername = "admin"
	defaultAdminEmail    = "admin@brewhouse.local"
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

func normalizeContact(c UserContact) UserContact {
	return UserContact{
		FirstName:    strings.TrimSpace(c.FirstName),
		LastName:     strings.TrimSpace(c.LastName),
		AddressLine1: strings.TrimSpace(c.AddressLine1),
		AddressLine2: strings.TrimSpace(c.AddressLine2),
		Phone:        strings.TrimSpace(c.Phone),
		Instagram:    strings.TrimSpace(c.Instagram),
	}
}

func validateInstagramURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("instagram must be a valid http or https URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("instagram must be a valid http or https URL")
	}
	if u.Host == "" {
		return fmt.Errorf("instagram must be a valid http or https URL")
	}
	return nil
}

func applyContact(u *User, c UserContact) {
	u.FirstName = c.FirstName
	u.LastName = c.LastName
	u.AddressLine1 = c.AddressLine1
	u.AddressLine2 = c.AddressLine2
	u.Phone = c.Phone
	u.Instagram = c.Instagram
}

const userSelectCols = `id, username, email, first_name, last_name, address_line1, address_line2, phone, instagram, role, active, created_at`

func scanUser(scanner interface {
	Scan(dest ...any) error
}, includePassword bool) (*User, error) {
	u := &User{}
	var active int64
	var err error
	if includePassword {
		err = scanner.Scan(
			&u.ID, &u.Username, &u.PasswordHash, &u.Email,
			&u.FirstName, &u.LastName, &u.AddressLine1, &u.AddressLine2, &u.Phone, &u.Instagram,
			&u.Role, &active,
		)
	} else {
		err = scanner.Scan(
			&u.ID, &u.Username, &u.Email,
			&u.FirstName, &u.LastName, &u.AddressLine1, &u.AddressLine2, &u.Phone, &u.Instagram,
			&u.Role, &active, &u.CreatedAt,
		)
	}
	if err != nil {
		return nil, err
	}
	u.Active = scanActive(active)
	return u, nil
}

// EnsureDefaultAdmin inserts the bootstrap admin user if it does not already exist.
// When created, returns the one-time plaintext password (also stored in bootstrap_admin until first login).
func (s *UserService) EnsureDefaultAdmin() (plainPassword string, created bool, err error) {
	var existingID int64
	err = s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, defaultAdminUsername).Scan(&existingID)
	if err == nil {
		return "", false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("lookup default admin: %w", err)
	}

	plain, err := generateSHA256Password()
	if err != nil {
		return "", false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", false, fmt.Errorf("hash default admin password: %w", err)
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO users (username, password_hash, email, first_name, last_name, address_line1, address_line2, phone, instagram, role, active, created_at)
		 VALUES (?, ?, ?, '', '', '', '', '', '', ?, 1, ?)`,
		defaultAdminUsername,
		string(hash),
		defaultAdminEmail,
		RoleAdmin,
		createdAt,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert default admin: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO bootstrap_admin (id, username, password_plain) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET username = excluded.username, password_plain = excluded.password_plain`,
		defaultAdminUsername, plain,
	)
	if err != nil {
		return "", false, fmt.Errorf("store bootstrap credentials: %w", err)
	}
	return plain, true, nil
}

// BootstrapCredentials returns pending first-boot credentials when present.
func (s *UserService) BootstrapCredentials() (username, password string, pending bool, err error) {
	err = s.db.QueryRow(
		`SELECT username, password_plain FROM bootstrap_admin WHERE id = 1`,
	).Scan(&username, &password)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("lookup bootstrap credentials: %w", err)
	}
	return username, password, true, nil
}

// ClearBootstrapCredentials removes the one-time first-boot password reveal.
func (s *UserService) ClearBootstrapCredentials() error {
	_, err := s.db.Exec(`DELETE FROM bootstrap_admin WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("clear bootstrap credentials: %w", err)
	}
	return nil
}

// ClearBootstrapCredentialsIfMatch clears pending credentials when username matches.
func (s *UserService) ClearBootstrapCredentialsIfMatch(username string) error {
	u, p, pending, err := s.BootstrapCredentials()
	if err != nil {
		return err
	}
	if !pending || u != username {
		return nil
	}
	_ = p
	return s.ClearBootstrapCredentials()
}

// Authenticate verifies username and password and returns the user on success.
func (s *UserService) Authenticate(username, password string) (*User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, password_hash, email, first_name, last_name, address_line1, address_line2, phone, instagram, role, active
		 FROM users WHERE username = ?`,
		username,
	)
	user, err := scanUser(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
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
	rows, err := s.db.Query(`SELECT ` + userSelectCols + ` FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		u, err := scanUser(rows, false)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// Get returns a user by id.
func (s *UserService) Get(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row, false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// Create inserts a user with a bcrypt-hashed password.
func (s *UserService) Create(username, password, email, role string, contact UserContact) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	contact = normalizeContact(contact)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	if !validUserRole(role) {
		return nil, fmt.Errorf("role must be user, superuser, or admin")
	}
	if err := validateInstagramURL(contact.Instagram); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, email, first_name, last_name, address_line1, address_line2, phone, instagram, role, active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		username, string(hash), email,
		contact.FirstName, contact.LastName, contact.AddressLine1, contact.AddressLine2, contact.Phone, contact.Instagram,
		role, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("user id: %w", err)
	}
	u := &User{ID: id, Username: username, Email: email, Role: role, Active: true, CreatedAt: createdAt}
	applyContact(u, contact)
	return u, nil
}

// Update changes username, email, role, and contact fields. Password is updated when non-empty.
func (s *UserService) Update(id int64, username, password, email, role string, contact UserContact) (*User, error) {
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
	contact = normalizeContact(contact)
	if err := validateInstagramURL(contact.Instagram); err != nil {
		return nil, err
	}

	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		_, err = s.db.Exec(
			`UPDATE users SET username = ?, password_hash = ?, email = ?, first_name = ?, last_name = ?, address_line1 = ?, address_line2 = ?, phone = ?, instagram = ?, role = ? WHERE id = ?`,
			username, string(hash), email,
			contact.FirstName, contact.LastName, contact.AddressLine1, contact.AddressLine2, contact.Phone, contact.Instagram,
			role, id,
		)
		if err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
	} else {
		_, err = s.db.Exec(
			`UPDATE users SET username = ?, email = ?, first_name = ?, last_name = ?, address_line1 = ?, address_line2 = ?, phone = ?, instagram = ?, role = ? WHERE id = ?`,
			username, email,
			contact.FirstName, contact.LastName, contact.AddressLine1, contact.AddressLine2, contact.Phone, contact.Instagram,
			role, id,
		)
		if err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
	}
	return s.Get(id)
}

// UpdateProfile updates the user's email, contact fields, and optionally password (self-service).
func (s *UserService) UpdateProfile(id int64, email, password string, contact UserContact) (*User, error) {
	email = strings.TrimSpace(email)
	contact = normalizeContact(contact)
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	if err := validateInstagramURL(contact.Instagram); err != nil {
		return nil, err
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		_, err = s.db.Exec(
			`UPDATE users SET email = ?, password_hash = ?, first_name = ?, last_name = ?, address_line1 = ?, address_line2 = ?, phone = ?, instagram = ? WHERE id = ?`,
			email, string(hash),
			contact.FirstName, contact.LastName, contact.AddressLine1, contact.AddressLine2, contact.Phone, contact.Instagram,
			id,
		)
		if err != nil {
			return nil, fmt.Errorf("update profile: %w", err)
		}
	} else {
		_, err := s.db.Exec(
			`UPDATE users SET email = ?, first_name = ?, last_name = ?, address_line1 = ?, address_line2 = ?, phone = ?, instagram = ? WHERE id = ?`,
			email,
			contact.FirstName, contact.LastName, contact.AddressLine1, contact.AddressLine2, contact.Phone, contact.Instagram,
			id,
		)
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

// generateSHA256Password returns 64 hex chars from SHA-256 of 32 random bytes.
func generateSHA256Password() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate password entropy: %w", err)
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}
