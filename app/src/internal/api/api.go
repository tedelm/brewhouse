package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"brewhouse/internal/auth"
	"brewhouse/internal/middleware"
	"brewhouse/internal/service"
	"brewhouse/internal/web"
)

// Handler serves JSON API and HTMX page endpoints.
type Handler struct {
	users      *service.UserService
	breweries  *service.BreweryService
	inventory  *service.InventoryService
	recipes    *service.RecipeService
	schedule   *service.ScheduleService
	settings   *service.SettingsService
	tokens     *auth.TokenIssuer
	web        *web.Handler
	appVersion string
	logger     *log.Logger
}

// Deps holds constructor dependencies for Handler.
type Deps struct {
	Users      *service.UserService
	Breweries  *service.BreweryService
	Inventory  *service.InventoryService
	Recipes    *service.RecipeService
	Schedule   *service.ScheduleService
	Settings   *service.SettingsService
	Tokens     *auth.TokenIssuer
	Web        *web.Handler
	AppVersion string
	Logger     *log.Logger
}

// New creates an API Handler.
func New(d Deps) *Handler {
	return &Handler{
		users:      d.Users,
		breweries:  d.Breweries,
		inventory:  d.Inventory,
		recipes:    d.Recipes,
		schedule:   d.Schedule,
		settings:   d.Settings,
		tokens:     d.Tokens,
		web:        d.Web,
		appVersion: d.AppVersion,
		logger:     d.Logger,
	}
}

func (h *Handler) actor(r *http.Request) (service.Actor, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return service.Actor{}, false
	}
	return service.Actor{UserID: claims.UserID, Role: claims.Role}, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
	case errors.Is(err, service.ErrForbidden):
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "forbidden"})
	case errors.Is(err, service.ErrNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
	case errors.Is(err, service.ErrConflict):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "conflict"})
	case errors.Is(err, service.ErrInsufficientStock):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "insufficient stock"})
	case errors.Is(err, service.ErrInvalidStatus):
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: "invalid status"})
	default:
		h.logger.Println("API error:", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
	}
}

func pathID(path, prefix string) (int64, bool) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// Version returns the app version for WASM stale-reload checks.
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, VersionResponse{Version: h.appVersion})
}

// Login authenticates credentials and returns a JWT.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "username and password required"})
		return
	}

	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	token, err := h.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		h.logger.Println("Failed to issue token:", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:    token,
		Username: user.Username,
		Role:     user.Role,
		UserID:   user.ID,
	})
}
