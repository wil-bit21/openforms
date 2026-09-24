package definition

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// checkValue validates and normalizes one answer (spec §5.4 + Plan 02 clarifications).
// provided=false with msg=="" means "not provided" (nil, blank string, empty list).
// msg != "" means the value is present but invalid.
func checkValue(t FieldType, options []Option, v *Validation, raw any) (val any, provided bool, msg string) {
	if raw == nil {
		return nil, false, ""
	}
	switch t {
	case FieldText, FieldTextarea, FieldEmail, FieldURL, FieldDate, FieldSelect:
		s, ok := raw.(string)
		if !ok {
			return nil, false, "must be a string"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false, ""
		}
		if m := checkString(t, options, v, s); m != "" {
			return nil, false, m
		}
		return s, true, ""
	case FieldNumber:
		n, ok := toFloat(raw)
		if !ok {
			return nil, false, "must be a number"
		}
		if v != nil && v.Min != nil && n < *v.Min {
			return nil, false, "must be at least " + formatNumber(*v.Min)
		}
		if v != nil && v.Max != nil && n > *v.Max {
			return nil, false, "must be at most " + formatNumber(*v.Max)
		}
		return n, true, ""
	case FieldCheckbox:
		b, ok := raw.(bool)
		if !ok {
			return nil, false, "must be true or false"
		}
		return b, true, ""
	case FieldMultiselect:
		items, ok := toStrings(raw)
		if !ok {
			return nil, false, "must be a list of options"
		}
		if len(items) == 0 {
			return nil, false, ""
		}
		seen := make(map[string]bool, len(items))
		out := make([]any, 0, len(items))
		for _, it := range items {
			if !hasOption(options, it) {
				return nil, false, "must only contain valid options"
			}
			if seen[it] {
				return nil, false, "must not contain duplicates"
			}
			seen[it] = true
			out = append(out, it)
		}
		return out, true, ""
	}
	return nil, false, "unsupported field type"
}

func checkString(t FieldType, options []Option, v *Validation, s string) string {
	switch t {
	case FieldEmail:
		if a, err := mail.ParseAddress(s); err != nil || a.Name != "" || a.Address != s {
			return "must be a valid email address"
		}
	case FieldURL:
		if u, err := url.Parse(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "must be an absolute http(s) URL"
		}
	case FieldDate:
		if d, err := time.Parse("2006-01-02", s); err != nil || d.Format("2006-01-02") != s {
			return "must be a date in YYYY-MM-DD format"
		}
	case FieldSelect:
		if !hasOption(options, s) {
			return "must be one of the options"
		}
	}
	if v == nil {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if v.MinLength != nil && n < *v.MinLength {
		return fmt.Sprintf("must be at least %d characters", *v.MinLength)
	}
	if v.MaxLength != nil && n > *v.MaxLength {
		return fmt.Sprintf("must be at most %d characters", *v.MaxLength)
	}
	if v.Pattern != "" {
		if re := anchored(v.Pattern); re != nil && !re.MatchString(s) {
			return "must match the required format"
		}
	}
	return ""
}

var patternCache sync.Map // pattern string → *regexp.Regexp (nil when uncompilable)

// anchored compiles ^(?:p)$, caching the result. Uncompilable patterns return nil
// and are ignored here; ValidateForm rejects them at definition time.
func anchored(p string) *regexp.Regexp {
	if re, ok := patternCache.Load(p); ok {
		return re.(*regexp.Regexp)
	}
	re, err := regexp.Compile(`^(?:` + p + `)$`)
	if err != nil {
		re = nil
	}
	patternCache.Store(p, re)
	return re
}

func hasOption(options []Option, v string) bool {
	for _, o := range options {
		if o.Value == v {
			return true
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	case int:
		f = float64(n)
	case int32:
		f = float64(n)
	case int64:
		f = float64(n)
	case json.Number:
		x, err := n.Float64()
		if err != nil {
			return 0, false
		}
		f = x
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func toStrings(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, len(s))
		for i, it := range s {
			str, ok := it.(string)
			if !ok {
				return nil, false
			}
			out[i] = str
		}
		return out, true
	}
	return nil, false
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// normalizeJSON maps Go values onto the shapes encoding/json produces, so values
// from definitions, requests and fixtures compare equal.
func normalizeJSON(v any) any {
	if f, ok := toFloat(v); ok {
		return f
	}
	switch x := v.(type) {
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, it := range x {
			out[i] = normalizeJSON(it)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, it := range x {
			out[k] = normalizeJSON(it)
		}
		return out
	}
	return v
}

func jsonEqual(a, b any) bool { return reflect.DeepEqual(normalizeJSON(a), normalizeJSON(b)) }
