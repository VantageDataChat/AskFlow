//go:build cgo

package shop

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"pgregory.net/rapid"

	"askflow/internal/product"
)

// setupTestDB creates an in-memory SQLite database with the required tables for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS products (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL UNIQUE,
			type            TEXT DEFAULT 'service',
			description     TEXT DEFAULT '',
			welcome_message TEXT DEFAULT '',
			allow_download  INTEGER DEFAULT 0,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS shops (
			id                     TEXT PRIMARY KEY,
			name                   TEXT NOT NULL,
			owner_id               INTEGER NOT NULL,
			storefront_id          INTEGER DEFAULT 0,
			software_name          TEXT NOT NULL DEFAULT 'vantagics',
			description            TEXT DEFAULT '',
			welcome_message        TEXT DEFAULT '',
			status                 TEXT NOT NULL DEFAULT 'pending',
			parent_product_id      TEXT NOT NULL DEFAULT '',
			shop_module_product_id TEXT DEFAULT '',
			created_at             DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at             DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS shop_activation_requests (
			id              TEXT PRIMARY KEY,
			shop_id         TEXT NOT NULL,
			software_name   TEXT NOT NULL,
			shop_name       TEXT NOT NULL,
			market_response TEXT DEFAULT '',
			status          TEXT NOT NULL DEFAULT 'pending',
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS documents (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			type         TEXT NOT NULL,
			status       TEXT NOT NULL,
			error        TEXT,
			content_hash TEXT DEFAULT '',
			product_id   TEXT DEFAULT '',
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id            TEXT PRIMARY KEY,
			document_id   TEXT NOT NULL,
			document_name TEXT NOT NULL,
			chunk_index   INTEGER NOT NULL,
			chunk_text    TEXT NOT NULL,
			embedding     BLOB NOT NULL,
			image_url     TEXT DEFAULT '',
			product_id    TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (document_id) REFERENCES documents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS admin_users (
			id            TEXT PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'editor',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS admin_user_products (
			admin_user_id TEXT NOT NULL,
			product_id    TEXT NOT NULL,
			PRIMARY KEY (admin_user_id, product_id),
			FOREIGN KEY (admin_user_id) REFERENCES admin_users(id) ON DELETE CASCADE,
			FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}

	return db
}

// newTestMarketServer creates a mock Market service that returns the given approval status.
func newTestMarketServer(approved bool, message string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: approved, Message: message})
	}))
}

// seedParentProduct inserts a parent product and returns its ID.
func seedParentProduct(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := "parent-product-001"
	_, err := db.Exec(
		`INSERT INTO products (id, name, type, description, welcome_message, allow_download) VALUES (?, ?, ?, ?, ?, ?)`,
		id, "Parent Product", "service", "parent desc", "", 0,
	)
	if err != nil {
		t.Fatalf("failed to seed parent product: %v", err)
	}
	return id
}

func TestActivate_ValidationError_EmptySoftwareName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	_, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "",
		ShopName:        "test-shop",
		ParentProductID: "p1",
	})
	if err == nil {
		t.Fatal("expected validation error for empty software_name")
	}
}

func TestActivate_ValidationError_EmptyShopName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	_, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "   ",
		ParentProductID: "p1",
	})
	if err == nil {
		t.Fatal("expected validation error for whitespace-only shop_name")
	}
}

func TestActivate_Approved(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	resp, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "my-shop",
		Description:     "Welcome to my shop!",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusApproved {
		t.Errorf("expected status %q, got %q", StatusApproved, resp.Status)
	}
	if resp.ShopModuleProductID == "" {
		t.Error("expected non-empty ShopModuleProductID for approved shop")
	}
	if resp.Shop.Name != "my-shop" {
		t.Errorf("expected shop name %q, got %q", "my-shop", resp.Shop.Name)
	}

	// Verify child product was created with description as welcome_message
	var welcomeMsg string
	err = db.QueryRow("SELECT welcome_message FROM products WHERE id = ?", resp.ShopModuleProductID).Scan(&welcomeMsg)
	if err != nil {
		t.Fatalf("failed to query child product: %v", err)
	}
	if welcomeMsg != "Welcome to my shop!" {
		t.Errorf("expected welcome_message %q, got %q", "Welcome to my shop!", welcomeMsg)
	}

	// Verify shop record in DB
	var dbStatus, dbModuleID string
	err = db.QueryRow("SELECT status, shop_module_product_id FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbStatus, &dbModuleID)
	if err != nil {
		t.Fatalf("failed to query shop: %v", err)
	}
	if dbStatus != StatusApproved {
		t.Errorf("expected DB status %q, got %q", StatusApproved, dbStatus)
	}
	if dbModuleID != resp.ShopModuleProductID {
		t.Errorf("expected DB module ID %q, got %q", resp.ShopModuleProductID, dbModuleID)
	}
}

// TestActivate_AlwaysApproved verifies that Activate always approves directly
// regardless of market server response (no-approval mode).
func TestActivate_AlwaysApproved(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Even with a market server that would reject, Activate approves directly
	srv := newTestMarketServer(false, "pending review")
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	resp, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "my-shop",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusApproved {
		t.Errorf("expected status %q, got %q", StatusApproved, resp.Status)
	}
	if resp.ShopModuleProductID == "" {
		t.Error("expected non-empty ShopModuleProductID for approved shop")
	}

	// Verify shop record in DB is approved
	var dbStatus string
	err = db.QueryRow("SELECT status FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbStatus)
	if err != nil {
		t.Fatalf("failed to query shop: %v", err)
	}
	if dbStatus != StatusApproved {
		t.Errorf("expected DB status %q, got %q", StatusApproved, dbStatus)
	}
}

func TestActivate_DuplicateOwner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	// First activation
	resp1, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "my-shop",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("first activation failed: %v", err)
	}

	// Second activation with same owner
	resp2, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "another-shop",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("second activation failed: %v", err)
	}

	// Should return existing shop info
	if resp2.Shop.ID != resp1.Shop.ID {
		t.Errorf("expected same shop ID %q, got %q", resp1.Shop.ID, resp2.Shop.ID)
	}
	if resp2.Message != "shop already exists" {
		t.Errorf("expected message %q, got %q", "shop already exists", resp2.Message)
	}
}

