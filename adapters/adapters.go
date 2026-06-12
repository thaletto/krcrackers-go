package adapters

import (
	"context"

	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/services/auth"
	"github.com/thaletto/krcrackers-go/services/orders"
)

type UserProviderAdapter struct {
	Repo auth.Repository
}

func (a *UserProviderAdapter) GetUser(ctx context.Context, id int) (orders.User, error) {
	u, err := a.Repo.GetByID(ctx, id)
	if err != nil {
		return orders.User{}, err
	}
	return orders.User{ID: u.ID, Email: u.Email, Name: u.Name, Phone: u.Phone}, nil
}

type AddressProviderAdapter struct {
	DB database.DB
}

func (a *AddressProviderAdapter) GetAddress(ctx context.Context, id int) (orders.Address, error) {
	rows, err := a.DB.Query(ctx, `
		SELECT id, user_id, label, street, city, state, pincode, country
		FROM customer_addresses WHERE id = ?
	`, id)
	if err != nil {
		return orders.Address{}, err
	}
	if len(rows) == 0 {
		return orders.Address{}, nil
	}
	row := rows[0]
	addrID, _ := row.Int("id")
	userID, _ := row.Int("user_id")
	label, _ := row.String("label")
	street, _ := row.String("street")
	city, _ := row.String("city")
	state, _ := row.String("state")
	pincode, _ := row.String("pincode")
	country, _ := row.String("country")
	return orders.Address{
		ID:      int(addrID),
		UserID:  int(userID),
		Label:   label,
		Street:  street,
		City:    city,
		State:   state,
		Pincode: pincode,
		Country: country,
	}, nil
}
