package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Bando struct {
	ID                    int64
	ServiceID             string
	Nome                  string
	BudgetTotale          float64
	ISEEMassimo           float64
	ScadenzaPresentazione string
	EngineType            string
	EngineConfig          string
	Attivo                bool
	CreatedAt             time.Time
}

func InsertBando(db *sql.DB, b *Bando) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO bandi (service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ServiceID, b.Nome, b.BudgetTotale, b.ISEEMassimo, b.ScadenzaPresentazione,
		b.EngineType, b.EngineConfig, boolToInt(b.Attivo), b.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("insert bando: %w", err)
	}
	return res.LastInsertId()
}

func ListBandi(db *sql.DB) ([]*Bando, error) {
	rows, err := db.Query(`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, created_at FROM bandi ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Bando
	for rows.Next() {
		b, err := scanBando(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func GetBando(db *sql.DB, id int64) (*Bando, error) {
	row := db.QueryRow(`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, created_at FROM bandi WHERE id = ?`, id)
	b, err := scanBando(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("bando %d non trovato", id)
	}
	return b, err
}

func GetBandoByServiceID(db *sql.DB, serviceID string) (*Bando, error) {
	row := db.QueryRow(`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, created_at FROM bandi WHERE service_id = ? AND attivo = 1`, serviceID)
	b, err := scanBando(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("bando per service_id %s non trovato", serviceID)
	}
	return b, err
}

func UpdateBando(db *sql.DB, b *Bando) error {
	_, err := db.Exec(
		`UPDATE bandi SET nome=?, budget_totale=?, isee_massimo=?, scadenza_presentazione=?, engine_type=?, engine_config=?, attivo=? WHERE id=?`,
		b.Nome, b.BudgetTotale, b.ISEEMassimo, b.ScadenzaPresentazione, b.EngineType, b.EngineConfig, boolToInt(b.Attivo), b.ID,
	)
	return err
}

func DisattivaBando(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE bandi SET attivo=0 WHERE id=?`, id)
	return err
}

func CountBandi(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM bandi`).Scan(&n)
	return n, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBando(s scanner) (*Bando, error) {
	var b Bando
	var attivoInt int
	var createdAtStr string
	err := s.Scan(&b.ID, &b.ServiceID, &b.Nome, &b.BudgetTotale, &b.ISEEMassimo,
		&b.ScadenzaPresentazione, &b.EngineType, &b.EngineConfig, &attivoInt, &createdAtStr)
	if err != nil {
		return nil, err
	}
	b.Attivo = attivoInt == 1
	b.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &b, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
