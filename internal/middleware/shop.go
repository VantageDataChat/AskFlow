package middleware

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"

	"askflow/internal/auth"
	"askflow/internal/shop"
)

// contextKey is an unexported type for context keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey string

const (
	// shopModuleProductIDKey is the context key for the shop's module product ID.
	shopModuleProductIDKey contextKey = "shop_module_product_id"
	// isShopOwnerKey is the context key indicating whether the request is from a shop owner.
	isShopOwnerKey contextKey = "is_shop_owner"
)

// GetShopModuleProductID extracts the shop_module_product_id from the request context.
// Returns the product ID and true if present, or empty string and false otherwise.
func GetShopModuleProductID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(shopModuleProductIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// IsShopOwner checks if the current request is from an authenticated shop owner.
func IsShopOwner(ctx context.Context) bool {
	v, ok := ctx.Value(isShopOwnerKey).(bool)
	return ok && v
}

// WithShopOwnerContext returns a new context with shop owner values set.
// This is intended for use in tests and internal handler logic.
func WithShopOwnerContext(ctx context.Context, shopModuleProductID string) context.Context {
	ctx = context.WithValue(ctx, shopModuleProductIDKey, shopModuleProductID)
	ctx = context.WithValue(ctx, isShopOwnerKey, true)
	return ctx
}

// ShopIsolation returns a middleware that resolves the shop owner's
// shop_module_product_id from the session and injects it into the request
// context. Non-shop users (no session, no SN user, no approved shop) pass
// through without shop context — they are not blocked.
//
// The middleware performs the following steps:
//  1. Extract session token from Authorization header
//  2. Validate session to get user_id
//  3. Look up the user's email (provider='sn') from the users table
//  4. Look up sn_users.id from the email
//  5. Call ShopService.GetByOwnerID to find the shop
//  6. If shop exists and is approved: inject shop_module_product_id into context
//  7. Otherwise: pass through without shop context
func ShopIsolation(sessionMgr *auth.SessionManager, readDB *sql.DB, shopSvc *shop.ShopService) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 1. Extract session token from Authorization header, with cookie fallback.
			authHeader := r.Header.Get("Authorization")
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" || token == authHeader {
				// Fallback: read from session cookie (iframe cross-origin scenario).
				if c, err := r.Cookie("session_id"); err == nil && c.Value != "" {
					token = c.Value
				} else {
					// No valid token — pass through as non-shop user.
					next(w, r)
					return
				}
			}

			// 2. Validate session.
			session, err := sessionMgr.ValidateSession(token)
			if err != nil {
				// Invalid/expired session — pass through.
				next(w, r)
				return
			}

			// 3. Resolve the shop for this session.
			//    Two paths: (a) admin session for a store owner ("admin_xxx" → admin_users
			//    username "store_{shopID}"), or (b) regular SN user session.
			var s *shop.Shop

			if strings.HasPrefix(session.UserID, "admin_") {
				// Path (a): Admin session — look up admin_users to find "store_{shopID}" username.
				subID := strings.TrimPrefix(session.UserID, "admin_")
				var username string
				err = readDB.QueryRow(
					"SELECT username FROM admin_users WHERE id = ?", subID,
				).Scan(&username)
				if err != nil {
					// Not a known admin sub-account — pass through.
					next(w, r)
					return
				}
				if strings.HasPrefix(username, "store_") {
					shopID := strings.TrimPrefix(username, "store_")
					s, err = shopSvc.GetByID(shopID)
					if err != nil {
						log.Printf("[ShopIsolation] failed to get shop by ID %s: %v", shopID, err)
						next(w, r)
						return
					}
				} else {
					// Non-store admin — pass through.
					next(w, r)
					return
				}
			} else {
				// Path (b): Regular SN user session — resolve via email → sn_users → shop.
				var email string
				err = readDB.QueryRow(
					"SELECT email FROM users WHERE id = ? AND provider = 'sn'",
					session.UserID,
				).Scan(&email)
				if err != nil {
					// Not an SN user — pass through.
					next(w, r)
					return
				}

				var ownerID int64
				err = readDB.QueryRow(
					"SELECT id FROM sn_users WHERE email = ?", email,
				).Scan(&ownerID)
				if err != nil {
					log.Printf("[ShopIsolation] failed to resolve sn_user for email %s: %v", email, err)
					next(w, r)
					return
				}

				s, err = shopSvc.GetByOwnerID(ownerID)
				if err != nil {
					log.Printf("[ShopIsolation] failed to get shop for owner %d: %v", ownerID, err)
					next(w, r)
					return
				}
			}

			// 6. If shop exists and is approved, inject shop_module_product_id.
			if s != nil && s.Status == shop.StatusApproved && s.ShopModuleProductID != "" {
				ctx = context.WithValue(ctx, shopModuleProductIDKey, s.ShopModuleProductID)
				ctx = context.WithValue(ctx, isShopOwnerKey, true)
				next(w, r.WithContext(ctx))
				return
			}

			// 7. No approved shop — pass through without shop context.
			next(w, r)
		}
	}
}
