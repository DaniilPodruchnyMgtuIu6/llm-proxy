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
		return strings.HasPrefix(path, "/docs")
	}
}
