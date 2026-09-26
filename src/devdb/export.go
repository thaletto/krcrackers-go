// Package devdb exports the production D1 database to a SQL file that can be
// loaded into the local SQLite file, replacing `wrangler d1 export` in the
// `dev-db` Make target. See database.NewDumper for why the export command
// cannot be used.
package devdb

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/thaletto/krcrackers-go/src/database"
)

// pageSize is how many rows are read per D1 request. D1 caps a single response,
// so the export walks each table in pages.
const pageSize = 500

// Result summarises what an export wrote.
type Result struct {
	Tables    int
	Rows      int
	Rebuilt   []string
	SkippedNo []string
}

// Export writes a self-contained SQL script that recreates the dumped database:
// every table, every row, and an FTS rebuild for each virtual table. Loading the
// script into an empty SQLite file yields a working local database, and loading
// it over an existing one is a no-op only if the file is deleted first.
//
// Triggers are intentionally not exported. The three products_fts triggers
// would fire on every product INSERT during the load, doing the same work as
// the single trailing 'rebuild' that the export emits instead.
func Export(ctx context.Context, d *database.Dumper, w io.Writer) (Result, error) {
	var res Result

	tables, err := d.Tables(ctx)
	if err != nil {
		return res, err
	}
	res.Tables = len(tables)

	// Foreign keys are off by default in SQLite and orders/order_items reference
	// users, so tables can be created and filled in any order. The pragma has to
	// precede BEGIN, since it is a no-op inside a transaction.
	if _, err := fmt.Fprintln(w, "PRAGMA foreign_keys=OFF;"); err != nil {
		return res, err
	}
	if _, err := fmt.Fprintln(w, "BEGIN TRANSACTION;"); err != nil {
		return res, err
	}

	for _, table := range tables {
		if _, err := fmt.Fprintf(w, "\n-- %s\n%s;\n", table.Name, table.CreateSQL); err != nil {
			return res, err
		}
		if table.Virtual {
			// Recreated above; its rows are derived from its content table.
			res.Rebuilt = append(res.Rebuilt, table.Name)
			continue
		}

		columns, err := d.Columns(ctx, table.Name)
		if err != nil {
			return res, err
		}
		if len(columns) == 0 {
			res.SkippedNo = append(res.SkippedNo, table.Name)
			continue
		}

		written, err := writeRows(ctx, d, w, table, columns)
		if err != nil {
			return res, err
		}
		res.Rows += written
	}

	// After all content tables are populated. This is the FTS5 idiom for
	// rebuilding a virtual table from its external content table.
	for _, name := range res.Rebuilt {
		if _, err := fmt.Fprintf(w, "\nINSERT INTO %q(%q) VALUES('rebuild');\n", name, name); err != nil {
			return res, err
		}
	}

	if _, err := fmt.Fprintln(w, "\nCOMMIT;"); err != nil {
		return res, err
	}
	return res, nil
}

// writeRows emits one INSERT statement per page, each closed with its own
// semicolon, so a multi-page table is a series of valid statements rather than
// one statement whose terminator is only known after the last page is read.
func writeRows(ctx context.Context, d *database.Dumper, w io.Writer, table database.DumpTable, columns []string) (int, error) {
	columnList := quoteAll(columns)
	total := 0
	for offset := 0; ; offset += pageSize {
		rows, err := d.Rows(ctx, table, columns, pageSize, offset)
		if err != nil {
			return total, err
		}
		if len(rows) == 0 {
			return total, nil
		}
		if _, err := fmt.Fprintf(w, "INSERT INTO %q (%s) VALUES\n", table.Name, columnList); err != nil {
			return total, err
		}
		for i, row := range rows {
			if i > 0 {
				if _, err := fmt.Fprint(w, ",\n"); err != nil {
					return total, err
				}
			}
			if _, err := fmt.Fprintf(w, "  (%s)", valueList(row, columns)); err != nil {
				return total, err
			}
		}
		if _, err := fmt.Fprintln(w, ";"); err != nil {
			return total, err
		}
		total += len(rows)
		if len(rows) < pageSize {
			return total, nil
		}
	}
}

func valueList(row map[string]any, columns []string) string {
	literals := make([]string, len(columns))
	for i, c := range columns {
		literals[i] = database.SQLLiteral(row[c])
	}
	return strings.Join(literals, ", ")
}

func quoteAll(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = `"` + strings.ReplaceAll(n, `"`, `""`) + `"`
	}
	return strings.Join(quoted, ", ")
}
