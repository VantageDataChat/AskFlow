package handler

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"askflow/internal/shop"
)

// ShopAuthRequest is the request body for POST /api/shop/auth.
type ShopAuthRequest struct {
	Token string `json:"token"`
}

// ShopAuthResponse is the response for POST /api/shop/auth.
type ShopAuthResponse struct {
	Success     bool   `json:"success"`
	LoginTicket string `json:"login_ticket,omitempty"`
	Message     string `json:"message,omitempty"`
}

// HandleShopAuth handles shop owner authentication by reusing the existing
// SN Token verification mechanism (marketplace-verify request to AuthServer).
// POST /api/shop/auth
func HandleShopAuth(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req ShopAuthRequest
		if err := ReadJSONBody(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, ShopAuthResponse{
				Success: false,
				Message: "token is required",
			})
			return
		}

		if req.Token == "" {
			WriteJSON(w, http.StatusBadRequest, ShopAuthResponse{
				Success: false,
				Message: "token is required",
			})
			return
		}

		// Reuse the existing SN login flow which verifies the token with
		// AuthServer via marketplace-verify, finds/creates the user, and
		// generates a one-time login ticket.
		snResp, status, err := app.HandleSNLogin(req.Token)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, ShopAuthResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		WriteJSON(w, status, ShopAuthResponse{
			Success:     snResp.Success,
			LoginTicket: snResp.LoginTicket,
			Message:     snResp.Message,
		})
	}
}

// shopActivateBody is the JSON body for POST /api/shop/activate.
type shopActivateBody struct {
	SoftwareName    string `json:"software_name"`
	ShopName        string `json:"shop_name"`
	Description     string `json:"description"`
	ParentProductID string `json:"parent_product_id"`
}

