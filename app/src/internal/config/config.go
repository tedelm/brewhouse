package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Version is the build-time app version, set via:
//
//	-ldflags "-X brewhouse/internal/config.Version=…"
//
// Default "dev" means no ldflags was used (e.g. go run).
var Version = "dev"

// Config holds process configuration loaded from the environment.
type Config struct {
	Port         string
	AppVersion   string
	DatabasePath string
	BackupDir    string
	JWTSecret    string
}

// Load reads configuration from environment variables with sensible defaults.
// An optional .env file is loaded first to fill missing keys; variables already
// set in the process environment (e.g. Azure App Settings) are never overridden.
func Load() *Config {
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("../.env")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
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
		AppVersion:   resolveAppVersion(),
		DatabasePath: databasePath,
		BackupDir:    backupDir,
		JWTSecret:    jwtSecret,
	}
}

// resolveAppVersion prefers build-time Version, then APP_VERSION env, then VERSION file, else "dev".
func resolveAppVersion() string {
	if v := strings.TrimSpace(Version); v != "" && v != "dev" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("APP_VERSION")); v != "" {
		return v
	}
	if v := readVersionFile(); v != "" {
		return v
	}
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	return "dev"
}

func readVersionFile() string {
	for _, path := range []string{"VERSION", "../VERSION"} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v
		}
	}
	return ""
}
