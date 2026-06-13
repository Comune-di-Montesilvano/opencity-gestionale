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
	bandi, _ := db.ListBandi(h.DB)

	type bandoConRun struct {
		Bando    *db.Bando
		UltimaRun *db.GraduatoriaRun
	}
	var items []bandoConRun
	for _, b := range bandi {
		if !op.IsAdmin() && !op.CanAccessService(b.ServiceID) {
			continue
		}
		runs, _ := db.ListRuns(h.DB, b.ID)
		var ultima *db.GraduatoriaRun
		if len(runs) > 0 {
			ultima = runs[0]
		}
		items = append(items, bandoConRun{Bando: b, UltimaRun: ultima})
	}

	renderTemplate(w, "dashboard.html", map[string]any{
		"Op":    op,
		"Items": items,
	})
}
