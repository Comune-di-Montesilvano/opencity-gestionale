# Admin Dashboard — Design Spec

**Data:** 2026-06-15  
**Progetto:** opencity-gestionale (Comune di Montesilvano)  
**Scope:** Ridisegnare la dashboard con layout sidebar + viste distinte per admin e operatore

---

## Obiettivo

L'admin entra nel gestionale e ha subito a disposizione il CRUD completo dei bandi (motori), senza dover navigare su pagine separate. L'operatore vede invece solo i bandi attivi a cui ha accesso, con le azioni operative (istruttoria, calcola run).

Il branding (logo, nome ente, favicon) è derivato automaticamente dall'istanza OpenCity — nessuna sezione impostazioni necessaria.

---

## Architettura

### Layout globale — `base.html`

Struttura flex orizzontale che sostituisce l'attuale navbar top:

```
┌──────────┬──────────────────────────────────────────┐
│ SIDEBAR  │  MAIN CONTENT                            │
│ (fissa)  │  (scrolla)                               │
└──────────┴──────────────────────────────────────────┘
```

**Sidebar** contiene:
- Logo + nome ente (top)
- Link navigazione: Dashboard, Audit
- Logout (bottom)

Impatta tutti i template esistenti (cambio struttura HTML/CSS in `base.html` e `static/style.css`). Nessuna logica Go modificata.

### Handler `/dashboard`

Un solo handler, due branch in base al ruolo:

```go
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
    op := middleware.FromContext(r.Context())
    if op.IsAdmin() {
        h.renderAdmin(w, r, op)
    } else {
        h.renderOperatore(w, r, op)
    }
}
```

- Admin → `dashboard_admin.html`
- Operatore → `dashboard_operatore.html`

Nessuna nuova route. Nessuna modifica middleware.

---

## Dashboard Admin (`dashboard_admin.html`)

### Layout

```
┌──────────┬────────────────────────────────────────────────┐
│  [logo]  │  Bandi                        [+ Nuovo bando] │
│  Comune  ├────────────────────────────────────────────────┤
│          │  ┌────────┐  ┌────────┐  ┌──────────────┐    │
│ ● Bandi  │  │Attivi  │  │ Bozze  │  │  Archiviati  │    │
│   Audit  │  │   3    │  │   1    │  │      5       │    │
│          │  └────────┘  └────────┘  └──────────────┘    │
│          │                                               │
│          │  [Attivi] [Bozze] [Archiviati]                │
│          │                                               │
│          │  Nome         Budget      Stato    Azioni     │
│          │  ──────────────────────────────────────────   │
│          │  Rette mense  €71.096    ● Attivo  Det Mod ⋯  │
│          │  Libri testo  —          ● Attivo  Det Mod ⋯  │
│          │                                               │
│ ──────── │                                               │
│ [Logout] │                                               │
└──────────┴────────────────────────────────────────────────┘
```

### Componenti

**Stats cards** (3 card orizzontali):
- Attivi: `COUNT WHERE stato_motore='attivo'`
- Bozze: `COUNT WHERE stato_motore='bozza'`
- Archiviati: `COUNT WHERE stato_motore='archiviato'`
- Implementazione: query `GROUP BY stato_motore` in `db/bandi.go` → nuova funzione `CountBandiPerStato`

**Tabs + tabella bandi:**
- Tab = query param `?stato=attivo` (default: `attivo`)
- Tabella colonne: Nome | Budget | ISEE max | Scadenza | Stato | Azioni
- Azioni inline: `Dettaglio` `Modifica` + dropdown `⋯` (Duplica / Archivia)
- Dropdown evita overflow riga con troppi bottoni — CSS puro, no JS framework

**Azioni tabella** (stessa logica già in `/motori`):
- `Dettaglio` → `GET /motori/{id}`
- `Modifica` → `GET /motori/{id}/wizard/2`
- `Duplica` → `POST /motori/{id}/duplica`
- `Archivia` → `POST /motori/{id}/archivia` (solo se stato ≠ archiviato)

---

## Dashboard Operatore (`dashboard_operatore.html`)

Contenuto = attuale `dashboard.html` (rinominato). Nessuna logica nuova.

```
┌──────────┬────────────────────────────────────────────────┐
│  [logo]  │  Dashboard                                     │
│  Comune  ├────────────────────────────────────────────────┤
│          │  ┌─────────────────┐ ┌─────────────────┐      │
│ ● Dash.  │  │ Rette e mense   │ │  Libri di testo │      │
│   Audit  │  │ ● Attivo        │ │  ● Attivo       │      │
│          │  │ Ultima run:...  │ │  Nessuna run    │      │
│          │  │ ⚠ 3 da verif.   │ │                 │      │
│          │  │ [Istruttoria]   │ │  [Calcola run]  │      │
│          │  └─────────────────┘ └─────────────────┘      │
│ ──────── │                                               │
│ [Logout] │                                               │
└──────────┴────────────────────────────────────────────────┘
```

Mostra solo bandi in `op.enabled_services_ids` (logica già presente in `dashboard.go:29`).

---

## Pagina `/motori`

Rimane invariata. Per gli operatori è la lista tabellare dei bandi attivi. Per l'admin è ridondante rispetto alla dashboard, ma non viene rimossa.

---

## Modifiche necessarie

| File | Tipo modifica |
|------|---------------|
| `templates/base.html` | Struttura HTML: navbar top → sidebar sinistra |
| `static/style.css` | Layout flex sidebar + main; rimuovi stili navbar top |
| `internal/web/handlers/dashboard.go` | Split handler: `renderAdmin` + `renderOperatore` |
| `internal/db/bandi.go` | Aggiunge `CountBandiPerStato() map[string]int` |
| `templates/dashboard.html` | Rinomina in `dashboard_operatore.html` |
| `templates/dashboard_admin.html` | Nuovo template: stats + tabs + tabella CRUD |

---

## Fuori scope

- Sezione impostazioni ente (branding da OpenCity)
- Filtri/ricerca nella tabella bandi
- Paginazione (bandi tipicamente < 50)
- Notifiche real-time
