package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/web/middleware"
)

type AuditHandler struct {
	DB *sql.DB
}

func (h *AuditHandler) GetAudit(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())

	f := db.AuditFilter{
		Operatore: r.URL.Query().Get("operatore"),
		Azione:    r.URL.Query().Get("azione"),
		Limit:     50,
	}
	if raw := r.URL.Query().Get("bando_id"); raw != "" {
		f.BandoID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		f.Offset, _ = strconv.Atoi(raw)
	}

	actions, total, _ := db.ListAudit(h.DB, f)
	bandi, _ := db.ListBandi(h.DB, "")

	renderTemplate(w, "audit.html", map[string]any{
		"Op":      op,
		"Actions": actions,
		"Total":   total,
		"Filter":  f,
		"Offset":  f.Offset,
		"Limit":   f.Limit,
		"Bandi":   bandi,
	})
}
