# Admin Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ridisegnare la dashboard con sidebar sinistra e viste distinte per admin (CRUD bandi) e operatore (bandi attivi + runs).

**Architecture:** Un solo handler `/dashboard` che sceglie il template in base al ruolo (`op.IsAdmin()`). Layout globale cambia da navbar top a sidebar sinistra fissa modificando `base.html` e `style.css`. Due template separati evitano `{{if .Op.IsAdmin}}` annidati.

**Tech Stack:** Go 1.22+, `net/http` stdlib, `html/template`, SQLite, CSS puro (no framework JS)

---

## File map

| File | Operazione | Responsabilità |
|------|------------|----------------|
| `internal/db/bandi.go` | Modifica | Aggiunge `CountBandiPerStato` |
| `internal/db/db_test.go` | Modifica | Test per `CountBandiPerStato` |
| `static/style.css` | Modifica | Sidebar CSS, rimozione navbar CSS, aggiornamento print media query |
| `templates/base.html` | Modifica | `<nav class="navbar">` → `<aside class="sidebar">`, aggiunge `.main-content` wrapper |
| `templates/dashboard.html` | Rinomina → `dashboard_operatore.html` | Aggiorna `{{define}}` name |
| `templates/dashboard_admin.html` | Crea | Stats cards + tabs + tabella CRUD bandi |
| `internal/web/handlers/dashboard.go` | Modifica | Split in `renderAdmin` + `renderOperatore` |

---

## Task 1: DB — `CountBandiPerStato`

