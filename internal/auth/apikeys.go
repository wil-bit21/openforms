package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

const apiKeyPrefix = "ofk_"

type APIKey struct {
	ID, OrgID  uuid.UUID
	Name       string
	Prefix     string
	Roles      []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

const apiKeyColumns = "id, org_id, name, prefix, roles, created_at, last_used_at, revoked_at"

func scanAPIKey(row pgx.Row) (APIKey, error) {
	var k APIKey
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.Roles, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if k.Roles == nil {
		k.Roles = []string{}
	}
	return k, err
}

func (s *Service) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name string, roles []string) (string, APIKey, error) {
	name = strings.TrimSpace(name)
	var probs []definition.Problem
	if name == "" || len(name) > 100 {
		probs = append(probs, definition.Problem{Path: "name", Message: "is required (max 100 characters)"})
	}
	rs, rp := normalizeRoles(roles)
	probs = append(probs, rp...)
	if len(probs) > 0 {
		return "", APIKey{}, &definition.ValidationError{Problems: probs}
	}
	secret, err := randomBase62(40)
	if err != nil {
		return "", APIKey{}, err
	}
	plaintext := apiKeyPrefix + secret
	k, err := scanAPIKey(s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (id, org_id, name, prefix, key_hash, roles)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+apiKeyColumns,
		uuid.New(), orgID, name, plaintext[:8], hashToken(plaintext), rs))
	if err != nil {
		return "", APIKey{}, err
	}
	return plaintext, k, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE org_id = $1 ORDER BY created_at DESC, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey is idempotent; it returns ErrNotFound only for unknown ids.
func (s *Service) RevokeAPIKey(ctx context.Context, orgID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) PrincipalFromAPIKey(ctx context.Context, plaintext string) (Principal, error) {
	if !strings.HasPrefix(plaintext, apiKeyPrefix) {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{Kind: PrincipalAPIKey}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, roles FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
		hashToken(plaintext)).Scan(&p.ID, &p.OrgID, &p.Name, &p.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	// Throttled so busy keys do not write on every request.
	if _, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now()
		 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, p.ID); err != nil {
		return Principal{}, err
	}
	return p, nil
}
