package definition

import "testing"

func baseForm() Form {
	return Form{
		Slug:     "job-application",
		Title:    "Job application",
		Workflow: "hiring",
		Settings: FormSettings{Public: true},
		Fields: []Field{
			{Key: "name", Type: FieldText, Label: "Full name", Required: true,
				Validation: &Validation{MinLength: intPtr(2), MaxLength: intPtr(100)}},
			{Key: "role", Type: FieldSelect, Label: "Role", Required: true,
				Options: []Option{{Value: "engineer", Label: "Engineer"}, {Value: "designer", Label: "Designer"}}},
			{Key: "portfolio", Type: FieldURL, Label: "Portfolio",
				ShowIf: &Condition{Field: "role", Equals: "designer"}},
			{Key: "years", Type: FieldNumber, Label: "Years of experience",
				Validation: &Validation{Min: floatPtr(0), Max: floatPtr(50)}},
		},
	}
}

func TestValidateFormAcceptsBase(t *testing.T) {
	if err := ValidateForm(baseForm()); err != nil {
		t.Fatalf("base form should be valid: %v", err)
	}
}

func TestValidateFormRules(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(f *Form)
		wantPath string
	}{
		{"bad slug", func(f *Form) { f.Slug = "Bad Slug" }, "slug"},
		{"empty title", func(f *Form) { f.Title = "  " }, "title"},
		{"bad workflow slug", func(f *Form) { f.Workflow = "Hiring!" }, "workflow"},
		{"no fields", func(f *Form) { f.Fields = nil }, "fields"},
		{"bad key", func(f *Form) { f.Fields[0].Key = "1name" }, "fields[0].key"},
		{"duplicate key", func(f *Form) { f.Fields[1].Key = "name" }, "fields[1].key"},
		{"empty label", func(f *Form) { f.Fields[0].Label = "" }, "fields[0].label"},
		{"unknown type", func(f *Form) { f.Fields[0].Type = "color" }, "fields[0].type"},
		{"select without options", func(f *Form) { f.Fields[1].Options = nil }, "fields[1].options"},
		{"duplicate option value", func(f *Form) { f.Fields[1].Options[1].Value = "engineer" }, "fields[1].options[1].value"},
		{"option value whitespace", func(f *Form) { f.Fields[1].Options[0].Value = " engineer" }, "fields[1].options[0].value"},
		{"options on text", func(f *Form) { f.Fields[0].Options = []Option{{Value: "a", Label: "A"}} }, "fields[0].options"},
		{"negative minLength", func(f *Form) { f.Fields[0].Validation.MinLength = intPtr(-1) }, "fields[0].validation.minLength"},
		{"minLength above maxLength", func(f *Form) {
			f.Fields[0].Validation.MinLength = intPtr(10)
			f.Fields[0].Validation.MaxLength = intPtr(5)
		}, "fields[0].validation.maxLength"},
		{"min above max", func(f *Form) { f.Fields[3].Validation.Min = floatPtr(60) }, "fields[3].validation.max"},
		{"minLength on number", func(f *Form) { f.Fields[3].Validation.MinLength = intPtr(1) }, "fields[3].validation.minLength"},
		{"pattern on number", func(f *Form) { f.Fields[3].Validation.Pattern = "x" }, "fields[3].validation.pattern"},
		{"min on text", func(f *Form) { f.Fields[0].Validation.Min = floatPtr(1) }, "fields[0].validation.min"},
		{"invalid pattern", func(f *Form) { f.Fields[0].Validation.Pattern = "(" }, "fields[0].validation.pattern"},
		{"self reference showIf", func(f *Form) { f.Fields[2].ShowIf.Field = "portfolio" }, "fields[2].showIf.field"},
		{"showIf later field", func(f *Form) { f.Fields[0].ShowIf = &Condition{Field: "role", Equals: "x"} }, "fields[0].showIf.field"},
		{"showIf unknown field", func(f *Form) { f.Fields[2].ShowIf.Field = "nope" }, "fields[2].showIf.field"},
		{"showIf without operator", func(f *Form) { f.Fields[2].ShowIf = &Condition{Field: "role"} }, "fields[2].showIf"},
		{"showIf two operators", func(f *Form) {
			f.Fields[2].ShowIf = &Condition{Field: "role", Equals: "designer", NotEquals: "engineer"}
		}, "fields[2].showIf"},
		{"showIf empty in", func(f *Form) { f.Fields[2].ShowIf = &Condition{Field: "role", In: []any{}} }, "fields[2].showIf.in"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseForm()
			tc.mutate(&f)
			paths := problemPaths(t, ValidateForm(f))
			if !hasPath(paths, tc.wantPath) {
				t.Fatalf("want problem at %q, got %v", tc.wantPath, paths)
			}
		})
	}
}

func TestValidateFormReportsAllProblems(t *testing.T) {
	f := baseForm()
	f.Slug = "BAD"
	f.Title = ""
	f.Fields[0].Key = "1x"
	paths := problemPaths(t, ValidateForm(f))
	for _, want := range []string{"slug", "title", "fields[0].key"} {
		if !hasPath(paths, want) {
			t.Errorf("missing %q in %v", want, paths)
		}
	}
}
