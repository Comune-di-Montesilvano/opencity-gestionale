package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/web/middleware"
)

type BandiHandler struct {
	DB *sql.DB
}

func (h *BandiHandler) ListBandi(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandi, _ := db.ListBandi(h.DB)
	var visibili []*db.Bando
	for _, b := range bandi {
		if op.IsAdmin() || op.CanAccessService(b.ServiceID) {
			visibili = append(visibili, b)
		}
	}
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "bandi.html", map[string]any{
		"Op":        op,
		"Bandi":     visibili,
		"Flash":     flash,
		"FlashType": flashType,
	})
}

func (h *BandiHandler) GetNuovoBando(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	renderTemplate(w, "bando_form.html", map[string]any{
		"Op":    op,
		"Bando": nil,
	})
}

func (h *BandiHandler) PostBando(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}
	b := &db.Bando{
		ServiceID:             strings.TrimSpace(r.FormValue("service_id")),
		Nome:                  strings.TrimSpace(r.FormValue("nome")),
		BudgetTotale:          parseFloat(r.FormValue("budget_totale")),
		ISEEMassimo:           parseFloat(r.FormValue("isee_massimo")),
		ScadenzaPresentazione: r.FormValue("scadenza_presentazione"),
		EngineType:            r.FormValue("engine_type"),
		EngineConfig:          "{}",
		Attivo:                true,
		CreatedAt:             time.Now(),
	}
	if b.EngineType == "" {
		b.EngineType = "mense_rette"
	}
	id, err := db.InsertBando(h.DB, b)
	if err != nil {
		http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bandi/"+strconv.FormatInt(id, 10)+"?flash=Bando+creato&flashType=success", http.StatusSeeOther)
}

func (h *BandiHandler) GetBando(w http.ResponseWriter, r *http.Request) {
	id := bandoIDFromPath(r)
	b, err := db.GetBando(h.DB, id)
	if err != nil {
		notFound(w, r)
		return
	}
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() && !op.CanAccessService(b.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	runs, _ := db.ListRuns(h.DB, b.ID)
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "bando_dettaglio.html", map[string]any{
		"Op":        op,
		"Bando":     b,
		"Runs":      runs,
		"Flash":     flash,
		"FlashType": flashType,
	})
}

func (h *BandiHandler) GetEditBando(w http.ResponseWriter, r *http.Request) {
	id := bandoIDFromPath(r)
	b, err := db.GetBando(h.DB, id)
	if err != nil {
		notFound(w, r)
		return
	}
	op := middleware.FromContext(r.Context())
	renderTemplate(w, "bando_form.html", map[string]any{
		"Op":    op,
		"Bando": b,
	})
}

func (h *BandiHandler) PutBando(w http.ResponseWriter, r *http.Request) {
	id := bandoIDFromPath(r)
	b, err := db.GetBando(h.DB, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}
	b.Nome = strings.TrimSpace(r.FormValue("nome"))
	b.BudgetTotale = parseFloat(r.FormValue("budget_totale"))
	b.ISEEMassimo = parseFloat(r.FormValue("isee_massimo"))
	b.ScadenzaPresentazione = r.FormValue("scadenza_presentazione")
	b.EngineType = r.FormValue("engine_type")
	if b.EngineType == "" {
		b.EngineType = "mense_rette"
	}
	if err := db.UpdateBando(h.DB, b); err != nil {
		http.Error(w, "Errore aggiornamento: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bandi/"+strconv.FormatInt(id, 10)+"?flash=Bando+aggiornato&flashType=success", http.StatusSeeOther)
}

func (h *BandiHandler) DeleteBando(w http.ResponseWriter, r *http.Request) {
	id := bandoIDFromPath(r)
	if err := db.DisattivaBando(h.DB, id); err != nil {
		http.Error(w, "Errore: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bandi?flash=Bando+disattivato&flashType=success", http.StatusSeeOther)
}

func bandoIDFromPath(r *http.Request) int64 {
	// Go 1.22+: r.PathValue("id")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}
