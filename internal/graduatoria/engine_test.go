package graduatoria_test

import (
	"encoding/json"
	"testing"

	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/opencity"
)

// fixture helpers

type anniFixture struct {
	Tipo          string
	Annualita     int
	Corrispettivo float64
	Beneficio     float64
}

func makeApp(id, status string, isee float64, figlioCF string, anni []anniFixture) opencity.Application {
	type anniEntry struct {
		Annualita1               int     `json:"annualita1"`
		Corrispettivo            float64 `json:"corrispettivo"`
		TipoRichiesta            string  `json:"tiporichiesta"`
		ImportoBeneficioRicevuto float64 `json:"importoDelBeneficioRicevuto"`
	}
	type iseeWrapper struct {
		Value float64 `json:"isee"`
	}
	type fixture struct {
		ISEE        iseeWrapper  `json:"ordinary_economic_situation_indicator"`
		Anni        []anniEntry  `json:"anni"`
		SelectChild string       `json:"select_child"`
	}

	var d fixture
	d.ISEE.Value = isee
	d.SelectChild = figlioCF
	for _, a := range anni {
		d.Anni = append(d.Anni, anniEntry{
			Annualita1:               a.Annualita,
			Corrispettivo:            a.Corrispettivo,
			TipoRichiesta:            a.Tipo,
			ImportoBeneficioRicevuto: a.Beneficio,
		})
	}

	data, _ := json.Marshal(d)
	return opencity.Application{ID: id, Status: status, Data: data}
}

func retta(id string, isee, corrispettivo float64) opencity.Application {
	return makeApp(id, "4000", isee, "CF"+id, []anniFixture{
		{Tipo: "rette", Annualita: 20242025, Corrispettivo: corrispettivo},
	})
}

func mensa(id string, isee, corrispettivo float64) opencity.Application {
	return makeApp(id, "4000", isee, "CF"+id, []anniFixture{
		{Tipo: "mensa", Annualita: 20242025, Corrispettivo: corrispettivo},
	})
}

var cfg = graduatoria.BandoConfig{BudgetTotale: 1000, ISEEMassimo: 40000}

func calcola(t *testing.T, apps []opencity.Application) *graduatoria.Graduatoria {
	t.Helper()
	g, err := graduatoria.CalcolaConConfig(apps, cfg)
	if err != nil {
		t.Fatalf("CalcolaConConfig: %v", err)
	}
	return g
}

func annoFor(t *testing.T, g *graduatoria.Graduatoria, anno int) *graduatoria.GraduatoriaAnnualita {
	t.Helper()
	for _, pa := range g.PerAnno {
		if pa.Annualita == anno {
			return pa
		}
	}
	t.Fatalf("annualita %d non trovata", anno)
	return nil
}

// --- test cases ---

func TestCalcola_Empty(t *testing.T) {
	g := calcola(t, nil)
	if len(g.PerAnno) == 0 {
		return
	}
	for _, pa := range g.PerAnno {
		if len(pa.Rette)+len(pa.Mense) > 0 {
			t.Errorf("atteso vuoto, trovate %d righe per anno %d", len(pa.Rette)+len(pa.Mense), pa.Annualita)
		}
	}
}

func TestCalcola_Ammessa(t *testing.T) {
	g := calcola(t, []opencity.Application{retta("R1", 15000, 300)})
	pa := annoFor(t, g, 20242025)
	if len(pa.Rette) != 1 {
		t.Fatalf("attesa 1 retta, trovate %d", len(pa.Rette))
	}
	if !pa.Rette[0].Ammessa {
		t.Error("retta deve essere ammessa")
	}
}

func TestCalcola_Ritirata(t *testing.T) {
	app := makeApp("R1", "20000", 15000, "CFRITIRATA", []anniFixture{
		{Tipo: "rette", Annualita: 20242025, Corrispettivo: 300},
	})
	g := calcola(t, []opencity.Application{app})
	if len(g.Escluse) != 1 {
		t.Fatalf("attesa 1 esclusa, trovate %d", len(g.Escluse))
	}
	if g.Escluse[0].NoteEsclusione != "istanza ritirata" {
		t.Errorf("nota errata: %q", g.Escluse[0].NoteEsclusione)
	}
}

func TestCalcola_ISEESuperiore(t *testing.T) {
	g := calcola(t, []opencity.Application{retta("R1", 50000, 300)})
	if len(g.Escluse) != 1 {
		t.Fatalf("attesa 1 esclusa per ISEE > max, trovate %d", len(g.Escluse))
	}
}

func TestCalcola_CorrispettivoNettoZero(t *testing.T) {
	app := makeApp("R1", "4000", 15000, "CF1", []anniFixture{
		{Tipo: "rette", Annualita: 20242025, Corrispettivo: 300, Beneficio: 300},
	})
	g := calcola(t, []opencity.Application{app})
	if len(g.Escluse) != 1 {
		t.Fatalf("attesa 1 esclusa per netto=0, trovate %d", len(g.Escluse))
	}
}

