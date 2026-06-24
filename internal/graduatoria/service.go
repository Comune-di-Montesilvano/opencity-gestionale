package graduatoria

import (
	"encoding/json"
	"fmt"
	"time"

	"opencity-gestionale/internal/opencity"
)

// BandoConfig contiene i parametri configurabili di un bando.
// Viene popolato dal DB (tabella bandi) e passato all'engine al momento del calcolo.
type BandoConfig struct {
	BudgetTotale  float64
	ISEEMassimo   float64
	Scadenza      time.Time
	ExtraJSON     json.RawMessage            // parametri engine-specific
	CampiExtra    map[string]map[string]string // praticaID → campo → valore override da istruttoria
	Approvate     map[string]bool            // praticaID → true se approvata in istruttoria
}

// ServiceEngine è l'interfaccia che ogni motore di servizio deve implementare.
// Permette di aggiungere nuovi servizi (libri di testo, centri estivi, ecc.)
// senza modificare il web layer.
type ServiceEngine interface {
	// Name ritorna l'identificatore dell'engine (es. "mense_rette").
	Name() string

	// Calcola esegue il calcolo della graduatoria per le istanze fornite.
	Calcola(apps []opencity.Application, cfg BandoConfig) (*Graduatoria, error)

	// CSVHeaders ritorna le intestazioni delle colonne CSV.
	CSVHeaders() []string

	// CSVRecord serializza una riga della graduatoria in record CSV.
	// baseURL è l'URL base OpenCity per costruire il link diretto all'istanza.
	CSVRecord(categoria string, r RigaGraduatoria, baseURL string) []string
}

var registry = map[string]ServiceEngine{}

// Register registra un engine per il dato nome.
// Chiamato via func init() da ogni implementazione.
func Register(e ServiceEngine) {
	registry[e.Name()] = e
}

// GetEngine ritorna l'engine registrato per il nome dato.
func GetEngine(name string) (ServiceEngine, error) {
	e, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("engine %q non trovato (registrati: %v)", name, registeredNames())
	}
	return e, nil
}

func registeredNames() []string {
	var names []string
	for k := range registry {
		names = append(names, k)
	}
	return names
}
