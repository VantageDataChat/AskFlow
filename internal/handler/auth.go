package handler

import (
	"log"
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
		if resp.Session != nil {
			SetSessionCookie(w, resp.Session.ID, 86400)
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
		var defaultProductID string
		if cfg != nil {
			loginRoute = cfg.Admin.LoginRoute
			anonymousMode = cfg.Admin.AnonymousMode
			anonymousFrontend = cfg.Admin.AnonymousFrontend
			defaultProductID = cfg.Admin.DefaultProductID
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"configured":         app.IsAdminConfigured(),
			"login_route":        loginRoute,
			"anonymous_mode":     anonymousMode,
			"anonymous_frontend": anonymousFrontend,
			"default_product_id": defaultProductID,
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
		if resp.Session != nil {
			SetSessionCookie(w, resp.Session.ID, 86400)
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
			log.Printf("[SNLogin] ReadJSONBody error: %v", err)
			WriteJSON(w, http.StatusBadRequest, SNLoginResponse{Success: false, Message: "token is required"})
			return
		}
		effectiveToken := req.GetToken()
		if effectiveToken == "" {
			log.Printf("[SNLogin] empty token: token=%q license_token=%q", req.Token, req.LicenseToken)
		}
		resp, status, err := app.HandleSNLogin(effectiveToken)
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
		log.Printf("[TicketLogin] incoming: scope=%q store_id=%q ticket_len=%d", scope, storeID, len(ticket))
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
// login ticket and returns the appropriate session based on scope.
//
// Scope behavior:
//   - scope=store  → validate ticket, resolve shop, create admin session
//   - scope=customer → validate ticket, create user session, attach store info
//   - (no scope)   → validate ticket, create user session (plain SN login)
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
		if err := ReadJSONBody(r, &req); err != nil || req.Ticket == "" {
			WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false, "message": "ticket is required",
			})
			return
		}

		log.Printf("[TicketExchange] scope=%q store_id=%d ticket_len=%d", req.Scope, req.StoreID, len(req.Ticket))

		// Step 1: Validate the one-time ticket (marks it as used atomically).
		ticketInfo, err := app.ValidateLoginTicket(req.Ticket)
		if err != nil {
			log.Printf("[TicketExchange] ValidateLoginTicket failed: err=%v scope=%q store_id=%d", err, req.Scope, req.StoreID)
			WriteJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"success": false, "message": err.Error(),
			})
			return
		}

		log.Printf("[TicketExchange] ticket valid: sn_user_id=%d email=%s", ticketInfo.SNUserID, ticketInfo.Email)

		// Step 2: Route by scope.
		switch req.Scope {
		case "store":
			handleStoreScope(w, app, ticketInfo, req.StoreID)
		case "customer":
			handleCustomerScope(w, app, ticketInfo, req.StoreID, req.Product)
		default:
			handlePlainScope(w, app, ticketInfo)
		}
	}
}

// handleStoreScope handles ticket-exchange for scope=store (shop owner → admin panel).
func handleStoreScope(w http.ResponseWriter, app *App, ti *TicketInfo, storeID int64) {
	if storeID <= 0 || app.shopService == nil {
		WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "message": "store_id is required for scope=store",
		})
		return
	}

	// Resolve the shop (by storefront_id, fallback to owner_id).
	foundShop, err := app.shopService.ResolveForLogin(storeID, ti.SNUserID)
	if err != nil {
		log.Printf("[TicketExchange/store] ResolveForLogin error: store_id=%d sn_user_id=%d err=%v", storeID, ti.SNUserID, err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "shop_resolve_error: " + err.Error(),
		})
		return
	}
	if foundShop == nil {
		log.Printf("[TicketExchange/store] no shop found: store_id=%d email=%s sn_user_id=%d", storeID, ti.Email, ti.SNUserID)
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":     false,
			"store_error": "未找到店铺记录，请先在市场申请开通客户支持",
		})
		return
	}

	log.Printf("[TicketExchange/store] shop found: id=%s name=%q status=%s module_product=%s",
		foundShop.ID, foundShop.Name, foundShop.Status, foundShop.ShopModuleProductID)

	// Create admin session for the store owner.
	sessionID, _, err := app.CreateStoreAdminSession(foundShop)
	if err != nil {
		log.Printf("[TicketExchange/store] CreateStoreAdminSession error: shop=%s err=%v", foundShop.ID, err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "admin_session_error: " + err.Error(),
		})
		return
	}

	session, err := app.sessionManager.ValidateSession(sessionID)
	if err != nil || session == nil {
		log.Printf("[TicketExchange/store] ValidateSession failed for just-created session: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "admin_session_error: session validation failed",
		})
		return
	}

	SetSessionCookie(w, session.ID, 86400) // 24h
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"admin_session": session,
		"admin_user": map[string]string{
			"username": foundShop.Name,
			"provider": "admin",
		},
		"store": map[string]interface{}{
			"store_id":               storeID,
			"store_name":             foundShop.Name,
			"welcome_message":        foundShop.WelcomeMessage,
			"scope":                  "store",
			"permissions":            []string{"documents", "pending", "knowledge", "faq"},
			"shop_module_product_id": foundShop.ShopModuleProductID,
			"parent_product_id":      foundShop.ParentProductID,
		},
	})
}

