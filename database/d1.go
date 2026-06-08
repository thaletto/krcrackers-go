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

func newD1(cfg *D1Config) (DB, error) {
	return &d1Client{
		inner:      cloudflare.NewClient(option.WithAPIToken(cfg.APIToken)),
		accountID:  cfg.AccountID,
		databaseID: cfg.DatabaseID,
	}, nil
}

func (c *d1Client) Query(ctx context.Context, sql string, params ...any) ([]Row, error) {
	res, err := c.run(ctx, sql, params)
	if err != nil {
		return nil, err
	}

	rows := make([]Row, 0)
	for _, r := range res.Result {
		for _, row := range r.Results {
			m, ok := row.(map[string]any)
			if !ok {
				b, err := json.Marshal(row)
				if err != nil {
					return nil, err
				}
				m = map[string]any{}
				if err := json.Unmarshal(b, &m); err != nil {
					return nil, err
				}
			}
			rows = append(rows, &d1Row{values: m})
		}
	}
	return rows, nil
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

func (c *d1Client) Close() error {
	return nil
}

func (c *d1Client) Begin(_ context.Context) (Tx, error) {
	return &d1Tx{client: c}, nil
}

// d1Tx is a best-effort transaction adapter for D1's HTTP API.
// Statements are buffered on Execute and executed sequentially on Commit.
// If any statement fails during Commit, cleanup is attempted for statements
// that already succeeded (DELETE for INSERTs). This is not truly atomic —
// a crash between successful statements leaves partial state.
type d1Tx struct {
	client    *d1Client
	stmts     []d1TxStmt
	ids       []int64 // lastInsertID per statement, for cleanup
}

type d1TxStmt struct {
	sql    string
	params []any
}

func (t *d1Tx) Query(ctx context.Context, sql string, params ...any) ([]Row, error) {
	return t.client.Query(ctx, sql, params...)
}

func (t *d1Tx) Execute(_ context.Context, sql string, params ...any) (Result, error) {
	t.stmts = append(t.stmts, d1TxStmt{sql: sql, params: params})
	return Result{}, nil
}

func (t *d1Tx) Commit() error {
	if len(t.stmts) == 0 {
		return nil
	}
	for i, stmt := range t.stmts {
		res, err := t.client.Execute(context.Background(), stmt.sql, stmt.params...)
		if err != nil {
			t.cleanup(i)
			return fmt.Errorf("d1 tx commit: %w", err)
		}
		t.ids = append(t.ids, res.LastInsertID)
	}
	return nil
}

// cleanup attempts to undo INSERTs that already succeeded by deleting rows
// by their last-inserted IDs. Best-effort: if cleanup itself fails, we
// return and leave partial state.
func (t *d1Tx) cleanup(failedAt int) {
	for i := failedAt - 1; i >= 0; i-- {
		stmt := t.stmts[i]
		if isInsertStmt(stmt.sql) && i < len(t.ids) && t.ids[i] > 0 {
			table := extractTableName(stmt.sql)
			if table != "" {
				_, _ = t.client.Execute(context.Background(),
					"DELETE FROM "+table+" WHERE id = ?", t.ids[i])
			}
		}
	}
}

func (t *d1Tx) Rollback() error {
	t.stmts = nil
	t.ids = nil
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
		case []byte:
			out[i] = string(v)
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
