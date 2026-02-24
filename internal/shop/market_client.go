package shop

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// ShopStatus represents the approval status returned by the Market service.
type ShopStatus struct {
	Approved bool   `json:"approved"`
	Message  string `json:"message"`
}

// MarketClient is an HTTP client for querying the external Market service.
type MarketClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewMarketClient creates a MarketClient with a 10-second timeout.
func NewMarketClient(baseURL string) *MarketClient {
	return &MarketClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// QueryStatus queries the Market service for the shop's approval status.
func (c *MarketClient) QueryStatus(softwareName, shopName string) (*ShopStatus, error) {
	u, err := url.Parse(c.baseURL + "/api/shop/status")
	if err != nil {
		return nil, fmt.Errorf("market client: invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("software_name", softwareName)
	q.Set("shop_name", shopName)
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		log.Printf("market client: request failed: %v", err)
		return nil, fmt.Errorf("market service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("market client: unexpected status code %d for shop %q", resp.StatusCode, shopName)
		return nil, fmt.Errorf("market service returned status %d", resp.StatusCode)
	}

	var status ShopStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status); err != nil {
		log.Printf("market client: failed to decode response: %v", err)
		return nil, fmt.Errorf("market client: failed to decode response: %w", err)
	}

	return &status, nil
}

// CheckStorefrontSupport queries the Marketplace's /api/storefront-support/check
// endpoint to verify whether a storefront has been approved for support.
func (c *MarketClient) CheckStorefrontSupport(storefrontID int64) (*StorefrontCheckResponse, error) {
	u, err := url.Parse(c.baseURL + "/api/storefront-support/check")
	if err != nil {
		return nil, fmt.Errorf("market client: invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("storefront_id", fmt.Sprintf("%d", storefrontID))
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		log.Printf("market client: storefront check request failed: %v", err)
		return nil, fmt.Errorf("market service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("market client: storefront check returned status %d", resp.StatusCode)
		return nil, fmt.Errorf("market service returned status %d", resp.StatusCode)
	}

	var result StorefrontCheckResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		log.Printf("market client: failed to decode storefront check response: %v", err)
		return nil, fmt.Errorf("market client: failed to decode response: %w", err)
	}

	return &result, nil
}
