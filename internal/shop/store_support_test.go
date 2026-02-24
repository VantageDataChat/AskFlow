package shop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCheckStorefrontSupport_Approved tests the market client check for approved storefront.
func TestCheckStorefrontSupport_Approved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sfID := r.URL.Query().Get("storefront_id")
		if sfID != "42" {
			t.Errorf("expected storefront_id=42, got %q", sfID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StorefrontCheckResponse{
			Approved:       true,
			StoreName:      "Test Store",
			WelcomeMessage: "Welcome",
			SoftwareName:   "vantagics",
		})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	resp, err := client.CheckStorefrontSupport(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Approved {
		t.Error("expected Approved to be true")
	}
	if resp.StoreName != "Test Store" {
		t.Errorf("expected StoreName 'Test Store', got %q", resp.StoreName)
	}
	if resp.WelcomeMessage != "Welcome" {
		t.Errorf("expected WelcomeMessage 'Welcome', got %q", resp.WelcomeMessage)
	}
	if resp.SoftwareName != "vantagics" {
		t.Errorf("expected SoftwareName 'vantagics', got %q", resp.SoftwareName)
	}
}

// TestCheckStorefrontSupport_NotApproved tests the market client check for non-approved storefront.
func TestCheckStorefrontSupport_NotApproved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StorefrontCheckResponse{
			Approved: false,
			Status:   "pending",
		})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	resp, err := client.CheckStorefrontSupport(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Approved {
		t.Error("expected Approved to be false")
	}
	if resp.Status != "pending" {
		t.Errorf("expected Status 'pending', got %q", resp.Status)
	}
}

// TestCheckStorefrontSupport_StatusNone tests the market client check for "none" status.
func TestCheckStorefrontSupport_StatusNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StorefrontCheckResponse{
			Approved: false,
			Status:   "none",
		})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	resp, err := client.CheckStorefrontSupport(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Approved {
		t.Error("expected Approved to be false")
	}
	if resp.Status != "none" {
		t.Errorf("expected Status 'none', got %q", resp.Status)
	}
}

// TestCheckStorefrontSupport_StatusDisabled tests the market client check for "disabled" status.
func TestCheckStorefrontSupport_StatusDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StorefrontCheckResponse{
			Approved: false,
			Status:   "disabled",
		})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	resp, err := client.CheckStorefrontSupport(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Approved {
		t.Error("expected Approved to be false")
	}
	if resp.Status != "disabled" {
		t.Errorf("expected Status 'disabled', got %q", resp.Status)
	}
}

// TestCheckStorefrontSupport_ServerError tests the market client check with server error.
func TestCheckStorefrontSupport_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.CheckStorefrontSupport(42)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestCheckStorefrontSupport_InvalidJSON tests the market client check with invalid JSON response.
func TestCheckStorefrontSupport_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.CheckStorefrontSupport(42)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestCheckStorefrontSupport_QueryParams verifies the correct query parameters are sent.
func TestCheckStorefrontSupport_QueryParams(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StorefrontCheckResponse{Approved: true})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.CheckStorefrontSupport(123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/api/storefront-support/check" {
		t.Errorf("expected path /api/storefront-support/check, got %q", capturedPath)
	}
	if capturedQuery != "storefront_id=123" {
		t.Errorf("expected query storefront_id=123, got %q", capturedQuery)
	}
}

// TestMarketClient_QueryStatus tests the QueryStatus method.
func TestMarketClient_QueryStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sn := r.URL.Query().Get("software_name")
		name := r.URL.Query().Get("shop_name")
		if sn != "vantagics" {
			t.Errorf("expected software_name=vantagics, got %q", sn)
		}
		if name != "TestShop" {
			t.Errorf("expected shop_name=TestShop, got %q", name)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: true, Message: "ok"})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	status, err := client.QueryStatus("vantagics", "TestShop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Approved {
		t.Error("expected Approved to be true")
	}
	if status.Message != "ok" {
		t.Errorf("expected Message 'ok', got %q", status.Message)
	}
}

// TestMarketClient_QueryStatus_ServerError tests QueryStatus with server error.
func TestMarketClient_QueryStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.QueryStatus("vantagics", "TestShop")
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestModels_DefaultSoftwareName verifies the constant value.
func TestModels_DefaultSoftwareName(t *testing.T) {
	if DefaultSoftwareName != "vantagics" {
		t.Errorf("expected DefaultSoftwareName='vantagics', got %q", DefaultSoftwareName)
	}
}

// TestModels_StatusConstants verifies status constant values.
func TestModels_StatusConstants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("expected StatusPending='pending', got %q", StatusPending)
	}
	if StatusApproved != "approved" {
		t.Errorf("expected StatusApproved='approved', got %q", StatusApproved)
	}
	if StatusRejected != "rejected" {
		t.Errorf("expected StatusRejected='rejected', got %q", StatusRejected)
	}
}
