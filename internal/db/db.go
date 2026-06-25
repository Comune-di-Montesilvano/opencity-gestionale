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
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS istruttorie_dati (pratica_id TEXT NOT NULL, bando_id INTEGER NOT NULL DEFAULT 0, dati_json TEXT NOT NULL DEFAULT '{}', nota TEXT NOT NULL DEFAULT '', aggiornato_il TEXT, PRIMARY KEY (pratica_id, bando_id))`)
	_, _ = db.Exec(`ALTER TABLE istruttorie_dati ADD COLUMN bando_id INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_istruttorie_dati_pratica_bando ON istruttorie_dati (pratica_id, bando_id)`)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN nota_lavoro TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE bandi ADD COLUMN iban_config TEXT NOT NULL DEFAULT '{}'`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS export_mappings (
		id           INTEGER PRIMARY KEY,
		bando_id     INTEGER NOT NULL,
		nome         TEXT NOT NULL,
		filtro_stati TEXT NOT NULL DEFAULT '[]',
		colonne_json TEXT NOT NULL DEFAULT '[]',
		created_at   TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS istruttorie_api_cache (
		pratica_id   TEXT NOT NULL,
		bando_id     INTEGER NOT NULL,
		dati_json    TEXT NOT NULL DEFAULT '{}',
		aggiornato_il TEXT,
		PRIMARY KEY (pratica_id, bando_id)
	)`)
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
