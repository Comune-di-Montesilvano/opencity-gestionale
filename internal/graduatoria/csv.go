package graduatoria

import (
	"strconv"
	"strings"
)

var HeaderGraduatoria = []string{
	"categoria", "posizione", "ammessa", "note_esclusione",
	"importo_rimborso",
	"id", "protocol_number", "submitted_at", "status", "status_name",
	"richiedente_cf", "richiedente_nome", "richiedente_cognome",
	"richiedente_email", "richiedente_telefono",
	"richiedente_indirizzo", "richiedente_civico", "richiedente_comune", "richiedente_cap", "richiedente_provincia",
	"figlio_selezionato_cf", "num_figli",
	"tipo_richiesta", "annualita", "istituto_codice", "gia_beneficiario",
	"isee", "isee_valido_fino", "isee_dsu_protocollo", "isee_fonte_pdnd", "isee_verificato_pdnd",
	"isee_da_verificare", "isee_scaduto",
	"annualita_valida", "iban_check_form", "residenza_montesilvano", "tutore", "anni_multipli",
	"corrispettivo_lordo", "beneficio_ricevuto", "corrispettivo_netto", "bonus_nidi_coerente",
	"iban", "iban_intestatario",
	"coniuge_cf", "coniuge_nome", "coniuge_cognome",
	"figli_nucleo",
}

func RigaToRecord(categoria string, r RigaGraduatoria) []string {
	ist := r.Istanza
	return []string{
		categoria,
		strconv.Itoa(r.Posizione),
		SiNo(r.Ammessa),
		r.NoteEsclusione,
		strconv.FormatFloat(r.ImportoRimborso, 'f', 2, 64),
		ist.ID,
		ist.ProtocolNumber,
		ist.SubmittedAt,
		ist.Status,
		ist.StatusName,
		ist.RichiedenteCF,
		ist.RichiedenteNome,
		ist.RichiedenteCognome,
		ist.RichiedenteEmail,
		ist.RichiedenteTel,
		ist.Indirizzo,
		ist.Civico,
		ist.Comune,
		ist.CAP,
		ist.Provincia,
		ist.FiglioSelezionatoCF,
		strconv.Itoa(ist.NumFigli),
		ist.TipoRichiesta,
		strconv.Itoa(ist.Annualita),
		ist.IstitutoCodice,
		ist.GiaBeneficiario,
		strconv.FormatFloat(ist.ISEE, 'f', 2, 64),
		ist.ISEEValidoFino,
		ist.ISEEDSUProtocollo,
		ist.ISEEFonte,
		SiNo(ist.ISEEVerificato),
		SiNo(IseeDaVerificare(ist)),
		SiNo(IseeScaduto(ist)),
		SiNo(AnnualitaValide[ist.Annualita]),
		ist.IBANCheck,
		SiNo(ResidenzaMontesilvano(ist)),
		SiNo(IsTutore(ist)),
		SiNo(ist.NumAnni > 1),
		strconv.FormatFloat(ist.Corrispettivo, 'f', 2, 64),
		strconv.FormatFloat(ist.BeneficioRicevuto, 'f', 2, 64),
		strconv.FormatFloat(CorrispettivoNetto(ist), 'f', 2, 64),
		BonusNidiCoerente(ist),
		ist.IBAN,
		ist.IBANIntestatario,
		ist.ConiugeCF,
		ist.ConiugeNome,
		ist.ConiugeCognome,
		strings.Join(ist.FigliNucleo, "|"),
	}
}

func Filtra(righe []RigaGraduatoria, fn func(RigaGraduatoria) bool) []RigaGraduatoria {
	var out []RigaGraduatoria
	for _, r := range righe {
		if fn(r) {
			out = append(out, r)
		}
	}
	return out
}

func FiltraEscluse(righe []RigaGraduatoria, prefisso string) []RigaGraduatoria {
	var out []RigaGraduatoria
	for _, r := range righe {
		if strings.HasPrefix(r.NoteEsclusione, prefisso) {
			out = append(out, r)
		}
	}
	return out
}

func ContaAmmesse(righe []RigaGraduatoria) int {
	n := 0
	for _, r := range righe {
		if r.Ammessa {
			n++
		}
	}
	return n
}
