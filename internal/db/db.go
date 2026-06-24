package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// Migration idempotente: aggiunge colonne mancanti per DB pre-esistenti.
	_, _ = db.Exec(`ALTER TABLE graduatorie_run ADD COLUMN stato TEXT NOT NULL DEFAULT 'bozza'`)
	_, _ = db.Exec(`ALTER TABLE bandi ADD COLUMN stato_motore TEXT NOT NULL DEFAULT 'bozza'`)
	_, _ = db.Exec(`ALTER TABLE bandi RENAME COLUMN stato_motore TO stato_bando`)
	_, _ = db.Exec(`ALTER TABLE bandi ADD COLUMN valori_superset TEXT NOT NULL DEFAULT '{}'`)
	_, _ = db.Exec(`ALTER TABLE bandi ADD COLUMN export_colonne TEXT NOT NULL DEFAULT '[]'`)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN app_status TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN dati_json TEXT NOT NULL DEFAULT '{}'`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS istruttorie_dati (pratica_id TEXT PRIMARY KEY, dati_json TEXT NOT NULL DEFAULT '{}', nota TEXT NOT NULL DEFAULT '', aggiornato_il TEXT)`)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN nota_lavoro TEXT NOT NULL DEFAULT ''`)
	// Migra note esistenti cross-bando → per-bando (una-tantum, non sovrascrive note già migrate).
	_, _ = db.Exec(`UPDATE istruttorie SET nota_lavoro = (
		SELECT nota FROM istruttorie_dati
		WHERE istruttorie_dati.pratica_id = istruttorie.pratica_id AND nota != ''
	) WHERE nota_lavoro = '' AND EXISTS (
		SELECT 1 FROM istruttorie_dati
		WHERE istruttorie_dati.pratica_id = istruttorie.pratica_id AND nota != ''
	)`)
	// Migrazione: rinomina chiavi PDND → Verifica nei blob engine_config
	_, _ = db.Exec(`UPDATE bandi SET engine_config =
		replace(replace(replace(engine_config,
			'"pdnd_path"', '"verifica_path"'),
			'"pdnd_op"',  '"verifica_op"'),
			'"pdnd_val"', '"verifica_val"')
		WHERE engine_config LIKE '%"pdnd_path"%'`)
	return db, nil
}
