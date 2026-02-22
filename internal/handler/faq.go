package handler

import (
	"net/http"
	"strings"

	"askflow/internal/faq"
)

// HandleFAQ handles GET (list top FAQ for a product).
// Public endpoint — any authenticated user can view FAQ.
func HandleFAQ(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		productID := r.URL.Query().Get("product_id")
		if productID == "" {
			firstID, err := app.GetFirstProductID()
			if err == nil && firstID != "" {
				productID = firstID
			}
		}
		if productID != "" && !IsValidOptionalID(productID) {
			WriteError(w, http.StatusBadRequest, "invalid product_id")
			return
		}
		if productID == "" {
			WriteJSON(w, http.StatusOK, map[string]interface{}{"faqs": []interface{}{}})
			return
		}
		faqs, err := app.ListFAQ(productID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "获取常见问题失败")
			return
		}
		if faqs == nil {
			WriteJSON(w, http.StatusOK, map[string]interface{}{"faqs": []interface{}{}})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{"faqs": faqs})
	}
}

// HandleFAQAdmin handles admin CRUD for FAQ entries.
// GET /api/admin/faq?product_id=xxx — list all FAQ for a product (admin, no weight filter)
// POST /api/admin/faq — create a new FAQ entry manually
// PUT /api/admin/faq/reorder — reorder FAQ entries
func HandleFAQAdminList(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}
		if role != "super_admin" && role != "editor" && role != "anonymous_viewer" {
			WriteError(w, http.StatusForbidden, "无权限")
			return
		}

		switch r.Method {
		case http.MethodGet:
			productID := r.URL.Query().Get("product_id")
			if productID != "" && !IsValidOptionalID(productID) {
				WriteError(w, http.StatusBadRequest, "invalid product_id")
				return
			}
			if productID == "" {
				WriteJSON(w, http.StatusOK, map[string]interface{}{"faqs": []interface{}{}})
				return
			}
			faqs, err := app.ListAllFAQ(productID)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "获取FAQ列表失败")
				return
			}
			if faqs == nil {
				faqs = []faq.Entry{}
			}
			WriteJSON(w, http.StatusOK, map[string]interface{}{"faqs": faqs})

		case http.MethodPost:
			if role == "anonymous_viewer" {
				WriteError(w, http.StatusForbidden, "此为参观模式，一切更改都不会生效")
				return
			}
			var req struct {
				ProductID string `json:"product_id"`
				Question  string `json:"question"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if req.ProductID == "" || !IsValidOptionalID(req.ProductID) {
				WriteError(w, http.StatusBadRequest, "invalid product_id")
				return
			}
			entry, err := app.CreateFAQ(req.ProductID, req.Question)
			if err != nil {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteJSON(w, http.StatusOK, entry)

		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// HandleFAQAdminReorder handles PUT /api/admin/faq/reorder — reorder FAQ entries.
func HandleFAQAdminReorder(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		_, role, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}
		if role != "super_admin" && role != "editor" {
			WriteError(w, http.StatusForbidden, "无权限")
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := app.ReorderFAQ(req.IDs); err != nil {
			WriteError(w, http.StatusInternalServerError, "排序失败")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// HandleFAQAdminByID handles PUT (update) and DELETE for a specific FAQ entry.
// PUT /api/admin/faq/{id} — update question text
// DELETE /api/admin/faq/{id} — delete entry
func HandleFAQAdminByID(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}
		if role != "super_admin" && role != "editor" {
			WriteError(w, http.StatusForbidden, "无权限")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/admin/faq/")
		if id == "" || id == r.URL.Path || !IsValidHexID(id) {
			WriteError(w, http.StatusBadRequest, "invalid FAQ ID")
			return
		}

		switch r.Method {
		case http.MethodDelete:
			if err := app.DeleteFAQ(id); err != nil {
				WriteError(w, http.StatusInternalServerError, "删除失败")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		case http.MethodPut:
			var req struct {
				Question string `json:"question"`
			}
			if err := ReadJSONBody(r, &req); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if err := app.UpdateFAQ(id, req.Question); err != nil {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

		default:
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