// TestActivate_SucceedsWithoutMarket verifies that Activate succeeds even when
// the market service is unavailable (no-approval mode, market not called).
func TestActivate_SucceedsWithoutMarket(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Market server that returns 500 — should not matter since Activate doesn't call it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	resp, err := svc.Activate(ActivateRequest{
		OwnerID:         1,
		SoftwareName:    "vantagics",
		ShopName:        "my-shop",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("expected no error in no-approval mode, got: %v", err)
	}
	if resp.Status != StatusApproved {
		t.Errorf("expected status %q, got %q", StatusApproved, resp.Status)
	}
	if resp.ShopModuleProductID == "" {
		t.Error("expected non-empty ShopModuleProductID")
	}
}

// Feature: shop-support, Property 2: 缺失必填字段的开通请求被拒绝
// Validates: Requirements 2.1, 2.3
func TestProperty_MissingRequiredFieldsRejected(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Set up fresh DB and service per iteration
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generator for blank/whitespace-only strings
		blankGen := rapid.StringMatching(`^\s*$`)

		// Pick which field(s) to make invalid: 0=software_name, 1=shop_name, 2=both
		scenario := rapid.IntRange(0, 2).Draw(rt, "scenario")

		var softwareName, shopName string
		switch scenario {
		case 0: // invalid software_name, valid shop_name
			softwareName = blankGen.Draw(rt, "software_name")
			shopName = rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		case 1: // valid software_name, invalid shop_name
			softwareName = rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "software_name")
			shopName = blankGen.Draw(rt, "shop_name")
		case 2: // both invalid
			softwareName = blankGen.Draw(rt, "software_name")
			shopName = blankGen.Draw(rt, "shop_name")
		}

		ownerID := rapid.Int64Range(1, 100000).Draw(rt, "owner_id")

		_, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    softwareName,
			ShopName:        shopName,
			ParentProductID: parentID,
		})

		// Must return an error
		if err == nil {
			rt.Fatalf("expected error for software_name=%q shop_name=%q, got nil", softwareName, shopName)
		}

		// Verify no records were created in the shops table
		var count int
		if qErr := db.QueryRow("SELECT COUNT(*) FROM shops").Scan(&count); qErr != nil {
			rt.Fatalf("failed to count shops: %v", qErr)
		}
		if count != 0 {
			rt.Fatalf("expected 0 shops, got %d", count)
		}

		// Verify no records were created in the shop_activation_requests table
		if qErr := db.QueryRow("SELECT COUNT(*) FROM shop_activation_requests").Scan(&count); qErr != nil {
			rt.Fatalf("failed to count activation requests: %v", qErr)
		}
		if count != 0 {
			rt.Fatalf("expected 0 activation requests, got %d", count)
		}
	})
}

