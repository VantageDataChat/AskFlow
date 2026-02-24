package shop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryStatus_Approved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: true, Message: "ok"})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	status, err := client.QueryStatus("vantagics", "test-shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Approved {
		t.Error("expected Approved to be true")
	}
	if status.Message != "ok" {
		t.Errorf("expected Message %q, got %q", "ok", status.Message)
	}
}

func TestQueryStatus_NotApproved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: false, Message: "pending review"})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	status, err := client.QueryStatus("vantagics", "test-shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Approved {
		t.Error("expected Approved to be false")
	}
	if status.Message != "pending review" {
		t.Errorf("expected Message %q, got %q", "pending review", status.Message)
	}
}

func TestQueryStatus_Non200StatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.QueryStatus("vantagics", "test-shop")
	if err == nil {
		t.Fatal("expected error for non-200 status code")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

func TestQueryStatus_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.QueryStatus("vantagics", "test-shop")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestQueryStatus_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: true, Message: "ok"})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	// Override with a very short timeout to trigger timeout error
	client.httpClient.Timeout = 50 * time.Millisecond

	_, err := client.QueryStatus("vantagics", "test-shop")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}

func TestQueryStatus_QueryParameters(t *testing.T) {
	var receivedSoftwareName, receivedShopName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSoftwareName = r.URL.Query().Get("software_name")
		receivedShopName = r.URL.Query().Get("shop_name")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShopStatus{Approved: true, Message: "ok"})
	}))
	defer srv.Close()

	client := NewMarketClient(srv.URL)
	_, err := client.QueryStatus("vantagics", "my-awesome-shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedSoftwareName != "vantagics" {
		t.Errorf("expected software_name %q, got %q", "vantagics", receivedSoftwareName)
	}
	if receivedShopName != "my-awesome-shop" {
		t.Errorf("expected shop_name %q, got %q", "my-awesome-shop", receivedShopName)
	}
}
