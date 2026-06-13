package opencity

import "encoding/json"

type Application struct {
	ID            string          `json:"id"`
	UserName      string          `json:"user_name"`
	Status        string          `json:"status"`
	StatusName    string          `json:"status_name"`
	Subject       string          `json:"subject"`
	SubmittedAt   string          `json:"submitted_at"`
	SubmissionTime int64          `json:"submission_time"`
	CreatedAt     string          `json:"created_at"`
	ProtocolNumber string         `json:"protocol_number"`
	Data          json.RawMessage `json:"data"`
}

// MenseData holds the nested Form.IO fields for "Rimborso spese rette e mense scolastiche".
type MenseData struct {
	ForWhom     string `json:"for_whom"`
	SelectChild string `json:"select_child"` // CF figlio selezionato

	Anni []struct {
		Annualita              int     `json:"annualita1"`
		Corrispettivo          float64 `json:"corrispettivo"`
		InfanziaMense          string  `json:"infanziaMense"`
		TipoRichiesta          string  `json:"tiporichiesta"` // "mensa" | "retta"
		GiaBeneficiario        string  `json:"giabenificiario"`
		ImportoBeneficioRicevuto float64 `json:"importoDelBeneficioRicevuto"`
	} `json:"anni"`

	IBAN struct {
		IBAN      string `json:"iban"`
		BicSwift  string `json:"bic_swift"`
		IBANCheck string `json:"iban_check"`
	} `json:"iban"`

	IntestataAl string `json:"intestatarioContoCorrenteCognomeENome"`

	ISEE struct {
		Value          float64 `json:"isee"`
		ValidUntil     string  `json:"valid_until"`
		SubmissionDate string  `json:"submission_date"`
		DSUProtocol    string  `json:"dsu_protocol_number"`
		Meta           struct {
			Source    string `json:"source"`
			Signature string `json:"signature"`
			CreatedAt string `json:"created_at"`
		} `json:"meta"`
	} `json:"ordinary_economic_situation_indicator"`

	Applicant struct {
		Email  string `json:"email_address"`
		Phone  string `json:"cell_number"`
		Gender struct {
			Gender string `json:"gender"`
		} `json:"gender"`
		CompleteName struct {
			Name    string `json:"name"`
			Surname string `json:"surname"`
		} `json:"completename"`
		FiscalCode struct {
			FiscalCode string `json:"fiscal_code"`
		} `json:"fiscal_code"`
		Born struct {
			NatoAIl    string `json:"natoAIl"`
			PlaceOfBirth string `json:"place_of_birth"`
		} `json:"Born"`
		Address struct {
			Address      string `json:"address"`
			HouseNumber  string `json:"house_number"`
			Municipality string `json:"municipality"`
			PostalCode   string `json:"postal_code"`
			County       string `json:"county"`
		} `json:"address"`
	} `json:"applicant"`

	Children struct {
		Children []struct {
			GivenName  string `json:"given_name"`
			FamilyName string `json:"family_name"`
			TaxID      string `json:"tax_id"`
			BirthDate  string `json:"birth_date"`
			BirthPlace string `json:"birth_place"`
			Gender     string `json:"gender"`
		} `json:"children"`
	} `json:"children"`

	Spouse struct {
		GivenName  string `json:"given_name"`
		FamilyName string `json:"family_name"`
		TaxID      string `json:"tax_id"`
	} `json:"spouse"`
}
