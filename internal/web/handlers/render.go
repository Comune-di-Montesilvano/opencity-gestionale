package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opencity-gestionale/internal/web/middleware"
)

var (
	tmplMu    sync.Mutex
	tmplCache map[string]*template.Template
)

var funcMap = template.FuncMap{
	"add":  func(a, b int) int { return a + b },
	"sub":  func(a, b int) int { return a - b },
	"inc":  func(i int) int { return i + 1 },
	"dec":  func(i int) int { return i - 1 },
	"jsonKeys": func(keys []string) template.JS {
		b, _ := json.Marshal(keys)
		return template.JS(b)
	},
	"join": func(s []string, sep string) string { return strings.Join(s, sep) },
	"cfOscurato": func(cf string) string {
		if len(cf) < 6 {
			return cf
		}
		return cf[:3] + "***" + cf[len(cf)-3:]
	},
	"protocolloBreve": func(id string) string {
		if len(id) <= 8 {
			return id
		}
		return "…" + id[len(id)-8:]
	},
	"hasCol": func(cols []string, col string) bool {
		for _, c := range cols {
			if c == col {
				return true
			}
		}
		return false
	},
	"safeJSON": func(s string) template.JS {
		return template.JS(s)
	},
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	tmplMu.Lock()
	if tmplCache == nil {
		tmplCache = make(map[string]*template.Template)
	}
	t, ok := tmplCache[name]
	tmplMu.Unlock()

	if !ok {
		// Prova prima ./templates (dev locale), poi /templates (Docker distroless).
		dir := "templates"
		if m, _ := filepath.Glob(filepath.Join(dir, "*.html")); len(m) == 0 {
			dir = "/templates"
		}

		basePath := filepath.Join(dir, "base.html")
		filePath := filepath.Join(dir, name)

		var err error
		t = template.New(name).Funcs(funcMap)

		if name != "base.html" && fileExists(basePath) && fileExists(filePath) {
			_, err = t.ParseFiles(basePath, filePath)
		} else {
			_, err = t.ParseFiles(filePath)
		}

		if err != nil {
			http.Error(w, "Errore caricamento template: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tmplMu.Lock()
		tmplCache[name] = t
		tmplMu.Unlock()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Errore template: "+err.Error(), http.StatusInternalServerError)
	}
}

// ReloadTemplates forza ricarico in sviluppo.
func ReloadTemplates() {
	tmplMu.Lock()
	tmplCache = nil
	tmplMu.Unlock()
}

func flashFromRequest(r *http.Request) (string, string) {
	return r.URL.Query().Get("flash"), r.URL.Query().Get("flashType")
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	renderTemplate(w, "404.html", map[string]any{
		"Op": middleware.FromContext(r.Context()),
	})
}

func internalError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	renderTemplate(w, "500.html", map[string]any{
		"Op": middleware.FromContext(r.Context()),
	})
}
