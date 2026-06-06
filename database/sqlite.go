package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func (s *sqliteDB) Query(ctx context.Context, sql string, params ...any) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, sql, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	types := make(map[string]string, len(colTypes))
	cols := make([]string, len(colTypes))
	for i, ct := range colTypes {
		cols[i] = ct.Name()
		types[ct.Name()] = strings.ToUpper(ct.DatabaseTypeName())
	}

	out := make([]Row, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := &sqliteRow{values: make(map[string]any, len(cols)), types: types}
		for i, c := range cols {
			row.values[c] = vals[i]
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

type sqliteRow struct {
	values map[string]any
	types  map[string]string
}

func (r *sqliteRow) lookup(name string) (any, string, error) {
	v, ok := r.values[name]
	if !ok {
		return nil, "", fmt.Errorf("database: no column %q", name)
	}
	return v, r.types[name], nil
}

func (r *sqliteRow) Int(name string) (int64, error) {
	v, t, err := r.lookup(name)
	if err != nil {
		return 0, err
	}
	if !isIntegerType(t) {
		return 0, &TypeError{Column: name, Source: t, Want: "integer"}
	}
	return toInt64(v)
}

func (r *sqliteRow) Float(name string) (float64, error) {
	v, t, err := r.lookup(name)
	if err != nil {
		return 0, err
	}
	if !isFloatType(t) {
		return 0, &TypeError{Column: name, Source: t, Want: "float"}
	}
	return toFloat64(v)
}

func (r *sqliteRow) String(name string) (string, error) {
	v, t, err := r.lookup(name)
	if err != nil {
		return "", err
	}
	if !isStringType(t) {
		return "", &TypeError{Column: name, Source: t, Want: "text"}
	}
	if v == nil {
		return "", nil
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	if b, ok := v.([]byte); ok {
		return string(b), nil
	}
	return "", &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "text"}
}

func (r *sqliteRow) NullableString(name string) (*string, error) {
	v, t, err := r.lookup(name)
	if err != nil {
		return nil, err
	}
	if !isStringType(t) {
		return nil, &TypeError{Column: name, Source: t, Want: "text"}
	}
	if v == nil {
		return nil, nil
	}
	if s, ok := v.(string); ok {
		return &s, nil
	}
	if b, ok := v.([]byte); ok {
		s := string(b)
		return &s, nil
	}
	return nil, &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "text"}
}

func isIntegerType(t string) bool {
	switch t {
	case "INTEGER", "INT", "INT2", "INT8", "TINYINT", "SMALLINT", "BIGINT", "MEDIUMINT":
		return true
	}
	return false
}

func isFloatType(t string) bool {
	switch t {
	case "REAL", "FLOAT", "DOUBLE", "NUMERIC", "DECIMAL":
		return true
	}
	return false
}

func isStringType(t string) bool {
	switch t {
	case "TEXT", "VARCHAR", "CHAR", "CLOB", "STRING":
		return true
	}
	return false
}

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case float64:
		return int64(x), nil
	}
	return 0, &TypeError{Column: "", Source: fmt.Sprintf("%T", v), Want: "integer"}
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int:
		return float64(x), nil
	}
	return 0, &TypeError{Column: "", Source: fmt.Sprintf("%T", v), Want: "float"}
}
