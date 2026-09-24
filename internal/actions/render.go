// Package actions runs workflow side effects (webhook, email, assign) as jobs.
package actions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// Render replaces {{a.b.c}} placeholders with values looked up in vars.
// Missing paths render as "". Output is plain text (no HTML escaping).
func Render(tmpl string, vars map[string]any) string {
	return placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
		path := placeholder.FindStringSubmatch(m)[1]
		return format(lookup(vars, strings.Split(path, ".")))
	})
}

func lookup(v any, parts []string) any {
	cur := v
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		if cur, ok = m[p]; !ok {
			return nil
		}
	}
	return cur
}

func format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
	case []string:
		return strings.Join(x, ", ")
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = format(e)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}
