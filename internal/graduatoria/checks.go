package graduatoria

import (
	"strings"
	"time"
)

var AnnualitaValide = map[int]bool{20232024: true, 20242025: true}

// BonusNidiMaxAnnuo restituisce il massimo Bonus Nidi INPS annuo (11 mesi, agosto escluso).
// Soglia unica €3.000,03: copre sia fascia ≤25k che fascia 25k-40k con Bonus Nidi universale
// (ISEE ≤40k + fratello nato dal 2024, stessa rata €272,73/mese). Importi DPCM 2024.
func BonusNidiMaxAnnuo() float64 {
	return 3000.03 // 11 × €272,73
}

func IseeDaVerificare(ist *Istanza) bool {
	return !ist.ISEEVerificato || ist.ISEEFonte == ""
}

// BonusNidiCoerente verifica che il beneficio dichiarato non superi il massimo INPS.
// Ritorna "si" (coerente), "no" (anomalia: dichiarato > massimo), "" (non applicabile).
func BonusNidiCoerente(ist *Istanza) string {
	if ist.TipoRichiesta != "rette" || ist.BeneficioRicevuto <= 0 {
		return ""
	}
	if ist.BeneficioRicevuto <= BonusNidiMaxAnnuo() {
		return "si"
	}
	return "no"
}

func IseeScaduto(ist *Istanza) bool {
	if ist.ISEEValidoFino == "" {
		return false
	}
	t, err := time.Parse("02/01/2006", ist.ISEEValidoFino)
	if err != nil {
		return false
	}
	return t.Before(time.Now().Truncate(24 * time.Hour))
}

func ResidenzaMontesilvano(ist *Istanza) bool {
	return strings.EqualFold(ist.Comune, "montesilvano")
}

func IsTutore(ist *Istanza) bool {
	return strings.Contains(strings.ToLower(ist.ForWhom), "tutore")
}

func SiNo(b bool) string {
	if b {
		return "si"
	}
	return "no"
}
