package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
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

// RigaFuoriFondi raggruppa una riga "fuori fondi/posti" con il nome del gruppo di appartenenza.
type RigaFuoriFondi struct {
	Gruppo string
	Riga   graduatoria.RigaGraduatoria
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

	var campiVerifica []string
	for nome, fm := range ecfg.Mapping {
		if fm.VerificaPath != "" {
			campiVerifica = append(campiVerifica, nome)
		}
	}
	sort.Strings(campiVerifica)

	// Domande "fuori fondi/posti" dall'ultima run — placeholder per istruttoria futura.
	var fuoriFondi []RigaFuoriFondi
	if run, err := db.GetLatestRun(h.DB, bando.ID); err == nil && run != nil {
		var grad graduatoria.Graduatoria
		if json.Unmarshal([]byte(run.DatiJSON), &grad) == nil {
			for _, g := range grad.Gruppi {
				for _, r := range g.Righe {
					if !r.Ammessa && (r.NoteEsclusione == "fondi esauriti" || r.NoteEsclusione == "posti esauriti") {
						fuoriFondi = append(fuoriFondi, RigaFuoriFondi{Gruppo: g.Nome, Riga: r})
					}
				}
			}
		}
	}

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
		"CampiVerifica":   campiVerifica,
		"FuoriFondi":      fuoriFondi,
	})
}

