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
}

func InsertRun(db *sql.DB, r *GraduatoriaRun) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO graduatorie_run (bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.BandoID, r.CalcolataDa, r.CalcolataAt.UTC().Format(time.RFC3339),
		r.DatiJSON, r.NumTotale, r.NumAmmesse, r.NumEscluse, r.BudgetUsato, r.Note,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	return res.LastInsertId()
}

func ListRuns(db *sql.DB, bandoID int64) ([]*GraduatoriaRun, error) {
	rows, err := db.Query(
		`SELECT id, bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, COALESCE(note,'')
		 FROM graduatorie_run WHERE bando_id = ? ORDER BY id DESC`, bandoID)
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

func GetRun(db *sql.DB, id int64) (*GraduatoriaRun, error) {
	row := db.QueryRow(
		`SELECT id, bando_id, calcolata_da, calcolata_at, dati_json, num_totale, num_ammesse, num_escluse, budget_usato, COALESCE(note,'')
		 FROM graduatorie_run WHERE id = ?`, id)
	r, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %d non trovata", id)
	}
	return r, err
}

func scanRun(s scanner) (*GraduatoriaRun, error) {
	var r GraduatoriaRun
	var atStr string
	err := s.Scan(&r.ID, &r.BandoID, &r.CalcolataDa, &atStr, &r.DatiJSON,
		&r.NumTotale, &r.NumAmmesse, &r.NumEscluse, &r.BudgetUsato, &r.Note)
	if err != nil {
		return nil, err
	}
	r.CalcolataAt, _ = time.Parse(time.RFC3339, atStr)
	return &r, nil
}
