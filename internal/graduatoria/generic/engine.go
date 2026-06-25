// Package generic implementa un engine di calcolo graduatoria configurabile via JSON.
// Supporta qualsiasi bando FSE+ con mapping campo → percorso JSON, filtri, ordinamento
// e tipologie con budget configurabile.
package generic

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/extractor"
	"opencity-gestionale/internal/opencity"
)

const EngineName = "generic"

func init() {
	graduatoria.Register(&Engine{})
}

type Engine struct{}

func (e *Engine) Name() string { return EngineName }

func (e *Engine) Calcola(apps []opencity.Application, cfg graduatoria.BandoConfig) (*graduatoria.Graduatoria, error) {
	var ecfg graduatoria.EngineConfig
	if len(cfg.ExtraJSON) > 0 && string(cfg.ExtraJSON) != "{}" {
		if err := json.Unmarshal(cfg.ExtraJSON, &ecfg); err != nil {
			return nil, fmt.Errorf("engine_config non valida: %w", err)
		}
	}

	var ammissibili []*graduatoria.Record
	var escluse []graduatoria.RigaGraduatoria

	for i := range apps {
		app := apps[i]
		if !passaFiltriIstanza(app, ecfg.Istanza) {
			continue
		}
		records, err := estraiRecord(app, ecfg, cfg.CampiExtra[app.ID])
		if err != nil {
			continue
		}
		for _, rec := range records {
			rec.DerivaCampi(ecfg.Rimborso)
			ok, motivo := applicaFiltri(rec, ecfg.Filtri)
			if !ok {
				escluse = append(escluse, graduatoria.RigaGraduatoria{
					Istanza:        rec.ToIstanza(),
					Ammessa:        false,
					NoteEsclusione: motivo,
				})
				continue
			}
			ammissibili = append(ammissibili, rec)
		}
	}

	grad := &graduatoria.Graduatoria{Escluse: escluse}

	switch ecfg.Modalita {
	case "ammissione", "lista_attesa":
		return buildGraduatoriaAmmissione(ammissibili, grad, ecfg), nil
	case "posti":
		return buildGraduatoriaPosti(ammissibili, grad, ecfg, cfg.Approvate), nil
	default: // "fondi" o non specificato
		return buildGraduatoriaFondi(ammissibili, grad, ecfg, cfg.BudgetTotale, cfg.Approvate), nil
	}
}

func buildGraduatoriaAmmissione(ammissibili []*graduatoria.Record, grad *graduatoria.Graduatoria, ecfg graduatoria.EngineConfig) *graduatoria.Graduatoria {
	ordinaRecord(ammissibili, ecfg.Ordinamento)
	ammissibili, dupl := deduplicaRecord(ammissibili, ecfg.Deduplicazione)
	appendDuplicati(grad, dupl, ecfg.Deduplicazione.Chiave)

	var righe []graduatoria.RigaGraduatoria
	for pos, rec := range ammissibili {
		righe = append(righe, graduatoria.RigaGraduatoria{
			Posizione: pos + 1,
			Istanza:   rec.ToIstanza(),
			Ammessa:   true,
		})
	}
	grad.Gruppi = append(grad.Gruppi, &graduatoria.GraduatoriaGruppo{
		Nome:  "ammessi",
		Righe: righe,
	})
	return grad
}

func buildGraduatoriaPosti(ammissibili []*graduatoria.Record, grad *graduatoria.Graduatoria, ecfg graduatoria.EngineConfig, approvate map[string]bool) *graduatoria.Graduatoria {
	sort.Slice(ecfg.Tipologie, func(i, j int) bool {
		return ecfg.Tipologie[i].Priorita < ecfg.Tipologie[j].Priorita
	})

	gruppiRecord := raggruppaPerTipologia(ammissibili, ecfg.Tipologie, grad)

	for _, tip := range ecfg.Tipologie {
		lista := gruppiRecord[tip.Nome]
		ordinaRecord(lista, ecfg.Ordinamento)
		lista, dupl := deduplicaRecord(lista, ecfg.Deduplicazione)
		appendDuplicati(grad, dupl, ecfg.Deduplicazione.Chiave)

		maxPosti := int(tip.Limite.Valore)
		var righe []graduatoria.RigaGraduatoria
		for pos, rec := range lista {
			riga := graduatoria.RigaGraduatoria{
				Posizione: pos + 1,
				Istanza:   rec.ToIstanza(),
				Ammessa:   maxPosti == 0 || pos < maxPosti,
			}
			if !riga.Ammessa {
				riga.NoteEsclusione = "posti esauriti"
			} else {
				motivi := rec.FlagMotivi(ecfg.Verifica)
				if len(motivi) > 0 && !approvate[rec.AppID] {
					riga.ConRiserva = true
					riga.MotiviRiserva = motivi
				}
			}
			righe = append(righe, riga)
		}
		grad.Gruppi = append(grad.Gruppi, &graduatoria.GraduatoriaGruppo{
			Nome:  tip.Nome,
			Righe: righe,
		})
	}
	return grad
}

