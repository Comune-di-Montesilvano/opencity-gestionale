package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type CollegamentoInfo struct {
	ID         int
	BandoID    int
	BandoNome  string
	PraticaID  string
	Protocollo string
}

func AddCollegamento(db *sql.DB, bandoIDA int, praticaIDA string, bandoIDB int, praticaIDB string) error {
	if bandoIDA > bandoIDB || (bandoIDA == bandoIDB && praticaIDA > praticaIDB) {
		bandoIDA, bandoIDB = bandoIDB, bandoIDA
		praticaIDA, praticaIDB = praticaIDB, praticaIDA
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO pratiche_collegate (bando_id_a, pratica_id_a, bando_id_b, pratica_id_b, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		bandoIDA, praticaIDA, bandoIDB, praticaIDB, time.Now().Format(time.RFC3339),
	)
	return err
}

func RemoveCollegamento(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM pratiche_collegate WHERE id=?`, id)
	return err
}

func GetCollegamenti(db *sql.DB, bandoID int, praticaIDs []string) (map[string][]CollegamentoInfo, error) {
	if len(praticaIDs) == 0 {
		return map[string][]CollegamentoInfo{}, nil
	}
	ph := strings.Repeat("?,", len(praticaIDs))
	ph = ph[:len(ph)-1]

	args := []any{bandoID}
	for _, id := range praticaIDs {
		args = append(args, id)
	}
	args = append(args, bandoID)
	for _, id := range praticaIDs {
		args = append(args, id)
	}

	q := `
		SELECT u.id, u.local_pid, u.other_pid, u.other_bid,
		       COALESCE(b.nome, 'Bando '||u.other_bid),
		       COALESCE(json_extract(c.dati_json, '$.protocollo'), '')
		FROM (
			SELECT pc.id, pc.pratica_id_a AS local_pid, pc.pratica_id_b AS other_pid, pc.bando_id_b AS other_bid
			FROM pratiche_collegate pc
			WHERE pc.bando_id_a = ? AND pc.pratica_id_a IN (` + ph + `)
			UNION ALL
			SELECT pc.id, pc.pratica_id_b, pc.pratica_id_a, pc.bando_id_a
			FROM pratiche_collegate pc
			WHERE pc.bando_id_b = ? AND pc.pratica_id_b IN (` + ph + `)
		) u
		LEFT JOIN bandi b ON b.id = u.other_bid
		LEFT JOIN istruttorie_api_cache c ON c.pratica_id = u.other_pid AND c.bando_id = u.other_bid`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]CollegamentoInfo{}
	for rows.Next() {
		var info CollegamentoInfo
		var localPid string
		if err := rows.Scan(&info.ID, &localPid, &info.PraticaID, &info.BandoID, &info.BandoNome, &info.Protocollo); err != nil {
			continue
		}
		out[localPid] = append(out[localPid], info)
	}
	return out, rows.Err()
}

// GetPraticheCollegabili restituisce il set di pratica_id del bando corrente
// che hanno almeno una pratica con stesso CF richiedente in un altro bando scansionato.
func GetPraticheCollegabili(db *sql.DB, bandoID int) (map[string]bool, error) {
	rows, err := db.Query(`
		SELECT DISTINCT c1.pratica_id
		FROM istruttorie_api_cache c1
		WHERE c1.bando_id = ?
		  AND COALESCE(json_extract(c1.dati_json, '$.richiedente_cf'), '') != ''
		  AND EXISTS (
			SELECT 1 FROM istruttorie_api_cache c2
			WHERE c2.bando_id != c1.bando_id
			  AND c2.pratica_id != c1.pratica_id
			  AND (json_extract(c2.dati_json, '$.richiedente_cf') = json_extract(c1.dati_json, '$.richiedente_cf')
			       OR json_extract(c2.dati_json, '$.richiedente') = json_extract(c1.dati_json, '$.richiedente_cf'))
		  )`, bandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var pid string
		rows.Scan(&pid)
		out[pid] = true
	}
	return out, rows.Err()
}

func TrovaPraticheStessoCF(db *sql.DB, bandoID int, praticaID string) ([]CollegamentoInfo, error) {
	var datiJSON string
	db.QueryRow(`SELECT COALESCE(dati_json,'{}') FROM istruttorie_api_cache WHERE bando_id=? AND pratica_id=?`,
		bandoID, praticaID).Scan(&datiJSON)

	var dati map[string]json.RawMessage
	json.Unmarshal([]byte(datiJSON), &dati)

	cf := ""
	for _, key := range []string{"richiedente_cf", "richiedente"} {
		if raw, ok := dati[key]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				cf = s
				break
			}
		}
	}
	if cf == "" {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT c.pratica_id, c.bando_id,
		       COALESCE(b.nome, 'Bando '||c.bando_id),
		       COALESCE(json_extract(c.dati_json, '$.protocollo'), '')
		FROM istruttorie_api_cache c
		LEFT JOIN bandi b ON b.id = c.bando_id
		WHERE c.bando_id != ? AND c.pratica_id != ?
		  AND (json_extract(c.dati_json, '$.richiedente_cf') = ?
		       OR json_extract(c.dati_json, '$.richiedente') = ?)
		ORDER BY b.nome, json_extract(c.dati_json, '$.protocollo')`,
		bandoID, praticaID, cf, cf,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CollegamentoInfo
	for rows.Next() {
		var info CollegamentoInfo
		rows.Scan(&info.PraticaID, &info.BandoID, &info.BandoNome, &info.Protocollo)
		out = append(out, info)
	}
	return out, rows.Err()
}
