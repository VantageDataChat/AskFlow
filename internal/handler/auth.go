package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"askflow/internal/captcha"
	"askflow/internal/middleware"
	"askflow/internal/shop"
)

// --- OAuth handlers ---

// HandleOAuthURL returns the OAuth authorization URL for the given provider.
func HandleOAuthURL(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		provider := r.URL.Query().Get("provider")
		if provider == "" {
			WriteError(w, http.StatusBadRequest, "missing provider parameter")
			return
		}
		url, err := app.GetOAuthURL(provider)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"url": url})
	}
}

// HandleOAuthCallback exchanges the auth code for user info and creates a session.
func HandleOAuthCallback(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Provider string `json:"provider"`
			Code     string `json:"code"`
			State    string `json:"state"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		// Validate OAuth state to prevent CSRF (state is required)
		if req.State == "" || !app.oauthClient.ValidateState(req.State) {
			WriteError(w, http.StatusBadRequest, "invalid or expired OAuth state")
			return
		}
		resp, err := app.HandleOAuthCallback(req.Provider, req.Code)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleOAuthProviderDelete removes an OAuth provider configuration.
func HandleOAuthProviderDelete(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// Require admin session
		_, role, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}
		if role != "super_admin" {
			WriteError(w, http.StatusForbidden, "无权限")
			return
		}
		// Extract provider name from URL: /api/oauth/providers/{name}
		provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/providers/")
		if provider == "" {
			WriteError(w, http.StatusBadRequest, "missing provider name")
			return
		}
		// Validate provider name to prevent injection
		if len(provider) > 50 || strings.ContainsAny(provider, "/<>\"'\\") {
			WriteError(w, http.StatusBadRequest, "invalid provider name")
			return
		}
		if err := app.DeleteOAuthProvider(provider); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// --- Admin login handler ---

// HandleAdminLogin authenticates an admin user with username, password, and captcha.
func HandleAdminLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Username      string `json:"username"`
			Password      string `json:"password"`
			CaptchaID     string `json:"captcha_id"`
			CaptchaAnswer string `json:"captcha_answer"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		// Try image captcha store first (captcha package), then text captcha store (app)
		captchaValid := captcha.Validate(req.CaptchaID, req.CaptchaAnswer)
		if !captchaValid {
			// Fallback: try text captcha store (answer is numeric string)
			if ans, err := strconv.Atoi(req.CaptchaAnswer); err == nil {
				captchaValid = ValidateCaptcha(req.CaptchaID, ans)
			}
		}
		if !captchaValid {
			WriteError(w, http.StatusBadRequest, "验证码错误")
			return
		}
		resp, err := app.AdminLogin(req.Username, req.Password, middleware.GetClientIP(r))
		if err != nil {
			WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleAdminSetup sets up the initial admin account.
func HandleAdminSetup(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		resp, err := app.AdminSetup(req.Username, req.Password)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleAnonymousLogin allows anonymous read-only access to the admin panel when enabled.
func HandleAnonymousLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		resp, err := app.AnonymousLogin()
		if err != nil {
			WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleAnonymousFrontendLogin allows anonymous access to the user-facing chat when enabled.
func HandleAnonymousFrontendLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		resp, err := app.AnonymousFrontendLogin()
		if err != nil {
			WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleAdminStatus returns whether the admin account has been configured.
func HandleAdminStatus(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cfg := app.configManager.Get()
		var loginRoute string
		var anonymousMode bool
		var anonymousFrontend bool
		if cfg != nil {
			loginRoute = cfg.Admin.LoginRoute
			anonymousMode = cfg.Admin.AnonymousMode
			anonymousFrontend = cfg.Admin.AnonymousFrontend
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"configured":         app.IsAdminConfigured(),
			"login_route":        loginRoute,
			"anonymous_mode":     anonymousMode,
			"anonymous_frontend": anonymousFrontend,
		})
	}
}

// --- User registration & login handlers ---

// HandleCaptcha generates a math captcha (text-based).
func HandleCaptcha() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cap := GenerateCaptcha()
		WriteJSON(w, http.StatusOK, cap)
	}
}

// HandleCaptchaImage generates an image captcha.
func HandleCaptchaImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cap := captcha.Generate()
		WriteJSON(w, http.StatusOK, cap)
	}
}

