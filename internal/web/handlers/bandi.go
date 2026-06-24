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
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/extractor"
	"opencity-gestionale/internal/graduatoria/generic"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type BandiHandler struct {
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
	VerificaPath string
	VerificaOp   string
	VerificaVal  string
}

// WizardNavItem rappresenta un elemento della barra di navigazione del wizard.
type WizardNavItem struct {
	Step   string // "2"…"6", "fine"
	Label  string
	Active bool
	URL    string // "" se non navigabile
}

// buildWizardNav costruisce la slice di WizardNavItem per il partial wizard-nav.
// navLinks=true rende ogni step cliccabile (edit mode o step 6 in creation).
func buildWizardNav(bandoID int64, stepCorrente string, navLinks bool) []WizardNavItem {
	steps := []struct{ step, label string }{
		{"2", "Tipo bando"},
		{"3", "Parametri"},
		{"4", "Filtri"},
		{"5", "Tipologie"},
		{"6", "Rimborso"},
		{"fine", "Fine"},
	}
	items := make([]WizardNavItem, 0, len(steps))
	for _, s := range steps {
		item := WizardNavItem{
			Step:   s.step,
			Label:  s.label,
			Active: s.step == stepCorrente,
		}
		if navLinks {
			item.URL = fmt.Sprintf("/bandi/%d/wizard/%s", bandoID, s.step)
		}
		items = append(items, item)
	}
	return items
}

// --- Lista bandi ---

func (h *BandiHandler) GetLista(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	stato := r.URL.Query().Get("stato")
	if stato == "" {
		stato = "attivo"
	}
	motori, _ := db.ListBandi(h.DB, stato)
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "bandi.html", map[string]any{
		"Op":        op,
		"Bandi":    motori,
		"Stato":     stato,
		"Flash":     flash,
		"FlashType": flashType,
	})
}

// --- Wizard step 1: connetti servizio ---

func (h *BandiHandler) GetNuovo(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	renderTemplate(w, "bando_wizard.html", map[string]any{
		"Op":      op,
		"BaseURL": h.BaseURL,
	})
}

