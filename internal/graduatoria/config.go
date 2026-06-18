package graduatoria

// EngineConfig contiene la configurazione completa di un engine generico.
// Serializzata come JSON nel campo engine_config della tabella bandi.
type EngineConfig struct {
	Mapping        map[string]FieldMapping `json:"mapping"`
	Espansione     string                  `json:"espansione"`
	Filtri         []FiltroConfig          `json:"filtri"`
	Deduplicazione DedupConfig             `json:"deduplicazione"`
	Ordinamento    []OrdineConfig          `json:"ordinamento"`
	Modalita       string                  `json:"modalita"` // "fondi"|"posti"|"ammissione"|"lista_attesa"
	Tipologie      []TipologiaConfig       `json:"tipologie"`
	Rimborso       RimborsoConfig          `json:"rimborso"`
	Verifica       VerificaConfig          `json:"verifica"`
}

type FieldMapping struct {
	Path     string `json:"path"`
	Tipo     string `json:"tipo"`                  // "float" | "int" | "string" | "count" | "time"
	Expand   bool   `json:"expand"`                // true = relativo a elemento array espansione
	Label    string `json:"label,omitempty"`       // nome leggibile (opzionale, default = chiave mapping)
	PDNDPath string `json:"pdnd_path,omitempty"`   // path del campo firma/signature PDND (es. meta.signature)
	PDNDOp   string `json:"pdnd_op,omitempty"`     // "non_vuoto" | "==" | "!=" (default: "non_vuoto")
	PDNDVal  string `json:"pdnd_val,omitempty"`    // valore confronto (solo per "==" e "!=")
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
	Nome     string      `json:"nome"`
	Campo    string      `json:"campo"`   // "" = corrisponde a tutti
	Valore   string      `json:"valore"`  // "" = corrisponde a tutti
	Priorita int         `json:"priorita"`
	Limite   LimiteConfig `json:"limite"`
}

// LimiteConfig definisce come vengono assegnate le risorse nella tipologia.
type LimiteConfig struct {
	Tipo   string  `json:"tipo"`   // "budget"|"posti"|"nessuno"|"residuo"|"percentuale"|"fisso"
	Valore float64 `json:"valore"` // € se budget/fisso, count se posti, percentuale (0-1) se percentuale
}

type RimborsoConfig struct {
	Tipo           string `json:"tipo"`            // "netto" | "lordo"
	CampoLordo     string `json:"campo_lordo"`
	CampoDeduzione string `json:"campo_deduzione"`
}

// VerificaConfig definisce i criteri di istruttoria pre-calcolo.
// Se Attiva=true, il calcolo graduatoria è bloccato finché tutte le
// domande flaggate non sono state smarcate dagli operatori.
type VerificaConfig struct {
	Attiva                 bool               `json:"attiva"`
	FiltriFlag             []FiltroFlagConfig  `json:"filtri_flag"`
	// VerificaCertificazione=true: flag automatico per ogni campo con PDNDPath configurato
	// ma con firma vuota (dato non proveniente da PDND/fonte certificata).
	VerificaCertificazione bool               `json:"verifica_certificazione"`
}

// FiltroFlagConfig è un filtro che, se soddisfatto, flagga la domanda
// per verifica manuale. Motivo è la label mostrata all'operatore.
type FiltroFlagConfig struct {
	Campo  string `json:"campo"`
	Op     string `json:"op"`
	Valore any    `json:"valore,omitempty"`
	Motivo string `json:"motivo"`
}