// HandleRegister creates a new user account with captcha validation.
func HandleRegister(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			RegisterRequest
			CaptchaID     string `json:"captcha_id"`
			CaptchaAnswer int    `json:"captcha_answer"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !ValidateCaptcha(req.CaptchaID, req.CaptchaAnswer) {
			WriteError(w, http.StatusBadRequest, "验证码错误")
			return
		}
		baseURL := GetBaseURL(r)
		if err := app.Register(req.RegisterRequest, baseURL); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "注册成功，请查收验证邮件"})
	}
}

// HandleUserPreferences handles GET/PUT for user default product preference.
func HandleUserPreferences(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := GetUserSession(app, r)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		switch r.Method {
		case http.MethodGet:
			defaultProductID, err := app.GetUserDefaultProduct(userID)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "获取用户偏好失败")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"default_product_id": defaultProductID})
		case http.MethodPut:
			var req struct {
				DefaultProductID string `json:"default_product_id"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if err := app.SetUserDefaultProduct(userID, req.DefaultProductID); err != nil {
				WriteError(w, http.StatusInternalServerError, "保存用户偏好失败")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// HandleUserLogin authenticates a user with email, password, and captcha.
func HandleUserLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Email         string `json:"email"`
			Password      string `json:"password"`
			CaptchaID     string `json:"captcha_id"`
			CaptchaAnswer int    `json:"captcha_answer"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !ValidateCaptcha(req.CaptchaID, req.CaptchaAnswer) {
			WriteError(w, http.StatusBadRequest, "验证码错误")
			return
		}
		resp, err := app.UserLogin(req.Email, req.Password, middleware.GetClientIP(r))
		if err != nil {
			WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleVerifyEmail verifies a user's email using a token from the URL.
func HandleVerifyEmail(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		token := r.URL.Query().Get("token")
		// Validate token format (32 hex chars)
		if len(token) != 32 || !IsValidHexID(token) {
			WriteError(w, http.StatusBadRequest, "无效的验证链接")
			return
		}
		if err := app.VerifyEmail(token); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "邮箱验证成功，请登录"})
	}
}

// HandleForgotPassword handles POST /api/auth/forgot-password — sends a password reset email.
func HandleForgotPassword(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Email string `json:"email"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		baseURL := GetBaseURL(r)
		if err := app.RequestPasswordReset(req.Email, baseURL); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "如果该邮箱已注册，重置链接将发送到您的邮箱"})
	}
}

// HandleResetPassword handles POST /api/auth/reset-password — resets the password using a token.
func HandleResetPassword(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if len(req.Token) != 32 || !IsValidHexID(req.Token) {
			WriteError(w, http.StatusBadRequest, "无效的重置链接")
			return
		}
		if err := app.ResetPassword(req.Token, req.Password); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "密码重置成功，请登录"})
	}
}

// HandleSNLogin handles POST /api/auth/sn-login — verifies a license server token
// and returns a one-time login ticket.
func HandleSNLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req SNLoginRequest
		if err := ReadJSONBody(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, SNLoginResponse{Success: false, Message: "token is required"})
			return
		}
		resp, status, err := app.HandleSNLogin(req.Token)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, SNLoginResponse{Success: false, Message: "internal error"})
			return
		}
		WriteJSON(w, status, resp)
	}
}

