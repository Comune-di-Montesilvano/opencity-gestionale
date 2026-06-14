package graduatoria

import (
	"testing"
	"time"
)

func TestPassaFiltro(t *testing.T) {
	dataNascita := time.Date(2010, 1, 15, 0, 0, 0, 0, time.UTC)

	baseRec := func() *Record {
		r := NewRecord("app-1", "4000")
		r.FloatMap["isee"] = 15000
		r.IntMap["figli"] = 2
		r.StringMap["tipo"] = "rette"
		r.StringMap["attivo"] = "true"
		r.StringMap["cf"] = "RSSMRA80A01H501U"  // M, 01/01/1980, H501
		r.StringMap["cf_f"] = "BNCMRA99T61H501E" // F, 21/12/1999, H501
		r.TimeMap["data"] = dataNascita
		return r
	}

	tests := []struct {
		name string
		rec  *Record
		f    FiltroConfig
		want bool
	}{
		// --- Numerici float ---
		{name: "float_lte_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "<=", Valore: 20000}, want: true},
		{name: "float_lte_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "<=", Valore: 10000}, want: false},
		{name: "float_gte_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: ">=", Valore: 10000}, want: true},
		{name: "float_gte_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: ">=", Valore: 20000}, want: false},
		{name: "float_eq_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "==", Valore: 15000}, want: true},
		{name: "float_eq_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "==", Valore: 10000}, want: false},
		{name: "float_neq_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "!=", Valore: 10000}, want: true},
		{name: "float_neq_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "!=", Valore: 15000}, want: false},
		{name: "float_lt_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "<", Valore: 20000}, want: true},
		{name: "float_lt_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "<", Valore: 15000}, want: false},
		{name: "float_gt_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: ">", Valore: 10000}, want: true},
		{name: "float_gt_fail", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: ">", Valore: 15000}, want: false},
		// tra
		{name: "tra_pass", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "tra", Valore: "10000,20000"}, want: true},
		{name: "tra_fail_low", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "tra", Valore: "20000,30000"}, want: false},
		{name: "tra_fail_high", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "tra", Valore: "1000,10000"}, want: false},
		{name: "tra_bordo_min", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "tra", Valore: "15000,20000"}, want: true},
		{name: "tra_bordo_max", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "tra", Valore: "10000,15000"}, want: true},

		// --- Numerici int ---
		{name: "int_eq_pass", rec: baseRec(), f: FiltroConfig{Campo: "figli", Op: "==", Valore: 2}, want: true},
		{name: "int_gt_pass", rec: baseRec(), f: FiltroConfig{Campo: "figli", Op: ">", Valore: 1}, want: true},
		{name: "int_lt_fail", rec: baseRec(), f: FiltroConfig{Campo: "figli", Op: "<", Valore: 2}, want: false},

		// --- Stringhe ---
		{name: "contiene_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "contiene", Valore: "ret"}, want: true},
		{name: "contiene_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "contiene", Valore: "mensa"}, want: false},
		{name: "contiene_case", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "contiene", Valore: "RET"}, want: true},
		{name: "inizia_con_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "inizia_con", Valore: "ret"}, want: true},
		{name: "inizia_con_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "inizia_con", Valore: "tte"}, want: false},
		{name: "finisce_con_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "finisce_con", Valore: "tte"}, want: true},
		{name: "finisce_con_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "finisce_con", Valore: "ret"}, want: false},
		{name: "in_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "in", Valore: "rette,mensa,trasporto"}, want: true},
		{name: "in_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "in", Valore: "mensa,trasporto"}, want: false},
		{name: "non_in_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "non_in", Valore: "mensa,trasporto"}, want: true},
		{name: "non_in_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "non_in", Valore: "rette,mensa"}, want: false},
		{name: "non_vuoto_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "non_vuoto"}, want: true},
		{name: "vuoto_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "vuoto"}, want: false},
		{name: "vuoto_pass_campo_mancante", rec: baseRec(), f: FiltroConfig{Campo: "campo_vuoto", Op: "vuoto"}, want: true},
		{name: "str_eq_case_insensitive", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "==", Valore: "RETTE"}, want: true},
		{name: "str_neq_pass", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "!=", Valore: "mensa"}, want: true},
		{name: "str_neq_fail", rec: baseRec(), f: FiltroConfig{Campo: "tipo", Op: "!=", Valore: "rette"}, want: false},

		// --- Booleano ---
		{name: "vero_pass", rec: baseRec(), f: FiltroConfig{Campo: "attivo", Op: "vero"}, want: true},
		{name: "falso_fail", rec: baseRec(), f: FiltroConfig{Campo: "attivo", Op: "falso"}, want: false},
		{
			name: "falso_pass",
			rec: func() *Record {
				r := baseRec()
				r.StringMap["attivo"] = "false"
				return r
			}(),
			f:    FiltroConfig{Campo: "attivo", Op: "falso"},
			want: true,
		},
		{
			name: "vero_con_1",
			rec: func() *Record {
				r := baseRec()
				r.StringMap["flag"] = "1"
				return r
			}(),
			f:    FiltroConfig{Campo: "flag", Op: "vero"},
			want: true,
		},

		// --- Data ---
		{name: "prima_di_pass", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "prima_di", Valore: "2015-01-01"}, want: true},
		{name: "prima_di_fail", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "prima_di", Valore: "2005-01-01"}, want: false},
		{name: "dopo_di_pass", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "dopo_di", Valore: "2005-01-01"}, want: true},
		{name: "dopo_di_fail", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "dopo_di", Valore: "2015-01-01"}, want: false},
		// eta_max/min: data=2010-01-15, oggi=2026 → età 16 anni
		{name: "eta_max_pass", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "eta_max", Valore: "20"}, want: true},
		{name: "eta_max_fail", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "eta_max", Valore: "5"}, want: false},
		{name: "eta_min_pass", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "eta_min", Valore: "10"}, want: true},
		{name: "eta_min_fail", rec: baseRec(), f: FiltroConfig{Campo: "data", Op: "eta_min", Valore: "30"}, want: false},

		// --- CF: RSSMRA80A01H501U = M, 01/01/1980, H501, età 46 nel 2026 ---
		{name: "cf_eta_max_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_eta_max", Valore: 50}, want: true},
		{name: "cf_eta_max_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_eta_max", Valore: 10}, want: false},
		{name: "cf_eta_min_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_eta_min", Valore: 40}, want: true},
		{name: "cf_eta_min_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_eta_min", Valore: 60}, want: false},
		{name: "cf_anno_max_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_anno_max", Valore: 1985}, want: true},
		{name: "cf_anno_max_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_anno_max", Valore: 1970}, want: false},
		{name: "cf_anno_min_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_anno_min", Valore: 1975}, want: true},
		{name: "cf_anno_min_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_anno_min", Valore: 1990}, want: false},
		{name: "cf_sesso_M_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_sesso", Valore: "M"}, want: true},
		{name: "cf_sesso_F_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_sesso", Valore: "F"}, want: false},
		// BNCMRA99T61H501E = F, 21/12/1999, H501
		{name: "cf_sesso_F_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf_f", Op: "cf_sesso", Valore: "F"}, want: true},
		{name: "cf_comune_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_comune", Valore: "H501"}, want: true},
		{name: "cf_comune_fail", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_comune", Valore: "F205"}, want: false},
		{name: "cf_valido_pass", rec: baseRec(), f: FiltroConfig{Campo: "cf", Op: "cf_valido"}, want: true},
		{
			name: "cf_valido_fail",
			rec: func() *Record {
				r := baseRec()
				r.StringMap["cf"] = "RSSMRA80A01H501Z" // checksum errato
				return r
			}(),
			f:    FiltroConfig{Campo: "cf", Op: "cf_valido"},
			want: false,
		},

		// --- Campo inesistente / op sconosciuto ---
		{name: "campo_mancante_num", rec: baseRec(), f: FiltroConfig{Campo: "inesistente", Op: "<=", Valore: 999}, want: false},
		{name: "op_sconosciuta", rec: baseRec(), f: FiltroConfig{Campo: "isee", Op: "foobar", Valore: 0}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rec.PassaFiltro(tt.f)
			if got != tt.want {
				t.Errorf("PassaFiltro(%s %s %v) = %v, want %v",
					tt.f.Campo, tt.f.Op, tt.f.Valore, got, tt.want)
			}
		})
	}
}