// Feature: shop-support, Property 3: Software Name 不变量
// Validates: Requirements 2.2
func TestProperty_SoftwareNameInvariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate any valid non-empty, non-whitespace software_name
		softwareName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_-]{0,19}`).Draw(rt, "software_name")
		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		ownerID := rapid.Int64Range(1, 100000).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    softwareName,
			ShopName:        shopName,
			Description:     "test shop",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}

		// Query the shops table for the created record
		var dbSoftwareName string
		qErr := db.QueryRow("SELECT software_name FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbSoftwareName)
		if qErr != nil {
			rt.Fatalf("failed to query shop: %v", qErr)
		}

		// The software_name in the DB must always be "vantagics"
		if dbSoftwareName != DefaultSoftwareName {
			rt.Fatalf("expected software_name=%q, got %q (input was %q)", DefaultSoftwareName, dbSoftwareName, softwareName)
		}
	})
}

// Feature: shop-support, Property 4: 有效开通请求直接审批
// Validates: Requirements 2.4 (no-approval mode: always approved)
func TestProperty_ValidActivationAlwaysApproved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Market response is irrelevant — Activate always approves directly
		srv := newTestMarketServer(false, "pending")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate valid software_name and shop_name
		softwareName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_-]{0,19}`).Draw(rt, "software_name")
		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		// Each iteration needs a unique owner_id to avoid duplicate detection
		ownerID := rapid.Int64Range(1, 1<<53).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    softwareName,
			ShopName:        shopName,
			Description:     "test description",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}

		// Verify response status is always approved (no-approval mode)
		if resp.Status != StatusApproved {
			rt.Fatalf("expected status %q, got %q", StatusApproved, resp.Status)
		}
		if resp.ShopModuleProductID == "" {
			rt.Fatal("expected non-empty ShopModuleProductID for approved shop")
		}

		// Verify a record exists in shops table with status "approved"
		var shopStatus string
		qErr := db.QueryRow("SELECT status FROM shops WHERE id = ?", resp.Shop.ID).Scan(&shopStatus)
		if qErr != nil {
			rt.Fatalf("failed to query shop record: %v", qErr)
		}
		if shopStatus != StatusApproved {
			rt.Fatalf("expected shop status %q in DB, got %q", StatusApproved, shopStatus)
		}
	})
}

// Feature: shop-support, Property 6: 直接审批模式下状态始终为 approved
// Validates: No-approval mode — Market response is not consulted
func TestProperty_ActivationAlwaysApproved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Fresh DB per iteration
		db := setupTestDB(t)
		defer db.Close()

		// Randomly decide whether Market would approve or not — should not matter
		approved := rapid.Bool().Draw(rt, "market_approved")

		srv := newTestMarketServer(approved, "market response")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		ownerID := rapid.Int64Range(1, 1<<53).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     "shop description",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}

		// In no-approval mode, status is always approved regardless of market response
		if resp.Status != StatusApproved {
			rt.Fatalf("expected status %q, got %q", StatusApproved, resp.Status)
		}
		if resp.ShopModuleProductID == "" {
			rt.Fatal("expected non-empty ShopModuleProductID")
		}

		// Verify DB record is always approved
		var dbStatus, dbModuleID string
		qErr := db.QueryRow("SELECT status, shop_module_product_id FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbStatus, &dbModuleID)
		if qErr != nil {
			rt.Fatalf("failed to query shop: %v", qErr)
		}
		if dbStatus != StatusApproved {
			rt.Fatalf("expected DB status %q, got %q", StatusApproved, dbStatus)
		}
		if dbModuleID == "" {
			rt.Fatal("expected non-empty DB shop_module_product_id")
		}
	})
}

