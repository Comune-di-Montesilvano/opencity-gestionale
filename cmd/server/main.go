package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"opencity-gestionale/internal/config"
	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web"

	_ "opencity-gestionale/internal/graduatoria/generic" // registra engine generico
)

var AppVersion = "dev"

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
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		url := "http://localhost:" + port + "/health"
		if len(os.Args) > 2 {
			url = os.Args[2]
		}
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
			os.Exit(1)
		}
		os.Exit(0)
	}

	loadDotEnv()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configurazione: %v\n", err)
		os.Exit(1)
	}

	dbConn, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database: %v\n", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	// Pulizia sessioni scadute all'avvio e ogni 6 ore
	db.PulisciSessioniScadute(dbConn)
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := db.PulisciSessioniScadute(dbConn); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup sessioni: %v\n", err)
			}
		}
	}()

	branding, err := opencity.FetchBranding(cfg.OpenCityBaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Impossibile recuperare il branding di OpenCity: %v. Uso branding di default.\n", err)
		branding = &opencity.Branding{
			Nome: "Gestionale OpenCity",
		}
	} else {
		fmt.Fprintf(os.Stderr, "Branding caricato con successo per: %s\n", branding.Nome)
	}

	handler := web.NewServer(cfg, dbConn, branding, AppVersion)

	fmt.Fprintf(os.Stderr, "Gestionale OpenCity %s avviato su :%s\n", AppVersion, cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Server: %v\n", err)
		os.Exit(1)
	}
}