// HandleShopActivate handles POST /api/shop/activate — submit a shop activation request.
// It reads the current user from the session, resolves the sn_users.id as OwnerID,
// and delegates to ShopService.Activate.
func HandleShopActivate(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// 1. Authenticate: get user ID from session.
		userID, err := GetUserSession(app, r)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "未登录")
			return
		}

		// 2. Parse JSON body.
		var body shopActivateBody
		if err := ReadJSONBody(r, &body); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// 3. Validate required fields.
		if strings.TrimSpace(body.SoftwareName) == "" || strings.TrimSpace(body.ShopName) == "" {
			WriteError(w, http.StatusBadRequest, "software_name and shop_name are required")
			return
		}

		// 4. Resolve sn_users.id from the session user.
		//    Session stores users.id (regularUserID). We look up the email
		//    (provider='sn'), then find the corresponding sn_users.id.
		var email string
		err = app.readDB.QueryRow(
			"SELECT email FROM users WHERE id = ? AND provider = 'sn'", userID,
		).Scan(&email)
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusForbidden, "非店铺用户")
			return
		}
		if err != nil {
			log.Printf("[ShopActivate] query user email: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var ownerID int64
		err = app.readDB.QueryRow(
			"SELECT id FROM sn_users WHERE email = ?", email,
		).Scan(&ownerID)
		if err != nil {
			log.Printf("[ShopActivate] query sn_user: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// 5. Build ActivateRequest and call service.
		req := shop.ActivateRequest{
			OwnerID:         ownerID,
			SoftwareName:    body.SoftwareName,
			ShopName:        body.ShopName,
			Description:     body.Description,
			ParentProductID: body.ParentProductID,
		}

		resp, err := app.shopService.Activate(req)
		if err != nil {
			log.Printf("[ShopActivate] service error: %v", err)
			// Distinguish validation errors from internal errors.
			errMsg := err.Error()
			if strings.Contains(errMsg, "required") ||
				strings.Contains(errMsg, "不能为空") {
				WriteError(w, http.StatusBadRequest, errMsg)
				return
			}
			if strings.Contains(errMsg, "not found") ||
				strings.Contains(errMsg, "不存在") {
				WriteError(w, http.StatusNotFound, errMsg)
				return
			}
			WriteError(w, http.StatusInternalServerError, errMsg)
			return
		}

		// 6. Create sub-admin account for the store owner
		if resp.Shop != nil && resp.ShopModuleProductID != "" {
			if _, saErr := app.FindOrCreateStoreOwnerAdmin(resp.Shop.ID, resp.Shop.Name, resp.ShopModuleProductID); saErr != nil {
				log.Printf("[ShopActivate] sub-admin creation failed: %v", saErr)
			}
		}

		// 7. Return the ActivateResponse as JSON.
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleShopInfo handles GET /api/shop/info — shop owner retrieves their own shop info.
func HandleShopInfo(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// 1. Authenticate: get user ID from session.
		userID, err := GetUserSession(app, r)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "未登录")
			return
		}

		// 2. Resolve sn_users.id from the session user.
		var email string
		err = app.readDB.QueryRow(
			"SELECT email FROM users WHERE id = ? AND provider = 'sn'", userID,
		).Scan(&email)
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusForbidden, "非店铺用户")
			return
		}
		if err != nil {
			log.Printf("[ShopInfo] query user email: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var ownerID int64
		err = app.readDB.QueryRow(
			"SELECT id FROM sn_users WHERE email = ?", email,
		).Scan(&ownerID)
		if err != nil {
			log.Printf("[ShopInfo] query sn_user: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// 3. Fetch shop by owner ID.
		s, err := app.shopService.GetByOwnerID(ownerID)
		if err != nil {
			log.Printf("[ShopInfo] service error: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if s == nil {
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"success": false,
				"message": "未找到店铺",
			})
			return
		}

		WriteJSON(w, http.StatusOK, s)
	}
}

// HandleAdminShops handles GET /api/admin/shops?product_id=xxx — admin lists shops under a product.
func HandleAdminShops(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// Admin permission check.
		_, _, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}

		productID := r.URL.Query().Get("product_id")
		if productID == "" {
			WriteError(w, http.StatusBadRequest, "product_id is required")
			return
		}

		var shops []shop.Shop
		var err2 error
		if productID == "all" {
			shops, err2 = app.shopService.ListAll()
		} else {
			shops, err2 = app.shopService.ListByProductID(productID)
		}
		if err2 != nil {
			log.Printf("[AdminShops] list shops error: %v", err2)
			WriteError(w, http.StatusInternalServerError, "获取店铺列表失败")
			return
		}
		if shops == nil {
			shops = []shop.Shop{}
		}

		// Auto-fix any pending shops (legacy data from before no-approval logic)
		for i := range shops {
			if shops[i].Status == "pending" {
				if fixErr := app.shopService.FixPendingShop(&shops[i]); fixErr != nil {
					log.Printf("[AdminShops] auto-fix pending shop %s failed: %v", shops[i].ID, fixErr)
				} else {
					// Also ensure sub-admin account exists for the fixed shop
					_, _ = app.FindOrCreateStoreOwnerAdmin(shops[i].ID, shops[i].Name, shops[i].ShopModuleProductID)
				}
			}
		}

		// Build product ID → name map for display
		productNameMap := make(map[string]string)
		if products, pErr := app.ListProducts(); pErr == nil {
			for _, p := range products {
				productNameMap[p.ID] = p.Name
			}
		}

		// Build response with parent_product_name
		type shopView struct {
			shop.Shop
			ParentProductName string `json:"parent_product_name"`
		}
		result := make([]shopView, len(shops))
		for i, s := range shops {
			result[i] = shopView{
				Shop:              s,
				ParentProductName: productNameMap[s.ParentProductID],
			}
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{"shops": result})
	}
}

// HandleAdminShopDelete handles DELETE /api/admin/shops/{id}?retain_knowledge=true — admin deletes a shop.
func HandleAdminShopDelete(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// Admin permission check.
		_, _, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}

		// Extract shop ID from URL path: /api/admin/shops/{id}
		shopID := strings.TrimPrefix(r.URL.Path, "/api/admin/shops/")
		if shopID == "" || shopID == r.URL.Path {
			WriteError(w, http.StatusBadRequest, "missing shop ID")
			return
		}

		// Get retain_knowledge query parameter (default false).
		retainKnowledge := r.URL.Query().Get("retain_knowledge") == "true"

		if err := app.shopService.Delete(shopID, retainKnowledge); err != nil {
			log.Printf("[AdminShopDelete] delete shop error: %v", err)
			errMsg := err.Error()
			if strings.Contains(errMsg, "not found") {
				WriteError(w, http.StatusNotFound, errMsg)
				return
			}
			WriteError(w, http.StatusInternalServerError, "删除店铺失败")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "店铺已删除",
		})
	}
}

