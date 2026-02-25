package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"askflow/internal/loginlog"
	"askflow/internal/middleware"
)

// resolveProductID returns the effective product_id for the request.
// For shop owners, it enforces their shop_module_product_id:
//   - If the request includes a product_id that differs from the shop's, returns a 403 error.
//   - Public documents (product_id="") are not accessible to shop owners.
//   - Otherwise, forces the product_id to the shop's shop_module_product_id.
//
// For non-shop users (admins, regular users), the requestedProductID is returned as-is.
func resolveProductID(r *http.Request, requestedProductID string) (string, error) {
	if !middleware.IsShopOwner(r.Context()) {
		return requestedProductID, nil
	}
	shopProductID, ok := middleware.GetShopModuleProductID(r.Context())
	if !ok {
		return "", fmt.Errorf("权限不足")
	}
	// Shop owners cannot access public documents (empty product_id).
	if requestedProductID == "" {
		return "", fmt.Errorf("权限不足")
	}
	if requestedProductID != shopProductID {
		return "", fmt.Errorf("权限不足")
	}
	return shopProductID, nil
}

// resolveShopListProductID resolves the product_id for list/query endpoints.
// Unlike resolveProductID, an empty requestedProductID is treated as "unspecified"
// and automatically filled with the shop's product ID (instead of being rejected).
// For non-shop users, the requestedProductID is returned as-is.
func resolveShopListProductID(r *http.Request, requestedProductID string) (string, error) {
	if !middleware.IsShopOwner(r.Context()) {
		return requestedProductID, nil
	}
	shopProductID, ok := middleware.GetShopModuleProductID(r.Context())
	if !ok {
		return "", fmt.Errorf("权限不足")
	}
	if requestedProductID != "" && requestedProductID != shopProductID {
		return "", fmt.Errorf("权限不足")
	}
	return shopProductID, nil
}

// ForbiddenError represents a 403 Forbidden error, distinct from 401 Unauthorized.
type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string {
	return e.Message
}

// GetBaseURL derives the public base URL from the request, respecting
// X-Forwarded-Proto for reverse-proxy setups.
// X-Forwarded-Host is validated to prevent host header injection.
func GetBaseURL(r *http.Request) string {
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		// Validate forwarded host: must not contain path separators or whitespace
		if !strings.ContainsAny(fwdHost, "/ \t\r\n") {
			host = fwdHost
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd == "https" || fwd == "http" {
		scheme = fwd
	}
	return scheme + "://" + host
}

// WriteJSON encodes data as JSON and writes it to the response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a JSON error response with the given status code and message.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// ReadJSONBody decodes the request body as JSON into v.
// It validates Content-Type, limits body size to 1MB, and rejects trailing data.
func ReadJSONBody(r *http.Request, v interface{}) error {
	// Validate content type
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		return fmt.Errorf("expected Content-Type application/json")
	}
	defer r.Body.Close()
	// Limit request body to 1MB to prevent large payload attacks
	limited := io.LimitReader(r.Body, 1<<20)
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(v); err != nil {
		return err
	}
	// Ensure no trailing data (prevents request smuggling)
	if decoder.More() {
		return fmt.Errorf("unexpected trailing data in request body")
	}
	return nil
}

// GetUserSession validates the Authorization bearer token and returns the user ID.
// sessionCookieName is the cookie name used for session tokens in iframe scenarios.
const sessionCookieName = "session_id"

// SetSessionCookie sets a session cookie with SameSite=None and Secure so that
// cross-origin iframes (e.g. market.vantagics.com embedding service.vantagics.com)
// can send the session token automatically.
func SetSessionCookie(w http.ResponseWriter, sessionID string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	})
}

// extractSessionToken reads the session token from the Authorization header first,
// falling back to the session_id cookie for cross-origin iframe scenarios.
func extractSessionToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != "" && token != authHeader {
		return token
	}
	// Fallback: read from cookie (iframe cross-origin scenario)
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

func GetUserSession(app *App, r *http.Request) (string, error) {
	token := extractSessionToken(r)
	if token == "" {
		return "", fmt.Errorf("未登录")
	}
	session, err := app.sessionManager.ValidateSession(token)
	if err != nil {
		return "", fmt.Errorf("会话已过期")
	}
	return session.UserID, nil
}

