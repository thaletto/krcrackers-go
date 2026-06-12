package auth

import (
	"context"
	"time"

	"github.com/thaletto/krcrackers-go/database"
)

// User represents a user account with authentication details.
type User struct {
	ID             int       `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	AvatarURL      string    `json:"avatarUrl"`
	AuthProvider   string    `json:"authProvider"`
	AuthProviderID string    `json:"-"`
	PasswordHash   string    `json:"-"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Repository defines the data access interface for users and refresh tokens.
type Repository interface {
	Create(ctx context.Context, email, name, phone, authProvider, authProviderID, passwordHash, role string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByID(ctx context.Context, id int) (User, error)
	Update(ctx context.Context, id int, name, phone string) (User, error)
	CreateRefreshToken(ctx context.Context, userID int, token string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, token string) (userID int, expiresAt time.Time, err error)
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteUserRefreshTokens(ctx context.Context, userID int) error
}

type repo struct {
	db database.DB
}

// NewRepository returns a new auth repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, email, name, phone, authProvider, authProviderID, passwordHash, role string) (User, error) {
	res, err := r.db.Execute(ctx, `
		INSERT INTO users (email, name, phone, auth_provider, auth_provider_id, password_hash, role)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, email, name, phone, authProvider, authProviderID, passwordHash, role)
	if err != nil {
		return User{}, err
	}
	return r.GetByID(ctx, int(res.LastInsertID))
}

func (r *repo) GetByEmail(ctx context.Context, email string) (User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, name, phone, avatar_url, auth_provider, auth_provider_id, password_hash, role, created_at, updated_at
		FROM users WHERE email = ?
	`, email)
	if err != nil {
		return User{}, err
	}
	if len(rows) == 0 {
		return User{}, nil
	}
	return rowToUser(rows[0])
}

func (r *repo) GetByID(ctx context.Context, id int) (User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, name, phone, avatar_url, auth_provider, auth_provider_id, password_hash, role, created_at, updated_at
		FROM users WHERE id = ?
	`, id)
	if err != nil {
		return User{}, err
	}
	if len(rows) == 0 {
		return User{}, nil
	}
	return rowToUser(rows[0])
}

func (r *repo) Update(ctx context.Context, id int, name, phone string) (User, error) {
	_, err := r.db.Execute(ctx, `
		UPDATE users SET name = ?, phone = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, name, phone, id)
	if err != nil {
		return User{}, err
	}
	return r.GetByID(ctx, id)
}

func (r *repo) CreateRefreshToken(ctx context.Context, userID int, token string, expiresAt time.Time) error {
	_, err := r.db.Execute(ctx, `
		INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)
	`, userID, token, expiresAt)
	return err
}

func (r *repo) GetRefreshToken(ctx context.Context, token string) (int, time.Time, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, expires_at FROM refresh_tokens WHERE token = ?
	`, token)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(rows) == 0 {
		return 0, time.Time{}, nil
	}
	userID, err := rows[0].Int("user_id")
	if err != nil {
		return 0, time.Time{}, err
	}
	expiresStr, err := rows[0].String("expires_at")
	if err != nil {
		return 0, time.Time{}, err
	}
	expiresAt, _ := time.Parse(time.DateTime, expiresStr)
	return int(userID), expiresAt, nil
}

func (r *repo) DeleteRefreshToken(ctx context.Context, token string) error {
	_, err := r.db.Execute(ctx, `DELETE FROM refresh_tokens WHERE token = ?`, token)
	return err
}

func (r *repo) DeleteUserRefreshTokens(ctx context.Context, userID int) error {
	_, err := r.db.Execute(ctx, `DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	return err
}

func rowToUser(row database.Row) (User, error) {
	id, err := row.Int("id")
	if err != nil {
		return User{}, err
	}
	email, err := row.String("email")
	if err != nil {
		return User{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return User{}, err
	}
	phone, err := row.String("phone")
	if err != nil {
		return User{}, err
	}
	avatarURL, err := row.String("avatar_url")
	if err != nil {
		return User{}, err
	}
	authProvider, err := row.String("auth_provider")
	if err != nil {
		return User{}, err
	}
	authProviderID, err := row.String("auth_provider_id")
	if err != nil {
		return User{}, err
	}
	passwordHash, err := row.String("password_hash")
	if err != nil {
		return User{}, err
	}
	role, err := row.String("role")
	if err != nil {
		return User{}, err
	}
	createdAtStr, err := row.String("created_at")
	if err != nil {
		return User{}, err
	}
	updatedAtStr, err := row.String("updated_at")
	if err != nil {
		return User{}, err
	}
	createdAt, _ := time.Parse(time.DateTime, createdAtStr)
	updatedAt, _ := time.Parse(time.DateTime, updatedAtStr)
	return User{
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
