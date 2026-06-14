package graduatoria

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/graduatoria/cf"
)

// FlagMotivi controlla il record contro i criteri di verifica e restituisce
// la lista dei motivi per cui la domanda deve essere esaminata manualmente.
// Restituisce nil se la domanda non ha anomalie.
func (r *Record) FlagMotivi(cfg VerificaConfig) []string {
	var motivi []string
	for _, f := range cfg.FiltriFlag {
		if r.PassaFiltro(FiltroConfig{Campo: f.Campo, Op: f.Op, Valore: f.Valore}) {
			motivi = append(motivi, f.Motivo)
		}
	}
	if cfg.VerificaCertificazione {
		for key, val := range r.StringMap {
			if strings.HasPrefix(key, "__cert_") && val == "" {
				fieldName := strings.TrimPrefix(key, "__cert_")
				motivi = append(motivi, "Campo \""+fieldName+"\" non certificato PDND")
			}
		}
	}
	return motivi
}

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
	switch f.Op {
	// --- operatori CF ---
	case "cf_eta_max", "cf_eta_min", "cf_sesso", "cf_comune", "cf_valido", "cf_anno_max", "cf_anno_min":
		return valutaCF(r.StringMap[f.Campo], f.Op, f.Valore)

	// --- operatori stringa ---
	case "contiene", "inizia_con", "finisce_con", "in", "non_in", "vuoto", "non_vuoto":
		return valutaStringa(r.StringMap[f.Campo], f.Op, f.Valore)

	// --- operatori booleano ---
	case "vero", "falso":
		return valutaBooleano(r.StringMap[f.Campo], f.Op)

	// --- operatori data ---
	case "prima_di", "dopo_di", "eta_max", "eta_min":
		return valutaData(r.TimeMap[f.Campo], f.Op, f.Valore)

	// --- operatori numerici (default) ---
	default:
		// tra: valore="min,max"
		if f.Op == "tra" {
			return valutaTra(r, f)
		}
		// Prova float → int → string con op standard
		if fv, ok := r.FloatMap[f.Campo]; ok {
			return cmpFloat(fv, f.Op, toFloat64(f.Valore))
		}
		if iv, ok := r.IntMap[f.Campo]; ok {
			return cmpInt(iv, f.Op, int(toFloat64(f.Valore)))
		}
		if sv, ok := r.StringMap[f.Campo]; ok {
			return cmpString(sv, f.Op, fmt.Sprintf("%v", f.Valore))
		}
		return false
	}
}

func valutaCF(codice, op string, valore any) bool {
	s := fmt.Sprintf("%v", valore)
	switch op {
	case "cf_eta_max":
		return cf.EtaAnni(codice) <= int(toFloat64(valore))
	case "cf_eta_min":
		return cf.EtaAnni(codice) >= int(toFloat64(valore))
	case "cf_anno_max":
		return cf.AnnoBirth(codice) <= int(toFloat64(valore))
	case "cf_anno_min":
		return cf.AnnoBirth(codice) >= int(toFloat64(valore))
	case "cf_sesso":
		return strings.EqualFold(cf.Sesso(codice), s)
	case "cf_comune":
		return strings.EqualFold(cf.ComuneNascita(codice), s)
	case "cf_valido":
		return cf.Valido(codice)
	}
	return false
}

func valutaStringa(val, op string, valore any) bool {
	s := strings.ToLower(fmt.Sprintf("%v", valore))
	v := strings.ToLower(val)
	switch op {
	case "contiene":
		return strings.Contains(v, s)
	case "inizia_con":
		return strings.HasPrefix(v, s)
	case "finisce_con":
		return strings.HasSuffix(v, s)
	case "in":
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == v {
				return true
			}
		}
		return false
	case "non_in":
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == v {
				return false
			}
		}
		return true
	case "vuoto":
		return val == ""
	case "non_vuoto":
		return val != ""
	}
	return false
}

func valutaBooleano(val, op string) bool {
	truthy := val == "true" || val == "1"
	switch op {
	case "vero":
		return truthy
	case "falso":
		return !truthy
	}
	return false
}

func valutaData(t time.Time, op string, valore any) bool {
	s := fmt.Sprintf("%v", valore)
	switch op {
	case "prima_di":
		ref := toTime(s)
		return t.Before(ref)
	case "dopo_di":
		ref := toTime(s)
		return t.After(ref)
	case "eta_max":
		n, _ := strconv.Atoi(s)
		anni := etaAnniDa(t)
		return anni <= n
	case "eta_min":
		n, _ := strconv.Atoi(s)
		anni := etaAnniDa(t)
		return anni >= n
	}
	return false
}

func etaAnniDa(t time.Time) int {
	oggi := time.Now()
	eta := oggi.Year() - t.Year()
	if oggi.Month() < t.Month() || (oggi.Month() == t.Month() && oggi.Day() < t.Day()) {
		eta--
	}
	return eta
}

func valutaTra(r *Record, f FiltroConfig) bool {
	s := fmt.Sprintf("%v", f.Valore)
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return false
	}
	min := toFloat64(strings.TrimSpace(parts[0]))
	max := toFloat64(strings.TrimSpace(parts[1]))
	if fv, ok := r.FloatMap[f.Campo]; ok {
		return fv >= min && fv <= max
	}
	if iv, ok := r.IntMap[f.Campo]; ok {
		return float64(iv) >= min && float64(iv) <= max
	}
	return false
}

// ToIstanza converte il Record in Istanza per compatibilità con i template.
// I campi non mappati restano al valore zero.
// I nomi dei campi corrispondono alle chiavi standard usate in EngineConfig.Mapping.
func (r *Record) ToIstanza() *Istanza {
	ist := &Istanza{
		ID:        r.AppID,
		Status:    r.AppStatus,
		ISEE:      r.FloatMap["isee"],
		Corrispettivo:       r.FloatMap["corrispettivo"],
		BeneficioRicevuto:   r.FloatMap["beneficio"],
		FiglioSelezionatoCF: r.StringMap["figlio_cf"],
		RichiedenteCF:       r.StringMap["richiedente_cf"],
		RichiedenteNome:     r.StringMap["richiedente_nome"],
		RichiedenteCognome:  r.StringMap["richiedente_cognome"],
		TipoRichiesta:       r.StringMap["tipo"],
		NumFigli:            r.IntMap["num_figli"],
		Annualita:           r.IntMap["annualita"],
		ProtocolNumber:      r.StringMap["protocollo"],
		IBAN:                r.StringMap["iban"],
		IBANIntestatario:    r.StringMap["iban_intestatario"],
		IBANCheck:           r.StringMap["iban_check"],
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
		return strings.EqualFold(a, b)
	case "!=":
		return !strings.EqualFold(a, b)
	}
	return false
}
