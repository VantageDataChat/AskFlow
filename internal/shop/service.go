package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"askflow/internal/product"
)

// ShopService provides shop management business logic.
type ShopService struct {
	readDB       *sql.DB
	writeDB      *sql.DB
	marketClient *MarketClient
	productSvc   *product.ProductService
}

// NewShopService creates a new ShopService with the given dependencies.
func NewShopService(readDB, writeDB *sql.DB, marketClient *MarketClient, productSvc *product.ProductService) *ShopService {
	return &ShopService{
		readDB:       readDB,
		writeDB:      writeDB,
		marketClient: marketClient,
		productSvc:   productSvc,
	}
}

// generateID creates a random hex string for use as a unique identifier.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Activate processes a shop activation request.
// It validates input, checks for duplicate shops, creates DB records,
// queries Market service for approval, and generates a child product if approved.
func (s *ShopService) Activate(req ActivateRequest) (*ActivateResponse, error) {
	// 1. Validate required fields
	softwareName := strings.TrimSpace(req.SoftwareName)
	shopName := strings.TrimSpace(req.ShopName)
	if softwareName == "" || shopName == "" {
		return nil, fmt.Errorf("software_name and shop_name are required")
	}

	// Enforce software_name invariant: always use DefaultSoftwareName
	softwareName = DefaultSoftwareName

	// 2. Check for duplicate: if owner already has a shop, return existing info
	existingShop, err := s.getByOwnerID(req.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing shop: %w", err)
	}
	if existingShop != nil {
		return &ActivateResponse{
			Shop:                existingShop,
			Status:              existingShop.Status,
			Message:             "shop already exists",
			ShopModuleProductID: existingShop.ShopModuleProductID,
		}, nil
	}

	// 3. Generate IDs
	shopID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate shop ID: %w", err)
	}
	requestID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate request ID: %w", err)
	}

	now := time.Now()

	// 4. Insert shop record with pending status
	_, err = s.writeDB.Exec(
		`INSERT INTO shops (id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, '', ?, ?, '', ?, ?)`,
		shopID, shopName, req.OwnerID, softwareName, req.Description, StatusPending, req.ParentProductID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create shop record: %w", err)
	}

	// 5. Insert activation request record
	_, err = s.writeDB.Exec(
		`INSERT INTO shop_activation_requests (id, shop_id, software_name, shop_name, market_response, status, created_at)
		 VALUES (?, ?, ?, ?, '', ?, ?)`,
		requestID, shopID, softwareName, shopName, StatusPending, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create activation request: %w", err)
	}

	// 6. Query Market service for approval status
	marketStatus, err := s.marketClient.QueryStatus(softwareName, shopName)
	if err != nil {
		log.Printf("market service error for shop %q: %v", shopName, err)
		return nil, fmt.Errorf("market service unavailable: %w", err)
	}

	// Store market response in activation request
	marketRespJSON, _ := json.Marshal(marketStatus)
	s.writeDB.Exec(
		`UPDATE shop_activation_requests SET market_response = ? WHERE id = ?`,
		string(marketRespJSON), requestID,
	)

	shop := &Shop{
		ID:                  shopID,
		Name:                shopName,
		OwnerID:             req.OwnerID,
		StorefrontID:        0,
		SoftwareName:        softwareName,
		Description:         req.Description,
		WelcomeMessage:      "",
		Status:              StatusPending,
		ParentProductID:     req.ParentProductID,
		ShopModuleProductID: "",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	// 7. If approved: update shop status, create child product, update shop_module_product_id
	if marketStatus.Approved {
		// Create child product using description as welcome_message
		childProduct, err := s.productSvc.Create(
			shopName,
			product.ProductTypeKnowledgeBase,
			req.Description,
			req.Description, // welcome_message = description
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create shop module product: %w", err)
		}

		// Update shop status to approved and set shop_module_product_id
		updatedAt := time.Now()
		_, err = s.writeDB.Exec(
			`UPDATE shops SET status = ?, shop_module_product_id = ?, updated_at = ? WHERE id = ?`,
			StatusApproved, childProduct.ID, updatedAt, shopID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update shop status: %w", err)
		}

		// Update activation request status
		s.writeDB.Exec(
			`UPDATE shop_activation_requests SET status = ? WHERE id = ?`,
			StatusApproved, requestID,
		)

		shop.Status = StatusApproved
		shop.ShopModuleProductID = childProduct.ID
		shop.UpdatedAt = updatedAt

		return &ActivateResponse{
			Shop:                shop,
			Status:              StatusApproved,
			Message:             "shop activated successfully",
			ShopModuleProductID: childProduct.ID,
		}, nil
	}

	// 8. Not approved: keep pending, return message
	message := "shop activation pending approval"
	if marketStatus.Message != "" {
		message = marketStatus.Message
	}

	return &ActivateResponse{
		Shop:    shop,
		Status:  StatusPending,
		Message: message,
	}, nil
}

