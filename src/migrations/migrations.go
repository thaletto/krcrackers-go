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
// The runner records applied versions in a small bookkeeping table called
// `schema_migrations` and executes statements through the shared
// database.DB interface, so it works for backends that do not expose a
// *sql.DB (e.g. Cloudflare D1's HTTP API). Section and statement splitting are
// local: the runner is the value-add, and the one thing it needs from elsewhere
// is a version number, which it reads off the filename itself.
//
// Three properties are worth knowing:
//
//   - "Applied" means one thing. Up, Down and GetStatus all read the recorded
//     versions as a set, so they cannot report different states for the same
//     database. A gap in the bookkeeping is filled, not skipped.
//   - Applying a migration is transactional where the adapter can be. On D1,
//     which has no interactive transactions, statements run one at a time and a
//     failure leaves the version unrecorded, so the next run retries it.
//   - GetStatus only reads. It does not create the bookkeeping table.
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

	"github.com/thaletto/krcrackers-go/src/database"
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

// parseVersion reads the leading digits of an NNNN_name.sql filename. The
// convention lives here and nowhere else: the name is the version, and the
// runner does not need a library to learn that.
func parseVersion(name string) (int64, string, error) {
	base := strings.TrimSuffix(name, ".sql")
	digits, rest, ok := strings.Cut(base, "_")
	if !ok {
		return 0, "", fmt.Errorf("invalid migration filename %q: expected NNNN_name.sql", name)
	}
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid version in %q: %w", name, err)
	}
	return v, rest, nil
}

// parseSections splits a migration file into its Up and Down statement lists
// based on `-- +goose Up` / `-- +goose Down` annotation lines. Statements
// inside each section are split on `;` boundaries, unless wrapped in
// `-- +goose StatementBegin` / `-- +goose StatementEnd` markers which treat
// everything between them as a single statement (needed for triggers, etc.).
func parseSections(content string) (up, down []string) {
	section := ""
	var buf strings.Builder
	inBlock := false
	flushStatement := func() {
		s := strings.TrimSpace(buf.String())
		if s != "" {
			switch section {
			case "up":
				up = append(up, s)
			case "down":
				down = append(down, s)
			}
		}
		buf.Reset()
	}
	flushSemicolons := func() {
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
			if inBlock {
				flushStatement()
				inBlock = false
			} else {
				flushSemicolons()
			}
			section = "up"
			continue
		case strings.HasPrefix(t, "-- +goose Down"):
			if inBlock {
				flushStatement()
				inBlock = false
			} else {
				flushSemicolons()
			}
			section = "down"
			continue
		case strings.HasPrefix(t, "-- +goose StatementBegin"):
			flushSemicolons()
			inBlock = true
			continue
		case strings.HasPrefix(t, "-- +goose StatementEnd"):
			flushStatement()
			inBlock = false
			continue
		}
		if section != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	if inBlock {
		flushStatement()
	} else {
		flushSemicolons()
	}
	return
}

// splitStatements splits a SQL string into statements on top-level `;`
// boundaries.
//
// It walks the text once, so comments and quoted text are handled by the same
// pass that finds the boundaries. That matters: a `;` inside a string literal
// or a comment is not a boundary, and a two-stage strip-then-split pipeline
// disagrees about that — it recognises quotes while stripping comments and then
// ignores them while splitting, so `DEFAULT ';'` splits mid-literal and the
// migration still records as applied.
//
// A `?`, a `[...]` identifier and a doubled quote inside a quoted run are all
// handled, because DDL in this project uses all three.
func splitStatements(sql string) []string {
	var (
		out      []string
		buf      strings.Builder
		quote    byte
		lineCmt  bool
		blockCmt bool
	)
	flush := func() {
		if s := strings.TrimSpace(buf.String()); s != "" {
			out = append(out, s)
		}
		buf.Reset()
	}

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		if lineCmt {
			if c == '\n' {
				lineCmt = false
				// Keep the newline so tokens either side stay separated.
				buf.WriteByte(c)
			}
			continue
		}
		if blockCmt {
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				blockCmt = false
				i++
			}
			continue
		}
		if quote != 0 {
			buf.WriteByte(c)
			if c == quote {
				// A doubled quote inside a quoted run is an escape, not the end.
				if i+1 < len(sql) && sql[i+1] == quote {
					buf.WriteByte(quote)
					i++
				} else {
					quote = 0
				}
			}
			continue
		}

		switch {
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			lineCmt = true
			i++
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			blockCmt = true
			i++
		case c == '\'' || c == '"' || c == '`':
			quote = c
			buf.WriteByte(c)
		case c == '[':
			quote = ']'
			buf.WriteByte(c)
		case c == ';':
			flush()
		default:
			buf.WriteByte(c)
		}
	}
	flush()
	return out
}

