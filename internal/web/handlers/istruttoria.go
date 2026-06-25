package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
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

	// Campi built-in editabili: non hanno VerificaPath ma l'operatore può inserire il valore corretto.
	type CampoBuiltin struct {
		Campo  string
		Label  string
		Motivo string // prefisso del motivo che attiva l'input
	}
	var campiBuiltin []CampoBuiltin
	if ecfg.Modalita == "fondi" && ecfg.Rimborso.CampoLordo != "" {
		campiBuiltin = append(campiBuiltin, CampoBuiltin{
			Campo:  ecfg.Rimborso.CampoLordo,
			Label:  "Importo speso (€)",
			Motivo: "Corrispettivo dichiarato €0,00",
		})
	}

	// Campi dichiarati e CF da mostrare in istruttoria senza aprire la domanda.
	// Sorgente primaria: cache API aggiornata durante la scan. Fallback: snapshot ultima run.
	campiDichiarati := map[string]map[string]string{} // praticaID → campi API dichiarati
	richiedenteCF := map[string]string{}              // praticaID → CF richiedente
	figlioCF := map[string]string{}                   // praticaID → CF figlio
	var fuoriFondi []RigaFuoriFondi

	// Sorgente primaria: cache API (aggiornata durante scan)
	if apiCache, err := db.GetAPICache(h.DB, int(bandoID)); err == nil {
		for pid, apiFields := range apiCache {
			campiDichiarati[pid] = apiFields
			for _, k := range []string{"richiedente_cf", "richiedente"} {
				if v := apiFields[k]; v != "" {
					richiedenteCF[pid] = v
					break
				}
			}
			for _, k := range []string{"figlio_cf", "figlio"} {
				if v := apiFields[k]; v != "" {
					figlioCF[pid] = v
					break
				}
			}
		}
	}

	// Fallback: snapshot ultima run (per app non ancora scansionate, fuori fondi, ecc.)
	if run, err := db.GetLatestRun(h.DB, bando.ID); err == nil && run != nil {
		var grad graduatoria.Graduatoria
		if json.Unmarshal([]byte(run.DatiJSON), &grad) == nil {
			addRiga := func(r graduatoria.RigaGraduatoria) {
				if r.Istanza == nil || r.Istanza.ID == "" {
					return
				}
				pid := r.Istanza.ID
				// Fallback: usa snapshot run solo se cache API non ha già i dati
				if campiDichiarati[pid] == nil {
					campiDichiarati[pid] = r.Istanza.CampiMappati
				}
				if richiedenteCF[pid] == "" {
					for _, k := range []string{"richiedente_cf", "richiedente"} {
						if v := r.Istanza.CampiMappati[k]; v != "" {
							richiedenteCF[pid] = v
							break
						}
					}
					if richiedenteCF[pid] == "" {
						richiedenteCF[pid] = r.Istanza.RichiedenteCF
					}
				}
				if figlioCF[pid] == "" {
					for _, k := range []string{"figlio_cf", "figlio"} {
						if v := r.Istanza.CampiMappati[k]; v != "" {
							figlioCF[pid] = v
							break
						}
					}
					if figlioCF[pid] == "" {
						figlioCF[pid] = r.Istanza.FiglioSelezionatoCF
					}
				}
			}
			for _, g := range grad.Gruppi {
				for _, r := range g.Righe {
					addRiga(r)
					if !r.Ammessa && (r.NoteEsclusione == "fondi esauriti" || r.NoteEsclusione == "posti esauriti") {
						fuoriFondi = append(fuoriFondi, RigaFuoriFondi{Gruppo: g.Nome, Riga: r})
					}
				}
			}
			for _, r := range grad.Escluse {
				addRiga(r)
			}
		}
	}

	var praticaIDs []string
	for _, ist := range istruttorie {
		praticaIDs = append(praticaIDs, ist.PraticaID)
	}
	noteAltriBandi, _ := db.GetNoteAltriBandi(h.DB, int(bandoID), praticaIDs)
	altriBandi, _ := db.GetAltriBandiPerPratiche(h.DB, int(bandoID), praticaIDs)

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
		"CampiVerifica":    campiVerifica,
		"CampiBuiltin":     campiBuiltin,
		"FuoriFondi":       fuoriFondi,
		"CampiDichiarati":  campiDichiarati,
		"RichiedenteCF":    richiedenteCF,
		"FiglioCF":         figlioCF,
		"NoteAltriBandi":   noteAltriBandi,
		"AltriBandi":       altriBandi,
	})
}

