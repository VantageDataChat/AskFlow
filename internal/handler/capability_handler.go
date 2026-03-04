package handler

import (
	"net/http"
)

// HandleSystemCapability returns the system's media processing capabilities.
func HandleSystemCapability(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// Require admin session to view system capabilities
		_, _, err := GetAdminSession(app, r)
		if err != nil {
			WriteAdminSessionError(w, err)
			return
		}

		cfg := app.configManager.Get()
		capability := DetectMediaCapability(cfg)

		// Add supported file types to response
		supportedTypes := GetSupportedFileTypes(capability)

		response := map[string]interface{}{
			"capability":      capability,
			"supported_types": supportedTypes,
			"message":         FormatCapabilityMessage(capability),
		}

		WriteJSON(w, http.StatusOK, response)
	}
}
