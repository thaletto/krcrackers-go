package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// The dev export tool reads a D1 database through the query API rather than
// `wrangler d1 export`, which fails outright on this project:
//
//	D1 Export error: cannot export databases with Virtual Tables (fts5)
//
// products_fts is created by migration 0001, so that command can never export
// this database and `make dev-db` fails every time. The query API has no such
// restriction, and reading through it does not lock the remote database the way
// export does.

// DumpTable describes one table to be recreated by an export.
type DumpTable struct {
	Name string
	// CreateSQL is the verbatim CREATE statement from sqlite_master. For an
	// FTS5 virtual table this is the CREATE VIRTUAL TABLE itself, which is
	// valid SQL and recreates the table plus its shadow tables.
	CreateSQL string
	// Virtual marks an FTS5 virtual table. Its rows are derived from its
	// content table and must not be INSERTed directly; the export ends with a
	// 'rebuild' command instead.
	Virtual bool
	// NoRowID marks a WITHOUT ROWID table, which has no rowid to paginate by.
	NoRowID bool
}

// fts5ShadowSuffixes are the internal tables FTS5 maintains for a virtual
// table. They are recreated by the virtual table's own CREATE statement and
// must not be exported.
var fts5ShadowSuffixes = []string{"_data", "_idx", "_content", "_docsize", "_config"}

// Dumper streams the contents of a D1 database for the dev export tool. It is
// intentionally separate from DB: an export needs untyped values in a known
// column order, which the typed Row accessors deliberately do not expose.
type Dumper struct {
	client *d1Client
}

// NewDumper creates a Dumper targeting the remote D1 database in cfg.
func NewDumper(cfg D1Config) (*Dumper, error) {
	if cfg.APIToken == "" || cfg.AccountID == "" || cfg.DatabaseID == "" {
		return nil, fmt.Errorf("d1 export requires CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_DATABASE_ID")
	}
	c, err := newD1(&cfg)
	if err != nil {
		return nil, err
	}
	return &Dumper{client: c}, nil
}

// Close releases the dumper. D1 is stateless over HTTP, so this is a no-op.
func (d *Dumper) Close() error { return nil }

// Tables lists the exportable tables in creation order.
//
// Excluded are Cloudflare's internal _cf_ tables (not application data) and
// the FTS5 shadow tables. Shadow tables are identified by matching the naming
// scheme against the virtual tables actually present, rather than a hardcoded
// list, so an unrelated table that merely ends in _config is not dropped.
func (d *Dumper) Tables(ctx context.Context) ([]DumpTable, error) {
	rows, err := d.client.rawRows(ctx, `
		SELECT name, sql FROM sqlite_master
		WHERE type = 'table'
		  AND sql IS NOT NULL
		  AND name NOT LIKE 'sqlite_%'
		  AND name NOT LIKE '\_cf\_%' ESCAPE '\'
		ORDER BY rowid
	`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	all := make([]DumpTable, 0, len(rows))
	virtuals := make([]string, 0, 2)
	for _, r := range rows {
		name, _ := r["name"].(string)
		create, _ := r["sql"].(string)
		if name == "" || create == "" {
			continue
		}
		trimmed := strings.TrimSpace(create)
		upper := strings.ToUpper(trimmed)
		virtual := strings.HasPrefix(upper, "CREATE VIRTUAL TABLE")
		if virtual {
			virtuals = append(virtuals, name)
		}
		all = append(all, DumpTable{
			Name:      name,
			CreateSQL: strings.TrimSuffix(trimmed, ";"),
			Virtual:   virtual,
			NoRowID:   strings.Contains(upper, "WITHOUT ROWID"),
		})
	}

	shadow := map[string]bool{}
	for _, v := range virtuals {
		for _, suffix := range fts5ShadowSuffixes {
			shadow[v+suffix] = true
		}
	}

	tables := make([]DumpTable, 0, len(all))
	for _, t := range all {
		if !t.Virtual && shadow[t.Name] {
			continue
		}
		tables = append(tables, t)
	}
	return tables, nil
}

// Columns returns a table's column names in schema order. The order matters:
// the export builds INSERT statements by walking this slice, since the rows
// themselves are unordered maps.
func (d *Dumper) Columns(ctx context.Context, table string) ([]string, error) {
	rows, err := d.client.rawRows(ctx, `PRAGMA table_info(`+quoteIdent(table)+`)`)
	if err != nil {
		return nil, fmt.Errorf("columns of %q: %w", table, err)
	}
	columns := make([]string, 0, len(rows))
	for _, r := range rows {
		if name, ok := r["name"].(string); ok && name != "" {
			columns = append(columns, name)
		}
	}
	return columns, nil
}

// Rows returns one page of a table's rows, selecting the given columns
// explicitly so the caller controls the value order. Pagination needs a stable
// order, so WITHOUT ROWID tables fall back to their first column, which is the
// primary key in the SQLite tables this project creates.
func (d *Dumper) Rows(ctx context.Context, table DumpTable, columns []string, limit, offset int) ([]map[string]any, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = quoteIdent(c)
	}
	order := "rowid"
	if table.NoRowID {
		order = quoted[0]
	}
	sql := `SELECT ` + strings.Join(quoted, ", ") +
		` FROM ` + quoteIdent(table.Name) +
		` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	rows, err := d.client.rawRows(ctx, sql, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("read %q offset %d: %w", table.Name, offset, err)
	}
	return rows, nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// SQLLiteral renders a value returned by the D1 API as a SQL literal. Numbers
// arrive as float64 because JSON has one number type, so integral floats are
// printed without a decimal point to keep integer columns integral.
func SQLLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "1"
		}
		return "0"
	case float64:
		if x == float64(int64(x)) && x < 1e15 && x > -1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case string:
		return quoteString(x)
	case []byte:
		const hex = "0123456789ABCDEF"
		out := make([]byte, 0, len(x)*2+3)
		out = append(out, 'X', '\'')
		for _, b := range x {
			out = append(out, hex[b>>4], hex[b&0x0f])
		}
		return string(append(out, '\''))
	default:
		return quoteString(fmt.Sprint(x))
	}
}

func quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
