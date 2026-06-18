package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/extractor"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type MotoriHandler struct {
	DB      *sql.DB
	BaseURL string
}

// ParametroMappato rappresenta un parametro logico configurato dall'utente nel wizard step 3.
type ParametroMappato struct {
	Nome     string
	Label    string
	Path     string
	Tipo     string
	Expand   bool
	PDNDPath string
	PDNDOp   string
	PDNDVal  string
}

// --- Lista motori ---

func (h *MotoriHandler) GetLista(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	stato := r.URL.Query().Get("stato")
	if stato == "" {
		stato = "attivo"
	}
	motori, _ := db.ListMotori(h.DB, stato)
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "motori.html", map[string]any{
		"Op":        op,
		"Motori":    motori,
		"Stato":     stato,
		"Flash":     flash,
		"FlashType": flashType,
	})
}

// --- Wizard step 1: connetti servizio ---

func (h *MotoriHandler) GetNuovo(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	renderTemplate(w, "motore_wizard.html", map[string]any{
		"Op":      op,
		"BaseURL": h.BaseURL,
	})
}

// PostConnettiServizi (HTMX) — fetch servizi da OpenCity e mostra form selezione.
func (h *MotoriHandler) PostConnettiServizi(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawServizi, err := client.FetchServices()
	if err != nil {
		renderTemplate(w, "motore_wizard_connetti.html", map[string]any{
			"Errore": err.Error(),
		})
		return
	}

	type Servizio struct {
		ID   string
		Nome string
	}
	var servizi []Servizio
	for _, raw := range rawServizi {
		var s struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &s) == nil && s.ID != "" {
			servizi = append(servizi, Servizio{ID: s.ID, Nome: s.Name})
		}
	}

	renderTemplate(w, "motore_wizard_connetti.html", map[string]any{
		"BaseURL":  h.BaseURL,
		"Servizi":  servizi,
	})
}

// PostCreaMotore — crea bando bozza dal servizio selezionato, redirect a step 2.
func (h *MotoriHandler) PostCreaMotore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}
	serviceID := strings.TrimSpace(r.FormValue("service_id"))
	if serviceID == "" {
		http.Error(w, "Selezionare un servizio", http.StatusBadRequest)
		return
	}
	bando := &db.Bando{
		ServiceID:             serviceID,
		Nome:                  strings.TrimSpace(r.FormValue("nome")),
		BudgetTotale:          parseFloat(r.FormValue("budget_totale")),
		ISEEMassimo:           parseFloat(r.FormValue("isee_massimo")),
		ScadenzaPresentazione: r.FormValue("scadenza"),
		EngineType:            "generic",
		EngineConfig:          "{}",
		Attivo:                true,
		StatoMotore:           "bozza",
		CreatedAt:             time.Now(),
	}
	if bando.Nome == "" {
		bando.Nome = "Motore " + serviceID[:8]
	}
	id, err := db.InsertBando(h.DB, bando)
	if err != nil {
		http.Error(w, "Errore creazione motore: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/2", id), http.StatusSeeOther)
}

// --- Wizard steps 2-5 + fine ---

func motoreIDFromPath(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func loadMotoreConConfig(h *MotoriHandler, r *http.Request) (*db.Bando, graduatoria.EngineConfig, error) {
	bando, err := db.GetBando(h.DB, motoreIDFromPath(r))
	if err != nil {
		return nil, graduatoria.EngineConfig{}, err
	}
	var cfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &cfg)
	if cfg.Mapping == nil {
		cfg.Mapping = make(map[string]graduatoria.FieldMapping)
	}
	return bando, cfg, nil
}

func saveEngineConfig(h *MotoriHandler, bandoID int64, cfg graduatoria.EngineConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return db.UpdateEngineConfig(h.DB, bandoID, string(data))
}

func campiMappati(cfg graduatoria.EngineConfig) []string {
	var nomi []string
	for nome := range cfg.Mapping {
		nomi = append(nomi, nome)
	}
	return nomi
}

