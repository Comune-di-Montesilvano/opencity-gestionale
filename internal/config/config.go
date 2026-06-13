package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port             string
	DBPath           string
	OpenCityBaseURL  string
	SecretKey        string
	AdminUsernames   []string
}

func Load() (*Config, error) {
	baseURL := os.Getenv("OPENCITY_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("OPENCITY_BASE_URL non impostata")
	}
	secret := os.Getenv("SECRET_KEY")
	if secret == "" {
		return nil, fmt.Errorf("SECRET_KEY non impostata")
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("SECRET_KEY deve essere almeno 32 caratteri")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "gestionale.db"
	}

	var admins []string
	if raw := os.Getenv("ADMIN_USERNAMES"); raw != "" {
		for _, u := range splitComma(raw) {
			if u != "" {
				admins = append(admins, u)
			}
		}
	}

	return &Config{
		Port:            port,
		DBPath:          dbPath,
		OpenCityBaseURL: baseURL,
		SecretKey:       secret,
		AdminUsernames:  admins,
	}, nil
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			token := strings.TrimSpace(s[start:i])
			out = append(out, token)
			start = i + 1
		}
	}
	return out
}
