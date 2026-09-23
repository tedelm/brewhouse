package main

import (
	"log"
	"net/http"
	"time"

	"brewhouse/internal/api"
	"brewhouse/internal/auth"
	"brewhouse/internal/config"
	"brewhouse/internal/database"
	"brewhouse/internal/router"
	"brewhouse/internal/service"
	"brewhouse/internal/web"
)

func main() {
	logger := log.Default()

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Println("Failed to open database:", err)
		return
	}
	defer db.Close()

	// Initialize services
	access := service.NewAccessService(db)
	users := service.NewUserService(db)
	if err := users.EnsureDemoUser(); err != nil {
		logger.Println("Failed to ensure demo user:", err)
		return
	}
	breweries := service.NewBreweryService(db, access)
	inventory := service.NewInventoryService(db, access)
	settings := service.NewSettingsService(db, access)
	recipes := service.NewRecipeService(db, access, inventory, settings)
	schedule := service.NewScheduleService(db, access)

	tokens := auth.NewTokenIssuer(cfg.JWTSecret, 24*time.Hour)

	// Initialize web layer
	webHandler, err := web.New(cfg.AppVersion)
	if err != nil {
		logger.Println("Failed to initialize web handler:", err)
		return
	}

	apiHandler := api.New(api.Deps{
		Users:      users,
		Breweries:  breweries,
		Inventory:  inventory,
		Recipes:    recipes,
		Schedule:   schedule,
		Settings:   settings,
		Tokens:     tokens,
		Web:        webHandler,
		AppVersion: cfg.AppVersion,
		Logger:     logger,
	})

	// Setup routes
	handler := router.New(webHandler, apiHandler, tokens)

	addr := ":" + cfg.Port
	logger.Println("Listening on", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Println("Failed to start server:", err)
		return
	}
}
