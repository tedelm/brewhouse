package config

import (
	"os"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	Port         string
	AppVersion   string
	DatabasePath string
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
		appVersion = "dev"
	}

	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "brewhouse.db"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "brewhouse-dev-secret-change-me"
	}

	return &Config{
		Port:         port,
		AppVersion:   appVersion,
		DatabasePath: databasePath,
		JWTSecret:    jwtSecret,
	}
}
