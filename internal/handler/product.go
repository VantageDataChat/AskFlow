package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"askflow/internal/product"
)

// HandleProducts handles GET (list all) and POST (create) for products.
func HandleProducts(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// ?only_id=xxx — return only the product with this ID (for market customer chat)
			if onlyID := r.URL.Query().Get("only_id"); onlyID != "" {
				if !IsValidHexID(onlyID) {
					WriteError(w, http.StatusBadRequest, "invalid only_id")
					return
				}
				p, err := app.GetProduct(onlyID)
				if err != nil || p == nil {
					WriteJSON(w, http.StatusOK, map[string]interface{}{"products": []product.Product{}})
					return
				}
				WriteJSON(w, http.StatusOK, map[string]interface{}{"products": []product.Product{*p}})
				return
			}

			products, err := app.ListProducts()
			if err != nil {
				log.Printf("[Products] list error: %v", err)
				WriteError(w, http.StatusInternalServerError, "获取产品列表失败")
				return
			}
			if products == nil {
				products = []product.Product{}
			}

			// ?exclude_shop=1 — hide shop sub-products from the public list
			if r.URL.Query().Get("exclude_shop") == "1" && app.shopService != nil {
				shopIDs, err := app.shopService.ListShopProductIDs()
				if err == nil && len(shopIDs) > 0 {
					exclude := make(map[string]bool, len(shopIDs))
					for _, id := range shopIDs {
						exclude[id] = true
					}
					filtered := make([]product.Product, 0, len(products))
					for _, p := range products {
						if !exclude[p.ID] {
							filtered = append(filtered, p)
						}
					}
					products = filtered
				}
			}

			WriteJSON(w, http.StatusOK, map[string]interface{}{"products": products})

		case http.MethodPost:
			_, role, err := GetAdminSession(app, r)
			if err != nil {
				WriteAdminSessionError(w, err)
				return
			}
			if role != "super_admin" {
				WriteError(w, http.StatusForbidden, "仅超级管理员可管理产品")
				return
			}
			var req struct {
				Name                string `json:"name"`
				Type                string `json:"type"`
				Description         string `json:"description"`
				WelcomeMessage      string `json:"welcome_message"`
				AllowDownload       bool   `json:"allow_download"`
				ConversationEnabled bool   `json:"conversation_enabled"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			p, err := app.CreateProduct(req.Name, req.Type, req.Description, req.WelcomeMessage, req.AllowDownload, req.ConversationEnabled)
			if err != nil {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteJSON(w, http.StatusOK, p)

		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// HandleProductByID handles PUT (update) and DELETE for a specific product.
func HandleProductByID(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/products/")
		if id == "" || id == r.URL.Path {
			WriteError(w, http.StatusBadRequest, "missing product ID")
			return
		}
		if !IsValidHexID(id) {
			WriteError(w, http.StatusBadRequest, "invalid product ID")
			return
		}

		switch r.Method {
		case http.MethodPut:
			_, role, err := GetAdminSession(app, r)
			if err != nil {
				WriteAdminSessionError(w, err)
				return
			}
			if role != "super_admin" {
				WriteError(w, http.StatusForbidden, "仅超级管理员可管理产品")
				return
			}
			var req struct {
				Name                string `json:"name"`
				Type                string `json:"type"`
				Description         string `json:"description"`
				WelcomeMessage      string `json:"welcome_message"`
				AllowDownload       bool   `json:"allow_download"`
				ConversationEnabled bool   `json:"conversation_enabled"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			p, err := app.UpdateProduct(id, req.Name, req.Type, req.Description, req.WelcomeMessage, req.AllowDownload, req.ConversationEnabled)
			if err != nil {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteJSON(w, http.StatusOK, p)

		case http.MethodDelete:
			_, role, err := GetAdminSession(app, r)
			if err != nil {
				WriteAdminSessionError(w, err)
				return
			}
			if role != "super_admin" {
				WriteError(w, http.StatusForbidden, "仅超级管理员可管理产品")
				return
			}
			confirm := r.URL.Query().Get("confirm")
			if confirm != "true" {
				hasData, err := app.HasProductDocumentsOrKnowledge(id)
				if err != nil {
					log.Printf("[Products] check data error for %s: %v", id, err)
					WriteError(w, http.StatusInternalServerError, "检查产品数据失败")
					return
				}
				if hasData {
					WriteJSON(w, http.StatusConflict, map[string]interface{}{
						"warning":  "该产品下存在关联的文档或知识条目，确认删除？",
						"has_data": true,
					})
					return
				}
			}
			if err := app.DeleteProduct(id); err != nil {
				log.Printf("[Products] delete error for %s: %v", id, err)
				WriteError(w, http.StatusInternalServerError, "删除产品失败")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// HandleMyProducts returns products accessible to the current admin user.
func HandleMyProducts(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, _, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}
		products, err := app.GetProductsByAdminUserID(userID)
		if err != nil {
			log.Printf("[Products] get my products error: %v", err)
			WriteError(w, http.StatusInternalServerError, "获取产品列表失败")
			return
		}
		if products == nil {
			products = []product.Product{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{"products": products})
	}
}

// HandleDefaultProduct handles GET/PUT for the system-wide default product.
// GET returns the current default product ID.
// PUT sets the default product ID (super_admin only).
func HandleDefaultProduct(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg := app.configManager.Get()
			var dpID string
			if cfg != nil {
				dpID = cfg.Admin.DefaultProductID
			}
			WriteJSON(w, http.StatusOK, map[string]string{"default_product_id": dpID})

		case http.MethodPut:
			_, role, err := GetAdminSession(app, r)
			if err != nil {
				WriteAdminSessionError(w, err)
				return
			}
			if role != "super_admin" {
				WriteError(w, http.StatusForbidden, "仅超级管理员可设置默认产品")
				return
			}
			var req struct {
				ProductID string `json:"product_id"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			// Allow empty string to clear default
			if req.ProductID != "" {
				if !IsValidHexID(req.ProductID) {
					WriteError(w, http.StatusBadRequest, "invalid product_id")
					return
				}
				// Verify product exists
				p, err := app.GetProduct(req.ProductID)
				if err != nil || p == nil {
					WriteError(w, http.StatusBadRequest, "产品不存在")
					return
				}
			}
			if err := app.UpdateConfig(map[string]interface{}{
				"admin.default_product_id": req.ProductID,
			}); err != nil {
				WriteError(w, http.StatusInternalServerError, "设置默认产品失败")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// HandleProductIntro returns the product introduction/welcome message and conversation status.
func HandleProductIntro(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		
		cfg := app.configManager.Get()
		systemConversationEnabled := cfg != nil && cfg.Conversation.Enabled
		
		productID := r.URL.Query().Get("product_id")
		response := map[string]interface{}{
			"product_intro": "",
			"conversation_enabled": false,
		}
		
		if productID != "" {
			if !IsValidOptionalID(productID) {
				WriteError(w, http.StatusBadRequest, "invalid product_id")
				return
			}
			p, err := app.GetProduct(productID)
			if err != nil {
				// Log error but continue with system defaults
				log.Printf("[ProductIntro] WARNING: failed to get product %s: %v (using system defaults)", productID, err)
			} else if p != nil {
				intro := p.WelcomeMessage
				if intro == "" {
					intro = p.Description
				}
				response["product_intro"] = intro
				// Conversation is enabled only if both system and product settings are enabled
				response["conversation_enabled"] = systemConversationEnabled && p.ConversationEnabled
				WriteJSON(w, http.StatusOK, response)
				return
			}
		}
		
		// No specific product or product not found, return system intro and system conversation status
		if cfg != nil {
			response["product_intro"] = cfg.ProductIntro
		}
		response["conversation_enabled"] = systemConversationEnabled
		WriteJSON(w, http.StatusOK, response)
	}
}

// HandleAppInfo returns public app info (product_name, enabled OAuth providers) for frontend display.
func HandleAppInfo(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cfg := app.configManager.Get()
		providers := app.GetEnabledOAuthProviders()
		if providers == nil {
			providers = []string{}
		}
		var productName string
		var maxUploadSizeMB int
		if cfg != nil {
			productName = cfg.ProductName
			maxUploadSizeMB = cfg.Video.MaxUploadSizeMB
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"product_name":       productName,
			"oauth_providers":    providers,
			"max_upload_size_mb": maxUploadSizeMB,
		})
	}
}

// HandleTranslateProductName translates the product name to the requested language using LLM.
func HandleTranslateProductName(app *App) http.HandlerFunc {
	// Simple in-memory cache for translated product names (avoids LLM call on every page load)
	type cacheEntry struct {
		text    string
		expires time.Time
	}
	var cacheMu sync.Mutex
	cache := make(map[string]cacheEntry)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// Rate limiting (via apiRateLimit wrapper) prevents LLM abuse
		lang := r.URL.Query().Get("lang")
		// Validate lang parameter to prevent injection
		if len(lang) > 20 {
			WriteError(w, http.StatusBadRequest, "invalid language parameter")
			return
		}
		cfg := app.configManager.Get()
		if cfg == nil {
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": ""})
			return
		}
		name := cfg.ProductName
		if name == "" {
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": ""})
			return
		}
		if lang == "" {
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": name})
			return
		}

		// Check cache first
		cacheKey := name + "\x00" + lang
		cacheMu.Lock()
		if entry, ok := cache[cacheKey]; ok && time.Now().Before(entry.expires) {
			cacheMu.Unlock()
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": entry.text})
			return
		}
		// Evict expired entries if cache is getting large
		if len(cache) > 1000 {
			now := time.Now()
			for k, v := range cache {
				if now.After(v.expires) {
					delete(cache, k)
				}
			}
		}
		cacheMu.Unlock()

		// Use a timeout context to prevent slow LLM calls from blocking the page load
		// and to ensure the goroutine is cancelled when the timeout fires.
		llmCtx, llmCancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer llmCancel()
		type result struct {
			text string
			err  error
		}
		ch := make(chan result, 1)
		go func() {
			translated, err := app.queryEngine.TranslateText(name, lang)
			select {
			case ch <- result{translated, err}:
			case <-llmCtx.Done():
				// Context cancelled, discard result
			}
		}()
		select {
		case res := <-ch:
			if res.err != nil || res.text == "" {
				WriteJSON(w, http.StatusOK, map[string]string{"product_name": name})
				return
			}
			// Cache the result for 30 minutes
			cacheMu.Lock()
			cache[cacheKey] = cacheEntry{text: res.text, expires: time.Now().Add(30 * time.Minute)}
			cacheMu.Unlock()
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": res.text})
		case <-llmCtx.Done():
			// LLM too slow, return original name
			WriteJSON(w, http.StatusOK, map[string]string{"product_name": name})
		}
	}
}
