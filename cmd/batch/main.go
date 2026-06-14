package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"opencity-gestionale/internal/graduatoria"
	_ "opencity-gestionale/internal/graduatoria/generic" // registra engine generico
	_ "opencity-gestionale/internal/graduatoria/mense"   // registra engine mense_rette
	"opencity-gestionale/internal/opencity"
)

const (
	baseURL   = "https://service.comune.montesilvano.pe.it"
	serviceID = "5756cd98-7fe6-4818-bad8-69a2c843b546"
)

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "Variabile d'ambiente mancante: %s\n", key)
		os.Exit(1)
	}
	return v
}

func main() {
	loadDotEnv()
	username := mustEnv("OPENCITY_USERNAME")
	password := mustEnv("OPENCITY_PASSWORD")

	fmt.Fprintln(os.Stderr, "Autenticazione...")
	jwt, err := opencity.Login(baseURL, username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Errore auth: %v\n", err)
		os.Exit(1)
	}
	client := opencity.NewClient(baseURL, jwt)

	fmt.Fprintln(os.Stderr, "Recupero istanze...")
	rawApps, err := client.FetchAllApplications(serviceID, func(fetched, total int) {
		fmt.Fprintf(os.Stderr, "  %d / %d\n", fetched, total)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Errore fetch: %v\n", err)
		os.Exit(1)
	}

	apps := make([]opencity.Application, 0, len(rawApps))
	for _, r := range rawApps {
		var a opencity.Application
		if err := json.Unmarshal(r, &a); err == nil {
			apps = append(apps, a)
		}
	}

	fmt.Fprintf(os.Stderr, "Calcolo graduatoria su %d istanze...\n", len(apps))
	grad, err := graduatoria.Calcola(apps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Errore graduatoria: %v\n", err)
		os.Exit(1)
	}

	var filesScritti []string

	for _, ga := range grad.PerAnno {
		anno := strconv.Itoa(ga.Annualita)
		dir := filepath.Join("output", anno)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Errore dir %s: %v\n", dir, err)
			os.Exit(1)
		}
		ammesse := append(ga.Rette, ga.Mense...)
		perAnno := map[string][]graduatoria.RigaGraduatoria{
			"rette_ammesse.csv":        graduatoria.Filtra(ga.Rette, func(r graduatoria.RigaGraduatoria) bool { return r.Ammessa }),
			"rette_fondi_esauriti.csv": graduatoria.Filtra(ga.Rette, func(r graduatoria.RigaGraduatoria) bool { return !r.Ammessa }),
			"mense_ammesse.csv":        graduatoria.Filtra(ga.Mense, func(r graduatoria.RigaGraduatoria) bool { return r.Ammessa }),
			"mense_fondi_esauriti.csv": graduatoria.Filtra(ga.Mense, func(r graduatoria.RigaGraduatoria) bool { return !r.Ammessa }),
			"isee_da_verificare.csv":   graduatoria.Filtra(ammesse, func(r graduatoria.RigaGraduatoria) bool { return r.Ammessa && graduatoria.IseeDaVerificare(r.Istanza) }),
		}
		for name, righe := range perAnno {
			path := filepath.Join(dir, name)
			if err := scriviFile(path, righe); err != nil {
				fmt.Fprintf(os.Stderr, "Errore scrittura %s: %v\n", path, err)
				os.Exit(1)
			}
			filesScritti = append(filesScritti, path)
		}
	}

	dirEscluse := filepath.Join("output", "escluse")
	if err := os.MkdirAll(dirEscluse, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Errore dir escluse: %v\n", err)
		os.Exit(1)
	}
	escluse := map[string][]graduatoria.RigaGraduatoria{
		"ritirate.csv":      graduatoria.FiltraEscluse(grad.Escluse, "istanza ritirata"),
		"duplicati.csv":     graduatoria.FiltraEscluse(grad.Escluse, "duplicato"),
		"isee_mancante.csv": graduatoria.FiltraEscluse(grad.Escluse, "ISEE non presente o non valido"),
		"corrispettivo.csv": graduatoria.FiltraEscluse(grad.Escluse, "corrispettivo non presente"),
	}
	for name, righe := range escluse {
		path := filepath.Join(dirEscluse, name)
		if err := scriviFile(path, righe); err != nil {
			fmt.Fprintf(os.Stderr, "Errore scrittura %s: %v\n", path, err)
			os.Exit(1)
		}
		filesScritti = append(filesScritti, path)
	}

	fmt.Fprintf(os.Stderr, "\n=== RIEPILOGO ===\n")
	fmt.Fprintf(os.Stderr, "Budget per annualità: €%.2f\n", graduatoria.BudgetPerAnnualita)
	var totUsato float64
	for _, ga := range grad.PerAnno {
		usato := ga.BudgetUsatoRette + ga.BudgetUsatoMense
		totUsato += usato
		fmt.Fprintf(os.Stderr, "\n  Annualità %d:\n", ga.Annualita)
		fmt.Fprintf(os.Stderr, "    Rette ammesse:     %d  (€%.2f)\n", graduatoria.ContaAmmesse(ga.Rette), ga.BudgetUsatoRette)
		fmt.Fprintf(os.Stderr, "    Rette fuori fondi: %d\n", len(ga.Rette)-graduatoria.ContaAmmesse(ga.Rette))
		fmt.Fprintf(os.Stderr, "    Mense ammesse:     %d  (€%.2f)\n", graduatoria.ContaAmmesse(ga.Mense), ga.BudgetUsatoMense)
		fmt.Fprintf(os.Stderr, "    Mense fuori fondi: %d\n", len(ga.Mense)-graduatoria.ContaAmmesse(ga.Mense))
		fmt.Fprintf(os.Stderr, "    Budget usato:      €%.2f / €%.2f\n", usato, graduatoria.BudgetPerAnnualita)
	}
	fmt.Fprintf(os.Stderr, "\nBudget totale:       €%.2f\n", graduatoria.BudgetTotale)
	fmt.Fprintf(os.Stderr, "Budget usato totale: €%.2f\n", totUsato)
	fmt.Fprintf(os.Stderr, "Budget residuo:      €%.2f\n", graduatoria.BudgetTotale-totUsato)
	fmt.Fprintf(os.Stderr, "Escluse totali:      %d\n", len(grad.Escluse))
	fmt.Fprintf(os.Stderr, "  ritirate:          %d\n", len(escluse["ritirate.csv"]))
	fmt.Fprintf(os.Stderr, "  duplicati:         %d\n", len(escluse["duplicati.csv"]))
	fmt.Fprintf(os.Stderr, "  ISEE mancante:     %d\n", len(escluse["isee_mancante.csv"]))
	fmt.Fprintf(os.Stderr, "  corrispettivo 0:   %d\n", len(escluse["corrispettivo.csv"]))

	if err := scriviProspetto(grad, escluse); err != nil {
		fmt.Fprintf(os.Stderr, "Errore prospetto HTML: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nFile scritti:\n")
	for _, path := range filesScritti {
		fmt.Fprintf(os.Stderr, "  %s\n", path)
	}
	fmt.Fprintf(os.Stderr, "  output/index.html\n")
}

func scriviFile(path string, righe []graduatoria.RigaGraduatoria) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = ';'
	w.Write(graduatoria.HeaderGraduatoria)
	for _, r := range righe {
		cat := r.Istanza.TipoRichiesta
		if !r.Ammessa && r.NoteEsclusione != "fondi esauriti" {
			cat = "esclusa"
		}
		w.Write(graduatoria.RigaToRecord(cat, r))
	}
	w.Flush()
	return w.Error()
}
