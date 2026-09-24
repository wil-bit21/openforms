package actions_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

func TestRender(t *testing.T) {
	vars := map[string]any{
		"submission": map[string]any{
			"data": map[string]any{
				"name":   "Ada",
				"score":  float64(3),
				"ratio":  0.25,
				"ok":     true,
				"tags":   []any{"a", "b"},
				"nested": map[string]any{"x": float64(1)},
				"none":   nil,
			},
		},
		"baseUrl": "https://forms.example.com",
	}
	cases := []struct{ tmpl, want string }{
		{"Hi {{submission.data.name}}!", "Hi Ada!"},
		{"Hi {{ submission.data.name }}!", "Hi Ada!"},
		{"{{submission.data.score}}", "3"},
		{"{{submission.data.ratio}}", "0.25"},
		{"{{submission.data.ok}}", "true"},
		{"{{submission.data.tags}}", "a, b"},
		{"{{submission.data.nested}}", `{"x":1}`},
		{"[{{submission.data.none}}]", "[]"},
		{"[{{submission.data.missing}}]", "[]"},
		{"[{{nope.deeper.still}}]", "[]"},
		{"[{{submission.data.name.first}}]", "[]"},
		{"{{baseUrl}}/x", "https://forms.example.com/x"},
		{"no placeholders", "no placeholders"},
		{"{{ not closed", "{{ not closed"},
		{"<b>{{submission.data.name}}</b>", "<b>Ada</b>"},
	}
	for _, c := range cases {
		if got := actions.Render(c.tmpl, vars); got != c.want {
			t.Errorf("Render(%q) = %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestTemplateVars(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	sub := submissions.Submission{
		ID: id, State: "screening", StateLabel: "Screening",
		Data:      map[string]any{"email": "ada@example.com"},
		Fields:    map[string]any{"score": float64(4)},
		CreatedAt: time.Now(),
	}
	form := definition.Form{Slug: "apply", Title: "Apply"}
	tr := &definition.Transition{Key: "invite", Label: "Invite"}
	cfg := config.Config{BaseURL: "https://forms.example.com/"}

	vars := actions.TemplateVars(cfg, form, sub, tr)
	checks := map[string]string{
		"{{submission.id}}":           id.String(),
		"{{submission.state}}":        "screening",
		"{{submission.stateLabel}}":   "Screening",
		"{{submission.data.email}}":   "ada@example.com",
		"{{submission.fields.score}}": "4",
		"{{submission.url}}":          "https://forms.example.com/admin/submissions/" + id.String(),
		"{{form.slug}}":               "apply",
		"{{form.title}}":              "Apply",
		"{{transition.key}}":          "invite",
		"{{transition.label}}":        "Invite",
		"{{baseUrl}}":                 "https://forms.example.com",
		"[{{submission.statusUrl}}]":  "[]",
	}
	for tmpl, want := range checks {
		if got := actions.Render(tmpl, vars); got != want {
			t.Errorf("%s = %q, want %q", tmpl, got, want)
		}
	}

	noTr := actions.TemplateVars(cfg, form, submissions.Submission{ID: id}, nil)
	if got := actions.Render("[{{transition.key}}][{{submission.data.x}}]", noTr); got != "[][]" {
		t.Errorf("nil transition/data rendered %q", got)
	}
}
