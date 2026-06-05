package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv     string
	APIToken   string
	AccountID  string
	DatabaseID string
	LocalPath  string
	Port       string
}

// Load reads config from the environment, with .env and .env.local as
// fallback sources. .env provides shared defaults; .env.local overrides
// them and is intended for personal overrides (gitignored).
//
// APP_ENV defaults to "production" when CLOUDFLARE_API_TOKEN is set,
// otherwise "development" — so a fresh `go run .` in a clone without
// secrets just works in dev mode.
func Load() (*Config, error) {
	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading .env: %w", err)
	}
	if err := godotenv.Overload(".env.local"); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading .env.local: %w", err)
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
			appEnv = "production"
		} else {
			appEnv = "development"
		}
	}

	cfg := &Config{
		AppEnv:     appEnv,
		APIToken:   os.Getenv("CLOUDFLARE_API_TOKEN"),
		AccountID:  os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		DatabaseID: os.Getenv("CLOUDFLARE_DATABASE_ID"),
		LocalPath:  getEnv("LOCAL_DB_PATH", ".data/dev.sqlite"),
		Port:       getEnv("PORT", "8080"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