// Feature: shop-support, Property 5: 店铺介绍保留为欢迎信息
// Validates: Requirements 2.5, 4.2
func TestProperty_DescriptionPreservedAsWelcomeMessage(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Market always approves so the child product is created
		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate a non-empty description (at least 1 visible character, no leading/trailing spaces
		// since Activate trims the description)
		description := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9]{0,49}`).Draw(rt, "description")
		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		ownerID := rapid.Int64Range(1, 1<<53).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     description,
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != StatusApproved {
			rt.Fatalf("expected approved status, got %q", resp.Status)
		}
		if resp.ShopModuleProductID == "" {
			rt.Fatal("expected non-empty ShopModuleProductID for approved shop")
		}

		// Query the child product's welcome_message from the products table
		var welcomeMsg string
		qErr := db.QueryRow("SELECT welcome_message FROM products WHERE id = ?", resp.ShopModuleProductID).Scan(&welcomeMsg)
		if qErr != nil {
			rt.Fatalf("failed to query child product welcome_message: %v", qErr)
		}

		// The welcome_message must match the trimmed description
		trimmedDesc := strings.TrimSpace(description)
		if welcomeMsg != trimmedDesc {
			rt.Fatalf("welcome_message mismatch: expected %q, got %q", trimmedDesc, welcomeMsg)
		}
	})
}

func TestGetByOwnerID_Found(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	ownerID := int64(42)
	_, err := svc.Activate(ActivateRequest{
		OwnerID:         ownerID,
		SoftwareName:    "vantagics",
		ShopName:        "TestShop",
		Description:     "desc",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}

	shop, err := svc.GetByOwnerID(ownerID)
	if err != nil {
		t.Fatalf("GetByOwnerID failed: %v", err)
	}
	if shop == nil {
		t.Fatal("expected shop, got nil")
	}
	if shop.OwnerID != ownerID {
		t.Fatalf("expected owner_id %d, got %d", ownerID, shop.OwnerID)
	}
	if shop.Name != "TestShop" {
		t.Fatalf("expected name TestShop, got %s", shop.Name)
	}
}

func TestGetByOwnerID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	shop, err := svc.GetByOwnerID(999)
	if err != nil {
		t.Fatalf("GetByOwnerID failed: %v", err)
	}
	if shop != nil {
		t.Fatalf("expected nil, got shop %+v", shop)
	}
}

func TestGetByModuleProductID_Found(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	resp, err := svc.Activate(ActivateRequest{
		OwnerID:         int64(55),
		SoftwareName:    "vantagics",
		ShopName:        "ModuleShop",
		Description:     "module desc",
		ParentProductID: parentID,
	})
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if resp.ShopModuleProductID == "" {
		t.Fatal("expected non-empty ShopModuleProductID after approval")
	}

	shop, err := svc.GetByModuleProductID(resp.ShopModuleProductID)
	if err != nil {
		t.Fatalf("GetByModuleProductID failed: %v", err)
	}
	if shop == nil {
		t.Fatal("expected shop, got nil")
	}
	if shop.ShopModuleProductID != resp.ShopModuleProductID {
		t.Fatalf("expected module product id %s, got %s", resp.ShopModuleProductID, shop.ShopModuleProductID)
	}
	if shop.Name != "ModuleShop" {
		t.Fatalf("expected name ModuleShop, got %s", shop.Name)
	}
}

func TestGetByModuleProductID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

	shop, err := svc.GetByModuleProductID("nonexistent-id")
	if err != nil {
		t.Fatalf("GetByModuleProductID failed: %v", err)
	}
	if shop != nil {
		t.Fatalf("expected nil, got shop %+v", shop)
	}
}

// seedApprovedShopWithData creates an approved shop with a child product, documents, and chunks.
// Returns shopID, moduleProductID.
func seedApprovedShopWithData(t *testing.T, db *sql.DB, svc *ShopService, parentProductID string) (string, string) {
	t.Helper()

	srv := newTestMarketServer(true, "ok")
	defer srv.Close()

	svc.marketClient = NewMarketClient(srv.URL)

	resp, err := svc.Activate(ActivateRequest{
		SoftwareName:    "vantagics",
		ShopName:        "DeleteTestShop",
		Description:     "test shop for delete",
		ParentProductID: parentProductID,
		OwnerID:         900,
	})
	if err != nil {
		t.Fatalf("failed to activate shop: %v", err)
	}
	if resp.Status != StatusApproved {
		t.Fatalf("expected approved status, got %s", resp.Status)
	}

	shopID := resp.Shop.ID
	moduleProductID := resp.ShopModuleProductID

	// Seed documents linked to the child product
	for i := 0; i < 3; i++ {
		docID := fmt.Sprintf("doc-%s-%d", shopID[:8], i)
		_, err := db.Exec(
			`INSERT INTO documents (id, name, type, status, product_id) VALUES (?, ?, ?, ?, ?)`,
			docID, fmt.Sprintf("doc-%d", i), "text", "ready", moduleProductID,
		)
		if err != nil {
			t.Fatalf("failed to seed document: %v", err)
		}
		// Seed a chunk per document
		chunkID := fmt.Sprintf("chunk-%s-%d", shopID[:8], i)
		_, err = db.Exec(
			`INSERT INTO chunks (id, document_id, document_name, chunk_index, chunk_text, embedding, product_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			chunkID, docID, fmt.Sprintf("doc-%d", i), 0, "chunk text", []byte{0x01}, moduleProductID,
		)
		if err != nil {
			t.Fatalf("failed to seed chunk: %v", err)
		}
	}

	// Seed an activation request
	_, err = db.Exec(
		`INSERT INTO shop_activation_requests (id, shop_id, software_name, shop_name, status) VALUES (?, ?, ?, ?, ?)`,
		fmt.Sprintf("ar-%s", shopID[:8]), shopID, "vantagics", "DeleteTestShop", "approved",
	)
	if err != nil {
		t.Fatalf("failed to seed activation request: %v", err)
	}

	return shopID, moduleProductID
}

func TestDelete_RetainKnowledge(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient("http://unused"), product.NewProductService(db, db))
	shopID, moduleProductID := seedApprovedShopWithData(t, db, svc, parentID)

	// Delete with retainKnowledge=true
	err := svc.Delete(shopID, true)
	if err != nil {
		t.Fatalf("Delete(retainKnowledge=true) failed: %v", err)
	}

	// Shop record should be gone
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM shops WHERE id = ?`, shopID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected shop to be deleted, found %d", count)
	}

	// Activation requests should be gone
	db.QueryRow(`SELECT COUNT(*) FROM shop_activation_requests WHERE shop_id = ?`, shopID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected activation requests to be deleted, found %d", count)
	}

	// Child product should be gone
	db.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, moduleProductID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected child product to be deleted, found %d", count)
	}

	// Documents should be preserved
	db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleProductID).Scan(&count)
	if count != 3 {
		t.Fatalf("expected 3 documents preserved, found %d", count)
	}

	// Chunks should be preserved
	db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleProductID).Scan(&count)
	if count != 3 {
		t.Fatalf("expected 3 chunks preserved, found %d", count)
	}
}

func TestDelete_WithoutRetainKnowledge(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	parentID := seedParentProduct(t, db)
	svc := NewShopService(db, db, NewMarketClient("http://unused"), product.NewProductService(db, db))
	shopID, moduleProductID := seedApprovedShopWithData(t, db, svc, parentID)

	// Delete with retainKnowledge=false
	err := svc.Delete(shopID, false)
	if err != nil {
		t.Fatalf("Delete(retainKnowledge=false) failed: %v", err)
	}

	// Shop record should be gone
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM shops WHERE id = ?`, shopID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected shop to be deleted, found %d", count)
	}

	// Activation requests should be gone
	db.QueryRow(`SELECT COUNT(*) FROM shop_activation_requests WHERE shop_id = ?`, shopID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected activation requests to be deleted, found %d", count)
	}

	// Child product should be gone
	db.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, moduleProductID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected child product to be deleted, found %d", count)
	}

	// Documents should be deleted
	db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleProductID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 documents, found %d", count)
	}

	// Chunks should be deleted
	db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleProductID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 chunks, found %d", count)
	}
}

