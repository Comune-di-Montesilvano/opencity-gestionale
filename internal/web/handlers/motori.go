package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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

// CampoLogico descrive un campo standard mappabile dell'engine.
type CampoLogico struct {
	Nome        string
	TipoDefault string
	Desc        string
	PathDefault string // path predefinito (es. $app:submitted_at)
}

var campiLogiciStandard = []CampoLogico{
	{"isee", "float", "Valore ISEE dell'istanza", ""},
	{"corrispettivo", "float", "Importo lordo (corrispettivo)", ""},
	{"beneficio", "float", "Beneficio già ricevuto (es. Bonus Nidi)", ""},
	{"figlio_cf", "string", "Codice fiscale figlio", ""},
	{"richiedente_cf", "string", "Codice fiscale richiedente", ""},
	{"tipo", "string", "Tipologia richiesta (rette/mensa/ecc.)", ""},
	{"annualita", "int", "Anno scolastico (es. 20242025)", ""},
	{"num_figli", "count", "Numero figli nel nucleo", ""},
	{"data_presentazione", "time", "Data presentazione domanda", "$app:submitted_at"},
	{"status", "string", "Stato istanza OpenCity", "$app:status"},
}

// CampoLogicoConValore arricchisce CampoLogico con i valori correnti dell'engine_config.
type CampoLogicoConValore struct {
	CampoLogico
	Path   string
	Tipo   string
	Expand bool
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
		// Fetch istanza campione per il viewer JSON.
		client := opencity.NewClient(h.BaseURL, op.JWT)
		var flatFields []extractor.FieldPreview
		var errCampione string
		app, err := client.FetchSampleApplication(bando.ServiceID)
		if err != nil {
			errCampione = err.Error()
		} else {
			flatFields = extractor.FlattenJSON(app.Data)
		}

		// Prepopola campi logici con valori correnti.
		var campi []CampoLogicoConValore
		for _, cl := range campiLogiciStandard {
			fm := cfg.Mapping[cl.Nome]
			path := fm.Path
			if path == "" {
				path = cl.PathDefault
			}
			tipo := fm.Tipo
			if tipo == "" {
				tipo = cl.TipoDefault
			}
			campi = append(campi, CampoLogicoConValore{
				CampoLogico: cl,
				Path:        path,
				Tipo:        tipo,
				Expand:      fm.Expand,
			})
		}

		renderTemplate(w, "motore_wizard_step2.html", map[string]any{
			"Op":           op,
			"Motore":       bando,
			"CampiLogici":  campi,
			"CampiFlat":    flatFields,
			"ErrCampione":  errCampione,
			"Espansione":   cfg.Espansione,
		})

	case "3":
		renderTemplate(w, "motore_wizard_step3.html", map[string]any{
			"Op":           op,
			"Motore":       bando,
			"Filtri":       cfg.Filtri,
			"CampiMappati": campiMappati(cfg),
		})

	case "4":
		renderTemplate(w, "motore_wizard_step4.html", map[string]any{
			"Op":             op,
			"Motore":         bando,
			"Tipologie":      cfg.Tipologie,
			"Ordinamento":    cfg.Ordinamento,
			"Deduplicazione": cfg.Deduplicazione,
			"CampiMappati":   campiMappati(cfg),
		})

	case "5":
		renderTemplate(w, "motore_wizard_step5.html", map[string]any{
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
		// Aggiorna mapping e espansione.
		cfg.Espansione = strings.TrimSpace(r.FormValue("espansione"))
		for _, cl := range campiLogiciStandard {
			path := strings.TrimSpace(r.FormValue("path_" + cl.Nome))
			if path == "" {
				delete(cfg.Mapping, cl.Nome)
				continue
			}
			cfg.Mapping[cl.Nome] = graduatoria.FieldMapping{
				Path:   path,
				Tipo:   r.FormValue("tipo_" + cl.Nome),
				Expand: r.FormValue("expand_"+cl.Nome) == "1",
			}
		}
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/3", bando.ID), http.StatusSeeOther)

	case "3":
		// Leggi filtri come array paralleli.
		campi := r.Form["filtro_campo"]
		ops := r.Form["filtro_op"]
		valori := r.Form["filtro_valore"]
		var filtri []graduatoria.FiltroConfig
		for i := range campi {
			if i >= len(ops) || i >= len(valori) {
				break
			}
			if campi[i] == "" {
				continue
			}
			val := parseFilterValue(valori[i])
			filtri = append(filtri, graduatoria.FiltroConfig{
				Campo:  campi[i],
				Op:     ops[i],
				Valore: val,
			})
		}
		cfg.Filtri = filtri
		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/4", bando.ID), http.StatusSeeOther)

	case "4":
		// Tipologie.
		nomi := r.Form["tip_nome"]
		campiTip := r.Form["tip_campo"]
		valoriTip := r.Form["tip_valore"]
		priorita := r.Form["tip_priorita"]
		budgetTipo := r.Form["tip_budget_tipo"]
		budgetValore := r.Form["tip_budget_valore"]
		var tipologie []graduatoria.TipologiaConfig
		for i := range nomi {
			if i >= len(campiTip) || nomi[i] == "" {
				continue
			}
			bv, _ := strconv.ParseFloat(budgetValore[safeIdx(budgetValore, i)], 64)
			pr, _ := strconv.Atoi(priorita[safeIdx(priorita, i)])
			tipologie = append(tipologie, graduatoria.TipologiaConfig{
				Nome:     nomi[i],
				Campo:    campiTip[safeIdx(campiTip, i)],
				Valore:   valoriTip[safeIdx(valoriTip, i)],
				Priorita: pr,
				Budget: graduatoria.BudgetConfig{
					Tipo:   budgetTipo[safeIdx(budgetTipo, i)],
					Valore: bv,
				},
			})
		}
		cfg.Tipologie = tipologie

		// Ordinamento.
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

		// Deduplicazione.
		cfg.Deduplicazione.Attiva = r.FormValue("dedup_attiva") == "1"
		chiaveRaw := strings.TrimSpace(r.FormValue("dedup_chiave"))
		if chiaveRaw != "" {
			cfg.Deduplicazione.Chiave = splitTrim(chiaveRaw, ",")
		} else {
			cfg.Deduplicazione.Chiave = nil
		}

		if err := saveEngineConfig(h, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/motori/%d/wizard/5", bando.ID), http.StatusSeeOther)

	case "5":
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
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "motore_dettaglio.html", map[string]any{
		"Op":        op,
		"Motore":    bando,
		"Runs":      runs,
		"Flash":     flash,
		"FlashType": flashType,
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
