package main

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/graduatoria"
)

const baseURLOperatori = "https://service.comune.montesilvano.pe.it/lang/it/operatori"

// scadenzaBando: 24 aprile 2026 ore 23:59:59 Europe/Rome
var scadenzaBando = time.Date(2026, 4, 24, 23, 59, 59, 0, time.FixedZone("CEST", 2*3600))

func linkPratica(id string) string {
	return baseURLOperatori + "/" + id + "/detail"
}

// scriviProspetto genera l'intero sito HTML di rendicontazione nella cartella output/.
func scriviProspetto(grad *graduatoria.Graduatoria, escluse map[string][]graduatoria.RigaGraduatoria) error {
	if err := scriviIndex(grad, escluse); err != nil {
		return err
	}
	for _, ga := range grad.PerAnno {
		anno := strconv.Itoa(ga.Annualita)
		if err := scriviPaginaTipo(anno, "rette", "Rette", ga.Rette); err != nil {
			return err
		}
		if err := scriviPaginaTipo(anno, "mense", "Mense", ga.Mense); err != nil {
			return err
		}
	}
	return scriviPaginaEscluse(escluse)
}

// ── index ────────────────────────────────────────────────────────────────────

func scriviIndex(grad *graduatoria.Graduatoria, escluse map[string][]graduatoria.RigaGraduatoria) error {
	f, err := os.Create(filepath.Join("output", "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	var totUsato float64
	for _, ga := range grad.PerAnno {
		totUsato += ga.BudgetUsatoRette + ga.BudgetUsatoMense
	}

	htmlHead(f, "Graduatoria FSE+ — Riepilogo", "..")
	fmt.Fprintf(f, "<h1>Rimborso Rette e Mense Scolastiche</h1>\n")
	fmt.Fprintf(f, "<p class=sub>Comune di Montesilvano &middot; Det. n.122 del 16.03.2026 &middot; Generato il %s</p>\n",
		time.Now().Format("02/01/2006 15:04"))

	// Budget totale
	fmt.Fprintf(f, "<h2>Budget</h2>\n<table>\n")
	fmt.Fprintf(f, "<tr><th>Fondo totale</th><td class=num>€%s</td></tr>\n", formatEuro(graduatoria.BudgetTotale))
	fmt.Fprintf(f, "<tr><th>Per annualità</th><td class=num>€%s</td></tr>\n", formatEuro(graduatoria.BudgetPerAnnualita))
	fmt.Fprintf(f, "<tr><th>Usato totale</th><td class=num ok>€%s</td></tr>\n", formatEuro(totUsato))
	fmt.Fprintf(f, "<tr><th>Residuo</th><td class=num>€%s</td></tr>\n", formatEuro(graduatoria.BudgetTotale-totUsato))
	fmt.Fprintf(f, "</table>\n")

	// Per annualità
	for _, ga := range grad.PerAnno {
		anno := strconv.Itoa(ga.Annualita)
		usato := ga.BudgetUsatoRette + ga.BudgetUsatoMense
		pct := usato / graduatoria.BudgetPerAnnualita * 100

		ammRette := graduatoria.ContaAmmesse(ga.Rette)
		ammMense := graduatoria.ContaAmmesse(ga.Mense)
		daVerRette := contaDaVerificare(ga.Rette)
		daVerMense := contaDaVerificare(ga.Mense)

		fmt.Fprintf(f, "<h2>Annualità %s</h2>\n", anno)
		barraProgressione(f, pct, usato, graduatoria.BudgetPerAnnualita)
		fmt.Fprintf(f, "<table>\n<tr><th>Categoria</th><th class=num>Ammesse</th><th class=num>⚠ Da verificare</th><th class=num>Fuori fondi</th><th class=num>Budget usato</th><th>Pagine</th></tr>\n")
		fmt.Fprintf(f, "<tr><td>Rette</td><td class=num ok>%d</td><td class=num warn>%d</td><td class=num ko>%d</td><td class=num>€%s</td><td><a href=%s/rette.html>apri →</a></td></tr>\n",
			ammRette, daVerRette, len(ga.Rette)-ammRette, formatEuro(ga.BudgetUsatoRette), anno)
		fmt.Fprintf(f, "<tr><td>Mense</td><td class=num ok>%d</td><td class=num warn>%d</td><td class=num ko>%d</td><td class=num>€%s</td><td><a href=%s/mense.html>apri →</a></td></tr>\n",
			ammMense, daVerMense, len(ga.Mense)-ammMense, formatEuro(ga.BudgetUsatoMense), anno)
		fmt.Fprintf(f, "</table>\n")
	}

	// Escluse
	fmt.Fprintf(f, "<h2>Escluse</h2>\n<table>\n")
	fmt.Fprintf(f, "<tr><th>Motivo</th><th class=num>N.</th><th>Link</th></tr>\n")
	for nome, label := range map[string]string{
		"ritirate.csv":      "Istanza ritirata",
		"duplicati.csv":     "Duplicato (stesso figlio/anno/tipo)",
		"isee_mancante.csv": "ISEE non presente o non valido",
		"corrispettivo.csv": "Corrispettivo netto = 0",
	} {
		fmt.Fprintf(f, "<tr><td>%s</td><td class=num ko>%d</td><td><a href=escluse/index.html#%s>vedi</a></td></tr>\n",
			label, len(escluse[nome]), strings.TrimSuffix(nome, ".csv"))
	}
	fmt.Fprintf(f, "</table>\n<p><a href=escluse/index.html>→ Pagina completa escluse</a></p>\n")

	htmlFoot(f)
	return nil
}

// ── pagina tipo (rette / mense) ───────────────────────────────────────────────

func scriviPaginaTipo(anno, slug, label string, righe []graduatoria.RigaGraduatoria) error {
	path := filepath.Join("output", anno, slug+".html")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	ammesse := graduatoria.Filtra(righe, func(r graduatoria.RigaGraduatoria) bool { return r.Ammessa })
	fuori := graduatoria.Filtra(righe, func(r graduatoria.RigaGraduatoria) bool { return !r.Ammessa })
	pulite := graduatoria.Filtra(ammesse, func(r graduatoria.RigaGraduatoria) bool { return len(flagsRiga(r.Istanza)) == 0 })
	daVer := graduatoria.Filtra(ammesse, func(r graduatoria.RigaGraduatoria) bool { return len(flagsRiga(r.Istanza)) > 0 })

	htmlHead(f, label+" — "+anno, "../..")
	fmt.Fprintf(f, "<p class=nav><a href=../index.html>← Riepilogo</a> &nbsp;|&nbsp; Annualità %s &nbsp;|&nbsp; %s</p>\n", anno, label)
	fmt.Fprintf(f, "<h1>%s — %s</h1>\n", label, anno)

	// Sezione 1: pronte
	fmt.Fprintf(f, "<h2 class=ok>✓ Ammesse — pronte per approvazione (%d)</h2>\n", len(pulite))
	if len(pulite) == 0 {
		fmt.Fprintf(f, "<p class=sub>Nessuna.</p>\n")
	} else {
		tabellaRighe(f, pulite, false)
	}

	// Sezione 2: da verificare
	fmt.Fprintf(f, "<h2 class=warn>⚠ Ammesse — da verificare prima dell'approvazione (%d)</h2>\n", len(daVer))
	if len(daVer) == 0 {
		fmt.Fprintf(f, "<p class=sub>Nessuna.</p>\n")
	} else {
		fmt.Fprintf(f, "<p class=sub>Verificare i flag evidenziati prima di procedere con l'approvazione.</p>\n")
		tabellaRighe(f, daVer, false)
	}

	// Sezione 3: fuori fondi
	fmt.Fprintf(f, "<h2 class=ko>✗ Fuori fondi (%d)</h2>\n", len(fuori))
	if len(fuori) == 0 {
		fmt.Fprintf(f, "<p class=sub>Nessuna.</p>\n")
	} else {
		fmt.Fprintf(f, "<p class=sub>Da notificare con comunicazione di diniego per fondi esauriti.</p>\n")
		tabellaRighe(f, fuori, true)
	}

	htmlFoot(f)
	return nil
}

// ── pagina escluse ────────────────────────────────────────────────────────────

func scriviPaginaEscluse(escluse map[string][]graduatoria.RigaGraduatoria) error {
	f, err := os.Create(filepath.Join("output", "escluse", "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	htmlHead(f, "Escluse", "../..")
	fmt.Fprintf(f, "<p class=nav><a href=../index.html>← Riepilogo</a></p>\n")
	fmt.Fprintf(f, "<h1>Istanze Escluse</h1>\n")

	sezioni := []struct {
		id    string
		label string
		nota  string
	}{
		{"ritirate", "Istanze ritirate", "Il richiedente ha revocato la domanda (status 20000)."},
		{"duplicati", "Duplicati", "Stesso figlio / stessa annualità / stesso tipo già presente in posizione migliore."},
		{"isee_mancante", "ISEE non presente o non valido", "ISEE = 0 o assente. Richiedente da contattare per integrazione documentale."},
		{"corrispettivo", "Corrispettivo netto = 0", "Corrispettivo lordo già interamente coperto da beneficio percepito, oppure non dichiarato."},
	}

	for _, s := range sezioni {
		righe := escluse[s.id+".csv"]
		fmt.Fprintf(f, "<h2 id=%s>%s (%d)</h2>\n<p class=sub>%s</p>\n", s.id, s.label, len(righe), s.nota)
		if len(righe) == 0 {
			fmt.Fprintf(f, "<p class=sub>Nessuna.</p>\n")
			continue
		}
		tabellaEscluse(f, righe)
	}

	htmlFoot(f)
	return nil
}

// ── tabelle ──────────────────────────────────────────────────────────────────

func tabellaRighe(w io.Writer, righe []graduatoria.RigaGraduatoria, soloBase bool) {
	fmt.Fprintf(w, `<div class=scroll><table>
<tr>
  <th>#</th><th>Cognome Nome</th><th>Presentata</th><th>CF richiedente</th><th>CF figlio</th>
  <th class=num>ISEE</th><th class=num>Lordo</th><th class=num>Beneficio</th><th class=num>Netto</th><th class=num>Rimborso</th>
  <th>IBAN</th><th>Intestatario</th><th>Email</th><th>Tel</th>
  <th>Verifica</th>
</tr>
`)
	for _, r := range righe {
		ist := r.Istanza
		flags := flagsRiga(ist) // include già TARDIVA se applicabile
		if ist.FiglioSelezionatoCF == "" {
			flags = append(flags, flagBadge{"CF figlio mancante", "badge-err"})
		}
		rowCls := "ok"
		if !r.Ammessa {
			rowCls = "ko"
		} else if len(flags) > 0 {
			rowCls = "warn"
		}
		fmt.Fprintf(w, "<tr class=%s>\n", rowCls)
		fmt.Fprintf(w, "  <td class=num>%d</td>\n", r.Posizione)
		fmt.Fprintf(w, "  <td><a href=%q>%s %s</a></td>\n", linkPratica(ist.ID), e(ist.RichiedenteCognome), e(ist.RichiedenteNome))
		if isTardiva(ist) {
			fmt.Fprintf(w, "  <td class='ko'>%s</td>\n", formatData(ist))
		} else {
			fmt.Fprintf(w, "  <td>%s</td>\n", formatData(ist))
		}
		fmt.Fprintf(w, "  <td class=mono>%s</td>\n", e(ist.RichiedenteCF))
		if ist.FiglioSelezionatoCF != "" {
			fmt.Fprintf(w, "  <td class=mono>%s</td>\n", e(ist.FiglioSelezionatoCF))
		} else {
			fmt.Fprintf(w, "  <td class='mono ko'>—</td>\n")
		}
		fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(ist.ISEE))
		fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(ist.Corrispettivo))
		if ist.BeneficioRicevuto > 0 {
			fmt.Fprintf(w, "  <td class='num warn'>€%s</td>\n", formatEuro(ist.BeneficioRicevuto))
		} else {
			fmt.Fprintf(w, "  <td class=num>—</td>\n")
		}
		fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(graduatoria.CorrispettivoNetto(ist)))
		if r.ImportoRimborso > 0 {
			fmt.Fprintf(w, "  <td class='num ok'>€%s</td>\n", formatEuro(r.ImportoRimborso))
		} else {
			fmt.Fprintf(w, "  <td class=num>—</td>\n")
		}
		if ist.IBAN != "" {
			fmt.Fprintf(w, "  <td class=mono>%s</td>\n", e(ist.IBAN))
		} else {
			fmt.Fprintf(w, "  <td class='mono ko'>—</td>\n")
		}
		fmt.Fprintf(w, "  <td>%s</td>\n", e(ist.IBANIntestatario))
		fmt.Fprintf(w, "  <td><a href=mailto:%s>%s</a></td>\n", e(ist.RichiedenteEmail), e(ist.RichiedenteEmail))
		fmt.Fprintf(w, "  <td>%s</td>\n", e(ist.RichiedenteTel))
		fmt.Fprintf(w, "  <td>")
		for _, fl := range flags {
			fmt.Fprintf(w, "<span class='badge %s'>%s</span> ", fl.cls, e(fl.label))
		}
		fmt.Fprintf(w, "</td>\n</tr>\n")
	}
	fmt.Fprintf(w, "</table></div>\n")
}

func tabellaEscluse(w io.Writer, righe []graduatoria.RigaGraduatoria) {
	fmt.Fprintf(w, `<div class=scroll><table>
<tr>
  <th>Cognome Nome</th><th>Presentata</th><th>CF richiedente</th><th>CF figlio</th>
  <th class=num>ISEE</th><th class=num>Lordo</th><th class=num>Beneficio</th>
  <th>Motivo</th><th>Email</th><th>Tel</th>
</tr>
`)
	for _, r := range righe {
		ist := r.Istanza
		fmt.Fprintf(w, "<tr>\n")
		fmt.Fprintf(w, "  <td><a href=%q>%s %s</a></td>\n", linkPratica(ist.ID), e(ist.RichiedenteCognome), e(ist.RichiedenteNome))
		fmt.Fprintf(w, "  <td>%s</td>\n", formatData(ist))
		fmt.Fprintf(w, "  <td class=mono>%s</td>\n", e(ist.RichiedenteCF))
		if ist.FiglioSelezionatoCF != "" {
			fmt.Fprintf(w, "  <td class=mono>%s</td>\n", e(ist.FiglioSelezionatoCF))
		} else {
			fmt.Fprintf(w, "  <td class='mono ko'>—</td>\n")
		}
		fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(ist.ISEE))
		fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(ist.Corrispettivo))
		if ist.BeneficioRicevuto > 0 {
			fmt.Fprintf(w, "  <td class=num>€%s</td>\n", formatEuro(ist.BeneficioRicevuto))
		} else {
			fmt.Fprintf(w, "  <td class=num>—</td>\n")
		}
		if r.OriginalID != "" {
			fmt.Fprintf(w, "  <td>%s &mdash; <a href=%q target=_blank>vedi originale →</a></td>\n",
				e(r.NoteEsclusione), linkPratica(r.OriginalID))
		} else {
			fmt.Fprintf(w, "  <td>%s</td>\n", e(r.NoteEsclusione))
		}
		fmt.Fprintf(w, "  <td><a href=mailto:%s>%s</a></td>\n", e(ist.RichiedenteEmail), e(ist.RichiedenteEmail))
		fmt.Fprintf(w, "  <td>%s</td>\n", e(ist.RichiedenteTel))
		fmt.Fprintf(w, "</tr>\n")
	}
	fmt.Fprintf(w, "</table></div>\n")
}