func buildGraduatoriaFondi(ammissibili []*graduatoria.Record, grad *graduatoria.Graduatoria, ecfg graduatoria.EngineConfig, budgetTotale float64, approvate map[string]bool) *graduatoria.Graduatoria {
	sort.Slice(ecfg.Tipologie, func(i, j int) bool {
		return ecfg.Tipologie[i].Priorita < ecfg.Tipologie[j].Priorita
	})

	gruppiRecord := raggruppaPerTipologia(ammissibili, ecfg.Tipologie, grad)
	residuo := budgetTotale

	for _, tip := range ecfg.Tipologie {
		lista := gruppiRecord[tip.Nome]
		ordinaRecord(lista, ecfg.Ordinamento)
		lista, dupl := deduplicaRecord(lista, ecfg.Deduplicazione)
		appendDuplicati(grad, dupl, ecfg.Deduplicazione.Chiave)

		budget := limiteTipologia(tip.Limite, budgetTotale, residuo)
		righe, usato := assegnaRecord(lista, budget, ecfg.Rimborso, ecfg.Verifica, approvate)
		residuo -= usato

		grad.Gruppi = append(grad.Gruppi, &graduatoria.GraduatoriaGruppo{
			Nome:        tip.Nome,
			Righe:       righe,
			BudgetUsato: usato,
		})
	}
	return grad
}

func (e *Engine) CSVHeaders() []string {
	return []string{
		"Posizione", "Categoria", "Protocollo", "Data Invio",
		"Cognome", "Nome", "Email", "Tel",
		"CF Richiedente", "CF Figlio",
		"ISEE", "ISEE Valido Fino",
		"Corrispettivo", "Beneficio Ricevuto", "Importo Rimborso",
		"Tipologia", "Annualita", "IBAN",
		"Stato", "Note Esclusione", "Link",
	}
}

func (e *Engine) CSVRecord(categoria string, r graduatoria.RigaGraduatoria, baseURL string) []string {
	ist := r.Istanza
	if ist == nil {
		return []string{fmt.Sprintf("%d", r.Posizione), categoria}
	}
	link := ""
	if baseURL != "" && ist.ID != "" {
		link = baseURL + "/lang/it/operatori/" + ist.ID + "/detail"
	}
	return []string{
		fmt.Sprintf("%d", r.Posizione),
		categoria,
		ist.ProtocolNumber,
		ist.SubmittedAt,
		ist.RichiedenteCognome,
		ist.RichiedenteNome,
		ist.RichiedenteEmail,
		ist.RichiedenteTel,
		ist.RichiedenteCF,
		ist.FiglioSelezionatoCF,
		fmt.Sprintf("%.2f", ist.ISEE),
		ist.ISEEValidoFino,
		fmt.Sprintf("%.2f", ist.Corrispettivo),
		fmt.Sprintf("%.2f", ist.BeneficioRicevuto),
		fmt.Sprintf("%.2f", r.ImportoRimborso),
		ist.TipoRichiesta,
		fmt.Sprintf("%d", ist.Annualita),
		ist.IBAN,
		ist.Status,
		r.NoteEsclusione,
		link,
	}
}

// --- helpers ---

// EstraiRecords è la versione esportata di estraiRecord, usata dall'istruttoria.
func EstraiRecords(app opencity.Application, cfg graduatoria.EngineConfig) ([]*graduatoria.Record, error) {
	return estraiRecord(app, cfg, nil)
}