**Files:**
- Modify: `internal/db/bandi.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Scrivi il test che fallisce**

In `internal/db/db_test.go`, aggiungi dopo `TestListBandi`:

```go
func TestCountBandiPerStato(t *testing.T) {
	conn := openTestDB(t)

	inserisci := func(stato string) {
		b := &db.Bando{
			ServiceID: "svc-" + stato + "-" + time.Now().String(),
			Nome:      "Bando " + stato,
			EngineType:   "generic",
			EngineConfig: "{}",
			Attivo:       stato != "archiviato",
			StatoMotore:  stato,
			CreatedAt:    time.Now(),
		}
		db.InsertBando(conn, b)
	}

	inserisci("attivo")
	inserisci("attivo")
	inserisci("bozza")
	inserisci("archiviato")

	counts, err := db.CountBandiPerStato(conn)
	if err != nil {
		t.Fatalf("CountBandiPerStato: %v", err)
	}
	if counts["attivo"] != 2 {
		t.Errorf("attivo: got %d, want 2", counts["attivo"])
	}
	if counts["bozza"] != 1 {
		t.Errorf("bozza: got %d, want 1", counts["bozza"])
	}
	if counts["archiviato"] != 1 {
		t.Errorf("archiviato: got %d, want 1", counts["archiviato"])
	}
}
```

- [ ] **Step 2: Verifica che il test fallisce**

```bash
go test ./internal/db/... -run TestCountBandiPerStato -v
```

Output atteso: `FAIL` con `undefined: db.CountBandiPerStato`

- [ ] **Step 3: Implementa `CountBandiPerStato` in `internal/db/bandi.go`**

Aggiungi dopo la funzione `CountBandi`:

```go
func CountBandiPerStato(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT COALESCE(stato_motore,'bozza'), COUNT(*) FROM bandi GROUP BY stato_motore`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{"attivo": 0, "bozza": 0, "archiviato": 0}
	for rows.Next() {
		var stato string
		var n int
		if err := rows.Scan(&stato, &n); err != nil {
			return nil, err
		}
		counts[stato] = n
	}
	return counts, rows.Err()
}
```

- [ ] **Step 4: Verifica che il test passa**

```bash
go test ./internal/db/... -v
```

Output atteso: tutti `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/db/bandi.go internal/db/db_test.go
git commit -m "feat(db): add CountBandiPerStato for dashboard stats"
```

---

## Task 2: Layout — sidebar CSS + base.html

**Files:**
- Modify: `static/style.css`
- Modify: `templates/base.html`

- [ ] **Step 1: Aggiorna `static/style.css`**

Sostituisci il blocco `/* Navbar */` (righe 7-13) con il seguente blocco sidebar:

```css
/* Sidebar layout */
.app-layout { display: flex; min-height: 100vh; }
.sidebar { width: 220px; background: #1e3a5f; color: #fff; display: flex; flex-direction: column; flex-shrink: 0; position: fixed; top: 0; bottom: 0; left: 0; overflow-y: auto; z-index: 100; }
.sidebar-brand { padding: 1.25rem 1rem; font-weight: 600; font-size: 15px; color: #fff; border-bottom: 1px solid rgba(255,255,255,.1); line-height: 1.3; }
.sidebar-nav { flex: 1; padding: .75rem 0; }
.sidebar-nav a { display: block; padding: .55rem 1.25rem; color: #cbd5e1; font-size: 13px; text-decoration: none; }
.sidebar-nav a:hover { color: #fff; background: rgba(255,255,255,.08); text-decoration: none; }
.sidebar-nav a.sidebar-active { color: #fff; background: rgba(255,255,255,.12); font-weight: 600; }
.sidebar-footer { padding: 1rem; border-top: 1px solid rgba(255,255,255,.1); }
.sidebar-user { font-size: 12px; color: #94a3b8; display: block; margin-bottom: .5rem; }
.main-content { margin-left: 220px; padding: 1.5rem; min-height: 100vh; }
```

Rimuovi anche il blocco `/* Container */` e il `.container` che lo segue (riga 16), sostituendolo con:

```css
/* Container — usato solo per pagine senza sidebar (login) */
.container { max-width: 1200px; margin: 0 auto; padding: 1.5rem 1rem; }
```

Aggiorna il blocco `@media print` — sostituisci le righe con `.navbar`:

```css
@media print {
  .no-print { display: none !important; }
  .sidebar { display: none !important; }
  .main-content { margin-left: 0 !important; padding: 0 !important; }
  body { background: white !important; font-size: 11px; margin: 0; }
  .stampa-table { font-size: 10px; }
  .stampa-table th, .stampa-table td { border: 1px solid #999; padding: 3px 6px; }
  .stampa-esclusa td { color: #999; }
  .stampa-sezione { page-break-inside: avoid; }
  h3, h4 { page-break-after: avoid; }
}
```

- [ ] **Step 2: Aggiorna `templates/base.html`**

Sostituisci l'intero contenuto del file con:

```html
{{define "base"}}<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{block "title" .}}Gestionale OpenCity{{end}}</title>
<link rel="stylesheet" href="/static/style.css">
<script src="/static/htmx.min.js"></script>
</head>
<body>
{{if .Op}}
<div class="app-layout">
  <aside class="sidebar no-print">
    <div class="sidebar-brand">Gestionale<br>OpenCity</div>
    <nav class="sidebar-nav">
      <a href="/dashboard">Dashboard</a>
      <a href="/motori">Motori</a>
      <a href="/audit">Audit</a>
    </nav>
    <div class="sidebar-footer">
      <span class="sidebar-user">{{.Op.Username}} ({{.Op.Ruolo}})</span>
      <a href="/logout" class="btn btn-sm btn-secondary btn-block">Esci</a>
    </div>
  </aside>
  <main class="main-content">
    {{if .Flash}}<div class="alert alert-{{.FlashType}}">{{.Flash}}</div>{{end}}
    {{block "content" .}}{{end}}
  </main>
</div>
{{else}}
<main class="container">
  {{if .Flash}}<div class="alert alert-{{.FlashType}}">{{.Flash}}</div>{{end}}
  {{block "content" .}}{{end}}
</main>
{{end}}
</body>
</html>{{end}}
```

- [ ] **Step 3: Verifica compilazione**

```bash
go build ./...
```

Output atteso: nessun errore

- [ ] **Step 4: Commit**

```bash
git add static/style.css templates/base.html
git commit -m "feat(ui): replace top navbar with fixed left sidebar"
```

---

## Task 3: Rinomina template operatore + aggiorna handler

**Files:**
- Rename: `templates/dashboard.html` → `templates/dashboard_operatore.html`
- Modify: `internal/web/handlers/dashboard.go` (stringa template name)

- [ ] **Step 1: Rinomina il file e aggiorna il `{{define}}`**

Crea `templates/dashboard_operatore.html` con il contenuto di `templates/dashboard.html` ma cambia la prima riga:

```html
{{define "dashboard_operatore.html"}}{{template "base" .}}{{end}}
```

Il resto del contenuto rimane identico all'attuale `dashboard.html`.

- [ ] **Step 2: Elimina il vecchio file**

```bash
rm templates/dashboard.html
```

- [ ] **Step 3: Aggiorna la chiamata in `dashboard.go`**

In `internal/web/handlers/dashboard.go`, alla riga con `renderTemplate`, cambia:

```go
renderTemplate(w, "dashboard.html", map[string]any{
```

in:

```go
renderTemplate(w, "dashboard_operatore.html", map[string]any{
```

- [ ] **Step 4: Verifica compilazione e avvio**

```bash
go build ./...
```

Output atteso: nessun errore. La dashboard operatore funziona con la nuova sidebar.

- [ ] **Step 5: Commit**

```bash
git add templates/dashboard_operatore.html internal/web/handlers/dashboard.go
git rm templates/dashboard.html
git commit -m "refactor(dashboard): rename template to dashboard_operatore.html"
```

---

## Task 4: Crea `dashboard_admin.html`

**Files:**
- Create: `templates/dashboard_admin.html`

- [ ] **Step 1: Crea il template**

```html
{{define "dashboard_admin.html"}}{{template "base" .}}{{end}}

{{define "title"}}Bandi{{end}}

{{define "content"}}
<div class="page-header">
  <h1>Bandi</h1>
  <a href="/motori/nuovo" class="btn btn-primary">+ Nuovo bando</a>
</div>

<div class="stats-row">
  <div class="stat">
    <span class="stat-value">{{index .Counts "attivo"}}</span>
    <span class="stat-label">Attivi</span>
  </div>
  <div class="stat">
    <span class="stat-value">{{index .Counts "bozza"}}</span>
    <span class="stat-label">Bozze</span>
  </div>
  <div class="stat">
    <span class="stat-value">{{index .Counts "archiviato"}}</span>
    <span class="stat-label">Archiviati</span>
  </div>
</div>

<div class="tab-bar">
  <a href="/dashboard?stato=attivo" class="tab {{if eq .Stato "attivo"}}tab-active{{end}}">Attivi</a>
  <a href="/dashboard?stato=bozza" class="tab {{if eq .Stato "bozza"}}tab-active{{end}}">Bozze</a>
  <a href="/dashboard?stato=archiviato" class="tab {{if eq .Stato "archiviato"}}tab-active{{end}}">Archiviati</a>
</div>

{{if not .Motori}}
<p class="text-muted" style="margin-top:1rem">Nessun bando in questo stato.</p>
{{else}}
<table class="table" style="margin-top:1rem">
  <thead>
    <tr>
      <th>Nome</th>
      <th>Budget</th>
      <th>ISEE max</th>
      <th>Scadenza</th>
      <th>Stato</th>
      <th></th>
    </tr>
  </thead>
  <tbody>
  {{range .Motori}}
  <tr>
    <td><a href="/motori/{{.ID}}">{{.Nome}}</a></td>
    <td>{{if .BudgetTotale}}€{{printf "%.2f" .BudgetTotale}}{{else}}—{{end}}</td>
    <td>{{if .ISEEMassimo}}€{{printf "%.2f" .ISEEMassimo}}{{else}}—{{end}}</td>
    <td>{{if .ScadenzaPresentazione}}{{.ScadenzaPresentazione}}{{else}}—{{end}}</td>
    <td>
      {{if eq .StatoMotore "attivo"}}<span class="badge badge-green">Attivo</span>
      {{else if eq .StatoMotore "archiviato"}}<span class="badge badge-gray">Archiviato</span>
      {{else}}<span class="badge badge-yellow">Bozza</span>{{end}}
    </td>
    <td class="td-actions">
      <a href="/motori/{{.ID}}" class="btn btn-sm btn-secondary">Dettaglio</a>
      <a href="/motori/{{.ID}}/wizard/2" class="btn btn-sm btn-secondary">Modifica</a>
      <form method="POST" action="/motori/{{.ID}}/duplica" style="display:inline">
        <button type="submit" class="btn btn-sm">Duplica</button>
      </form>
      {{if ne .StatoMotore "archiviato"}}
      <form method="POST" action="/motori/{{.ID}}/archivia" style="display:inline">
        <button type="submit" class="btn btn-sm btn-danger" onclick="return confirm('Archiviare questo bando?')">Archivia</button>
      </form>
      {{end}}
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
{{end}}
{{end}}
```

- [ ] **Step 2: Verifica compilazione**

```bash
go build ./...
```

Output atteso: nessun errore

- [ ] **Step 3: Commit**

```bash
git add templates/dashboard_admin.html
git commit -m "feat(ui): add admin dashboard template with bandi CRUD"
```

---

## Task 5: Split handler — branch admin vs operatore

**Files:**
- Modify: `internal/web/handlers/dashboard.go`

- [ ] **Step 1: Riscrivi `dashboard.go`**

Sostituisci l'intero contenuto del file con:

```go
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/web/middleware"
)

type DashboardHandler struct {
	DB *sql.DB
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if op.IsAdmin() {
		h.renderAdmin(w, r, op)
	} else {
		h.renderOperatore(w, r, op)
	}
}

func (h *DashboardHandler) renderAdmin(w http.ResponseWriter, r *http.Request, op *middleware.OperatorCtx) {
	stato := r.URL.Query().Get("stato")
	if stato == "" {
		stato = "attivo"
	}
	motori, _ := db.ListMotori(h.DB, stato)
	counts, _ := db.CountBandiPerStato(h.DB)
	renderTemplate(w, "dashboard_admin.html", map[string]any{
		"Op":     op,
		"Motori": motori,
		"Stato":  stato,
		"Counts": counts,
	})
}

func (h *DashboardHandler) renderOperatore(w http.ResponseWriter, r *http.Request, op *middleware.OperatorCtx) {
	motori, _ := db.ListMotori(h.DB, "attivo")

	type motoreConRun struct {
		Motore           *db.Bando
		UltimaRun        *db.GraduatoriaRun
		IstruttoriaStats db.IstruttoriaStats
		VerificaAttiva   bool
	}
	var items []motoreConRun
	for _, m := range motori {
		if !op.CanAccessService(m.ServiceID) {
			continue
		}
		runs, _ := db.ListRuns(h.DB, m.ID, true)
		var ultima *db.GraduatoriaRun
		if len(runs) > 0 {
			ultima = runs[0]
		}
		var ecfg graduatoria.EngineConfig
		json.Unmarshal([]byte(m.EngineConfig), &ecfg)
		var istStats db.IstruttoriaStats
		if ecfg.Verifica.Attiva {
			istStats, _ = db.GetIstruttoriaStats(h.DB, int(m.ID))
		}
		items = append(items, motoreConRun{
			Motore:           m,
			UltimaRun:        ultima,
			IstruttoriaStats: istStats,
			VerificaAttiva:   ecfg.Verifica.Attiva,
		})
	}

	renderTemplate(w, "dashboard_operatore.html", map[string]any{
		"Op":    op,
		"Items": items,
	})
}
```

**Nota:** `renderOperatore` usa `soloPublicate: true` (terzo param di `ListRuns`) perché l'operatore vede solo run pubblicate. L'admin accede alle run da `/motori/{id}`, non dalla dashboard.

- [ ] **Step 2: Verifica compilazione**

```bash
go build ./...
```

Output atteso: nessun errore

- [ ] **Step 3: Esegui tutti i test**

```bash
go test ./...
```

Output atteso: tutti `PASS`

- [ ] **Step 4: Commit**

```bash
git add internal/web/handlers/dashboard.go
git commit -m "feat(dashboard): split handler — admin CRUD view vs operator operational view"
```

---

## Self-review

**Copertura spec:**
- ✅ Layout sidebar sinistra (`base.html` + CSS — Task 2)
- ✅ Dashboard admin con stats cards + tabs + tabella CRUD (Task 4)
- ✅ Dashboard operatore = vista attuale rinominata (Task 3)
- ✅ Handler singolo `/dashboard` con branch ruolo (Task 5)
- ✅ `CountBandiPerStato` DB function con test (Task 1)
- ✅ Print media query aggiornata (Task 2, `.sidebar` invece di `.navbar`)

**Placeholder scan:** Nessun TBD o TODO nei task.

**Type consistency:**
- `db.CountBandiPerStato` → definita Task 1, usata Task 5 ✅
- `"dashboard_operatore.html"` → definita Task 3 (file + define), chiamata Task 5 ✅
- `"dashboard_admin.html"` → definita Task 4 (file + define), chiamata Task 5 ✅
- `map[string]any{"Counts": counts, "Stato": stato, "Motori": motori}` → corrisponde a `{{index .Counts "attivo"}}`, `{{eq .Stato "attivo"}}`, `{{range .Motori}}` nel template ✅
- `db.ListRuns(h.DB, m.ID, true)` → firma con 3 parametri confermata da `dashboard.go` originale riga 32 ✅
