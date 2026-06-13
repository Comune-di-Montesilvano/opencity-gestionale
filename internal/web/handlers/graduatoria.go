package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
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

	cfg := graduatoria.BandoConfig{
		BudgetTotale: bando.BudgetTotale,
		ISEEMassimo:  bando.ISEEMassimo,
	}
	if bando.ScadenzaPresentazione != "" {
		cfg.Scadenza, _ = time.Parse("2006-01-02", bando.ScadenzaPresentazione)
	}

	grad, err := engine.Calcola(apps, cfg)
	if err != nil {
		http.Error(w, "Errore calcolo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	datiJSON, _ := json.Marshal(grad)
	numAmmesse := 0
	for _, pa := range grad.PerAnno {
		numAmmesse += graduatoria.ContaAmmesse(pa.Rette) + graduatoria.ContaAmmesse(pa.Mense)
	}
	var budgetUsato float64
	for _, pa := range grad.PerAnno {
		budgetUsato += pa.BudgetUsatoRette + pa.BudgetUsatoMense
	}

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

	var grad graduatoria.Graduatoria
	if err := json.Unmarshal([]byte(run.DatiJSON), &grad); err != nil {
		http.Error(w, "Dati corrotti", http.StatusInternalServerError)
		return
	}

	renderTemplate(w, "run_dettaglio.html", map[string]any{
		"Op":    op,
		"Bando": bando,
		"Run":   run,
		"Grad":  &grad,
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
	for _, pa := range grad.PerAnno {
		if pa.Annualita != anno {
			continue
		}
		switch tipo {
		case "rette":
			righe = pa.Rette
		case "mense":
			righe = pa.Mense
		}
	}
	if tipo == "escluse" {
		righe = grad.Escluse
	}

	renderTemplate(w, "run_tabella.html", map[string]any{
		"Op":     op,
		"Bando":  bando,
		"Run":    run,
		"Anno":   anno,
		"Tipo":   tipo,
		"Righe":  righe,
		"RunID":  runID,
		"BandoID": bandoID,
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

	engine, _ := graduatoria.GetEngine(bando.EngineType)

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var righe []graduatoria.RigaGraduatoria
	for _, pa := range grad.PerAnno {
		if pa.Annualita != anno {
			continue
		}
		switch tipo {
		case "rette":
			righe = pa.Rette
		case "mense":
			righe = pa.Mense
		}
	}
	if tipo == "escluse" {
		righe = grad.Escluse
	}

	filename := fmt.Sprintf("run%d_%d_%s.csv", runID, anno, tipo)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.Write(engine.CSVHeaders())
	for _, riga := range righe {
		cat := tipo
		if !riga.Ammessa && riga.NoteEsclusione != "fondi esauriti" {
			cat = "esclusa"
		}
		cw.Write(engine.CSVRecord(cat, riga))
	}
	cw.Flush()
}
