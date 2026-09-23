package api

import (
	"encoding/json"
	"net/http"
)

// Me handles GET/PATCH /api/me for the authenticated user.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		user, err := h.users.Get(actor.UserID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPatch, http.MethodPut:
		var req UpdateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		user, err := h.users.UpdateProfile(actor.UserID, req.Email, req.Password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, user)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}
