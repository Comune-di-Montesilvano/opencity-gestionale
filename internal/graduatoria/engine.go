package graduatoria

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"opencity-backend/internal/opencity"
)

const (
	BudgetTotale       float64 = 71096.37
	BudgetPerAnnualita float64 = BudgetTotale / 2
	ISEEMassimo        float64 = 40000.00
)

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

	TipoRichiesta      string // "rette" | "mensa"
	Annualita          int
	Corrispettivo      float64 // importo lordo dichiarato
	BeneficioRicevuto  float64 // Bonus Nidi o altro già percepito
	IstitutoCodice     string
	GiaBeneficiario    string

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
	ImportoRimborso float64 // quanto viene rimborsato (può essere parziale)
	Ammessa         bool    // true = nei fondi disponibili
	NoteEsclusione  string
	OriginalID      string // solo per duplicati: ID dell'istanza tenuta
}

type GraduatoriaAnnualita struct {
	Annualita        int
	Rette            []RigaGraduatoria
	Mense            []RigaGraduatoria
	BudgetUsatoRette float64
	BudgetUsatoMense float64
}

type Graduatoria struct {
	PerAnno []*GraduatoriaAnnualita
	Escluse []RigaGraduatoria
}

func ParseIstanze(app opencity.Application) ([]*Istanza, error) {
	var d opencity.MenseData
	if err := json.Unmarshal(app.Data, &d); err != nil {
		return nil, err
	}

	var figliNucleo []string
	for _, f := range d.Children.Children {
		figliNucleo = append(figliNucleo, f.TaxID)
	}

	base := Istanza{
		ID:             app.ID,
		ProtocolNumber: app.ProtocolNumber,
		SubmittedAt:    app.SubmittedAt,
		Status:         app.Status,
		StatusName:     app.StatusName,

		RichiedenteCF:      d.Applicant.FiscalCode.FiscalCode,
		RichiedenteNome:    d.Applicant.CompleteName.Name,
		RichiedenteCognome: d.Applicant.CompleteName.Surname,
		RichiedenteEmail:   d.Applicant.Email,
		RichiedenteTel:     d.Applicant.Phone,
		Indirizzo:          d.Applicant.Address.Address,
		Civico:             d.Applicant.Address.HouseNumber,
		Comune:             d.Applicant.Address.Municipality,
		CAP:                d.Applicant.Address.PostalCode,
		Provincia:          d.Applicant.Address.County,

		FiglioSelezionatoCF: d.SelectChild,
		NumFigli:            len(d.Children.Children),

		ISEE:              d.ISEE.Value,
		ISEEValidoFino:    d.ISEE.ValidUntil,
		ISEEFonte:         d.ISEE.Meta.Source,
		ISEEVerificato:    d.ISEE.Meta.Signature != "",
		ISEEDSUProtocollo: d.ISEE.DSUProtocol,

		IBAN:             d.IBAN.IBAN,
		IBANIntestatario: d.IntestataAl,
		IBANCheck:        d.IBAN.IBANCheck,

		ConiugeCF:      d.Spouse.TaxID,
		ConiugeNome:    d.Spouse.GivenName,
		ConiugeCognome: d.Spouse.FamilyName,

		FigliNucleo: figliNucleo,
		ForWhom:     d.ForWhom,
		NumAnni:     len(d.Anni),
	}

	if len(d.Anni) == 0 {
		return []*Istanza{&base}, nil
	}

	out := make([]*Istanza, 0, len(d.Anni))
	for _, a := range d.Anni {
		ist := base // copia struct
		ist.TipoRichiesta = a.TipoRichiesta
		ist.Annualita = a.Annualita
		ist.Corrispettivo = a.Corrispettivo
		ist.BeneficioRicevuto = a.ImportoBeneficioRicevuto
		ist.IstitutoCodice = a.InfanziaMense
		ist.GiaBeneficiario = a.GiaBeneficiario
		out = append(out, &ist)
	}
	return out, nil
}

func CorrispettivoNetto(ist *Istanza) float64 {
	return math.Max(0, ist.Corrispettivo-ist.BeneficioRicevuto)
}

func isAmmissibile(ist *Istanza) (bool, string) {
	return isAmmissibileConISEE(ist, ISEEMassimo)
}

func isAmmissibileConISEE(ist *Istanza, iseeMax float64) (bool, string) {
	if ist.Status == "20000" {
		return false, "istanza ritirata"
	}
	if ist.ISEE <= 0 {
		return false, "ISEE non presente o non valido"
	}
	if ist.ISEE > iseeMax {
		return false, fmt.Sprintf("ISEE superiore a €%.0f", iseeMax)
	}
	if CorrispettivoNetto(ist) <= 0 {
		return false, "corrispettivo non presente"
	}
	return true, ""
}

func sortByISEENumFigli(list []*Istanza) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].ISEE != list[j].ISEE {
			return list[i].ISEE < list[j].ISEE // ISEE crescente
		}
		return list[i].NumFigli > list[j].NumFigli // più figli = priorità
	})
}

