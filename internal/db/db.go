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
	migrateIstruttorieDati(db)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN nota_lavoro TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE istruttorie ADD COLUMN includi_dufficio INTEGER NOT NULL DEFAULT 0`)
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

func migrateIstruttorieDati(db *sql.DB) {
	// 1. Assicuriamoci che la tabella esista (se è una nuova installazione, viene creata con la PK corretta)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS istruttorie_dati (
		pratica_id TEXT NOT NULL,
		bando_id INTEGER NOT NULL DEFAULT 0,
		dati_json TEXT NOT NULL DEFAULT '{}',
		nota TEXT NOT NULL DEFAULT '',
		aggiornato_il TEXT,
		PRIMARY KEY (pratica_id, bando_id)
	)`)

	// 2. Controlliamo se bando_id fa parte della primary key
	var bandoIdIsPK bool
	rows, err := db.Query(`PRAGMA table_info(istruttorie_dati)`)
	if err == nil {
		for rows.Next() {
			var cid int
			var name, dtype string
			var notnull, pk int
			var dfltVal sql.NullString
			if rows.Scan(&cid, &name, &dtype, &notnull, &dfltVal, &pk) == nil {
				if name == "bando_id" && pk > 0 {
					bandoIdIsPK = true
				}
			}
		}
		rows.Close()
	}

	// 3. Se bando_id non è parte della PK, dobbiamo ricostruire la tabella
	if !bandoIdIsPK {
		// Rinominiamo la tabella vecchia
		_, _ = db.Exec(`ALTER TABLE istruttorie_dati RENAME TO old_istruttorie_dati`)

		// Creiamo la nuova tabella con la PK composita
		_, _ = db.Exec(`CREATE TABLE istruttorie_dati (
			pratica_id TEXT NOT NULL,
			bando_id INTEGER NOT NULL DEFAULT 0,
			dati_json TEXT NOT NULL DEFAULT '{}',
			nota TEXT NOT NULL DEFAULT '',
			aggiornato_il TEXT,
			PRIMARY KEY (pratica_id, bando_id)
		)`)

		// Copiamo i dati. Se la tabella vecchia aveva già bando_id, usiamolo; altrimenti usiamo 0
		var oldHasBandoId bool
		oldRows, oldErr := db.Query(`PRAGMA table_info(old_istruttorie_dati)`)
		if oldErr == nil {
			for oldRows.Next() {
				var cid int
				var name, dtype string
				var notnull, pk int
				var dfltVal sql.NullString
				if oldRows.Scan(&cid, &name, &dtype, &notnull, &dfltVal, &pk) == nil {
					if name == "bando_id" {
						oldHasBandoId = true
					}
				}
			}
			oldRows.Close()
		}

		if oldHasBandoId {
			_, _ = db.Exec(`INSERT INTO istruttorie_dati (pratica_id, bando_id, dati_json, nota, aggiornato_il)
				SELECT index_cols.pratica_id, index_cols.bando_id, index_cols.dati_json, index_cols.nota, index_cols.aggiornato_il FROM old_istruttorie_dati index_cols`)
		} else {
			_, _ = db.Exec(`INSERT INTO istruttorie_dati (pratica_id, bando_id, dati_json, nota, aggiornato_il)
				SELECT index_cols.pratica_id, 0, index_cols.dati_json, index_cols.nota, index_cols.aggiornato_il FROM old_istruttorie_dati index_cols`)
		}

		// Rimuoviamo la tabella temporanea
		_, _ = db.Exec(`DROP TABLE old_istruttorie_dati`)
	}
}
