package service_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"brewhouse/internal/database"
	"brewhouse/internal/service"
)

func TestBackupService_CreateListPruneRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	backupDir := filepath.Join(dir, "backups")

	db, err := database.OpenHolder(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	access := service.NewAccessService(db)
	users := service.NewUserService(db)
	admin := ensureAdminActor(t, users)
	backup := service.NewBackupService(db, access, backupDir)

	info, err := backup.Create(admin, service.BackupKindManual)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if info.Kind != service.BackupKindManual {
		t.Fatalf("kind: %s", info.Kind)
	}

	list, err := backup.List(admin)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len: %d", len(list))
	}

	src := filepath.Join(backupDir, info.Name)
	for i := 0; i < 9; i++ {
		ts := time.Date(2020, 1, 1+i, 1, 0, 0, 0, time.Local)
		dest := filepath.Join(backupDir, "brewhouse-"+ts.Format("20060102-150405")+"-daily.db")
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read src: %v", err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := backup.Prune(admin); err != nil {
		t.Fatalf("prune: %v", err)
	}
	list, err = backup.List(admin)
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	dailyCount := 0
	for _, b := range list {
		if b.Kind == service.BackupKindDaily {
			dailyCount++
		}
	}
	if dailyCount != 7 {
		t.Fatalf("expected 7 daily after prune, got %d (total %d)", dailyCount, len(list))
	}

	if _, err := db.Exec(`UPDATE users SET email = ? WHERE username = ?`, "changed@example.com", "admin"); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if err := backup.RestoreFromStored(admin, info.Name); err != nil {
		t.Fatalf("restore: %v", err)
	}
	var email string
	if err := db.QueryRow(`SELECT email FROM users WHERE username = ?`, "admin").Scan(&email); err != nil {
		t.Fatalf("query after restore: %v", err)
	}
	if email == "changed@example.com" {
		t.Fatalf("restore did not revert email, got %q", email)
	}
}

func TestBackupScheduler_NextBackupAt(t *testing.T) {
	loc := time.Local
	from := time.Date(2026, 3, 15, 0, 30, 0, 0, loc)
	next := service.NextBackupAtForTest(from)
	want := time.Date(2026, 3, 15, 1, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
	from = time.Date(2026, 3, 15, 1, 0, 0, 0, loc)
	next = service.NextBackupAtForTest(from)
	want = time.Date(2026, 3, 16, 1, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}
