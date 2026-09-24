package actions

import (
	"strings"

	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// TemplateVars builds the variables available to email templates (spec §6.9).
func TemplateVars(cfg config.Config, form definition.Form, sub submissions.Submission, transition *definition.Transition) map[string]any {
	base := strings.TrimRight(cfg.BaseURL, "/")
	tr := map[string]any{}
	if transition != nil {
		tr["key"] = transition.Key
		tr["label"] = transition.Label
	}
	return map[string]any{
		"baseUrl":    base,
		"form":       map[string]any{"slug": form.Slug, "title": form.Title},
		"transition": tr,
		"submission": map[string]any{
			"id":         sub.ID.String(),
			"state":      sub.State,
			"stateLabel": sub.StateLabel,
			"data":       nonNil(sub.Data),
			"fields":     nonNil(sub.Fields),
			"url":        base + "/admin/submissions/" + sub.ID.String(),
		},
	}
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
