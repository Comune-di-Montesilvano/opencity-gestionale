package generic_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/graduatoria/generic"
	"opencity-gestionale/internal/opencity"
)

func makeApp(id int, dataJSON string) opencity.Application {
	return opencity.Application{
		ID:     fmt.Sprintf("app-%d", id),
		Status: "4000",
		Data:   json.RawMessage(dataJSON),
	}
}

func makeAppSubmitted(id int, dataJSON, submittedAt string) opencity.Application {
	app := makeApp(id, dataJSON)
	app.SubmittedAt = submittedAt
	return app
}

func configJSON(ecfg graduatoria.EngineConfig) json.RawMessage {
	b, _ := json.Marshal(ecfg)
	return b
}

func TestEngineAmmissione(t *testing.T) {
	e := &generic.Engine{}
	apps := []opencity.Application{
		makeApp(1, `{"isee": 10000}`),
		makeApp(2, `{"isee": 30000}`),
		makeApp(3, `{"isee": 50000}`), // sopra soglia → esclusa
	}
	cfg := graduatoria.BandoConfig{
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping: map[string]graduatoria.FieldMapping{
				"isee": {Path: "isee", Tipo: "float"},
			},
			Filtri: []graduatoria.FiltroConfig{
				{Campo: "isee", Op: "<=", Valore: 40000},
			},
			Modalita: "ammissione",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if len(grad.Gruppi) != 1 {
		t.Fatalf("Gruppi: got %d, want 1", len(grad.Gruppi))
	}
	if grad.Gruppi[0].Nome != "ammessi" {
		t.Errorf("Gruppi[0].Nome = %q, want ammessi", grad.Gruppi[0].Nome)
	}
	if got := grad.TotaleAmmesse(); got != 2 {
		t.Errorf("TotaleAmmesse() = %d, want 2", got)
	}
	if len(grad.Escluse) != 1 {
		t.Errorf("Escluse: got %d, want 1", len(grad.Escluse))
	}
	// tutti gli ammessi hanno ImportoRimborso=0
	for _, r := range grad.Gruppi[0].Righe {
		if r.ImportoRimborso != 0 {
			t.Errorf("ammissione: ImportoRimborso = %.2f, want 0", r.ImportoRimborso)
		}
		if !r.Ammessa {
			t.Errorf("ammissione: Ammessa = false, want true")
		}
	}
}

func TestEngineListaAttesa(t *testing.T) {
	e := &generic.Engine{}
	// tutti passano i filtri, devono essere ordinati per data_presentazione asc
	apps := []opencity.Application{
		makeAppSubmitted(1, `{}`, "2026-03-01T10:00:00Z"),
		makeAppSubmitted(2, `{}`, "2026-01-01T10:00:00Z"), // prima → posizione 1
		makeAppSubmitted(3, `{}`, "2026-02-01T10:00:00Z"),
	}
	cfg := graduatoria.BandoConfig{
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping: map[string]graduatoria.FieldMapping{
				"data_presentazione": {Path: "$app:submitted_at", Tipo: "time"},
			},
			Ordinamento: []graduatoria.OrdineConfig{
				{Campo: "data_presentazione", Dir: "asc"},
			},
			Modalita: "lista_attesa",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if got := grad.TotaleAmmesse(); got != 3 {
		t.Errorf("TotaleAmmesse() = %d, want 3", got)
	}
	if len(grad.Escluse) != 0 {
		t.Errorf("Escluse: got %d, want 0", len(grad.Escluse))
	}
	righe := grad.Gruppi[0].Righe
	if len(righe) != 3 {
		t.Fatalf("Righe: got %d, want 3", len(righe))
	}
	// ordine atteso: app-2 (gen), app-3 (feb), app-1 (mar)
	wantOrder := []string{"app-2", "app-3", "app-1"}
	for i, r := range righe {
		if r.Istanza.ID != wantOrder[i] {
			t.Errorf("righe[%d].ID = %q, want %q", i, r.Istanza.ID, wantOrder[i])
		}
		if r.Posizione != i+1 {
			t.Errorf("righe[%d].Posizione = %d, want %d", i, r.Posizione, i+1)
		}
	}
}

func TestEnginePosti(t *testing.T) {
	e := &generic.Engine{}
	apps := []opencity.Application{
		makeApp(1, `{"isee": 10000}`),
		makeApp(2, `{"isee": 20000}`),
		makeApp(3, `{"isee": 30000}`),
		makeApp(4, `{"isee": 40000}`),
		makeApp(5, `{"isee": 50000}`),
	}
	cfg := graduatoria.BandoConfig{
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping: map[string]graduatoria.FieldMapping{
				"isee": {Path: "isee", Tipo: "float"},
			},
			Ordinamento: []graduatoria.OrdineConfig{
				{Campo: "isee", Dir: "asc"},
			},
			Tipologie: []graduatoria.TipologiaConfig{
				{
					Nome:     "tutti",
					Priorita: 1,
					Limite:   graduatoria.LimiteConfig{Tipo: "posti", Valore: 2},
				},
			},
			Modalita: "posti",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if got := grad.TotaleAmmesse(); got != 2 {
		t.Errorf("TotaleAmmesse() = %d, want 2", got)
	}
	righe := grad.Gruppi[0].Righe
	if len(righe) != 5 {
		t.Fatalf("Righe: got %d, want 5", len(righe))
	}
	// prime 2 ammesse (isee 10k, 20k), resto escluse
	for i, r := range righe {
		wantAmmessa := i < 2
		if r.Ammessa != wantAmmessa {
			t.Errorf("righe[%d] Ammessa = %v, want %v", i, r.Ammessa, wantAmmessa)
		}
		if !r.Ammessa && r.NoteEsclusione != "posti esauriti" {
			t.Errorf("righe[%d] NoteEsclusione = %q, want posti esauriti", i, r.NoteEsclusione)
		}
	}
}

func TestEngineFondi(t *testing.T) {
	e := &generic.Engine{}
	// budget=90, rimborso lordo €30/app → prime 3 ammesse, 4a e 5a escluse
	apps := []opencity.Application{
		makeApp(1, `{"isee": 10000, "importo": 30}`),
		makeApp(2, `{"isee": 20000, "importo": 30}`),
		makeApp(3, `{"isee": 30000, "importo": 30}`),
		makeApp(4, `{"isee": 40000, "importo": 30}`),
		makeApp(5, `{"isee": 50000, "importo": 30}`),
	}
	cfg := graduatoria.BandoConfig{
		BudgetTotale: 90,
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping: map[string]graduatoria.FieldMapping{
				"isee":    {Path: "isee", Tipo: "float"},
				"importo": {Path: "importo", Tipo: "float"},
			},
			Ordinamento: []graduatoria.OrdineConfig{
				{Campo: "isee", Dir: "asc"},
			},
			Tipologie: []graduatoria.TipologiaConfig{
				{
					Nome:     "tutti",
					Priorita: 1,
					Limite:   graduatoria.LimiteConfig{Tipo: "residuo"},
				},
			},
			Rimborso: graduatoria.RimborsoConfig{
				Tipo:       "lordo",
				CampoLordo: "importo",
			},
			Modalita: "fondi",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if got := grad.TotaleAmmesse(); got != 3 {
		t.Errorf("TotaleAmmesse() = %d, want 3", got)
	}
	righe := grad.Gruppi[0].Righe
	if len(righe) != 5 {
		t.Fatalf("Righe: got %d, want 5", len(righe))
	}
	// prime 3 ammesse con €30, resto "fondi esauriti"
	for i, r := range righe {
		wantAmmessa := i < 3
		if r.Ammessa != wantAmmessa {
			t.Errorf("righe[%d] Ammessa = %v, want %v", i, r.Ammessa, wantAmmessa)
		}
		if r.Ammessa && r.ImportoRimborso != 30 {
			t.Errorf("righe[%d] ImportoRimborso = %.2f, want 30", i, r.ImportoRimborso)
		}
		if !r.Ammessa && r.NoteEsclusione != "fondi esauriti" {
			t.Errorf("righe[%d] NoteEsclusione = %q, want fondi esauriti", i, r.NoteEsclusione)
		}
	}
	if budgetUsato := grad.TotaleBudgetUsato(); budgetUsato != 90 {
		t.Errorf("TotaleBudgetUsato() = %.2f, want 90", budgetUsato)
	}
}

func TestEngineDeduplicazione(t *testing.T) {
	e := &generic.Engine{}
	// 3 app con 2 stessi CF figlio → 1 duplicato escluso
	apps := []opencity.Application{
		makeApp(1, `{"isee": 10000, "cf_figlio": "RSSMRA80A01H501U"}`),
		makeApp(2, `{"isee": 20000, "cf_figlio": "RSSMRA80A01H501U"}`), // duplicato
		makeApp(3, `{"isee": 30000, "cf_figlio": "BNCMRA99T61H501E"}`),
	}
	cfg := graduatoria.BandoConfig{
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping: map[string]graduatoria.FieldMapping{
				"isee":      {Path: "isee", Tipo: "float"},
				"cf_figlio": {Path: "cf_figlio", Tipo: "string"},
			},
			Ordinamento: []graduatoria.OrdineConfig{
				{Campo: "isee", Dir: "asc"},
			},
			Deduplicazione: graduatoria.DedupConfig{
				Attiva: true,
				Chiave: []string{"cf_figlio"},
			},
			Modalita: "ammissione",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if got := grad.TotaleAmmesse(); got != 2 {
		t.Errorf("TotaleAmmesse() = %d, want 2", got)
	}
	if len(grad.Escluse) != 1 {
		t.Errorf("Escluse: got %d, want 1 (duplicato)", len(grad.Escluse))
	}
}

func TestEngineDefaultExtraction(t *testing.T) {
	e := &generic.Engine{}
	appJSON := `{
		"applicant": {
			"completename": {
				"name": "Mario",
				"surname": "Rossi"
			},
			"fiscal_code": {
				"fiscal_code": "RSSMRA80A01H501U"
			},
			"email_address": "mario.rossi@example.com",
			"cell_number": "1234567890",
			"address": {
				"address": "Via Roma",
				"house_number": "10",
				"municipality": "Montesilvano",
				"postal_code": "65015",
				"county": "PE"
			}
		},
		"ordinary_economic_situation_indicator": {
			"isee": 15000,
			"valid_until": "31/12/2026",
			"dsu_protocol_number": "INPS-DSU-2026-123456"
		}
	}`
	apps := []opencity.Application{
		makeApp(1, appJSON),
	}
	cfg := graduatoria.BandoConfig{
		ExtraJSON: configJSON(graduatoria.EngineConfig{
			Mapping:  map[string]graduatoria.FieldMapping{},
			Modalita: "ammissione",
		}),
	}
	grad, err := e.Calcola(apps, cfg)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if len(grad.Gruppi) != 1 || len(grad.Gruppi[0].Righe) != 1 {
		t.Fatalf("Expected 1 row, got %v", grad)
	}
	ist := grad.Gruppi[0].Righe[0].Istanza
	if ist == nil {
		t.Fatalf("Expected Istanza to be populated")
	}
	if ist.RichiedenteNome != "Mario" {
		t.Errorf("Expected RichiedenteNome = Mario, got %q", ist.RichiedenteNome)
	}
	if ist.RichiedenteCognome != "Rossi" {
		t.Errorf("Expected RichiedenteCognome = Rossi, got %q", ist.RichiedenteCognome)
	}
	if ist.RichiedenteCF != "RSSMRA80A01H501U" {
		t.Errorf("Expected RichiedenteCF = RSSMRA80A01H501U, got %q", ist.RichiedenteCF)
	}
	if ist.RichiedenteEmail != "mario.rossi@example.com" {
		t.Errorf("Expected RichiedenteEmail = mario.rossi@example.com, got %q", ist.RichiedenteEmail)
	}
	if ist.RichiedenteTel != "1234567890" {
		t.Errorf("Expected RichiedenteTel = 1234567890, got %q", ist.RichiedenteTel)
	}
	if ist.Indirizzo != "Via Roma" {
		t.Errorf("Expected Indirizzo = Via Roma, got %q", ist.Indirizzo)
	}
	if ist.Civico != "10" {
		t.Errorf("Expected Civico = 10, got %q", ist.Civico)
	}
	if ist.Comune != "Montesilvano" {
		t.Errorf("Expected Comune = Montesilvano, got %q", ist.Comune)
	}
	if ist.CAP != "65015" {
		t.Errorf("Expected CAP = 65015, got %q", ist.CAP)
	}
	if ist.Provincia != "PE" {
		t.Errorf("Expected Provincia = PE, got %q", ist.Provincia)
	}
	if ist.ISEEValidoFino != "31/12/2026" {
		t.Errorf("Expected ISEEValidoFino = 31/12/2026, got %q", ist.ISEEValidoFino)
	}
	if ist.ISEEDSUProtocollo != "INPS-DSU-2026-123456" {
		t.Errorf("Expected ISEEDSUProtocollo = INPS-DSU-2026-123456, got %q", ist.ISEEDSUProtocollo)
	}
}
