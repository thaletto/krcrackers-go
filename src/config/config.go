// Package config handles application configuration loaded from environment
// variables and .env files. It supports both local (SQLite) and production
// (Cloudflare D1) database modes.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/thaletto/krcrackers-go/src/database"
)

// Config holds all application configuration values.
type Config struct {
	Database     database.Config
	Port         string
	R2           R2Config
	JWT          JWTConfig
	WhatsApp     WhatsAppConfig
	IsProduction bool
}

type R2Config struct {
	AccountID     string
	AccessKeyID   string
	SecretKey     string
	BucketName    string
	PublicURLBase string
}

type JWTConfig struct {
	Secret string
}

type WhatsAppConfig struct {
	APIToken     string
	PhoneNumberID string
	FromNumber   string
}

// Load reads configuration from .env and .env.local files, then selects
// the database backend based on APP_ENV (defaults to "development").
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
		Port:         getEnv("PORT", "8080"),
		IsProduction: appEnv == "production",
		R2: R2Config{
			AccountID:     os.Getenv("R2_ACCOUNT_ID"),
			AccessKeyID:   os.Getenv("R2_ACCESS_KEY_ID"),
			SecretKey:     os.Getenv("R2_SECRET_ACCESS_KEY"),
			BucketName:    os.Getenv("R2_BUCKET_NAME"),
			PublicURLBase: os.Getenv("R2_PUBLIC_URL_BASE"),
		},
		JWT: JWTConfig{
			Secret: os.Getenv("JWT_SECRET"),
		},
		WhatsApp: WhatsAppConfig{
			APIToken:      os.Getenv("WHATSAPP_API_TOKEN"),
			PhoneNumberID: os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
			FromNumber:    os.Getenv("WHATSAPP_FROM_NUMBER"),
		},
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
