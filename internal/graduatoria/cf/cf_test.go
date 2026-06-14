package cf_test

import (
	"testing"

	"opencity-gestionale/internal/graduatoria/cf"
)

// CF di test (non reali, checksum calcolato con algoritmo Agenzia Entrate)
// RSSMRA80A01H501U → Mario Rossi, M, 01/01/1980, Roma (H501)
// BNCMRA99T61H501E → Maria Bianchi, F, 21/12/1999, Roma (H501)

func TestEtaAnni(t *testing.T) {
	eta := cf.EtaAnni("RSSMRA80A01H501U")
	if eta < 40 || eta > 60 {
		t.Errorf("EtaAnni(RSSMRA80A01H501U) = %d, atteso tra 40 e 60", eta)
	}
}

func TestAnnoBirth(t *testing.T) {
	cases := []struct {
		cf   string
		want int
	}{
		{"RSSMRA80A01H501U", 1980},
		{"BNCMRA99T61H501E", 1999},
	}
	for _, tc := range cases {
		if got := cf.AnnoBirth(tc.cf); got != tc.want {
			t.Errorf("AnnoBirth(%s) = %d, want %d", tc.cf, got, tc.want)
		}
	}
}

func TestSesso(t *testing.T) {
	cases := []struct {
		cf   string
		want string
	}{
		{"RSSMRA80A01H501U", "M"}, // giorno 01
		{"BNCMRA99T61H501E", "F"}, // giorno 61 (41+20)
	}
	for _, tc := range cases {
		if got := cf.Sesso(tc.cf); got != tc.want {
			t.Errorf("Sesso(%s) = %q, want %q", tc.cf, got, tc.want)
		}
	}
}

func TestComuneNascita(t *testing.T) {
	cases := []struct {
		cf   string
		want string
	}{
		{"RSSMRA80A01H501U", "H501"},
		{"BNCMRA99T61H501E", "H501"},
	}
	for _, tc := range cases {
		if got := cf.ComuneNascita(tc.cf); got != tc.want {
			t.Errorf("ComuneNascita(%s) = %q, want %q", tc.cf, got, tc.want)
		}
	}
}

func TestValido(t *testing.T) {
	cases := []struct {
		cf   string
		want bool
	}{
		{"RSSMRA80A01H501U", true},
		{"BNCMRA99T61H501E", true},
		{"RSSMRA80A01H501Z", false}, // checksum sbagliato
		{"", false},
		{"TROPPO_CORTO", false},
	}
	for _, tc := range cases {
		if got := cf.Valido(tc.cf); got != tc.want {
			t.Errorf("Valido(%s) = %v, want %v", tc.cf, got, tc.want)
		}
	}
}

func TestEdgeCases(t *testing.T) {
	if cf.EtaAnni("") != -1 {
		t.Error("EtaAnni(\"\") dovrebbe restituire -1")
	}
	if cf.Sesso("TROPPO_CORTO") != "" {
		t.Error("Sesso(corto) dovrebbe restituire \"\"")
	}
}
