package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type Istruttoria struct {
	ID           int
	BandoID      int
	PraticaID    string
	Motivi       []string
	Stato        string // "da_verificare" | "approvata" | "esclusa"
	Nota         string
	Operatore    string
	AggiornatoIl time.Time
}

// UpsertIstruttoria inserisce o aggiorna il record di istruttoria per una pratica.
// Se la pratica è già stata smarcata (approvata/esclusa), preserva lo stato esistente.
func UpsertIstruttoria(db *sql.DB, bandoID int, praticaID string, motivi []string) error {
	mjson, _ := json.Marshal(motivi)
	_, err := db.Exec(`
		INSERT INTO istruttorie (bando_id, pratica_id, motivi_json, stato, aggiornato_il)
		VALUES (?, ?, ?, 'da_verificare', ?)
		ON CONFLICT(bando_id, pratica_id) DO UPDATE SET
			motivi_json   = excluded.motivi_json,
			aggiornato_il = excluded.aggiornato_il
		WHERE stato = 'da_verificare'`,
		bandoID, praticaID, string(mjson), time.Now().Format(time.RFC3339),
	)
	return err
}

// ListIstruttorie restituisce le istruttorie per un bando, opzionalmente filtrate per stato.
// statoFilter = "" → tutte.
func ListIstruttorie(db *sql.DB, bandoID int, statoFilter string) ([]Istruttoria, error) {
	q := `SELECT id, bando_id, pratica_id, motivi_json, stato, COALESCE(nota,''), COALESCE(operatore,''), COALESCE(aggiornato_il,'')
	      FROM istruttorie WHERE bando_id = ?`
	args := []any{bandoID}
	if statoFilter != "" {
		q += " AND stato = ?"
		args = append(args, statoFilter)
	}
	q += " ORDER BY stato DESC, id ASC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Istruttoria
	for rows.Next() {
		var ist Istruttoria
		var mj, agAt string
		if err := rows.Scan(&ist.ID, &ist.BandoID, &ist.PraticaID, &mj, &ist.Stato, &ist.Nota, &ist.Operatore, &agAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(mj), &ist.Motivi)
		if agAt != "" {
			ist.AggiornatoIl, _ = time.Parse(time.RFC3339, agAt)
		}
		out = append(out, ist)
	}
	return out, rows.Err()
}

// CountPending restituisce il numero di istruttorie in stato "da_verificare" per un bando.
func CountPending(db *sql.DB, bandoID int) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM istruttorie WHERE bando_id=? AND stato='da_verificare'`, bandoID).Scan(&n)
	return n, err
}

// CountIstruttorie restituisce conteggi per stato (da_verificare, approvata, esclusa).
type IstruttoriaStats struct {
	DaVerificare int
	Approvate    int
	Escluse      int
}

func GetIstruttoriaStats(db *sql.DB, bandoID int) (IstruttoriaStats, error) {
	rows, err := db.Query(`SELECT stato, COUNT(*) FROM istruttorie WHERE bando_id=? GROUP BY stato`, bandoID)
	if err != nil {
		return IstruttoriaStats{}, err
	}
	defer rows.Close()
	var s IstruttoriaStats
	for rows.Next() {
		var stato string
		var n int
		rows.Scan(&stato, &n)
		switch stato {
		case "da_verificare":
			s.DaVerificare = n
		case "approvata":
			s.Approvate = n
		case "esclusa":
			s.Escluse = n
		}
	}
	return s, rows.Err()
}

// SetStato aggiorna stato, nota e operatore di una singola istruttoria.
func SetStato(db *sql.DB, id int, stato, nota, operatore string) error {
	_, err := db.Exec(`UPDATE istruttorie SET stato=?, nota=?, operatore=?, aggiornato_il=? WHERE id=?`,
		stato, nota, operatore, time.Now().Format(time.RFC3339), id)
	return err
}

// BatchSetStato aggiorna stato, nota e operatore per una lista di id.
func BatchSetStato(db *sql.DB, ids []int, stato, nota, operatore string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().Format(time.RFC3339)
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := []any{stato, nota, operatore, now}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := db.Exec(`UPDATE istruttorie SET stato=?, nota=?, operatore=?, aggiornato_il=? WHERE id IN (`+ph+`)`, args...)
	return err
}

// ListEscluse restituisce i pratica_id delle domande escluse in istruttoria.
func ListEscluse(db *sql.DB, bandoID int) ([]string, error) {
	rows, err := db.Query(`SELECT pratica_id FROM istruttorie WHERE bando_id=? AND stato='esclusa'`, bandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out = append(out, id)
	}
	return out, rows.Err()
}
