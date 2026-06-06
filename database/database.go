package database

import (
	"context"
	"fmt"
)

type Result struct {
	LastInsertID int64
	RowsAffected int64
}

// Row is the typed view of a single result row at the database seam.
// Adapters know the source type of each column and surface a TypeError
// when a caller's accessor does not match it, rather than silently
// returning a zero value.
type Row interface {
	Int(name string) (int64, error)
	Float(name string) (float64, error)
	String(name string) (string, error)
	NullableString(name string) (*string, error)
}

// TypeError reports a column whose source type does not match the
// accessor's expectation. Returned by Row accessors on mismatch.
type TypeError struct {
	Column string
	Source string
	Want   string
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("database: column %q has source type %s, want %s", e.Column, e.Source, e.Want)
}

type DB interface {
	Query(ctx context.Context, sql string, params ...any) ([]Row, error)
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
