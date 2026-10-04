package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/logging"
)

func withRequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := logging.ResolveRequestID(r.Header.Get(logging.HeaderRequestID))
		ctx := logging.WithRequestID(r.Context(), id)
		r = r.WithContext(ctx)
		w.Header().Set(logging.HeaderRequestID, id)

		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)

		latency := time.Since(start).Milliseconds()
		path := r.URL.Path
		kv := []any{
			"method", r.Method,
			"path", path,
			"status", rw.status,
			"latency_ms", latency,
		}
		if isQuietPath(path) {
			logging.Debugf(ctx, "http_access", kv...)
			return
		}
		logging.Infof(ctx, "http_access", kv...)
	})
}

func isQuietPath(path string) bool {
	switch path {
	case "/healthz", "/openapi.yaml", "/v1/completions", "/v1/completion", "/v1/route":
		return true
	case "/v1/chat/completions":
		return true
	default:
		// docs noise + chat covered by chat_done at info
		return strings.HasPrefix(path, "/docs")
	}
}