// EseguiScansioneIstruttoria esegue la scansione delle istanze per flaggare le domande da verificare.
// Se statiSelezionati è vuoto, analizza tutti gli stati.
func EseguiScansioneIstruttoria(dbConn *sql.DB, baseURL string, bando *db.Bando, jwtToken string, operatore string, statiSelezionati []string) (int, error) {
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	if !ecfg.Verifica.Attiva {
		return 0, nil
	}

	statiSet := map[string]bool{}
	for _, s := range statiSelezionati {
		statiSet[s] = true
	}

	client := opencity.NewClient(baseURL, jwtToken)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		return 0, fmt.Errorf("fetch istanze: %w", err)
	}

	// Pulisce i record da_verificare
	if err := db.ResetDaVerificare(dbConn, int(bando.ID)); err != nil {
		return 0, fmt.Errorf("reset da_verificare: %w", err)
	}

	// Legge dati locali già salvati per override
	datiLocali, _ := db.GetIstruttorieDati(dbConn, int(bando.ID))

	nuove := 0
	for _, raw := range rawApps {
		var app opencity.Application
		if json.Unmarshal(raw, &app) != nil {
			continue
		}
		if len(statiSet) > 0 && !statiSet[app.Status] {
			continue
		}
		if !generic.PassaFiltriIstanza(app, ecfg.Istanza) {
			continue
		}
		records, err := generic.EstraiRecordsConExtras(app, ecfg, datiLocali[app.ID])
		if err != nil || len(records) == 0 {
			continue
		}
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
		if err := db.UpsertIstruttoria(dbConn, int(bando.ID), app.ID, motivi, app.Status); err == nil {
			nuove++
		}
		// Salva metadati utili all'operatore durante l'istruttoria (cross-bando in istruttorie_dati).
		if len(passingRecords) > 0 {
			rec := passingRecords[0]
			// CF richiedente e figlio — per evitare di dover aprire la domanda
			for _, k := range []string{"richiedente_cf", "richiedente"} {
				if v := rec.StringMap[k]; v != "" {
					db.SaveDatoIstruttoria(dbConn, int(bando.ID), app.ID, "__richiedente_cf", v) //nolint
					break
				}
			}
			for _, k := range []string{"figlio_cf", "figlio"} {
				if v := rec.StringMap[k]; v != "" {
					db.SaveDatoIstruttoria(dbConn, int(bando.ID), app.ID, "__figlio_cf", v) //nolint
					break
				}
			}
			// Valori dichiarati per i campi cert non verificati
			for nome, fm := range ecfg.Mapping {
				if fm.VerificaPath == "" {
					continue
				}
				if certVal := rec.StringMap["__cert_"+nome]; certVal != "" {
					continue // già verificato dalla fonte
				}
				declKey := "__decl_" + nome
				if sv := rec.StringMap[nome]; sv != "" {
					db.SaveDatoIstruttoria(dbConn, int(bando.ID), app.ID, declKey, sv) //nolint
				} else if fv, ok := rec.FloatMap[nome]; ok {
					db.SaveDatoIstruttoria(dbConn, int(bando.ID), app.ID, declKey, strconv.FormatFloat(fv, 'f', 2, 64)) //nolint
				}
			}
		}
	}

	db.InsertAudit(dbConn, &db.AuditAction{
		Operatore: operatore,
		Azione:    "istruttoria_scansione_automatica",
		BandoID:   bando.ID,
		Esito:     "ok",
		Messaggio: fmt.Sprintf("%d domande flaggate (automatica)", nuove),
	})

	return nuove, nil
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

	nuove, err := EseguiScansioneIstruttoria(h.DB, h.BaseURL, bando, op.JWT, op.Username, statiSelezionati)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=Errore+scansione:+%s&flashType=error", bandoID, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

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

	// Auto-rescan condizionale:
	// - esclusa → sempre (qualcuno esce dalla pool, la graduatoria scala)
	// - approvata → solo se ci sono dati locali modificati (override che influenzano il calcolo)
	deveRescan := stato == "esclusa" || (stato == "approvata" && db.HasDatiOverride(h.DB, ids))
	if deveRescan {
		statiApp, _ := db.ListStatiApp(h.DB, int(bandoID))
		EseguiScansioneIstruttoria(h.DB, h.BaseURL, bando, op.JWT, op.Username, statiApp) //nolint
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

	flashMsg := fmt.Sprintf("%d+domande+%s", len(ids), stato)
	if deveRescan {
		flashMsg += "+—+istruttoria+aggiornata"
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/istruttoria?flash=%s&flashType=success", bandoID, flashMsg), http.StatusSeeOther)
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

	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	// Salva il dato locale.
	if err := db.SaveDatoIstruttoria(h.DB, int(bandoID), praticaID, campo, valore); err != nil {
		http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ri-valuta i motivi recuperando l'app da OpenCity e applicando i dati locali aggiornati.
	var motivi []string

	client := opencity.NewClient(h.BaseURL, op.JWT)
	if app, err := client.FetchApplication(praticaID); err == nil {
		// Legge tutti i dati locali aggiornati per questa pratica.
		allDati, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
		extras := allDati[praticaID]

		// Per campi verifica: determina se il valore è sovrascitto o confermato.
		if fm, ok := ecfg.Mapping[campo]; ok && fm.VerificaPath != "" && valore != "" {
			recOrig, _ := generic.EstraiRecordsConExtras(*app, ecfg, nil)
			stato := "sovrascitto"
			if len(recOrig) > 0 {
				rec := recOrig[0]
				origStr := ""
				if v, ok := rec.FloatMap[campo]; ok {
					origStr = strconv.FormatFloat(v, 'f', -1, 64)
				} else if v, ok := rec.IntMap[campo]; ok {
					origStr = strconv.Itoa(v)
				} else {
					origStr = rec.StringMap[campo]
				}
				if strings.TrimSpace(valore) == strings.TrimSpace(origStr) {
					stato = "confermato"
				}
			}
			_ = db.SaveDatoIstruttoria(h.DB, int(bandoID), praticaID, "__stato_verifica_"+campo, stato)
			// Rilegge extras aggiornati con lo stato verifica.
			allDati, _ = db.GetIstruttorieDati(h.DB, int(bandoID))
			extras = allDati[praticaID]
		} else if valore == "" {
			// Valore cancellato: rimuove anche il metadato stato.
			_ = db.SaveDatoIstruttoria(h.DB, int(bandoID), praticaID, "__stato_verifica_"+campo, "")
			allDati, _ = db.GetIstruttorieDati(h.DB, int(bandoID))
			extras = allDati[praticaID]
		}

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

	// Carica dati aggiornati per il partial (include stato_verifica).
	allDatiPost, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
	datiPratica := allDatiPost[praticaID]

	var campiVerifica []string
	for nome, fm := range ecfg.Mapping {
		if fm.VerificaPath != "" {
			campiVerifica = append(campiVerifica, nome)
		}
	}
	sort.Strings(campiVerifica)

	renderTemplate(w, "istruttoria_dato_partial.html", map[string]any{
		"Motivi":        motivi,
		"CampiVerifica": campiVerifica,
		"Dati":          datiPratica,
		"BandoID":       bandoID,
		"PraticaID":     praticaID,
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