// GetAdminSession validates the session and checks if it's an admin session.
// Returns (userID, role, error). role is "super_admin", "editor", or "anonymous_viewer".
// Anonymous viewers are restricted to GET requests only.
func GetAdminSession(app *App, r *http.Request) (string, string, error) {
	token := extractSessionToken(r)
	if token == "" {
		return "", "", fmt.Errorf("未登录")
	}
	session, err := app.sessionManager.ValidateSession(token)
	if err != nil {
		loginlog.Log(loginlog.EventSessionExpired, "unknown", middleware.GetClientIP(r), "admin_session")
		return "", "", fmt.Errorf("会话无效")
	}
	if !app.IsAdminSession(session.UserID) {
		return "", "", fmt.Errorf("无权限")
	}
	role := app.GetAdminRole(session.UserID)
	if role == "" {
		return "", "", fmt.Errorf("无权限")
	}
	// Anonymous viewers can only perform read operations
	if role == "anonymous_viewer" && r.Method != http.MethodGet {
		return "", "", &ForbiddenError{Message: "此为参观模式，一切更改都不会生效"}
	}
	return session.UserID, role, nil
}

// WriteAdminSessionError writes the appropriate HTTP error for a GetAdminSession failure.
// Returns 403 for ForbiddenError (anonymous write rejection), 401 for all other errors.
func WriteAdminSessionError(w http.ResponseWriter, err error) {
	if _, ok := err.(*ForbiddenError); ok {
		WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	WriteError(w, http.StatusUnauthorized, err.Error())
}

// IsValidHexID checks if the given string is a valid 32-character lowercase hex ID.
func IsValidHexID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// IsValidVideoMagicBytes checks if the file data starts with known video format magic bytes.
func IsValidVideoMagicBytes(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	// MP4/MOV: starts with ftyp box (offset 4)
	if string(data[4:8]) == "ftyp" {
		return true
	}
	// AVI: starts with RIFF....AVI
	if string(data[0:4]) == "RIFF" && len(data) >= 12 && string(data[8:12]) == "AVI " {
		return true
	}
	// MKV/WebM: starts with EBML header (0x1A 0x45 0xDF 0xA3)
	if data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return true
	}
	return false
}

// IsValidAudioMagicBytes checks if the file data starts with known audio format magic bytes.
func IsValidAudioMagicBytes(data []byte, fileType string) bool {
	if len(data) < 12 {
		return false
	}
	switch fileType {
	case "mp3":
		// MP3: starts with ID3 tag or MPEG sync word (0xFF 0xFB/0xF3/0xF2)
		if len(data) >= 3 && string(data[:3]) == "ID3" {
			return true
		}
		if data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
			return true
		}
	case "m4a":
		// M4A: MPEG-4 container, starts with ftyp box (same as MP4)
		if string(data[4:8]) == "ftyp" {
			return true
		}
	case "wav":
		// WAV: RIFF....WAVE
		if string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
			return true
		}
	case "flac":
		// FLAC: starts with "fLaC"
		if len(data) >= 4 && string(data[:4]) == "fLaC" {
			return true
		}
	case "ogg":
		// OGG: starts with "OggS"
		if len(data) >= 4 && string(data[:4]) == "OggS" {
			return true
		}
	}
	return false
}

// IsValidOptionalID validates an optional ID parameter (empty is allowed, non-empty must be hex).
func IsValidOptionalID(id string) bool {
	if id == "" {
		return true
	}
	if len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// DetectFileType maps file extensions to the internal file type names.
func DetectFileType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "pdf"
	case strings.HasSuffix(lower, ".docx"):
		return "word"
	case strings.HasSuffix(lower, ".doc"):
		return "word_legacy"
	case strings.HasSuffix(lower, ".xlsx"):
		return "excel"
	case strings.HasSuffix(lower, ".xls"):
		return "excel_legacy"
	case strings.HasSuffix(lower, ".pptx"):
		return "ppt"
	case strings.HasSuffix(lower, ".ppt"):
		return "ppt_legacy"
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".markdown"):
		return "markdown"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "html"
	case strings.HasSuffix(lower, ".mp4"):
		return "mp4"
	case strings.HasSuffix(lower, ".avi"):
		return "avi"
	case strings.HasSuffix(lower, ".mkv"):
		return "mkv"
	case strings.HasSuffix(lower, ".mov"):
		return "mov"
	case strings.HasSuffix(lower, ".webm"):
		return "webm"
	case strings.HasSuffix(lower, ".mp3"):
		return "mp3"
	case strings.HasSuffix(lower, ".m4a"):
		return "m4a"
	case strings.HasSuffix(lower, ".wav"):
		return "wav"
	case strings.HasSuffix(lower, ".flac"):
		return "flac"
	case strings.HasSuffix(lower, ".ogg"):
		return "ogg"
	default:
		return "unknown"
	}
}

// ValidatePassword checks password length and complexity requirements.
// Returns an error message if validation fails, or empty string if valid.
func ValidatePassword(password string) string {
	if len(password) < 8 {
		return "密码至少8位"
	}
	if len(password) > 72 {
		return "密码不能超过72位"
	}
	hasLower := false
	hasUpper := false
	hasDigit := false
	for _, c := range password {
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasLower || !hasUpper || !hasDigit {
		return "密码必须包含大写字母、小写字母和数字"
	}
	return ""
}
