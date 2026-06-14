package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"opencity-gestionale/internal/web/middleware"
)

var (
	tmplOnce  sync.Once
	tmplCache *template.Template
)

var funcMap = template.FuncMap{
	"add":  func(a, b int) int { return a + b },
	"sub":  func(a, b int) int { return a - b },
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
}

func loadTemplates() *template.Template {
	tmplOnce.Do(func() {
		tmplCache = template.Must(
			template.New("").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.html")),
		)
	})
	return tmplCache
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	t := loadTemplates()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Errore template: "+err.Error(), http.StatusInternalServerError)
	}
}

// ReloadTemplates forza ricarico in sviluppo.
func ReloadTemplates() {
	tmplOnce = sync.Once{}
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
