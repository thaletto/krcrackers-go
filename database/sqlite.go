package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type sqliteDB struct {
	db *sql.DB
}

func newSQLite(cfg Config) (DB, error) {
	if dir := filepath.Dir(cfg.LocalPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", cfg.LocalPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}
	return &sqliteDB{db: db}, nil
}

func (s *sqliteDB) Query(ctx context.Context, sql string, params ...any) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, sql, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		row := make(map[string]any, len(cols))
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, c := range cols {
			row[c] = dest[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *sqliteDB) Execute(ctx context.Context, sql string, params ...any) (Result, error) {
	res, err := s.db.ExecContext(ctx, sql, params...)
	if err != nil {
		return Result{}, err
	}
	id, _ := res.LastInsertId()
	affected, _ := res.RowsAffected()
	return Result{LastInsertID: id, RowsAffected: affected}, nil
}

func (s *sqliteDB) Close() error {
	return s.db.Close()
}
