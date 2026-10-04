package api

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	path := s.openAPIPath
	if path == "" {
		path = "api/openapi.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// try relative to executable working dir variants
		alt := filepath.Join(".", "api", "openapi.yaml")
		data, err = os.ReadFile(alt)
		if err != nil {
			writeError(w, http.StatusNotFound, "openapi.yaml not found")
			return
		}
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>LLM Proxy API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css" />
  <style>
    body { margin: 0; background: #fafafa; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis],
      layout: 'BaseLayout',
      tryItOutEnabled: true,
      persistAuthorization: true
    });
  </script>
</body>
</html>
`