// PostConnettiServizi (HTMX) — fetch servizi da OpenCity e mostra form selezione.
func (h *BandiHandler) PostConnettiServizi(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawServizi, err := client.FetchServices()
	if err != nil {
		renderTemplate(w, "bando_wizard_connetti.html", map[string]any{
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

	renderTemplate(w, "bando_wizard_connetti.html", map[string]any{
		"BaseURL":  h.BaseURL,
		"Servizi":  servizi,
	})
}

// PostCreaMotore — crea bando bozza dal servizio selezionato, redirect a step 2.
func (h *BandiHandler) PostCreaBando(w http.ResponseWriter, r *http.Request) {
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
		StatoBando:           "bozza",
		CreatedAt:             time.Now(),
	}
	if bando.Nome == "" {
		bando.Nome = "Bando " + serviceID[:8]
	}
	id, err := db.InsertBando(h.DB, bando)
	if err != nil {
		http.Error(w, "Errore creazione bando: "+err.Error(), http.StatusInternalServerError)
		return
	}
	bando.ID = id
	op := middleware.FromContext(r.Context())
	if op != nil {
		EseguiScansioneIstruttoria(h.DB, h.BaseURL, bando, op.JWT, op.Username, nil)
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/2", id), http.StatusSeeOther)
}

// --- Wizard steps 2-5 + fine ---

func motoreIDFromPath(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func loadBandoConConfig(h *BandiHandler, r *http.Request) (*db.Bando, graduatoria.EngineConfig, error) {
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

func saveEngineConfig(h *BandiHandler, r *http.Request, bandoID int64, cfg graduatoria.EngineConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := db.UpdateEngineConfig(h.DB, bandoID, string(data)); err != nil {
		return err
	}
	bando, err := db.GetBando(h.DB, bandoID)
	if err == nil {
		op := middleware.FromContext(r.Context())
		if op != nil {
			jwt := op.JWT
			username := op.Username
			go EseguiScansioneIstruttoria(h.DB, h.BaseURL, bando, jwt, username, nil)
		}
	}
	return nil
}

func campiMappati(cfg graduatoria.EngineConfig) []string {
	var nomi []string
	for nome := range cfg.Mapping {
		nomi = append(nomi, nome)
	}
	return nomi
}

// GetWizardStep — visualizza un passo del wizard (GET).
func (h *BandiHandler) GetWizardStep(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	step := r.PathValue("step")
	bando, cfg, err := loadBandoConConfig(h, r)
	if err != nil {
		notFound(w, r)
		return
	}

	switch step {
	case "2":
		isEdit := bando.StatoBando == "attivo"
		navLinks := isEdit || step == "6"
		renderTemplate(w, "bando_wizard_step2.html", map[string]any{
			"Op":           op,
			"Bando":        bando,
			"Modalita":     cfg.Modalita,
			"Istanza":      cfg.Istanza,
			"StepCorrente": step,
			"WizardNav":    buildWizardNav(bando.ID, step, navLinks),
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
		rawDataJSON := ""
		if app != nil {
			flatFields = extractor.FlattenJSON(app.Data)
			flatJSON, _ = json.Marshal(flatFields)
			rawDataJSON = string(app.Data)
		}

		viewerFilter := r.URL.Query().Get("viewer_filter")

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
				VerificaPath: fm.VerificaPath,
				VerificaOp:   fm.VerificaOp,
				VerificaVal:  fm.VerificaVal,
			})
		}

		isEdit3 := bando.StatoBando == "attivo"
		navLinks3 := isEdit3 || step == "6"
		renderTemplate(w, "bando_wizard_step3.html", map[string]any{
			"Op":               op,
			"Bando":            bando,
			"ParametriMappati": params,
			"CampiFlat":        flatFields,
			"ErrCampione":      errCampione,
			"Espansione":       cfg.Espansione,
			"SampleID":         sampleIDParam,
			"SampleOffset":     sampleOffset,
			"SampleTotal":      sampleTotal,
			"FlatJSON":         string(flatJSON),
			"RawDataJSON":      rawDataJSON,
			"ViewerFilter":     viewerFilter,
			"SupersetJSON":     bando.ValoriSuperset,
			"StepCorrente":     step,
			"WizardNav":        buildWizardNav(bando.ID, step, navLinks3),
		})

	case "4":
		type AppEsempio struct {
			ID             string
			ProtocolNumber string
			URL            string
		}
		type StatoItem struct {
			Code   string
			Label  string
			Count  int
			Esempi []AppEsempio
		}
		var statiDisponibili []StatoItem
		if bando.ValoriSuperset != "" {
			var sup map[string]map[string][]string
			if json.Unmarshal([]byte(bando.ValoriSuperset), &sup) == nil {
				statusCounts := sup["$status_counts"] // map[code][]{"N"}
				for _, code := range sup["$app"]["status"] {
					label := code
					if names := sup["$status_names"][code]; len(names) > 0 {
						label = names[0] + " (" + code + ")"
					}
					count := 0
					if statusCounts != nil {
						if vals := statusCounts[code]; len(vals) > 0 {
							count, _ = strconv.Atoi(vals[0])
						}
					}
					statiDisponibili = append(statiDisponibili, StatoItem{Code: code, Label: label, Count: count})
				}
				sort.Slice(statiDisponibili, func(i, j int) bool {
					return statiDisponibili[i].Code < statiDisponibili[j].Code
				})
				// Fetch up to 3 example applications per status
				client := opencity.NewClient(h.BaseURL, op.JWT)
				for i := range statiDisponibili {
					apps, err := client.FetchApplicationsByStatus(bando.ServiceID, statiDisponibili[i].Code, 3)
					if err != nil {
						continue
					}
					for _, app := range apps {
						statiDisponibili[i].Esempi = append(statiDisponibili[i].Esempi, AppEsempio{
							ID:             app.ID,
							ProtocolNumber: app.ProtocolNumber,
							URL:            h.BaseURL + "/lang/applications/" + app.ID,
						})
					}
				}
			}
		}
		isEdit4 := bando.StatoBando == "attivo"
		navLinks4 := isEdit4 || step == "6"
		renderTemplate(w, "bando_wizard_step4.html", map[string]any{
			"Op":               op,
			"Bando":            bando,
			"Filtri":           cfg.Filtri,
			"Verifica":         cfg.Verifica,
			"CampiMappati":     campiMappati(cfg),
			"Istanza":          cfg.Istanza,
			"StatiDisponibili": statiDisponibili,
			"StepCorrente":     step,
			"WizardNav":        buildWizardNav(bando.ID, step, navLinks4),
		})

	case "5":
		if cfg.Modalita == "ammissione" || cfg.Modalita == "lista_attesa" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/fine", bando.ID), http.StatusSeeOther)
			return
		}
		mappingJSON, _ := json.Marshal(cfg.Mapping)
		isEdit5 := bando.StatoBando == "attivo"
		navLinks5 := isEdit5 || step == "6"
		renderTemplate(w, "bando_wizard_step5.html", map[string]any{
			"Op":             op,
			"Bando":          bando,
			"Modalita":       cfg.Modalita,
			"Tipologie":      cfg.Tipologie,
			"Ordinamento":    cfg.Ordinamento,
			"Deduplicazione": cfg.Deduplicazione,
			"CampiMappati":   campiMappati(cfg),
			"SupersetJSON":   bando.ValoriSuperset,
			"MappingJSON":    string(mappingJSON),
			"Espansione":     cfg.Espansione,
			"StepCorrente":   step,
			"WizardNav":      buildWizardNav(bando.ID, step, navLinks5),
		})

	case "6":
		if cfg.Modalita != "fondi" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/fine", bando.ID), http.StatusSeeOther)
			return
		}
		isEdit6 := bando.StatoBando == "attivo"
		navLinks6 := isEdit6 || step == "6"
		renderTemplate(w, "bando_wizard_step6.html", map[string]any{
			"Op":           op,
			"Bando":        bando,
			"Rimborso":     cfg.Rimborso,
			"CampiMappati": campiMappati(cfg),
			"StepCorrente": step,
			"WizardNav":    buildWizardNav(bando.ID, step, navLinks6),
		})

	case "fine":
		isEditFine := bando.StatoBando == "attivo"
		navLinksFine := isEditFine || step == "6"
		renderTemplate(w, "bando_wizard_fine.html", map[string]any{
			"Op":           op,
			"Bando":        bando,
			"Config":       cfg,
			"StepCorrente": step,
			"WizardNav":    buildWizardNav(bando.ID, step, navLinksFine),
		})

	default:
		notFound(w, r)
	}
}

// PostWizardStep — salva un passo del wizard e procede al successivo (POST).
func (h *BandiHandler) PostWizardStep(w http.ResponseWriter, r *http.Request) {
	step := r.PathValue("step")
	bando, cfg, err := loadBandoConConfig(h, r)
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
		cfg.Istanza.DataMinima = strings.TrimSpace(r.FormValue("data_minima"))
		cfg.Istanza.DataMassima = strings.TrimSpace(r.FormValue("data_massima"))
		if err := saveEngineConfig(h, r, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if r.FormValue("save_only") == "1" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/2", bando.ID), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/3", bando.ID), http.StatusSeeOther)

	case "3":
		cfg.Espansione = strings.TrimSpace(r.FormValue("espansione"))
		cfg.Mapping = make(map[string]graduatoria.FieldMapping)

		nomiForm := r.Form["p_nome"]
		paths := r.Form["p_path"]
		tipos := r.Form["p_tipo"]
		labels := r.Form["p_label"]
		expands := r.Form["p_expand"]
		verificaPaths := r.Form["p_verifica_path"]
		verificaOps := r.Form["p_verifica_op"]
		verificaVals := r.Form["p_verifica_val"]

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
			verificaPath := ""
			if i < len(verificaPaths) {
				verificaPath = strings.TrimSpace(verificaPaths[i])
			}
			verificaOp := ""
			if i < len(verificaOps) {
				verificaOp = verificaOps[i]
			}
			verificaVal := ""
			if i < len(verificaVals) {
				verificaVal = strings.TrimSpace(verificaVals[i])
			}
			cfg.Mapping[nome] = graduatoria.FieldMapping{
				Path:         path,
				Tipo:         tipo,
				Label:        label,
				Expand:       expand,
				VerificaPath: verificaPath,
				VerificaOp:   verificaOp,
				VerificaVal:  verificaVal,
			}
		}
		if err := saveEngineConfig(h, r, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		vf := r.FormValue("viewer_filter")
		if navOffset := strings.TrimSpace(r.FormValue("nav_to_offset")); navOffset != "" {
			u := fmt.Sprintf("/bandi/%d/wizard/3?sample_offset=%s", bando.ID, url.QueryEscape(navOffset))
			if vf != "" {
				u += "&viewer_filter=" + url.QueryEscape(vf)
			}
			http.Redirect(w, r, u, http.StatusSeeOther)
			return
		}
		if r.FormValue("save_only") == "1" {
			offset := strings.TrimSpace(r.FormValue("current_offset"))
			if offset == "" {
				offset = "0"
			}
			u := fmt.Sprintf("/bandi/%d/wizard/3?sample_offset=%s", bando.ID, url.QueryEscape(offset))
			if vf != "" {
				u += "&viewer_filter=" + url.QueryEscape(vf)
			}
			http.Redirect(w, r, u, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/4", bando.ID), http.StatusSeeOther)

	case "4":
		campos := r.Form["filtro_campo"]
		ops := r.Form["filtro_op"]
		valori := r.Form["filtro_valore"]
		gruppi := r.Form["filtro_gruppo"]
		var filtri []graduatoria.FiltroConfig
		for i := range campos {
			if i >= len(ops) || i >= len(valori) || campos[i] == "" {
				continue
			}
			gruppo := 0
			if i < len(gruppi) {
				gruppo, _ = strconv.Atoi(strings.TrimSpace(gruppi[i]))
			}
			filtri = append(filtri, graduatoria.FiltroConfig{
				Campo:  campos[i],
				Op:     ops[i],
				Valore: parseFilterValue(valori[i]),
				Gruppo: gruppo,
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

		// Filtri istanza (stato OpenCity + date presentazione)
		statiAmmessi := r.Form["stati_ammessi"]
		cfg.Istanza = graduatoria.FiltriIstanzaConfig{
			StatiAmmessi: statiAmmessi,
			DataMassima:  strings.TrimSpace(r.FormValue("data_massima")),
			DataMinima:   strings.TrimSpace(r.FormValue("data_minima")),
		}

		if err := saveEngineConfig(h, r, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if r.FormValue("save_only") == "1" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/4", bando.ID), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/5", bando.ID), http.StatusSeeOther)

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
		if err := r.ParseForm(); err == nil {
			chiave := make([]string, 0)
			for _, c := range r.Form["dedup_campo"] {
				if c = strings.TrimSpace(c); c != "" {
					chiave = append(chiave, c)
				}
			}
			if len(chiave) > 0 {
				cfg.Deduplicazione.Chiave = chiave
			} else {
				cfg.Deduplicazione.Chiave = nil
			}
		}
		if err := saveEngineConfig(h, r, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if r.FormValue("save_only") == "1" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/5", bando.ID), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/6", bando.ID), http.StatusSeeOther)

	case "6":
		cfg.Rimborso = graduatoria.RimborsoConfig{
			Tipo:           r.FormValue("rimborso_tipo"),
			CampoLordo:     r.FormValue("rimborso_campo_lordo"),
			CampoDeduzione: r.FormValue("rimborso_campo_deduzione"),
		}
		if err := saveEngineConfig(h, r, bando.ID, cfg); err != nil {
			http.Error(w, "Errore salvataggio: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if r.FormValue("save_only") == "1" {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/6", bando.ID), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/fine", bando.ID), http.StatusSeeOther)

	default:
		notFound(w, r)
	}
}

// PostTestEngine (HTMX) — esegue il calcolo senza salvare e restituisce preview.
func (h *BandiHandler) PostTestEngine(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bando, cfg, err := loadBandoConConfig(h, r)
	if err != nil {
		renderTemplate(w, "bando_wizard_test.html", map[string]any{"Errore": err.Error()})
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		renderTemplate(w, "bando_wizard_test.html", map[string]any{"Errore": "Fetch istanze: " + err.Error()})
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
		renderTemplate(w, "bando_wizard_test.html", map[string]any{"Errore": err.Error()})
		return
	}
	grad, err := engine.Calcola(apps, bandoCfg)
	if err != nil {
		renderTemplate(w, "bando_wizard_test.html", map[string]any{"Errore": "Calcolo: " + err.Error()})
		return
	}

	renderTemplate(w, "bando_wizard_test.html", map[string]any{
		"Grad":       grad,
		"NumIstanze": len(apps),
	})
}

// PostAttivaBando — imposta stato_bando='attivo', redirect a dettaglio.
func (h *BandiHandler) PostAttivaBando(w http.ResponseWriter, r *http.Request) {
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
	if err := db.AttivaBando(h.DB, id); err != nil {
		http.Error(w, "Errore attivazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Bando+attivato&flashType=success", id), http.StatusSeeOther)
}

// --- Dettaglio bando ---

func (h *BandiHandler) GetDettaglio(w http.ResponseWriter, r *http.Request) {
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
	renderTemplate(w, "bando_dettaglio.html", map[string]any{
		"Op":               op,
		"Bando":           bando,
		"Runs":             runs,
		"Config":           ecfg,
		"IstruttoriaStats": istrStats,
		"Flash":            flash,
		"FlashType":        flashType,
	})
}

// BandoExport è il formato JSON per export/import di un bando.
type BandoExport struct {
	Version               string                   `json:"version"`
	ExportedAt            string                   `json:"exported_at"`
	Nome                  string                   `json:"nome"`
	ServiceID             string                   `json:"service_id"`
	BudgetTotale          float64                  `json:"budget_totale"`
	ISEEMassimo           float64                  `json:"isee_massimo"`
	ScadenzaPresentazione string                   `json:"scadenza_presentazione"`
	EngineType            string                   `json:"engine_type"`
	EngineConfig          graduatoria.EngineConfig `json:"engine_config"`
}

// GetExportBando scarica la configurazione completa di un bando come JSON.
func (h *BandiHandler) GetExportBando(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	bandoID := bandoIDFromPath(r)
	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var ecfg graduatoria.EngineConfig
	if bando.EngineConfig != "" {
		if err := json.Unmarshal([]byte(bando.EngineConfig), &ecfg); err != nil {
			http.Error(w, "Configurazione engine corrotta: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	exp := BandoExport{
		Version:               "1",
		ExportedAt:            time.Now().UTC().Format(time.RFC3339),
		Nome:                  bando.Nome,
		ServiceID:             bando.ServiceID,
		BudgetTotale:          bando.BudgetTotale,
		ISEEMassimo:           bando.ISEEMassimo,
		ScadenzaPresentazione: bando.ScadenzaPresentazione,
		EngineType:            bando.EngineType,
		EngineConfig:          ecfg,
	}

	out, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		http.Error(w, "Errore serializzazione", http.StatusInternalServerError)
		return
	}

	// Sanitizza nome per filename (sostituisce caratteri non-safe)
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, bando.Nome)
	filename := fmt.Sprintf("bando_%s.json", safeName)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write(out)
}

// GetImport carica la pagina di importazione bando mostrando la lista dei servizi disponibili.
func (h *BandiHandler) GetImport(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawServizi, err := client.FetchServices()

	type Servizio struct {
		ID   string
		Nome string
	}
	var servizi []Servizio
	var errText string
	if err != nil {
		errText = err.Error()
	} else {
		for _, raw := range rawServizi {
			var s struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &s) == nil && s.ID != "" {
				servizi = append(servizi, Servizio{ID: s.ID, Nome: s.Name})
			}
		}
	}

	renderTemplate(w, "bando_import.html", map[string]any{
		"Op":      op,
		"Servizi": servizi,
		"Errore":  errText,
	})
}

// PostImportBando carica un file JSON esportato e crea un nuovo bando in stato bozza.
func (h *BandiHandler) PostImportBando(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	// Limite 1 MB per il file
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Redirect(w, r, "/?flash=File+troppo+grande+o+form+non+valido&flashType=error", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("bando_json")
	if err != nil {
		http.Redirect(w, r, "/?flash=File+non+fornito&flashType=error", http.StatusSeeOther)
		return
	}
	defer file.Close()

	var exp BandoExport
	if err := json.NewDecoder(file).Decode(&exp); err != nil {
		http.Redirect(w, r, "/?flash=JSON+non+valido:+"+url.QueryEscape(err.Error())+"&flashType=error", http.StatusSeeOther)
		return
	}

	if exp.Version != "1" {
		http.Redirect(w, r, "/?flash=Versione+export+non+supportata&flashType=error", http.StatusSeeOther)
		return
	}

	serviceID := strings.TrimSpace(r.FormValue("service_id"))
	if serviceID == "" {
		serviceID = exp.ServiceID
	}

	if serviceID == "" || exp.EngineType == "" {
		http.Redirect(w, r, "/?flash=File+export+non+valido+(service_id+o+engine_type+mancante)&flashType=error", http.StatusSeeOther)
		return
	}

	ecfgBytes, err := json.Marshal(exp.EngineConfig)
	if err != nil {
		http.Redirect(w, r, "/?flash=Errore+serializzazione+config&flashType=error", http.StatusSeeOther)
		return
	}

	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		nome = exp.Nome
		if nome == "" {
			nome = "Bando importato"
		} else {
			nome = nome + " (importato)"
		}
	}

	newBando := &db.Bando{
		ServiceID:             serviceID,
		Nome:                  nome,
		BudgetTotale:          exp.BudgetTotale,
		ISEEMassimo:           exp.ISEEMassimo,
		ScadenzaPresentazione: exp.ScadenzaPresentazione,
		EngineType:            exp.EngineType,
		EngineConfig:          string(ecfgBytes),
		Attivo:                true,
		StatoBando:           "bozza",
		CreatedAt:             time.Now(),
	}

	newID, err := db.InsertBando(h.DB, newBando)
	if err != nil {
		http.Redirect(w, r, "/?flash=Errore+creazione+bando:+"+url.QueryEscape(err.Error())+"&flashType=error", http.StatusSeeOther)
		return
	}
	newBando.ID = newID
	EseguiScansioneIstruttoria(h.DB, h.BaseURL, newBando, op.JWT, op.Username, nil)

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/2?flash=Bando+importato+con+successo&flashType=success", newID), http.StatusSeeOther)
}

// GetEditParametri visualizza il form HTMX per la modifica dei parametri di un bando.
func (h *BandiHandler) GetEditParametri(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	id := bandoIDFromPath(r)
	bando, err := db.GetBando(h.DB, id)
	if err != nil {
		notFound(w, r)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawServizi, err := client.FetchServices()

	type Servizio struct {
		ID   string
		Nome string
	}
	var servizi []Servizio
	if err == nil {
		for _, raw := range rawServizi {
			var s struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &s) == nil && s.ID != "" {
				servizi = append(servizi, Servizio{ID: s.ID, Nome: s.Name})
			}
		}
	}

	renderTemplate(w, "bando_parametri_edit.html", map[string]any{
		"Op":      op,
		"Bando":   bando,
		"Servizi": servizi,
	})
}

// PostEditParametri salva le modifiche ai parametri del bando.
func (h *BandiHandler) PostEditParametri(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	if !op.IsAdmin() {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}
	id := bandoIDFromPath(r)
	bando, err := db.GetBando(h.DB, id)
	if err != nil {
		notFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}

	nome := strings.TrimSpace(r.FormValue("nome"))
	serviceID := strings.TrimSpace(r.FormValue("service_id"))

	if nome == "" || serviceID == "" {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Nome+e+Servizio+sono+obbligatori&flashType=error", id), http.StatusSeeOther)
		return
	}

	bando.Nome = nome
	bando.ServiceID = serviceID
	bando.BudgetTotale = parseFloat(r.FormValue("budget_totale"))
	bando.ISEEMassimo = parseFloat(r.FormValue("isee_massimo"))
	bando.ScadenzaPresentazione = r.FormValue("scadenza")

	if err := db.UpdateBando(h.DB, bando); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Errore+salvataggio:+%s&flashType=error", id, url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}
	EseguiScansioneIstruttoria(h.DB, h.BaseURL, bando, op.JWT, op.Username, nil)

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Parametri+aggiornati&flashType=success", id), http.StatusSeeOther)
}


// --- CRUD actions ---

func (h *BandiHandler) PostDuplica(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	newID, err := db.DuplicaBando(h.DB, id)
	if err != nil {
		http.Error(w, "Errore duplicazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/wizard/2?flash=Bando+duplicato,+configurazione+copiata&flashType=success", newID), http.StatusSeeOther)
}

func (h *BandiHandler) PostArchivia(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	if err := db.ArchiviaBando(h.DB, id); err != nil {
		http.Error(w, "Errore archiviazione: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bandi?flash=Bando+archiviato&flashType=success", http.StatusSeeOther)
}

func (h *BandiHandler) PostRinomina(w http.ResponseWriter, r *http.Request) {
	id := motoreIDFromPath(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}
	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Nome+non+può+essere+vuoto&flashType=error", id), http.StatusSeeOther)
		return
	}
	bando, err := db.GetBando(h.DB, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	bando.Nome = nome
	if err := db.UpdateBando(h.DB, bando); err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d?flash=Bando+rinominato&flashType=success", id), http.StatusSeeOther)
}

// --- Superset valori campo ---

// PostBuildSuperset scarica tutte le istanze del servizio e raccoglie i valori unici
// di ogni sub-campo degli array trovati nel payload. Salva in bandi.valori_superset.
// POST /bandi/{id}/wizard/superset
func (h *BandiHandler) PostBuildSuperset(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bando, err := db.GetBando(h.DB, bandoIDFromPath(r))
	if err != nil {
		http.Error(w, "motore non trovato", http.StatusNotFound)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		http.Error(w, "errore fetch istanze: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Carica filtri istanza (date e stati) configurati nello step 2/4
	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	// sets[arrayPath][fieldKey] = set di valori unici
	sets := map[string]map[string]map[string]struct{}{}
	statusCounts := map[string]int{} // code → numero istanze

	for _, rawApp := range rawApps {
		var app opencity.Application
		if json.Unmarshal(rawApp, &app) != nil || len(app.Data) == 0 {
			continue
		}
		if !generic.PassaFiltriIstanza(app, ecfg.Istanza) {
			continue
		}
		// Raccoglie app.Status sotto "$app"."status", labels e conteggi
		if app.Status != "" {
			statusCounts[app.Status]++
			if _, ok := sets["$app"]; !ok {
				sets["$app"] = map[string]map[string]struct{}{}
			}
			if _, ok := sets["$app"]["status"]; !ok {
				sets["$app"]["status"] = map[string]struct{}{}
			}
			sets["$app"]["status"][app.Status] = struct{}{}
			if app.StatusName != "" {
				if _, ok := sets["$status_names"]; !ok {
					sets["$status_names"] = map[string]map[string]struct{}{}
				}
				if _, ok := sets["$status_names"][app.Status]; !ok {
					sets["$status_names"][app.Status] = map[string]struct{}{}
				}
				sets["$status_names"][app.Status][app.StatusName] = struct{}{}
			}
		}
		var dataMap map[string]any
		if json.Unmarshal(app.Data, &dataMap) != nil {
			continue
		}
		for topKey, topVal := range dataMap {
			arr, ok := topVal.([]any)
			if !ok || len(arr) == 0 {
				continue
			}
			// Salta array di file (Form.IO upload): primo elemento ha chiave "url"
			if m0, ok := arr[0].(map[string]any); ok {
				if _, hasURL := m0["url"]; hasURL {
					continue // allegati: non utili nel superset
				}
			}
			if _, hasSet := sets[topKey]; !hasSet {
				sets[topKey] = map[string]map[string]struct{}{}
			}
			for _, elem := range arr {
				m, ok := elem.(map[string]any)
				if !ok {
					continue
				}
				for k, v := range m {
					if v == nil {
						continue
					}
					var s string
					switch n := v.(type) {
					case float64:
						s = strconv.FormatFloat(n, 'f', -1, 64)
					case bool:
						if n {
							s = "true"
						} else {
							s = "false"
						}
					case string:
						s = n
					default:
						s = fmt.Sprintf("%v", n)
					}
					if s == "" {
						continue
					}
					if _, hasField := sets[topKey][k]; !hasField {
						sets[topKey][k] = map[string]struct{}{}
					}
					if len(sets[topKey][k]) < 200 {
						sets[topKey][k][s] = struct{}{}
					}
				}
			}
		}
	}

	// Converti set → slice ordinata
	superset := make(map[string]map[string][]string, len(sets))
	for arrayPath, fields := range sets {
		superset[arrayPath] = make(map[string][]string, len(fields))
		for field, valSet := range fields {
			vals := make([]string, 0, len(valSet))
			for v := range valSet {
				vals = append(vals, v)
			}
			sort.Strings(vals)
			superset[arrayPath][field] = vals
		}
	}
	// Salva conteggi per stato come "$status_counts"[code] = ["N"]
	if len(statusCounts) > 0 {
		counts := make(map[string][]string, len(statusCounts))
		for code, n := range statusCounts {
			counts[code] = []string{strconv.Itoa(n)}
		}
		superset["$status_counts"] = counts
	}

	b, _ := json.Marshal(superset)
	if err := db.SaveValoriSuperset(h.DB, bando.ID, string(b)); err != nil {
		http.Error(w, "errore salvataggio: "+err.Error(), http.StatusInternalServerError)
		return
	}

	arrayCount := len(superset)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Superset aggiornato: %d istanze, %d array", len(rawApps), arrayCount)
}

// --- API helper wizard step 5 ---

// GetStatisticheField aggrega i valori di un campo mappato su tutte le istanze del servizio.
// Per campi stringa/testo: valori unici con conteggio. Per numerici: somma + conteggio elementi.
// GET /bandi/{id}/api/statistiche-campo?campo=tipo
func (h *BandiHandler) GetStatisticheField(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bando, cfg, err := loadBandoConConfig(h, r)
	if err != nil {
		http.Error(w, `{"errore":"motore non trovato"}`, http.StatusNotFound)
		return
	}

	campo := strings.TrimSpace(r.URL.Query().Get("campo"))
	if campo == "" {
		http.Error(w, `{"errore":"parametro campo obbligatorio"}`, http.StatusBadRequest)
		return
	}
	fm, ok := cfg.Mapping[campo]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"errore": "campo non trovato nel mapping"})
		return
	}

	isNumerico := fm.Tipo == "float" || fm.Tipo == "int" || fm.Tipo == "count"

	// Per campi stringa: cerca fm.Path nel superset DB (tutti gli array, non solo cfg.Espansione).
	// Gestisce expand=false, espansione vuota, e mismatch di path.
	if !isNumerico && bando.ValoriSuperset != "" && bando.ValoriSuperset != "{}" {
		var superset map[string]map[string][]string
		if json.Unmarshal([]byte(bando.ValoriSuperset), &superset) == nil {
			// Cerca prima nell'array configurato, poi in tutti gli altri.
			ordine := []string{cfg.Espansione}
			for k := range superset {
				if k != cfg.Espansione {
					ordine = append(ordine, k)
				}
			}
			for _, arrKey := range ordine {
				if arrKey == "" {
					continue
				}
				if vals, ok := superset[arrKey][fm.Path]; ok && len(vals) > 0 {
					valori := vals
					if len(valori) > 20 {
						valori = valori[:20]
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{
						"valori":          valori,
						"conteggi":        map[string]int{},
						"totale_istanze":  0,
						"totale_elementi": 0,
						"tipo_campo":      fm.Tipo,
						"da_superset":     true,
					})
					return
				}
			}
		}
	}

	// Fallback: fetch live da OpenCity (campi numerici o senza superset).
	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"errore": err.Error()})
		return
	}

	valCount := make(map[string]int)
	var somma float64
	totalElems := 0

	for _, rawApp := range rawApps {
		var app opencity.Application
		if json.Unmarshal(rawApp, &app) != nil || len(app.Data) == 0 {
			continue
		}

		espansione := cfg.Espansione
		if fm.Expand && espansione == "" {
			// Autodetect: cerca il path nell'array del superset già noto.
			// Se superset è vuoto, tenta comunque con ogni chiave top-level dell'app.
			var dm map[string]any
			if json.Unmarshal(app.Data, &dm) == nil {
				for k, v := range dm {
					if _, isArr := v.([]any); isArr {
						espansione = k
						break
					}
				}
			}
		}
		if fm.Expand && espansione != "" {
			elems, err := extractor.ArrayElements(app.Data, espansione)
			if err != nil {
				continue
			}
			for _, elem := range elems {
				totalElems++
				if isNumerico {
					v, err := extractor.Float(elem, fm.Path)
					if err != nil {
						continue
					}
					somma += v
					valCount[strconv.FormatFloat(v, 'f', -1, 64)]++
				} else {
					v, err := extractor.Str(elem, fm.Path)
					if err != nil || v == "" {
						continue
					}
					valCount[v]++
				}
			}
		} else {
			totalElems++
			if strings.HasPrefix(fm.Path, "$app:") {
				continue
			}
			if isNumerico {
				v, err := extractor.Float(app.Data, fm.Path)
				if err != nil {
					continue
				}
				somma += v
				valCount[strconv.FormatFloat(v, 'f', -1, 64)]++
			} else {
				v, err := extractor.Str(app.Data, fm.Path)
				if err != nil || v == "" {
					continue
				}
				valCount[v]++
			}
		}
	}

	type kv struct {
		Val   string
		Count int
	}
	kvs := make([]kv, 0, len(valCount))
	for v, c := range valCount {
		kvs = append(kvs, kv{v, c})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].Count != kvs[j].Count {
			return kvs[i].Count > kvs[j].Count
		}
		return kvs[i].Val < kvs[j].Val
	})

	valori := make([]string, len(kvs))
	conteggi := make(map[string]int, len(kvs))
	for i, kv := range kvs {
		valori[i] = kv.Val
		conteggi[kv.Val] = kv.Count
	}

	resp := map[string]any{
		"valori":          valori,
		"conteggi":        conteggi,
		"totale_istanze":  len(rawApps),
		"totale_elementi": totalElems,
		"tipo_campo":      fm.Tipo,
	}
	if isNumerico {
		resp["somma"] = somma
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- API helper wizard step 3 ---

// GetValoriCampo recupera i valori unici di un campo all'interno di un array,
// aggregati su tutte le istanze del servizio. Usato dal wizard step 3 per popolare
// il datalist del builder array.
// GET /bandi/{id}/api/valori-campo?array=anni&field=tiporichiesta
func (h *BandiHandler) GetValoriCampo(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bando, _, err := loadBandoConConfig(h, r)
	if err != nil {
		http.Error(w, `{"errore":"motore non trovato"}`, http.StatusNotFound)
		return
	}

	arrayPath := strings.TrimSpace(r.URL.Query().Get("array"))
	field := strings.TrimSpace(r.URL.Query().Get("field"))
	if arrayPath == "" || field == "" {
		http.Error(w, `{"errore":"parametri array e field obbligatori"}`, http.StatusBadRequest)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"errore": err.Error()})
		return
	}

	// Conta occorrenze di ogni valore unico
	valCount := make(map[string]int)
	for _, rawApp := range rawApps {
		var app opencity.Application
		if json.Unmarshal(rawApp, &app) != nil || len(app.Data) == 0 {
			continue
		}
		elems, err := extractor.ArrayElements(app.Data, arrayPath)
		if err != nil {
			continue
		}
		seen := make(map[string]struct{})
		for _, elem := range elems {
			v, err := extractor.Str(elem, field)
			if err != nil || v == "" {
				continue
			}
			if _, already := seen[v]; !already {
				valCount[v]++
				seen[v] = struct{}{}
			}
		}
	}

	// Ordina per frequenza decrescente poi alfabetico
	type kv struct {
		Val   string
		Count int
	}
	kvs := make([]kv, 0, len(valCount))
	for v, c := range valCount {
		kvs = append(kvs, kv{v, c})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].Count != kvs[j].Count {
			return kvs[i].Count > kvs[j].Count
		}
		return kvs[i].Val < kvs[j].Val
	})

	const soglia = 20
	tipo := "select"
	if len(kvs) > soglia {
		tipo = "libero"
		kvs = kvs[:soglia]
	}

	valori := make([]string, len(kvs))
	for i, kv := range kvs {
		valori[i] = kv.Val
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"valori":         valori,
		"totale_istanze": len(rawApps),
		"tipo":           tipo,
	})
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
