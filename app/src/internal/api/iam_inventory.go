package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"brewhouse/internal/service"
)

// Users handles /api/users and /api/users/{id}.
func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	if !actor.IsAdmin() {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "forbidden"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/users")
	path = strings.Trim(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			users, err := h.users.List()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, users)
		case http.MethodPost:
			var req CreateUserRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			user, err := h.users.Create(req.Username, req.Password, req.Email, req.Role)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
				return
			}
			if req.BreweryID != nil && *req.BreweryID > 0 {
				memberRole := service.MembershipRoleForCreate(req.Role)
				if err := h.breweries.AddMember(actor, *req.BreweryID, user.ID, memberRole); err != nil {
					h.writeErr(w, err)
					return
				}
			}
			writeJSON(w, http.StatusCreated, user)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	parts := splitPath(path)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	if len(parts) == 2 && parts[1] == "active" {
		if r.Method != http.MethodPatch && r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		var req SetActiveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		if !req.Active && actor.UserID == id {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "cannot deactivate your own account"})
			return
		}
		user, err := h.users.SetActive(id, req.Active)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
		return
	}

	if len(parts) != 1 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		user, err := h.users.Get(id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPut, http.MethodPatch:
		var req UpdateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		if actor.UserID == id && req.Role != service.RoleAdmin {
			existing, err := h.users.Get(id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			if existing.Role == service.RoleAdmin {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "cannot change your own role away from admin"})
				return
			}
		}
		user, err := h.users.Update(id, req.Username, req.Password, req.Email, req.Role)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodDelete:
		if err := h.users.Delete(id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

// Breweries handles brewery CRUD and membership under /api/breweries/...
func (h *Handler) Breweries(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/breweries")
	path = strings.Trim(path, "/")
	parts := splitPath(path)

	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.breweries.List(actor)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var req CreateBreweryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			b, err := h.breweries.Create(actor, req.Name, req.ContactName, req.ContactEmail, req.ContactPhone, req.BreweryAdminUserID)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, b)
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
			b, err := h.breweries.Get(actor, id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, b)
		case http.MethodPut, http.MethodPatch:
			var req CreateBreweryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			b, err := h.breweries.Update(actor, id, req.Name, req.ContactName, req.ContactEmail, req.ContactPhone, req.BreweryAdminUserID)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, b)
		case http.MethodDelete:
			if err := h.breweries.Delete(actor, id); err != nil {
				h.writeErr(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	if parts[1] == "members" {
		if len(parts) == 2 && r.Method == http.MethodGet {
			members, err := h.breweries.ListMembers(actor, id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, members)
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPost {
			var req MemberRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			if err := h.breweries.AddMember(actor, id, req.UserID, req.Role); err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
			return
		}
		if len(parts) == 3 && r.Method == http.MethodDelete {
			uid, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid user id"})
				return
			}
			if err := h.breweries.RemoveMember(actor, id, uid); err != nil {
				h.writeErr(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// Inventory handles /api/inventory and wishlist orders.
func (h *Handler) Inventory(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/inventory")
	path = strings.Trim(path, "/")
	parts := splitPath(path)

	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			cat := r.URL.Query().Get("category")
			var items []service.InventoryItem
			var err error
			if cat != "" {
				items, err = h.inventory.ListByCategory(cat)
			} else {
				items, err = h.inventory.ListAll()
			}
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var req InventoryItemRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			item, err := h.inventory.Create(actor, service.InventoryItem{
				Category:  req.Category,
				Name:      req.Name,
				Unit:      req.Unit,
				Qty:       req.Qty,
				CostPrice: req.CostPrice,
				Producer:  req.Producer,
				ItemType:  req.ItemType,
				MinEBC:    req.MinEBC,
				MaxEBC:    req.MaxEBC,
				Link:      req.Link,
			})
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, item)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	if parts[0] == "orders" {
		h.inventoryOrders(w, r, actor, parts[1:])
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := h.inventory.Get(id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodPut, http.MethodPatch:
		var req InventoryItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		item, err := h.inventory.Update(actor, id, service.InventoryItem{
			Name:      req.Name,
			Unit:      req.Unit,
			Qty:       req.Qty,
			CostPrice: req.CostPrice,
			Producer:  req.Producer,
			ItemType:  req.ItemType,
			MinEBC:    req.MinEBC,
			MaxEBC:    req.MaxEBC,
			Link:      req.Link,
		})
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := h.inventory.Delete(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}

func (h *Handler) inventoryOrders(w http.ResponseWriter, r *http.Request, actor service.Actor, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			orders, err := h.inventory.ListOrders()
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, orders)
		case http.MethodPost:
			var req WishlistRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			isWishlist := false
			for _, l := range req.Lines {
				if l.Missing > 0 {
					isWishlist = true
					break
				}
			}
			if isWishlist {
				var lines []service.StockShortfall
				for _, l := range req.Lines {
					lines = append(lines, service.StockShortfall{
						InventoryItemID: l.InventoryItemID,
						Name:            l.Name,
						Category:        l.Category,
						Missing:         l.Missing,
						BreweryID:       l.BreweryID,
					})
				}
				order, err := h.inventory.CreateWishlistOrder(actor, req.Notes, lines)
				if err != nil {
					h.writeErr(w, err)
					return
				}
				writeJSON(w, http.StatusCreated, order)
				return
			}
			var lines []service.OrderLineInput
			for _, l := range req.Lines {
				qty := l.Qty
				if qty <= 0 {
					continue
				}
				lines = append(lines, service.OrderLineInput{
					InventoryItemID: l.InventoryItemID,
					Qty:             qty,
					BreweryID:       l.BreweryID,
				})
			}
			order, err := h.inventory.CreateOrder(actor, req.Notes, req.ExternalOrderID, lines)
			if err != nil {
				h.writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, order)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		}
		return
	}

	if parts[0] == "planning" && len(parts) == 2 && parts[1] == "lines" && r.Method == http.MethodPost {
		var req OrderLineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		order, err := h.inventory.EnsurePlanningOrder(actor)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		order, err = h.inventory.AddOrderLine(actor, order.ID, req.InventoryItemID, req.Qty, req.BreweryID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	if len(parts) >= 2 && parts[1] == "lines" {
		if len(parts) >= 3 {
			lineID, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid line id"})
				return
			}
			if r.Method != http.MethodPatch && r.Method != http.MethodPut {
				writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
				return
			}
			var req UpdateOrderLineRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
				return
			}
			if req.Link == nil && req.OrderedQty == nil {
				writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ordered_qty or link required"})
				return
			}
			var order *service.InventoryOrder
			if req.Link != nil {
				order, err = h.inventory.UpdateOrderLineProductLink(actor, id, lineID, *req.Link)
				if err != nil {
					h.writeErr(w, err)
					return
				}
			}
			if req.OrderedQty != nil {
				order, err = h.inventory.UpdateOrderLineOrderedQty(actor, id, lineID, *req.OrderedQty)
				if err != nil {
					h.writeErr(w, err)
					return
				}
			}
			writeJSON(w, http.StatusOK, order)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
			return
		}
		var req OrderLineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		order, err := h.inventory.AddOrderLine(actor, id, req.InventoryItemID, req.Qty, req.BreweryID)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	switch r.Method {
	case http.MethodGet:
		order, err := h.inventory.GetOrder(id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	case http.MethodPatch, http.MethodPut:
		var req UpdateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
			return
		}
		var order *service.InventoryOrder
		if req.Notes != nil || req.ExternalOrderID != nil {
			order, err = h.inventory.UpdateOrder(actor, id, req.Notes, req.ExternalOrderID)
			if err != nil {
				h.writeErr(w, err)
				return
			}
		}
		if req.Status != nil {
			order, err = h.inventory.SetOrderStatus(actor, id, *req.Status)
			if err != nil {
				h.writeErr(w, err)
				return
			}
		}
		if order == nil {
			order, err = h.inventory.GetOrder(id)
			if err != nil {
				h.writeErr(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, order)
	case http.MethodDelete:
		if err := h.inventory.DeleteOrder(actor, id); err != nil {
			h.writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
	}
}
