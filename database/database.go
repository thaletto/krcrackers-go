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
	Begin(ctx context.Context) (Tx, error)
	Close() error
}

// Tx is a database transaction. Call Commit to persist, Rollback to discard.
// SQLite adapters wrap *sql.Tx for real atomicity. D1 adapters buffer
// statements and execute them on Commit (best-effort; not truly atomic).
type Tx interface {
	Query(ctx context.Context, sql string, params ...any) ([]Row, error)
	Execute(ctx context.Context, sql string, params ...any) (Result, error)
	Commit() error
	Rollback() error
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
