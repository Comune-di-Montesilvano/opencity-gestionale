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

func AddCollegamento(db *sql.DB, bandoIDA int, praticaIDA string, bandoIDB int, praticaIDB string) (int64, error) {
	if bandoIDA > bandoIDB || (bandoIDA == bandoIDB && praticaIDA > praticaIDB) {
		bandoIDA, bandoIDB = bandoIDB, bandoIDA
		praticaIDA, praticaIDB = praticaIDB, praticaIDA
	}
	res, err := db.Exec(
		`INSERT OR IGNORE INTO pratiche_collegate (bando_id_a, pratica_id_a, bando_id_b, pratica_id_b, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		bandoIDA, praticaIDA, bandoIDB, praticaIDB, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		db.QueryRow(`SELECT id FROM pratiche_collegate WHERE bando_id_a=? AND pratica_id_a=? AND bando_id_b=? AND pratica_id_b=?`,
			bandoIDA, praticaIDA, bandoIDB, praticaIDB).Scan(&id)
	}
	return id, nil
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

	// 4 arm: bando_a=current, bando_b=current, e stessa pratica_id in bando diverso (duplicati automatici)
	args := []any{bandoID}
	for _, id := range praticaIDs {
		args = append(args, id)
	}
	args = append(args, bandoID)
	for _, id := range praticaIDs {
		args = append(args, id)
	}
	args = append(args, bandoID)
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
			UNION ALL
			SELECT pc.id, pc.pratica_id_a AS local_pid, pc.pratica_id_b AS other_pid, pc.bando_id_b AS other_bid
			FROM pratiche_collegate pc
			WHERE pc.bando_id_a != ? AND pc.pratica_id_a IN (` + ph + `)
			UNION ALL
			SELECT pc.id, pc.pratica_id_b AS local_pid, pc.pratica_id_a AS other_pid, pc.bando_id_a AS other_bid
			FROM pratiche_collegate pc
			WHERE pc.bando_id_b != ? AND pc.pratica_id_b IN (` + ph + `)
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
// che hanno almeno una pratica collegabile (stesso CF, non già linkata, dedup key matchante)
// in un altro bando scansionato. dedupCampi: campi non-expand della chiave di deduplicazione.
func GetPraticheCollegabili(db *sql.DB, bandoID int, dedupCampi []string) (map[string]bool, error) {
	dedupCond := ""
	for _, campo := range dedupCampi {
		safe := strings.ReplaceAll(campo, "'", "")
		dedupCond += " AND (COALESCE(json_extract(c1.dati_json,'$." + safe + "'),'')=''" +
			" OR json_extract(c2.dati_json,'$." + safe + "')=json_extract(c1.dati_json,'$." + safe + "'))"
	}
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
			  AND NOT EXISTS (
				SELECT 1 FROM pratiche_collegate pc
				WHERE (pc.bando_id_a = c1.bando_id AND pc.pratica_id_a = c1.pratica_id
				       AND pc.bando_id_b = c2.bando_id AND pc.pratica_id_b = c2.pratica_id)
				   OR (pc.bando_id_b = c1.bando_id AND pc.pratica_id_b = c1.pratica_id
				       AND pc.bando_id_a = c2.bando_id AND pc.pratica_id_a = c2.pratica_id)
			  )
			  AND NOT (
			    EXISTS (SELECT 1 FROM istruttorie_api_cache c3
			            WHERE c3.pratica_id = c1.pratica_id AND c3.bando_id = c2.bando_id)
			    AND EXISTS (SELECT 1 FROM istruttorie_api_cache c4
			                WHERE c4.pratica_id = c2.pratica_id AND c4.bando_id = c1.bando_id)
			  )`+dedupCond+`
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

// TrovaPraticheStessoCF cerca pratiche in altri bandi con stesso CF richiedente.
// dedupFilters: mappa campo→valore estratti dalla chiave di deduplicazione del bando corrente;
// se non vuota, filtra i candidati per corrispondenza su quei campi.
func TrovaPraticheStessoCF(db *sql.DB, bandoID int, praticaID string, dedupFilters map[string]string) ([]CollegamentoInfo, error) {
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

	// Costruisce filtri dedup dinamici e condizione NOT EXISTS per già collegati
	dedupWhere := ""
	args := []any{bandoID, praticaID, cf, cf, bandoID, praticaID, bandoID, praticaID, praticaID, bandoID}
	for campo, valore := range dedupFilters {
		if valore == "" {
			continue
		}
		dedupWhere += " AND json_extract(c.dati_json, '$." + strings.ReplaceAll(campo, "'", "") + "') = ?"
		args = append(args, valore)
	}

	q := `
		SELECT c.pratica_id, c.bando_id,
		       COALESCE(b.nome, 'Bando '||c.bando_id),
		       COALESCE(json_extract(c.dati_json, '$.protocollo'), '')
		FROM istruttorie_api_cache c
		LEFT JOIN bandi b ON b.id = c.bando_id
		WHERE c.bando_id != ? AND c.pratica_id != ?
		  AND (json_extract(c.dati_json, '$.richiedente_cf') = ?
		       OR json_extract(c.dati_json, '$.richiedente') = ?)
		  AND NOT EXISTS (
		    SELECT 1 FROM pratiche_collegate pc
		    WHERE (pc.bando_id_a=? AND pc.pratica_id_a=? AND pc.bando_id_b=c.bando_id AND pc.pratica_id_b=c.pratica_id)
		       OR (pc.bando_id_b=? AND pc.pratica_id_b=? AND pc.bando_id_a=c.bando_id AND pc.pratica_id_a=c.pratica_id)
		  )
		  AND NOT (
		    EXISTS (SELECT 1 FROM istruttorie_api_cache c3
		            WHERE c3.pratica_id = ? AND c3.bando_id = c.bando_id)
		    AND EXISTS (SELECT 1 FROM istruttorie_api_cache c4
		               WHERE c4.pratica_id = c.pratica_id AND c4.bando_id = ?)
		  )` + dedupWhere + `
		ORDER BY b.nome, json_extract(c.dati_json, '$.protocollo')`

	rows, err := db.Query(q, args...)
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