// ── flag logic ────────────────────────────────────────────────────────────────

type flagBadge struct {
	label string
	cls   string // css class: "badge-warn" | "badge-err"
}

func parsePresentazione(ist *graduatoria.Istanza) time.Time {
	t, _ := time.Parse(time.RFC3339, ist.SubmittedAt)
	return t
}

func isTardiva(ist *graduatoria.Istanza) bool {
	t := parsePresentazione(ist)
	return !t.IsZero() && t.After(scadenzaBando)
}

func formatData(ist *graduatoria.Istanza) string {
	t := parsePresentazione(ist)
	if t.IsZero() {
		return ist.SubmittedAt
	}
	return t.Format("02/01/2006 15:04")
}

func flagsRiga(ist *graduatoria.Istanza) []flagBadge {
	var out []flagBadge
	if graduatoria.IseeDaVerificare(ist) {
		out = append(out, flagBadge{"ISEE non PDND", "badge-warn"})
	}
	if graduatoria.IsTutore(ist) {
		out = append(out, flagBadge{"tutore legale", "badge-warn"})
	}
	if graduatoria.BonusNidiCoerente(ist) == "no" {
		out = append(out, flagBadge{"Bonus Nidi ⚠", "badge-err"})
	}
	if ist.IBANCheck != "" && ist.IBANCheck != "Valido" {
		out = append(out, flagBadge{"IBAN: " + ist.IBANCheck, "badge-err"})
	}
	if isTardiva(ist) {
		out = append(out, flagBadge{"TARDIVA", "badge-err"})
	}
	return out
}

