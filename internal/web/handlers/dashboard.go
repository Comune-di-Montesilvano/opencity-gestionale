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
	motori, _ := db.ListMotori(h.DB, "attivo")

	type motoreConRun struct {
		Motore           *db.Bando
		UltimaRun        *db.GraduatoriaRun
		IstruttoriaStats db.IstruttoriaStats
		VerificaAttiva   bool
	}
	var items []motoreConRun
	for _, m := range motori {
		if !op.IsAdmin() && !op.CanAccessService(m.ServiceID) {
			continue
		}
		runs, _ := db.ListRuns(h.DB, m.ID, !op.IsAdmin())
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
