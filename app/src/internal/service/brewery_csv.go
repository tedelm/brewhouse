package service

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
)

const breweriesCSVHeader = "name,contact_name,contact_email,contact_phone,instagram"

// ExportBreweriesCSV returns breweries visible to the actor as CSV.
func (s *BreweryService) ExportBreweriesCSV(actor Actor) ([]byte, error) {
	list, err := s.List(actor)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(strings.Split(breweriesCSVHeader, ",")); err != nil {
		return nil, fmt.Errorf("write breweries csv header: %w", err)
	}
	for _, b := range list {
		if err := w.Write([]string{b.Name, b.ContactName, b.ContactEmail, b.ContactPhone, b.Instagram}); err != nil {
			return nil, fmt.Errorf("write breweries csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportBreweriesCSV upserts breweries by name. Admin only.
func (s *BreweryService) ImportBreweriesCSV(actor Actor, data []byte) (ImportResult, error) {
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
	idx, err := mapCSVHeader(records[0], []string{"name", "contact_name", "contact_email", "contact_phone", "instagram"})
	if err != nil {
		return ImportResult{}, err
	}

	var result ImportResult
	for i, rec := range records[1:] {
		rowNum := i + 2
		action, err := s.importBreweryRow(actor, idx, rec)
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

func (s *BreweryService) importBreweryRow(actor Actor, idx map[string]int, rec []string) (string, error) {
	name := strings.TrimSpace(csvCol(rec, idx, "name"))
	contactName := strings.TrimSpace(csvCol(rec, idx, "contact_name"))
	contactEmail := strings.TrimSpace(csvCol(rec, idx, "contact_email"))
	contactPhone := strings.TrimSpace(csvCol(rec, idx, "contact_phone"))
	instagram := strings.TrimSpace(csvCol(rec, idx, "instagram"))
	if name == "" {
		return "", fmt.Errorf("name required")
	}

	existing, err := s.getByName(name)
	if errors.Is(err, ErrNotFound) {
		if _, err := s.Create(actor, name, contactName, contactEmail, contactPhone, instagram, nil); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := s.Update(actor, existing.ID, name, contactName, contactEmail, contactPhone, instagram, nil); err != nil {
		return "", err
	}
	return "updated", nil
}

func (s *BreweryService) getByName(name string) (*Brewery, error) {
	b := &Brewery{}
	err := s.db.QueryRow(
		`SELECT id, name, contact_name, contact_email, contact_phone, instagram, created_at FROM breweries WHERE name = ?`,
		name,
	).Scan(&b.ID, &b.Name, &b.ContactName, &b.ContactEmail, &b.ContactPhone, &b.Instagram, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get brewery by name: %w", err)
	}
	return b, nil
}

const membersCSVHeader = "brewery_name,username,role"

// ExportMembersCSV returns all brewery memberships as CSV. Admin only.
func (s *BreweryService) ExportMembersCSV(actor Actor) ([]byte, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	rows, err := s.db.Query(
		`SELECT b.name, u.username, m.role
		 FROM brewery_members m
		 INNER JOIN breweries b ON b.id = m.brewery_id
		 INNER JOIN users u ON u.id = m.user_id
		 ORDER BY b.name, u.username`,
	)
	if err != nil {
		return nil, fmt.Errorf("list members for csv: %w", err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(strings.Split(membersCSVHeader, ",")); err != nil {
		return nil, fmt.Errorf("write members csv header: %w", err)
	}
	for rows.Next() {
		var breweryName, username, role string
		if err := rows.Scan(&breweryName, &username, &role); err != nil {
			return nil, fmt.Errorf("scan member csv row: %w", err)
		}
		if err := w.Write([]string{breweryName, username, role}); err != nil {
			return nil, fmt.Errorf("write members csv row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportMembersCSV adds or updates memberships from CSV. Never removes members. Admin only.
func (s *BreweryService) ImportMembersCSV(actor Actor, data []byte) (ImportResult, error) {
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
	idx, err := mapCSVHeader(records[0], []string{"brewery_name", "username", "role"})
	if err != nil {
		return ImportResult{}, err
	}

	var result ImportResult
	for i, rec := range records[1:] {
		rowNum := i + 2
		action, err := s.importMemberRow(actor, idx, rec)
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

func (s *BreweryService) importMemberRow(actor Actor, idx map[string]int, rec []string) (string, error) {
	breweryName := strings.TrimSpace(csvCol(rec, idx, "brewery_name"))
	username := strings.TrimSpace(csvCol(rec, idx, "username"))
	role := strings.TrimSpace(csvCol(rec, idx, "role"))
	if breweryName == "" {
		return "", fmt.Errorf("brewery_name required")
	}
	if username == "" {
		return "", fmt.Errorf("username required")
	}
	if role == "" {
		return "", fmt.Errorf("role required")
	}
	switch role {
	case RoleUser, RoleSuperuser, RoleBreweryAdmin:
	default:
		return "", fmt.Errorf("invalid membership role %q", role)
	}

	brewery, err := s.getByName(breweryName)
	if errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("unknown brewery %q", breweryName)
	}
	if err != nil {
		return "", err
	}

	var userID int64
	err = s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("unknown username %q", username)
	}
	if err != nil {
		return "", fmt.Errorf("lookup user: %w", err)
	}

	var existingRole string
	err = s.db.QueryRow(
		`SELECT role FROM brewery_members WHERE brewery_id = ? AND user_id = ?`,
		brewery.ID, userID,
	).Scan(&existingRole)
	existed := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup membership: %w", err)
	}

	if err := s.AddMember(actor, brewery.ID, userID, role); err != nil {
		return "", err
	}
	if existed {
		return "updated", nil
	}
	return "created", nil
}
