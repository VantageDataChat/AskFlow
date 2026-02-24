//go:build cgo

package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"askflow/internal/auth"
	"askflow/internal/shop"
)

// setupShopMiddlewareTestDB creates an in-memory SQLite database with the
// tables needed by the ShopIsolation middleware.
func setupShopMiddlewareTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id             TEXT PRIMARY KEY,
			email          TEXT UNIQUE,
			name           TEXT,
			provider       TEXT NOT NULL,
			provider_id    TEXT NOT NULL,
			password_hash  TEXT,
			email_verified INTEGER DEFAULT 0,
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login     DATETIME,
			default_product_id TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS sn_users (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			email          TEXT UNIQUE NOT NULL,
			display_name   TEXT,
			sn             TEXT,
			last_login_at  DATETIME,
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
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
		`CREATE TABLE IF NOT EXISTS admin_users (
			id            TEXT PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'editor',
			permissions   TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	return db
}

// seedApprovedShopOwner inserts a full chain of records: users, sn_users,
// products (parent + child), and an approved shop. Returns the session token
// and the expected shop_module_product_id.
func seedApprovedShopOwner(t *testing.T, db *sql.DB, sm *auth.SessionManager) (sessionToken string, moduleProductID string) {
	t.Helper()

	// 1. Insert parent product.
	db.Exec(`INSERT INTO products (id, name) VALUES ('parent-prod-1', 'Parent Product')`)

	// 2. Insert child product (shop module).
	moduleProductID = "child-prod-1"
	db.Exec(`INSERT INTO products (id, name, welcome_message) VALUES (?, 'Shop Module', 'welcome')`, moduleProductID)

	// 3. Insert sn_user.
	db.Exec(`INSERT INTO sn_users (email, display_name, sn) VALUES ('owner@test.com', 'Owner', 'SN-1')`)
	var ownerID int64
	db.QueryRow(`SELECT id FROM sn_users WHERE email = 'owner@test.com'`).Scan(&ownerID)

	// 4. Insert users record (provider='sn').
	userID := "sn_owner_1"
	db.Exec(`INSERT INTO users (id, email, name, provider, provider_id) VALUES (?, 'owner@test.com', 'Owner', 'sn', 'SN-1')`, userID)

	// 5. Insert approved shop.
	db.Exec(`INSERT INTO shops (id, name, owner_id, software_name, status, parent_product_id, shop_module_product_id)
		VALUES ('shop-1', 'Test Shop', ?, 'vantagics', 'approved', 'parent-prod-1', ?)`, ownerID, moduleProductID)

	// 6. Create session.
	session, err := sm.CreateSession(userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	return session.ID, moduleProductID
}

func TestShopIsolation_ApprovedShopOwner(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	token, expectedProductID := seedApprovedShopOwner(t, db, sm)

	var capturedProductID string
	var capturedIsOwner bool

	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		capturedProductID, _ = GetShopModuleProductID(r.Context())
		capturedIsOwner = IsShopOwner(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if capturedProductID != expectedProductID {
		t.Errorf("expected product ID %q, got %q", expectedProductID, capturedProductID)
	}
	if !capturedIsOwner {
		t.Error("expected IsShopOwner to be true")
	}
}

func TestShopIsolation_NoAuthHeader(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	called := false
	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Should have no shop context.
		if _, ok := GetShopModuleProductID(r.Context()); ok {
			t.Error("expected no shop_module_product_id in context")
		}
		if IsShopOwner(r.Context()) {
			t.Error("expected IsShopOwner to be false")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestShopIsolation_InvalidSession(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	called := false
	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if IsShopOwner(r.Context()) {
			t.Error("expected IsShopOwner to be false for invalid session")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-xyz")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestShopIsolation_NonSNUser(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	// Insert a non-SN user (provider='local').
	db.Exec(`INSERT INTO users (id, email, name, provider, provider_id) VALUES ('local_user_1', 'local@test.com', 'Local', 'local', 'local_user_1')`)
	session, _ := sm.CreateSession("local_user_1")

	called := false
	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if IsShopOwner(r.Context()) {
			t.Error("expected IsShopOwner to be false for non-SN user")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+session.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestShopIsolation_PendingShop(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	// Insert SN user with a pending (not approved) shop.
	db.Exec(`INSERT INTO products (id, name) VALUES ('parent-prod-2', 'Parent 2')`)
	db.Exec(`INSERT INTO sn_users (email, display_name, sn) VALUES ('pending@test.com', 'Pending', 'SN-2')`)
	var ownerID int64
	db.QueryRow(`SELECT id FROM sn_users WHERE email = 'pending@test.com'`).Scan(&ownerID)
	db.Exec(`INSERT INTO users (id, email, name, provider, provider_id) VALUES ('sn_pending_1', 'pending@test.com', 'Pending', 'sn', 'SN-2')`)
	db.Exec(`INSERT INTO shops (id, name, owner_id, software_name, status, parent_product_id, shop_module_product_id)
		VALUES ('shop-pending', 'Pending Shop', ?, 'vantagics', 'pending', 'parent-prod-2', '')`, ownerID)

	session, _ := sm.CreateSession("sn_pending_1")

	called := false
	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if IsShopOwner(r.Context()) {
			t.Error("expected IsShopOwner to be false for pending shop")
		}
		if _, ok := GetShopModuleProductID(r.Context()); ok {
			t.Error("expected no shop_module_product_id for pending shop")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+session.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestShopIsolation_AdminSession_StoreOwner(t *testing.T) {
	db := setupShopMiddlewareTestDB(t)
	defer db.Close()

	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	shopSvc := shop.NewShopService(db, db, nil, nil)

	// Set up: parent product, child product, sn_user, approved shop, admin_users record.
	moduleProductID := "child-prod-admin-1"
	db.Exec(`INSERT INTO products (id, name) VALUES ('parent-prod-admin', 'Parent')`)
	db.Exec(`INSERT INTO products (id, name, welcome_message) VALUES (?, 'Shop Module Admin', 'welcome')`, moduleProductID)
	db.Exec(`INSERT INTO sn_users (email, display_name, sn) VALUES ('admin_owner@test.com', 'AdminOwner', 'SN-A1')`)
	var ownerID int64
	db.QueryRow(`SELECT id FROM sn_users WHERE email = 'admin_owner@test.com'`).Scan(&ownerID)
	shopID := "shop-admin-1"
	db.Exec(`INSERT INTO shops (id, name, owner_id, software_name, status, parent_product_id, shop_module_product_id)
		VALUES (?, 'Admin Test Shop', ?, 'vantagics', 'approved', 'parent-prod-admin', ?)`, shopID, ownerID, moduleProductID)

	// Create admin_users record with username "store_{shopID}"
	subAdminID := "sub-admin-id-1"
	db.Exec(`INSERT INTO admin_users (id, username, password_hash, role, permissions) VALUES (?, ?, '$unusable$xxx', 'editor', 'documents,pending,knowledge,faq')`,
		subAdminID, "store_"+shopID)

	// Create admin session with UserID = "admin_" + subAdminID
	adminSession, err := sm.CreateSession("admin_" + subAdminID)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}

	var capturedProductID string
	var capturedIsOwner bool

	handler := ShopIsolation(sm, db, shopSvc)(func(w http.ResponseWriter, r *http.Request) {
		capturedProductID, _ = GetShopModuleProductID(r.Context())
		capturedIsOwner = IsShopOwner(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+adminSession.ID)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if capturedProductID != moduleProductID {
		t.Errorf("expected product ID %q, got %q", moduleProductID, capturedProductID)
	}
	if !capturedIsOwner {
		t.Error("expected IsShopOwner to be true for admin session store owner")
	}
}

func TestGetShopModuleProductID_EmptyContext(t *testing.T) {
	ctx := context.Background()
	id, ok := GetShopModuleProductID(ctx)
	if ok {
		t.Error("expected ok=false for empty context")
	}
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestIsShopOwner_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if IsShopOwner(ctx) {
		t.Error("expected false for empty context")
	}
}
