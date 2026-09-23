package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"brewhouse/internal/service"
)

// Recipes handles recipe lifecycle under /api/recipes/...
func (h *Handler) Recipes(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/recipes")
	path = strings.Trim(path, "/")
	parts := splitPath(path)

	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			var breweryID int64
			if v := r.URL.Query().Get("brewery_id"); v != "" {
				id, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid brewery_id"})
					return
				}
				breweryID = id
			}
			list, err := h.recipes.List(actor, breweryID)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req CreateRecipeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			var ings []service.IngredientInput
			for _, i := range req.Ingredients {
				ings = append(ings, service.IngredientInput{InventoryItemID: i.InventoryItemID, Qty: i.Qty, Unit: i.Unit})
			}
			result, err := h.recipes.Create(actor, req.BreweryID, req.Name, ings)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, result)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			recipe, err := h.recipes.Get(actor, id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, recipe)
		case http.MethodPut, http.MethodPatch:
			var req CreateRecipeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			var ings []service.IngredientInput
			for _, i := range req.Ingredients {
				ings = append(ings, service.IngredientInput{InventoryItemID: i.InventoryItemID, Qty: i.Qty, Unit: i.Unit})
			}
			result, err := h.recipes.Update(actor, id, req.Name, ings)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
		case http.MethodDelete:
			if err := h.recipes.Delete(actor, id); err != nil {
				h.writeErr(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	switch parts[1] {
	case "schedule":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		var req service.BookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		req.RecipeID = id
		if err := h.schedule.Book(actor, req); err != nil {
			h.writeErr(w, err)
			return
		}
		recipe, err := h.recipes.Get(actor, id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
	case "brewday":
		if r.Method != http.MethodPost && r.Method != http.MethodPatch {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		var req BrewdayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		recipe, err := h.recipes.SetBrewday(actor, id, req.OG, req.BrewVolume)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
	case "hygiene":
		h.recipeHygiene(w, r, actor, id, parts[2:])
	case "delivery":
		if len(parts) >= 3 && parts[2] == "revoke" {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
				return
			}
			recipe, err := h.recipes.RevokeDelivery(actor, id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, recipe)
			return
		}
		if r.Method != http.MethodPost && r.Method != http.MethodPatch {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		var req DeliveryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		recipe, err := h.recipes.SetDelivery(actor, id, req.FG, req.DeliveryVolume)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
	case "deliver":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		recipe, err := h.recipes.Deliver(actor, id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
	}
}

func (h *Handler) recipeHygiene(w http.ResponseWriter, r *http.Request, actor service.Actor, recipeID int64, parts []string) {
	if len(parts) == 1 && parts[0] == "complete" && r.Method == http.MethodPost {
		recipe, err := h.recipes.CompleteAllHygiene(actor, recipeID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
		return
	}
	if len(parts) == 0 && r.Method == http.MethodGet {
		routines, done, err := h.recipes.HygieneStatus(actor, recipeID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		type row struct {
			service.HygieneRoutine
			Done bool `json:"done"`
		}
		var out []row
		for _, rt := range routines {
			out = append(out, row{HygieneRoutine: rt, Done: done[rt.ID]})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if len(parts) == 0 && (r.Method == http.MethodPost || r.Method == http.MethodPatch) {
		var req HygieneCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		recipe, err := h.recipes.CompleteHygiene(actor, recipeID, req.RoutineID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recipe)
		return
	}
	writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
}

// Schedule handles /api/schedule availability and bookings list.
func (h *Handler) Schedule(w http.ResponseWriter, r *http.Request) {
	_, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/schedule")
	path = strings.Trim(path, "/")

	if path == "availability" && r.Method == http.MethodGet {
		date := r.URL.Query().Get("date")
		okAvail, err := h.schedule.IsDateAvailable(date)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"available": okAvail})
		return
	}

	if path == "" && r.Method == http.MethodGet {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from == "" {
			from = time.Now().Format("2006-01-02")
		}
		if to == "" {
			to = time.Now().AddDate(0, 1, 0).Format("2006-01-02")
		}
		list, err := h.schedule.ListBookings(from, to)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
}

// Settings handles tanks, tax tiers, multipliers, hygiene under /api/settings/...
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/settings")
	path = strings.Trim(path, "/")
	parts := splitPath(path)
	if len(parts) == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	switch parts[0] {
	case "tanks":
		h.settingsTanks(w, r, actor, parts[1:])
	case "tax-tiers":
		h.settingsTax(w, r, actor, parts[1:])
	case "tax-config":
		h.settingsTaxConfig(w, r, actor, parts[1:])
	case "multipliers":
		h.settingsMultipliers(w, r, actor, parts[1:])
	case "beer-price":
		h.settingsBeerPrice(w, r, actor, parts[1:])
	case "hygiene-routines":
		h.settingsHygiene(w, r, actor, parts[1:])
	default:
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
	}
}

func (h *Handler) settingsTanks(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.settings.ListTanks()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req TankRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			t, err := h.settings.CreateTank(actor, req.Name, req.CapacityLiters)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, t)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var req TankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		t, err := h.settings.UpdateTank(actor, id, req.Name, req.CapacityLiters)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if err := h.settings.DeleteTank(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) settingsTax(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.settings.ListTaxTiers()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req TaxTierRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			t, err := h.settings.CreateTaxTier(actor, req.MinABV, req.MaxABV, req.SEKPerLiter)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, t)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var req TaxTierRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		t, err := h.settings.UpdateTaxTier(actor, id, req.MinABV, req.MaxABV, req.SEKPerLiter)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if err := h.settings.DeleteTaxTier(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) settingsTaxConfig(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) != 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.settings.GetAlcoholTaxConfig()
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut, http.MethodPatch:
		var req TaxConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		cfg, err := h.settings.UpdateAlcoholTaxConfig(actor, req.RateSEK, req.FreeMaxABV, req.Discount)
		if err != nil {
			if errors.Is(err, service.ErrForbidden) {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) settingsBeerPrice(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) != 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.settings.GetBeerPriceConfig()
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut, http.MethodPatch:
		var req BeerPriceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		cfg, err := h.settings.UpdateBeerPriceConfig(actor, req.MinNetSEKPerLiter)
		if err != nil {
			if errors.Is(err, service.ErrForbidden) {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) settingsMultipliers(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.settings.ListMultipliers()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req MultiplierRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			m, err := h.settings.CreateMultiplier(actor, req.Name, req.Multiplier)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, m)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var req MultiplierRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		m, err := h.settings.UpdateMultiplier(actor, id, req.Name, req.Multiplier)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	case http.MethodDelete:
		if err := h.settings.DeleteMultiplier(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) settingsHygiene(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.settings.ListHygieneRoutines()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req HygieneRoutineRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			rt, err := h.settings.CreateHygieneRoutine(actor, req.Name, req.Description, req.SortOrder)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, rt)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var req HygieneRoutineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		rt, err := h.settings.UpdateHygieneRoutine(actor, id, req.Name, req.Description, req.SortOrder)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rt)
	case http.MethodDelete:
		if err := h.settings.DeleteHygieneRoutine(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}
