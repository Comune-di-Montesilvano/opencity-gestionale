package graduatoria

// EngineConfig contiene la configurazione completa di un engine generico.
// Serializzata come JSON nel campo engine_config della tabella bandi.
type EngineConfig struct {
	Mapping        map[string]FieldMapping `json:"mapping"`
	Espansione     string                  `json:"espansione"`
	Filtri         []FiltroConfig          `json:"filtri"`
	Deduplicazione DedupConfig             `json:"deduplicazione"`
	Ordinamento    []OrdineConfig          `json:"ordinamento"`
	Tipologie      []TipologiaConfig       `json:"tipologie"`
	Rimborso       RimborsoConfig          `json:"rimborso"`
}

type FieldMapping struct {
	Path   string `json:"path"`
	Tipo   string `json:"tipo"`   // "float" | "int" | "string" | "count" | "time"
	Expand bool   `json:"expand"` // true = relativo a elemento array espansione
}

type FiltroConfig struct {
	Campo  string `json:"campo"`
	Op     string `json:"op"`    // "<=" | ">=" | "==" | "<" | ">" | "!="
	Valore any    `json:"valore"`
}

type DedupConfig struct {
	Attiva bool     `json:"attiva"`
	Chiave []string `json:"chiave"`
}

type OrdineConfig struct {
	Campo string `json:"campo"`
	Dir   string `json:"dir"` // "asc" | "desc"
}

type TipologiaConfig struct {
	Nome     string       `json:"nome"`
	Campo    string       `json:"campo"`
	Valore   string       `json:"valore"`
	Priorita int          `json:"priorita"`
	Budget   BudgetConfig `json:"budget"`
}

type BudgetConfig struct {
	Tipo   string  `json:"tipo"`   // "residuo" | "percentuale" | "fisso"
	Valore float64 `json:"valore"` // usato per percentuale e fisso
}

type RimborsoConfig struct {
	Tipo           string `json:"tipo"`            // "netto" | "lordo"
	CampoLordo     string `json:"campo_lordo"`
	CampoDeduzione string `json:"campo_deduzione"`
}