// EstraiRecordsConExtras è come EstraiRecords ma applica override locali ai campi.
func EstraiRecordsConExtras(app opencity.Application, cfg graduatoria.EngineConfig, extras map[string]string) ([]*graduatoria.Record, error) {
	return estraiRecord(app, cfg, extras)
}

func estraiRecord(app opencity.Application, cfg graduatoria.EngineConfig, extras map[string]string) ([]*graduatoria.Record, error) {
	baseMapping := make(map[string]graduatoria.FieldMapping)
	expandMapping := make(map[string]graduatoria.FieldMapping)
	for nome, fm := range cfg.Mapping {
		if fm.Expand {
			expandMapping[nome] = fm
		} else {
			baseMapping[nome] = fm
		}
	}

	base := graduatoria.NewRecord(app.ID, app.Status)
	for nome, fm := range baseMapping {
		_ = popolaCampo(base, nome, fm, app.Data, app)
	}
	applicaExtras(base, extras, cfg.Mapping)

	// Campi sistema sempre disponibili se non già configurati nel mapping.
	// Nota: run calcolate prima di questo fix non hanno questi campi in CampiMappati
	// senza ricalcolo.
	if _, mapped := cfg.Mapping["protocollo"]; !mapped {
		if base.StringMap["protocollo"] == "" {
			base.StringMap["protocollo"] = app.ProtocolNumber
		}
	}
	if _, mapped := cfg.Mapping["data_invio"]; !mapped {
		if base.StringMap["data_invio"] == "" {
			// Formatta come dd/mm/yyyy per coerenza con csvRecordDynamic e template
			if t, err := time.Parse(time.RFC3339, app.SubmittedAt); err == nil {
				base.StringMap["data_invio"] = t.Format("02/01/2006")
			} else {
				base.StringMap["data_invio"] = app.SubmittedAt
			}
		}
	}

	// Built-in: CF richiedente da path standard OpenCity se non già estratto dal mapping.
	if base.StringMap["richiedente_cf"] == "" {
		if cf, _ := extractor.Str(app.Data, "applicant.fiscal_code.fiscal_code"); cf != "" {
			base.StringMap["richiedente_cf"] = cf
		}
	}

	if cfg.Espansione == "" || len(expandMapping) == 0 {
		return []*graduatoria.Record{base}, nil
	}

	elements, err := extractor.ArrayElements(app.Data, cfg.Espansione)
	if err != nil {
		// Path invalido → fallback a base record (comportamento legacy)
		return []*graduatoria.Record{base}, nil
	}
	if len(elements) == 0 {
		// Espansione configurata ma nessun elemento matchante → nessun dato per questo bando
		return nil, nil
	}

	out := make([]*graduatoria.Record, 0, len(elements))
	for _, elem := range elements {
		rec := copyRecord(base)
		for nome, fm := range expandMapping {
			_ = popolaCampoRaw(rec, nome, fm, elem)
		}
		// Riapplica extras dopo l'expansion: popolaCampoRaw sovrascrive i campi expand
		// con i valori API, cancellando gli override. Riapplicarli qui li ripristina.
		applicaExtras(rec, extras, cfg.Mapping)
		out = append(out, rec)
	}
	return out, nil
}

// applicaExtras sovrascrive i campi del record con i valori di override locali.
func applicaExtras(rec *graduatoria.Record, extras map[string]string, mapping map[string]graduatoria.FieldMapping) {
	for campo, valore := range extras {
		fm, ok := mapping[campo]
		if !ok {
			rec.StringMap[campo] = valore
			continue
		}
		switch fm.Tipo {
		case "float":
			normalized := strings.ReplaceAll(strings.TrimSpace(valore), ",", ".")
			if f, err := strconv.ParseFloat(normalized, 64); err == nil {
				rec.FloatMap[campo] = f
			}
		case "int":
			normalized := strings.ReplaceAll(strings.TrimSpace(valore), ",", ".")
			if i, err := strconv.Atoi(normalized); err != nil {
				if f, err := strconv.ParseFloat(normalized, 64); err == nil {
					rec.IntMap[campo] = int(f)
				}
			} else {
				rec.IntMap[campo] = i
			}
		default:
			rec.StringMap[campo] = valore
		}
		if fm.VerificaPath != "" && valore != "" {
			rec.StringMap["__cert_"+campo] = "override_manuale"
		}
	}
}

var appTimeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"}

func popolaCampo(rec *graduatoria.Record, nome string, fm graduatoria.FieldMapping, data json.RawMessage, app opencity.Application) error {
	if strings.HasPrefix(fm.Path, "$app:") {
		field := strings.TrimPrefix(fm.Path, "$app:")
		val, err := extractor.AppField(app, field)
		if err != nil {
			return err
		}
		if fm.Tipo == "time" {
			for _, layout := range appTimeLayouts {
				if t, err2 := time.Parse(layout, val); err2 == nil {
					rec.TimeMap[nome] = t
					return nil
				}
			}
			return fmt.Errorf("impossibile parsare %q come time", val)
		}
		rec.StringMap[nome] = val
	} else {
		if err := popolaCampoRaw(rec, nome, fm, data); err != nil {
			return err
		}
	}
	// Se il campo ha VerificaPath, controlla la condizione e salva __cert_{nome}
	if fm.VerificaPath != "" {
		sig, _ := extractor.Str(data, fm.VerificaPath)
		certified := evaluateVerificaCondition(sig, fm.VerificaOp, fm.VerificaVal)
		if certified {
			rec.StringMap["__cert_"+nome] = sig
		} else {
			rec.StringMap["__cert_"+nome] = ""
		}
	}
	return nil
}

func popolaCampoRaw(rec *graduatoria.Record, nome string, fm graduatoria.FieldMapping, data json.RawMessage) error {
	switch fm.Tipo {
	case "float":
		v, err := extractor.Float(data, fm.Path)
		if err != nil {
			return err
		}
		rec.FloatMap[nome] = v
	case "int":
		v, err := extractor.Float(data, fm.Path)
		if err != nil {
			return err
		}
		rec.IntMap[nome] = int(v)
	case "count":
		v, err := extractor.Count(data, fm.Path)
		if err != nil {
			return err
		}
		rec.IntMap[nome] = v
	case "time":
		v, err := extractor.Time(data, fm.Path)
		if err != nil {
			return err
		}
		rec.TimeMap[nome] = v
	default:
		v, err := extractor.Str(data, fm.Path)
		if err != nil {
			return err
		}
		rec.StringMap[nome] = v
	}
	return nil
}

func copyRecord(src *graduatoria.Record) *graduatoria.Record {
	dst := graduatoria.NewRecord(src.AppID, src.AppStatus)
	for k, v := range src.FloatMap {
		dst.FloatMap[k] = v
	}
	for k, v := range src.StringMap {
		dst.StringMap[k] = v
	}
	for k, v := range src.IntMap {
		dst.IntMap[k] = v
	}
	for k, v := range src.TimeMap {
		dst.TimeMap[k] = v
	}
	return dst
}

// ApplicaFiltri è la versione esportata di applicaFiltri, usata dall'istruttoria.
func ApplicaFiltri(rec *graduatoria.Record, filtri []graduatoria.FiltroConfig) (bool, string) {
	return applicaFiltri(rec, filtri)
}

func applicaFiltri(rec *graduatoria.Record, filtri []graduatoria.FiltroConfig) (bool, string) {
	// Separa filtri standalone (gruppo 0) da gruppi OR (gruppo > 0).
	gruppi := map[int][]graduatoria.FiltroConfig{}
	for _, f := range filtri {
		gruppi[f.Gruppo] = append(gruppi[f.Gruppo], f)
	}
	for gid, lista := range gruppi {
		if gid == 0 {
			// AND: tutti devono passare
			for _, f := range lista {
				if !rec.PassaFiltro(f) {
					return false, fmt.Sprintf("filtro %s %s %v non soddisfatto", f.Campo, f.Op, f.Valore)
				}
			}
		} else {
			// OR: almeno uno deve passare
			ok := false
			for _, f := range lista {
				if rec.PassaFiltro(f) {
					ok = true
					break
				}
			}
			if !ok {
				campi := make([]string, 0, len(lista))
				for _, f := range lista {
					campi = append(campi, fmt.Sprintf("%s %s %v", f.Campo, f.Op, f.Valore))
				}
				return false, fmt.Sprintf("gruppo OR %d: nessuna condizione soddisfatta (%s)", gid, strings.Join(campi, " | "))
			}
		}
	}
	return true, ""
}

