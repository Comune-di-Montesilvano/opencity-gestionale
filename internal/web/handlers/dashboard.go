package handlers

import (
	"database/sql"
	"net/http"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/web/middleware"
)

type DashboardHandler struct {
	DB *sql.DB
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	motori, _ := db.ListMotori(h.DB, "attivo")

	type motoreConRun struct {
		Motore    *db.Bando
		UltimaRun *db.GraduatoriaRun
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
		items = append(items, motoreConRun{Motore: m, UltimaRun: ultima})
	}

	renderTemplate(w, "dashboard.html", map[string]any{
		"Op":    op,
		"Items": items,
	})
}
