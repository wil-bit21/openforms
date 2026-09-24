package definition

import (
	"fmt"
	"regexp"
	"strings"
)

var formFieldTypes = map[FieldType]bool{
	FieldText: true, FieldTextarea: true, FieldEmail: true, FieldNumber: true, FieldSelect: true,
	FieldMultiselect: true, FieldCheckbox: true, FieldDate: true, FieldURL: true,
}

// stringFieldTypes may carry minLength/maxLength/pattern.
var stringFieldTypes = map[FieldType]bool{FieldText: true, FieldTextarea: true, FieldEmail: true, FieldURL: true}

// ValidateForm applies the semantic rules of spec §5.3 to a form. It reports
// every problem it finds, not just the first.
func ValidateForm(f Form) error {
	var ps problems
	if !slugRe.MatchString(f.Slug) {
		ps.add("slug", slugRule)
	}
	if strings.TrimSpace(f.Title) == "" {
		ps.add("title", "is required")
	}
	if f.Workflow != "" && !slugRe.MatchString(f.Workflow) {
		ps.add("workflow", "must be a valid workflow slug")
	}
	if len(f.Fields) == 0 {
		ps.add("fields", "at least one field is required")
	}
	seen := make(map[string]int, len(f.Fields)) // key → index of first declaration
	for i, fld := range f.Fields {
		p := fmt.Sprintf("fields[%d]", i)
		if !keyRe.MatchString(fld.Key) {
			ps.add(p+".key", keyRule)
		} else if j, dup := seen[fld.Key]; dup {
			ps.add(p+".key", fmt.Sprintf("duplicate key %q (also used by fields[%d])", fld.Key, j))
		} else {
			seen[fld.Key] = i
		}
		if !formFieldTypes[fld.Type] {
			ps.add(p+".type", fmt.Sprintf("unknown field type %q", fld.Type))
		}
		if strings.TrimSpace(fld.Label) == "" {
			ps.add(p+".label", "is required")
		}
		validateOptions(&ps, p, fld.Type == FieldSelect || fld.Type == FieldMultiselect,
			"options are only allowed on select and multiselect fields", fld.Options)
		validateFieldValidation(&ps, p, fld)
		if fld.ShowIf != nil {
			validateCondition(&ps, p+".showIf", *fld.ShowIf, seen, i)
		}
	}
	return ps.err()
}

func validateFieldValidation(ps *problems, p string, fld Field) {
	v := fld.Validation
	if v == nil {
		return
	}
	vp := p + ".validation"
	if !stringFieldTypes[fld.Type] {
		const msg = "is only allowed on text, textarea, email and url fields"
		if v.MinLength != nil {
			ps.add(vp+".minLength", "minLength "+msg)
		}
		if v.MaxLength != nil {
			ps.add(vp+".maxLength", "maxLength "+msg)
		}
		if v.Pattern != "" {
			ps.add(vp+".pattern", "pattern "+msg)
		}
	}
	if fld.Type != FieldNumber {
		if v.Min != nil {
			ps.add(vp+".min", "min is only allowed on number fields")
		}
		if v.Max != nil {
			ps.add(vp+".max", "max is only allowed on number fields")
		}
	}
	if v.MinLength != nil && *v.MinLength < 0 {
		ps.add(vp+".minLength", "must be 0 or greater")
	}
	if v.MaxLength != nil && *v.MaxLength < 0 {
		ps.add(vp+".maxLength", "must be 0 or greater")
	}
	if v.MinLength != nil && v.MaxLength != nil && *v.MinLength > *v.MaxLength {
		ps.add(vp+".maxLength", "must be greater than or equal to minLength")
	}
	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		ps.add(vp+".max", "must be greater than or equal to min")
	}
	if v.Pattern != "" {
		if _, err := regexp.Compile(v.Pattern); err != nil {
			ps.add(vp+".pattern", "is not a valid regular expression: "+err.Error())
		}
	}
}

// validateCondition checks a showIf. seen maps field keys to the index of
// their first declaration; the controller must be declared before index i.
func validateCondition(ps *problems, p string, c Condition, seen map[string]int, i int) {
	if j, ok := seen[c.Field]; !ok || j >= i {
		ps.add(p+".field", fmt.Sprintf("must reference a field declared before this one (got %q)", c.Field))
	}
	n := 0
	if c.Equals != nil {
		n++
	}
	if c.NotEquals != nil {
		n++
	}
	if c.In != nil {
		n++
	}
	switch {
	case n != 1:
		ps.add(p, "exactly one of equals, notEquals or in is required")
	case c.In != nil && len(c.In) == 0:
		ps.add(p+".in", "must not be empty")
	}
}
