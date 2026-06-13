// Package generic implementa un engine di calcolo graduatoria configurabile via JSON.
// Supporta qualsiasi bando FSE+ con mapping campo → percorso JSON, filtri, ordinamento
// e tipologie con budget configurabile.
package generic

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
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
		records, err := estraiRecord(app, ecfg)
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

	// Rispetta ordine di priorità delle tipologie.
	sort.Slice(ecfg.Tipologie, func(i, j int) bool {
		return ecfg.Tipologie[i].Priorita < ecfg.Tipologie[j].Priorita
	})

	// Raggruppa ammissibili per tipologia.
	gruppiRecord := make(map[string][]*graduatoria.Record)
	for _, rec := range ammissibili {
		nome := tipologiaDiRecord(rec, ecfg.Tipologie)
		if nome == "" {
			escluse = append(escluse, graduatoria.RigaGraduatoria{
				Istanza:        rec.ToIstanza(),
				Ammessa:        false,
				NoteEsclusione: "tipologia non riconosciuta",
			})
			continue
		}
		gruppiRecord[nome] = append(gruppiRecord[nome], rec)
	}

	grad := &graduatoria.Graduatoria{Escluse: escluse}
	residuo := cfg.BudgetTotale

	for _, tip := range ecfg.Tipologie {
		lista := gruppiRecord[tip.Nome]

		ordinaRecord(lista, ecfg.Ordinamento)

		lista, dupl := deduplicaRecord(lista, ecfg.Deduplicazione)
		for _, d := range dupl {
			grad.Escluse = append(grad.Escluse, graduatoria.RigaGraduatoria{
				Istanza:        d.rec.ToIstanza(),
				Ammessa:        false,
				NoteEsclusione: "duplicato: stesso " + strings.Join(ecfg.Deduplicazione.Chiave, "+") + " già presente",
				OriginalID:     d.origID,
			})
		}

		budget := budgetTipologia(tip.Budget, cfg.BudgetTotale, residuo)
		righe, usato := assegnaRecord(lista, budget, ecfg.Rimborso)
		residuo -= usato

		grad.Gruppi = append(grad.Gruppi, &graduatoria.GraduatoriaGruppo{
			Nome:        tip.Nome,
			Righe:       righe,
			BudgetUsato: usato,
		})
	}

	return grad, nil
}

func (e *Engine) CSVHeaders() []string {
	return []string{
		"Posizione", "Ammessa", "AppID", "CF Richiedente", "CF Figlio",
		"ISEE", "Corrispettivo", "Beneficio Ricevuto", "Importo Rimborso",
		"Tipologia", "Annualita", "Note",
	}
}

func (e *Engine) CSVRecord(categoria string, r graduatoria.RigaGraduatoria) []string {
	ist := r.Istanza
	if ist == nil {
		return []string{fmt.Sprintf("%d", r.Posizione), categoria}
	}
	ammessa := "no"
	if r.Ammessa {
		ammessa = "si"
	}
	return []string{
		fmt.Sprintf("%d", r.Posizione),
		ammessa,
		ist.ID,
		ist.RichiedenteCF,
		ist.FiglioSelezionatoCF,
		fmt.Sprintf("%.2f", ist.ISEE),
		fmt.Sprintf("%.2f", ist.Corrispettivo),
		fmt.Sprintf("%.2f", ist.BeneficioRicevuto),
		fmt.Sprintf("%.2f", r.ImportoRimborso),
		ist.TipoRichiesta,
		fmt.Sprintf("%d", ist.Annualita),
		r.NoteEsclusione,
	}
}

// --- helpers ---

func estraiRecord(app opencity.Application, cfg graduatoria.EngineConfig) ([]*graduatoria.Record, error) {
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

	if cfg.Espansione == "" || len(expandMapping) == 0 {
		return []*graduatoria.Record{base}, nil
	}

	elements, err := extractor.ArrayElements(app.Data, cfg.Espansione)
	if err != nil || len(elements) == 0 {
		return []*graduatoria.Record{base}, nil
	}

	out := make([]*graduatoria.Record, 0, len(elements))
	for _, elem := range elements {
		rec := copyRecord(base)
		for nome, fm := range expandMapping {
			_ = popolaCampoRaw(rec, nome, fm, elem)
		}
		out = append(out, rec)
	}
	return out, nil
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
		return nil
	}
	return popolaCampoRaw(rec, nome, fm, data)
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

func applicaFiltri(rec *graduatoria.Record, filtri []graduatoria.FiltroConfig) (bool, string) {
	for _, f := range filtri {
		if !rec.PassaFiltro(f) {
			return false, fmt.Sprintf("filtro %s %s %v non soddisfatto", f.Campo, f.Op, f.Valore)
		}
	}
	return true, ""
}

func tipologiaDiRecord(rec *graduatoria.Record, tipologie []graduatoria.TipologiaConfig) string {
	for _, tip := range tipologie {
		if rec.StringMap[tip.Campo] == tip.Valore {
			return tip.Nome
		}
	}
	return ""
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

func budgetTipologia(cfg graduatoria.BudgetConfig, totale, residuo float64) float64 {
	switch cfg.Tipo {
	case "percentuale":
		return totale * cfg.Valore
	case "fisso":
		return cfg.Valore
	default: // "residuo"
		return residuo
	}
}

func assegnaRecord(lista []*graduatoria.Record, budget float64, rimborso graduatoria.RimborsoConfig) ([]graduatoria.RigaGraduatoria, float64) {
	residuo := budget
	var righe []graduatoria.RigaGraduatoria
	for pos, rec := range lista {
		riga := graduatoria.RigaGraduatoria{
			Posizione: pos + 1,
			Istanza:   rec.ToIstanza(),
			Ammessa:   true,
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
