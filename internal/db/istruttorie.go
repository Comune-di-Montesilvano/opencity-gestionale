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

// SaveDatoIstruttoria aggiunge/aggiorna un campo in istruttorie_dati (per-bando).
// Valore vuoto rimuove il campo dal dizionario.
func SaveDatoIstruttoria(db *sql.DB, bandoID int, praticaID, campo, valore string) error {
	return SaveDatiIstruttoria(db, bandoID, praticaID, map[string]string{campo: valore})
}

// SaveDatiIstruttoria aggiunge/aggiorna multipli campi in istruttorie_dati (per-bando) in una singola transazione.
// Valori vuoti rimuovono i rispettivi campi dal dizionario.
func SaveDatiIstruttoria(db *sql.DB, bandoID int, praticaID string, overrides map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var datiJSON string
	err = tx.QueryRow(
		`SELECT COALESCE(dati_json, '{}') FROM istruttorie_dati WHERE bando_id=? AND pratica_id=?`,
		bandoID, praticaID,
	).Scan(&datiJSON)
	if err != nil {
		datiJSON = "{}"
	}

	dati := map[string]string{}
	json.Unmarshal([]byte(datiJSON), &dati)

	for campo, valore := range overrides {
		if valore == "" {
			delete(dati, campo)
		} else {
			dati[campo] = valore
		}
	}

	b, _ := json.Marshal(dati)
	_, err = tx.Exec(`
		INSERT INTO istruttorie_dati (bando_id, pratica_id, dati_json, aggiornato_il)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bando_id, pratica_id) DO UPDATE SET dati_json=excluded.dati_json, aggiornato_il=excluded.aggiornato_il`,
		bandoID, praticaID, string(b), time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetIstruttorieDati restituisce map[praticaID]map[string]string con i dati locali salvati (per-bando).
// Usato dal calcolo graduatoria per applicare override ai record estratti.
// JOIN con istruttorie_api_cache (non istruttorie) per includere anche pratiche non flaggate
// ma presenti nella scan del bando (es. override ISEE da pagina dati_locali).
func GetIstruttorieDati(db *sql.DB, bandoID int) (map[string]map[string]string, error) {
	rows, err := db.Query(`
		SELECT id.pratica_id, id.dati_json
		FROM istruttorie_dati id
		JOIN istruttorie_api_cache c ON c.pratica_id = id.pratica_id AND c.bando_id = id.bando_id
		WHERE id.bando_id = ? AND id.dati_json NOT IN ('{}', '')`,
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
		       COALESCE(i.app_status,''), COALESCE(id.dati_json,'{}'), COALESCE(i.nota_lavoro,'')
		FROM istruttorie i
		LEFT JOIN istruttorie_dati id ON id.pratica_id = i.pratica_id AND id.bando_id = i.bando_id
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
	             COALESCE(i.app_status,''), COALESCE(id.dati_json,'{}'), COALESCE(i.nota_lavoro,'')
	      FROM istruttorie i
	      LEFT JOIN istruttorie_dati id ON id.pratica_id = i.pratica_id AND id.bando_id = i.bando_id
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

// ResetDaVerificare resetta o cancella i record "da_verificare" per un bando.
// Preserva i record che hanno note di lavoro (nota_lavoro) o dati locali (istruttorie_dati) per evitare perdite di dati.
func ResetDaVerificare(db *sql.DB, bandoID int) error {
	// 1. Resetta i motivi per le pratiche 'da_verificare' che hanno note o dati locali
	_, err := db.Exec(`
		UPDATE istruttorie 
		SET motivi_json = '[]' 
		WHERE bando_id = ? AND stato = 'da_verificare' AND (
			COALESCE(nota_lavoro, '') != '' OR EXISTS (
				SELECT 1 FROM istruttorie_dati id 
				WHERE id.pratica_id = istruttorie.pratica_id AND id.bando_id = istruttorie.bando_id AND id.dati_json NOT IN ('{}', '')
			)
		)`, bandoID)
	if err != nil {
		return err
	}

	// 2. Rimuove le pratiche 'da_verificare' che NON hanno note né dati locali né flag includi_dufficio
	_, err = db.Exec(`
		DELETE FROM istruttorie
		WHERE bando_id = ? AND stato = 'da_verificare' AND COALESCE(nota_lavoro, '') = ''
		AND includi_dufficio = 0
		AND NOT EXISTS (
			SELECT 1 FROM istruttorie_dati id
			WHERE id.pratica_id = istruttorie.pratica_id AND id.bando_id = istruttorie.bando_id AND id.dati_json NOT IN ('{}', '')
		)`, bandoID)
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

// SaveNota salva la nota di lavoro per-bando in istruttorie.nota_lavoro.
func SaveNota(db *sql.DB, bandoID int, praticaID, nota string) error {
	_, err := db.Exec(
		`UPDATE istruttorie SET nota_lavoro=?, aggiornato_il=? WHERE bando_id=? AND pratica_id=?`,
		nota, time.Now().Format(time.RFC3339), bandoID, praticaID,
	)
	return err
}

func SetStato(db *sql.DB, id int, stato, nota, operatore string) error {
	_, err := db.Exec(`UPDATE istruttorie SET stato=?, nota=?, operatore=?, aggiornato_il=? WHERE id=?`,
		stato, nota, operatore, time.Now().Format(time.RFC3339), id)
	return err
}

// UpsertStatoIstruttoria imposta lo stato per una pratica (per pratica_id).
// Crea la riga se non esiste ancora (es. pratiche "non_rientranti" senza row istruttoria).
func UpsertStatoIstruttoria(db *sql.DB, bandoID int, praticaID, stato, nota, operatore string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO istruttorie (bando_id, pratica_id, motivi_json, stato, nota, operatore, aggiornato_il)
		VALUES (?, ?, '[]', ?, ?, ?, ?)
		ON CONFLICT(bando_id, pratica_id) DO UPDATE SET
			stato=excluded.stato,
			nota=excluded.nota,
			operatore=excluded.operatore,
			aggiornato_il=excluded.aggiornato_il`,
		bandoID, praticaID, stato, nota, operatore, now)
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
		JOIN istruttorie i ON i.pratica_id = id2.pratica_id AND i.bando_id = id2.bando_id
		WHERE i.id IN (`+ph+`) AND id2.dati_json NOT IN ('{}', '')`,
		args...).Scan(&n)
	return n > 0
}

// NoteAltroBando rappresenta una nota di lavoro proveniente da un bando diverso.
type NoteAltroBando struct {
	BandoNome string
	Nota      string
}

// GetNoteAltriBandi restituisce le note salvate per le stesse pratiche in ALTRI bandi (abbinando per ID pratica).
// Recupera sia le note di lavoro (nota_lavoro) sia le motivazioni di stato (nota).
func GetNoteAltriBandi(db *sql.DB, bandoID int, praticaIDs []string) (map[string][]NoteAltroBando, error) {
	if len(praticaIDs) == 0 {
		return nil, nil
	}
	ph := strings.Repeat("?,", len(praticaIDs))
	ph = ph[:len(ph)-1]
	args := []any{bandoID}
	for _, id := range praticaIDs {
		args = append(args, id)
	}

	rows, err := db.Query(`
		SELECT i.pratica_id, COALESCE(b.nome, 'Bando '||i.bando_id), COALESCE(i.nota_lavoro, ''), COALESCE(i.nota, ''), i.stato
		FROM istruttorie i
		LEFT JOIN bandi b ON b.id = i.bando_id
		WHERE i.bando_id != ? AND i.pratica_id IN (`+ph+`) AND (COALESCE(i.nota_lavoro, '') != '' OR COALESCE(i.nota, '') != '')
		ORDER BY i.pratica_id, b.nome`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]NoteAltroBando{}
	for rows.Next() {
		var pid, bandoNome, notaLavoro, notaStato, stato string
		if err := rows.Scan(&pid, &bandoNome, &notaLavoro, &notaStato, &stato); err == nil {
			if notaLavoro != "" {
				out[pid] = append(out[pid], NoteAltroBando{
					BandoNome: bandoNome,
					Nota:      notaLavoro,
				})
			}
			if notaStato != "" {
				out[pid] = append(out[pid], NoteAltroBando{
					BandoNome: bandoNome,
					Nota:      "[" + stato + "] " + notaStato,
				})
			}
		}
	}
	return out, rows.Err()
}

func GetAltriBandiPerPratiche(db *sql.DB, bandoID int, praticaIDs []string) (map[string][]string, error) {
	if len(praticaIDs) == 0 {
		return nil, nil
	}
	ph := strings.Repeat("?,", len(praticaIDs))
	ph = ph[:len(ph)-1]
	args := []any{bandoID}
	for _, id := range praticaIDs {
		args = append(args, id)
	}

	rows, err := db.Query(`
		SELECT c.pratica_id, COALESCE(b.nome, 'Bando '||c.bando_id)
		FROM istruttorie_api_cache c
		LEFT JOIN bandi b ON b.id = c.bando_id
		WHERE c.bando_id != ? AND c.pratica_id IN (`+ph+`)
		ORDER BY c.pratica_id, b.nome`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var pid, bandoNome string
		if err := rows.Scan(&pid, &bandoNome); err == nil {
			out[pid] = append(out[pid], bandoNome)
		}
	}
	return out, rows.Err()
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

func ListInclusiDufficio(db *sql.DB, bandoID int) ([]string, error) {
	rows, err := db.Query(`SELECT pratica_id FROM istruttorie WHERE bando_id=? AND includi_dufficio=1`, bandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func SetIncludiDufficio(db *sql.DB, bandoID int, praticaID string, includi bool) error {
	val := 0
	if includi {
		val = 1
	}
	_, err := db.Exec(`
		INSERT INTO istruttorie (bando_id, pratica_id, motivi_json, stato, includi_dufficio)
		VALUES (?, ?, '[]', 'da_verificare', ?)
		ON CONFLICT(bando_id, pratica_id) DO UPDATE SET includi_dufficio=excluded.includi_dufficio`,
		bandoID, praticaID, val,
	)
	return err
}

// UpsertAPICache salva il JSON blob dei campi estratti dall'API per una pratica/bando.
// Chiamato durante la scan per tutte le app che passano filtri_istanza.
func UpsertAPICache(db *sql.DB, bandoID int, praticaID, datiJSON string) error {
	_, err := db.Exec(`
		INSERT INTO istruttorie_api_cache (pratica_id, bando_id, dati_json, aggiornato_il)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pratica_id, bando_id) DO UPDATE SET
			dati_json=excluded.dati_json, aggiornato_il=excluded.aggiornato_il`,
		praticaID, bandoID, datiJSON, time.Now().Format(time.RFC3339),
	)
	return err
}

// UpsertAPICacheField aggiorna un singolo campo nella cache API (usato in PostSaveDato).
func UpsertAPICacheField(db *sql.DB, bandoID int, praticaID, campo, valore string) error {
	var datiJSON string
	err := db.QueryRow(
		`SELECT COALESCE(dati_json, '{}') FROM istruttorie_api_cache WHERE pratica_id=? AND bando_id=?`,
		praticaID, bandoID,
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
		INSERT INTO istruttorie_api_cache (pratica_id, bando_id, dati_json, aggiornato_il)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pratica_id, bando_id) DO UPDATE SET
			dati_json=excluded.dati_json, aggiornato_il=excluded.aggiornato_il`,
		praticaID, bandoID, string(b), time.Now().Format(time.RFC3339),
	)
	return err
}

// GetAPICache restituisce map[praticaID]map[campo]valore con i dati dichiarati dall'API per un bando.
func GetAPICache(db *sql.DB, bandoID int) (map[string]map[string]string, error) {
	rows, err := db.Query(`
		SELECT pratica_id, dati_json FROM istruttorie_api_cache
		WHERE bando_id=? AND dati_json NOT IN ('{}', '')`,
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

func ListApprovate(db *sql.DB, bandoID int) ([]string, error) {
	rows, err := db.Query(`SELECT pratica_id FROM istruttorie WHERE bando_id=? AND stato='approvata'`, bandoID)
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

