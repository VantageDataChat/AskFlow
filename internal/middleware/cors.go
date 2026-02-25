package middleware

import "net/http"

// CORS 返回处理跨域请求的中间件。
// 允许同源请求以及来自 market.vantagics.com 的跨域请求（iframe 嵌入场景）。
// 对 OPTIONS 预检请求返回 204 No Content。
func CORS() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Always set Vary: Origin to prevent cache poisoning
			w.Header().Set("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := false

				// Same-origin check
				requestHost := r.Host
				if requestHost != "" && (origin == "http://"+requestHost || origin == "https://"+requestHost) {
					allowed = true
				}

				// Allow cross-origin from market.vantagics.com (iframe embed)
				if origin == allowedEmbedOrigin {
					allowed = true
				}

				if allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Max-Age", "3600")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}
}
