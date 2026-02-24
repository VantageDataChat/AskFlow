// Package shop provides shop support module management for the askflow system.
// It handles shop activation, Market service integration, and shop lifecycle management.
package shop

import "time"

// Shop status constants.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// DefaultSoftwareName is the fixed software name used in the main program.
const DefaultSoftwareName = "vantagics"

// Shop represents a shop entity registered in the system.
type Shop struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	OwnerID             int64     `json:"owner_id"`
	StorefrontID        int64     `json:"storefront_id"`
	SoftwareName        string    `json:"software_name"`
	Description         string    `json:"description"`
	WelcomeMessage      string    `json:"welcome_message"`
	Status              string    `json:"status"`
	ParentProductID     string    `json:"parent_product_id"`
	ShopModuleProductID string    `json:"shop_module_product_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ShopActivationRequest represents a shop activation request record.
type ShopActivationRequest struct {
	ID             string    `json:"id"`
	ShopID         string    `json:"shop_id"`
	SoftwareName   string    `json:"software_name"`
	ShopName       string    `json:"shop_name"`
	MarketResponse string    `json:"market_response"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// ActivateRequest is the input payload for the shop activation endpoint.
type ActivateRequest struct {
	OwnerID         int64  `json:"-"`
	SoftwareName    string `json:"software_name"`
	ShopName        string `json:"shop_name"`
	Description     string `json:"description"`
	ParentProductID string `json:"parent_product_id"`
}

// ActivateResponse is the response returned after processing an activation request.
type ActivateResponse struct {
	Shop                *Shop  `json:"shop"`
	Status              string `json:"status"`
	Message             string `json:"message"`
	ShopModuleProductID string `json:"shop_module_product_id,omitempty"`
}

// RegisterRequest is the input payload for POST /api/store-support/register.
// Marketplace calls this to register a store's support system in Service Portal.
type RegisterRequest struct {
	Token           string `json:"token"`
	SoftwareName    string `json:"software_name"`
	StoreName       string `json:"store_name"`
	WelcomeMessage  string `json:"welcome_message"`
	ParentProductID string `json:"parent_product_id"`
}

// RegisterResponse is the response for POST /api/store-support/register.
type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// UpdateWelcomeRequest is the input payload for POST /api/store-support/update-welcome.
type UpdateWelcomeRequest struct {
	StorefrontID   int64  `json:"storefront_id"`
	WelcomeMessage string `json:"welcome_message"`
}

// UpdateWelcomeResponse is the response for POST /api/store-support/update-welcome.
type UpdateWelcomeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// StorefrontCheckResponse represents the response from Marketplace's
// GET /api/storefront-support/check endpoint.
type StorefrontCheckResponse struct {
	Approved       bool   `json:"approved"`
	StoreName      string `json:"store_name,omitempty"`
	WelcomeMessage string `json:"welcome_message,omitempty"`
	SoftwareName   string `json:"software_name,omitempty"`
	Status         string `json:"status,omitempty"`
}
