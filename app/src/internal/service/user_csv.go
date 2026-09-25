package service

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	usersCSVHeader = "username,password,email,role,active,first_name,last_name,address_line1,address_line2,phone,instagram"
	importErrorCap = 20
)

// ExportUsersCSV returns all users as CSV (password column empty).
func (s *UserService) ExportUsersCSV() ([]byte, error) {
	users, err := s.List()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(strings.Split(usersCSVHeader, ",")); err != nil {
		return nil, fmt.Errorf("write users csv header: %w", err)
	}
	for _, u := range users {
		active := "0"
		if u.Active {
			active = "1"
		}
		if err := w.Write([]string{
			u.Username,
			"",
			u.Email,
			u.Role,
			active,
			u.FirstName,
			u.LastName,
			u.AddressLine1,
			u.AddressLine2,
			u.Phone,
			u.Instagram,
		}); err != nil {
			return nil, fmt.Errorf("write users csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportUsersCSV upserts users from CSV. Password is applied only when creating.
func (s *UserService) ImportUsersCSV(actor Actor, data []byte) (ImportResult, error) {
	if !actor.IsAdmin() {
		return ImportResult{}, ErrForbidden
	}
	records, err := readCSVRecords(data)
	if err != nil {
		return ImportResult{}, err
	}
	if len(records) == 0 {
		return ImportResult{}, fmt.Errorf("empty csv")
	}
	idx, err := mapCSVHeader(records[0], []string{
		"username", "password", "email", "role", "active",
		"first_name", "last_name", "address_line1", "address_line2", "phone", "instagram",
	})
	if err != nil {
		return ImportResult{}, err
	}

	var result ImportResult
	for i, rec := range records[1:] {
		rowNum := i + 2
		action, err := s.importUserRow(idx, rec)
		if err != nil {
			result.Failed++
			appendImportError(&result, rowNum, err)
			continue
		}
		switch action {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		}
	}
	return result, nil
}

func (s *UserService) importUserRow(idx map[string]int, rec []string) (action string, err error) {
	username := strings.TrimSpace(csvCol(rec, idx, "username"))
	password := csvCol(rec, idx, "password")
	email := strings.TrimSpace(csvCol(rec, idx, "email"))
	role := strings.TrimSpace(csvCol(rec, idx, "role"))
	activeRaw := strings.TrimSpace(csvCol(rec, idx, "active"))
	contact := UserContact{
		FirstName:    csvCol(rec, idx, "first_name"),
		LastName:     csvCol(rec, idx, "last_name"),
		AddressLine1: csvCol(rec, idx, "address_line1"),
		AddressLine2: csvCol(rec, idx, "address_line2"),
		Phone:        csvCol(rec, idx, "phone"),
		Instagram:    csvCol(rec, idx, "instagram"),
	}
	if username == "" {
		return "", fmt.Errorf("username required")
	}
	if email == "" {
		return "", fmt.Errorf("email required")
	}
	if role == "" {
		role = RoleUser
	}
	if !validUserRole(role) {
		return "", fmt.Errorf("invalid role %q", role)
	}

	existing, err := s.getByUsername(username)
	if errors.Is(err, ErrNotFound) {
		if strings.TrimSpace(password) == "" {
			return "", fmt.Errorf("password required for new user")
		}
		created, err := s.Create(username, password, email, role, contact)
		if err != nil {
			return "", err
		}
		active := true
		if activeRaw != "" {
			active, err = parseCSVBool(activeRaw)
			if err != nil {
				return "", err
			}
		}
		if !active {
			if _, err := s.SetActive(created.ID, false); err != nil {
				return "", err
			}
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}

	if _, err := s.Update(existing.ID, username, "", email, role, contact); err != nil {
		return "", err
	}
	if activeRaw != "" {
		active, err := parseCSVBool(activeRaw)
		if err != nil {
			return "", err
		}
		if active != existing.Active {
			if _, err := s.SetActive(existing.ID, active); err != nil {
				return "", err
			}
		}
	}
	return "updated", nil
}

func (s *UserService) getByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE username = ?`, username)
	u, err := scanUser(row, false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func readCSVRecords(data []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	var records [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse csv: %w", err)
		}
		records = append(records, rec)
	}
	return records, nil
}

func mapCSVHeader(header []string, required []string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		idx[key] = i
	}
	for _, name := range required {
		if _, ok := idx[name]; !ok {
			return nil, fmt.Errorf("missing csv column %q", name)
		}
	}
	return idx, nil
}

func csvCol(rec []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func parseCSVBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y":
		return true, nil
	case "0", "false", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}

func appendImportError(result *ImportResult, rowNum int, err error) {
	if len(result.Errors) >= importErrorCap {
		return
	}
	result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", rowNum, err))
}
