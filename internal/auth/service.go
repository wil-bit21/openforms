package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/db"
)

// bcryptCost is a variable so tests can lower it.
var bcryptCost = 12

// defaultOrgLock is the advisory lock key serialising default-org creation.
const defaultOrgLock = 7243001

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// EnsureDefaultOrg returns the single org, creating "Default" on first call.
func (s *Service) EnsureDefaultOrg(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, defaultOrgLock); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id FROM orgs ORDER BY created_at, id LIMIT 1`).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			id = uuid.New()
			_, err = tx.Exec(ctx, `INSERT INTO orgs (id, name) VALUES ($1, 'Default')`, id)
		}
		return err
	})
	return id, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
