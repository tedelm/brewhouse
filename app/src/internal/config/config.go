package config

import (
	"os"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	Port         string
	AppVersion   string
	DatabasePath string
	BackupDir    string
	JWTSecret    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appVersion := os.Getenv("APP_VERSION")
	if appVersion == "" {
		appVersion = "0.5.1"
	}

	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "brewhouse.db"
	}

	backupDir := os.Getenv("BACKUP_DIR")
	if backupDir == "" {
		backupDir = "backups"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "brewhouse-dev-secret-change-me"
	}

	return &Config{
		Port:         port,
		AppVersion:   appVersion,
		DatabasePath: databasePath,
		BackupDir:    backupDir,
		JWTSecret:    jwtSecret,
	}
}
