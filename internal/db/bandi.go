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
	StatoBando           string // "bozza" | "attivo" | "archiviato"
	ValoriSuperset        string // JSON blob: map[arrayPath]map[fieldName][]string
	CreatedAt             time.Time
}

func InsertBando(db *sql.DB, b *Bando) (int64, error) {
	stato := b.StatoBando
	if stato == "" {
		stato = "bozza"
	}
	res, err := db.Exec(
		`INSERT INTO bandi (service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, stato_bando, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ServiceID, b.Nome, b.BudgetTotale, b.ISEEMassimo, b.ScadenzaPresentazione,
		b.EngineType, b.EngineConfig, boolToInt(b.Attivo), stato, b.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("insert bando: %w", err)
	}
	return res.LastInsertId()
}


// ListBandi restituisce i bandi filtrati per stato ("bozza", "attivo", "archiviato", "" = tutti).
func ListBandi(db *sql.DB, stato string) ([]*Bando, error) {
	if stato == "archiviato" {
		return listBandiQuery(db,
			`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, COALESCE(stato_bando,'bozza'), COALESCE(valori_superset,'{}'), created_at FROM bandi WHERE attivo=0 ORDER BY id DESC`)
	}
	if stato != "" {
		return listBandiQuery(db,
			`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, COALESCE(stato_bando,'bozza'), COALESCE(valori_superset,'{}'), created_at FROM bandi WHERE stato_bando=? AND attivo=1 ORDER BY id DESC`,
			stato)
	}
	return listBandiQuery(db,
		`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, COALESCE(stato_bando,'bozza'), COALESCE(valori_superset,'{}'), created_at FROM bandi ORDER BY id DESC`)
}

func listBandiQuery(db *sql.DB, q string, args ...any) ([]*Bando, error) {
	rows, err := db.Query(q, args...)
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
	row := db.QueryRow(
		`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, COALESCE(stato_bando,'bozza'), COALESCE(valori_superset,'{}'), created_at FROM bandi WHERE id = ?`, id)
	b, err := scanBando(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("bando %d non trovato", id)
	}
	return b, err
}

func GetBandoByServiceID(db *sql.DB, serviceID string) (*Bando, error) {
	row := db.QueryRow(
		`SELECT id, service_id, nome, budget_totale, isee_massimo, scadenza_presentazione, engine_type, engine_config, attivo, COALESCE(stato_bando,'bozza'), COALESCE(valori_superset,'{}'), created_at FROM bandi WHERE service_id = ? AND attivo = 1`, serviceID)
	b, err := scanBando(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("bando per service_id %s non trovato", serviceID)
	}
	return b, err
}

func UpdateBando(db *sql.DB, b *Bando) error {
	_, err := db.Exec(
		`UPDATE bandi SET nome=?, budget_totale=?, isee_massimo=?, scadenza_presentazione=?, engine_type=?, engine_config=?, attivo=?, stato_bando=? WHERE id=?`,
		b.Nome, b.BudgetTotale, b.ISEEMassimo, b.ScadenzaPresentazione, b.EngineType, b.EngineConfig, boolToInt(b.Attivo), b.StatoBando, b.ID,
	)
	return err
}

func UpdateEngineConfig(db *sql.DB, id int64, engineConfig string) error {
	_, err := db.Exec(`UPDATE bandi SET engine_config=? WHERE id=?`, engineConfig, id)
	return err
}

func AttivaBando(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE bandi SET stato_bando='attivo' WHERE id=?`, id)
	return err
}

func ArchiviaBando(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE bandi SET attivo=0, stato_bando='archiviato' WHERE id=?`, id)
	return err
}

func DuplicaBando(db *sql.DB, id int64) (int64, error) {
	b, err := GetBando(db, id)
	if err != nil {
		return 0, err
	}
	copia := &Bando{
		ServiceID:             b.ServiceID,
		Nome:                  b.Nome + " (copia)",
		BudgetTotale:          b.BudgetTotale,
		ISEEMassimo:           b.ISEEMassimo,
		ScadenzaPresentazione: b.ScadenzaPresentazione,
		EngineType:            b.EngineType,
		EngineConfig:          b.EngineConfig,
		Attivo:                true,
		StatoBando:           "bozza",
		CreatedAt:             time.Now(),
	}
	return InsertBando(db, copia)
}

func DisattivaBando(db *sql.DB, id int64) error {
	return ArchiviaBando(db, id)
}

func CountBandi(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM bandi`).Scan(&n)
	return n, err
}

func CountBandiPerStato(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT COALESCE(stato_bando,'bozza'), COUNT(*) FROM bandi GROUP BY stato_bando`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{"attivo": 0, "bozza": 0, "archiviato": 0}
	for rows.Next() {
		var stato string
		var n int
		if err := rows.Scan(&stato, &n); err != nil {
			return nil, err
		}
		counts[stato] = n
	}
	return counts, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBando(s scanner) (*Bando, error) {
	var b Bando
	var attivoInt int
	var createdAtStr string
	err := s.Scan(&b.ID, &b.ServiceID, &b.Nome, &b.BudgetTotale, &b.ISEEMassimo,
		&b.ScadenzaPresentazione, &b.EngineType, &b.EngineConfig, &attivoInt, &b.StatoBando, &b.ValoriSuperset, &createdAtStr)
	if err != nil {
		return nil, err
	}
	b.Attivo = attivoInt == 1
	b.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &b, nil
}

func SaveValoriSuperset(db *sql.DB, bandoID int64, jsonBlob string) error {
	_, err := db.Exec(`UPDATE bandi SET valori_superset=? WHERE id=?`, jsonBlob, bandoID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