func contaDaVerificare(righe []graduatoria.RigaGraduatoria) int {
	n := 0
	for _, r := range righe {
		if r.Ammessa && len(flagsRiga(r.Istanza)) > 0 {
			n++
		}
	}
	return n
}

// ── html helpers ──────────────────────────────────────────────────────────────

func e(s string) string { return html.EscapeString(s) }

func formatEuro(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	dot := strings.Index(s, ".")
	intPart, decPart := s[:dot], s[dot+1:]
	n := len(intPart)
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(ch)
	}
	b.WriteByte(',')
	b.WriteString(decPart)
	return b.String()
}

func htmlHead(w io.Writer, title, _ string) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width">
<title>%s</title>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,sans-serif;font-size:.92rem;max-width:1400px;margin:0 auto;padding:1rem 1.5rem;color:#111;background:#fafafa}
h1{font-size:1.3rem;margin:.5rem 0 0}
h2{font-size:1rem;margin:1.8rem 0 .4rem;padding:.3rem .6rem;border-left:4px solid #4a90d9;background:#f0f5ff}
h2.ok{border-color:#2a7a3a;background:#f0fff4}
h2.warn{border-color:#c97a00;background:#fffbea}
h2.ko{border-color:#b00020;background:#fff0f0}
.nav{color:#555;font-size:.85rem;margin:.2rem 0 1rem}
.nav a{color:#2c5f8a;text-decoration:none}
.sub{color:#555;font-size:.85rem;margin:.2rem 0 .8rem}
.scroll{overflow-x:auto}
table{border-collapse:collapse;width:100%%;margin:.3rem 0 1rem;font-size:.85rem}
th,td{border:1px solid #ddd;padding:.3rem .5rem;white-space:nowrap}
th{background:#eef2f8;font-weight:600;position:sticky;top:0}
tr.ok{background:#fff}
tr.warn{background:#fffbea}
tr.ko{background:#f9f9f9;color:#666}
td.ok{color:#1a7a3a;font-weight:600}
td.warn{color:#c97a00;font-weight:600}
td.ko{color:#b00020;font-weight:600}
td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}
td.mono{font-family:monospace;font-size:.8rem}
.badge{display:inline-block;padding:.1rem .35rem;border-radius:3px;font-size:.75rem;font-weight:600;margin:.1rem}
.badge-warn{background:#fff3cd;color:#856404;border:1px solid #ffc107}
.badge-err{background:#f8d7da;color:#842029;border:1px solid #f5c6cb}
.bar-wrap{height:14px;background:#e0e7ef;border-radius:4px;margin:.3rem 0 .8rem}
.bar-fill{height:14px;background:#4a90d9;border-radius:4px}
a{color:#2c5f8a}
</style>
</head>
<body>
`, e(title))
}

func htmlFoot(w io.Writer) {
	fmt.Fprintf(w, "\n</body>\n</html>\n")
}

func barraProgressione(w io.Writer, pct, usato, totale float64) {
	if pct > 100 {
		pct = 100
	}
	fmt.Fprintf(w, "<div class=bar-wrap><div class=bar-fill style=\"width:%.0f%%\"></div></div>\n<p class=sub>Budget usato: €%s / €%s (%.0f%%)</p>\n",
		pct, formatEuro(usato), formatEuro(totale), pct)
}
