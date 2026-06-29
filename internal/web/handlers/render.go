package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

var (
	tmplMu    sync.Mutex
	tmplCache *template.Template // ParseGlob set condiviso — partials unici (wizard-nav ecc.)
	tmplDir   string             // directory template scoperta al primo caricamento

	BrandingData *opencity.Branding
	Version      string = "dev"
)

func SetBranding(b *opencity.Branding) {
	tmplMu.Lock()
	defer tmplMu.Unlock()
	BrandingData = b
}

func SetVersion(v string) {
	tmplMu.Lock()
	defer tmplMu.Unlock()
	Version = v
}

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
	"statoVerificaKey": func(campo string) string {
		return "__stato_verifica_" + campo
	},
	"statusLabel": func(code string) string {
		labels := map[string]string{
			"1000":  "Bozza (1000)",
			"2000":  "Bozza (2000)",
			"3000":  "Inviata (3000)",
			"4000":  "In attesa istruttoria (4000)",
			"9000":  "Approvata (9000)",
			"20000": "Ritirata (20000)",
		}
		if l, ok := labels[code]; ok {
			return l
		}
		return code
	},
	"isCertFallito": func(motivi []string, campo string) bool {
		needle := `Campo "` + campo + `" non verificato`
		for _, m := range motivi {
			if m == needle {
				return true
			}
		}
		return false
	},
	"hasMotivoPrefix": func(motivi []string, prefix string) bool {
		for _, m := range motivi {
			if strings.HasPrefix(m, prefix) {
				return true
			}
		}
		return false
	},
	"motivoRisolto": func(motiviCorrenti []string, motivo string) bool {
		for _, m := range motiviCorrenti {
			if m == motivo {
				return false
			}
		}
		return true
	},
	"primoNonVuoto": func(vals ...string) string {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
		return ""
	},
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	tmplMu.Lock()
	shared := tmplCache
	dir := tmplDir
	tmplMu.Unlock()

	if shared == nil {
		// Prova prima ./templates (dev locale), poi /templates (Docker distroless).
		dir = "templates"
		if m, _ := filepath.Glob(filepath.Join(dir, "*.html")); len(m) == 0 {
			dir = "/templates"
		}

		var err error
		shared, err = template.New("").Funcs(funcMap).ParseGlob(filepath.Join(dir, "*.html"))
		if err != nil {
			http.Error(w, "Errore caricamento template: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tmplMu.Lock()
		tmplCache = shared
		tmplDir = dir
		tmplMu.Unlock()
	}

	// Clone il set condiviso (mantiene tutti i partial: wizard-nav ecc.) e
	// ri-parsa il file specifico per far "vincere" il suo {{define "content"}}
	// invece di quello dell'ultimo file caricato alfabeticamente da ParseGlob.
	t, err := shared.Clone()
	if err != nil {
		http.Error(w, "Errore clone template: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err = t.ParseFiles(filepath.Join(dir, name)); err != nil {
		http.Error(w, "Errore caricamento template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if data == nil {
		data = map[string]any{}
	}
	if m, ok := data.(map[string]any); ok {
		tmplMu.Lock()
		b := BrandingData
		v := Version
		tmplMu.Unlock()
		if b != nil {
			m["Branding"] = b
		}
		m["Version"] = v
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Errore template: "+err.Error(), http.StatusInternalServerError)
	}
}

// ReloadTemplates forza ricarico in sviluppo (DEV=true).
func ReloadTemplates() {
	tmplMu.Lock()
	tmplCache = nil
	tmplDir = ""
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
