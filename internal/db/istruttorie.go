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
	Nota         string // nota di approvazione/esclusione (per-bando)
	Operatore    string
	AggiornatoIl time.Time
	AppStatus    string
	Dati         map[string]string // override campi mancanti (cross-bando, da istruttorie_dati)
	NotaLavoro   string            // nota di lavoro operatore (cross-bando, da istruttorie_dati)
}

// UpsertIstruttoria inserisce o aggiorna il record di istruttoria per una pratica.
// Se la pratica è già stata smarcata (approvata/esclusa), preserva lo stato esistente ma aggiorna motivi e app_status.
func UpsertIstruttoria(db *sql.DB, bandoID int, praticaID string, motivi []string, appStatus string) error {
	mjson, _ := json.Marshal(motivi)
	_, err := db.Exec(`
		INSERT INTO istruttorie (bando_id, pratica_id, motivi_json, stato, app_status, aggiornato_il)
		VALUES (?, ?, ?, 'da_verificare', ?, ?)
		ON CONFLICT(bando_id, pratica_id) DO UPDATE SET
			motivi_json   = excluded.motivi_json,
			app_status    = excluded.app_status,
			aggiornato_il = excluded.aggiornato_il
		WHERE stato = 'da_verificare'`,
		bandoID, praticaID, string(mjson), appStatus, time.Now().Format(time.RFC3339),
	)
	return err
}

// UpdateMotiviIstruttoria aggiorna motivi_json indipendentemente dallo stato (usato dopo salvataggio dato locale).
func UpdateMotiviIstruttoria(db *sql.DB, bandoID int, praticaID string, motivi []string) error {
	mjson, _ := json.Marshal(motivi)
	_, err := db.Exec(
		`UPDATE istruttorie SET motivi_json=?, aggiornato_il=? WHERE bando_id=? AND pratica_id=?`,
		string(mjson), time.Now().Format(time.RFC3339), bandoID, praticaID,
	)
	return err
}

// SaveDatoIstruttoria aggiunge/aggiorna un campo in istruttorie_dati (cross-bando).
// Valore vuoto rimuove il campo dal dizionario.
func SaveDatoIstruttoria(db *sql.DB, bandoID int, praticaID, campo, valore string) error {
	var datiJSON string
	err := db.QueryRow(
		`SELECT COALESCE(dati_json, '{}') FROM istruttorie_dati WHERE pratica_id=?`,
		praticaID,
	).Scan(&datiJSON)
	if err != nil {
		datiJSON = "{}"
	}
	dati := map[string]string{}
	json.Unmarshal([]byte(datiJSON), &dati)
	if valore == "" {
		delete(dati, campo)
	} else {
		dati[campo] = valore
	}
	b, _ := json.Marshal(dati)
	_, err = db.Exec(`
		INSERT INTO istruttorie_dati (pratica_id, dati_json, aggiornato_il)
		VALUES (?, ?, ?)
		ON CONFLICT(pratica_id) DO UPDATE SET dati_json=excluded.dati_json, aggiornato_il=excluded.aggiornato_il`,
		praticaID, string(b), time.Now().Format(time.RFC3339),
	)
	return err
}

// GetIstruttorieDati restituisce map[praticaID]map[string]string con i dati locali salvati (cross-bando).
// Usato dal calcolo graduatoria per applicare override ai record estratti.
func GetIstruttorieDati(db *sql.DB, bandoID int) (map[string]map[string]string, error) {
	rows, err := db.Query(`
		SELECT id.pratica_id, id.dati_json
		FROM istruttorie_dati id
		JOIN istruttorie i ON i.pratica_id = id.pratica_id AND i.bando_id = ?
		WHERE id.dati_json NOT IN ('{}', '')`,
		bandoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var pid, dj string
		rows.Scan(&pid, &dj)
		var dati map[string]string
		if json.Unmarshal([]byte(dj), &dati) == nil && len(dati) > 0 {
			out[pid] = dati
		}
	}
	return out, rows.Err()
}

// GetIstruttoriaByPratica restituisce il record di istruttoria per una pratica specifica.
func GetIstruttoriaByPratica(db *sql.DB, bandoID int, praticaID string) (*Istruttoria, error) {
	var ist Istruttoria
	var mj, dj, agAt string
	err := db.QueryRow(`
		SELECT i.id, i.bando_id, i.pratica_id, i.motivi_json, i.stato,
		       COALESCE(i.nota,''), COALESCE(i.operatore,''), COALESCE(i.aggiornato_il,''),
		       COALESCE(i.app_status,''), COALESCE(id.dati_json,'{}'), COALESCE(id.nota,'')
		FROM istruttorie i
		LEFT JOIN istruttorie_dati id ON id.pratica_id = i.pratica_id
		WHERE i.bando_id=? AND i.pratica_id=?`,
		bandoID, praticaID,
	).Scan(&ist.ID, &ist.BandoID, &ist.PraticaID, &mj, &ist.Stato,
		&ist.Nota, &ist.Operatore, &agAt, &ist.AppStatus, &dj, &ist.NotaLavoro)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(mj), &ist.Motivi)
	ist.Dati = map[string]string{}
	json.Unmarshal([]byte(dj), &ist.Dati)
	if agAt != "" {
		ist.AggiornatoIl, _ = time.Parse(time.RFC3339, agAt)
	}
	return &ist, nil
}

