package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
)

type User struct {
	ID, OrgID uuid.UUID
	Email     string
	Name      string
	Roles     []string
	CreatedAt time.Time
}

type UpdateUserInput struct {
	Name     *string
	Password *string
	Roles    []string // nil = unchanged
}

const userColumns = "id, org_id, email, name, roles, created_at"

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Roles, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if u.Roles == nil {
		u.Roles = []string{}
	}
	return u, err
}

func collectUsers(rows pgx.Rows, err error) ([]User, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) CreateUser(ctx context.Context, orgID uuid.UUID, email, name, password string, roles []string) (User, error) {
	e, emailProb := normalizeEmail(email)
	name = strings.TrimSpace(name)
	probs := collect(nil, emailProb, nameProblem(name), passwordProblem(password))
	rs, roleProbs := normalizeRoles(roles)
	probs = append(probs, roleProbs...)
	if len(probs) > 0 {
		return User{}, &definition.ValidationError{Problems: probs}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, err
	}
	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (id, org_id, email, name, password_hash, roles)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+userColumns,
		uuid.New(), orgID, e, name, string(hash), rs))
	if isUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	return u, err
}

func (s *Service) ListUsers(ctx context.Context, orgID uuid.UUID) ([]User, error) {
	return collectUsers(s.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 ORDER BY created_at, email`, orgID))
}

func (s *Service) GetUserByEmail(ctx context.Context, orgID uuid.UUID, email string) (User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 AND lower(email) = lower($2)`,
		orgID, strings.TrimSpace(email)))
}

func (s *Service) UsersWithRole(ctx context.Context, orgID uuid.UUID, role string) ([]User, error) {
	return collectUsers(s.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 AND $2 = ANY(roles) ORDER BY created_at, id`,
		orgID, role))
}

// lockAdmins locks every admin row (ordered by id, so concurrent callers never deadlock).
func lockAdmins(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM users WHERE org_id = $1 AND $2 = ANY(roles) ORDER BY id FOR UPDATE`, orgID, RoleAdmin)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func lastAdminError(admins []uuid.UUID, id uuid.UUID, path string) error {
	for _, a := range admins {
		if a != id {
			return nil
		}
	}
	return &definition.ValidationError{Problems: []definition.Problem{{Path: path, Message: "at least one admin must remain"}}}
}

func (s *Service) UpdateUser(ctx context.Context, orgID, id uuid.UUID, in UpdateUserInput) (User, error) {
	var probs []definition.Problem
	var name *string
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		probs = collect(probs, nameProblem(n))
		name = &n
	}
	if in.Password != nil {
		probs = collect(probs, passwordProblem(*in.Password))
	}
	var roles []string
	if in.Roles != nil {
		rs, rp := normalizeRoles(in.Roles)
		roles, probs = rs, append(probs, rp...)
	}
	if len(probs) > 0 {
		return User{}, &definition.ValidationError{Problems: probs}
	}
	var hash *string
	if in.Password != nil {
		h, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcryptCost)
		if err != nil {
			return User{}, err
		}
		hs := string(h)
		hash = &hs
	}

	var out User
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var admins []uuid.UUID
		if roles != nil {
			var err error
			if admins, err = lockAdmins(ctx, tx, orgID); err != nil {
				return err
			}
		}
		cur, err := scanUser(tx.QueryRow(ctx,
			`SELECT `+userColumns+` FROM users WHERE id = $1 AND org_id = $2 FOR UPDATE`, id, orgID))
		if err != nil {
			return err
		}
		if roles == nil {
			roles = cur.Roles
		} else if slices.Contains(cur.Roles, RoleAdmin) && !slices.Contains(roles, RoleAdmin) {
			if err := lastAdminError(admins, id, "roles"); err != nil {
				return err
			}
		}
		if name == nil {
			name = &cur.Name
		}
		out, err = scanUser(tx.QueryRow(ctx,
			`UPDATE users SET name = $3, roles = $4, password_hash = COALESCE($5, password_hash)
			 WHERE id = $1 AND org_id = $2 RETURNING `+userColumns,
			id, orgID, *name, roles, hash))
		if err != nil {
			return err
		}
		if hash != nil {
			_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
		}
		return err
	})
	return out, err
}

func (s *Service) DeleteUser(ctx context.Context, orgID, id uuid.UUID) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		admins, err := lockAdmins(ctx, tx, orgID)
		if err != nil {
			return err
		}
		cur, err := scanUser(tx.QueryRow(ctx,
			`SELECT `+userColumns+` FROM users WHERE id = $1 AND org_id = $2 FOR UPDATE`, id, orgID))
		if err != nil {
			return err
		}
		if slices.Contains(cur.Roles, RoleAdmin) {
			if err := lastAdminError(admins, id, "id"); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1 AND org_id = $2`, id, orgID)
		return err
	})
}
