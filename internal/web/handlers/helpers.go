package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/graduatoria"
)

func bandoIDFromPath(r *http.Request) int64 {
	v, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return v
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// istruttoriaStatoMap restituisce map[praticaID]stato per tutti i record istruttoria di un bando.
func istruttoriaStatoMap(dbConn *sql.DB, bandoID int64) map[string]string {
	rows, _ := db.ListIstruttorie(dbConn, int(bandoID), "", "")
	m := make(map[string]string, len(rows))
	for _, ist := range rows {
		m[ist.PraticaID] = ist.Stato
	}
	return m
}

// recuperaDatiCache carica in memoria i dati anagrafici estratti da istruttorie_api_cache per un bando.
func recuperaDatiCache(dbConn *sql.DB, bandoID int64) map[string]map[string]string {
	cacheData := make(map[string]map[string]string)
	rows, err := dbConn.Query("SELECT pratica_id, dati_json FROM istruttorie_api_cache WHERE bando_id=?", bandoID)
	if err != nil {
		return cacheData
	}
	defer rows.Close()
	for rows.Next() {
		var pid, djson string
		if rows.Scan(&pid, &djson) == nil {
			m := make(map[string]string)
			if json.Unmarshal([]byte(djson), &m) == nil {
				cacheData[pid] = m
			}
		}
	}
	return cacheData
}

// applicaFallbackCache popola i campi anagrafici dell'istanza se vuoti usando i dati della cache.
func applicaFallbackCache(ist *graduatoria.Istanza, cacheData map[string]map[string]string) {
	if ist == nil {
		return
	}
	m, ok := cacheData[ist.ID]
	if !ok {
		return
	}
	if ist.RichiedenteCognome == "" {
		ist.RichiedenteCognome = m["richiedente_cognome"]
	}
	if ist.RichiedenteNome == "" {
		ist.RichiedenteNome = m["richiedente_nome"]
	}
	if ist.RichiedenteCF == "" {
		ist.RichiedenteCF = m["richiedente_cf"]
	}
	if ist.RichiedenteEmail == "" {
		ist.RichiedenteEmail = m["richiedente_email"]
	}
	if ist.RichiedenteTel == "" {
		ist.RichiedenteTel = m["richiedente_tel"]
	}
	if ist.Indirizzo == "" {
		ist.Indirizzo = m["indirizzo"]
	}
	if ist.Civico == "" {
		ist.Civico = m["civico"]
	}
	if ist.Comune == "" {
		ist.Comune = m["comune"]
	}
	if ist.CAP == "" {
		ist.CAP = m["cap"]
	}
	if ist.Provincia == "" {
		ist.Provincia = m["provincia"]
	}
	if ist.ISEEValidoFino == "" {
		ist.ISEEValidoFino = m["isee_valido_fino"]
	}
	if ist.ISEEDSUProtocollo == "" {
		ist.ISEEDSUProtocollo = m["isee_dsu_protocollo"]
	}
}
