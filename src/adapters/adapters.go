// Package adapters provides shared adapter implementations that bridge
// different service interfaces, eliminating duplication between entry points.
package adapters

import (
	"context"
	"errors"

	"github.com/thaletto/krcrackers-go/src/services/auth"
	"github.com/thaletto/krcrackers-go/src/services/customers"
	"github.com/thaletto/krcrackers-go/src/services/orders"
	"github.com/thaletto/krcrackers-go/src/services/products"
)

// AccountDirectory is the part of the Account module this package needs: a
// read of the public shape of an account. It is deliberately narrower than
// auth.Repository, so an order intake lookup does not drag password_hash,
// google_id and auth_provider_id across the seam to use four fields.
type AccountDirectory interface {
	Profile(ctx context.Context, id int) (auth.Profile, error)
}

// UserProviderAdapter implements orders.UserProvider over the Account module's
// profile read.
type UserProviderAdapter struct {
	Accounts AccountDirectory
}

func (a *UserProviderAdapter) GetUser(ctx context.Context, id int) (orders.User, error) {
	p, err := a.Accounts.Profile(ctx, id)
	if err != nil {
		return orders.User{}, err
	}
	if p.ID == 0 {
		return orders.User{}, nil
	}
	return orders.User{ID: p.ID, Email: p.Email, Name: p.Name, Phone: p.Phone}, nil
}

// ProductCatalogueAdapter implements orders.Catalogue over the Product module.
// The Order module asks it what a product costs; it never reads the products
// table, so a basket price can only be the catalogue's figure.
type ProductCatalogueAdapter struct {
	Products *products.Service
}

// Prices returns the catalogue's name and price for each requested id. Ids the
// catalogue does not have are absent from the result, which is how the Order
// module recognises a basket line for a product that does not exist.
func (a *ProductCatalogueAdapter) Prices(ctx context.Context, productIDs []int) (map[int]orders.PricedProduct, error) {
	found, err := a.Products.GetByIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	prices := make(map[int]orders.PricedProduct, len(found))
	for _, p := range found {
		prices[p.ID] = orders.PricedProduct{Name: p.Name, Price: p.Price}
	}
	return prices, nil
}

// AddressDirectory is the part of the Customer module this package needs: a
// lookup of one of a shopper's own addresses. It is deliberately narrower than
// customers.Service, and it is scoped by the owning module rather than by a
// copy of its SQL here.
type AddressDirectory interface {
	AddressFor(ctx context.Context, userID, addressID int) (customers.Address, error)
}

// AddressProviderAdapter implements orders.AddressProvider over the Customer
// module. The owner check is the Customer module's query, so a shopper naming
// somebody else's address simply misses.
type AddressProviderAdapter struct {
	Addresses AddressDirectory
}

func (a *AddressProviderAdapter) AddressFor(ctx context.Context, shopperID, addressID int) (orders.Address, error) {
	addr, err := a.Addresses.AddressFor(ctx, shopperID, addressID)
	if err != nil {
		if errors.Is(err, customers.ErrNotFound) {
			return orders.Address{}, orders.ErrAddressNotFound
		}
		return orders.Address{}, err
	}
	return orders.Address{
		ID:      addr.ID,
		UserID:  addr.UserID,
		Label:   addr.Label,
		Street:  addr.Street,
		City:    addr.City,
		State:   addr.State,
		Pincode: addr.Pincode,
		Country: addr.Country,
	}, nil
}
