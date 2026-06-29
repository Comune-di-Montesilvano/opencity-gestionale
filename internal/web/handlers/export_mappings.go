package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/extractor"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type ExportMappingsHandler struct {
	DB      *sql.DB
	BaseURL string
}

// GetExportMappings — lista template export per un bando.
func (h *ExportMappingsHandler) GetExportMappings(w http.ResponseWriter, r *http.Request) {
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

	mappings, _ := db.ListExportMappings(h.DB, bandoID)
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "export_mappings.html", map[string]any{
		"Op":        op,
		"Bando":     bando,
		"Mappings":  mappings,
		"Flash":     flash,
		"FlashType": flashType,
	})
}

// PostCreateMapping — crea nuovo template export con un preset opzionale.
func (h *ExportMappingsHandler) PostCreateMapping(w http.ResponseWriter, r *http.Request) {
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

	r.ParseForm()
	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		http.Redirect(w, r, fmt.Sprintf("/bandi/%d/export-mappings?flash=Nome+obbligatorio&flashType=error", bandoID), http.StatusSeeOther)
		return
	}

	preset := r.FormValue("preset")
	var colonne []db.ExportColonna
	switch preset {
	case "ragioneria":
		colonne = db.ColonneDefaultRagioneria
	case "generico":
		colonne = db.ColonneDefaultGenerico
	default:
		colonne = []db.ExportColonna{}
	}

	m := &db.ExportMapping{
		BandoID: bandoID,
		Nome:    nome,
		Colonne: colonne,
	}
	newID, err := db.InsertExportMapping(h.DB, m)
	if err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/export-mappings/%d?flash=Template+creato&flashType=success", bandoID, newID), http.StatusSeeOther)
}

// GetEditMapping — pagina di editing colonne per un template.
func (h *ExportMappingsHandler) GetEditMapping(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	mapID, _ := strconv.ParseInt(r.PathValue("mapID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	m, err := db.GetExportMapping(h.DB, mapID)
	if err != nil || m == nil || m.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}

	var engCfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &engCfg)
	mappingKeys := make([]string, 0, len(engCfg.Mapping))
	for k := range engCfg.Mapping {
		mappingKeys = append(mappingKeys, k)
	}
	sort.Strings(mappingKeys)

	// Codici stato disponibili dal superset per il filtro
	var supersetData map[string]map[string][]string
	json.Unmarshal([]byte(bando.ValoriSuperset), &supersetData)
	statusNames := map[string]string{}
	if statusNamesRaw, ok := supersetData["$status_names"]; ok {
		for code, names := range statusNamesRaw {
			if len(names) > 0 {
				statusNames[code] = names[0]
			}
		}
	}

	colonneJSON, _ := json.Marshal(m.Colonne)
	flash, flashType := flashFromRequest(r)
	renderTemplate(w, "export_mapping_edit.html", map[string]any{
		"Op":          op,
		"Bando":       bando,
		"Mapping":     m,
		"MappingKeys": mappingKeys,
		"SistemaKeys": db.SistemaChiavi,
		"SupersetData": bando.ValoriSuperset,
		"StatusNames": statusNames,
		"ColonneJSON": string(colonneJSON),
		"Flash":       flash,
		"FlashType":   flashType,
	})
}

