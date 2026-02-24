package handler

import (
	"log"
	"net/http"
	"strings"

	"askflow/internal/shop"
)

// HandleStoreSupportRegister handles POST /api/store-support/register.
// Marketplace calls this to register a store's support system in Service Portal.
//
// Flow: VerifySNToken → FindOrCreateSNUser → shopService.Register → FindOrCreateStoreOwnerAdmin
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
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{Success: false, Message: "token is required"})
			return
		}
		if strings.TrimSpace(req.SoftwareName) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{Success: false, Message: "software_name is required"})
			return
		}
		if strings.TrimSpace(req.StoreName) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{Success: false, Message: "store_name is required"})
			return
		}
		if strings.TrimSpace(req.WelcomeMessage) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{Success: false, Message: "welcome_message is required"})
			return
		}
		if strings.TrimSpace(req.ParentProductID) == "" {
			WriteJSON(w, http.StatusBadRequest, shop.RegisterResponse{Success: false, Message: "parent_product_id is required"})
			return
		}

		// Step 1: Verify token with license server (no side effects).
		tokenInfo, err := app.VerifySNToken(req.Token)
		if err != nil {
			log.Printf("[StoreSupportRegister] VerifySNToken failed: %v", err)
			WriteJSON(w, http.StatusUnauthorized, shop.RegisterResponse{
				Success: false,
				Message: "token 验证失败",
			})
			return
		}

		// Step 2: Find or create the SN user to get owner_id.
		ownerID, err := app.FindOrCreateSNUser(tokenInfo.Email, tokenInfo.SN)
		if err != nil {
			log.Printf("[StoreSupportRegister] FindOrCreateSNUser error: %v", err)
			WriteJSON(w, http.StatusInternalServerError, shop.RegisterResponse{
				Success: false,
				Message: "internal error",
			})
			return
		}

		// Step 3: Register the shop.
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

		// Step 4: Create sub-admin account for the store owner.
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
// Uses VerifySNToken directly — no ticket/session side effects.
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

		// Authenticate with license server (no side effects).
		if strings.TrimSpace(req.Token) == "" {
			WriteJSON(w, http.StatusUnauthorized, shop.UpdateWelcomeResponse{
				Success: false,
				Message: "token is required",
			})
			return
		}
		if _, err := app.VerifySNToken(req.Token); err != nil {
			log.Printf("[StoreSupportUpdateWelcome] VerifySNToken failed: %v", err)
			WriteJSON(w, http.StatusUnauthorized, shop.UpdateWelcomeResponse{
				Success: false,
				Message: "token 验证失败",
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
