package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type GraduatoriaHandler struct {
	DB      *sql.DB
	BaseURL string
}

// csvSistemaCols sono le colonne sistema sempre presenti nell'export CSV.
var csvSistemaCols = []string{
	"Posizione", "Categoria", "Protocollo", "Data Invio",
	"Stato", "Importo Rimborso", "Note Esclusione", "Link",
}

func csvHeadersDynamic(dinamiche []string) []string {
	headers := make([]string, len(csvSistemaCols))
	copy(headers, csvSistemaCols)
	return append(headers, dinamiche...)
}

func csvRecordDynamic(categoria string, r graduatoria.RigaGraduatoria, baseURL string, dinamiche []string) []string {
	ist := r.Istanza
	link := ""
	if ist != nil && baseURL != "" && ist.ID != "" {
		link = baseURL + "/lang/it/operatori/" + ist.ID + "/detail"
	}
	importo := ""
	if r.Ammessa {
		importo = fmt.Sprintf("%.2f", r.ImportoRimborso)
	}
	protocollo := ""
	dataInvio := ""
	stato := ""
	if ist != nil {
		protocollo = ist.ProtocolNumber
		dataInvio = ist.SubmittedAt
		if t, err := time.Parse(time.RFC3339, dataInvio); err == nil {
			dataInvio = t.Format("02/01/2006")
		}
		stato = ist.Status
	}
	row := []string{
		fmt.Sprintf("%d", r.Posizione),
		categoria,
		protocollo,
		dataInvio,
		stato,
		importo,
		r.NoteEsclusione,
		link,
	}
	for _, col := range dinamiche {
		val := ""
		if ist != nil {
			val = ist.CampiMappati[col]
		}
		row = append(row, val)
	}
	return row
}

