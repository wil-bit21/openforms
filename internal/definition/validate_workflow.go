package definition

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

var workflowFieldTypes = map[FieldType]bool{
	FieldText: true, FieldTextarea: true, FieldNumber: true, FieldSelect: true, FieldCheckbox: true, FieldDate: true,
}

var stateColors = map[string]bool{"gray": true, "blue": true, "green": true, "yellow": true, "red": true, "purple": true}

// ValidateWorkflow applies the semantic rules of spec §5.3 to a workflow and
// reports every problem it finds.
func ValidateWorkflow(w Workflow) error {
	var ps problems
	if !slugRe.MatchString(w.Slug) {
		ps.add("slug", slugRule)
	}
	if strings.TrimSpace(w.Title) == "" {
		ps.add("title", "is required")
	}

	if len(w.States) == 0 {
		ps.add("states", "at least one state is required")
	}
	states := make(map[string]State, len(w.States))
	for i, s := range w.States {
		p := fmt.Sprintf("states[%d]", i)
		if !keyRe.MatchString(s.Key) {
			ps.add(p+".key", keyRule)
		} else if _, dup := states[s.Key]; dup {
			ps.add(p+".key", fmt.Sprintf("duplicate state key %q", s.Key))
		} else {
			states[s.Key] = s
		}
		if strings.TrimSpace(s.Label) == "" {
			ps.add(p+".label", "is required")
		}
		if s.Color != "" && !stateColors[s.Color] {
			ps.add(p+".color", "must be one of gray, blue, green, yellow, red, purple")
		}
	}
	if _, ok := states[w.Initial]; !ok {
		ps.add("initial", fmt.Sprintf("unknown state %q", w.Initial))
	}

	fields := make(map[string]bool, len(w.Fields))
	for i, f := range w.Fields {
		p := fmt.Sprintf("fields[%d]", i)
		if !keyRe.MatchString(f.Key) {
			ps.add(p+".key", keyRule)
		} else if fields[f.Key] {
			ps.add(p+".key", fmt.Sprintf("duplicate workflow field key %q", f.Key))
		} else {
			fields[f.Key] = true
		}
		if !workflowFieldTypes[f.Type] {
			ps.add(p+".type", fmt.Sprintf("type %q is not allowed for workflow fields (use text, textarea, number, select, checkbox or date)", f.Type))
		}
		if strings.TrimSpace(f.Label) == "" {
			ps.add(p+".label", "is required")
		}
		validateOptions(&ps, p, f.Type == FieldSelect, "options are only allowed on select fields", f.Options)
	}

	for j, a := range w.OnSubmit {
		validateAction(&ps, fmt.Sprintf("onSubmit[%d]", j), a)
	}

	seen := make(map[string]bool, len(w.Transitions))
	for i, t := range w.Transitions {
		p := fmt.Sprintf("transitions[%d]", i)
		if !keyRe.MatchString(t.Key) {
			ps.add(p+".key", keyRule)
		} else if seen[t.Key] {
			ps.add(p+".key", fmt.Sprintf("duplicate transition key %q", t.Key))
		} else {
			seen[t.Key] = true
		}
		if strings.TrimSpace(t.Label) == "" {
			ps.add(p+".label", "is required")
		}
		if len(t.From) == 0 {
			ps.add(p+".from", "at least one source state is required")
		}
		for j, from := range t.From {
			fp := fmt.Sprintf("%s.from[%d]", p, j)
			s, ok := states[from]
			switch {
			case !ok:
				ps.add(fp, fmt.Sprintf("unknown state %q", from))
			case s.Terminal:
				ps.add(fp, fmt.Sprintf("cannot transition out of terminal state %q", from))
			}
		}
		if _, ok := states[t.To]; !ok {
			ps.add(p+".to", fmt.Sprintf("unknown state %q", t.To))
		}
		for j, r := range t.Guard.Roles {
			if strings.TrimSpace(r) == "" {
				ps.add(fmt.Sprintf("%s.guard.roles[%d]", p, j), "must not be empty")
			}
		}
		for j, rf := range t.Guard.RequireFields {
			if !fields[rf] {
				ps.add(fmt.Sprintf("%s.guard.requireFields[%d]", p, j), fmt.Sprintf("unknown workflow field %q", rf))
			}
		}
		for j, a := range t.Actions {
			validateAction(&ps, fmt.Sprintf("%s.actions[%d]", p, j), a)
		}
	}
	return ps.err()
}

// validateAction checks the per-type rules of spec §5.2.
func validateAction(ps *problems, p string, a Action) {
	notAllowed := func(name, val string) {
		if val != "" {
			ps.add(p+"."+name, fmt.Sprintf("not allowed on %s actions", a.Type))
		}
	}
	switch a.Type {
	case ActionWebhook:
		if a.URL == "" {
			ps.add(p+".url", "is required")
		} else if u, err := url.Parse(a.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			ps.add(p+".url", "must be an absolute http(s) URL")
		}
		notAllowed("to", a.To)
		notAllowed("subject", a.Subject)
		notAllowed("body", a.Body)
		notAllowed("user", a.User)
		notAllowed("role", a.Role)
	case ActionEmail:
		if strings.TrimSpace(a.To) == "" {
			ps.add(p+".to", "is required")
		}
		if strings.TrimSpace(a.Subject) == "" {
			ps.add(p+".subject", "is required")
		}
		notAllowed("url", a.URL)
		notAllowed("user", a.User)
		notAllowed("role", a.Role)
	case ActionAssign:
		if (a.User == "") == (a.Role == "") {
			ps.add(p, "exactly one of user or role is required")
		}
		if a.User != "" {
			if addr, err := mail.ParseAddress(a.User); err != nil || addr.Address != a.User {
				ps.add(p+".user", "must be an email address")
			}
		}
		notAllowed("url", a.URL)
		notAllowed("to", a.To)
		notAllowed("subject", a.Subject)
		notAllowed("body", a.Body)
	default:
		ps.add(p+".type", fmt.Sprintf("unknown action type %q", a.Type))
	}
}
