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

// Repository defines the data access interface for customer profiles and addresses.
type Repository interface {
	GetProfile(ctx context.Context, userID int) (auth.User, error)
	UpdateProfile(ctx context.Context, userID int, name, phone string) (auth.User, error)
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

func (r *repo) GetProfile(ctx context.Context, userID int) (auth.User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, name, phone, avatar_url, auth_provider, auth_provider_id, password_hash, role, created_at, updated_at
		FROM users WHERE id = ?
	`, userID)
	if err != nil {
		return auth.User{}, err
	}
	if len(rows) == 0 {
		return auth.User{}, nil
	}
	return rowToUser(rows[0])
}

func (r *repo) UpdateProfile(ctx context.Context, userID int, name, phone string) (auth.User, error) {
	_, err := r.db.Execute(ctx, `
		UPDATE users SET name = ?, phone = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, name, phone, userID)
	if err != nil {
		return auth.User{}, err
	}
	return r.GetProfile(ctx, userID)
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
		return Address{}, nil
	}
	return rowToAddress(rows[0])
}

func (r *repo) CreateAddress(ctx context.Context, userID int, input AddressInput) (Address, error) {
	if input.IsDefault {
		r.db.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID)
	}

	country := input.Country
	if country == "" {
		country = "India"
	}
	label := input.Label
	if label == "" {
		label = "Home"
	}

	res, err := r.db.Execute(ctx, `
		INSERT INTO customer_addresses (user_id, label, street, city, state, pincode, country, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, label, input.Street, input.City, input.State, input.Pincode, country, input.IsDefault)
	if err != nil {
		return Address{}, err
	}
	return r.GetAddress(ctx, userID, int(res.LastInsertID))
}

func (r *repo) UpdateAddress(ctx context.Context, userID, addressID int, input AddressInput) (Address, error) {
	if input.IsDefault {
		r.db.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID)
	}

	_, err := r.db.Execute(ctx, `
		UPDATE customer_addresses SET label = ?, street = ?, city = ?, state = ?, pincode = ?, country = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, input.Label, input.Street, input.City, input.State, input.Pincode, input.Country, input.IsDefault, addressID, userID)
	if err != nil {
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
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Execute(ctx, `UPDATE customer_addresses SET is_default = FALSE WHERE user_id = ?`, userID)
	if err != nil {
		tx.Rollback()
		return err
	}
	res, err := tx.Execute(ctx, `UPDATE customer_addresses SET is_default = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`, addressID, userID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		return ErrNotFound
	}
	return tx.Commit()
}

var ErrNotFound = fmt.Errorf("not found")

func rowToUser(row database.Row) (auth.User, error) {
	id, err := row.Int("id")
	if err != nil {
		return auth.User{}, err
	}
	email, err := row.String("email")
	if err != nil {
		return auth.User{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return auth.User{}, err
	}
	phone, err := row.String("phone")
	if err != nil {
		return auth.User{}, err
	}
	avatarURL, err := row.String("avatar_url")
	if err != nil {
		return auth.User{}, err
	}
	authProvider, err := row.String("auth_provider")
	if err != nil {
		return auth.User{}, err
	}
	authProviderID, err := row.String("auth_provider_id")
	if err != nil {
		return auth.User{}, err
	}
	passwordHash, err := row.String("password_hash")
	if err != nil {
		return auth.User{}, err
	}
	role, err := row.String("role")
	if err != nil {
		return auth.User{}, err
	}
	createdAtStr, err := row.String("created_at")
	if err != nil {
		return auth.User{}, err
	}
	updatedAtStr, err := row.String("updated_at")
	if err != nil {
		return auth.User{}, err
	}
	createdAt, _ := time.Parse(time.DateTime, createdAtStr)
	updatedAt, _ := time.Parse(time.DateTime, updatedAtStr)
	return auth.User{
		ID:             int(id),
		Email:          email,
		Name:           name,
		Phone:          phone,
		AvatarURL:      avatarURL,
		AuthProvider:   authProvider,
		AuthProviderID: authProviderID,
		PasswordHash:   passwordHash,
		Role:           role,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

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