// PostCalcola — calcola nuova run per un bando
func (h *GraduatoriaHandler) PostCalcola(w http.ResponseWriter, r *http.Request) {
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

	engine, err := graduatoria.GetEngine(bando.EngineType)
	if err != nil {
		http.Error(w, "Engine non trovato: "+bando.EngineType, http.StatusInternalServerError)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		http.Error(w, "Errore fetch istanze: "+err.Error(), http.StatusBadGateway)
		return
	}

	apps := make([]opencity.Application, 0, len(rawApps))
	for _, raw := range rawApps {
		var a opencity.Application
		if json.Unmarshal(raw, &a) == nil {
			apps = append(apps, a)
		}
	}

	// Istruttoria: filtra escluse se attiva.
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)
	if ecfg.Verifica.Attiva {
		escluse, _ := db.ListEscluse(h.DB, int(bandoID))
		if len(escluse) > 0 {
			excludeSet := make(map[string]bool, len(escluse))
			for _, id := range escluse {
				excludeSet[id] = true
			}
			filtered := apps[:0]
			for _, app := range apps {
				if !excludeSet[app.ID] {
					filtered = append(filtered, app)
				}
			}
			apps = filtered
		}
	}

	cfg := graduatoria.BandoConfig{
		BudgetTotale: bando.BudgetTotale,
		ISEEMassimo:  bando.ISEEMassimo,
		ExtraJSON:    []byte(bando.EngineConfig),
	}
	if bando.ScadenzaPresentazione != "" {
		cfg.Scadenza, _ = time.Parse("2006-01-02", bando.ScadenzaPresentazione)
	}
	if overrides, err := db.GetIstruttorieDati(h.DB, int(bandoID)); err == nil && len(overrides) > 0 {
		cfg.CampiExtra = overrides
	}

	grad, err := engine.Calcola(apps, cfg)
	if err != nil {
		http.Error(w, "Errore calcolo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	datiJSON, _ := json.Marshal(grad)
	numAmmesse := grad.TotaleAmmesse()
	budgetUsato := grad.TotaleBudgetUsato()

	run := &db.GraduatoriaRun{
		BandoID:     bando.ID,
		CalcolataDa: op.Username,
		CalcolataAt: time.Now(),
		DatiJSON:    string(datiJSON),
		NumTotale:   len(apps),
		NumAmmesse:  numAmmesse,
		NumEscluse:  len(grad.Escluse),
		BudgetUsato: budgetUsato,
	}
	runID, err := db.InsertRun(h.DB, run)
	if err != nil {
		http.Error(w, "Errore salvataggio run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	db.InsertAudit(h.DB, &db.AuditAction{
		Operatore: op.Username,
		Azione:    "calcola",
		BandoID:   bando.ID,
		RunID:     runID,
		Esito:     "ok",
	})

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/run/%d", bandoID, runID), http.StatusSeeOther)
}

// GetRun — visualizza dettaglio run (indice annualità)
func (h *GraduatoriaHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	if !op.IsAdmin() && run.Stato == "bozza" {
		http.Error(w, "Graduatoria non ancora pubblicata", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	if err := json.Unmarshal([]byte(run.DatiJSON), &grad); err != nil {
		http.Error(w, "Dati corrotti", http.StatusInternalServerError)
		return
	}

	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "run_dettaglio.html", map[string]any{
		"Op":          op,
		"Bando":       bando,
		"Run":         run,
		"Grad":        &grad,
		"NumRiserva":  grad.TotaleConRiserva(),
		"Flash":       flash,
		"FlashType":   flashType,
	})
}

// GetRunTabella — tabella con checkbox per anno/tipo
func (h *GraduatoriaHandler) GetRunTabella(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	anno, _ := strconv.Atoi(r.PathValue("anno"))
	tipo := r.PathValue("tipo") // "rette" | "mense" | "escluse"

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var righe []graduatoria.RigaGraduatoria
	if tipo == "escluse" {
		righe = grad.Escluse
	}

	datiLocali, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
	renderTemplate(w, "run_tabella.html", map[string]any{
		"Op":               op,
		"Bando":            bando,
		"Run":              run,
		"Anno":             anno,
		"Tipo":             tipo,
		"Righe":            righe,
		"RunID":            runID,
		"BandoID":          bandoID,
		"BaseURL":          h.BaseURL,
		"MappingKeys":      mappingKeysFromBando(bando.EngineConfig),
		"IstruttoriaStato": istruttoriaStatoMap(h.DB, bandoID),
		"DatiLocali":       datiLocali,
	})
}

// GetExportCSV — download CSV per anno/tipo
func (h *GraduatoriaHandler) GetExportCSV(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	anno, _ := strconv.Atoi(r.PathValue("anno"))
	tipo := r.PathValue("tipo")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var righe []graduatoria.RigaGraduatoria
	if tipo == "escluse" {
		righe = grad.Escluse
	}

	var exportColonne []string
	json.Unmarshal([]byte(bando.ExportColonne), &exportColonne)

	filename := fmt.Sprintf("run%d_%d_%s.csv", runID, anno, tipo)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write(csvHeadersDynamic(exportColonne))
	for _, riga := range righe {
		cat := tipo
		if !riga.Ammessa && riga.NoteEsclusione != "fondi esauriti" {
			cat = "esclusa"
		}
		_ = cw.Write(csvRecordDynamic(cat, riga, h.BaseURL, exportColonne))
	}
	cw.Flush()
}

// GetStampa — pagina stampabile con colonne configurabili per una run.
func (h *GraduatoriaHandler) GetStampa(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	if !op.IsAdmin() && run.Stato == "bozza" {
		http.Error(w, "Graduatoria non ancora pubblicata", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	// Colonne selezionate da query param (default se assenti).
	colParam := r.URL.Query()["col"]
	var exportColonne []string
	json.Unmarshal([]byte(bando.ExportColonne), &exportColonne)
	if len(colParam) == 0 {
		if len(exportColonne) > 0 {
			colParam = exportColonne
		} else {
			colParam = []string{"posizione", "importo", "ammessa"}
		}
	}

	renderTemplate(w, "run_stampa.html", map[string]any{
		"Op":            op,
		"Bando":         bando,
		"Run":           run,
		"Grad":          &grad,
		"Colonne":       colParam,
		"ExportColonne": exportColonne,
	})
}

// mappingKeysFromBando estrae le chiavi del mapping in ordine alfabetico dall'engine config JSON.
func mappingKeysFromBando(engineConfig string) []string {
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(engineConfig), &ecfg)
	keys := make([]string, 0, len(ecfg.Mapping))
	for k := range ecfg.Mapping {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetRunGruppo — tabella righe per un gruppo dell'engine generic.
func (h *GraduatoriaHandler) GetRunGruppo(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	nome := r.PathValue("nome")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	if !op.IsAdmin() && run.Stato == "bozza" {
		http.Error(w, "Graduatoria non ancora pubblicata", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var gruppo *graduatoria.GraduatoriaGruppo
	for _, g := range grad.Gruppi {
		if g.Nome == nome {
			gruppo = g
			break
		}
	}

	if gruppo == nil {
		http.NotFound(w, r)
		return
	}

	datiLocali, _ := db.GetIstruttorieDati(h.DB, int(bandoID))
	renderTemplate(w, "run_tabella_gruppo.html", map[string]any{
		"Op":               op,
		"Bando":            bando,
		"Run":              run,
		"Nome":             nome,
		"Gruppo":           gruppo,
		"Righe":            gruppo.Righe,
		"RunID":            runID,
		"BandoID":          bandoID,
		"BaseURL":          h.BaseURL,
		"MappingKeys":      mappingKeysFromBando(bando.EngineConfig),
		"IstruttoriaStato": istruttoriaStatoMap(h.DB, bandoID),
		"DatiLocali":       datiLocali,
	})
}

// GetExportCSVGruppo — CSV export per un gruppo dell'engine generic.
func (h *GraduatoriaHandler) GetExportCSVGruppo(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	nome := r.PathValue("nome")

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var righe []graduatoria.RigaGraduatoria
	for _, g := range grad.Gruppi {
		if g.Nome == nome {
			righe = g.Righe
			break
		}
	}

	var exportColonne []string
	json.Unmarshal([]byte(bando.ExportColonne), &exportColonne)

	filename := fmt.Sprintf("run%d_%s.csv", runID, nome)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write(csvHeadersDynamic(exportColonne))
	for _, riga := range righe {
		cat := nome
		if !riga.Ammessa {
			cat = "fuori_fondi"
		}
		_ = cw.Write(csvRecordDynamic(cat, riga, h.BaseURL, exportColonne))
	}
	cw.Flush()
}

// PostPubblica — pubblica una run (da bozza a pubblicata). Solo admin.
func (h *GraduatoriaHandler) PostPubblica(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Solo gli amministratori possono pubblicare", http.StatusForbidden)
		return
	}
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)

	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if run.Stato != "bozza" {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/run/%d?flash=Già+pubblicata&flashType=error", bandoID, runID), http.StatusSeeOther)
		return
	}
	if err := db.PubblicaRun(h.DB, runID); err != nil {
		http.Error(w, "Errore pubblicazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.InsertAudit(h.DB, &db.AuditAction{
		Operatore: op.Username,
		Azione:    "pubblica",
		BandoID:   bandoID,
		RunID:     runID,
		Esito:     "ok",
	})
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/run/%d?flash=Graduatoria+pubblicata&flashType=success", bandoID, runID), http.StatusSeeOther)
}

// GetExportIBAN — CSV ammesse con IBAN per ufficio ragioneria (ordini bonifici).
// Richiede che il bando abbia "iban" nel mapping; se assente la colonna sarà vuota.
func (h *GraduatoriaHandler) GetExportIBAN(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := db.GetRun(h.DB, runID)
	if err != nil || run.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	filename := fmt.Sprintf("run%d_iban_bonifici.csv", runID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.Write([]string{
		"Posizione", "Protocollo", "CF Richiedente", "Cognome", "Nome",
		"CF Figlio", "Annualità", "Tipologia", "Importo (€)", "IBAN", "Intestatario IBAN",
	})

	// fallbackCampo prova lo struct Istanza poi CampiMappati con chiavi alternative.
	fallbackCampo := func(structVal string, ist *graduatoria.Istanza, chiavi ...string) string {
		if structVal != "" {
			return structVal
		}
		for _, k := range chiavi {
			if v := ist.CampiMappati[k]; v != "" {
				return v
			}
		}
		return ""
	}

	pos := 0
	for _, g := range grad.Gruppi {
		for _, riga := range g.Righe {
			if !riga.Ammessa || riga.Istanza == nil {
				continue
			}
			pos++
			ist := riga.Istanza
			annualita := fallbackCampo(func() string {
				if ist.Annualita != 0 {
					return fmt.Sprintf("%d", ist.Annualita)
				}
				return ""
			}(), ist, "annualita", "annualita1", "anno")
			cw.Write([]string{
				fmt.Sprintf("%d", pos),
				ist.ProtocolNumber,
				fallbackCampo(ist.RichiedenteCF, ist, "richiedente_cf", "richiedente"),
				fallbackCampo(ist.RichiedenteCognome, ist, "richiedente_cognome", "cognome"),
				fallbackCampo(ist.RichiedenteNome, ist, "richiedente_nome", "nome"),
				fallbackCampo(ist.FiglioSelezionatoCF, ist, "figlio_cf", "figlio"),
				annualita,
				g.Nome,
				fmt.Sprintf("%.2f", riga.ImportoRimborso),
				fallbackCampo(ist.IBAN, ist, "iban"),
				fallbackCampo(ist.IBANIntestatario, ist, "iban_intestatario", "intestatario"),
			})
		}
	}
	cw.Flush()
}
