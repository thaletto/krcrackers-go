// Package migrations provides versioned, ordered schema migrations for
// both the local SQLite database and the remote Cloudflare D1 database.
//
// Migration files live in this directory with the naming convention
//
//	NNNN_name.sql
//
// where NNNN is a monotonically increasing version number. Each file uses
// goose-style annotations to delimit the up and down sections:
//
//	-- +goose Up
//	CREATE TABLE foo (...);
//
//	-- +goose Down
//	DROP TABLE foo;
//
// The runner tracks applied versions in a small bookkeeping table called
// `schema_migrations` and executes statements one at a time through the
// shared database.DB interface, so it works for backends that do not
// expose a *sql.DB (e.g. Cloudflare D1's HTTP API).
package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/thaletto/krcrackers-go/database"
)

//go:embed *.sql
var sqlFS embed.FS

const versionTable = "schema_migrations"

type migration struct {
	Version int64
	Name    string
	Up      []string
	Down    []string
}

type Status struct {
	Version int64
	Name    string
	Applied bool
}

func (s Status) String() string {
	state := "pending"
	if s.Applied {
		state = "applied"
	}
	return fmt.Sprintf("%04d_%s\t%s", s.Version, s.Name, state)
}

func load() ([]migration, error) {
	entries, err := fs.ReadDir(sqlFS, ".")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := sqlFS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		m, err := parseFile(e.Name(), string(data))
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func parseFile(name, content string) (migration, error) {
	v, n, err := parseVersion(name)
	if err != nil {
		return migration{}, err
	}
	up, down := parseSections(content)
	if len(up) == 0 {
		return migration{}, fmt.Errorf("migration %s: no -- +goose Up section", name)
	}
	return migration{Version: v, Name: n, Up: up, Down: down}, nil
}

func parseVersion(name string) (int64, string, error) {
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid migration filename %q: expected NNNN_name.sql", name)
	}
	v, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid version in %q: %w", name, err)
	}
	return v, parts[1], nil
}

// parseSections splits a migration file into its Up and Down statement lists
// based on `-- +goose Up` / `-- +goose Down` annotation lines. Statements
// inside each section are split on `;` boundaries.
func parseSections(content string) (up, down []string) {
	section := ""
	var buf strings.Builder
	flush := func() {
		for _, s := range splitStatements(buf.String()) {
			switch section {
			case "up":
				up = append(up, s)
			case "down":
				down = append(down, s)
			}
		}
		buf.Reset()
	}
	for line := range strings.SplitSeq(content, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "-- +goose Up"):
			flush()
			section = "up"
			continue
		case strings.HasPrefix(t, "-- +goose Down"):
			flush()
			section = "down"
			continue
		case strings.HasPrefix(t, "-- +goose StatementBegin"),
			strings.HasPrefix(t, "-- +goose StatementEnd"):
			continue
		}
		if section != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	flush()
	return
}

// splitStatements splits a SQL string on `;` boundaries, ignoring lines that
// are comments or blank. Note: this does not handle `;` inside string literals;
// none of the current migrations rely on that, and DDL rarely does.
func splitStatements(sql string) []string {
	var out []string
	for raw := range strings.SplitSeq(sql, ";") {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "--") {
			continue
		}
		out = append(out, s)
	}
	return out
}

func ensureVersionTable(ctx context.Context, db database.DB) error {
	_, err := db.Execute(ctx,
		"CREATE TABLE IF NOT EXISTS "+versionTable+
			" (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)")
	return err
}

func currentVersion(ctx context.Context, db database.DB) (int64, error) {
	rows, err := db.Query(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM "+versionTable)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	for _, v := range rows[0] {
		switch x := v.(type) {
		case int64:
			return x, nil
		case float64:
			return int64(x), nil
		case int:
			return int64(x), nil
		}
	}
	return 0, nil
}

func recordApplied(ctx context.Context, db database.DB, version int64) error {
	_, err := db.Execute(ctx,
		"INSERT INTO "+versionTable+" (version) VALUES (?)", version)
	return err
}

func recordRolledBack(ctx context.Context, db database.DB, version int64) error {
	_, err := db.Execute(ctx,
		"DELETE FROM "+versionTable+" WHERE version = ?", version)
	return err
}

// Up applies every migration whose version is greater than the current
// recorded version, in ascending order.
func Up(ctx context.Context, db database.DB) (int, error) {
	migs, err := load()
	if err != nil {
		return 0, err
	}
	if err := ensureVersionTable(ctx, db); err != nil {
		return 0, err
	}
	current, err := currentVersion(ctx, db)
	if err != nil {
		return 0, err
	}
	applied := 0
	for _, m := range migs {
		if m.Version <= current {
			continue
		}
		log.Printf("migrate: applying %04d_%s (up)", m.Version, m.Name)
		for _, stmt := range m.Up {
			if _, err := db.Execute(ctx, stmt); err != nil {
				return applied, fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
			}
		}
		if err := recordApplied(ctx, db, m.Version); err != nil {
			return applied, fmt.Errorf("recording %04d_%s: %w", m.Version, m.Name, err)
		}
		applied++
	}
	return applied, nil
}

// Down rolls back the most recently applied migration.
func Down(ctx context.Context, db database.DB) error {
	migs, err := load()
	if err != nil {
		return err
	}
	if err := ensureVersionTable(ctx, db); err != nil {
		return err
	}
	current, err := currentVersion(ctx, db)
	if err != nil {
		return err
	}
	var target *migration
	for i := len(migs) - 1; i >= 0; i-- {
		if migs[i].Version == current {
			target = &migs[i]
			break
		}
	}
	if target == nil {
		log.Printf("migrate: nothing to roll back (current version: %d)", current)
		return nil
	}
	log.Printf("migrate: rolling back %04d_%s (down)", target.Version, target.Name)
	for _, stmt := range target.Down {
		if _, err := db.Execute(ctx, stmt); err != nil {
			return fmt.Errorf("rollback %04d_%s: %w", target.Version, target.Name, err)
		}
	}
	if err := recordRolledBack(ctx, db, target.Version); err != nil {
		return fmt.Errorf("recording rollback %04d_%s: %w", target.Version, target.Name, err)
	}
	return nil
}

// GetStatus returns the applied/pending state of every known migration.
func GetStatus(ctx context.Context, db database.DB) ([]Status, error) {
	migs, err := load()
	if err != nil {
		return nil, err
	}
	if err := ensureVersionTable(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, "SELECT version FROM "+versionTable)
	if err != nil {
		return nil, err
	}
	applied := make(map[int64]bool)
	for _, r := range rows {
		for _, v := range r {
			switch x := v.(type) {
			case int64:
				applied[x] = true
			case float64:
				applied[int64(x)] = true
			case int:
				applied[int64(x)] = true
			}
		}
	}
	out := make([]Status, 0, len(migs))
	for _, m := range migs {
		out = append(out, Status{Version: m.Version, Name: m.Name, Applied: applied[m.Version]})
	}
	return out, nil
}