// PostSaveMapping — salva colonne e filtro stati di un template.
func (h *ExportMappingsHandler) PostSaveMapping(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	mapID, _ := strconv.ParseInt(r.PathValue("mapID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	m, err := db.GetExportMapping(h.DB, mapID)
	if err != nil || m == nil || m.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		nome = m.Nome
	}
	filtroStati := r.Form["filtro_stati"]

	var colonne []db.ExportColonna
	colonneJSON := r.FormValue("colonne_json")
	if colonneJSON != "" {
		if err := json.Unmarshal([]byte(colonneJSON), &colonne); err != nil {
			http.Redirect(w, r, fmt.Sprintf("/bandi/%d/export-mappings/%d?flash=Colonne+non+valide&flashType=error", bandoID, mapID), http.StatusSeeOther)
			return
		}
	}

	m.Nome = nome
	m.FiltroStati = filtroStati
	m.Colonne = colonne
	if err := db.UpdateExportMapping(h.DB, m); err != nil {
		http.Error(w, "Errore DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/export-mappings/%d?flash=Template+salvato&flashType=success", bandoID, mapID), http.StatusSeeOther)
}

// PostDeleteMapping — elimina un template.
func (h *ExportMappingsHandler) PostDeleteMapping(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	mapID, _ := strconv.ParseInt(r.PathValue("mapID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	m, err := db.GetExportMapping(h.DB, mapID)
	if err != nil || m == nil || m.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}

	db.DeleteExportMapping(h.DB, mapID)
	http.Redirect(w, r, fmt.Sprintf("/bandi/%d/export-mappings?flash=Template+eliminato&flashType=success", bandoID), http.StatusSeeOther)
}

// GetExportCSVMapped — download CSV applicando un template export salvato.
func (h *ExportMappingsHandler) GetExportCSVMapped(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	mapID, _ := strconv.ParseInt(r.PathValue("mapID"), 10, 64)

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

	mapping, err := db.GetExportMapping(h.DB, mapID)
	if err != nil || mapping == nil || mapping.BandoID != bandoID {
		http.NotFound(w, r)
		return
	}

	var grad graduatoria.Graduatoria
	json.Unmarshal([]byte(run.DatiJSON), &grad)

	var ecfg graduatoria.EngineConfig
	json.Unmarshal([]byte(bando.EngineConfig), &ecfg)

	// Re-fetch da OpenCity se ci sono colonne "raw".
	rawAppData := map[string]json.RawMessage{} // appID → app.Data
	for _, col := range mapping.Colonne {
		if col.Sorgente == "raw" && col.Path != "" {
			client := opencity.NewClient(h.BaseURL, op.JWT)
			rawApps, err := client.FetchAllApplications(bando.ServiceID, nil)
			if err != nil {
				http.Error(w, "Errore fetch OpenCity: "+err.Error(), http.StatusBadGateway)
				return
			}
			for _, raw := range rawApps {
				var a opencity.Application
				if json.Unmarshal(raw, &a) == nil {
					rawAppData[a.ID] = a.Data
				}
			}
			break // un solo fetch basta per tutte le colonne raw
		}
	}

	// Filtro stati (vuoto = tutti).
	filtroSet := map[string]bool{}
	for _, s := range mapping.FiltroStati {
		filtroSet[s] = true
	}

	filename := fmt.Sprintf("run%d_%s.csv", runID, sanitizeFilename(mapping.Nome))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	cw.Comma = ';'

	// Header CSV
	headers := make([]string, len(mapping.Colonne))
	for i, col := range mapping.Colonne {
		headers[i] = col.Label
	}
	cw.Write(headers)

	pos := 0
	for _, g := range grad.Gruppi {
		for _, riga := range g.Righe {
			if riga.Istanza == nil {
				continue
			}
			ist := riga.Istanza
			// Applica filtro stato se configurato
			if len(filtroSet) > 0 && !filtroSet[ist.Status] {
				continue
			}
			pos++
			row := make([]string, len(mapping.Colonne))
			for i, col := range mapping.Colonne {
				row[i] = estraiColonnaExport(col, ecfg, pos, g.Nome, riga, ist, rawAppData)
			}
			cw.Write(row)
		}
	}
	cw.Flush()
}

// estraiColonnaExport estrae il valore di una colonna in base alla sorgente.
func estraiColonnaExport(col db.ExportColonna, ecfg graduatoria.EngineConfig, pos int, gruppo string, riga graduatoria.RigaGraduatoria, ist *graduatoria.Istanza, rawAppData map[string]json.RawMessage) string {
	switch col.Sorgente {
	case "sistema":
		switch col.Chiave {
		case "posizione":
			return fmt.Sprintf("%d", pos)
		case "protocollo":
			return ist.ProtocolNumber
		case "cognome":
			return ist.RichiedenteCognome
		case "nome":
			return ist.RichiedenteNome
		case "cf_richiedente":
			return ist.RichiedenteCF
		case "tipologia":
			return gruppo
		case "importo":
			if !riga.Ammessa {
				netto := ist.Corrispettivo - ist.BeneficioRicevuto
				if ecfg.Rimborso.Tipo == "lordo" && ecfg.Rimborso.CampoLordo != "" {
					if valStr, ok := ist.CampiMappati[ecfg.Rimborso.CampoLordo]; ok {
						if f, err := strconv.ParseFloat(strings.ReplaceAll(valStr, ",", "."), 64); err == nil {
							return fmt.Sprintf("%.2f", f)
						}
					}
					return fmt.Sprintf("%.2f", ist.Corrispettivo)
				}
				if netto < 0 {
					netto = 0
				}
				return fmt.Sprintf("%.2f", netto)
			}
			return fmt.Sprintf("%.2f", riga.ImportoRimborso)
		case "annualita":
			if ist.Annualita != 0 {
				return fmt.Sprintf("%d", ist.Annualita)
			}
			if v := ist.CampiMappati["annualita"]; v != "" {
				return v
			}
			return ist.CampiMappati["annualita1"]
		case "stato_app":
			return ist.Status
		}
	case "mappato":
		return ist.CampiMappati[col.Chiave]
	case "raw":
		if col.Path == "" {
			return ""
		}
		if data, ok := rawAppData[ist.ID]; ok {
			v, _ := extractor.Str(data, col.Path)
			return v
		}
	}
	return ""
}

func sanitizeFilename(s string) string {
	r := strings.NewReplacer(" ", "_", "/", "-", "\\", "-", ":", "-")
	return r.Replace(s)
}