// HandleTicketLogin handles GET /auth/ticket-login?ticket=xxx — redirects to the
// SPA with the ticket as a query parameter so the frontend can exchange it via JS.
func HandleTicketLogin(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Redirect(w, r, "/login?error=method_not_allowed", http.StatusFound)
			return
		}
		ticket := r.URL.Query().Get("ticket")
		if ticket == "" || len(ticket) > 128 {
			http.Redirect(w, r, "/login?error=invalid_ticket", http.StatusFound)
			return
		}
		// Validate ticket contains only safe characters (hex + dashes)
		for _, c := range ticket {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || c == '-') {
				http.Redirect(w, r, "/login?error=invalid_ticket", http.StatusFound)
				return
			}
		}

		// Build redirect URL with ticket
		redirectURL := "/?ticket=" + ticket

		// Support scope and store_id parameters for store management sessions
		scope := r.URL.Query().Get("scope")
		storeID := r.URL.Query().Get("store_id")
		if (scope == "store" || scope == "customer") && storeID != "" {
			// Validate store_id contains only digits
			for _, c := range storeID {
				if c < '0' || c > '9' {
					http.Redirect(w, r, "/login?error=invalid_store_id", http.StatusFound)
					return
				}
			}
			redirectURL += "&scope=" + scope + "&store_id=" + storeID

			// For customer scope, pass the product parameter (URL-encoded product name)
			if scope == "customer" {
				product := r.URL.Query().Get("product")
				if product != "" {
					redirectURL += "&product=" + url.QueryEscape(product)
				}
			}
		}

		// Pass ticket (and optional scope/store_id) to frontend — the SPA will
		// call /api/auth/ticket-exchange to validate it and store the session.
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// HandleTicketExchange handles POST /api/auth/ticket-exchange — validates a one-time
// login ticket and returns {session, user} JSON for the frontend to store in localStorage.
func HandleTicketExchange(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Ticket  string `json:"ticket"`
			Scope   string `json:"scope"`
			StoreID int64  `json:"store_id"`
			Product string `json:"product"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false, "message": "ticket is required",
			})
			return
		}
		if req.Ticket == "" {
			WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false, "message": "ticket is required",
			})
			return
		}

		sessionID, err := app.ValidateLoginTicket(req.Ticket)
		if err != nil {
			status := http.StatusUnauthorized
			WriteJSON(w, status, map[string]interface{}{
				"success": false, "message": err.Error(),
			})
			return
		}

		// Fetch session details
		session, err := app.sessionManager.ValidateSession(sessionID)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"success": false, "message": "internal error",
			})
			return
		}

		// Fetch user info
		var email, name, provider string
		_ = app.readDB.QueryRow(
			"SELECT COALESCE(email,''), COALESCE(name,''), COALESCE(provider,'') FROM users WHERE id = ?",
			session.UserID,
		).Scan(&email, &name, &provider)

		resp := map[string]interface{}{
			"session": session,
			"user": map[string]string{
				"id":       session.UserID,
				"email":    email,
				"name":     name,
				"provider": provider,
			},
		}

		// If scope=store and store_id is provided, create store management admin session
		if req.Scope == "store" && req.StoreID > 0 && app.shopService != nil {
			var foundShop *shop.Shop

			// First try to find by storefront_id
			foundShop, _ = app.shopService.GetByStorefrontID(req.StoreID)

			// If not found by storefront_id, try by owner_id and link the storefront_id
			if foundShop == nil {
				var ownerID int64
				err := app.readDB.QueryRow(
					"SELECT id FROM sn_users WHERE email = ?", email,
				).Scan(&ownerID)
				if err == nil {
					foundShop, _ = app.shopService.GetByOwnerID(ownerID)
					if foundShop != nil {
						_ = app.shopService.SetStorefrontID(ownerID, req.StoreID)
					}
				}
			} else if foundShop.StorefrontID == 0 {
				// Shop found but storefront_id not yet set — link it
				var ownerID int64
				err := app.readDB.QueryRow(
					"SELECT id FROM sn_users WHERE email = ?", email,
				).Scan(&ownerID)
				if err == nil {
					_ = app.shopService.SetStorefrontID(ownerID, req.StoreID)
				}
			}

			if foundShop != nil {
				// Create an admin session for the store owner so they can access the admin panel
				storeOwnerID := "store_owner_" + foundShop.ID
				_ = app.sessionManager.DeleteSession(sessionID) // clean up the regular user session
				adminSession, err := app.sessionManager.CreateSession(storeOwnerID)
				if err != nil {
					WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
						"success": false, "message": "internal error",
					})
					return
				}

				resp["admin_session"] = adminSession
				resp["admin_user"] = map[string]string{
					"username": foundShop.Name,
					"provider": "store_owner",
				}
				resp["store"] = map[string]interface{}{
					"store_id":               req.StoreID,
					"store_name":             foundShop.Name,
					"welcome_message":        foundShop.WelcomeMessage,
					"scope":                  "store",
					"permissions":            []string{"documents", "pending", "knowledge", "faq"},
					"shop_module_product_id": foundShop.ShopModuleProductID,
					"parent_product_id":      foundShop.ParentProductID,
				}
			}
		}

		// If scope=customer and store_id is provided, include customer view info
		if req.Scope == "customer" && req.StoreID > 0 && app.shopService != nil {
			foundShop, _ := app.shopService.GetByStorefrontID(req.StoreID)
			if foundShop != nil && foundShop.Status == shop.StatusApproved {
				resp["store"] = map[string]interface{}{
					"store_id":               req.StoreID,
					"store_name":             foundShop.Name,
					"welcome_message":        foundShop.WelcomeMessage,
					"scope":                  "customer",
					"product":                req.Product,
					"shop_module_product_id": foundShop.ShopModuleProductID,
				}
			} else {
				resp["store_error"] = "该店铺未开通客户支持"
			}
		}

		WriteJSON(w, http.StatusOK, resp)
	}
}
