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
		writeJSON(w, http.StatusConflict, ErrorResponse{Error: err.Error()})
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

// Bootstrap returns one-time first-boot admin credentials when still pending.
func (h *Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	username, password, pending, err := h.users.BootstrapCredentials()
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !pending {
		writeJSON(w, http.StatusOK, BootstrapResponse{Pending: false})
		return
	}
	writeJSON(w, http.StatusOK, BootstrapResponse{
		Pending:  true,
		Username: username,
		Password: password,
	})
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
	if err := h.users.ClearBootstrapCredentialsIfMatch(user.Username); err != nil {
		h.logger.Println("Failed to clear bootstrap credentials:", err)
	}

	role, canElevate := sessionRoleForAccount(user.Role)
	token, err := h.tokens.Issue(user.ID, user.Username, role, canElevate)
	if err != nil {
		h.logger.Println("Failed to issue token:", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:      token,
		Username:   user.Username,
		Role:       role,
		UserID:     user.ID,
		CanElevate: canElevate,
	})
}

// sessionRoleForAccount returns the effective JWT role and elevate flag for a DB account role.
func sessionRoleForAccount(accountRole string) (role string, canElevate bool) {
	if accountRole == service.RoleAdmin {
		return service.RoleSuperuser, true
	}
	return accountRole, false
}

// Elevate re-issues a JWT with admin or superuser role for accounts that may elevate.
func (h *Handler) Elevate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	var req ElevateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
		return
	}

	user, err := h.users.Get(actor.UserID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if user.Role != service.RoleAdmin {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "forbidden"})
		return
	}

	role := service.RoleSuperuser
	if req.Elevated {
		role = service.RoleAdmin
	}
	token, err := h.tokens.Issue(user.ID, user.Username, role, true)
	if err != nil {
		h.logger.Println("Failed to issue token:", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:      token,
		Username:   user.Username,
		Role:       role,
		UserID:     user.ID,
		CanElevate: true,
	})
}

// Refresh re-issues a JWT after re-checking the account is active.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	claims, _ := middleware.ClaimsFromContext(r.Context())

	user, err := h.users.Get(actor.UserID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !user.Active {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	role, canElevate := sessionRoleForAccount(user.Role)
	if user.Role == service.RoleAdmin && claims != nil && claims.Role == service.RoleAdmin {
		role = service.RoleAdmin
		canElevate = true
	}

	token, err := h.tokens.Issue(user.ID, user.Username, role, canElevate)
	if err != nil {
		h.logger.Println("Failed to issue token:", err)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:      token,
		Username:   user.Username,
		Role:       role,
		UserID:     user.ID,
		CanElevate: canElevate,
	})
}

// Logo serves the app brand logo (public).
func (h *Handler) Logo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	h.serveBrandImage(w, h.settings.GetLogo, "images/cb.png", "image/png")
}

// Favicon serves the app favicon (public).
func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	h.serveBrandImage(w, h.settings.GetFavicon, "images/favico_cb.png", "image/png")
}

// BreweryLogo serves a brewery logo by id (public). Returns 404 when not configured.
func (h *Handler) BreweryLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/brewery/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "logo" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	contentType, data, ok, err := h.breweries.GetLogo(id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) serveBrandImage(
	w http.ResponseWriter,
	get func() (string, []byte, bool, error),
	fallbackPath, fallbackType string,
) {
	contentType, data, ok, err := get()
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !ok {
		data, err = h.web.ReadStatic(fallbackPath)
		if err != nil {
			h.logger.Println("Failed to read default brand image:", err)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		contentType = fallbackType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
