package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"sync"
)

var (
	tmplOnce  sync.Once
	tmplCache *template.Template
)

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
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
