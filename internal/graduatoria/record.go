package graduatoria

import (
	"fmt"
	"strings"
	"time"
)

// Record è il record estratto da un'istanza OpenCity tramite engine generico.
// I campi sono indicizzati per nome logico (come definito in EngineConfig.Mapping).
type Record struct {
	AppID      string
	AppStatus  string
	FloatMap   map[string]float64
	StringMap  map[string]string
	IntMap     map[string]int
	TimeMap    map[string]time.Time
}

func NewRecord(appID, appStatus string) *Record {
	return &Record{
		AppID:     appID,
		AppStatus: appStatus,
		FloatMap:  make(map[string]float64),
		StringMap: make(map[string]string),
		IntMap:    make(map[string]int),
		TimeMap:   make(map[string]time.Time),
	}
}

func (r *Record) Float(campo string) float64  { return r.FloatMap[campo] }
func (r *Record) Str(campo string) string     { return r.StringMap[campo] }
func (r *Record) Int(campo string) int        { return r.IntMap[campo] }
func (r *Record) Time(campo string) time.Time { return r.TimeMap[campo] }

// HasFloat controlla se il campo float è presente (utile per filtri).
func (r *Record) HasFloat(campo string) bool {
	_, ok := r.FloatMap[campo]
	return ok
}

// DerivaCampi calcola i campi derivati in base alla RimborsoConfig.
func (r *Record) DerivaCampi(cfg RimborsoConfig) {
	if cfg.Tipo == "netto" {
		lordo := r.FloatMap[cfg.CampoLordo]
		deduzione := r.FloatMap[cfg.CampoDeduzione]
		netto := lordo - deduzione
		if netto < 0 {
			netto = 0
		}
		r.FloatMap["corrispettivo_netto"] = netto
	}
}

// ChiaveDedup costruisce la chiave di deduplicazione come stringa composita.
func (r *Record) ChiaveDedup(campi []string) string {
	parts := make([]string, len(campi))
	for i, c := range campi {
		if v, ok := r.StringMap[c]; ok {
			parts[i] = v
		} else if v, ok := r.FloatMap[c]; ok {
			parts[i] = fmt.Sprintf("%v", v)
		} else if v, ok := r.IntMap[c]; ok {
			parts[i] = fmt.Sprintf("%d", v)
		}
	}
	return strings.Join(parts, "|")
}

// PassaFiltro verifica se il record supera il filtro dato.
func (r *Record) PassaFiltro(f FiltroConfig) bool {
	// Prova float
	if fv, ok := r.FloatMap[f.Campo]; ok {
		target := toFloat64(f.Valore)
		return cmpFloat(fv, f.Op, target)
	}
	// Prova time
	if tv, ok := r.TimeMap[f.Campo]; ok {
		target := toTime(f.Valore)
		return cmpTime(tv, f.Op, target)
	}
	// Prova int
	if iv, ok := r.IntMap[f.Campo]; ok {
		target := int(toFloat64(f.Valore))
		return cmpInt(iv, f.Op, target)
	}
	// Prova string
	if sv, ok := r.StringMap[f.Campo]; ok {
		target := fmt.Sprintf("%v", f.Valore)
		return cmpString(sv, f.Op, target)
	}
	return false
}

// ToIstanza converte il Record in Istanza per compatibilità con i template.
// I campi non mappati restano al valore zero.
func (r *Record) ToIstanza() *Istanza {
	ist := &Istanza{
		ID:        r.AppID,
		Status:    r.AppStatus,
		ISEE:      r.FloatMap["isee"],
		Corrispettivo:     r.FloatMap["corrispettivo"],
		BeneficioRicevuto: r.FloatMap["beneficio"],
		FiglioSelezionatoCF: r.StringMap["figlio_cf"],
		RichiedenteCF:       r.StringMap["richiedente_cf"],
		TipoRichiesta: r.StringMap["tipo"],
		NumFigli:      r.IntMap["num_figli"],
		Annualita:     r.IntMap["annualita"],
	}
	if t, ok := r.TimeMap["data_presentazione"]; ok {
		ist.SubmittedAt = t.Format(time.RFC3339)
	}
	return ist
}

// --- helpers confronto ---

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	}
	return 0
}

func toTime(v any) time.Time {
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func cmpFloat(a float64, op string, b float64) bool {
	switch op {
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	}
	return false
}

func cmpTime(a time.Time, op string, b time.Time) bool {
	switch op {
	case "<=":
		return !a.After(b)
	case ">=":
		return !a.Before(b)
	case "==":
		return a.Equal(b)
	case "!=":
		return !a.Equal(b)
	case "<":
		return a.Before(b)
	case ">":
		return a.After(b)
	}
	return false
}

func cmpInt(a int, op string, b int) bool {
	switch op {
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	}
	return false
}

func cmpString(a, op, b string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	}
	return false
}
