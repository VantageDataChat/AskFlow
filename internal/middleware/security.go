package middleware

import (
	"net/http"
	"strings"
)

// allowedEmbedOrigin is the origin permitted to embed this site in an iframe.
const allowedEmbedOrigin = "https://market.vantagics.com"

// isEmbedRequest returns true when the request originates from an allowed
// cross-origin embedder (iframe on market.vantagics.com).
// Checks Referer and Origin headers — at least one must match.
func isEmbedRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == allowedEmbedOrigin {
		return true
	}
	ref := r.Header.Get("Referer")
	if strings.HasPrefix(ref, allowedEmbedOrigin+"/") || ref == allowedEmbedOrigin {
		return true
	}
	return false
}

// SecurityHeaders 返回设置安全响应头的中间件。
// 包含 OWASP 推荐的安全头，防止常见的 Web 攻击。
// 当请求来自允许的嵌入方 (market.vantagics.com) 时，放宽跨域策略以支持 iframe。
func SecurityHeaders() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-XSS-Protection", "0")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'self' https://market.vantagics.com")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

			if isEmbedRequest(r) {
				// Relax cross-origin policies so the iframe and its
				// sub-resources load correctly inside market.vantagics.com.
				w.Header().Set("Cross-Origin-Opener-Policy", "unsafe-none")
				w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
			} else {
				w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
				w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			}

			next(w, r)
		}
	}
}