func TestDelete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewShopService(db, db, NewMarketClient("http://unused"), product.NewProductService(db, db))

	err := svc.Delete("nonexistent-shop-id", false)
	if err == nil {
		t.Fatal("expected error for nonexistent shop, got nil")
	}
}
// Feature: shop-support, Property 7: 直接审批模式下支持模块始终存在
// Validates: Requirements 4.1, 4.3 (no-approval mode: always approved with module)
func TestProperty_ModuleAlwaysExistsInNoApprovalMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Market response is irrelevant in no-approval mode
		approved := rapid.Bool().Draw(rt, "market_approved")

		srv := newTestMarketServer(approved, "market response")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		ownerID := rapid.Int64Range(1, 1<<53).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     "shop description",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}

		// Query the shops table for the created record
		var dbStatus, dbModuleProductID string
		qErr := db.QueryRow("SELECT status, shop_module_product_id FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbStatus, &dbModuleProductID)
		if qErr != nil {
			rt.Fatalf("failed to query shop: %v", qErr)
		}

		// In no-approval mode, shop is always approved with a module product
		if dbStatus != StatusApproved {
			rt.Fatalf("expected status %q, got %q", StatusApproved, dbStatus)
		}
		if dbModuleProductID == "" {
			rt.Fatal("approved shop has empty shop_module_product_id")
		}

		// A matching product record MUST exist in the products table
		var productCount int
		qErr = db.QueryRow("SELECT COUNT(*) FROM products WHERE id = ?", dbModuleProductID).Scan(&productCount)
		if qErr != nil {
			rt.Fatalf("failed to query products table: %v", qErr)
		}
		if productCount != 1 {
			rt.Fatalf("expected exactly 1 product for shop_module_product_id=%q, got %d", dbModuleProductID, productCount)
		}
	})
}

// Feature: shop-support, Property 8: 支持模块包含完整字段
// Validates: Requirements 4.4
func TestProperty_ModuleContainsCompleteFields(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Market approved=true so child product is created
		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, "shop_name")
		description := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 ]{0,49}`).Draw(rt, "description")
		ownerID := rapid.Int64Range(1, 1<<53).Draw(rt, "owner_id")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     description,
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != StatusApproved {
			rt.Fatalf("expected approved status, got %q", resp.Status)
		}
		if resp.ShopModuleProductID == "" {
			rt.Fatal("expected non-empty ShopModuleProductID for approved shop")
		}

		// Query the child product record from the products table
		var productName, welcomeMsg string
		qErr := db.QueryRow("SELECT name, welcome_message FROM products WHERE id = ?", resp.ShopModuleProductID).Scan(&productName, &welcomeMsg)
		if qErr != nil {
			rt.Fatalf("failed to query child product: %v", qErr)
		}

		// Child product must have a non-empty name
		if productName == "" {
			rt.Fatal("child product has empty name")
		}

		// Child product must have a non-empty welcome_message
		if welcomeMsg == "" {
			rt.Fatal("child product has empty welcome_message")
		}

		// Verify the shop record has a valid parent_product_id
		var dbParentProductID string
		qErr = db.QueryRow("SELECT parent_product_id FROM shops WHERE id = ?", resp.Shop.ID).Scan(&dbParentProductID)
		if qErr != nil {
			rt.Fatalf("failed to query shop parent_product_id: %v", qErr)
		}
		if dbParentProductID == "" {
			rt.Fatal("shop has empty parent_product_id")
		}
		// parent_product_id must reference an existing product
		var parentCount int
		qErr = db.QueryRow("SELECT COUNT(*) FROM products WHERE id = ?", dbParentProductID).Scan(&parentCount)
		if qErr != nil {
			rt.Fatalf("failed to verify parent product exists: %v", qErr)
		}
		if parentCount != 1 {
			rt.Fatalf("expected exactly 1 parent product for parent_product_id=%q, got %d", dbParentProductID, parentCount)
		}
	})
}

// Feature: shop-support, Property 11: 店铺列表包含完整信息
// Validates: Requirements 6.2
func TestProperty_ShopListContainsCompleteInfo(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate a random number of shops (1-5)
		shopCount := rapid.IntRange(1, 5).Draw(rt, "shop_count")

		// Use distinct owner_ids to avoid duplicate-owner detection in Activate
		for i := 0; i < shopCount; i++ {
			ownerID := int64(i + 1)
			// Append index to ensure unique shop names (product name uniqueness constraint)
			baseName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{0,9}`).Draw(rt, fmt.Sprintf("shop_name_%d", i))
			shopName := fmt.Sprintf("%s_%d", baseName, i)

			_, err := svc.Activate(ActivateRequest{
				OwnerID:         ownerID,
				SoftwareName:    "vantagics",
				ShopName:        shopName,
				Description:     "shop description",
				ParentProductID: parentID,
			})
			if err != nil {
				rt.Fatalf("failed to activate shop %d: %v", i, err)
			}
		}

		// Call ListByProductID
		shops, err := svc.ListByProductID(parentID)
		if err != nil {
			rt.Fatalf("ListByProductID failed: %v", err)
		}

		// Verify the returned list has the expected count
		if len(shops) != shopCount {
			rt.Fatalf("expected %d shops, got %d", shopCount, len(shops))
		}

		// For each shop, verify the four required fields are non-empty / non-zero
		for idx, s := range shops {
			if s.Name == "" {
				rt.Fatalf("shop[%d] has empty name", idx)
			}
			if s.Status == "" {
				rt.Fatalf("shop[%d] has empty status", idx)
			}
			if s.ParentProductID == "" {
				rt.Fatalf("shop[%d] has empty parent_product_id", idx)
			}
			if s.CreatedAt.IsZero() {
				rt.Fatalf("shop[%d] has zero created_at", idx)
			}
		}
	})
}

