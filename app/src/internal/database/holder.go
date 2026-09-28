package database

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Holder wraps *sql.DB so the connection can be swapped after a restore
// without rewriting every service call site.
type Holder struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewHolder wraps an open database at path.
func NewHolder(db *sql.DB, path string) *Holder {
	return &Holder{db: db, path: path}
}

// OpenHolder opens a SQLite database and returns a swappable Holder.
func OpenHolder(path string) (*Holder, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	return NewHolder(db, path), nil
}

// Path returns the on-disk database path.
func (h *Holder) Path() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.path
}

// DB returns the current *sql.DB (may change after Replace).
func (h *Holder) DB() *sql.DB {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.db
}

// Query forwards to the current database.
func (h *Holder) Query(query string, args ...any) (*sql.Rows, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, fmt.Errorf("database is closed")
	}
	return h.db.Query(query, args...)
}

// QueryRow forwards to the current database.
func (h *Holder) QueryRow(query string, args ...any) *sql.Row {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.db.QueryRow(query, args...)
}

// Exec forwards to the current database.
func (h *Holder) Exec(query string, args ...any) (sql.Result, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, fmt.Errorf("database is closed")
	}
	return h.db.Exec(query, args...)
}

// Begin forwards to the current database.
func (h *Holder) Begin() (*sql.Tx, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, fmt.Errorf("database is closed")
	}
	return h.db.Begin()
}

// Ping forwards to the current database.
func (h *Holder) Ping() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return fmt.Errorf("database is closed")
	}
	return h.db.Ping()
}

// Close closes the current database connection.
func (h *Holder) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.db == nil {
		return nil
	}
	err := h.db.Close()
	h.db = nil
	return err
}

// Replace closes the current connection, opens path, and swaps it in.
func (h *Holder) Replace(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.replaceLocked(path)
}

func (h *Holder) replaceLocked(path string) error {
	newDB, err := Open(path)
	if err != nil {
		return fmt.Errorf("open replacement db: %w", err)
	}
	if h.db != nil {
		_ = h.db.Close()
	}
	h.db = newDB
	h.path = path
	return nil
}

// RestoreFrom replaces the live database file with src under an exclusive lock
// so concurrent queries wait until the swap completes.
func (h *Holder) RestoreFrom(src string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	absDB, err := filepath.Abs(h.path)
	if err != nil {
		return fmt.Errorf("abs db path: %w", err)
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("abs src path: %w", err)
	}

	preRestore := absDB + ".pre-restore"
	_ = os.Remove(preRestore)
	if err := copyFile(absDB, preRestore); err != nil {
		return fmt.Errorf("pre-restore snapshot: %w", err)
	}

	staging := absDB + ".restore-staging"
	_ = os.Remove(staging)
	if err := copyFile(absSrc, staging); err != nil {
		return fmt.Errorf("stage restore: %w", err)
	}

	if h.db != nil {
		_ = h.db.Close()
		h.db = nil
	}

	if err := os.Rename(staging, absDB); err != nil {
		_ = os.Remove(staging)
		if reopenErr := h.replaceLocked(absDB); reopenErr != nil {
			return fmt.Errorf("replace database file: %v (reopen: %w)", err, reopenErr)
		}
		return fmt.Errorf("replace database file: %w", err)
	}

	if err := h.replaceLocked(absDB); err != nil {
		if copyErr := copyFile(preRestore, absDB); copyErr == nil {
			_ = h.replaceLocked(absDB)
		}
		return fmt.Errorf("reopen database after restore: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
