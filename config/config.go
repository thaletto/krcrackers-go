package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/thaletto/krcrackers-go/database"
)

type Config struct {
	Database database.Config
	Port     string
}

// Load reads config from the environment, with .env for shared defaults
// and .env.local for personal overrides (gitignored).
//
// APP_ENV defaults to "development" unless CLOUDFLARE_API_TOKEN is set,
// in which case it defaults to "production".
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
		Port: getEnv("PORT", "8080"),
	}

	switch appEnv {
	case "production":
		cfg.Database = database.Config{
			Mode: database.ModeD1,
			D1: &database.D1Config{
				APIToken:   os.Getenv("CLOUDFLARE_API_TOKEN"),
				AccountID:  os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
				DatabaseID: os.Getenv("CLOUDFLARE_DATABASE_ID"),
			},
		}
	case "development", "":
		cfg.Database = database.Config{
			Mode:  database.ModeLocal,
			Local: &database.LocalConfig{Path: getEnv("LOCAL_DB_PATH", ".data/dev.sqlite")},
		}
	default:
		return nil, fmt.Errorf("unknown APP_ENV %q (expected %q or %q)", appEnv, "development", "production")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
