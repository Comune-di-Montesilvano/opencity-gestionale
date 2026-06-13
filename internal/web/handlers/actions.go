package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/middleware"
)

type ActionsHandler struct {
	DB      *sql.DB
	BaseURL string
}

type batchRequest struct {
	PraticaIDs []string `json:"pratica_ids"`
	Messaggio  string   `json:"messaggio"`
}

type batchResult struct {
	OK     []string          `json:"ok"`
	Errori map[string]string `json:"errori"`
}

func (h *ActionsHandler) PostApprovaBatch(w http.ResponseWriter, r *http.Request) {
	h.postBatch(w, r, "approva")
}

func (h *ActionsHandler) PostRifiutaBatch(w http.ResponseWriter, r *http.Request) {
	h.postBatch(w, r, "rifiuta")
}

func (h *ActionsHandler) postBatch(w http.ResponseWriter, r *http.Request, azione string) {
	op := middleware.FromContext(r.Context())
	bandoID := bandoIDFromPath(r)
	runID, _ := strconv.ParseInt(r.PathValue("runID"), 10, 64)

	bando, err := db.GetBando(h.DB, bandoID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !op.IsAdmin() && !op.CanAccessService(bando.ServiceID) {
		http.Error(w, "Accesso negato", http.StatusForbidden)
		return
	}

	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Body non valido", http.StatusBadRequest)
		return
	}
	if len(req.PraticaIDs) == 0 {
		http.Error(w, "Nessuna pratica selezionata", http.StatusBadRequest)
		return
	}

	client := opencity.NewClient(h.BaseURL, op.JWT)
	result := batchResult{Errori: map[string]string{}}

	for _, praticaID := range req.PraticaIDs {
		var actionErr error
		if azione == "approva" {
			actionErr = client.Approve(praticaID, req.Messaggio)
		} else {
			actionErr = client.Reject(praticaID, req.Messaggio)
		}

		esito := "ok"
		var errDet string
		if actionErr != nil {
			esito = "errore"
			errDet = actionErr.Error()
			result.Errori[praticaID] = errDet
		} else {
			result.OK = append(result.OK, praticaID)
		}

		db.InsertAudit(h.DB, &db.AuditAction{
			Operatore:       op.Username,
			Azione:          azione,
			PraticaID:       praticaID,
			BandoID:         bandoID,
			RunID:           runID,
			Messaggio:       req.Messaggio,
			Esito:           esito,
			ErroreDettaglio: errDet,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
