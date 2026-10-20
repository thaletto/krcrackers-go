package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/thaletto/krcrackers-go/src/database"
)

// User represents a user account with authentication details.
//
// AuthProvider records how the account was created ("email" or "google") and is
// the value reported to clients. GoogleID is the linked Google subject ("" when
// unlinked) and is independent of AuthProvider, so a password account can also
// sign in with Google. PasswordHash is the bcrypt hash ("" for Google-only
// accounts), which is what makes that second sign-in method possible.
type User struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	AvatarURL    string    `json:"avatarUrl"`
	AuthProvider string    `json:"authProvider"`
	GoogleID     string    `json:"-"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Profile is the public shape of an account: what a client may see about it,
// and nothing about how it authenticates. A Profile read never selects
// password_hash, google_id or auth_provider_id, so no secret crosses into
// another module to produce one.
type Profile struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	AvatarURL    string    `json:"avatarUrl"`
	AuthProvider string    `json:"authProvider"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// userColumns is the shared projection for user rows read by the Account
// module itself. Keeping it in one place stops the SELECT lists and rowToUser
// from drifting apart.
const userColumns = `id, email, name, phone, avatar_url, auth_provider, google_id, auth_provider_id, password_hash, role, created_at, updated_at`

// profileColumns is the projection for a Profile. It deliberately omits
// password_hash, google_id and auth_provider_id: a profile is what a client may
// see, so the secrets never leave the Account module's own reads.
const profileColumns = `id, email, name, phone, avatar_url, auth_provider, role, created_at, updated_at`

