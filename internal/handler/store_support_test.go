package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"askflow/internal/shop"
)

// TestStoreSupportRegister_MethodNotAllowed tests register with wrong HTTP method.
func TestStoreSupportRegister_MethodNotAllowed(t *testing.T) {
	// Handler checks method before any DB access, so nil App is fine.
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/api/store-support/register", nil)
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// TestStoreSupportRegister_MissingToken tests register with missing token.
func TestStoreSupportRegister_MissingToken(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.RegisterRequest{
		Token:          "",
		SoftwareName:   "vantagics",
		StoreName:      "测试店铺",
		WelcomeMessage: "欢迎",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp shop.RegisterResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected success=false")
	}
}

// TestStoreSupportRegister_MissingStoreName tests register with missing store_name.
func TestStoreSupportRegister_MissingStoreName(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.RegisterRequest{
		Token:          "sometoken",
		SoftwareName:   "vantagics",
		StoreName:      "",
		WelcomeMessage: "欢迎",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportRegister_MissingSoftwareName tests register with missing software_name.
func TestStoreSupportRegister_MissingSoftwareName(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.RegisterRequest{
		Token:          "sometoken",
		SoftwareName:   "",
		StoreName:      "测试店铺",
		WelcomeMessage: "欢迎",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportRegister_MissingWelcomeMessage tests register with missing welcome_message.
func TestStoreSupportRegister_MissingWelcomeMessage(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.RegisterRequest{
		Token:          "sometoken",
		SoftwareName:   "vantagics",
		StoreName:      "测试店铺",
		WelcomeMessage: "",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportRegister_InvalidJSON tests register with invalid JSON body.
func TestStoreSupportRegister_InvalidJSON(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/register", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportRegister(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportUpdateWelcome_MethodNotAllowed tests update-welcome with wrong HTTP method.
func TestStoreSupportUpdateWelcome_MethodNotAllowed(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/api/store-support/update-welcome", nil)
	rec := httptest.NewRecorder()

	HandleStoreSupportUpdateWelcome(app)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// TestStoreSupportUpdateWelcome_MissingStorefrontID tests update-welcome with missing storefront_id.
func TestStoreSupportUpdateWelcome_MissingStorefrontID(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.UpdateWelcomeRequest{
		StorefrontID:   0,
		WelcomeMessage: "新欢迎语",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/update-welcome", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportUpdateWelcome(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportUpdateWelcome_MissingWelcomeMessage tests update-welcome with empty message.
func TestStoreSupportUpdateWelcome_MissingWelcomeMessage(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.UpdateWelcomeRequest{
		StorefrontID:   42,
		WelcomeMessage: "",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/update-welcome", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportUpdateWelcome(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportUpdateWelcome_InvalidJSON tests update-welcome with invalid JSON body.
func TestStoreSupportUpdateWelcome_InvalidJSON(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/update-welcome", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportUpdateWelcome(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestStoreSupportUpdateWelcome_NegativeStorefrontID tests update-welcome with negative storefront_id.
func TestStoreSupportUpdateWelcome_NegativeStorefrontID(t *testing.T) {
	app := &App{}

	body, _ := json.Marshal(shop.UpdateWelcomeRequest{
		StorefrontID:   -1,
		WelcomeMessage: "新欢迎语",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/store-support/update-welcome", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	HandleStoreSupportUpdateWelcome(app)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// --- TicketLogin tests (pure redirect logic, no DB needed) ---

// TestTicketLogin_WithScopeAndStoreID tests that ticket-login passes scope and store_id to frontend.
func TestTicketLogin_WithScopeAndStoreID(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=abc123def456&scope=store&store_id=42", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	expected := "/?ticket=abc123def456&scope=store&store_id=42"
	if location != expected {
		t.Errorf("expected redirect to %q, got %q", expected, location)
	}
}

// TestTicketLogin_WithoutScope tests that ticket-login without scope preserves original behavior.
func TestTicketLogin_WithoutScope(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=abc123def456", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	expected := "/?ticket=abc123def456"
	if location != expected {
		t.Errorf("expected redirect to %q, got %q", expected, location)
	}
}

// TestTicketLogin_InvalidStoreID tests that ticket-login rejects invalid store_id.
func TestTicketLogin_InvalidStoreID(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=abc123def456&scope=store&store_id=abc", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/login?error=invalid_store_id" {
		t.Errorf("expected redirect to /login?error=invalid_store_id, got %q", location)
	}
}

// TestTicketLogin_EmptyTicket tests that ticket-login rejects empty ticket.
func TestTicketLogin_EmptyTicket(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/login?error=invalid_ticket" {
		t.Errorf("expected redirect to /login?error=invalid_ticket, got %q", location)
	}
}

// TestTicketLogin_MethodNotAllowed tests that ticket-login rejects POST.
func TestTicketLogin_MethodNotAllowed(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodPost, "/auth/ticket-login?ticket=abc123", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/login?error=method_not_allowed" {
		t.Errorf("expected redirect to /login?error=method_not_allowed, got %q", location)
	}
}

// TestTicketLogin_ScopeStoreWithoutStoreID tests scope=store without store_id falls back to normal.
func TestTicketLogin_ScopeStoreWithoutStoreID(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=abc123def456&scope=store", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	// Without store_id, scope=store is ignored
	expected := "/?ticket=abc123def456"
	if location != expected {
		t.Errorf("expected redirect to %q, got %q", expected, location)
	}
}

// TestTicketLogin_InvalidTicketChars tests that ticket-login rejects tickets with invalid characters.
func TestTicketLogin_InvalidTicketChars(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=ABC123", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/login?error=invalid_ticket" {
		t.Errorf("expected redirect to /login?error=invalid_ticket, got %q", location)
	}
}

// TestTicketLogin_LargeStoreID tests that ticket-login accepts large numeric store_id.
func TestTicketLogin_LargeStoreID(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest(http.MethodGet, "/auth/ticket-login?ticket=abc123def456&scope=store&store_id=999999999", nil)
	rec := httptest.NewRecorder()

	HandleTicketLogin(app)(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	expected := "/?ticket=abc123def456&scope=store&store_id=999999999"
	if location != expected {
		t.Errorf("expected redirect to %q, got %q", expected, location)
	}
}
