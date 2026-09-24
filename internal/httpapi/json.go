package httpapi

import (
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

type userJSON struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"createdAt"`
}

func toUserJSON(u auth.User) userJSON {
	return userJSON{ID: u.ID, Email: u.Email, Name: u.Name, Roles: nonNil(u.Roles), CreatedAt: u.CreatedAt.UTC()}
}

type principalJSON struct {
	Kind  auth.PrincipalKind `json:"kind"`
	ID    uuid.UUID          `json:"id"`
	Name  string             `json:"name"`
	Email string             `json:"email"`
	Roles []string           `json:"roles"`
}

func toPrincipalJSON(p auth.Principal) principalJSON {
	return principalJSON{Kind: p.Kind, ID: p.ID, Name: p.Name, Email: p.Email, Roles: nonNil(p.Roles)}
}

// nonNil makes sure slices serialise as [] rather than null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