// GetWizardStep — visualizza un passo del wizard (GET).
func (h *MotoriHandler) GetWizardStep(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	step := r.PathValue("step")
	bando, cfg, err := loadMotoreConConfig(h, r)
	if err != nil {
		notFound(w, r)
		return
	}

	switch step {
	case "2":
		renderTemplate(w, "motore_wizard_step2.html", map[string]any{
			"Op":       op,
			"Motore":   bando,
			"Modalita": cfg.Modalita,
		})

	case "3":
		client := opencity.NewClient(h.BaseURL, op.JWT)
		var errCampione string
		var sampleOffset int
		var sampleTotal int

		sampleIDParam := r.URL.Query().Get("sample_id")
		offsetParam := r.URL.Query().Get("sample_offset")

		var app *opencity.Application
		if sampleIDParam != "" {
			var err2 error
			app, err2 = client.FetchApplication(sampleIDParam)
			if err2 != nil {
				errCampione = err2.Error()
			}
			// Recupera il totale per UI prev/next (1 call separata)
			_, sampleTotal, _ = client.FetchApplicationAtOffset(bando.ServiceID, 0)
		} else {
			if offsetParam != "" {
				sampleOffset, _ = strconv.Atoi(offsetParam)
				if sampleOffset < 0 {
					sampleOffset = 0
				}
			}
			var err2 error
			app, sampleTotal, err2 = client.FetchApplicationAtOffset(bando.ServiceID, sampleOffset)
			if err2 != nil {
				errCampione = err2.Error()
			}
		}

		var flatFields []extractor.FieldPreview
		var flatJSON []byte
		if app != nil {
			flatFields = extractor.FlattenJSON(app.Data)
			flatJSON, _ = json.Marshal(flatFields)
		}

		// Costruisce lista parametri dal mapping corrente (ordine alfabetico)
		nomi := make([]string, 0, len(cfg.Mapping))
		for n := range cfg.Mapping {
			nomi = append(nomi, n)
		}
		sort.Strings(nomi)
		var params []ParametroMappato
		for _, n := range nomi {
			fm := cfg.Mapping[n]
			params = append(params, ParametroMappato{
				Nome:     n,
				Label:    fm.Label,
				Path:     fm.Path,
				Tipo:     fm.Tipo,
				Expand:   fm.Expand,
				PDNDPath: fm.PDNDPath,
				PDNDOp:   fm.PDNDOp,
				PDNDVal:  fm.PDNDVal,
			})
		}

		renderTemplate(w, "motore_wizard_step3.html", map[string]any{
			"Op":               op,
			"Motore":           bando,
			"ParametriMappati": params,
			"CampiFlat":        flatFields,
			"ErrCampione":      errCampione,
			"Espansione":       cfg.Espansione,
			"SampleID":         sampleIDParam,
			"SampleOffset":     sampleOffset,
			"SampleTotal":      sampleTotal,
			"FlatJSON":         string(flatJSON),
		})

	case "4":
		renderTemplate(w, "motore_wizard_step4.html", map[string]any{
			"Op":           op,
			"Motore":       bando,
			"Filtri":       cfg.Filtri,
			"Verifica":     cfg.Verifica,
			"CampiMappati": campiMappati(cfg),
		})

	case "5":
		if cfg.Modalita == "ammissione" || cfg.Modalita == "lista_attesa" {
			http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/fine", bando.ID), http.StatusSeeOther)
			return
		}
		renderTemplate(w, "motore_wizard_step5.html", map[string]any{
			"Op":             op,
			"Motore":         bando,
			"Modalita":       cfg.Modalita,
			"Tipologie":      cfg.Tipologie,
			"Ordinamento":    cfg.Ordinamento,
			"Deduplicazione": cfg.Deduplicazione,
			"CampiMappati":   campiMappati(cfg),
		})

	case "6":
		if cfg.Modalita != "fondi" {
			http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/fine", bando.ID), http.StatusSeeOther)
			return
		}
		renderTemplate(w, "motore_wizard_step6.html", map[string]any{
			"Op":           op,
			"Motore":       bando,
			"Rimborso":     cfg.Rimborso,
			"CampiMappati": campiMappati(cfg),
		})

	case "fine":
		renderTemplate(w, "motore_wizard_fine.html", map[string]any{
			"Op":     op,
			"Motore": bando,
			"Config": cfg,
		})

	default:
		notFound(w, r)
	}
}

