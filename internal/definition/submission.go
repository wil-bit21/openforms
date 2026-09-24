package definition

// Visible reports, for every field key, whether the field is shown for data
// (spec §5.4). It shares one evaluator with ValidateSubmission so the two can
// never disagree.
func Visible(f Form, data map[string]any) map[string]bool {
	visible, _, _ := evaluate(f, data)
	return visible
}

// ValidateSubmission cleans data (drops unknown keys, hidden fields and
// not-provided values; trims strings; normalizes numbers and lists) and
// validates every visible field. It returns the cleaned data or a
// *ValidationError whose problems use paths "data.<key>" in field order.
func ValidateSubmission(f Form, data map[string]any) (map[string]any, error) {
	_, clean, ps := evaluate(f, data)
	if err := ps.err(); err != nil {
		return nil, err
	}
	return clean, nil
}

// evaluate walks the fields in declaration order. A field's condition is
// evaluated against the cleaned values of earlier fields; a hidden controller
// hides its dependants; an invalid value counts as absent.
func evaluate(f Form, data map[string]any) (map[string]bool, map[string]any, problems) {
	visible := make(map[string]bool, len(f.Fields))
	clean := make(map[string]any, len(f.Fields))
	types := make(map[string]FieldType, len(f.Fields))
	var ps problems
	for _, fld := range f.Fields {
		types[fld.Key] = fld.Type
		vis := true
		if c := fld.ShowIf; c != nil {
			if !visible[c.Field] {
				vis = false
			} else {
				val, present := clean[c.Field]
				vis = holds(*c, types[c.Field], val, present)
			}
		}
		visible[fld.Key] = vis
		if !vis {
			continue
		}
		path := "data." + fld.Key
		val, provided, msg := checkValue(fld.Type, fld.Options, fld.Validation, data[fld.Key])
		switch {
		case msg != "":
			ps.add(path, msg)
		case !provided:
			if fld.Required {
				ps.add(path, "is required")
			}
		case fld.Type == FieldCheckbox && fld.Required && val == false:
			ps.add(path, "must be checked")
		default:
			clean[fld.Key] = val
		}
	}
	return visible, clean, ps
}

// holds evaluates a condition against the controller's cleaned value.
func holds(c Condition, ctrl FieldType, val any, present bool) bool {
	multi := ctrl == FieldMultiselect
	switch {
	case c.Equals != nil:
		if !present {
			return false
		}
		if multi {
			return containsJSON(val.([]any), c.Equals)
		}
		return jsonEqual(val, c.Equals)
	case c.NotEquals != nil:
		if !present {
			return true
		}
		if multi {
			return !containsJSON(val.([]any), c.NotEquals)
		}
		return !jsonEqual(val, c.NotEquals)
	case c.In != nil:
		if !present {
			return false
		}
		if multi {
			for _, item := range val.([]any) {
				if containsJSON(c.In, item) {
					return true
				}
			}
			return false
		}
		return containsJSON(c.In, val)
	}
	return true
}

func containsJSON(list []any, v any) bool {
	for _, it := range list {
		if jsonEqual(it, v) {
			return true
		}
	}
	return false
}
