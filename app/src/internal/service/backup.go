package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"brewhouse/internal/database"
)

const (
	BackupKindDaily   = "daily"
	BackupKindMonthly = "monthly"
	BackupKindManual  = "manual"

	dailyRetain   = 7
	monthlyRetain = 3
	manualMaxAge  = 90 * 24 * time.Hour
)

var backupNameRe = regexp.MustCompile(`^brewhouse-\d{8}-\d{6}-(daily|monthly|manual)\.db$`)

// BackupInfo describes one on-disk backup file.
type BackupInfo struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// BackupService creates, lists, downloads, restores, and prunes SQLite backups.
type BackupService struct {
	db        *database.Holder
	access    *AccessService
	backupDir string
	mu        sync.Mutex
}

// NewBackupService creates a BackupService.
func NewBackupService(db *database.Holder, access *AccessService, backupDir string) *BackupService {
	return &BackupService{db: db, access: access, backupDir: backupDir}
}

func (s *BackupService) requireAdmin(actor Actor) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

func (s *BackupService) ensureDir() error {
	return os.MkdirAll(s.backupDir, 0o755)
}

func escapeSQLitePath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}

func backupFileName(kind string, at time.Time) string {
	return fmt.Sprintf("brewhouse-%s-%s.db", at.Format("20060102-150405"), kind)
}

func parseBackupName(name string) (kind string, createdAt time.Time, ok bool) {
	if !backupNameRe.MatchString(name) {
		return "", time.Time{}, false
	}
	// brewhouse-YYYYMMDD-HHMMSS-kind.db
	base := strings.TrimSuffix(name, ".db")
	parts := strings.Split(base, "-")
	if len(parts) != 4 {
		return "", time.Time{}, false
	}
	kind = parts[3]
	createdAt, err := time.ParseInLocation("20060102-150405", parts[1]+"-"+parts[2], time.Local)
	if err != nil {
		return "", time.Time{}, false
	}
	return kind, createdAt, true
}

func (s *BackupService) resolveSafe(name string) (string, error) {
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%w: invalid backup name", ErrNotFound)
	}
	if !backupNameRe.MatchString(name) {
		return "", fmt.Errorf("%w: invalid backup name", ErrNotFound)
	}
	if err := s.ensureDir(); err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(s.backupDir)
	if err != nil {
		return "", fmt.Errorf("abs backup dir: %w", err)
	}
	full := filepath.Join(absDir, name)
	if filepath.Dir(full) != absDir {
		return "", fmt.Errorf("%w: invalid backup name", ErrNotFound)
	}
	return full, nil
}

// Create writes a consistent SQLite snapshot via VACUUM INTO.
func (s *BackupService) Create(actor Actor, kind string) (*BackupInfo, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	return s.create(kind)
}

func (s *BackupService) create(kind string) (*BackupInfo, error) {
	switch kind {
	case BackupKindDaily, BackupKindMonthly, BackupKindManual:
	default:
		return nil, fmt.Errorf("invalid backup kind %q", kind)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(); err != nil {
		return nil, fmt.Errorf("ensure backup dir: %w", err)
	}

	now := time.Now()
	name := backupFileName(kind, now)
	dest := filepath.Join(s.backupDir, name)
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return nil, fmt.Errorf("abs backup path: %w", err)
	}

	sql := fmt.Sprintf(`VACUUM INTO '%s'`, escapeSQLitePath(absDest))
	if _, err := s.db.Exec(sql); err != nil {
		_ = os.Remove(absDest)
		return nil, fmt.Errorf("vacuum into backup: %w", err)
	}

	info, err := os.Stat(absDest)
	if err != nil {
		return nil, fmt.Errorf("stat backup: %w", err)
	}
	return &BackupInfo{
		Name:      name,
		Kind:      kind,
		SizeBytes: info.Size(),
		CreatedAt: now,
	}, nil
}

// CreateScheduled runs an unattended daily (and monthly on the 1st) backup + prune.
func (s *BackupService) CreateScheduled() error {
	if _, err := s.create(BackupKindDaily); err != nil {
		return err
	}
	now := time.Now()
	if now.Day() == 1 {
		if _, err := s.create(BackupKindMonthly); err != nil {
			return err
		}
	}
	return s.prune()
}

