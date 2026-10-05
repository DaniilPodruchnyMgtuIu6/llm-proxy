package api

import (
	"net/http"
	"strings"

	"github.com/llm-proxy/llm-proxy/internal/logging"
)

func withAPIKey(apiKey string, next http.Handler) http.Handler {
	if strings.TrimSpace(apiKey) == "" {
		return next
	}
	expected := strings.TrimSpace(apiKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearerOrAPIKey(r)
		if got == "" || got != expected {
			logging.Infof(r.Context(), "auth_rejected", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "unauthorized: provide Authorization Bearer or X-API-Key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerOrAPIKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-API-Key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

func isPublicPath(path string) bool {
	switch path {
	case "/healthz", "/openapi.yaml", "/architecture", "/architecture/", "/architecture.md":
		return true
	default:
		if strings.HasPrefix(path, "/docs") {
			return true
		}
		// Admin UI endpoints (LAN-trusted); /v1 still protected by PROXY_API_KEY when set.
		return strings.HasPrefix(path, "/admin/")
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-LLM-Proxy-Model, X-LLM-Proxy-Provider, X-LLM-Proxy-Attempts, X-LLM-Proxy-Preset")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
