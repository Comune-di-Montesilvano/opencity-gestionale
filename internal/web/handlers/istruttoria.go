package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	appStatusFilter := r.URL.Query().Get("app_status")
	istruttorie, err := db.ListIstruttorie(h.DB, int(bandoID), statoFilter, appStatusFilter)
	if err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}
	stats, _ := db.GetIstruttoriaStats(h.DB, int(bandoID))
	statiApp, _ := db.ListStatiApp(h.DB, int(bandoID))

	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "istruttoria.html", map[string]any{
		"Op":              op,
		"Bando":           bando,
		"Config":          ecfg,
		"Istruttorie":     istruttorie,
		"Stats":           stats,
		"StatoFilter":     statoFilter,
		"AppStatusFilter": appStatusFilter,
		"StatiApp":        statiApp,
		"BaseURL":         h.BaseURL,
		"Flash":           flash,
		"FlashType":       flashType,
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
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Verifica+non+attiva+per+questo+bando&flashType=error", bandoID), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}
	statiSelezionati := r.Form["stati"]
	statiSet := map[string]bool{}
	for _, s := range statiSelezionati {
		statiSet[s] = true
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Errore+fetch+istanze:+%s&flashType=error", bandoID, err.Error()), http.StatusSeeOther)
		return
	}

	// Pulisce i record da_verificare: la scansione reimposta lo stato da zero.
	// Record approvata/esclusa vengono preservati.
	if err := db.ResetDaVerificare(h.DB, int(bandoID)); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Errore+reset:+%s&flashType=error", bandoID, err.Error()), http.StatusSeeOther)
		return
	}

	// Legge dati locali già salvati per applicare override durante la scansione.
	datiLocali, _ := db.GetIstruttorieDati(h.DB, int(bandoID))

	nuove := 0
	for _, raw := range rawApps {
		var app opencity.Application
		if json.Unmarshal(raw, &app) != nil {
			continue
		}
		// Filtra per stato se selezionato almeno uno.
		if len(statiSet) > 0 && !statiSet[app.Status] {
			continue
		}
		// Applica FiltriIstanza (stati ammessi + date) — stessa logica di Calcola().
		if !generic.PassaFiltriIstanza(app, ecfg.Istanza) {
			continue
		}
		records, err := generic.EstraiRecordsConExtras(app, ecfg, datiLocali[app.ID])
		if err != nil || len(records) == 0 {
			continue
		}
		// Salta app che verrebbero comunque escluse dai filtri ammissibilità.
		// Accumula anche i record passanti per raccogliere solo i loro motivi.
		var passingRecords []*graduatoria.Record
		for _, rec := range records {
			rec.DerivaCampi(ecfg.Rimborso)
			if ok, _ := generic.ApplicaFiltri(rec, ecfg.Filtri); ok {
				passingRecords = append(passingRecords, rec)
			}
		}
		if len(passingRecords) == 0 {
			continue
		}
		// Salta app dove nessun record passante matcha una tipologia —
		// in calcolo andrebbero tutte in escluse, non servono verifica.
		if len(ecfg.Tipologie) > 0 {
			hasTipologia := false
			for _, rec := range passingRecords {
				if generic.TipologiaDiRecord(rec, ecfg.Tipologie) != "" {
					hasTipologia = true
					break
				}
			}
			if !hasTipologia {
				continue
			}
		}
		motiviSet := map[string]struct{}{}
		for _, rec := range passingRecords {
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
		if err := db.UpsertIstruttoria(h.DB, int(bandoID), app.ID, motivi, app.Status); err == nil {
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

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Scansione+completata:+%d+domande+flaggate&flashType=success", bandoID, nuove), http.StatusSeeOther)
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
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Nessuna+domanda+selezionata&flashType=error", bandoID), http.StatusSeeOther)
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

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=%d+domande+%s&flashType=success", bandoID, len(ids), stato), http.StatusSeeOther)
}

// PostSaveDato — salva un valore locale per un campo mancante, ri-valuta i motivi via API.
// HTMX: risponde con partial HTML aggiornato (motivi badges).
func (h *IstruttoriaHandler) PostSaveDato(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	praticaID := r.PathValue("praticaID")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.Error(w, "Bando non trovato", http.StatusNotFound)
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
	campo := strings.TrimSpace(r.FormValue("campo"))
	valore := strings.TrimSpace(r.FormValue("valore"))
	if campo == "" {
		http.Error(w, "Campo obbligatorio", http.StatusBadRequest)
		return
	}

	// Salva il dato locale.
	if err := db.SaveDatoIstruttoria(h.DB, int(bandoID), praticaID, campo, valore); err != nil {
		http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ri-valuta i motivi recuperando l'app da OpenCity e applicando i dati locali aggiornati.
	var motivi []string
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	client := opencity.NewClient(h.BaseURL, op.JWT)
	if app, err := client.FetchApplication(praticaID); err == nil {
		// Legge tutti i dati locali aggiornati per questa pratica.
		allDati, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
		extras := allDati[praticaID]
		records, _ := generic.EstraiRecordsConExtras(*app, ecfg, extras)
		motiviSet := map[string]struct{}{}
		for _, rec := range records {
			for _, m := range rec.FlagMotivi(ecfg.Verifica) {
				motiviSet[m] = struct{}{}
			}
		}
		for m := range motiviSet {
			motivi = append(motivi, m)
		}
		// Aggiorna motivi_json nel DB.
		db.UpdateMotiviIstruttoria(h.DB, int(bandoID), praticaID, motivi)
	} else {
		// Fallback: legge motivi dal DB senza ri-valutare.
		if ist, err := db.GetIstruttoriaByPratica(h.DB, int(bandoID), praticaID); err == nil {
			motivi = ist.Motivi
		}
	}

	renderTemplate(w, "istruttoria_dato_partial.html", map[string]any{
		"Motivi": motivi,
	})
}

// PostRiapri — riporta una pratica approvata/esclusa a da_verificare.
func (h *IstruttoriaHandler) PostRiapri(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	praticaID := r.PathValue("praticaID")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	ist, err := db.GetIstruttoriaByPratica(h.DB, int(bandoID), praticaID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := db.SetStato(h.DB, ist.ID, "da_verificare", ist.Nota, op.Username); err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria", bandoID), http.StatusSeeOther)
}

// PostSaveNota — salva nota inline su una pratica. HTMX: risponde 200 vuoto.
func (h *IstruttoriaHandler) PostSaveNota(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	praticaID := r.PathValue("praticaID")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.Error(w, "Bando non trovato", http.StatusNotFound)
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
	nota := strings.TrimSpace(r.FormValue("nota"))
	if err := db.SaveNota(h.DB, int(bandoID), praticaID, nota); err != nil {
		http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
