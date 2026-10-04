package api

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) handleArchitecture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(architectureHTML))
}

func (s *Server) handleArchitectureMarkdown(w http.ResponseWriter, r *http.Request) {
	data, err := readDocsFile("docs/architecture.md")
	if err != nil {
		writeError(w, http.StatusNotFound, "architecture.md not found")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func readDocsFile(rel string) ([]byte, error) {
	candidates := []string{
		rel,
		filepath.Join(".", rel),
	}
	var last error
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, nil
		}
		last = err
	}
	return nil, last
}

const architectureHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>LLM Proxy — Architecture</title>
  <script src="https://cdn.jsdelivr.net/npm/marked@15.0.7/marked.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/mermaid@11.6.0/dist/mermaid.min.js"></script>
  <style>
    :root {
      --bg: #f6f7f9;
      --card: #ffffff;
      --text: #1f2937;
      --muted: #6b7280;
      --border: #e5e7eb;
      --accent: #0f766e;
    }
    body {
      margin: 0;
      font-family: "Segoe UI", system-ui, sans-serif;
      background: var(--bg);
      color: var(--text);
      line-height: 1.55;
    }
    header {
      position: sticky;
      top: 0;
      z-index: 10;
      background: rgba(246,247,249,.92);
      backdrop-filter: blur(8px);
      border-bottom: 1px solid var(--border);
      padding: 12px 20px;
      display: flex;
      gap: 16px;
      align-items: center;
      flex-wrap: wrap;
    }
    header strong { color: var(--accent); }
    header a { color: var(--accent); text-decoration: none; }
    header a:hover { text-decoration: underline; }
    main {
      max-width: 980px;
      margin: 24px auto 64px;
      padding: 28px 32px;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      box-shadow: 0 8px 24px rgba(15, 23, 42, 0.04);
    }
    main h1, main h2, main h3 { line-height: 1.25; }
    main h1 { margin-top: 0; }
    main h2 {
      margin-top: 2rem;
      padding-top: 0.75rem;
      border-top: 1px solid var(--border);
    }
    main table {
      border-collapse: collapse;
      width: 100%;
      margin: 1rem 0;
      font-size: 0.95rem;
    }
    main th, main td {
      border: 1px solid var(--border);
      padding: 8px 10px;
      text-align: left;
      vertical-align: top;
    }
    main th { background: #f3f4f6; }
    main code {
      background: #f3f4f6;
      padding: 0.1em 0.35em;
      border-radius: 4px;
      font-size: 0.92em;
    }
    main pre {
      background: #111827;
      color: #f9fafb;
      padding: 14px 16px;
      border-radius: 8px;
      overflow-x: auto;
    }
    main pre code { background: transparent; color: inherit; padding: 0; }
    main blockquote {
      margin: 1rem 0;
      padding: 0.75rem 1rem;
      border-left: 4px solid var(--accent);
      background: #ecfdf5;
      color: #065f46;
    }
    .mermaid {
      background: #fafafa;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 16px;
      margin: 1.25rem 0;
      overflow-x: auto;
      text-align: center;
    }
    .err {
      color: #b91c1c;
      padding: 24px;
    }
  </style>
</head>
<body>
  <header>
    <strong>LLM Proxy Architecture</strong>
    <a href="/docs">Swagger</a>
    <a href="/architecture.md">Raw markdown</a>
    <a href="/v1/models">Models API</a>
    <span style="color:var(--muted);font-size:0.9rem">Mermaid рендерится здесь (не в Cursor preview)</span>
  </header>
  <main id="content"><p>Loading…</p></main>
  <script>
    mermaid.initialize({
      startOnLoad: false,
      theme: "neutral",
      securityLevel: "loose",
      flowchart: { htmlLabels: true, curve: "basis" }
    });

    async function render() {
      const el = document.getElementById("content");
      try {
        const res = await fetch("/architecture.md");
        if (!res.ok) throw new Error("failed to load architecture.md: " + res.status);
        const md = await res.text();
        el.innerHTML = marked.parse(md, { mangle: false, headerIds: true });

        const blocks = el.querySelectorAll("pre code.language-mermaid");
        for (const code of blocks) {
          const pre = code.parentElement;
          const div = document.createElement("div");
          div.className = "mermaid";
          div.textContent = code.textContent;
          pre.replaceWith(div);
        }
        await mermaid.run({ querySelector: ".mermaid" });
      } catch (e) {
        el.innerHTML = '<p class="err">' + (e && e.message ? e.message : e) + "</p>";
      }
    }
    render();
  </script>
</body>
</html>
`
