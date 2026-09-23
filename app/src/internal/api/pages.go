package api

import (
	"net/http"
	"strings"
)

// Pages serves HTMX HTML fragments under /app/*.
func (h *Handler) Pages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, ok := h.actor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/app/")
	path = strings.Trim(path, "/")

	switch path {
	case "recipes":
		h.web.RenderPartial(w, "partial_recipes.html", nil)
	case "schedule":
		h.web.RenderPartial(w, "partial_schedule.html", nil)
	case "brewday":
		h.web.RenderPartial(w, "partial_brewday.html", nil)
	case "guide", "brewery-101":
		h.web.RenderPartial(w, "partial_guide.html", nil)
	case "inventory/malt":
		h.web.RenderPartial(w, "partial_inventory.html", map[string]string{"Category": "malt", "Title": "Malt"})
	case "inventory/hops":
		h.web.RenderPartial(w, "partial_inventory.html", map[string]string{"Category": "hops", "Title": "Hops"})
	case "inventory/yeast":
		h.web.RenderPartial(w, "partial_inventory.html", map[string]string{"Category": "yeast", "Title": "Yeast"})
	case "inventory/misc":
		h.web.RenderPartial(w, "partial_inventory.html", map[string]string{"Category": "misc", "Title": "Misc"})
	case "inventory/orders":
		h.web.RenderPartial(w, "partial_orders.html", nil)
	case "hygiene":
		h.web.RenderPartial(w, "partial_hygiene.html", nil)
	case "economy":
		h.web.RenderPartial(w, "partial_economy.html", nil)
	case "economy/deliveries":
		h.web.RenderPartial(w, "partial_economy_deliveries.html", nil)
	case "delivery":
		h.web.RenderPartial(w, "partial_delivery.html", nil)
	case "iam", "iam/users":
		h.web.RenderPartial(w, "partial_iam_users.html", nil)
	case "iam/breweries":
		h.web.RenderPartial(w, "partial_iam_breweries.html", nil)
	case "settings", "settings/brand":
		h.web.RenderPartial(w, "partial_settings_brand.html", nil)
	case "settings/tanks":
		h.web.RenderPartial(w, "partial_settings_tanks.html", nil)
	case "settings/multipliers":
		h.web.RenderPartial(w, "partial_settings_multipliers.html", nil)
	case "settings/beer-price":
		h.web.RenderPartial(w, "partial_settings_beer_price.html", nil)
	case "settings/hygiene":
		h.web.RenderPartial(w, "partial_settings_hygiene.html", nil)
	default:
		http.NotFound(w, r)
	}
}
