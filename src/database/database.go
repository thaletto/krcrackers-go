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
//
// An accessor that cannot represent the column's value returns a TypeError
// rather than a zero value, so a caller that asks the wrong question finds
// out. How faithfully that can be enforced depends on what the transport
// reports about a column:
//
//   - The SQLite adapter reads each column's declared type from the driver, so
//     a mismatch is caught exactly.
//   - The D1 adapter receives values as JSON, which does not carry the
//     declared type, so it can only reject a value whose JSON type cannot
//     stand for the requested one (text for a number, a number for text).
//     A REAL column read with Int truncates on D1 where SQLite would raise a
//     TypeError. Do not rely on that difference: read a column the way its
//     declared type says, and both adapters agree.
type Row interface {
	Int(name string) (int64, error)
	Float(name string) (float64, error)
	NullableFloat(name string) (*float64, error)
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

// Capabilities describes what an adapter actually guarantees, so a caller that
// depends on a guarantee can ask instead of assuming.
type Capabilities struct {
	// AtomicTransactions is true when a Tx either commits every statement or
	// none of them. When false, a failure part-way through leaves the earlier
	// statements applied, and Rollback only compensates what it can.
	AtomicTransactions bool

	// NullParameters is true when a nil parameter binds as SQL NULL. When
	// false, the adapter substitutes the NULL literal into the statement
	// instead, which requires bare `?` placeholders.
	NullParameters bool

	// BinaryParameters is true when a []byte parameter can be bound. When
	// false, binding one is an error rather than a silent corruption.
	BinaryParameters bool
}

type DB interface {
	Query(ctx context.Context, sql string, params ...any) ([]Row, error)
	Execute(ctx context.Context, sql string, params ...any) (Result, error)
	Begin(ctx context.Context) (Tx, error)
	// Capabilities reports what this adapter guarantees. It is a property of
	// the adapter, not of the call, and does not change over its lifetime.
	Capabilities() Capabilities
	Close() error
}

// Tx is a database transaction. Call Commit to persist, Rollback to discard.
//
// Whether that is all-or-nothing is a property of the adapter, not of the
// interface: see Capabilities(). A caller whose correctness depends on
// all-or-nothing must either check, or order its statements so that a partial
// failure cannot leave it inconsistent.
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

	// BaseURL overrides the Cloudflare API endpoint. It exists so the D1
	// adapter can be pointed at a test server; production leaves it empty.
	BaseURL string
}

type LocalConfig struct {
	Path string
}

type Config struct {
	Mode  Mode
	D1    *D1Config
	Local *LocalConfig
}

func New(cfg Config) (DB, error) {
	switch cfg.Mode {
	case ModeD1:
		if cfg.D1 == nil || cfg.D1.APIToken == "" || cfg.D1.AccountID == "" || cfg.D1.DatabaseID == "" {
			return nil, fmt.Errorf("d1 mode requires CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_DATABASE_ID")
		}
		return newD1(cfg.D1)
	case ModeLocal, "":
		if cfg.Local == nil || cfg.Local.Path == "" {
			return nil, fmt.Errorf("local mode requires LOCAL_DB_PATH")
		}
		return newSQLite(cfg.Local)
	default:
		return nil, fmt.Errorf("unknown mode %q (expected %q or %q)", cfg.Mode, ModeLocal, ModeD1)
	}
}