func assegna(lista []*Istanza, budgetDisponibile float64) (righe []RigaGraduatoria, budgetUsato float64) {
	residuo := budgetDisponibile
	for pos, ist := range lista {
		riga := RigaGraduatoria{
			Posizione: pos + 1,
			Istanza:   ist,
			Ammessa:   true,
		}
		if residuo <= 0 {
			riga.Ammessa = false
			riga.NoteEsclusione = "fondi esauriti"
			righe = append(righe, riga)
			continue
		}
		rimborso := math.Min(CorrispettivoNetto(ist), residuo)
		riga.ImportoRimborso = math.Round(rimborso*100) / 100
		residuo -= rimborso
		righe = append(righe, riga)
	}
	budgetUsato = budgetDisponibile - residuo
	return
}

// chiaveDuplicato identifica univocamente una domanda: stesso richiedente, stesso figlio, stesso anno, stesso tipo.
// chiaveDuplicato identifica univocamente una domanda per figlio: stesso figlio, stesso anno, stesso tipo.
// Non include il richiedente — un figlio può avere un solo rimborso per tipo/anno,
// indipendentemente da quale genitore (o tutore) abbia presentato la domanda.
type chiaveDuplicato struct {
	FiglioSelezionatoCF string
	Annualita           int
	TipoRichiesta       string
}

// deduplicaOrdinata riceve la lista già ordinata per ISEE+figli e restituisce
// (lista de-duplicata, righe escluse per duplicato).
// La prima occorrenza per ogni chiave viene tenuta, le successive escluse.
func deduplicaOrdinata(lista []*Istanza) ([]*Istanza, []RigaGraduatoria) {
	seenID := make(map[chiaveDuplicato]string) // chiave → ID istanza originale
	var unici []*Istanza
	var duplicati []RigaGraduatoria

	for _, ist := range lista {
		k := chiaveDuplicato{
			FiglioSelezionatoCF: ist.FiglioSelezionatoCF,
			Annualita:           ist.Annualita,
			TipoRichiesta:       ist.TipoRichiesta,
		}
		if origID, ok := seenID[k]; ok {
			duplicati = append(duplicati, RigaGraduatoria{
				Istanza:        ist,
				Ammessa:        false,
				NoteEsclusione: "duplicato: stesso figlio già presente in posizione migliore",
				OriginalID:     origID,
			})
		} else {
			seenID[k] = ist.ID
			unici = append(unici, ist)
		}
	}
	return unici, duplicati
}

// Calcola esegue il calcolo con i parametri hardcoded del bando FSE+ 2026.
// Usato dal CLI batch. Per il web server usare CalcolaConConfig.
func Calcola(apps []opencity.Application) (*Graduatoria, error) {
	return CalcolaConConfig(apps, BandoConfig{
		BudgetTotale: BudgetTotale,
		ISEEMassimo:  ISEEMassimo,
	})
}

// CalcolaConConfig esegue il calcolo con parametri forniti dall'esterno (bando dal DB).
func CalcolaConConfig(apps []opencity.Application, cfg BandoConfig) (*Graduatoria, error) {
	budgetPerAnno := cfg.BudgetTotale / 2
	iseeMax := cfg.ISEEMassimo
	if iseeMax <= 0 {
		iseeMax = ISEEMassimo
	}

	rettePerAnno := make(map[int][]*Istanza)
	mensePerAnno := make(map[int][]*Istanza)
	var escluse []RigaGraduatoria

	for i := range apps {
		istanze, err := ParseIstanze(apps[i])
		if err != nil {
			continue
		}
		for _, ist := range istanze {
			ok, motivo := isAmmissibileConISEE(ist, iseeMax)
			if !ok {
				escluse = append(escluse, RigaGraduatoria{
					Istanza:        ist,
					Ammessa:        false,
					NoteEsclusione: motivo,
				})
				continue
			}
			if ist.TipoRichiesta == "rette" {
				rettePerAnno[ist.Annualita] = append(rettePerAnno[ist.Annualita], ist)
			} else {
				mensePerAnno[ist.Annualita] = append(mensePerAnno[ist.Annualita], ist)
			}
		}
	}

	annualita := []int{20232024, 20242025}
	grad := &Graduatoria{Escluse: escluse}

	for _, anno := range annualita {
		rette := rettePerAnno[anno]
		mense := mensePerAnno[anno]

		sortByISEENumFigli(rette)
		sortByISEENumFigli(mense)

		retteUnici, rettiDupl := deduplicaOrdinata(rette)
		menseUniche, menseDupl := deduplicaOrdinata(mense)
		grad.Escluse = append(grad.Escluse, rettiDupl...)
		grad.Escluse = append(grad.Escluse, menseDupl...)

		righeRette, usatoRette := assegna(retteUnici, budgetPerAnno)
		righeMense, usatoMense := assegna(menseUniche, budgetPerAnno-usatoRette)

		grad.PerAnno = append(grad.PerAnno, &GraduatoriaAnnualita{
			Annualita:        anno,
			Rette:            righeRette,
			Mense:            righeMense,
			BudgetUsatoRette: usatoRette,
			BudgetUsatoMense: usatoMense,
		})
	}

	return grad, nil
}