// PassaFiltriIstanza verifica stato e date presentazione prima dell'estrazione campi.
// Esportata per uso in istruttoria scansiona.
func PassaFiltriIstanza(app opencity.Application, cfg graduatoria.FiltriIstanzaConfig) bool {
	return passaFiltriIstanza(app, cfg)
}

// TipologiaDiRecord ritorna il nome della tipologia che matcha il record, o "" se nessuna.
// Esportata per uso in istruttoria scansiona (coerenza con calcolo).
func TipologiaDiRecord(rec *graduatoria.Record, tipologie []graduatoria.TipologiaConfig) string {
	nome, _ := tipologiaDiRecord(rec, tipologie)
	return nome
}

func passaFiltriIstanza(app opencity.Application, cfg graduatoria.FiltriIstanzaConfig) bool {
	if len(cfg.StatiAmmessi) > 0 {
		ok := false
		for _, s := range cfg.StatiAmmessi {
			if s == app.Status {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	var layouts = []string{"2006-01-02T15:04:05Z07:00", time.RFC3339, "2006-01-02"}
	parseDate := func(s string) (time.Time, bool) {
		for _, l := range layouts {
			if t, err := time.Parse(l, s); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}
	if cfg.DataMassima != "" {
		if tmax, ok := parseDate(cfg.DataMassima); ok {
			if sub, ok2 := parseDate(app.SubmittedAt); ok2 && sub.After(tmax) {
				return false
			}
		}
	}
	if cfg.DataMinima != "" {
		if tmin, ok := parseDate(cfg.DataMinima); ok {
			if sub, ok2 := parseDate(app.SubmittedAt); ok2 && sub.Before(tmin) {
				return false
			}
		}
	}
	return true
}

func raggruppaPerTipologia(ammissibili []*graduatoria.Record, tipologie []graduatoria.TipologiaConfig, grad *graduatoria.Graduatoria) map[string][]*graduatoria.Record {
	gruppi := make(map[string][]*graduatoria.Record)
	for _, rec := range ammissibili {
		nome, motivo := tipologiaDiRecord(rec, tipologie)
		if nome == "" {
			grad.Escluse = append(grad.Escluse, graduatoria.RigaGraduatoria{
				Istanza:        rec.ToIstanza(),
				Ammessa:        false,
				NoteEsclusione: motivo,
			})
			continue
		}
		gruppi[nome] = append(gruppi[nome], rec)
	}
	return gruppi
}

func tipologiaDiRecord(rec *graduatoria.Record, tipologie []graduatoria.TipologiaConfig) (string, string) {
	for _, tip := range tipologie {
		if tip.Campo == "" || rec.StringMap[tip.Campo] == tip.Valore {
			return tip.Nome, ""
		}
	}
	if len(tipologie) == 0 {
		return "", "nessuna tipologia configurata"
	}
	// Costruisce motivo dettagliato
	seen := map[string]bool{}
	var nonTrovati, nonMatchati []string
	for _, tip := range tipologie {
		if tip.Campo == "" || seen[tip.Campo] {
			continue
		}
		seen[tip.Campo] = true
		trovato := rec.StringMap[tip.Campo]
		if trovato == "" {
			nonTrovati = append(nonTrovati, tip.Campo)
		} else {
			nonMatchati = append(nonMatchati, fmt.Sprintf("%s=%q", tip.Campo, trovato))
		}
	}
	var motivo string
	if len(nonTrovati) > 0 && len(nonMatchati) == 0 {
		motivo = "campo non presente nel record (" + strings.Join(nonTrovati, ", ") + ") — verificare path/condizione mapping"
	} else if len(nonMatchati) > 0 {
		motivo = "nessuna tipologia per " + strings.Join(nonMatchati, ", ")
	} else {
		motivo = "tipologia non riconosciuta"
	}
	return "", motivo
}

func appendDuplicati(grad *graduatoria.Graduatoria, dupl []recordDuplicato, chiave []string) {
	for _, d := range dupl {
		grad.Escluse = append(grad.Escluse, graduatoria.RigaGraduatoria{
			Istanza:        d.rec.ToIstanza(),
			Ammessa:        false,
			NoteEsclusione: "duplicato: stesso " + strings.Join(chiave, "+") + " già presente",
			OriginalID:     d.origID,
		})
	}
}

func ordinaRecord(lista []*graduatoria.Record, ordini []graduatoria.OrdineConfig) {
	sort.SliceStable(lista, func(i, j int) bool {
		for _, o := range ordini {
			a, b := lista[i], lista[j]
			if af, bf := a.FloatMap[o.Campo], b.FloatMap[o.Campo]; af != bf {
				if o.Dir == "desc" {
					return af > bf
				}
				return af < bf
			}
			if ai, bi := a.IntMap[o.Campo], b.IntMap[o.Campo]; ai != bi {
				if o.Dir == "desc" {
					return ai > bi
				}
				return ai < bi
			}
			if at, bt := a.TimeMap[o.Campo], b.TimeMap[o.Campo]; !at.Equal(bt) {
				if o.Dir == "desc" {
					return at.After(bt)
				}
				return at.Before(bt)
			}
		}
		return false
	})
}

type recordDuplicato struct {
	rec    *graduatoria.Record
	origID string
}

func deduplicaRecord(lista []*graduatoria.Record, cfg graduatoria.DedupConfig) ([]*graduatoria.Record, []recordDuplicato) {
	if !cfg.Attiva || len(cfg.Chiave) == 0 {
		return lista, nil
	}
	seen := make(map[string]string)
	var unici []*graduatoria.Record
	var dupl []recordDuplicato
	for _, rec := range lista {
		k := rec.ChiaveDedup(cfg.Chiave)
		if origID, ok := seen[k]; ok {
			dupl = append(dupl, recordDuplicato{rec: rec, origID: origID})
		} else {
			seen[k] = rec.AppID
			unici = append(unici, rec)
		}
	}
	return unici, dupl
}

func limiteTipologia(cfg graduatoria.LimiteConfig, totale, residuo float64) float64 {
	switch cfg.Tipo {
	case "percentuale":
		return totale * cfg.Valore
	case "fisso", "budget":
		return cfg.Valore
	default: // "residuo" o non specificato
		return residuo
	}
}

func assegnaRecord(lista []*graduatoria.Record, budget float64, rimborso graduatoria.RimborsoConfig, verifica graduatoria.VerificaConfig, approvate map[string]bool) ([]graduatoria.RigaGraduatoria, float64) {
	residuo := budget
	var righe []graduatoria.RigaGraduatoria
	for pos, rec := range lista {
		riga := graduatoria.RigaGraduatoria{
			Posizione: pos + 1,
			Istanza:   rec.ToIstanza(),
			Ammessa:   true,
		}
		motivi := rec.FlagMotivi(verifica)
		if len(motivi) > 0 && !approvate[rec.AppID] {
			riga.ConRiserva = true
			riga.MotiviRiserva = motivi
		}
		netto := rec.FloatMap["corrispettivo_netto"]
		if rimborso.Tipo == "lordo" {
			netto = rec.FloatMap[rimborso.CampoLordo]
		}
		if residuo <= 0 {
			riga.Ammessa = false
			riga.NoteEsclusione = "fondi esauriti"
			righe = append(righe, riga)
			continue
		}
		importo := math.Min(netto, residuo)
		riga.ImportoRimborso = math.Round(importo*100) / 100
		residuo -= importo
		righe = append(righe, riga)
	}
	return righe, budget - residuo
}

// evaluateVerificaCondition valuta se un valore soddisfa la condizione di verifica.
// op vuoto o "non_vuoto": verificato se val != "" (comportamento default).
func evaluateVerificaCondition(val, op, expected string) bool {
	switch op {
	case "==":
		return val == expected
	case "!=":
		return val != expected
	default: // "non_vuoto" o ""
		return val != ""
	}
}
