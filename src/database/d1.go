package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/d1"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/cloudflare/cloudflare-go/v7/packages/pagination"
)

type d1Client struct {
	inner      *cloudflare.Client
	accountID  string
	databaseID string
}

func newD1(cfg *D1Config) (*d1Client, error) {
	opts := []option.RequestOption{option.WithAPIToken(cfg.APIToken)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &d1Client{
		inner:      cloudflare.NewClient(opts...),
		accountID:  cfg.AccountID,
		databaseID: cfg.DatabaseID,
	}, nil
}

func (c *d1Client) Query(ctx context.Context, sql string, params ...any) ([]Row, error) {
	values, err := c.rawRows(ctx, sql, params...)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(values))
	for _, m := range values {
		rows = append(rows, &d1Row{values: m})
	}
	return rows, nil
}

// rawRows runs a query and returns each row as an untyped column-keyed map.
// Unlike Query it imposes no type expectations, which is what the dev export
// tool needs in order to reproduce values it has no schema knowledge of.
func (c *d1Client) rawRows(ctx context.Context, sql string, params ...any) ([]map[string]any, error) {
	res, err := c.run(ctx, sql, params)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for _, r := range res.Result {
		for _, row := range r.Results {
			m, err := coerceRow(row)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
	}
	return out, nil
}

func coerceRow(row any) (map[string]any, error) {
	if m, ok := row.(map[string]any); ok {
		return m, nil
	}
	b, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *d1Client) Execute(ctx context.Context, sql string, params ...any) (Result, error) {
	res, err := c.run(ctx, sql, params)
	if err != nil {
		return Result{}, err
	}

	var out Result
	for _, r := range res.Result {
		if r.Meta.LastRowID > 0 {
			out.LastInsertID = int64(r.Meta.LastRowID)
		}
		out.RowsAffected += int64(r.Meta.Changes)
	}
	return out, nil
}

func (c *d1Client) Begin(ctx context.Context) (Tx, error) {
	return &d1Tx{client: c, ctx: ctx}, nil
}

// Capabilities reports that D1's HTTP API cannot offer what the SQLite adapter
// does: there are no interactive transactions, parameters are strings rather
// than values, and binary is not transportable.
func (c *d1Client) Capabilities() Capabilities {
	return Capabilities{
		AtomicTransactions: false,
		NullParameters:     true,
		BinaryParameters:   false,
	}
}

func (c *d1Client) Close() error {
	return nil
}

// d1Tx is a best-effort transaction adapter for D1's HTTP API.
//
// D1 has no interactive transactions over HTTP, so every statement runs
// immediately (one HTTP round trip each) and Commit is a no-op. Successful
// INSERTs are tracked so Rollback can delete them in reverse order as
// compensation. This is not atomic: a failure after an UPDATE or a DELETE
// leaves that statement applied, and a crash between statements leaves partial
// state. Capabilities().AtomicTransactions says so, and a caller that must not
// depend on a partial rollback has to order its statements accordingly.
type d1Tx struct {
	client  *d1Client
	ctx     context.Context
	inserts []d1Insert
}

type d1Insert struct {
	table string
	id    int64
}

func (t *d1Tx) Query(ctx context.Context, sql string, params ...any) ([]Row, error) {
	return t.client.Query(ctx, sql, params...)
}

func (t *d1Tx) Execute(ctx context.Context, sql string, params ...any) (Result, error) {
	res, err := t.client.Execute(ctx, sql, params...)
	if err != nil {
		return Result{}, err
	}
	if id := res.LastInsertID; id > 0 && isInsertStmt(sql) {
		if table := extractTableName(sql); table != "" {
			t.inserts = append(t.inserts, d1Insert{table: table, id: id})
		}
	}
	return res, nil
}

func (t *d1Tx) Commit() error {
	t.inserts = nil
	return nil
}

// Rollback compensates the INSERTs this transaction made, newest first, and
// reports the first failure. A caller on an error path usually discards this
// error; it is returned so that a caller which can act on it may.
func (t *d1Tx) Rollback() error {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var firstErr error
	for i := len(t.inserts) - 1; i >= 0; i-- {
		ins := t.inserts[i]
		if _, err := t.client.Execute(ctx, "DELETE FROM "+ins.table+" WHERE id = ?", ins.id); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("database: compensating delete from %s: %w", ins.table, err)
		}
	}
	t.inserts = nil
	return firstErr
}

func isInsertStmt(sql string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(sql))
	return strings.HasPrefix(trimmed, "insert")
}

