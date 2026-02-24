package shop

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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

// findOrCreateProduct creates a new product or returns an existing one if the name is taken.
func (s *ShopService) findOrCreateProduct(name, welcomeMsg string) (*product.Product, error) {
	p, err := s.productSvc.Create(name, product.ProductTypeKnowledgeBase, welcomeMsg, welcomeMsg, false)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		// Product name taken — find and reuse the existing one
		existing, lookupErr := s.productSvc.GetByName(name)
		if lookupErr != nil || existing == nil {
			return nil, fmt.Errorf("product name conflict and lookup failed: %w", err)
		}
		log.Printf("[Shop] reusing existing product %q (id=%s)", name, existing.ID)
		return existing, nil
	}
	return p, err
}

// Activate processes a shop activation request.
// It validates input, checks for duplicate shops, creates a child product,
// and inserts the shop record as approved.
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
		// If already approved, return as-is
		if existingShop.Status == StatusApproved && existingShop.ShopModuleProductID != "" {
			return &ActivateResponse{
				Shop:                existingShop,
				Status:              existingShop.Status,
				Message:             "shop already exists",
				ShopModuleProductID: existingShop.ShopModuleProductID,
			}, nil
		}

		// Still pending — complete activation now (no approval step)
		desc := existingShop.Description
		if desc == "" {
			desc = req.Description
		}
		welcomeMsg := desc
		if welcomeMsg == "" {
			welcomeMsg = "欢迎来到 " + existingShop.Name + " 的客户支持"
		}
		childProduct, cpErr := s.findOrCreateProduct(existingShop.Name, welcomeMsg)
		if cpErr != nil {
			return nil, fmt.Errorf("failed to create shop module product: %w", cpErr)
		}

		updatedAt := time.Now()
		_, updErr := s.writeDB.Exec(
			`UPDATE shops SET status = ?, shop_module_product_id = ?, updated_at = ? WHERE id = ?`,
			StatusApproved, childProduct.ID, updatedAt, existingShop.ID,
		)
		if updErr != nil {
			return nil, fmt.Errorf("failed to update shop status: %w", updErr)
		}

		existingShop.Status = StatusApproved
		existingShop.ShopModuleProductID = childProduct.ID
		existingShop.UpdatedAt = updatedAt

		log.Printf("[ShopActivate] activated existing shop %q (id=%s) for owner %d, product=%s",
			existingShop.Name, existingShop.ID, req.OwnerID, childProduct.ID)

		return &ActivateResponse{
			Shop:                existingShop,
			Status:              StatusApproved,
			Message:             "shop activated successfully",
			ShopModuleProductID: childProduct.ID,
		}, nil
	}

	// 3. Generate shop ID
	shopID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate shop ID: %w", err)
	}

	now := time.Now()

	desc := strings.TrimSpace(req.Description)
	welcomeMsg := desc
	if welcomeMsg == "" {
		welcomeMsg = "欢迎来到 " + shopName + " 的客户支持"
	}

	// 4. Create child product immediately (no approval step)
	childProduct, err := s.findOrCreateProduct(shopName, welcomeMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to create shop module product: %w", err)
	}

	// 5. Insert shop record as approved
	_, err = s.writeDB.Exec(
		`INSERT INTO shops (id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)`,
		shopID, shopName, req.OwnerID, softwareName, desc, welcomeMsg, StatusApproved, req.ParentProductID, childProduct.ID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create shop record: %w", err)
	}

	log.Printf("[ShopActivate] created and activated shop %q (id=%s) for owner %d, product=%s",
		shopName, shopID, req.OwnerID, childProduct.ID)

	shop := &Shop{
		ID:                  shopID,
		Name:                shopName,
		OwnerID:             req.OwnerID,
		StorefrontID:        0,
		SoftwareName:        softwareName,
		Description:         desc,
		WelcomeMessage:      welcomeMsg,
		Status:              StatusApproved,
		ParentProductID:     req.ParentProductID,
		ShopModuleProductID: childProduct.ID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	return &ActivateResponse{
		Shop:                shop,
		Status:              StatusApproved,
		Message:             "shop activated successfully",
		ShopModuleProductID: childProduct.ID,
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

// FixPendingShop activates a pending shop by creating its child product.
// Auto-fixes shops created before the no-approval logic was deployed.
func (s *ShopService) FixPendingShop(sh *Shop) error {
	if sh.Status != StatusPending {
		return nil
	}
	welcomeMsg := sh.WelcomeMessage
	if welcomeMsg == "" {
		welcomeMsg = "欢迎来到 " + sh.Name + " 的客户支持"
	}
	childProduct, err := s.findOrCreateProduct(sh.Name, welcomeMsg)
	if err != nil {
		return fmt.Errorf("failed to create shop module product: %w", err)
	}
	_, err = s.writeDB.Exec(
		`UPDATE shops SET status = ?, shop_module_product_id = ?, updated_at = ? WHERE id = ?`,
		StatusApproved, childProduct.ID, time.Now(), sh.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update shop status: %w", err)
	}
	sh.Status = StatusApproved
	sh.ShopModuleProductID = childProduct.ID
	log.Printf("[ShopAutoFix] activated pending shop %q (id=%s), product=%s", sh.Name, sh.ID, childProduct.ID)
	return nil
}

// ListAll returns all shops regardless of parent_product_id.
// Returns an empty slice (not nil) if no shops are found.
func (s *ShopService) ListAll() ([]Shop, error) {
	rows, err := s.readDB.Query(
		`SELECT id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at
		 FROM shops ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query all shops: %w", err)
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
		log.Printf("[ShopRegister] owner %d already has shop %q (id=%s, status=%s), checking",
			ownerID, existing.Name, existing.ID, existing.Status)

		// Update storefront_id if it was 0 and now provided
		if existing.StorefrontID == 0 && req.StorefrontID > 0 {
			_, _ = s.writeDB.Exec(
				`UPDATE shops SET storefront_id = ?, updated_at = ? WHERE id = ?`,
				req.StorefrontID, time.Now(), existing.ID,
			)
			log.Printf("[ShopRegister] linked storefront_id=%d to shop %s", req.StorefrontID, existing.ID)
		}

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

		// If already approved, nothing to do
		if existing.Status == StatusApproved && existing.ShopModuleProductID != "" {
			return nil
		}

		// Still pending — complete activation now (Marketplace already verified eligibility)
		welcomeMsg := existing.WelcomeMessage
		if welcomeMsg == "" {
			welcomeMsg = "欢迎来到 " + existing.Name + " 的客户支持"
		}
		childProduct, err := s.findOrCreateProduct(existing.Name, welcomeMsg)
		if err != nil {
			return fmt.Errorf("failed to create shop module product: %w", err)
		}

		updatedAt := time.Now()
		_, err = s.writeDB.Exec(
			`UPDATE shops SET status = ?, shop_module_product_id = ?, updated_at = ? WHERE id = ?`,
			StatusApproved, childProduct.ID, updatedAt, existing.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update shop status: %w", err)
		}
		log.Printf("[ShopRegister] activated existing shop %q (id=%s) for owner %d, product=%s",
			existing.Name, existing.ID, ownerID, childProduct.ID)

		return nil
	}

	shopID, err := generateID()
	if err != nil {
		return fmt.Errorf("failed to generate shop ID: %w", err)
	}

	parentProductID := strings.TrimSpace(req.ParentProductID)

	now := time.Now()
	welcomeMsg := req.WelcomeMessage
	if welcomeMsg == "" {
		welcomeMsg = "欢迎来到 " + storeName + " 的客户支持"
	}

	storefrontID := req.StorefrontID // 0 if not provided by Marketplace

	// Marketplace already verified eligibility — create shop and activate immediately
	childProduct, err := s.findOrCreateProduct(storeName, welcomeMsg)
	if err != nil {
		return fmt.Errorf("failed to create shop module product: %w", err)
	}

	_, err = s.writeDB.Exec(
		`INSERT INTO shops (id, name, owner_id, storefront_id, software_name, description, welcome_message, status, parent_product_id, shop_module_product_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)`,
		shopID, storeName, ownerID, storefrontID, softwareName, welcomeMsg, StatusApproved, parentProductID, childProduct.ID, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to create shop record: %w", err)
	}

	log.Printf("[ShopRegister] created and activated shop %q (id=%s) for owner %d, storefront_id=%d, product=%s",
		storeName, shopID, ownerID, storefrontID, childProduct.ID)

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

	// 6. Delete the child product and its admin_user_products assignments
	if moduleProductID != "" {
		tx.Exec(`DELETE FROM admin_user_products WHERE product_id = ?`, moduleProductID)
		if _, err := tx.Exec(`DELETE FROM products WHERE id = ?`, moduleProductID); err != nil {
			return fmt.Errorf("failed to delete shop module product: %w", err)
		}
	}

	// 7. Clean up store owner sub-admin account (username = store_{shopID})
	storeUsername := "store_" + shopID
	var subAdminID string
	if err := s.readDB.QueryRow(`SELECT id FROM admin_users WHERE username = ?`, storeUsername).Scan(&subAdminID); err == nil {
		tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, "admin_"+subAdminID)
		tx.Exec(`DELETE FROM admin_users WHERE id = ?`, subAdminID)
	}

	// 8. Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete transaction: %w", err)
	}
	return nil
}

