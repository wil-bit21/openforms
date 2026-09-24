package actions

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/openforms/openforms/internal/jobs"
)

func (r *runner) email(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	a := ac.payload.Action
	to := strings.TrimSpace(Render(a.To, ac.vars))
	if to == "" {
		return jobs.Permanent(fmt.Errorf("email: recipient template %q rendered empty", a.To))
	}
	if _, err := mail.ParseAddress(to); err != nil || strings.ContainsAny(to, "\r\n") {
		return jobs.Permanent(fmt.Errorf("email: invalid recipient %q", to))
	}
	if err := r.d.Mailer.Send(ctx, to, Render(a.Subject, ac.vars), Render(a.Body, ac.vars)); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	return r.recordSuccess(ctx, ac)
}
