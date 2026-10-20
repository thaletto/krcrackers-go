package customers

import (
	"context"
	"fmt"
	"time"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/services/auth"
)

// Address represents a customer shipping address.
type Address struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userId"`
	Label     string    `json:"label"`
	Street    string    `json:"street"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	Pincode   string    `json:"pincode"`
	Country   string    `json:"country"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AddressInput is the request payload for creating or updating an address.
type AddressInput struct {
	Label     string `json:"label"`
	Street    string `json:"street"`
	City      string `json:"city"`
	State     string `json:"state"`
	Pincode   string `json:"pincode"`
	Country   string `json:"country"`
	IsDefault bool   `json:"isDefault"`
}

// Accounts is the seam onto the Account module, which owns the users table.
// The Customer module keeps addresses; a profile is an account, so it asks
// rather than keeping a second copy of the users projection and row mapper.
type Accounts interface {
	Profile(ctx context.Context, userID int) (auth.Profile, error)
	UpdateProfile(ctx context.Context, userID int, name, phone string) (auth.Profile, error)
}

// Repository defines the data access interface for customer addresses.
type Repository interface {
	ListAddresses(ctx context.Context, userID int) ([]Address, error)
	GetAddress(ctx context.Context, userID, addressID int) (Address, error)
	CreateAddress(ctx context.Context, userID int, input AddressInput) (Address, error)
	UpdateAddress(ctx context.Context, userID, addressID int, input AddressInput) (Address, error)
	DeleteAddress(ctx context.Context, userID, addressID int) error
	SetDefaultAddress(ctx context.Context, userID, addressID int) error
}

type repo struct {
	db database.DB
}

// NewRepository returns a new customers repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) ListAddresses(ctx context.Context, userID int) ([]Address, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, label, street, city, state, pincode, country, is_default, created_at, updated_at
		FROM customer_addresses WHERE user_id = ? ORDER BY id
	`, userID)
	if err != nil {
		return nil, err
	}
	addresses := make([]Address, 0, len(rows))
	for _, row := range rows {
		addr, err := rowToAddress(row)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, addr)
	}
	return addresses, nil
}

func (r *repo) GetAddress(ctx context.Context, userID, addressID int) (Address, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, label, street, city, state, pincode, country, is_default, created_at, updated_at
		FROM customer_addresses WHERE id = ? AND user_id = ?
	`, addressID, userID)
	if err != nil {
		return Address{}, err
	}
	if len(rows) == 0 {
		return Address{}, ErrNotFound
	}
	return rowToAddress(rows[0])
}

func (r *repo) CreateAddress(ctx context.Context, userID int, input AddressInput) (Address, error) {
	country := input.Country
	if country == "" {
		country = "India"
	}
	label := input.Label
	if label == "" {
		label = "Home"
	}

	// Clearing the previous default and inserting the new one share a
	// transaction: if the insert fails the customer keeps the default they had,
	// rather than being left with none.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Address{}, err
	}
	if input.IsDefault {
		if _, err := tx.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID); err != nil {
			_ = tx.Rollback()
			return Address{}, err
		}
	}

	res, err := tx.Execute(ctx, `
		INSERT INTO customer_addresses (user_id, label, street, city, state, pincode, country, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, label, input.Street, input.City, input.State, input.Pincode, country, input.IsDefault)
	if err != nil {
		_ = tx.Rollback()
		return Address{}, err
	}
	if err := tx.Commit(); err != nil {
		return Address{}, err
	}
	return r.GetAddress(ctx, userID, int(res.LastInsertID))
}

func (r *repo) UpdateAddress(ctx context.Context, userID, addressID int, input AddressInput) (Address, error) {
	country := input.Country
	if country == "" {
		country = "India"
	}

	// RowsAffected is the ownership check. Without it a write aimed at another
	// owner's address reports success and returns a zero-valued address.
	owned, err := r.db.Query(ctx, `SELECT id FROM customer_addresses WHERE id = ? AND user_id = ?`, addressID, userID)
	if err != nil {
		return Address{}, err
	}
	if len(owned) == 0 {
		return Address{}, ErrNotFound
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Address{}, err
	}
	if input.IsDefault {
		if _, err := tx.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID); err != nil {
			_ = tx.Rollback()
			return Address{}, err
		}
	}
	if _, err := tx.Execute(ctx, `
		UPDATE customer_addresses SET label = ?, street = ?, city = ?, state = ?, pincode = ?, country = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, input.Label, input.Street, input.City, input.State, input.Pincode, country, input.IsDefault, addressID, userID); err != nil {
		_ = tx.Rollback()
		return Address{}, err
	}
	if err := tx.Commit(); err != nil {
		return Address{}, err
	}
	return r.GetAddress(ctx, userID, addressID)
}

func (r *repo) DeleteAddress(ctx context.Context, userID, addressID int) error {
	res, err := r.db.Execute(ctx, `DELETE FROM customer_addresses WHERE id = ? AND user_id = ?`, addressID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) SetDefaultAddress(ctx context.Context, userID, addressID int) error {
	// Confirm the address is the caller's before clearing anything: a rejected
	// set-default must not leave the customer with no default at all.
	owned, err := r.db.Query(ctx, `SELECT id FROM customer_addresses WHERE id = ? AND user_id = ?`, addressID, userID)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return ErrNotFound
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Execute(ctx, `UPDATE customer_addresses SET is_default = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`, addressID, userID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

var ErrNotFound = fmt.Errorf("not found")

func rowToAddress(row database.Row) (Address, error) {
	id, err := row.Int("id")
	if err != nil {
		return Address{}, err
	}
	userID, err := row.Int("user_id")
	if err != nil {
		return Address{}, err
	}
	label, err := row.String("label")
	if err != nil {
		return Address{}, err
	}
	street, err := row.String("street")
	if err != nil {
		return Address{}, err
	}
	city, err := row.String("city")
	if err != nil {
		return Address{}, err
	}
	state, err := row.String("state")
	if err != nil {
		return Address{}, err
	}
	pincode, err := row.String("pincode")
	if err != nil {
		return Address{}, err
	}
	country, err := row.String("country")
	if err != nil {
		return Address{}, err
	}
	isDefaultInt, err := row.Int("is_default")
	if err != nil {
		return Address{}, err
	}
	createdAtStr, err := row.String("created_at")
	if err != nil {
		return Address{}, err
	}
	updatedAtStr, err := row.String("updated_at")
	if err != nil {
		return Address{}, err
	}
	createdAt, _ := time.Parse(time.DateTime, createdAtStr)
	updatedAt, _ := time.Parse(time.DateTime, updatedAtStr)
	return Address{
		ID:        int(id),
		UserID:    int(userID),
		Label:     label,
		Street:    street,
		City:      city,
		State:     state,
		Pincode:   pincode,
		Country:   country,
		IsDefault: isDefaultInt != 0,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
