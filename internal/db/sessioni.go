package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Sessione struct {
	ID          string
	Operatore   string
	UserID      string
	JWTOpenCity string
	Ruolo       string
	ServiceIDs  string // JSON array: ["uuid1","uuid2",...]
	ScadeAt     time.Time
	CreatedAt   time.Time
}

func InsertSessione(db *sql.DB, s *Sessione) error {
	_, err := db.Exec(
		`INSERT INTO sessioni (id, operatore, user_id, jwt_opencity, ruolo, service_ids, scade_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Operatore, s.UserID, s.JWTOpenCity, s.Ruolo, s.ServiceIDs,
		s.ScadeAt.UTC().Format(time.RFC3339),
		s.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert sessione: %w", err)
	}
	return nil
}

func GetSessione(db *sql.DB, id string) (*Sessione, error) {
	row := db.QueryRow(
		`SELECT id, operatore, user_id, jwt_opencity, ruolo, service_ids, scade_at, created_at
		 FROM sessioni WHERE id = ?`, id)
	var s Sessione
	var scadeStr, createdStr string
	err := row.Scan(&s.ID, &s.Operatore, &s.UserID, &s.JWTOpenCity, &s.Ruolo, &s.ServiceIDs, &scadeStr, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get sessione: %w", err)
	}
	s.ScadeAt, err = time.Parse(time.RFC3339, scadeStr)
	if err != nil {
		return nil, fmt.Errorf("parse scade_at: %w", err)
	}
	s.CreatedAt, err = time.Parse(time.RFC3339, createdStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &s, nil
}

func DeleteSessione(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM sessioni WHERE id = ?`, id)
	return err
}

func PulisciSessioniScadute(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM sessioni WHERE scade_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}
