package graduatoria

import (
	"testing"
)

func TestBonusNidiCoerente(t *testing.T) {
	tests := []struct {
		name   string
		ist    Istanza
		expect string
	}{
		{"rette coerente", Istanza{TipoRichiesta: "rette", BeneficioRicevuto: 1000}, "si"},
		{"rette al limite", Istanza{TipoRichiesta: "rette", BeneficioRicevuto: 3000.03}, "si"},
		{"rette anomalia", Istanza{TipoRichiesta: "rette", BeneficioRicevuto: 3500}, "no"},
		{"mensa non applicabile", Istanza{TipoRichiesta: "mensa", BeneficioRicevuto: 5000}, ""},
		{"rette beneficio zero", Istanza{TipoRichiesta: "rette", BeneficioRicevuto: 0}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BonusNidiCoerente(&tc.ist)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestIseeScaduto(t *testing.T) {
	tests := []struct {
		name   string
		ist    Istanza
		expect bool
	}{
		{"scaduto", Istanza{ISEEValidoFino: "31/12/2020"}, true},
		{"valido", Istanza{ISEEValidoFino: "31/12/2099"}, false},
		{"vuoto", Istanza{ISEEValidoFino: ""}, false},
		{"formato invalido", Istanza{ISEEValidoFino: "2025-12-31"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IseeScaduto(&tc.ist)
			if got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestIseeDaVerificare(t *testing.T) {
	tests := []struct {
		name   string
		ist    Istanza
		expect bool
	}{
		{"verificato PDND", Istanza{ISEEVerificato: true, ISEEFonte: "INPS"}, false},
		{"non verificato", Istanza{ISEEVerificato: false, ISEEFonte: "INPS"}, true},
		{"fonte vuota", Istanza{ISEEVerificato: true, ISEEFonte: ""}, true},
		{"entrambi mancanti", Istanza{ISEEVerificato: false, ISEEFonte: ""}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IseeDaVerificare(&tc.ist)
			if got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestIsTutore(t *testing.T) {
	tests := []struct {
		name   string
		ist    Istanza
		expect bool
	}{
		{"tutore minuscolo", Istanza{ForWhom: "per conto di tutore legale"}, true},
		{"Tutore maiuscolo", Istanza{ForWhom: "Tutore"}, true},
		{"genitore", Istanza{ForWhom: "genitore"}, false},
		{"vuoto", Istanza{ForWhom: ""}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsTutore(&tc.ist)
			if got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestResidenzaMontesilvano(t *testing.T) {
	tests := []struct {
		comune string
		expect bool
	}{
		{"montesilvano", true},
		{"Montesilvano", true},
		{"MONTESILVANO", true},
		{"pescara", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.comune, func(t *testing.T) {
			got := ResidenzaMontesilvano(&Istanza{Comune: tc.comune})
			if got != tc.expect {
				t.Errorf("comune %q: got %v, want %v", tc.comune, got, tc.expect)
			}
		})
	}
}

func TestCorrispettivoNetto(t *testing.T) {
	tests := []struct {
		corrispettivo float64
		beneficio     float64
		expect        float64
	}{
		{100, 30, 70},
		{100, 100, 0},
		{100, 120, 0},  // floor a 0
		{0, 0, 0},
		{500, 0, 500},
	}
	for _, tc := range tests {
		ist := &Istanza{Corrispettivo: tc.corrispettivo, BeneficioRicevuto: tc.beneficio}
		got := CorrispettivoNetto(ist)
		if got != tc.expect {
			t.Errorf("corr=%.2f ben=%.2f: got %.2f, want %.2f", tc.corrispettivo, tc.beneficio, got, tc.expect)
		}
	}
}
