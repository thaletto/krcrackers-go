// Package customers provides customer profile and address management.
// All routes require authentication via the auth middleware.
package customers

import (
	"context"
	"errors"

	"github.com/thaletto/krcrackers-go/src/services/auth"
)

// ListAddressesResponse is the response for listing customer addresses.
type ListAddressesResponse struct {
	Items []Address `json:"items"`
	Total int       `json:"total"`
}

// Service owns the business operations for customer profiles and addresses.
// A profile belongs to the Account module and is read through the Accounts
// seam; this module owns addresses. It is HTTP-agnostic; the apis/customers
// package adapts it to routes.
type Service struct {
	repo    Repository
	account Accounts
}

// NewService creates a new customers service over an address repository and
// the Account module that owns profiles.
func NewService(repo Repository, account Accounts) *Service {
	return &Service{repo: repo, account: account}
}

// GetProfile returns the customer record for the given user. A missing account
// is reported as ErrNotFound so the HTTP layer maps it like a missing address.
func (s *Service) GetProfile(ctx context.Context, userID int) (auth.Profile, error) {
	profile, err := s.account.Profile(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return auth.Profile{}, ErrNotFound
		}
		return auth.Profile{}, err
	}
	return profile, nil
}

// UpdateProfile updates the customer's name and phone.
func (s *Service) UpdateProfile(ctx context.Context, userID int, name, phone string) (auth.Profile, error) {
	profile, err := s.account.UpdateProfile(ctx, userID, name, phone)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return auth.Profile{}, ErrNotFound
		}
		return auth.Profile{}, err
	}
	return profile, nil
}

// ListAddresses returns all addresses for the given customer.
func (s *Service) ListAddresses(ctx context.Context, userID int) ([]Address, error) {
	return s.repo.ListAddresses(ctx, userID)
}

// AddressFor returns one of the given customer's own addresses. An address
// belonging to somebody else is ErrNotFound, the same as one that does not
// exist, so a caller cannot use this to discover another account's addresses.
// Other modules that need a delivery address come through here rather than
// querying customer_addresses themselves.
func (s *Service) AddressFor(ctx context.Context, userID, addressID int) (Address, error) {
	addr, err := s.repo.GetAddress(ctx, userID, addressID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Address{}, ErrNotFound
		}
		return Address{}, err
	}
	return addr, nil
}

// CreateAddress validates and persists a new address.
func (s *Service) CreateAddress(ctx context.Context, userID int, input AddressInput) (Address, error) {
	if err := ValidateAddressInput(input); err != nil {
		return Address{}, err
	}
	return s.repo.CreateAddress(ctx, userID, input)
}

// UpdateAddress validates and persists an address update.
func (s *Service) UpdateAddress(ctx context.Context, userID, addressID int, input AddressInput) (Address, error) {
	if err := ValidateAddressInput(input); err != nil {
		return Address{}, err
	}
	return s.repo.UpdateAddress(ctx, userID, addressID, input)
}

// DeleteAddress removes an address. Returns ErrNotFound if the address
// does not belong to the user or does not exist.
func (s *Service) DeleteAddress(ctx context.Context, userID, addressID int) error {
	err := s.repo.DeleteAddress(ctx, userID, addressID)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// SetDefaultAddress marks the given address as the user's default. Returns
// ErrNotFound if the address does not belong to the user.
func (s *Service) SetDefaultAddress(ctx context.Context, userID, addressID int) error {
	err := s.repo.SetDefaultAddress(ctx, userID, addressID)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// ErrAddressIncomplete is returned when a new or updated address is missing one
// of the four fields needed to deliver to it. It is a sentinel rather than a
// formatted string so the HTTP layer can match it with errors.Is: the handler
// used to compare err.Error() against this exact text, which meant rewording
// the message silently turned a 422 into a 500.
var ErrAddressIncomplete = errors.New("street, city, state, and pincode are required")

// ValidateAddressInput checks the minimal required fields on a new or
// updated address.
func ValidateAddressInput(input AddressInput) error {
	if input.Street == "" || input.City == "" || input.State == "" || input.Pincode == "" {
		return ErrAddressIncomplete
	}
	return nil
}
