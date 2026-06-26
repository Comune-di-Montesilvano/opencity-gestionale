package db

import (
	"database/sql"
	"fmt"
	"time"
)

type GraduatoriaRun struct {
	ID          int64
	BandoID     int64
	CalcolataDa string
	CalcolataAt time.Time
	DatiJSON    string
	NumTotale   int
	NumAmmesse  int
	NumEscluse  int
	BudgetUsato float64
	Note        string
	Stato       string // "bozza" | "pubblicata"
}

// RunAltroBandoInfo è usato per calcolare il badge "Anche in" basandosi sulle run degli altri bandi.
type RunAltroBandoInfo struct {
	BandoNome string
	DatiJSON  string
}

// GetLatestRunsAltriBandi restituisce la run più recente per ogni bando eccetto excludeBandoID.
// Il JSON di ogni run va parsato dal chiamante per estrarre le pratica IDs.
func GetLatestRunsAltriBandi(db *sql.DB, excludeBandoID int64) ([]RunAltroBandoInfo, error) {
	rows, err := db.Query(`
		SELECT COALESCE(b.nome, 'Bando '||r.bando_id), r.dati_json
		FROM graduatorie_run r
		LEFT JOIN bandi b ON b.id = r.bando_id
		WHERE r.bando_id != ? AND r.id = (
			SELECT MAX(r2.id) FROM graduatorie_run r2 WHERE r2.bando_id = r.bando_id
		)`, excludeBandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunAltroBandoInfo
	for rows.Next() {
		var nome, datiJSON string
		if err := rows.Scan(&nome, &datiJSON); err == nil {
			out = append(out, RunAltroBandoInfo{BandoNome: nome, DatiJSON: datiJSON})
		}
	}
	return out, rows.Err()
}

func InsertRun(db *sql.DB, r *GraduatoriaRun) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO graduatorie_run (bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, note, stato)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'bozza')`,
		r.BandoID, r.CalcolataDa, r.CalcolataAt.UTC().Format(time.RFC3339),
		r.DatiJSON, r.NumTotale, r.NumAmmesse, r.NumEscluse, r.BudgetUsato, r.Note,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	return res.LastInsertId()
}

// ListRuns restituisce le run per un bando.
// Se soloPublicate è true, filtra solo quelle in stato 'pubblicata' (per operatori non-admin).
func ListRuns(db *sql.DB, bandoID int64, soloPublicate ...bool) ([]*GraduatoriaRun, error) {
	q := `SELECT id, bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, COALESCE(note,''), COALESCE(stato,'bozza')
		  FROM graduatorie_run WHERE bando_id = ?`
	if len(soloPublicate) > 0 && soloPublicate[0] {
		q += ` AND stato = 'pubblicata'`
	}
	q += ` ORDER BY id DESC`
	rows, err := db.Query(q, bandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GraduatoriaRun
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetLatestRun restituisce la run più recente per un bando (nil se nessuna).
func GetLatestRun(db *sql.DB, bandoID int64) (*GraduatoriaRun, error) {
	row := db.QueryRow(
		`SELECT id, bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, COALESCE(note,''), COALESCE(stato,'bozza')
		 FROM graduatorie_run WHERE bando_id = ? ORDER BY id DESC LIMIT 1`, bandoID)
	r, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func GetRun(db *sql.DB, id int64) (*GraduatoriaRun, error) {
	row := db.QueryRow(
		`SELECT id, bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, COALESCE(note,''), COALESCE(stato,'bozza')
		 FROM graduatorie_run WHERE id = ?`, id)
	r, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %d non trovata", id)
	}
	return r, err
}

func PubblicaRun(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE graduatorie_run SET stato = 'pubblicata' WHERE id = ?`, id)
	return err
}

// DeleteRunBozza elimina una run solo se è in stato 'bozza'. Ritorna errore se non trovata o già pubblicata.
func DeleteRunBozza(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM graduatorie_run WHERE id = ? AND stato = 'bozza'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("run %d non trovata o già pubblicata", id)
	}
	return nil
}

func scanRun(s scanner) (*GraduatoriaRun, error) {
	var r GraduatoriaRun
	var atStr string
	err := s.Scan(&r.ID, &r.BandoID, &r.CalcolataDa, &atStr, &r.DatiJSON,
		&r.NumTotale, &r.NumAmmesse, &r.NumEscluse, &r.BudgetUsato, &r.Note, &r.Stato)
	if err != nil {
		return nil, err
	}
	r.CalcolataAt, _ = time.Parse(time.RFC3339, atStr)
	return &r, nil
}