// List returns backups newest first.
func (s *BackupService) List(actor Actor) ([]BackupInfo, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	if err := s.ensureDir(); err != nil {
		return nil, fmt.Errorf("ensure backup dir: %w", err)
	}
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		kind, createdAt, ok := parseBackupName(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Name:      e.Name(),
			Kind:      kind,
			SizeBytes: info.Size(),
			CreatedAt: createdAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// OpenDownload returns a read-only file handle for a stored backup.
func (s *BackupService) OpenDownload(actor Actor, name string) (*os.File, *BackupInfo, error) {
	if err := s.requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	full, err := s.resolveSafe(name)
	if err != nil {
		return nil, nil, err
	}
	kind, createdAt, ok := parseBackupName(name)
	if !ok {
		return nil, nil, fmt.Errorf("%w: invalid backup name", ErrNotFound)
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("open backup: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, &BackupInfo{
		Name:      name,
		Kind:      kind,
		SizeBytes: st.Size(),
		CreatedAt: createdAt,
	}, nil
}

// Delete removes a stored backup file.
func (s *BackupService) Delete(actor Actor, name string) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	full, err := s.resolveSafe(name)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// RestoreFromStored replaces the live database with a stored backup.
func (s *BackupService) RestoreFromStored(actor Actor, name string) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	full, err := s.resolveSafe(name)
	if err != nil {
		return err
	}
	return s.restoreFromPath(full)
}

// RestoreFromUpload replaces the live database with an uploaded SQLite file.
func (s *BackupService) RestoreFromUpload(actor Actor, r io.Reader) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("ensure backup dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.backupDir, "upload-*.db")
	if err != nil {
		return fmt.Errorf("create temp upload: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close upload: %w", err)
	}
	if err := validateSQLiteFile(tmpPath); err != nil {
		return err
	}
	return s.restoreFromPath(tmpPath)
}

func validateSQLiteFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open candidate: %w", err)
	}
	defer f.Close()
	hdr := make([]byte, 16)
	n, err := io.ReadFull(f, hdr)
	if err != nil || n < 16 {
		return fmt.Errorf("invalid sqlite file: too short")
	}
	if string(hdr) != "SQLite format 3\x00" {
		return fmt.Errorf("invalid sqlite file: bad header")
	}
	return nil
}

func (s *BackupService) restoreFromPath(src string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateSQLiteFile(src); err != nil {
		return err
	}
	return s.db.RestoreFrom(src)
}

// Prune applies retention: 7 daily, 3 monthly, manual older than 90 days.
func (s *BackupService) Prune(actor Actor) error {
	if err := s.requireAdmin(actor); err != nil {
		return err
	}
	return s.prune()
}

func (s *BackupService) prune() error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		return err
	}
	var daily, monthly, manual []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		kind, createdAt, ok := parseBackupName(e.Name())
		if !ok {
			continue
		}
		info := BackupInfo{Name: e.Name(), Kind: kind, CreatedAt: createdAt}
		switch kind {
		case BackupKindDaily:
			daily = append(daily, info)
		case BackupKindMonthly:
			monthly = append(monthly, info)
		case BackupKindManual:
			manual = append(manual, info)
		}
	}
	sortNewest := func(list []BackupInfo) {
		sort.Slice(list, func(i, j int) bool {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		})
	}
	sortNewest(daily)
	sortNewest(monthly)
	sortNewest(manual)

	var remove []string
	if len(daily) > dailyRetain {
		for _, b := range daily[dailyRetain:] {
			remove = append(remove, b.Name)
		}
	}
	if len(monthly) > monthlyRetain {
		for _, b := range monthly[monthlyRetain:] {
			remove = append(remove, b.Name)
		}
	}
	cutoff := time.Now().Add(-manualMaxAge)
	for _, b := range manual {
		if b.CreatedAt.Before(cutoff) {
			remove = append(remove, b.Name)
		}
	}
	for _, name := range remove {
		_ = os.Remove(filepath.Join(s.backupDir, name))
	}
	return nil
}