// Repository defines the data access interface for users and refresh tokens.
type Repository interface {
	Create(ctx context.Context, email, name, phone, authProvider, googleID, passwordHash, role string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByGoogleID(ctx context.Context, googleID string) (User, error)
	GetByID(ctx context.Context, id int) (User, error)
	// Profile reads the public shape of an account, without the columns that
	// describe how it authenticates.
	Profile(ctx context.Context, id int) (Profile, error)
	// CountCreatedSince counts accounts created at or after the given time.
	CountCreatedSince(ctx context.Context, since time.Time) (int, error)
	// LinkGoogleID attaches a Google subject to an existing account, preserving
	// the password hash so an already-registered email keeps working.
	LinkGoogleID(ctx context.Context, id int, googleID string) (User, error)
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

func (r *repo) Create(ctx context.Context, email, name, phone, authProvider, googleID, passwordHash, role string) (User, error) {
	res, err := r.db.Execute(ctx, `
		INSERT INTO users (email, name, phone, auth_provider, google_id, password_hash, role)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, email, name, phone, authProvider, googleID, passwordHash, role)
	if err != nil {
		return User{}, err
	}
	return r.GetByID(ctx, int(res.LastInsertID))
}

func (r *repo) GetByEmail(ctx context.Context, email string) (User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+userColumns+`
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

// GetByGoogleID resolves a Google subject to its linked account. Unlinked
// accounts store an empty or NULL google_id, so they never match.
func (r *repo) GetByGoogleID(ctx context.Context, googleID string) (User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+userColumns+`
		FROM users WHERE google_id = ?
	`, googleID)
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
		SELECT `+userColumns+`
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

func (r *repo) Profile(ctx context.Context, id int) (Profile, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+profileColumns+`
		FROM users WHERE id = ?
	`, id)
	if err != nil {
		return Profile{}, err
	}
	if len(rows) == 0 {
		return Profile{}, nil
	}
	return rowToProfile(rows[0])
}

func (r *repo) CountCreatedSince(ctx context.Context, since time.Time) (int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT COUNT(*) AS n FROM users WHERE created_at >= ?
	`, since.UTC().Format(time.DateTime))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	n, err := rows[0].Int("n")
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *repo) LinkGoogleID(ctx context.Context, id int, googleID string) (User, error) {
	_, err := r.db.Execute(ctx, `
		UPDATE users SET google_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, googleID, id)
	if err != nil {
		return User{}, err
	}
	return r.GetByID(ctx, id)
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

// refreshTokenFormat is how a refresh token's expiry is written and read.
// It is UTC with no zone suffix, so the value is unambiguous and identical on
// both backends.
//
// Handing the adapter a bare time.Time instead let each one format it its own
// way: SQLite wrote the local wall clock and GetRefreshToken read it back as
// UTC, so a token expiring at 00:30 IST was judged live until 00:30 UTC — five
// and a half hours past its life.
const refreshTokenFormat = "2006-01-02 15:04:05"

func (r *repo) CreateRefreshToken(ctx context.Context, userID int, token string, expiresAt time.Time) error {
	_, err := r.db.Execute(ctx, `
		INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)
	`, userID, hashRefreshToken(token), expiresAt.UTC().Format(refreshTokenFormat))
	return err
}

// hashRefreshToken is what the refresh_tokens table holds instead of the token
// itself. A read of that table — a backup, a dump, a mistake in some other
// query — must not hand over a week of bearer credentials for every account,
// the way the bcrypt'd password column does not.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *repo) GetRefreshToken(ctx context.Context, token string) (int, time.Time, error) {
	// A token issued before hashing was introduced is still stored verbatim, so
	// the lookup tries both forms and upgrades the row when it matches the old
	// one. The column is an opaque key either way, which is why this needs no
	// schema change.
	digest := hashRefreshToken(token)
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, expires_at, token FROM refresh_tokens WHERE token IN (?, ?)
	`, digest, token)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(rows) == 0 {
		return 0, time.Time{}, nil
	}

	id, err := rows[0].Int("id")
	if err != nil {
		return 0, time.Time{}, err
	}
	userID, err := rows[0].Int("user_id")
	if err != nil {
		return 0, time.Time{}, err
	}
	expiresStr, err := rows[0].String("expires_at")
	if err != nil {
		return 0, time.Time{}, err
	}
	expiresAt, err := time.Parse(refreshTokenFormat, expiresStr)
	if err != nil {
		// A value this code cannot read is a bug, not an expired token. Returning
		// it stops a malformed row from reading as "expires at the zero time",
		// which would make every use of it look expired.
		return 0, time.Time{}, fmt.Errorf("refresh token expiry %q: %w", expiresStr, err)
	}

	if stored, err := rows[0].String("token"); err == nil && stored == token && token != digest {
		_, _ = r.db.Execute(ctx, `UPDATE refresh_tokens SET token = ? WHERE id = ?`, digest, id)
	}
	return int(userID), expiresAt, nil
}

func (r *repo) DeleteRefreshToken(ctx context.Context, token string) error {
	_, err := r.db.Execute(ctx, `DELETE FROM refresh_tokens WHERE token IN (?, ?)`,
		hashRefreshToken(token), token)
	return err
}

func (r *repo) DeleteUserRefreshTokens(ctx context.Context, userID int) error {
	_, err := r.db.Execute(ctx, `DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	return err
}

func rowToProfile(row database.Row) (Profile, error) {
	id, err := row.Int("id")
	if err != nil {
		return Profile{}, err
	}
	email, err := row.String("email")
	if err != nil {
		return Profile{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return Profile{}, err
	}
	phone, err := row.String("phone")
	if err != nil {
		return Profile{}, err
	}
	avatarURL, err := row.String("avatar_url")
	if err != nil {
		return Profile{}, err
	}
	authProvider, err := row.String("auth_provider")
	if err != nil {
		return Profile{}, err
	}
	role, err := row.String("role")
	if err != nil {
		return Profile{}, err
	}
	createdAtStr, err := row.String("created_at")
	if err != nil {
		return Profile{}, err
	}
	updatedAtStr, err := row.String("updated_at")
	if err != nil {
		return Profile{}, err
	}
	createdAt, _ := time.Parse(time.DateTime, createdAtStr)
	updatedAt, _ := time.Parse(time.DateTime, updatedAtStr)
	return Profile{
		ID:           int(id),
		Email:        email,
		Name:         name,
		Phone:        phone,
		AvatarURL:    avatarURL,
		AuthProvider: authProvider,
		Role:         role,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
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
	// Nullable: rows that predate the google_id column hold NULL there.
	googleID, err := row.NullableString("google_id")
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
	user := User{
		ID:           int(id),
		Email:        email,
		Name:         name,
		Phone:        phone,
		AvatarURL:    avatarURL,
		AuthProvider: authProvider,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if googleID != nil {
		user.GoogleID = *googleID
	}
	return user, nil
}