func TestCalcola_Duplicato(t *testing.T) {
	// stessa chiave (CF figlio, annualita, tipo) → solo il primo passa
	app1 := makeApp("R1", "4000", 10000, "CFDUP", []anniFixture{
		{Tipo: "rette", Annualita: 20242025, Corrispettivo: 300},
	})
	app2 := makeApp("R2", "4000", 20000, "CFDUP", []anniFixture{
		{Tipo: "rette", Annualita: 20242025, Corrispettivo: 300},
	})
	g := calcola(t, []opencity.Application{app1, app2})
	pa := annoFor(t, g, 20242025)
	if len(pa.Rette) != 1 {
		t.Fatalf("attesa 1 retta (deduplicata), trovate %d", len(pa.Rette))
	}
	var dupInEscluse bool
	for _, e := range g.Escluse {
		if e.Istanza != nil && e.Istanza.ID == "R2" {
			dupInEscluse = true
		}
	}
	if !dupInEscluse {
		t.Error("R2 (duplicato) deve essere in escluse")
	}
}

func TestCalcola_OrdineISEE(t *testing.T) {
	// ISEE crescente: R3 primo, R1 ultimo
	apps := []opencity.Application{
		retta("R1", 30000, 100),
		retta("R2", 20000, 100),
		retta("R3", 10000, 100),
	}
	g := calcola(t, apps)
	pa := annoFor(t, g, 20242025)
	if len(pa.Rette) != 3 {
		t.Fatalf("attese 3 rette, trovate %d", len(pa.Rette))
	}
	if pa.Rette[0].Istanza.ID != "R3" {
		t.Errorf("posizione 1 attesa R3 (ISEE 10k), trovato %s", pa.Rette[0].Istanza.ID)
	}
	if pa.Rette[2].Istanza.ID != "R1" {
		t.Errorf("posizione 3 attesa R1 (ISEE 30k), trovato %s", pa.Rette[2].Istanza.ID)
	}
}

func TestCalcola_PrioritaRetteSuMense(t *testing.T) {
	// budget = 100, retta costa 80, mensa costa 80 → retta ammessa, mensa fuori fondi
	smallCfg := graduatoria.BandoConfig{BudgetTotale: 160, ISEEMassimo: 40000} // 80 per annualità
	apps := []opencity.Application{
		retta("R1", 10000, 80),
		mensa("M1", 10000, 80),
	}
	g, err := graduatoria.CalcolaConConfig(apps, smallCfg)
	if err != nil {
		t.Fatal(err)
	}
	pa := annoFor(t, g, 20242025)
	if len(pa.Rette) == 0 || !pa.Rette[0].Ammessa {
		t.Error("retta deve essere ammessa prima della mensa")
	}
	if len(pa.Mense) == 0 || pa.Mense[0].Ammessa {
		t.Error("mensa deve essere fuori fondi se budget esaurito da rette")
	}
}

func TestCalcola_BudgetEsaurito(t *testing.T) {
	// budget 100 per annualità, 3 mense da 50 → 2 ammesse, 1 fuori fondi
	smallCfg := graduatoria.BandoConfig{BudgetTotale: 200, ISEEMassimo: 40000}
	apps := []opencity.Application{
		mensa("M1", 10000, 50),
		mensa("M2", 20000, 50),
		mensa("M3", 30000, 50),
	}
	g, err := graduatoria.CalcolaConConfig(apps, smallCfg)
	if err != nil {
		t.Fatal(err)
	}
	pa := annoFor(t, g, 20242025)
	ammesse := 0
	for _, r := range pa.Mense {
		if r.Ammessa {
			ammesse++
		}
	}
	if ammesse != 2 {
		t.Errorf("attese 2 mense ammesse, trovate %d", ammesse)
	}
}

func TestCalcola_RimborsoParziale(t *testing.T) {
	// budget 70 per annualità, mensa da 100 → rimborso parziale 70
	smallCfg := graduatoria.BandoConfig{BudgetTotale: 140, ISEEMassimo: 40000}
	apps := []opencity.Application{mensa("M1", 10000, 100)}
	g, err := graduatoria.CalcolaConConfig(apps, smallCfg)
	if err != nil {
		t.Fatal(err)
	}
	pa := annoFor(t, g, 20242025)
	if len(pa.Mense) == 0 {
		t.Fatal("nessuna mensa")
	}
	r := pa.Mense[0]
	if !r.Ammessa {
		t.Fatal("deve essere ammessa (budget sufficiente parzialmente)")
	}
	if r.ImportoRimborso != 70 {
		t.Errorf("rimborso parziale: atteso 70, got %.2f", r.ImportoRimborso)
	}
}