// PostWizardStep — salva un passo del wizard e procede al successivo (POST).
func (h *MotoriHandler) PostWizardStep(w http.ResponseWriter, r *http.Request) {
	step := r.PathValue("step")
	bando, cfg, err := loadMotoreConConfig(h, r)
	if err != nil {
		notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}

	switch step {
	case "2":
		cfg.Modalita = strings.TrimSpace(r.FormValue("modalita"))
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/3", bando.ID), http.StatusSeeOther)

	case "3":
		cfg.Espansione = strings.TrimSpace(r.FormValue("espansione"))
		cfg.Mapping = make(map[string]graduatoria.FieldMapping)

		nomiForm := r.Form["p_nome"]
		paths := r.Form["p_path"]
		tipos := r.Form["p_tipo"]
		labels := r.Form["p_label"]
		expands := r.Form["p_expand"]
		pdndPaths := r.Form["p_pdnd_path"]
		pdndOps := r.Form["p_pdnd_op"]
		pdndVals := r.Form["p_pdnd_val"]

		for i, nome := range nomiForm {
			nome = strings.TrimSpace(nome)
			if nome == "" {
				continue
			}
			path := ""
			if i < len(paths) {
				path = strings.TrimSpace(paths[i])
			}
			if path == "" {
				continue
			}
			tipo := "string"
			if i < len(tipos) && tipos[i] != "" {
				tipo = tipos[i]
			}
			label := ""
			if i < len(labels) {
				label = strings.TrimSpace(labels[i])
			}
			expand := false
			if i < len(expands) && expands[i] == "1" {
				expand = true
			}
			pdndPath := ""
			if i < len(pdndPaths) {
				pdndPath = strings.TrimSpace(pdndPaths[i])
			}
			pdndOp := ""
			if i < len(pdndOps) {
				pdndOp = pdndOps[i]
			}
			pdndVal := ""
			if i < len(pdndVals) {
				pdndVal = strings.TrimSpace(pdndVals[i])
			}
			cfg.Mapping[nome] = graduatoria.FieldMapping{
				Path:     path,
				Tipo:     tipo,
				Label:    label,
				Expand:   expand,
				PDNDPath: pdndPath,
				PDNDOp:   pdndOp,
				PDNDVal:  pdndVal,
			}
		}
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/4", bando.ID), http.StatusSeeOther)

	case "4":
		campos := r.Form["filtro_campo"]
		ops := r.Form["filtro_op"]
		valori := r.Form["filtro_valore"]
		var filtri []graduatoria.FiltroConfig
		for i := range campos {
			if i >= len(ops) || i >= len(valori) || campos[i] == "" {
				continue
			}
			filtri = append(filtri, graduatoria.FiltroConfig{
				Campo:  campos[i],
				Op:     ops[i],
				Valore: parseFilterValue(valori[i]),
			})
		}
		cfg.Filtri = filtri

		// Filtri istruttoria (verifica manuale pre-calcolo)
		fCampi := r.Form["flag_campo"]
		fOps := r.Form["flag_op"]
		fValori := r.Form["flag_valore"]
		fMotivi := r.Form["flag_motivo"]
		var filtriFlag []graduatoria.FiltroFlagConfig
		for i := range fCampi {
			if fCampi[i] == "" {
				continue
			}
			var valore any
			if i < len(fValori) {
				valore = parseFilterValue(fValori[i])
			}
			motivo := ""
			if i < len(fMotivi) {
				motivo = fMotivi[i]
			}
			op2 := ""
			if i < len(fOps) {
				op2 = fOps[i]
			}
			filtriFlag = append(filtriFlag, graduatoria.FiltroFlagConfig{
				Campo:  fCampi[i],
				Op:     op2,
				Valore: valore,
				Motivo: motivo,
			})
		}
		cfg.Verifica = graduatoria.VerificaConfig{
			Attiva:                 r.FormValue("verifica_attiva") == "1",
			FiltriFlag:             filtriFlag,
			VerificaCertificazione: r.FormValue("verifica_certificazione") == "1",
		}

		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/5", bando.ID), http.StatusSeeOther)

	case "5":
		nomi := r.Form["tip_nome"]
		campiTip := r.Form["tip_campo"]
		valoriTip := r.Form["tip_valore"]
		priorita := r.Form["tip_priorita"]
		budgetTipo := r.Form["tip_budget_tipo"]
		budgetValore := r.Form["tip_budget_valore"]
		var tipologie []graduatoria.TipologiaConfig
		for i := range nomi {
			if nomi[i] == "" {
				continue
			}
			bv, _ := strconv.ParseFloat(budgetValore[safeIdx(budgetValore, i)], 64)
			pr, _ := strconv.Atoi(priorita[safeIdx(priorita, i)])
			tipologie = append(tipologie, graduatoria.TipologiaConfig{
				Nome:     nomi[i],
				Campo:    campiTip[safeIdx(campiTip, i)],
				Valore:   valoriTip[safeIdx(valoriTip, i)],
				Priorita: pr,
				Limite: graduatoria.LimiteConfig{
					Tipo:   budgetTipo[safeIdx(budgetTipo, i)],
					Valore: bv,
				},
			})
		}
		cfg.Tipologie = tipologie
		ordCampi := r.Form["ord_campo"]
		ordDir := r.Form["ord_dir"]
		var ordini []graduatoria.OrdineConfig
		for i := range ordCampi {
			if ordCampi[i] == "" {
				continue
			}
			dir := "asc"
			if i < len(ordDir) {
				dir = ordDir[i]
			}
			ordini = append(ordini, graduatoria.OrdineConfig{Campo: ordCampi[i], Dir: dir})
		}
		cfg.Ordinamento = ordini
		cfg.Deduplicazione.Attiva = r.FormValue("dedup_attiva") == "1"
		if chiaveRaw := strings.TrimSpace(r.FormValue("dedup_chiave")); chiaveRaw != "" {
			cfg.Deduplicazione.Chiave = splitTrim(chiaveRaw, ",")
		} else {
			cfg.Deduplicazione.Chiave = nil
		}
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/6", bando.ID), http.StatusSeeOther)

	case "6":
		cfg.Rimborso = graduatoria.RimborsoConfig{
			Tipo:           r.FormValue("rimborso_tipo"),
			CampoLordo:     r.FormValue("rimborso_campo_lordo"),
			CampoDeduzione: r.FormValue("rimborso_campo_deduzione"),
		}
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/fine", bando.ID), http.StatusSeeOther)

	default:
		notFound(w, r)
	}
}

