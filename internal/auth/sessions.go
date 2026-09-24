package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const SessionTTL = 30 * 24 * time.Hour

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// compareDummy spends the same time as a real password check so unknown emails
// are not distinguishable by timing.
func compareDummy(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("openforms-timing-equaliser"), bcryptCost)
	})
	bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) //nolint:errcheck
}

func (s *Service) Login(ctx context.Context, email, password string) (string, User, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	var u User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE lower(email) = $1 ORDER BY created_at LIMIT 1`, e).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Roles, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		compareDummy(password)
		return "", User{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", User{}, ErrInvalidCredentials
	}
	token, err := randomToken(32)
	if err != nil {
		return "", User{}, err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", User{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), u.ID, time.Now().Add(SessionTTL)); err != nil {
		return "", User{}, err
	}
	if u.Roles == nil {
		u.Roles = []string{}
	}
	return token, u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (s *Service) PrincipalFromSession(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{Kind: PrincipalUser}
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.org_id, u.email, u.name, u.roles
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(token)).
		Scan(&p.ID, &p.OrgID, &p.Email, &p.Name, &p.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	return p, err
}
