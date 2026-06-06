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

type Mode string

const (
	ModeLocal Mode = "local"
	ModeD1    Mode = "d1"
)

type D1Config struct {
	APIToken   string
	AccountID  string
	DatabaseID string
}

type LocalConfig struct {
	Path string
}

type Config struct {
	Mode  Mode
	D1    *D1Config
	Local *LocalConfig

	APIToken   string
	AccountID  string
	DatabaseID string
	LocalPath  string
}

func New(cfg Config) (DB, error) {
	switch cfg.Mode {
	case ModeD1:
		if cfg.D1 == nil || cfg.D1.APIToken == "" || cfg.D1.AccountID == "" || cfg.D1.DatabaseID == "" {
			return nil, fmt.Errorf("d1 mode requires CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_DATABASE_ID")
		}
		cfg.APIToken = cfg.D1.APIToken
		cfg.AccountID = cfg.D1.AccountID
		cfg.DatabaseID = cfg.D1.DatabaseID
		return newD1(cfg)
	case ModeLocal, "":
		if cfg.Local == nil || cfg.Local.Path == "" {
			return nil, fmt.Errorf("local mode requires LOCAL_DB_PATH")
		}
		cfg.LocalPath = cfg.Local.Path
		return newSQLite(cfg)
	default:
		return nil, fmt.Errorf("unknown mode %q (expected %q or %q)", cfg.Mode, ModeLocal, ModeD1)
	}
}