// ListIstruttorie restituisce le istruttorie per un bando, opzionalmente filtrate per stato e/o app_status.
func ListIstruttorie(db *sql.DB, bandoID int, statoFilter, appStatusFilter string) ([]Istruttoria, error) {
	q := `SELECT i.id, i.bando_id, i.pratica_id, i.motivi_json, i.stato,
	             COALESCE(i.nota,''), COALESCE(i.operatore,''), COALESCE(i.aggiornato_il,''),
	             COALESCE(i.app_status,''), COALESCE(id.dati_json,'{}'), COALESCE(id.nota,'')
	      FROM istruttorie i
	      LEFT JOIN istruttorie_dati id ON id.pratica_id = i.pratica_id
	      WHERE i.bando_id = ?`
	args := []any{bandoID}
	if statoFilter != "" {
		q += " AND i.stato = ?"
		args = append(args, statoFilter)
	}
	if appStatusFilter != "" {
		q += " AND i.app_status = ?"
		args = append(args, appStatusFilter)
	}
	q += " ORDER BY i.stato DESC, i.id ASC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Istruttoria
	for rows.Next() {
		var ist Istruttoria
		var mj, dj, agAt string
		if err := rows.Scan(&ist.ID, &ist.BandoID, &ist.PraticaID, &mj, &ist.Stato,
			&ist.Nota, &ist.Operatore, &agAt, &ist.AppStatus, &dj, &ist.NotaLavoro); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(mj), &ist.Motivi)
		ist.Dati = map[string]string{}
		json.Unmarshal([]byte(dj), &ist.Dati)
		if agAt != "" {
			ist.AggiornatoIl, _ = time.Parse(time.RFC3339, agAt)
		}
		out = append(out, ist)
	}
	return out, rows.Err()
}

// ListStatiApp restituisce i valori distinti di app_status per un bando (usati come filtro scan).
func ListStatiApp(db *sql.DB, bandoID int) ([]string, error) {
	rows, err := db.Query(
		`SELECT DISTINCT app_status FROM istruttorie WHERE bando_id=? AND app_status != '' ORDER BY app_status`,
		bandoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ResetDaVerificare cancella tutti i record "da_verificare" per un bando.
// Usato da PostScansiona per fare una scansione pulita dopo modifiche alla config.
func ResetDaVerificare(db *sql.DB, bandoID int) error {
	_, err := db.Exec(`DELETE FROM istruttorie WHERE bando_id=? AND stato='da_verificare'`, bandoID)
	return err
}

// CountPending restituisce il numero di istruttorie in stato "da_verificare" per un bando.
func CountPending(db *sql.DB, bandoID int) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM istruttorie WHERE bando_id=? AND stato='da_verificare'`, bandoID).Scan(&n)
	return n, err
}

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

// SaveNota salva la nota di lavoro cross-bando in istruttorie_dati.
func SaveNota(db *sql.DB, bandoID int, praticaID, nota string) error {
	_, err := db.Exec(`
		INSERT INTO istruttorie_dati (pratica_id, nota, aggiornato_il)
		VALUES (?, ?, ?)
		ON CONFLICT(pratica_id) DO UPDATE SET nota=excluded.nota, aggiornato_il=excluded.aggiornato_il`,
		praticaID, nota, time.Now().Format(time.RFC3339),
	)
	return err
}

func SetStato(db *sql.DB, id int, stato, nota, operatore string) error {
	_, err := db.Exec(`UPDATE istruttorie SET stato=?, nota=?, operatore=?, aggiornato_il=? WHERE id=?`,
		stato, nota, operatore, time.Now().Format(time.RFC3339), id)
	return err
}

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

// HasDatiOverride controlla se almeno una delle istruttorie (per ID) ha override in istruttorie_dati.
// Usato per decidere se serve auto-rescan dopo batch-approve.
func HasDatiOverride(db *sql.DB, ids []int) bool {
	if len(ids) == 0 {
		return false
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var n int
	db.QueryRow(`
		SELECT COUNT(*) FROM istruttorie_dati id2
		JOIN istruttorie i ON i.pratica_id = id2.pratica_id
		WHERE i.id IN (`+ph+`) AND id2.dati_json NOT IN ('{}', '')`,
		args...).Scan(&n)
	return n > 0
}

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
