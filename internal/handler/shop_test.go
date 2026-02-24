//go:build cgo

package handler

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"pgregory.net/rapid"

	"askflow/internal/auth"
	"askflow/internal/config"
)

// setupShopAuthTestDB creates an in-memory SQLite database with the tables
// required by the HandleShopAuth → HandleSNLogin → ValidateLoginTicket flow.
func setupShopAuthTestDB(t *testing.T) *sql.DB {
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
		`CREATE TABLE IF NOT EXISTS login_tickets (
			ticket      TEXT PRIMARY KEY,
			user_id     INTEGER NOT NULL,
			used        INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at  DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES sn_users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS login_attempts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ip         TEXT NOT NULL,
			success    INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	return db
}

// newMockAuthServer creates a TLS test server that mimics the AuthServer
// marketplace-verify endpoint, returning success with a generated email/sn.
func newMockAuthServer() *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "bad request",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"sn":      "SN-" + req.Token,
			"email":   req.Token + "@test.shop",
		})
	}))
}

// newTestConfigManager creates a ConfigManager with the given AuthServer host.
func newTestConfigManager(t *testing.T, authServerHost string) *config.ConfigManager {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	configPath := t.TempDir() + "/config.json"
	cm, err := config.NewConfigManagerWithKey(configPath, key)
	if err != nil {
		t.Fatalf("create config manager: %v", err)
	}
	if err := cm.Update(map[string]interface{}{
		"auth_server": authServerHost,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	return cm
}

// Feature: shop-support, Property 1: 有效 Token 认证产生有效会话
// Validates: Requirements 1.1, 1.2
func TestProperty_ValidTokenProducesValidSession(t *testing.T) {
	// Set up shared resources outside rapid.Check to avoid per-iteration overhead.
	db := setupShopAuthTestDB(t)
	defer db.Close()

	authServer := newMockAuthServer()
	defer authServer.Close()

	// Override http.DefaultTransport to trust the test TLS certificate.
	origTransport := http.DefaultTransport
	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	defer func() { http.DefaultTransport = origTransport }()

	authHost := authServer.URL[len("https://"):]
	cm := newTestConfigManager(t, authHost)
	sm := auth.NewSessionManager(db, db, 24*time.Hour)
	app := NewApp(db, db, nil, nil, nil, nil, sm, cm, nil, nil, nil, nil)

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random valid token (alphanumeric, 8-32 chars).
		token := rapid.StringMatching(`[a-zA-Z0-9]{8,32}`).Draw(rt, "token")

		// Build the HTTP request to HandleShopAuth.
		body, _ := json.Marshal(ShopAuthRequest{Token: token})
		req := httptest.NewRequest(http.MethodPost, "/api/shop/auth", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		// Call the handler.
		HandleShopAuth(app)(rec, req)

		// Decode response.
		var resp ShopAuthResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			rt.Fatalf("decode response: %v", err)
		}

		// Property: authentication must succeed.
		if !resp.Success {
			rt.Fatalf("expected success=true, got false; message=%s", resp.Message)
		}

		// Property: login_ticket must be non-empty.
		if resp.LoginTicket == "" {
			rt.Fatal("expected non-empty login_ticket")
		}

		// Validate the login ticket produces valid ticket info.
		ticketInfo, err := app.ValidateLoginTicket(resp.LoginTicket)
		if err != nil {
			rt.Fatalf("ValidateLoginTicket failed: %v", err)
		}
		if ticketInfo == nil {
			rt.Fatal("expected non-nil ticket info")
		}
		if ticketInfo.Email == "" {
			rt.Fatal("expected non-empty email in ticket info")
		}

		// Create a user session from the ticket info.
		sessionID, err := app.CreateUserSession(ticketInfo.Email, ticketInfo.DisplayName)
		if err != nil {
			rt.Fatalf("CreateUserSession failed: %v", err)
		}
		if sessionID == "" {
			rt.Fatal("expected non-empty session ID")
		}

		// Property: the session must be valid via SessionManager.
		session, err := sm.ValidateSession(sessionID)
		if err != nil {
			rt.Fatalf("ValidateSession failed: %v", err)
		}
		if session == nil {
			rt.Fatal("expected non-nil session")
		}
		if session.ID != sessionID {
			rt.Fatalf("session ID mismatch: got %s, want %s", session.ID, sessionID)
		}
	})
}
