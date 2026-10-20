package customers_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	"github.com/thaletto/krcrackers-go/src/services/customers"
)

// newFixture wires the Customer module over its address repository and the
// Account module that owns profiles, both on one migrated SQLite file.
func newFixture(t *testing.T) (*customers.Service, *auth.Service, database.DB) {
	t.Helper()
	ctx := context.Background()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "customers.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}

	authSvc := auth.NewServiceWithGoogleVerifier(auth.NewRepository(db), "test-secret", nil)
	return customers.NewService(customers.NewRepository(db), authSvc), authSvc, db
}

func seedUser(t *testing.T, db database.DB, email string) int {
	t.Helper()
	if _, err := db.Execute(context.Background(), `
		INSERT INTO users (email, name, phone, auth_provider, password_hash, role)
		VALUES (?, 'Seeded', '9000000000', 'email', 'a-bcrypt-hash', 'customer')
	`, email); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	rows, err := db.Query(context.Background(), `SELECT id FROM users WHERE email = ?`, email)
	if err != nil {
		t.Fatalf("read user id: %v", err)
	}
	id, _ := rows[0].Int("id")
	return int(id)
}

// A profile comes from the Account module, so the Customer module returns what
// the account holds rather than keeping its own copy of the users row.
func TestProfileComesFromTheAccountModule(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newFixture(t)
	userID := seedUser(t, db, "shopper@example.com")

	profile, err := svc.GetProfile(ctx, userID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile.ID != userID || profile.Email != "shopper@example.com" {
		t.Errorf("profile = %+v, want the seeded account", profile)
	}
	if profile.AuthProvider != "email" || profile.Role != "customer" {
		t.Errorf("profile auth fields = (%q, %q), want (email, customer)", profile.AuthProvider, profile.Role)
	}
}

func TestUpdateProfileGoesThroughTheAccountModule(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newFixture(t)
	userID := seedUser(t, db, "shopper@example.com")

	updated, err := svc.UpdateProfile(ctx, userID, "Renamed", "9111111111")
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Name != "Renamed" || updated.Phone != "9111111111" {
		t.Errorf("profile = (%q, %q), want (Renamed, 9111111111)", updated.Name, updated.Phone)
	}

	// The write landed on the users row, which only the Account module touches.
	rows, err := db.Query(ctx, `SELECT name, phone FROM users WHERE id = ?`, userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	name, _ := rows[0].String("name")
	phone, _ := rows[0].String("phone")
	if name != "Renamed" || phone != "9111111111" {
		t.Errorf("stored user = (%q, %q), want (Renamed, 9111111111)", name, phone)
	}
}

// A missing account is reported the same way a missing address is, so the HTTP
// layer maps both to 404.
func TestMissingProfileIsNotFound(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newFixture(t)

	if _, err := svc.GetProfile(ctx, 4242); !errors.Is(err, customers.ErrNotFound) {
		t.Errorf("GetProfile for a missing account returned %v, want ErrNotFound", err)
	}
	if _, err := svc.UpdateProfile(ctx, 4242, "X", "1"); !errors.Is(err, customers.ErrNotFound) {
		t.Errorf("UpdateProfile for a missing account returned %v, want ErrNotFound", err)
	}
}

// The Customer module owns addresses, and they stay scoped to their owner.
func TestAddressesStayScopedToTheirOwner(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newFixture(t)
	owner := seedUser(t, db, "owner@example.com")
	stranger := seedUser(t, db, "stranger@example.com")

	created, err := svc.CreateAddress(ctx, owner, customers.AddressInput{
		Street: "1 Test Street", City: "Mumbai", State: "Maharashtra", Pincode: "400001", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	if created.UserID != owner {
		t.Errorf("address userId = %d, want %d", created.UserID, owner)
	}

	// The Customer module offers no single-address read, so a stranger cannot
	// fetch one; the writes are scoped and must stay that way.
	if err := svc.DeleteAddress(ctx, stranger, created.ID); !errors.Is(err, customers.ErrNotFound) {
		t.Errorf("a stranger deleted the address (err = %v)", err)
	}
	if _, err := svc.UpdateAddress(ctx, stranger, created.ID, customers.AddressInput{
		Street: "9 Attacker Road", City: "Pune", State: "Maharashtra", Pincode: "411001",
	}); !errors.Is(err, customers.ErrNotFound) {
		t.Errorf("a stranger rewrote the address (err = %v)", err)
	}
	if err := svc.DeleteAddress(ctx, owner, created.ID); err != nil {
		t.Errorf("the owner could not delete their own address: %v", err)
	}
}

// A rejected set-default must not cost the customer the default they already
// had. Clearing the old default and setting the new one used to be two
// statements with the scope check in between, so an out-of-scope call left the
// customer with none.
func TestRejectedSetDefaultLeavesTheExistingDefaultAlone(t *testing.T) {
	ctx := context.Background()
	svc, _, db := newFixture(t)
	owner := seedUser(t, db, "owner@example.com")
	stranger := seedUser(t, db, "stranger@example.com")

	mine, err := svc.CreateAddress(ctx, owner, customers.AddressInput{
		Street: "1 Test Street", City: "Mumbai", State: "Maharashtra", Pincode: "400001", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	other, err := svc.CreateAddress(ctx, owner, customers.AddressInput{
		Street: "2 Second Street", City: "Mumbai", State: "Maharashtra", Pincode: "400002",
	})
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}

	// A stranger aiming at the owner's address is refused...
	if err := svc.SetDefaultAddress(ctx, stranger, mine.ID); !errors.Is(err, customers.ErrNotFound) {
		t.Fatalf("a stranger set the owner's default (err = %v)", err)
	}
	// ...and the owner's default is untouched.
	assertDefault(t, db, mine.ID, true)

	if err := svc.SetDefaultAddress(ctx, owner, other.ID); err != nil {
		t.Fatalf("SetDefaultAddress: %v", err)
	}
	assertDefault(t, db, mine.ID, false)
	assertDefault(t, db, other.ID, true)
}

func assertDefault(t *testing.T, db database.DB, addressID int, want bool) {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT is_default FROM customer_addresses WHERE id = ?`, addressID)
	if err != nil {
		t.Fatalf("read is_default: %v", err)
	}
	got, _ := rows[0].Int("is_default")
	if (got != 0) != want {
		t.Errorf("address %d is_default = %v, want %v", addressID, got != 0, want)
	}
}
