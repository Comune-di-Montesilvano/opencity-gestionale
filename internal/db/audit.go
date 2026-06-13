package db

import (
	"database/sql"
	"fmt"
	"time"
)

type AuditAction struct {
	ID              int64
	Operatore       string
	Azione          string
	PraticaID       string
	BandoID         int64
	RunID           int64
	Messaggio       string
	Esito           string
	ErroreDettaglio string
	CreatedAt       time.Time
}

func InsertAudit(db *sql.DB, a *AuditAction) error {
	_, err := db.Exec(
		`INSERT INTO audit_actions (operatore, azione, pratica_id, bando_id, run_id, messaggio, esito, errore_dettaglio, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Operatore, a.Azione,
		nullStr(a.PraticaID), nullInt(a.BandoID), nullInt(a.RunID),
		nullStr(a.Messaggio), a.Esito, nullStr(a.ErroreDettaglio),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

type AuditFilter struct {
	Operatore string
	Azione    string
	BandoID   int64
	Limit     int
	Offset    int
}

func ListAudit(db *sql.DB, f AuditFilter) ([]*AuditAction, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if f.Operatore != "" {
		where += " AND operatore = ?"
		args = append(args, f.Operatore)
	}
	if f.Azione != "" {
		where += " AND azione = ?"
		args = append(args, f.Azione)
	}
	if f.BandoID > 0 {
		where += " AND bando_id = ?"
		args = append(args, f.BandoID)
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_actions"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	query := "SELECT id, operatore, azione, COALESCE(pratica_id,''), COALESCE(bando_id,0), COALESCE(run_id,0), COALESCE(messaggio,''), esito, COALESCE(errore_dettaglio,''), created_at FROM audit_actions" +
		where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := db.Query(query, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*AuditAction
	for rows.Next() {
		var a AuditAction
		var createdStr string
		if err := rows.Scan(&a.ID, &a.Operatore, &a.Azione, &a.PraticaID, &a.BandoID, &a.RunID,
			&a.Messaggio, &a.Esito, &a.ErroreDettaglio, &createdStr); err != nil {
			return nil, 0, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		out = append(out, &a)
	}
	return out, total, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}
