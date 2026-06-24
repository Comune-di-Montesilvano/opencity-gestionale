package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"opencity-gestionale/internal/db"
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