// Feature: shop-support, Property 12: 保留知识的删除操作
// Validates: Requirements 6.4
func TestProperty_RetainKnowledgeDelete(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate a random number of documents to seed (1-5)
		docCount := rapid.IntRange(1, 5).Draw(rt, "doc_count")

		// Create an approved shop with a unique owner_id
		ownerID := int64(rapid.IntRange(1, 100000).Draw(rt, "owner_id"))
		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{2,9}`).Draw(rt, "shop_name")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     "test shop for property 12",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("failed to activate shop: %v", err)
		}
		if resp.Status != StatusApproved {
			rt.Fatalf("expected approved status, got %s", resp.Status)
		}

		shopID := resp.Shop.ID
		moduleProductID := resp.ShopModuleProductID

		// Seed documents and chunks linked to the child product
		for i := 0; i < docCount; i++ {
			docID := fmt.Sprintf("doc-%s-%d", shopID[:8], i)
			_, err := db.Exec(
				`INSERT INTO documents (id, name, type, status, product_id) VALUES (?, ?, ?, ?, ?)`,
				docID, fmt.Sprintf("doc-%d", i), "text", "ready", moduleProductID,
			)
			if err != nil {
				rt.Fatalf("failed to seed document: %v", err)
			}
			chunkID := fmt.Sprintf("chunk-%s-%d", shopID[:8], i)
			_, err = db.Exec(
				`INSERT INTO chunks (id, document_id, document_name, chunk_index, chunk_text, embedding, product_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				chunkID, docID, fmt.Sprintf("doc-%d", i), 0, "chunk text", []byte{0x01}, moduleProductID,
			)
			if err != nil {
				rt.Fatalf("failed to seed chunk: %v", err)
			}
		}

		// Seed an activation request
		_, err = db.Exec(
			`INSERT INTO shop_activation_requests (id, shop_id, software_name, shop_name, status) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("ar-%s", shopID[:8]), shopID, "vantagics", shopName, "approved",
		)
		if err != nil {
			rt.Fatalf("failed to seed activation request: %v", err)
		}

		// Execute retainKnowledge=true delete
		err = svc.Delete(shopID, true)
		if err != nil {
			rt.Fatalf("Delete(retainKnowledge=true) failed: %v", err)
		}

		var count int

		// Verify shops record is gone
		db.QueryRow(`SELECT COUNT(*) FROM shops WHERE id = ?`, shopID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected shop record to be deleted, found %d", count)
		}

		// Verify child product record is gone
		db.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, moduleProductID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected child product to be deleted, found %d", count)
		}

		// Verify shop_activation_requests are gone
		db.QueryRow(`SELECT COUNT(*) FROM shop_activation_requests WHERE shop_id = ?`, shopID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected activation requests to be deleted, found %d", count)
		}

		// Verify documents still exist (count unchanged)
		db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleProductID).Scan(&count)
		if count != docCount {
			rt.Fatalf("expected %d documents preserved, found %d", docCount, count)
		}

		// Verify chunks still exist (count unchanged)
		db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleProductID).Scan(&count)
		if count != docCount {
			rt.Fatalf("expected %d chunks preserved, found %d", docCount, count)
		}
	})
}

// Feature: shop-support, Property 13: 不保留知识的完全删除操作
// **Validates: Requirements 6.5**
func TestProperty_FullDeleteWithoutRetainKnowledge(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate a random number of documents to seed (1-5)
		docCount := rapid.IntRange(1, 5).Draw(rt, "doc_count")

		// Create an approved shop with a unique owner_id
		ownerID := int64(rapid.IntRange(1, 100000).Draw(rt, "owner_id"))
		shopName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{2,9}`).Draw(rt, "shop_name")

		resp, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    "vantagics",
			ShopName:        shopName,
			Description:     "test shop for property 13",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("failed to activate shop: %v", err)
		}
		if resp.Status != StatusApproved {
			rt.Fatalf("expected approved status, got %s", resp.Status)
		}

		shopID := resp.Shop.ID
		moduleProductID := resp.ShopModuleProductID

		// Seed documents and chunks linked to the child product
		for i := 0; i < docCount; i++ {
			docID := fmt.Sprintf("doc-%s-%d", shopID[:8], i)
			_, err := db.Exec(
				`INSERT INTO documents (id, name, type, status, product_id) VALUES (?, ?, ?, ?, ?)`,
				docID, fmt.Sprintf("doc-%d", i), "text", "ready", moduleProductID,
			)
			if err != nil {
				rt.Fatalf("failed to seed document: %v", err)
			}
			chunkID := fmt.Sprintf("chunk-%s-%d", shopID[:8], i)
			_, err = db.Exec(
				`INSERT INTO chunks (id, document_id, document_name, chunk_index, chunk_text, embedding, product_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				chunkID, docID, fmt.Sprintf("doc-%d", i), 0, "chunk text", []byte{0x01}, moduleProductID,
			)
			if err != nil {
				rt.Fatalf("failed to seed chunk: %v", err)
			}
		}

		// Seed an activation request
		_, err = db.Exec(
			`INSERT INTO shop_activation_requests (id, shop_id, software_name, shop_name, status) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("ar-%s", shopID[:8]), shopID, "vantagics", shopName, "approved",
		)
		if err != nil {
			rt.Fatalf("failed to seed activation request: %v", err)
		}

		// Execute retainKnowledge=false delete (full delete)
		err = svc.Delete(shopID, false)
		if err != nil {
			rt.Fatalf("Delete(retainKnowledge=false) failed: %v", err)
		}

		var count int

		// Verify shops record is gone
		db.QueryRow(`SELECT COUNT(*) FROM shops WHERE id = ?`, shopID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected shop record to be deleted, found %d", count)
		}

		// Verify child product record is gone
		db.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, moduleProductID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected child product to be deleted, found %d", count)
		}

		// Verify shop_activation_requests are gone
		db.QueryRow(`SELECT COUNT(*) FROM shop_activation_requests WHERE shop_id = ?`, shopID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected activation requests to be deleted, found %d", count)
		}

		// Verify documents are also deleted
		db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleProductID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected all documents to be deleted, found %d", count)
		}

		// Verify chunks are also deleted
		db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleProductID).Scan(&count)
		if count != 0 {
			rt.Fatalf("expected all chunks to be deleted, found %d", count)
		}
	})
}



