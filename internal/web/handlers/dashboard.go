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
	motori, _ := db.ListBandi(h.DB, stato)
	counts, _ := db.CountBandiPerStato(h.DB)
	renderTemplate(w, "dashboard_admin.html", map[string]any{
		"Op":     op,
		"Bandi": motori,
		"Stato":  stato,
		"Counts": counts,
	})
}

func (h *DashboardHandler) renderOperatore(w http.ResponseWriter, _ *http.Request, op *middleware.OperatorCtx) {
	motori, _ := db.ListBandi(h.DB, "attivo")

	type bandoConRun struct {
		Motore           *db.Bando
		UltimaRun        *db.GraduatoriaRun
		IstruttoriaStats db.IstruttoriaStats
		VerificaAttiva   bool
	}
	var items []bandoConRun
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
		items = append(items, bandoConRun{
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
