// Package customers provides customer profile and address management.
// All routes require authentication via the auth middleware.
package customers

import (
	"context"
	"errors"
	"fmt"

	"github.com/thaletto/krcrackers-go/src/services/auth"
)

// ListAddressesResponse is the response for listing customer addresses.
type ListAddressesResponse struct {
	Items []Address `json:"items"`
	Total int       `json:"total"`
}

// Service owns the business operations for customer profiles and addresses.
// It is HTTP-agnostic; the apis/customers package adapts it to routes.
type Service struct {
	repo Repository
}

// NewService creates a new customers service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetProfile returns the customer record (auth.User) for the given user.
func (s *Service) GetProfile(ctx context.Context, userID int) (auth.User, error) {
	profile, err := s.repo.GetProfile(ctx, userID)
	if err != nil {
		return auth.User{}, err
	}
	if profile.ID == 0 {
		return auth.User{}, ErrNotFound
	}
	return profile, nil
}

// UpdateProfile updates the customer's name and phone.
func (s *Service) UpdateProfile(ctx context.Context, userID int, name, phone string) (auth.User, error) {
	return s.repo.UpdateProfile(ctx, userID, name, phone)
}

// ListAddresses returns all addresses for the given customer.
func (s *Service) ListAddresses(ctx context.Context, userID int) ([]Address, error) {
	return s.repo.ListAddresses(ctx, userID)
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

// ValidateAddressInput checks the minimal required fields on a new or
// updated address.
func ValidateAddressInput(input AddressInput) error {
	if input.Street == "" || input.City == "" || input.State == "" || input.Pincode == "" {
		return fmt.Errorf("street, city, state, and pincode are required")
	}
	return nil
}