// Feature: shop-support, Property 14: 店铺间数据完全隔离
// **Validates: Requirements 7.1, 7.2**
func TestProperty_ShopDataFullIsolation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		srv := newTestMarketServer(true, "ok")
		defer srv.Close()

		parentID := seedParentProduct(t, db)
		svc := NewShopService(db, db, NewMarketClient(srv.URL), product.NewProductService(db, db))

		// Generate random document counts for each shop (1-5)
		docCountA := rapid.IntRange(1, 5).Draw(rt, "doc_count_a")
		docCountB := rapid.IntRange(1, 5).Draw(rt, "doc_count_b")

		// --- Create Shop A ---
		ownerA := int64(rapid.IntRange(1, 50000).Draw(rt, "owner_a"))
		shopNameA := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{2,9}`).Draw(rt, "shop_name_a")

		respA, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerA,
			SoftwareName:    "vantagics",
			ShopName:        shopNameA,
			Description:     "shop A description",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("failed to activate shop A: %v", err)
		}
		if respA.Status != StatusApproved {
			rt.Fatalf("expected shop A approved, got %s", respA.Status)
		}
		moduleA := respA.ShopModuleProductID

		// --- Create Shop B (different owner) ---
		ownerB := ownerA + 50000 // ensure different owner_id
		shopNameB := shopNameA + "_b"

		respB, err := svc.Activate(ActivateRequest{
			OwnerID:         ownerB,
			SoftwareName:    "vantagics",
			ShopName:        shopNameB,
			Description:     "shop B description",
			ParentProductID: parentID,
		})
		if err != nil {
			rt.Fatalf("failed to activate shop B: %v", err)
		}
		if respB.Status != StatusApproved {
			rt.Fatalf("expected shop B approved, got %s", respB.Status)
		}
		moduleB := respB.ShopModuleProductID

		// Sanity: the two modules must be different
		if moduleA == moduleB {
			rt.Fatalf("shop A and B have the same module product id: %s", moduleA)
		}

		// --- Seed documents and chunks for Shop A ---
		for i := 0; i < docCountA; i++ {
			docID := fmt.Sprintf("docA-%s-%d", respA.Shop.ID[:8], i)
			_, err := db.Exec(
				`INSERT INTO documents (id, name, type, status, product_id) VALUES (?, ?, ?, ?, ?)`,
				docID, fmt.Sprintf("docA-%d", i), "text", "ready", moduleA,
			)
			if err != nil {
				rt.Fatalf("failed to seed document for shop A: %v", err)
			}
			chunkID := fmt.Sprintf("chunkA-%s-%d", respA.Shop.ID[:8], i)
			_, err = db.Exec(
				`INSERT INTO chunks (id, document_id, document_name, chunk_index, chunk_text, embedding, product_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				chunkID, docID, fmt.Sprintf("docA-%d", i), 0, "chunk A text", []byte{0x01}, moduleA,
			)
			if err != nil {
				rt.Fatalf("failed to seed chunk for shop A: %v", err)
			}
		}

		// --- Seed documents and chunks for Shop B ---
		for i := 0; i < docCountB; i++ {
			docID := fmt.Sprintf("docB-%s-%d", respB.Shop.ID[:8], i)
			_, err := db.Exec(
				`INSERT INTO documents (id, name, type, status, product_id) VALUES (?, ?, ?, ?, ?)`,
				docID, fmt.Sprintf("docB-%d", i), "text", "ready", moduleB,
			)
			if err != nil {
				rt.Fatalf("failed to seed document for shop B: %v", err)
			}
			chunkID := fmt.Sprintf("chunkB-%s-%d", respB.Shop.ID[:8], i)
			_, err = db.Exec(
				`INSERT INTO chunks (id, document_id, document_name, chunk_index, chunk_text, embedding, product_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				chunkID, docID, fmt.Sprintf("docB-%d", i), 0, "chunk B text", []byte{0x01}, moduleB,
			)
			if err != nil {
				rt.Fatalf("failed to seed chunk for shop B: %v", err)
			}
		}

		// --- Query documents by Shop A's product_id: must not contain Shop B's data ---
		var docsA int
		if err := db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleA).Scan(&docsA); err != nil {
			rt.Fatalf("failed to count documents for shop A: %v", err)
		}
		if docsA != docCountA {
			rt.Fatalf("expected %d documents for shop A, got %d", docCountA, docsA)
		}

		// Verify no document under moduleA actually belongs to moduleB
		var leakAtoB int
		if err := db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ? AND id LIKE 'docB%%'`, moduleA).Scan(&leakAtoB); err != nil {
			rt.Fatalf("failed to check leak A->B: %v", err)
		}
		if leakAtoB != 0 {
			rt.Fatalf("found %d shop B documents leaking into shop A query", leakAtoB)
		}

		// --- Query documents by Shop B's product_id: must not contain Shop A's data ---
		var docsB int
		if err := db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ?`, moduleB).Scan(&docsB); err != nil {
			rt.Fatalf("failed to count documents for shop B: %v", err)
		}
		if docsB != docCountB {
			rt.Fatalf("expected %d documents for shop B, got %d", docCountB, docsB)
		}

		var leakBtoA int
		if err := db.QueryRow(`SELECT COUNT(*) FROM documents WHERE product_id = ? AND id LIKE 'docA%%'`, moduleB).Scan(&leakBtoA); err != nil {
			rt.Fatalf("failed to check leak B->A: %v", err)
		}
		if leakBtoA != 0 {
			rt.Fatalf("found %d shop A documents leaking into shop B query", leakBtoA)
		}

		// --- Same isolation check for chunks ---
		var chunksA int
		if err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleA).Scan(&chunksA); err != nil {
			rt.Fatalf("failed to count chunks for shop A: %v", err)
		}
		if chunksA != docCountA {
			rt.Fatalf("expected %d chunks for shop A, got %d", docCountA, chunksA)
		}

		var chunkLeakAtoB int
		if err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ? AND id LIKE 'chunkB%%'`, moduleA).Scan(&chunkLeakAtoB); err != nil {
			rt.Fatalf("failed to check chunk leak A->B: %v", err)
		}
		if chunkLeakAtoB != 0 {
			rt.Fatalf("found %d shop B chunks leaking into shop A query", chunkLeakAtoB)
		}

		var chunksB int
		if err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ?`, moduleB).Scan(&chunksB); err != nil {
			rt.Fatalf("failed to count chunks for shop B: %v", err)
		}
		if chunksB != docCountB {
			rt.Fatalf("expected %d chunks for shop B, got %d", docCountB, chunksB)
		}

		var chunkLeakBtoA int
		if err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE product_id = ? AND id LIKE 'chunkA%%'`, moduleB).Scan(&chunkLeakBtoA); err != nil {
			rt.Fatalf("failed to check chunk leak B->A: %v", err)
		}
		if chunkLeakBtoA != 0 {
			rt.Fatalf("found %d shop A chunks leaking into shop B query", chunkLeakBtoA)
		}
	})
}
