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

	handler := web.NewServer(cfg, dbConn)

	fmt.Fprintf(os.Stderr, "Gestionale OpenCity %s avviato su :%s\n", AppVersion, cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Server: %v\n", err)
		os.Exit(1)
	}
}
