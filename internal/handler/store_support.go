package handler

import (
	"log"
	"net/http"
	"strings"

	"askflow/internal/shop"
)

// HandleStoreSupportRegister handles POST /api/store-support/register.
// Marketplace calls this to register a store's support system in Service Portal.
func HandleStoreSupportRegister(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req shop.RegisterRequest
		if err := ReadJSONBody(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "invalid request body",
			})
			return
		}

		// Validate required fields
		if strings.TrimSpace(req.Token) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "token is required",
			})
			return
		}
		if strings.TrimSpace(req.SoftwareName) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "software_name is required",
			})
			return
		}
		if strings.TrimSpace(req.StoreName) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "store_name is required",
			})
			return
		}
		if strings.TrimSpace(req.WelcomeMessage) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "welcome_message is required",
			})
			return
		}
		if strings.TrimSpace(req.ParentProductID) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{
				Success: false,
				Message: "parent_product_id is required",
			})
			return
		}

		// Verify token using the existing SN login flow (which calls License_Server)
		snResp, status, err := app.HandleSNLogin(req.Token)
		if err != nil {
			log.Printf("[StoreSupportRegister] SN login error: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}
		if !snResp.Success {
			WriteJSON(w, status, shop.RegisterResponse{
				Success: false,
				Message: "token 验证失败",
			})
			return
		}

		// Find the sn_user by looking up the login ticket we just created
		// The HandleSNLogin creates a ticket; we need the owner_id (sn_users.id).
		// We can get it by validating the ticket, which gives us the user.
		sessionID, err := app.ValidateLoginTicket(snResp.LoginTicket)
		if err != nil {
			log.Printf("[StoreSupportRegister] ticket validation error: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		// Get the user from the session to find the sn_user
		session, err := app.sessionManager.ValidateSession(sessionID)
		if err != nil {
			log.Printf("[StoreSupportRegister] session validation error: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		// Look up the email from users table, then find sn_users.id
		var email string
		err = app.readDB.QueryRow(
			"SELECT email FROM users WHERE id = ? AND provider = 'sn'", session.UserID,
		).Scan(&email)
		if err != nil {
			log.Printf("[StoreSupportRegister] query user email: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		var ownerID int64
		err = app.readDB.QueryRow(
			"SELECT id FROM sn_users WHERE email = ?", email,
		).Scan(&ownerID)
		if err != nil {
			log.Printf("[StoreSupportRegister] query sn_user: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		// Clean up the session we just created (it was only for token verification)
		_ = app.sessionManager.DeleteSession(sessionID)

		// Register the shop
		log.Printf("[StoreSupportRegister] registering shop for owner_id=%d, store=%q, parent_product_id=%q",
			ownerID, req.StoreName, req.ParentProductID)
		if err := app.shopService.Register(ownerID, req); err != nil {
			log.Printf("[StoreSupportRegister] register error: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		// Create sub-admin account for the store owner right after registration
		if registeredShop, err := app.shopService.GetByOwnerID(ownerID); err == nil && registeredShop != nil {
			if _, saErr := app.FindOrCreateStoreOwnerAdmin(registeredShop.ID, registeredShop.Name, registeredShop.ShopModuleProductID); saErr != nil {
				log.Printf("[StoreSupportRegister] sub-admin creation failed for shop %s: %v", registeredShop.ID, saErr)
			}
		}

		WriteJSON(w, http.StatusOK, shop.RegisterResponse{
			Success: true,
			Message: "注册成功",
		})
	}
}

// HandleStoreSupportUpdateWelcome handles POST /api/store-support/update-welcome.
// Marketplace calls this to sync the welcome message when a store's description changes.
func HandleStoreSupportUpdateWelcome(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req shop.UpdateWelcomeRequest
		if err := ReadJSONBody(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, shop.UpdateWelcomeResponse{
				Success: false,
				Message: "invalid request body",
			})
			return
		}

		if req.StorefrontID <= 0 {
			WriteJSON(w, http.StatusBadRequest, shop.UpdateWelcomeResponse{
				Success: false,
				Message: "storefront_id is required",
			})
			return
		}

		if strings.TrimSpace(req.WelcomeMessage) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.UpdateWelcomeResponse{
				Success: false,
				Message: "welcome_message is required",
			})
			return
		}

		if err := app.shopService.UpdateWelcomeMessage(req.StorefrontID, req.WelcomeMessage); err != nil {
			log.Printf("[StoreSupportUpdateWelcome] update error: %v", err)
			errMsg := err.Error()
			status := http.StatusInternalServerError
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "未找到") {
				status = http.StatusNotFound
			}
			WriteJSON(w, status, shop.UpdateWelcomeResponse{
				Success: false,
				Message: errMsg,
			})
			return
		}

		WriteJSON(w, http.StatusOK, shop.UpdateWelcomeResponse{
			Success: true,
		})
	}
}