// handleCustomerScope handles ticket-exchange for scope=customer (end-user → chat).
func handleCustomerScope(w http.ResponseWriter, app *App, ti *TicketInfo, storeID int64, product string) {
	// Create a regular user session.
	sessionID, err := app.CreateUserSession(ti.Email, ti.DisplayName)
	if err != nil {
		log.Printf("[TicketExchange/customer] CreateUserSession error: email=%s err=%v", ti.Email, err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "user_session_error: " + err.Error(),
		})
		return
	}

	session, err := app.sessionManager.ValidateSession(sessionID)
	if err != nil || session == nil {
		log.Printf("[TicketExchange/customer] ValidateSession failed for just-created session: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "user_session_error: session validation failed",
		})
		return
	}

	SetSessionCookie(w, session.ID, 86400) // 24h

	resp := map[string]interface{}{
		"session": session,
		"user": map[string]string{
			"id":       session.UserID,
			"email":    ti.Email,
			"name":     ti.DisplayName,
			"provider": "sn",
		},
	}

	// Attach store info if available.
	if storeID > 0 && app.shopService != nil {
		foundShop, _ := app.shopService.GetByStorefrontID(storeID)
		if foundShop != nil && foundShop.Status == shop.StatusApproved && foundShop.ShopModuleProductID != "" {
			resp["store"] = map[string]interface{}{
				"store_id":               storeID,
				"store_name":             foundShop.Name,
				"welcome_message":        foundShop.WelcomeMessage,
				"scope":                  "customer",
				"product":                product,
				"shop_module_product_id": foundShop.ShopModuleProductID,
			}
		} else {
			resp["store_error"] = "该店铺未开通客户支持"
		}
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handlePlainScope handles ticket-exchange with no scope (plain SN login → chat).
func handlePlainScope(w http.ResponseWriter, app *App, ti *TicketInfo) {
	sessionID, err := app.CreateUserSession(ti.Email, ti.DisplayName)
	if err != nil {
		log.Printf("[TicketExchange/plain] CreateUserSession error: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "user_session_error: " + err.Error(),
		})
		return
	}

	session, err := app.sessionManager.ValidateSession(sessionID)
	if err != nil || session == nil {
		log.Printf("[TicketExchange/plain] ValidateSession failed for just-created session: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "message": "user_session_error: session validation failed",
		})
		return
	}

	SetSessionCookie(w, session.ID, 86400) // 24h
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"session": session,
		"user": map[string]string{
			"id":       session.UserID,
			"email":    ti.Email,
			"name":     ti.DisplayName,
			"provider": "sn",
		},
	})
}

// HandleReissueTicket handles POST /api/auth/reissue-ticket — creates a new
// one-time login ticket for the currently authenticated SN user.
// This is used by the iframe "expand" button: the iframe already has a valid
// session, but opening a new browser tab requires a fresh ticket because the
// original ticket was consumed when the iframe loaded.
func HandleReissueTicket(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		userID, err := GetUserSession(app, r)
		if err != nil {
			WriteJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"success": false, "message": "未登录",
			})
			return
		}

		ticket, err := app.ReissueTicketFromSession(userID)
		if err != nil {
			log.Printf("[ReissueTicket] failed for user_id=%s: %v", userID, err)
			WriteJSON(w, http.StatusForbidden, map[string]interface{}{
				"success": false, "message": "cannot reissue ticket",
			})
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"ticket":  ticket,
		})
	}
}
