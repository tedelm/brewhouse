package router

import (
	"net/http"

	"brewhouse/internal/api"
	"brewhouse/internal/auth"
	"brewhouse/internal/middleware"
	"brewhouse/internal/web"
)

// New wires HTTP routes for the web shell, static assets, and API.
func New(webHandler *web.Handler, apiHandler *api.Handler, tokens *auth.TokenIssuer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", webHandler.Index)
	mux.Handle("/static/", webHandler.Static())

	mux.HandleFunc("/api/login", apiHandler.Login)
	mux.HandleFunc("/api/version", apiHandler.Version)
	mux.HandleFunc("/logo", apiHandler.Logo)
	mux.HandleFunc("/favicon", apiHandler.Favicon)

	authMW := middleware.Auth(tokens)
	protect := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authMW(h))
	}

	protect("/api/me", apiHandler.Me)
	protect("/api/session/elevate", apiHandler.Elevate)
	protect("/api/users", apiHandler.Users)
	protect("/api/users/", apiHandler.Users)
	protect("/api/breweries", apiHandler.Breweries)
	protect("/api/breweries/", apiHandler.Breweries)
	protect("/api/inventory", apiHandler.Inventory)
	protect("/api/inventory/", apiHandler.Inventory)
	protect("/api/recipes", apiHandler.Recipes)
	protect("/api/recipes/", apiHandler.Recipes)
	protect("/api/schedule", apiHandler.Schedule)
	protect("/api/schedule/", apiHandler.Schedule)
	protect("/api/settings/", apiHandler.Settings)

	protect("/app/", apiHandler.Pages)

	return mux
}