// PostTestEngine (HTMX) — esegue il calcolo senza salvare e restituisce preview.
func (h *MotoriHandler) PostTestEngine(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bando, cfg, err := loadMotoreConConfig(h, r)
	if err != nil {
		renderTemplate(w, "motore_wizard_test.html", map[string]any{"Errore": err.Error()})
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		renderTemplate(w, "motore_wizard_test.html", map[string]any{"Errore": "Fetch istanze: " + err.Error()})
		return
	}

	apps := make([]opencity.Application, 0, len(rawApps))
	for _, raw := range rawApps {
		var a opencity.Application
		if json.Unmarshal(raw, &a) == nil {
			apps = append(apps, a)
		}
	}

	cfgJSON, _ := json.Marshal(cfg)
	bandoCfg := graduatoria.BandoConfig{
		BudgetTotale: bando.BudgetTotale,
		ISEEMassimo:  bando.ISEEMassimo,
		ExtraJSON:    cfgJSON,
	}
	if bando.ScadenzaPresentazione != "" {
		bandoCfg.Scadenza, _ = time.Parse("2006-01-02", bando.ScadenzaPresentazione)
	}

	engine, err := graduatoria.GetEngine("generic")
	if err != nil {
		renderTemplate(w, "motore_wizard_test.html", map[string]any{"Errore": err.Error()})
		return
	}
	grad, err := engine.Calcola(apps, bandoCfg)
	if err != nil {
		renderTemplate(w, "motore_wizard_test.html", map[string]any{"Errore": "Calcolo: " + err.Error()})
		return
	}

	renderTemplate(w, "motore_wizard_test.html", map[string]any{
		"Grad":       grad,
		"NumIstanze": len(apps),
	})
}

// PostAttivaMotore — imposta stato_motore='attivo', redirect a dettaglio.
func (h *MotoriHandler) PostAttivaMotore(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	if err := r.ParseForm(); err == nil {
		if nome := strings.TrimSpace(r.FormValue("nome")); nome != "" {
			bando, err := db.GetBando(h.DB, id)
			if err == nil {
				bando.Nome = nome
				db.UpdateBando(h.DB, bando)
			}
		}
	}
	if err := db.AttivaMotore(h.DB, id); err != nil {
		http.Error(w, "Errore attivazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/motori/%d?flash=Motore+attivato&flashType=success", id), http.StatusSeeOther)
}

// --- Dettaglio motore ---

func (h *MotoriHandler) GetDettaglio(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	id := motoreIDFromPath(r)
	bando, err := db.GetBando(h.DB, id)
	if err != nil {
		notFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	runs, _ := db.ListRuns(h.DB, bando.ID, !op.IsAdmin())
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)
	istrStats, _ := db.GetIstruttoriaStats(h.DB, int(bando.ID))
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "motore_dettaglio.html", map[string]any{
		"Op":               op,
		"Motore":           bando,
		"Runs":             runs,
		"Config":           ecfg,
		"IstruttoriaStats": istrStats,
		"Flash":            flash,
		"FlashType":        flashType,
	})
}

// --- CRUD actions ---

func (h *MotoriHandler) PostDuplica(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	newID, err := db.DuplicaBando(h.DB, id)
	if err != nil {
		http.Error(w, "Errore duplicazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/2?flash=Motore+duplicato,+configurazione+copiata&flashType=success", newID), http.StatusSeeOther)
}

func (h *MotoriHandler) PostArchivia(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	if err := db.ArchiviaMotore(h.DB, id); err != nil {
		http.Error(w, "Errore archiviazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/motori?flash=Motore+archiviato&flashType=success", http.StatusSeeOther)
}

// --- helpers ---

func parseFilterValue(s string) any {
	// Prova int, poi float, poi stringa.
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return float64(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func safeIdx(s []string, i int) int {
	if i < len(s) {
		return i
	}
	return 0
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
