CREATE TABLE IF NOT EXISTS bandi (
  id            INTEGER PRIMARY KEY,
  service_id    TEXT NOT NULL,
  nome          TEXT NOT NULL,
  budget_totale REAL,
  isee_massimo  REAL,
  scadenza_presentazione TEXT,
  engine_type   TEXT NOT NULL DEFAULT 'generic',
  engine_config TEXT NOT NULL DEFAULT '{}',
  attivo        INTEGER NOT NULL DEFAULT 1,
  stato_motore  TEXT NOT NULL DEFAULT 'bozza',
  created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS graduatorie_run (
  id           INTEGER PRIMARY KEY,
  bando_id     INTEGER NOT NULL REFERENCES bandi(id),
  calcolata_da TEXT NOT NULL,
  calcolata_at TEXT NOT NULL,
  dati_json    TEXT NOT NULL,
  num_totale   INTEGER,
  num_ammesse  INTEGER,
  num_escluse  INTEGER,
  budget_usato REAL,
  note         TEXT,
  stato        TEXT NOT NULL DEFAULT 'bozza'
);

CREATE TABLE IF NOT EXISTS audit_actions (
  id               INTEGER PRIMARY KEY,
  operatore        TEXT NOT NULL,
  azione           TEXT NOT NULL,
  pratica_id       TEXT,
  bando_id         INTEGER,
  run_id           INTEGER,
  messaggio        TEXT,
  esito            TEXT,
  errore_dettaglio TEXT,
  created_at       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessioni (
  id           TEXT PRIMARY KEY,
  operatore    TEXT NOT NULL,
  user_id      TEXT NOT NULL,
  jwt_opencity TEXT NOT NULL,
  ruolo        TEXT NOT NULL,
  service_ids  TEXT NOT NULL DEFAULT '[]',
  scade_at     TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
