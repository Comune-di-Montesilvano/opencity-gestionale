package graduatoria

import "math"

// Istanza rappresenta una singola istanza OpenCity deserializzata.
// I campi vengono popolati dal Record.ToIstanza() dell'engine generico.
// Campi non mappati restano al valore zero.
type Istanza struct {
	ID             string
	ProtocolNumber string
	SubmittedAt    string
	Status         string
	StatusName     string

	RichiedenteCF      string
	RichiedenteNome    string
	RichiedenteCognome string
	RichiedenteEmail   string
	RichiedenteTel     string
	Indirizzo          string
	Civico             string
	Comune             string
	CAP                string
	Provincia          string

	FiglioSelezionatoCF string
	NumFigli            int

	TipoRichiesta     string
	Annualita         int
	Corrispettivo     float64
	BeneficioRicevuto float64
	IstitutoCodice    string
	GiaBeneficiario   string

	ISEE              float64
	ISEEValidoFino    string
	ISEEFonte         string
	ISEEVerificato    bool
	ISEEDSUProtocollo string

	IBAN             string
	IBANIntestatario string
	IBANCheck        string

	ConiugeCF      string
	ConiugeNome    string
	ConiugeCognome string
	FigliNucleo    []string

	ForWhom string
	NumAnni int
}

type RigaGraduatoria struct {
	Posizione       int
	Istanza         *Istanza
	ImportoRimborso float64
	Ammessa         bool
	NoteEsclusione  string
	OriginalID      string
}

func (r RigaGraduatoria) CorrispettivoNetto() float64 {
	if r.Istanza == nil {
		return 0
	}
	return CorrispettivoNetto(r.Istanza)
}

// GraduatoriaGruppo rappresenta una tipologia nel risultato dell'engine generico.
type GraduatoriaGruppo struct {
	Nome        string
	Righe       []RigaGraduatoria
	BudgetUsato float64
}

type Graduatoria struct {
	Gruppi  []*GraduatoriaGruppo
	Escluse []RigaGraduatoria
}

func (g *Graduatoria) TotaleAmmesse() int {
	count := 0
	for _, gr := range g.Gruppi {
		for _, r := range gr.Righe {
			if r.Ammessa {
				count++
			}
		}
	}
	return count
}

func (g *Graduatoria) TotaleBudgetUsato() float64 {
	var tot float64
	for _, gr := range g.Gruppi {
		tot += gr.BudgetUsato
	}
	return tot
}

func CorrispettivoNetto(ist *Istanza) float64 {
	return math.Max(0, ist.Corrispettivo-ist.BeneficioRicevuto)
}