// EseguiScansioneIstruttoria esegue la scansione delle istanze per flaggare le domande da verificare.
// Se statiSelezionati è vuoto, analizza tutti gli stati.
func EseguiScansioneIstruttoria(dbConn *sql.DB, baseURL string, bando *db.Bando, jwtToken string, operatore string, statiSelezionati []string) (int, error) {
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	hasFondiCheck := ecfg.Modalita == "fondi" && ecfg.Rimborso.CampoLordo != ""
	if !ecfg.Verifica.Attiva && !hasFondiCheck {
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

	// Legge dati locali già salvati per override PRIMA del reset (altrimenti la INNER JOIN con istruttorie fallisce per le righe cancellate)
	datiLocali, _ := db.GetIstruttorieDati(dbConn, int(bando.ID))

	// Pulisce i record da_verificare (preservando quelli con note o dati locali)
	if err := db.ResetDaVerificare(dbConn, int(bando.ID)); err != nil {
		return 0, fmt.Errorf("reset da_verificare: %w", err)
	}

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

		// Salva i dati dichiarati dall'API per questa pratica (prima di applicare override locali).
		// Usato per la vista "dati locali" e per mostrare dichiarato vs sovrascitto in istruttoria.
		apiRecs, _ := generic.EstraiRecordsConExtras(app, ecfg, nil)
		if len(apiRecs) > 0 {
			rec := apiRecs[0]
			apiFields := map[string]string{
				"protocollo": app.ProtocolNumber,
				"status":     app.Status,
			}
			for k, v := range rec.FloatMap {
				apiFields[k] = strconv.FormatFloat(v, 'f', -1, 64)
			}
			for k, v := range rec.StringMap {
				if v != "" {
					apiFields[k] = v
				}
			}
			for k, v := range rec.IntMap {
				apiFields[k] = strconv.Itoa(v)
			}
			if b, err2 := json.Marshal(apiFields); err2 == nil {
				db.UpsertAPICache(dbConn, int(bando.ID), app.ID, string(b)) //nolint
			}
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
		// Built-in: corrispettivo=0 in modalità fondi (sempre, indipendentemente da Verifica.Attiva)
		if ecfg.Modalita == "fondi" && ecfg.Rimborso.CampoLordo != "" {
			lordo := ecfg.Rimborso.CampoLordo
			for _, rec := range passingRecords {
				val := float64(0)
				found := false
				if v, ok := rec.FloatMap[lordo]; ok {
					val = v
					found = true
				} else if sv, ok := rec.StringMap[lordo]; ok {
					// campo mappato come "string": parsa il valore
					sv = strings.TrimSpace(sv)
					if sv == "" || sv == "0" {
						val = 0
						found = true
					} else if parsed, err := strconv.ParseFloat(sv, 64); err == nil {
						val = parsed
						found = true
					}
				}
				if found && val == 0 {
					motiviSet["Corrispettivo dichiarato €0,00 — inserire importo speso come dato locale"] = struct{}{}
				}
			}
		}
		// Built-in: CF richiedente mancante — controlla sia richiedente_cf (built-in) che richiedente (chiave mappata comune)
		for _, rec := range passingRecords {
			hasCF := rec.StringMap["richiedente_cf"] != "" || rec.StringMap["richiedente"] != ""
			if !hasCF {
				motiviSet["CF richiedente mancante — verificare identità del richiedente"] = struct{}{}
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
		// Salva CF richiedente/figlio cross-bando — usato come fallback in istruttoria prima di una run.
		if len(passingRecords) > 0 {
			rec := passingRecords[0]
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

	hasFondiCheck := ecfg.Modalita == "fondi" && ecfg.Rimborso.CampoLordo != ""
	if !ecfg.Verifica.Attiva && !hasFondiCheck {
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
	ctx := r.FormValue("ctx") // "dati" → risponde con span minimale invece del partial motivi
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
		// Salva/aggiorna il valore API dichiarato nella cache (separato dagli override).
		apiRecs, _ := generic.EstraiRecordsConExtras(*app, ecfg, nil)
		if len(apiRecs) > 0 {
			rec := apiRecs[0]
			var apiVal string
			if v, ok := rec.FloatMap[campo]; ok {
				apiVal = strconv.FormatFloat(v, 'f', -1, 64)
			} else if v, ok := rec.IntMap[campo]; ok {
				apiVal = strconv.Itoa(v)
			} else if v, ok := rec.StringMap[campo]; ok {
				apiVal = v
			}
			if apiVal != "" {
				_ = db.UpsertAPICacheField(h.DB, int(bandoID), praticaID, campo, apiVal)
			}
		}

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
			allDati, _ = db.GetIstruttorieDati(h.DB, int(bandoID))
			extras = allDati[praticaID]
		} else if valore == "" {
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
		// Re-valuta check built-in dopo l'override.
		if ecfg.Modalita == "fondi" && ecfg.Rimborso.CampoLordo != "" {
			lordo := ecfg.Rimborso.CampoLordo
			for _, rec := range records {
				val := float64(0)
				found := false
				if v, ok := rec.FloatMap[lordo]; ok {
					val = v
					found = true
				} else if sv, ok := rec.StringMap[lordo]; ok {
					sv = strings.TrimSpace(sv)
					if sv == "" || sv == "0" {
						val = 0; found = true
					} else if parsed, err2 := strconv.ParseFloat(sv, 64); err2 == nil {
						val = parsed; found = true
					}
				}
				if found && val == 0 {
					motiviSet["Corrispettivo dichiarato €0,00 — inserire importo speso come dato locale"] = struct{}{}
				}
			}
		}
		for _, rec := range records {
			hasCF := rec.StringMap["richiedente_cf"] != "" || rec.StringMap["richiedente"] != ""
			if !hasCF {
				motiviSet["CF richiedente mancante — verificare identità del richiedente"] = struct{}{}
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

	// ctx="dati" → risposta minimale per la pagina dati_locali (span status per campo)
	if ctx == "dati" {
		spanID := "dati-status-" + praticaID + "-" + campo
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if valore == "" {
			fmt.Fprintf(w, `<span id="%s" style="color:#9ca3af;font-size:.8rem">—</span>`, html.EscapeString(spanID))
		} else {
			fmt.Fprintf(w, `<span id="%s" style="color:#16a34a;font-size:.8rem;font-family:monospace">%s ✓</span>`,
				html.EscapeString(spanID), html.EscapeString(valore))
		}
		return
	}

	stato := "da_verificare"
	if ist, err := db.GetIstruttoriaByPratica(h.DB, int(bandoID), praticaID); err == nil {
		stato = ist.Stato
	}

	renderTemplate(w, "istruttoria_dato_partial.html", map[string]any{
		"Motivi":        motivi,
		"CampiVerifica": campiVerifica,
		"Dati":          datiPratica,
		"BandoID":       bandoID,
		"PraticaID":     praticaID,
		"Stato":         stato,
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

// PraticaConDati aggrega i dati di una singola pratica per la vista "dati locali".
type PraticaConDati struct {
	PraticaID  string
	Protocollo string
	StatusApp  string
	Badge      string // "ammessa"|"fuori_fondi"|"da_verificare"|"approvata"|"esclusa"|"non_rientrante"
	DatiAPI    map[string]string // valori dichiarati dall'API (dalla cache scan)
	DatiLocali map[string]string // override operatore (da istruttorie_dati)
}

// GetDatiLocali — vista "tutte le domande" con campi API + override operatore per ciascuna.
func (h *IstruttoriaHandler) GetDatiLocali(w http.ResponseWriter, r *http.Request) {
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

	// Campi del mapping ordinati — usati per intestazioni colonne e form dinamico.
	var tuttiCampi []string
	for nome := range ecfg.Mapping {
		tuttiCampi = append(tuttiCampi, nome)
	}
	sort.Strings(tuttiCampi)

	// Dati API cache (aggiornati durante scan).
	apiCache, _ := db.GetAPICache(h.DB, int(bandoID))
	// Override operatore per bando.
	datiLocali, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
	// Stato istruttoria per pratica.
	istruttorieMap := map[string]string{} // praticaID → stato
	if istruttorie, err := db.ListIstruttorie(h.DB, int(bandoID), "", ""); err == nil {
		for _, ist := range istruttorie {
			istruttorieMap[ist.PraticaID] = ist.Stato
		}
	}
	// Ammesse/fuori fondi dall'ultima run.
	ammesseMap := map[string]string{} // praticaID → "ammessa"|"fuori_fondi"
	if run, err := db.GetLatestRun(h.DB, bando.ID); err == nil && run != nil {
		var grad graduatoria.Graduatoria
		if json.Unmarshal([]byte(run.DatiJSON), &grad) == nil {
			for _, g := range grad.Gruppi {
				for _, riga := range g.Righe {
					if riga.Istanza == nil {
						continue
					}
					if riga.Ammessa {
						ammesseMap[riga.Istanza.ID] = "ammessa"
					} else if riga.NoteEsclusione == "fondi esauriti" || riga.NoteEsclusione == "posti esauriti" {
						ammesseMap[riga.Istanza.ID] = "fuori_fondi"
					}
				}
			}
		}
	}

	// Costruisce lista pratiche ordinate.
	var pratiche []PraticaConDati
	for praticaID, apiFields := range apiCache {
		badge := "non_rientrante"
		if stato, ok := istruttorieMap[praticaID]; ok {
			badge = stato
		} else if b, ok := ammesseMap[praticaID]; ok {
			badge = b
		}
		pratiche = append(pratiche, PraticaConDati{
			PraticaID:  praticaID,
			Protocollo: apiFields["protocollo"],
			StatusApp:  apiFields["status"],
			Badge:      badge,
			DatiAPI:    apiFields,
			DatiLocali: datiLocali[praticaID],
		})
	}
	sort.Slice(pratiche, func(i, j int) bool {
		if pratiche[i].Badge != pratiche[j].Badge {
			order := map[string]int{"da_verificare": 0, "non_rientrante": 1, "ammessa": 2, "fuori_fondi": 3, "approvata": 4, "esclusa": 5}
			return order[pratiche[i].Badge] < order[pratiche[j].Badge]
		}
		return pratiche[i].Protocollo < pratiche[j].Protocollo
	})

	badgeFilter := r.URL.Query().Get("badge")
	filtered := pratiche
	if badgeFilter != "" {
		filtered = filtered[:0]
		for _, p := range pratiche {
			if p.Badge == badgeFilter {
				filtered = append(filtered, p)
			}
		}
	}

	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "dati_locali.html", map[string]any{
		"Op":          op,
		"Bando":       bando,
		"Config":      ecfg,
		"TuttiCampi":  tuttiCampi,
		"Pratiche":    filtered,
		"TotPratiche": len(pratiche),
		"BadgeFilter": badgeFilter,
		"Flash":       flash,
		"FlashType":   flashType,
	})
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
