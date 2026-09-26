package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	return &d1Client{
		inner:      cloudflare.NewClient(option.WithAPIToken(cfg.APIToken)),
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

func (c *d1Client) Close() error {
	return nil
}

// d1Tx is a best-effort transaction adapter for D1's HTTP API.
// D1 has no interactive transactions over HTTP, so every statement runs
// immediately (one HTTP round-trip each) and Commit is a no-op. Successful
// INSERTs are tracked so Rollback can delete them in reverse order as
// compensation. This is not truly atomic — a crash between statements
// leaves partial state.
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

func (t *d1Tx) Rollback() error {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for i := len(t.inserts) - 1; i >= 0; i-- {
		ins := t.inserts[i]
		_, _ = t.client.Execute(ctx, "DELETE FROM "+ins.table+" WHERE id = ?", ins.id)
	}
	t.inserts = nil
	return nil
}

func isInsertStmt(sql string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(sql))
	return strings.HasPrefix(trimmed, "insert")
}

func extractTableName(sql string) string {
	lower := strings.ToLower(sql)
	for _, prefix := range []string{"insert into ", "update ", "delete from "} {
		idx := strings.Index(lower, prefix)
		if idx >= 0 {
			start := idx + len(prefix)
			end := start
			for end < len(sql) && sql[end] != ' ' && sql[end] != '(' && sql[end] != '\n' {
				end++
			}
			return sql[start:end]
		}
	}
	return ""
}

func (c *d1Client) run(ctx context.Context, sql string, params []any) (*pagination.SinglePage[d1.QueryResult], error) {
	return c.inner.D1.Database.Query(ctx, c.databaseID, d1.DatabaseQueryParams{
		AccountID: cloudflare.F(c.accountID),
		Body: d1.DatabaseQueryParamsBodyD1SingleQuery{
			Sql:    cloudflare.F(sql),
			Params: cloudflare.F(paramsToStrings(params)),
		},
	})
}

func paramsToStrings(params []any) []string {
	out := make([]string, len(params))
	for i, p := range params {
		switch v := p.(type) {
		case nil:
			out[i] = ""
		case string:
			out[i] = v
		case *string:
			if v != nil {
				out[i] = *v
			} else {
				out[i] = ""
			}
		case []byte:
			out[i] = string(v)
		case int:
			out[i] = fmt.Sprint(v)
		case int64:
			out[i] = fmt.Sprint(v)
		case float64:
			out[i] = fmt.Sprint(v)
		case bool:
			if v {
				out[i] = "1"
			} else {
				out[i] = "0"
			}
		default:
			out[i] = fmt.Sprint(v)
		}
	}
	return out
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
