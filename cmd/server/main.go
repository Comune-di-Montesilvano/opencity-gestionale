package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"opencity-gestionale/internal/config"
	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/web"

	_ "opencity-gestionale/internal/graduatoria/mense" // registra engine
)

var AppVersion = "dev"

func main() {
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
