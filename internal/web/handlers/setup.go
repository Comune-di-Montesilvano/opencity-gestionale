package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type SetupHandler struct {
	DB      *sql.DB
	BaseURL string
}

// GetSetup — step 1: mostra wizard (solo se DB vuoto o admin)
func (h *SetupHandler) GetSetup(w http.ResponseWriter, r *http.Request) {
	count, _ := db.CountBandi(h.DB)
	op := middleware.FromContext(r.Context())
	renderTemplate(w, "setup.html", map[string]any{
		"Op":      op,
		"Virgine": count == 0,
	})
}

// PostSetupStep1 — testa connessione OpenCity, restituisce lista servizi via HTMX
func (h *SetupHandler) PostSetupStep1(w http.ResponseWriter, r *http.Request) {
	op := middleware.FromContext(r.Context())
	client := opencity.NewClient(h.BaseURL, op.JWT)
	raw, err := client.FetchServices()
	if err != nil {
		http.Error(w, "Errore connessione OpenCity: "+err.Error(), http.StatusBadGateway)
		return
	}

	type servizio struct {
		ID   string `json:"id"`
		Nome string `json:"name"`
	}
	var servizi []servizio
	for _, r := range raw {
		var s servizio
		json.Unmarshal(r, &s)
		if s.ID != "" {
			servizi = append(servizi, s)
		}
	}
	renderTemplate(w, "setup_step1_result.html", map[string]any{
		"Servizi": servizi,
		"BaseURL": h.BaseURL,
	})
}

// PostSetupStep2 — salva bandi configurati dal wizard
func (h *SetupHandler) PostSetupStep2(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form non valido", http.StatusBadRequest)
		return
	}

	serviceIDs := r.Form["service_id"]
	nomi := r.Form["nome"]
	budgets := r.Form["budget_totale"]
	isees := r.Form["isee_massimo"]
	scadenze := r.Form["scadenza"]
	engines := r.Form["engine_type"]

	var creati int
	for i, svcID := range serviceIDs {
		if svcID == "" {
			continue
		}
		nome := safeGet(nomi, i)
		budget := parseFloat(safeGet(budgets, i))
		isee := parseFloat(safeGet(isees, i))
		scad := safeGet(scadenze, i)
		engine := safeGet(engines, i)
		if engine == "" {
			engine = "mense_rette"
		}

		b := &db.Bando{
			ServiceID:             svcID,
			Nome:                  nome,
			BudgetTotale:          budget,
			ISEEMassimo:           isee,
			ScadenzaPresentazione: scad,
			EngineType:            engine,
			EngineConfig:          "{}",
			Attivo:                true,
			CreatedAt:             time.Now(),
		}
		db.InsertBando(h.DB, b)
		creati++
	}

	renderTemplate(w, "setup_step2_result.html", map[string]any{
		"Creati": creati,
	})
}

func safeGet(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