// getByOwnerID queries a shop by owner_id (internal helper).
func (s *ShopService) getByOwnerID(ownerID int64) (*Shop, error) {
	var shop Shop
	err := s.readDB.QueryRow(
		`SELECT id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at
		 FROM shops WHERE owner_id = ? LIMIT 1`,
		ownerID,
	).Scan(&shop.ID, &shop.Name, &shop.OwnerID, &shop.StorefrontID, &shop.SoftwareName, &shop.Description,
		&shop.WelcomeMessage, &shop.Status, &shop.ParentProductID, &shop.ShopModuleProductID, &shop.CreatedAt, &shop.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &shop, nil
}

// GetByOwnerID returns the shop associated with the given owner ID.
// Returns nil, nil if no shop is found.
func (s *ShopService) GetByOwnerID(ownerID int64) (*Shop, error) {
	return s.getByOwnerID(ownerID)
}

// GetByModuleProductID returns the shop associated with the given shop_module_product_id.
// Returns nil, nil if no shop is found.
func (s *ShopService) GetByModuleProductID(moduleProductID string) (*Shop, error) {
	var shop Shop
	err := s.readDB.QueryRow(
		`SELECT id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at
		 FROM shops WHERE shop_module_product_id = ? LIMIT 1`,
		moduleProductID,
	).Scan(&shop.ID, &shop.Name, &shop.OwnerID, &shop.StorefrontID, &shop.SoftwareName, &shop.Description,
		&shop.WelcomeMessage, &shop.Status, &shop.ParentProductID, &shop.ShopModuleProductID, &shop.CreatedAt, &shop.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &shop, nil
}

// ListByProductID returns all shops under the given parent_product_id.
// Returns an empty slice (not nil) if no shops are found.
func (s *ShopService) ListByProductID(productID string) ([]Shop, error) {
	rows, err := s.readDB.Query(
		`SELECT id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at
		 FROM shops WHERE parent_product_id = ?`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query shops by product ID: %w", err)
	}
	defer rows.Close()

	shops := make([]Shop, 0)
	for rows.Next() {
		var shop Shop
		if err := rows.Scan(&shop.ID, &shop.Name, &shop.OwnerID, &shop.StorefrontID, &shop.SoftwareName, &shop.Description,
			&shop.WelcomeMessage, &shop.Status, &shop.ParentProductID, &shop.ShopModuleProductID, &shop.CreatedAt, &shop.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan shop row: %w", err)
		}
		shops = append(shops, shop)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating shop rows: %w", err)
	}
	return shops, nil
}

// Register creates a new shop support record from a Marketplace registration request.
// The token is verified externally (by the handler); this method receives the verified owner_id.
func (s *ShopService) Register(ownerID int64, req RegisterRequest) error {
	softwareName := strings.TrimSpace(req.SoftwareName)
	storeName := strings.TrimSpace(req.StoreName)
	if softwareName == "" || storeName == "" {
		return fmt.Errorf("software_name and store_name are required")
	}

	// Enforce software_name invariant
	softwareName = DefaultSoftwareName

	// Check for duplicate: if owner already has a shop, skip
	existing, err := s.getByOwnerID(ownerID)
	if err != nil {
		return fmt.Errorf("failed to check existing shop: %w", err)
	}
	if existing != nil {
		// Update welcome_message if changed
		if req.WelcomeMessage != "" && req.WelcomeMessage != existing.WelcomeMessage {
			_, err = s.writeDB.Exec(
				`UPDATE shops SET welcome_message = ?, updated_at = ? WHERE id = ?`,
				req.WelcomeMessage, time.Now(), existing.ID,
			)
			if err != nil {
				return fmt.Errorf("failed to update welcome message: %w", err)
			}
		}
		return nil // already registered
	}

	shopID, err := generateID()
	if err != nil {
		return fmt.Errorf("failed to generate shop ID: %w", err)
	}

	now := time.Now()
	welcomeMsg := req.WelcomeMessage
	if welcomeMsg == "" {
		welcomeMsg = "欢迎来到 " + storeName + " 的客户支持"
	}

	_, err = s.writeDB.Exec(
		`INSERT INTO shops (id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, '', ?, ?, '', '', ?, ?)`,
		shopID, storeName, ownerID, softwareName, welcomeMsg, StatusPending, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to create shop record: %w", err)
	}

	return nil
}

// UpdateWelcomeMessage updates the welcome_message for a shop identified by storefront_id.
func (s *ShopService) UpdateWelcomeMessage(storefrontID int64, welcomeMessage string) error {
	result, err := s.writeDB.Exec(
		`UPDATE shops SET welcome_message = ?, updated_at = ? WHERE storefront_id = ?`,
		welcomeMessage, time.Now(), storefrontID,
	)
	if err != nil {
		return fmt.Errorf("failed to update welcome message: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("shop not found for storefront_id %d", storefrontID)
	}
	return nil
}

// GetByStorefrontID returns the shop associated with the given storefront_id.
// Returns nil, nil if no shop is found.
func (s *ShopService) GetByStorefrontID(storefrontID int64) (*Shop, error) {
	var shop Shop
	err := s.readDB.QueryRow(
		`SELECT id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at
		 FROM shops WHERE storefront_id = ? LIMIT 1`,
		storefrontID,
	).Scan(&shop.ID, &shop.Name, &shop.OwnerID, &shop.StorefrontID, &shop.SoftwareName, &shop.Description,
		&shop.WelcomeMessage, &shop.Status, &shop.ParentProductID, &shop.ShopModuleProductID, &shop.CreatedAt, &shop.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &shop, nil
}

// SetStorefrontID sets the storefront_id for a shop identified by owner_id.
// This is called when the Marketplace's storefront_id is first associated with a shop.
func (s *ShopService) SetStorefrontID(ownerID int64, storefrontID int64) error {
	_, err := s.writeDB.Exec(
		`UPDATE shops SET storefront_id = ?, updated_at = ? WHERE owner_id = ?`,
		storefrontID, time.Now(), ownerID,
	)
	return err
}

// Delete removes a shop and its associated data within a transaction.
// If retainKnowledge is true, only the shop record, activation requests, and child product
// are deleted; documents and chunks are preserved.
// If retainKnowledge is false, documents and chunks linked to the shop's child product
// are also deleted.
func (s *ShopService) Delete(shopID string, retainKnowledge bool) error {
	// 1. Look up the shop to get shop_module_product_id
	var moduleProductID string
	err := s.readDB.QueryRow(
		`SELECT shop_module_product_id FROM shops WHERE id = ?`, shopID,
	).Scan(&moduleProductID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("shop not found: %s", shopID)
	}
	if err != nil {
		return fmt.Errorf("failed to look up shop: %w", err)
	}

	// 2. Begin transaction
	tx, err := s.writeDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 3. Delete activation requests for this shop
	if _, err := tx.Exec(`DELETE FROM shop_activation_requests WHERE shop_id = ?`, shopID); err != nil {
		return fmt.Errorf("failed to delete activation requests: %w", err)
	}

	// 4. Delete the shop record
	if _, err := tx.Exec(`DELETE FROM shops WHERE id = ?`, shopID); err != nil {
		return fmt.Errorf("failed to delete shop: %w", err)
	}

	// 5. If not retaining knowledge, delete documents and chunks
	if !retainKnowledge && moduleProductID != "" {
		if _, err := tx.Exec(`DELETE FROM chunks WHERE product_id = ?`, moduleProductID); err != nil {
			return fmt.Errorf("failed to delete chunks: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM documents WHERE product_id = ?`, moduleProductID); err != nil {
			return fmt.Errorf("failed to delete documents: %w", err)
		}
	}

	// 6. Delete the child product (shop module)
	if moduleProductID != "" {
		if _, err := tx.Exec(`DELETE FROM products WHERE id = ?`, moduleProductID); err != nil {
			return fmt.Errorf("failed to delete shop module product: %w", err)
		}
	}

	// 7. Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete transaction: %w", err)
	}
	return nil
}
