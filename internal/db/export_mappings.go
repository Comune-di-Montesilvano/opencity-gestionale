package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ExportColonna descrive una colonna in un template di esportazione CSV.
type ExportColonna struct {
	Label    string `json:"label"`
	Sorgente string `json:"sorgente"`          // "sistema" | "mappato" | "raw"
	Chiave   string `json:"chiave,omitempty"`  // per sorgente sistema/mappato
	Path     string `json:"path,omitempty"`    // per sorgente raw (dot-notation in app.Data)
}

// ExportMapping è un template CSV riutilizzabile per un bando.
type ExportMapping struct {
	ID          int64
	BandoID     int64
	Nome        string
	FiltroStati []string        // vuoto = tutti gli stati
	Colonne     []ExportColonna
	CreatedAt   time.Time
}

// Chiavi valide per sorgente "sistema" (estratte dal run snapshot senza re-fetch).
var SistemaChiavi = []string{
	"posizione", "protocollo", "cognome", "nome", "cf_richiedente",
	"tipologia", "importo", "annualita", "stato_app",
}

// ColonneDefaultRagioneria è il set predefinito per il template "Ragioneria / Bonifici".
var ColonneDefaultRagioneria = []ExportColonna{
	{Label: "IBAN", Sorgente: "raw", Path: "iban.iban"},
	{Label: "Intestatario", Sorgente: "raw", Path: ""},
	{Label: "Importo (€)", Sorgente: "sistema", Chiave: "importo"},
	{Label: "Comune residenza", Sorgente: "mappato", Chiave: "comune"},
	{Label: "Provincia", Sorgente: "mappato", Chiave: "provincia"},
}

// ColonneDefaultGenerico è il set predefinito per il template generico.
var ColonneDefaultGenerico = []ExportColonna{
	{Label: "Posizione", Sorgente: "sistema", Chiave: "posizione"},
	{Label: "Protocollo", Sorgente: "sistema", Chiave: "protocollo"},
	{Label: "Cognome", Sorgente: "sistema", Chiave: "cognome"},
	{Label: "Nome", Sorgente: "sistema", Chiave: "nome"},
	{Label: "Tipologia", Sorgente: "sistema", Chiave: "tipologia"},
	{Label: "Importo (€)", Sorgente: "sistema", Chiave: "importo"},
}

func ListExportMappings(db *sql.DB, bandoID int64) ([]*ExportMapping, error) {
	rows, err := db.Query(
		`SELECT id, bando_id, nome, filtro_stati, colonne_json, created_at
		 FROM export_mappings WHERE bando_id=? ORDER BY id ASC`, bandoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ExportMapping
	for rows.Next() {
		m, err := scanExportMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func GetExportMapping(db *sql.DB, id int64) (*ExportMapping, error) {
	row := db.QueryRow(
		`SELECT id, bando_id, nome, filtro_stati, colonne_json, created_at
		 FROM export_mappings WHERE id=?`, id)
	m, err := scanExportMapping(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

func InsertExportMapping(db *sql.DB, m *ExportMapping) (int64, error) {
	filtroJSON, _ := json.Marshal(m.FiltroStati)
	colonneJSON, _ := json.Marshal(m.Colonne)
	res, err := db.Exec(
		`INSERT INTO export_mappings (bando_id, nome, filtro_stati, colonne_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.BandoID, m.Nome, string(filtroJSON), string(colonneJSON),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateExportMapping(db *sql.DB, m *ExportMapping) error {
	filtroJSON, _ := json.Marshal(m.FiltroStati)
	colonneJSON, _ := json.Marshal(m.Colonne)
	_, err := db.Exec(
		`UPDATE export_mappings SET nome=?, filtro_stati=?, colonne_json=? WHERE id=?`,
		m.Nome, string(filtroJSON), string(colonneJSON), m.ID,
	)
	return err
}

func DeleteExportMapping(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM export_mappings WHERE id=?`, id)
	return err
}

type exportMappingScanner interface {
	Scan(dest ...any) error
}

func scanExportMapping(s exportMappingScanner) (*ExportMapping, error) {
	var m ExportMapping
	var filtroJSON, colonneJSON, createdAtStr string
	err := s.Scan(&m.ID, &m.BandoID, &m.Nome, &filtroJSON, &colonneJSON, &createdAtStr)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(filtroJSON), &m.FiltroStati)
	json.Unmarshal([]byte(colonneJSON), &m.Colonne)
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &m, nil
}