// versionTableExists reports whether the bookkeeping table is present, without
// creating it. A status query must not write to the schema.
func versionTableExists(ctx context.Context, db database.DB) (bool, error) {
	rows, err := db.Query(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", versionTable)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// appliedVersions reads the bookkeeping table as the set of versions recorded
// as applied. This is the one definition of "applied": Up, Down and GetStatus
// all read it, so they cannot disagree about a database's state.
//
// A missing table means nothing has been applied yet, which is a normal state
// for a fresh database rather than an error.
func appliedVersions(ctx context.Context, db database.DB) (map[int64]bool, error) {
	exists, err := versionTableExists(ctx, db)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[int64]bool{}, nil
	}
	rows, err := db.Query(ctx, "SELECT version FROM "+versionTable)
	if err != nil {
		return nil, err
	}
	applied := make(map[int64]bool, len(rows))
	for _, r := range rows {
		v, _ := r.Int("version")
		applied[v] = true
	}
	return applied, nil
}

func ensureVersionTable(ctx context.Context, db database.DB) error {
	_, err := db.Execute(ctx,
		"CREATE TABLE IF NOT EXISTS "+versionTable+
			" (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)")
	return err
}

func recordApplied(ctx context.Context, r runner, version int64) error {
	_, err := r.exec(ctx, "INSERT INTO "+versionTable+" (version) VALUES (?)", version)
	return err
}

func recordRolledBack(ctx context.Context, r runner, version int64) error {
	_, err := r.exec(ctx, "DELETE FROM "+versionTable+" WHERE version = ?", version)
	return err
}

// runner carries the two things a migration run needs, so apply and rollback
// share one implementation whether or not a transaction is in play.
type runner struct {
	db database.DB
	tx database.Tx
}

func (r runner) exec(ctx context.Context, sql string, params ...any) (database.Result, error) {
	if r.tx != nil {
		return r.tx.Execute(ctx, sql, params...)
	}
	return r.db.Execute(ctx, sql, params...)
}

// begin returns a transactional runner when the adapter can offer one, and a
// non-transactional runner when it cannot. D1's HTTP API has no interactive
// transactions, so a migration there applies statement by statement: a failure
// half-way leaves the schema partly applied and the version unrecorded, so the
// next run tries that migration again.
func begin(ctx context.Context, db database.DB) (runner, error) {
	if !db.Capabilities().AtomicTransactions {
		return runner{db: db}, nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return runner{}, fmt.Errorf("begin tx: %w", err)
	}
	return runner{db: db, tx: tx}, nil
}

func (r runner) commit() error {
	if r.tx == nil {
		return nil
	}
	return r.tx.Commit()
}

func (r runner) abort() {
	if r.tx != nil {
		_ = r.tx.Rollback()
	}
}

// inTransaction runs body inside a transaction when the adapter supports one,
// committing on success and rolling back on failure.
func inTransaction(ctx context.Context, db database.DB, version int64, body func(runner) error) error {
	r, err := begin(ctx, db)
	if err != nil {
		return err
	}
	if err := body(r); err != nil {
		r.abort()
		return err
	}
	if err := r.commit(); err != nil {
		r.abort()
		return fmt.Errorf("committing %04d: %w", version, err)
	}
	return nil
}

// Up applies every migration that is not already recorded as applied, in
// ascending order, and returns the number applied.
//
// It reads the recorded versions as a set rather than as a high-water mark, so
// a gap in the bookkeeping (a version recorded, a later one not) is filled
// rather than skipped. That is the same view GetStatus reports.
func Up(ctx context.Context, db database.DB) (int, error) {
	migs, err := load()
	if err != nil {
		return 0, err
	}
	if err := ensureVersionTable(ctx, db); err != nil {
		return 0, err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, m := range migs {
		if applied[m.Version] {
			continue
		}
		if err := apply(ctx, db, m); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// apply runs one migration's Up statements and records it as applied.
func apply(ctx context.Context, db database.DB, m migration) error {
	log.Printf("migrate: applying %04d_%s (up)", m.Version, m.Name)
	return inTransaction(ctx, db, m.Version, func(r runner) error {
		for _, stmt := range m.Up {
			if _, err := r.exec(ctx, stmt); err != nil {
				return fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
			}
		}
		if err := recordApplied(ctx, r, m.Version); err != nil {
			return fmt.Errorf("recording %04d_%s: %w", m.Version, m.Name, err)
		}
		return nil
	})
}

// Down rolls back the highest recorded version that still has a migration file.
func Down(ctx context.Context, db database.DB) error {
	migs, err := load()
	if err != nil {
		return err
	}
	if err := ensureVersionTable(ctx, db); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	var target *migration
	for i := len(migs) - 1; i >= 0; i-- {
		if applied[migs[i].Version] {
			target = &migs[i]
			break
		}
	}
	if target == nil {
		// A version recorded with no matching file means the database is at a
		// state this build cannot describe. Reporting "nothing to roll back"
		// would leave that state looking deliberate, so name it.
		return unknownVersionsError(applied, migs)
	}

	log.Printf("migrate: rolling back %04d_%s (down)", target.Version, target.Name)
	return inTransaction(ctx, db, target.Version, func(r runner) error {
		for _, stmt := range target.Down {
			if _, err := r.exec(ctx, stmt); err != nil {
				return fmt.Errorf("rollback %04d_%s: %w", target.Version, target.Name, err)
			}
		}
		if err := recordRolledBack(ctx, r, target.Version); err != nil {
			return fmt.Errorf("recording rollback %04d_%s: %w", target.Version, target.Name, err)
		}
		return nil
	})
}

// unknownVersionsError reports recorded versions that have no migration file,
// or nil when there are none.
func unknownVersionsError(applied map[int64]bool, migs []migration) error {
	known := make(map[int64]bool, len(migs))
	for _, m := range migs {
		known[m.Version] = true
	}
	var unknown []int64
	for v := range applied {
		if !known[v] {
			unknown = append(unknown, v)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i] < unknown[j] })
	names := make([]string, 0, len(unknown))
	for _, v := range unknown {
		names = append(names, fmt.Sprintf("%04d", v))
	}
	return fmt.Errorf(
		"migrate: %s recorded as applied but this build has no such migration file",
		strings.Join(names, ", "))
}

// GetStatus returns the applied/pending state of every known migration. It only
// reads: asking about the schema must not change it, so it does not create the
// bookkeeping table and a database with no migrations reports all pending.
func GetStatus(ctx context.Context, db database.DB) ([]Status, error) {
	migs, err := load()
	if err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(migs))
	for _, m := range migs {
		out = append(out, Status{Version: m.Version, Name: m.Name, Applied: applied[m.Version]})
	}
	return out, nil
}
