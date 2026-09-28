package service

import (
	"log"
	"time"
)

// BackupScheduler runs nightly database backups at 01:00 local time.
type BackupScheduler struct {
	backup *BackupService
	logger *log.Logger
	stop   chan struct{}
}

// NewBackupScheduler creates a BackupScheduler.
func NewBackupScheduler(backup *BackupService, logger *log.Logger) *BackupScheduler {
	if logger == nil {
		logger = log.Default()
	}
	return &BackupScheduler{
		backup: backup,
		logger: logger,
		stop:   make(chan struct{}),
	}
}

func nextBackupAt(from time.Time) time.Time {
	loc := from.Location()
	next := time.Date(from.Year(), from.Month(), from.Day(), 1, 0, 0, 0, loc)
	if !next.After(from) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// NextBackupAtForTest exposes nextBackupAt for unit tests.
func NextBackupAtForTest(from time.Time) time.Time {
	return nextBackupAt(from)
}

// Start launches the background scheduler goroutine.
func (s *BackupScheduler) Start() {
	go s.loop()
}

// Stop signals the scheduler to exit.
func (s *BackupScheduler) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *BackupScheduler) loop() {
	for {
		wait := time.Until(nextBackupAt(time.Now()))
		timer := time.NewTimer(wait)
		select {
		case <-s.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
			if err := s.backup.CreateScheduled(); err != nil {
				s.logger.Println("Scheduled backup failed:", err)
			} else {
				s.logger.Println("Scheduled backup completed")
			}
		}
	}
}
