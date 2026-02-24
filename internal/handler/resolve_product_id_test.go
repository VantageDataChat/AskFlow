package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"askflow/internal/middleware"
	"pgregory.net/rapid"
)

func TestResolveProductID_NonShopOwner_PassThrough(t *testing.T) {
	// Regular (non-shop-owner) request: product_id passes through unchanged.
	r := httptest.NewRequest(http.MethodGet, "/test?product_id=abc123", nil)
	pid, err := resolveProductID(r, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != "abc123" {
		t.Fatalf("expected abc123, got %s", pid)
	}
}

func TestResolveProductID_NonShopOwner_EmptyProductID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	pid, err := resolveProductID(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != "" {
		t.Fatalf("expected empty, got %s", pid)
	}
}

func TestResolveProductID_ShopOwner_ForcesShopProductID(t *testing.T) {
	shopPID := "shop_product_abc"
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := middleware.WithShopOwnerContext(r.Context(), shopPID)
	r = r.WithContext(ctx)

	// Empty requested product_id → forced to shop's product_id.
	pid, err := resolveProductID(r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != shopPID {
		t.Fatalf("expected %s, got %s", shopPID, pid)
	}
}

func TestResolveProductID_ShopOwner_SameProductID_OK(t *testing.T) {
	shopPID := "shop_product_abc"
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := middleware.WithShopOwnerContext(r.Context(), shopPID)
	r = r.WithContext(ctx)

	// Requested product_id matches shop's → allowed.
	pid, err := resolveProductID(r, shopPID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != shopPID {
		t.Fatalf("expected %s, got %s", shopPID, pid)
	}
}

func TestResolveProductID_ShopOwner_DifferentProductID_Forbidden(t *testing.T) {
	shopPID := "shop_product_abc"
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := middleware.WithShopOwnerContext(r.Context(), shopPID)
	r = r.WithContext(ctx)

	// Requested product_id differs from shop's → 403.
	_, err := resolveProductID(r, "other_product_xyz")
	if err == nil {
		t.Fatal("expected error for cross-shop access, got nil")
	}
	if err.Error() != "权限不足" {
		t.Fatalf("expected '权限不足', got %q", err.Error())
	}
}

// Feature: shop-support, Property 9: 店铺主人操作限定在自身模块范围
// Validates: Requirements 5.2, 5.3, 5.4, 5.5, 5.6
func TestProperty9_ShopOwnerOperationsLimitedToOwnModule(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random non-empty shop_module_product_id.
		shopModuleProductID := rapid.StringMatching(`[a-f0-9]{8,32}`).Draw(t, "shop_module_product_id")

		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := middleware.WithShopOwnerContext(r.Context(), shopModuleProductID)
		r = r.WithContext(ctx)

		// Sub-property A: empty requested product_id → forced to shop's product_id.
		pid, err := resolveProductID(r, "")
		if err != nil {
			t.Fatalf("unexpected error for empty product_id: %v", err)
		}
		if pid != shopModuleProductID {
			t.Fatalf("expected %s, got %s", shopModuleProductID, pid)
		}

		// Sub-property B: matching requested product_id → allowed, returns shop's product_id.
		pid, err = resolveProductID(r, shopModuleProductID)
		if err != nil {
			t.Fatalf("unexpected error for matching product_id: %v", err)
		}
		if pid != shopModuleProductID {
			t.Fatalf("expected %s, got %s", shopModuleProductID, pid)
		}

		// Sub-property C: different requested product_id → returns error.
		otherProductID := rapid.StringMatching(`[a-f0-9]{8,32}`).
			Filter(func(s string) bool { return s != shopModuleProductID }).
			Draw(t, "other_product_id")

		_, err = resolveProductID(r, otherProductID)
		if err == nil {
			t.Fatalf("expected error for cross-shop access with product_id=%s, got nil", otherProductID)
		}
		if err.Error() != "权限不足" {
			t.Fatalf("expected '权限不足', got %q", err.Error())
		}
	})
}

// Feature: shop-support, Property 10: 跨店铺访问被拒绝
// Validates: Requirements 5.7
func TestProperty10_CrossShopAccessDenied(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate two distinct shop_module_product_ids (shopA and shopB).
		shopA := rapid.StringMatching(`[a-f0-9]{8,32}`).Draw(t, "shopA_product_id")
		shopB := rapid.StringMatching(`[a-f0-9]{8,32}`).
			Filter(func(s string) bool { return s != shopA }).
			Draw(t, "shopB_product_id")

		// Create a request with shop owner context set to shopA's product_id.
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := middleware.WithShopOwnerContext(r.Context(), shopA)
		r = r.WithContext(ctx)

		// Call resolveProductID with shopB's product_id — should be denied.
		_, err := resolveProductID(r, shopB)
		if err == nil {
			t.Fatalf("expected permission denied error when shopA (product_id=%s) accesses shopB (product_id=%s), got nil", shopA, shopB)
		}
		if err.Error() != "权限不足" {
			t.Fatalf("expected error message '权限不足', got %q", err.Error())
		}
	})
}

