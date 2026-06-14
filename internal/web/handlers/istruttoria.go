package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/generic"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type IstruttoriaHandler struct {
	DB      *sql.DB
	BaseURL string
}

// GetIstruttoria — dashboard istruttoria per un bando.
func (h *IstruttoriaHandler) GetIstruttoria(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	statoFilter := r.URL.Query().Get("stato")
	istruttorie, err := db.ListIstruttorie(h.DB, int(bandoID), statoFilter)
	if err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}
	stats, _ := db.GetIstruttoriaStats(h.DB, int(bandoID))

	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "istruttoria.html", map[string]any{
		"Op":          op,
		"Bando":       bando,
		"Config":      ecfg,
		"Istruttorie": istruttorie,
		"Stats":       stats,
		"StatoFilter": statoFilter,
		"Flash":       flash,
		"FlashType":   flashType,
	})
}

// PostScansiona — fetch tutte le app, applica FiltriFlag+PDND, popola istruttorie.
func (h *IstruttoriaHandler) PostScansiona(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	if !ecfg.Verifica.Attiva {
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/istruttoria?flash=Verifica+non+attiva+per+questo+bando&flashType=error", bandoID), http.StatusSeeOther)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/istruttoria?flash=Errore+fetch+istanze:+%s&flashType=error", bandoID, err.Error()), http.StatusSeeOther)
		return
	}

	nuove := 0
	for _, raw := range rawApps {
		var app opencity.Application
		if json.Unmarshal(raw, &app) != nil {
			continue
		}
		records, err := generic.EstraiRecords(app, ecfg)
		if err != nil || len(records) == 0 {
			continue
		}
		// Raccogli motivi unici su tutti i record dell'app (es. righe espansione)
		motiviSet := map[string]struct{}{}
		for _, rec := range records {
			for _, m := range rec.FlagMotivi(ecfg.Verifica) {
				motiviSet[m] = struct{}{}
			}
		}
		if len(motiviSet) == 0 {
			continue
		}
		motivi := make([]string, 0, len(motiviSet))
		for m := range motiviSet {
			motivi = append(motivi, m)
		}
		if err := db.UpsertIstruttoria(h.DB, int(bandoID), app.ID, motivi); err == nil {
			nuove++
		}
	}

	db.InsertAudit(h.DB, &db.AuditAction{
		Operatore: op.Username,
		Azione:    "istruttoria_scansione",
		BandoID:   bandoID,
		Esito:     "ok",
		Messaggio: fmt.Sprintf("%d domande flaggate", nuove),
	})

	http.Redirect(w, r, fmt.Sprintf("/motori/%d/istruttoria?flash=Scansione+completata:+%d+domande+flaggate&flashType=success", bandoID, nuove), http.StatusSeeOther)
}

// PostIstruttoriaBatch — approva o escludi le istruttorie selezionate.
func (h *IstruttoriaHandler) PostIstruttoriaBatch(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}

	stato := r.FormValue("stato")
	if stato != "approvata" && stato != "esclusa" {
		http.Error(w, "Stato non valido", http.StatusBadRequest)
		return
	}
	nota := r.FormValue("nota")

	var ids []int
	for _, s := range r.Form["ids"] {
		id, err := strconv.Atoi(s)
		if err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/istruttoria?flash=Nessuna+domanda+selezionata&flashType=error", bandoID), http.StatusSeeOther)
		return
	}

	if err := db.BatchSetStato(h.DB, ids, stato, nota, op.Username); err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, id := range ids {
		db.InsertAudit(h.DB, &db.AuditAction{
			Operatore: op.Username,
			Azione:    "istruttoria_" + stato,
			BandoID:   bandoID,
			Messaggio: nota,
			Esito:     "ok",
		})
		_ = id
	}

	http.Redirect(w, r, fmt.Sprintf("/motori/%d/istruttoria?flash=%d+domande+%s&flashType=success", bandoID, len(ids), stato), http.StatusSeeOther)
}
