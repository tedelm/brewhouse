package api

// LoginRequest is the JSON body for POST /api/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is returned on successful authentication.
type LoginResponse struct {
	Token      string `json:"token"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	UserID     int64  `json:"user_id"`
	CanElevate bool   `json:"can_elevate"`
}

// ElevateRequest is the body for POST /api/session/elevate.
type ElevateRequest struct {
	Elevated bool `json:"elevated"`
}

// ErrorResponse is a JSON error payload.
type ErrorResponse struct {
	Error string `json:"error"`
}

// VersionResponse is returned by GET /api/version.
type VersionResponse struct {
	Version string `json:"version"`
}

// CreateUserRequest is the body for user create.
type CreateUserRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	BreweryID *int64 `json:"brewery_id"`
}

// UpdateUserRequest is the body for user update.
type UpdateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// SetActiveRequest is the body for PATCH /api/users/{id}/active.
type SetActiveRequest struct {
	Active bool `json:"active"`
}

// UpdateProfileRequest is the body for PATCH /api/me.
type UpdateProfileRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateBreweryRequest is the body for brewery create.
type CreateBreweryRequest struct {
	Name               string `json:"name"`
	ContactName        string `json:"contact_name"`
	ContactEmail       string `json:"contact_email"`
	ContactPhone       string `json:"contact_phone"`
	BreweryAdminUserID *int64 `json:"brewery_admin_user_id"`
}

// MemberRequest is the body for adding a brewery member.
type MemberRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

// InventoryItemRequest is create/update body for inventory.
type InventoryItemRequest struct {
	Category  string  `json:"category"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Qty       float64 `json:"qty"`
	CostPrice float64 `json:"cost_price"`
	Producer  string  `json:"producer"`
	ItemType  string  `json:"item_type"`
	MinEBC    float64 `json:"min_ebc"`
	MaxEBC    float64 `json:"max_ebc"`
	Link      string  `json:"link"`
}

// WishlistRequest creates a wishlist from shortfalls.
type WishlistRequest struct {
	Notes           string `json:"notes"`
	ExternalOrderID string `json:"external_order_id"`
	Lines           []struct {
		InventoryItemID int64   `json:"inventory_item_id"`
		Name            string  `json:"name"`
		Category        string  `json:"category"`
		Missing         float64 `json:"missing"`
		Qty             float64 `json:"qty"`
		BreweryID       *int64  `json:"brewery_id,omitempty"`
	} `json:"lines"`
}

// OrderLineRequest adds a line to a planning order.
type OrderLineRequest struct {
	InventoryItemID int64   `json:"inventory_item_id"`
	Qty             float64 `json:"qty"`
	BreweryID       *int64  `json:"brewery_id,omitempty"`
}

// UpdateOrderLineRequest patches ordered qty and/or product link on a line.
type UpdateOrderLineRequest struct {
	OrderedQty *float64 `json:"ordered_qty,omitempty"`
	Link       *string  `json:"link,omitempty"`
}

// UpdateOrderRequest patches notes, status, and/or external order id.
type UpdateOrderRequest struct {
	Notes           *string `json:"notes,omitempty"`
	Status          *string `json:"status,omitempty"`
	ExternalOrderID *string `json:"external_order_id,omitempty"`
}

// CreateRecipeRequest is the body for recipe create/update.
type CreateRecipeRequest struct {
	BreweryID   int64  `json:"brewery_id"`
	Name        string `json:"name"`
	Ingredients []struct {
		InventoryItemID int64   `json:"inventory_item_id"`
		Qty             float64 `json:"qty"`
		Unit            string  `json:"unit"`
	} `json:"ingredients"`
}

// BrewdayRequest sets OG and brew volume.
type BrewdayRequest struct {
	OG         float64 `json:"og"`
	BrewVolume float64 `json:"brew_volume"`
}

// DeliveryRequest sets FG, delivery volume, beer net SEK/L, and multiplier.
type DeliveryRequest struct {
	FG                 float64 `json:"fg"`
	DeliveryVolume     float64 `json:"delivery_volume"`
	BeerNetSEKPerLiter float64 `json:"beer_net_sek_per_liter"`
	MultiplierID       int64   `json:"multiplier_id"`
}

// HygieneCheckRequest marks a hygiene routine complete.
type HygieneCheckRequest struct {
	RoutineID int64 `json:"routine_id"`
}

// TankRequest is create/update for fermentation tanks.
type TankRequest struct {
	Name           string  `json:"name"`
	CapacityLiters float64 `json:"capacity_liters"`
}

// TaxTierRequest is create/update for tax tiers.
type TaxTierRequest struct {
	MinABV      float64 `json:"min_abv"`
	MaxABV      float64 `json:"max_abv"`
	SEKPerLiter float64 `json:"sek_per_liter"`
}

// TaxConfigRequest is update for Swedish beer tax formula settings.
type TaxConfigRequest struct {
	RateSEK    float64 `json:"rate_sek"`
	FreeMaxABV float64 `json:"free_max_abv"`
	Discount   float64 `json:"discount"`
}

// MultiplierRequest is create/update for price multipliers.
type MultiplierRequest struct {
	Name       string  `json:"name"`
	Multiplier float64 `json:"multiplier"`
}

// ActiveRequest toggles active state for tanks or multipliers.
type ActiveRequest struct {
	Active bool `json:"active"`
}

// BeerPriceRequest is update for minimum net SEK per liter.
type BeerPriceRequest struct {
	MinNetSEKPerLiter float64 `json:"min_net_sek_per_liter"`
}

// HygieneRoutineRequest is create/update for hygiene routines.
type HygieneRoutineRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// BrandColorRequest updates the welcome logo backdrop color.
type BrandColorRequest struct {
	LogoBgHex string `json:"logo_bg_hex"`
}
