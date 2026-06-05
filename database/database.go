package database

import (
	"context"
	"fmt"
)

type Result struct {
	LastInsertID int64
	RowsAffected int64
}

type DB interface {
	Query(ctx context.Context, sql string, params ...any) ([]map[string]any, error)
	Execute(ctx context.Context, sql string, params ...any) (Result, error)
	Close() error
}

type Config struct {
	Mode       string
	APIToken   string
	AccountID  string
	DatabaseID string
	LocalPath  string
}

const (
	ModeProduction  = "production"
	ModeDevelopment = "development"
)

func New(cfg Config) (DB, error) {
	switch cfg.Mode {
	case ModeProduction:
		if cfg.APIToken == "" || cfg.AccountID == "" || cfg.DatabaseID == "" {
			return nil, fmt.Errorf("production mode requires CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_DATABASE_ID")
		}
		return newD1(cfg)
	case ModeDevelopment, "":
		if cfg.LocalPath == "" {
			return nil, fmt.Errorf("development mode requires LOCAL_DB_PATH")
		}
		return newSQLite(cfg)
	default:
		return nil, fmt.Errorf("unknown APP_ENV %q (expected %q or %q)", cfg.Mode, ModeDevelopment, ModeProduction)
	}
}