// extractTableName returns the table an INSERT targets, for compensation. It
// assumes a single-row primary key named `id`, which is what every table this
// project compensates for has; a table without one is simply not compensated.
func extractTableName(sql string) string {
	const prefix = "insert into "
	lower := strings.ToLower(sql)
	idx := strings.Index(lower, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := start
	for end < len(sql) && sql[end] != ' ' && sql[end] != '(' && sql[end] != '\n' {
		end++
	}
	return strings.Trim(sql[start:end], `"`+"`"+`[]`)
}

func (c *d1Client) run(ctx context.Context, sql string, params []any) (*pagination.SinglePage[d1.QueryResult], error) {
	bound, stmt, err := paramsToStrings(sql, params)
	if err != nil {
		return nil, err
	}
	if stmt != "" {
		sql = stmt
	}
	return c.inner.D1.Database.Query(ctx, c.databaseID, d1.DatabaseQueryParams{
		AccountID: cloudflare.F(c.accountID),
		Body: d1.DatabaseQueryParamsBodyD1SingleQuery{
			Sql:    cloudflare.F(sql),
			Params: cloudflare.F(bound),
		},
	})
}

// paramsToStrings encodes parameters the way database/sql would, so both
// adapters send the same value for the same input.
//
// The D1 API takes a []string, which cannot express SQL NULL and cannot carry
// binary. A nil parameter is therefore substituted into the statement as the
// NULL literal rather than sent as an empty string, and a []byte parameter is
// an error rather than a string that will not round-trip. Anything the
// standard converter cannot handle is an error too: stringifying it with
// fmt.Sprint is how a *int used to reach D1 as a pointer address.
func paramsToStrings(stmt string, params []any) ([]string, string, error) {
	bound := make([]string, 0, len(params))
	var nulls []int

	for i, p := range params {
		if isNilParam(p) {
			nulls = append(nulls, i)
			continue
		}
		encoded, err := encodeParam(p)
		if err != nil {
			return nil, "", fmt.Errorf("database: parameter %d: %w", i+1, err)
		}
		bound = append(bound, encoded)
	}

	if len(nulls) == 0 {
		return bound, "", nil
	}
	rewritten, err := substituteNulls(stmt, nulls)
	if err != nil {
		return nil, "", err
	}
	return bound, rewritten, nil
}

func isNilParam(p any) bool {
	if p == nil {
		return true
	}
	switch v := p.(type) {
	case *string:
		return v == nil
	case *int:
		return v == nil
	case *int64:
		return v == nil
	case *float64:
		return v == nil
	case *bool:
		return v == nil
	}
	return false
}

// encodeParam renders one parameter as D1 expects it. Values are converted
// through database/sql's own converter first, which is what makes a *int, an
// int, an int64 and a named integer type all encode identically.
func encodeParam(p any) (string, error) {
	v, err := driver.DefaultParameterConverter.ConvertValue(p)
	if err != nil {
		return "", err
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return "", fmt.Errorf("the D1 adapter cannot bind a binary parameter")
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), nil
	case bool:
		// SQLite has no portable boolean literal before 3.23, and "1"/"0"
		// round-trips through the column's numeric affinity everywhere.
		if x {
			return "1", nil
		}
		return "0", nil
	case time.Time:
		// Match the format CURRENT_TIMESTAMP writes, so a datetime column
		// holds one representation rather than two.
		return x.UTC().Format(time.DateTime), nil
	case nil:
		return "", fmt.Errorf("is nil")
	default:
		return "", fmt.Errorf("cannot encode a %T", v)
	}
}

// substituteNulls replaces the placeholder at each of the given parameter
// positions with the NULL literal. Only bare `?` placeholders are recognised;
// a numbered (`?1`) placeholder is rejected rather than rewritten wrongly.
func substituteNulls(stmt string, nullPositions []int) (string, error) {
	targets := make(map[int]bool, len(nullPositions))
	for _, p := range nullPositions {
		targets[p] = true
	}

	var (
		out     strings.Builder
		quote   byte
		ordinal int
	)
	for i := 0; i < len(stmt); i++ {
		ch := stmt[i]

		if quote != 0 {
			out.WriteByte(ch)
			// A doubled quote inside a quoted run is an escaped quote, not a
			// terminator.
			if ch == quote {
				if i+1 < len(stmt) && stmt[i+1] == quote {
					out.WriteByte(quote)
					i++
				} else {
					quote = 0
				}
			}
			continue
		}

		switch ch {
		case '\'', '"', '`':
			quote = ch
			out.WriteByte(ch)
		case '?':
			if i+1 < len(stmt) && stmt[i+1] >= '0' && stmt[i+1] <= '9' {
				return "", fmt.Errorf("database: cannot substitute NULL into a numbered placeholder at offset %d", i)
			}
			if targets[ordinal] {
				out.WriteString("NULL")
			} else {
				out.WriteByte('?')
			}
			ordinal++
		default:
			out.WriteByte(ch)
		}
	}

	if ordinal != len(nullPositions)+countNonNull(stmt, nullPositions) {
		// Guard against a statement whose placeholder count disagrees with the
		// parameter count: rewriting it would shift every later value.
		return "", fmt.Errorf("database: statement has %d placeholders but %d parameters", ordinal, countNonNull(stmt, nullPositions)+len(nullPositions))
	}
	return out.String(), nil
}

func countNonNull(stmt string, nullPositions []int) int {
	nulls := make(map[int]bool, len(nullPositions))
	for _, p := range nullPositions {
		nulls[p] = true
	}
	total, quote, ordinal := 0, byte(0), 0
	for i := 0; i < len(stmt); i++ {
		ch := stmt[i]
		if quote != 0 {
			if ch == quote {
				if i+1 < len(stmt) && stmt[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '?':
			ordinal++
			if !nulls[ordinal-1] {
				total++
			}
		}
	}
	return total
}

type d1Row struct {
	values map[string]any
}

func (r *d1Row) lookup(name string) (any, error) {
	v, ok := r.values[name]
	if !ok {
		return nil, fmt.Errorf("database: no column %q", name)
	}
	return v, nil
}

func (r *d1Row) Int(name string) (int64, error) {
	v, err := r.lookup(name)
	if err != nil {
		return 0, err
	}
	switch x := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return int64(x), nil
	case float32:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	}
	return 0, &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "integer"}
}

func (r *d1Row) Float(name string) (float64, error) {
	v, err := r.lookup(name)
	if err != nil {
		return 0, err
	}
	switch x := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	}
	return 0, &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "float"}
}

func (r *d1Row) NullableFloat(name string) (*float64, error) {
	v, err := r.lookup(name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	value, err := r.Float(name)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *d1Row) String(name string) (string, error) {
	v, err := r.lookup(name)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "text"}
}

func (r *d1Row) NullableString(name string) (*string, error) {
	v, err := r.lookup(name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	if s, ok := v.(string); ok {
		return &s, nil
	}
	return nil, &TypeError{Column: name, Source: fmt.Sprintf("%T", v), Want: "text"}
}
