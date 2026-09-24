package service

// Role constants for global and brewery membership roles.
const (
	RoleAdmin        = "admin"
	RoleBreweryAdmin = "brewery_admin"
	RoleSuperuser    = "superuser"
	RoleUser         = "user"
)

// Recipe status constants.
const (
	StatusCreated          = "created"
	StatusScheduled        = "scheduled"
	StatusBrewday          = "brewday"
	StatusHygieneDone      = "hygiene_done"
	StatusReadyForDelivery = "ready_for_delivery"
	StatusDelivered        = "delivered"
)

// Inventory category constants.
const (
	CategoryMalt  = "malt"
	CategoryHops  = "hops"
	CategoryYeast = "yeast"
	CategoryMisc  = "misc"
)

// Ingredient fulfillment status for a recipe or line.
const (
	IngredientStatusOK        = "ok"
	IngredientStatusPartial   = "partial"
	IngredientStatusShort     = "short"
	IngredientStatusCompleted = "completed"
)

// Inventory order status constants.
const (
	OrderStatusPlanning  = "planning"
	OrderStatusPaused    = "paused"
	OrderStatusOrdered   = "ordered"
	OrderStatusCompleted = "completed"
)

// User is a persisted application user.
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// Brewery is a sub-brewery under the shared inventory umbrella.
type Brewery struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ContactName  string `json:"contact_name"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// BreweryMember links a user to a brewery with a membership role.
type BreweryMember struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username,omitempty"`
	BreweryID int64  `json:"brewery_id"`
	Role      string `json:"role"`
}

// InventoryItem is a shared stock line.
type InventoryItem struct {
	ID        int64   `json:"id"`
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

// InventoryOrder is a purchase / wishlist order.
type InventoryOrder struct {
	ID              int64                `json:"id"`
	Status          string               `json:"status"`
	Notes           string               `json:"notes"`
	ExternalOrderID string               `json:"external_order_id"`
	CreatedBy       int64                `json:"created_by"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
	OrderedAt       *string              `json:"ordered_at,omitempty"`
	Lines           []InventoryOrderLine `json:"lines,omitempty"`
}

// InventoryOrderLine is one order line.
type InventoryOrderLine struct {
	ID              int64    `json:"id"`
	OrderID         int64    `json:"order_id"`
	InventoryItemID *int64   `json:"inventory_item_id,omitempty"`
	ItemName        string   `json:"item_name"`
	Category        string   `json:"category"`
	Qty             float64  `json:"qty"`
	OrderedQty      *float64 `json:"ordered_qty,omitempty"`
	Unit            string   `json:"unit"`
	Link            string   `json:"link,omitempty"`
	BreweryID       *int64   `json:"brewery_id,omitempty"`
	BreweryName     string   `json:"brewery_name,omitempty"`
	RecipeID        *int64   `json:"recipe_id,omitempty"`
}

// OrderLineInput adds a catalog item to a planning order.
type OrderLineInput struct {
	InventoryItemID int64   `json:"inventory_item_id"`
	Qty             float64 `json:"qty"`
	BreweryID       *int64  `json:"brewery_id,omitempty"`
}

// FermentationTank is a fermenter used in scheduling.
type FermentationTank struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	CapacityLiters float64 `json:"capacity_liters"`
	Active         bool    `json:"active"`
}

// AlcoholTaxTier is a legacy ABV tax band (SEK/liter); unused by delivery formula.
type AlcoholTaxTier struct {
	ID          int64   `json:"id"`
	MinABV      float64 `json:"min_abv"`
	MaxABV      float64 `json:"max_abv"`
	SEKPerLiter float64 `json:"sek_per_liter"`
}

// AlcoholTaxConfig is Swedish beer tax: SEK/L = 0 if ABV <= FreeMaxABV, else ABV × RateSEK × Discount.
type AlcoholTaxConfig struct {
	RateSEK    float64 `json:"rate_sek"`
	FreeMaxABV float64 `json:"free_max_abv"`
	Discount   float64 `json:"discount"`
}

// BeerPriceConfig holds the minimum net sale price per liter (SEK).
type BeerPriceConfig struct {
	MinNetSEKPerLiter float64 `json:"min_net_sek_per_liter"`
}

// BrandImageMeta reports whether a custom brand asset is stored.
type BrandImageMeta struct {
	Configured bool `json:"configured"`
}

// BrandColorConfig holds the welcome logo backdrop color.
type BrandColorConfig struct {
	LogoBgHex string `json:"logo_bg_hex"`
}

// PriceMultiplier scales (cost+tax) to net price.
type PriceMultiplier struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Multiplier float64 `json:"multiplier"`
	Active     bool    `json:"active"`
}

// HygieneRoutine is a brewday checklist item.
type HygieneRoutine struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// RecipeIngredient is a stock line on a recipe/batch.
type RecipeIngredient struct {
	ID              int64   `json:"id"`
	RecipeID        int64   `json:"recipe_id"`
	InventoryItemID int64   `json:"inventory_item_id"`
	ItemName        string  `json:"item_name,omitempty"`
	Category        string  `json:"category,omitempty"`
	Qty             float64 `json:"qty"`
	Unit            string  `json:"unit"`
	CheckedOut      float64 `json:"checked_out"`
	CostPrice       float64 `json:"cost_price"`
}

// Recipe is a one-shot brew/batch.
type Recipe struct {
	ID               int64              `json:"id"`
	BreweryID        int64              `json:"brewery_id"`
	BreweryName      string             `json:"brewery_name,omitempty"`
	Name             string             `json:"name"`
	Status           string             `json:"status"`
	BookedDate       *string            `json:"booked_date,omitempty"`
	TankID           *int64             `json:"tank_id,omitempty"`
	OG               *float64           `json:"og,omitempty"`
	FG               *float64           `json:"fg,omitempty"`
	BrewVolume       *float64           `json:"brew_volume,omitempty"`
	DeliveryVolume   *float64           `json:"delivery_volume,omitempty"`
	Cost             *float64           `json:"cost,omitempty"`
	Tax              *float64           `json:"tax,omitempty"`
	Net              *float64           `json:"net,omitempty"`
	CreatedBy        *int64             `json:"created_by,omitempty"`
	CreatedAt        string             `json:"created_at"`
	DeliveredAt      *string            `json:"delivered_at,omitempty"`
	Active           bool               `json:"active"`
	Ingredients      []RecipeIngredient `json:"ingredients,omitempty"`
	IngredientStatus string             `json:"ingredient_status,omitempty"`
	HygieneComplete  bool               `json:"hygiene_complete,omitempty"`
}

// IngredientInput is a requested amount for recipe create/update.
type IngredientInput struct {
	InventoryItemID int64   `json:"inventory_item_id"`
	Qty             float64 `json:"qty"`
	Unit            string  `json:"unit"`
}

// StockShortfall describes a missing amount for wishlist prompting.
type StockShortfall struct {
	InventoryItemID int64   `json:"inventory_item_id"`
	Name            string  `json:"name"`
	Category        string  `json:"category"`
	Requested       float64 `json:"requested"`
	Available       float64 `json:"available"`
	Missing         float64 `json:"missing"`
	BreweryID       *int64  `json:"brewery_id,omitempty"`
}

// Actor is the authenticated caller used for authorization.
type Actor struct {
	UserID int64
	Role   string
}
