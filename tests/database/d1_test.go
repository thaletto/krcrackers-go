package database_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
)

// d1Request is what the D1 HTTP API receives.
type d1Request struct {
	SQL    string   `json:"sql"`
	Params []string `json:"params"`
}

// fakeD1 stands in for Cloudflare's D1 query endpoint. It records the
// statements and parameters the adapter sends and replays a canned result, so
// the adapter's own behaviour — parameter encoding, result decoding, the
// compensation path — is testable without a Cloudflare account.
type fakeD1 struct {
	t        *testing.T
	server   *httptest.Server
	requests []d1Request

	// results are replayed in order, one per request. The last one repeats.
	results []d1Result
}

type d1Result struct {
	rows     []map[string]any
	lastRow  int
	changes  int
	success  bool
	failWith string
}

func newFakeD1(t *testing.T) *fakeD1 {
	t.Helper()
	f := &fakeD1{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req d1Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("fake d1: decode request: %v", err)
		}
		f.requests = append(f.requests, req)

		res := f.next()
		if res.failWith != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"` + res.failWith + `"}]}`))
			return
		}
		rows := res.rows
		if rows == nil {
			rows = []map[string]any{}
		}
		payload := map[string]any{
			"success": res.success || res.failWith == "",
			"result": []map[string]any{{
				"meta": map[string]any{
					"changed_db":  true,
					"changes":     res.changes,
					"last_row_id": res.lastRow,
					"rows_read":   len(rows),
				},
				"results": rows,
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeD1) next() d1Result {
	i := len(f.requests) - 1
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	if i < 0 {
		return d1Result{success: true}
	}
	return f.results[i]
}

// last returns the most recent request the adapter sent.
func (f *fakeD1) last() d1Request {
	f.t.Helper()
	if len(f.requests) == 0 {
		f.t.Fatal("the adapter sent no request")
	}
	return f.requests[len(f.requests)-1]
}

// newD1DB points the D1 adapter at the fake endpoint.
func newD1DB(t *testing.T, f *fakeD1) database.DB {
	t.Helper()
	db, err := database.New(database.Config{
		Mode: database.ModeD1,
		D1: &database.D1Config{
			APIToken:   "test-token",
			AccountID:  "test-account",
			DatabaseID: "test-database",
			BaseURL:    f.server.URL,
		},
	})
	if err != nil {
		t.Fatalf("database.New(d1): %v", err)
	}
	return db
}

// The bug this guards: a *int reached D1 through a hand-written encoder that
// had no case for it, fell through to fmt.Sprint, and arrived as a pointer
// address. orders.user_id is a *int, so signed-in checkout wrote a garbage id
// and guest checkout wrote the text "<nil>" — both invisible to every test,
// because the D1 adapter could not be reached at all.
func TestD1BindsPointersByValue(t *testing.T) {
	f := newFakeD1(t)
	db := newD1DB(t, f)
	ctx := context.Background()

	id := 42
	note := "gift wrap"
	var missing *int

	if _, err := db.Execute(ctx,
		`INSERT INTO orders (user_id, notes) VALUES (?, ?)`,
		&id, &note); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := f.last()
	if len(got.Params) != 2 || got.Params[0] != "42" || got.Params[1] != "gift wrap" {
		t.Errorf("params = %q, want [42 gift wrap]", got.Params)
	}

	// A nil *int is SQL NULL, which this adapter substitutes as a literal
	// because the D1 API cannot carry a null parameter.
	if _, err := db.Execute(ctx,
		`INSERT INTO orders (user_id, notes) VALUES (?, ?)`,
		missing, nil); err != nil {
		t.Fatalf("Execute(nil params): %v", err)
	}
	got = f.last()
	if got.SQL != `INSERT INTO orders (user_id, notes) VALUES (NULL, NULL)` {
		t.Errorf("sql = %q, want the NULL literals substituted", got.SQL)
	}
	if len(got.Params) != 0 {
		t.Errorf("params = %q, want none", got.Params)
	}
}

// Substitution must not touch a `?` that is inside a string literal, or a
// later parameter would shift into the wrong placeholder.
func TestD1NullSubstitutionLeavesQuotedPlaceholdersAlone(t *testing.T) {
	f := newFakeD1(t)
	db := newD1DB(t, f)

	if _, err := db.Execute(context.Background(),
		`INSERT INTO t (a, b, c) VALUES (?, 'question? mark', ?)`,
		nil, "kept"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := f.last()
	if got.SQL != `INSERT INTO t (a, b, c) VALUES (NULL, 'question? mark', ?)` {
		t.Errorf("sql = %q", got.SQL)
	}
	if len(got.Params) != 1 || got.Params[0] != "kept" {
		t.Errorf("params = %q, want [kept]", got.Params)
	}
}

// A parameter the encoder cannot represent is an error, not a string that will
// not round-trip.
func TestD1RefusesUnencodableParameters(t *testing.T) {
	f := newFakeD1(t)
	db := newD1DB(t, f)

	if _, err := db.Execute(context.Background(), `INSERT INTO t (blob) VALUES (?)`, []byte("raw")); err == nil {
		t.Error("binding a []byte succeeded; it should be an error on this transport")
	}
	if _, err := db.Execute(context.Background(), `INSERT INTO t (a) VALUES (?)`, struct{ X int }{1}); err == nil {
		t.Error("binding an arbitrary struct succeeded; it should be an error")
	}
}

// A time parameter must reach D1 in the same representation CURRENT_TIMESTAMP
// writes, or a datetime column ends up holding two different formats.
func TestD1BindsTimeAsASQLiteDatetime(t *testing.T) {
	f := newFakeD1(t)
	db := newD1DB(t, f)

	expires := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := db.Execute(context.Background(),
		`INSERT INTO refresh_tokens (expires_at) VALUES (?)`, expires); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := f.last().Params; len(got) != 1 || got[0] != "2026-03-04 05:06:07" {
		t.Errorf("params = %q, want [2026-03-04 05:06:07]", got)
	}
}

// A float must not be rendered in a form SQLite's numeric affinity mangles.
func TestD1BindsFloatsWithoutScientificNotation(t *testing.T) {
	f := newFakeD1(t)
	db := newD1DB(t, f)

	if _, err := db.Execute(context.Background(), `INSERT INTO t (total) VALUES (?)`, 199.99); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := f.last().Params; len(got) != 1 || got[0] != "199.99" {
		t.Errorf("params = %q, want [199.99]", got)
	}
}

// The adapter decodes results and reports the same numbers a caller gets from
// SQLite, and the transaction reports the capabilities the caller must plan
// around.
func TestD1DecodesResultsAndReportsCapabilities(t *testing.T) {
	f := newFakeD1(t)
	f.results = []d1Result{{
		success: true,
		rows: []map[string]any{{
			"id": float64(7), "total": 199.99, "email": "a@b.c", "note": nil,
		}},
	}}
	db := newD1DB(t, f)
	ctx := context.Background()

	rows, err := db.Query(ctx, `SELECT id, total, email, note FROM orders WHERE id = ?`, 7)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if v, err := rows[0].Int("id"); err != nil || v != 7 {
		t.Errorf("Int(id) = %d, %v; want 7, nil", v, err)
	}
	if v, err := rows[0].Float("total"); err != nil || v != 199.99 {
		t.Errorf("Float(total) = %v, %v; want 199.99, nil", v, err)
	}
	if v, err := rows[0].String("email"); err != nil || v != "a@b.c" {
		t.Errorf("String(email) = %q, %v; want a@b.c, nil", v, err)
	}
	if v, err := rows[0].NullableString("note"); err != nil || v != nil {
		t.Errorf("NullableString(note) = %v, %v; want nil, nil", v, err)
	}
	// A number read as text is a mistake the adapter can catch.
	if _, err := rows[0].String("total"); err == nil {
		t.Error("String(total) on a numeric column succeeded; want a TypeError")
	}

	caps := db.Capabilities()
	if caps.AtomicTransactions {
		t.Error("D1 reported atomic transactions; its HTTP API has none")
	}
	if !caps.NullParameters {
		t.Error("D1 reported that it cannot bind NULL; it substitutes the literal")
	}
	if caps.BinaryParameters {
		t.Error("D1 reported that it can bind binary; the transport is strings")
	}
}

// Rollback on D1 can only undo the INSERTs it saw. That limitation is now
// stated, and it is reported when a compensating delete fails rather than
// being swallowed.
func TestD1RollbackCompensatesInsertsAndReportsFailure(t *testing.T) {
	f := newFakeD1(t)
	f.results = []d1Result{
		{success: true, lastRow: 11, changes: 1},
		{success: true, lastRow: 12, changes: 1},
		{failWith: "disk on fire"},
	}
	db := newD1DB(t, f)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Execute(ctx, `INSERT INTO orders (name) VALUES (?)`, "first"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := tx.Execute(ctx, `INSERT INTO order_items (order_id) VALUES (?)`, 11); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	if err := tx.Rollback(); err == nil {
		t.Error("Rollback succeeded although a compensating delete failed")
	}

	// One DELETE per tracked insert, newest insert compensated first.
	requests := f.requests[len(f.requests)-2:]
	if requests[0].SQL != "DELETE FROM order_items WHERE id = ?" {
		t.Errorf("first compensation = %q, want the order_items delete", requests[0].SQL)
	}
	if requests[1].SQL != "DELETE FROM orders WHERE id = ?" {
		t.Errorf("second compensation = %q, want the orders delete", requests[1].SQL)
	}
}

// Both adapters must agree where they can. These are the cases the seam's
// documentation promises are identical.
func TestAdapterParityOnTheCasesTheSeamPromises(t *testing.T) {
	ctx := context.Background()

	sqlitePath := filepath.Join(t.TempDir(), "parity.sqlite")
	local, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: sqlitePath},
	})
	if err != nil {
		t.Fatalf("database.New(local): %v", err)
	}
	t.Cleanup(func() { _ = local.Close() })
	if _, err := migrations.Up(ctx, local); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}

	if !local.Capabilities().AtomicTransactions {
		t.Error("the SQLite adapter reported non-atomic transactions")
	}
	if !local.Capabilities().BinaryParameters {
		t.Error("the SQLite adapter reported that it cannot bind binary")
	}

	id := 42
	note := "gift wrap"
	if _, err := local.Execute(ctx, `INSERT INTO users (id, email, name, phone, auth_provider, password_hash, role) VALUES (?, 'a@b.c', 'A', '1', 'email', '', 'customer')`, id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	rows, err := local.Query(ctx, `SELECT id, email FROM users WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if v, err := rows[0].Int("id"); err != nil || v != int64(id) {
		t.Errorf("local Int(id) = %d, %v", v, err)
	}
	if v, err := rows[0].String("email"); err != nil || v != "a@b.c" {
		t.Errorf("local String(email) = %q, %v", v, err)
	}

	// A NULL parameter lands as NULL on SQLite, and D1 substitutes the same
	// literal, so a nullable column reads the same on both.
	if _, err := local.Execute(ctx, `INSERT INTO orders (user_id, user_name, email, phone, street, town_or_city, state, pincode, delivery_region, delivery_location, total, notes) VALUES (NULL, 'G', 'g@b.c', '1', 's', 't', 'st', 'p', 'r', 'l', 10.5, ?)`, &note); err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderRows, err := local.Query(ctx, `SELECT user_id, notes, total FROM orders WHERE id = 1`)
	if err != nil {
		t.Fatalf("Query order: %v", err)
	}
	if v, _ := orderRows[0].NullableString("notes"); v == nil || *v != "gift wrap" {
		t.Errorf("notes = %v, want the dereferenced value", v)
	}
	if v, err := orderRows[0].Float("total"); err != nil || v != 10.5 {
		t.Errorf("Float(total) = %v, %v", v, err)
	}
}
