package workflow

import (
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

// mergeFields validates patch against the workflow's fields and applies it to
// current. A nil value in patch unsets that field. It returns the merged map
// and the changed subset (removed keys map to nil).
func mergeFields(wf definition.Workflow, current, patch map[string]any) (map[string]any, map[string]any, error) {
	merged := make(map[string]any, len(current)+len(patch))
	for k, v := range current {
		merged[k] = v
	}
	changed := map[string]any{}
	sets := map[string]any{}
	var unset []string
	var problems []definition.Problem
	for k, v := range patch {
		if v != nil {
			sets[k] = v
			continue
		}
		if _, ok := wf.Field(k); !ok {
			problems = append(problems, definition.Problem{Path: "fields." + k, Message: "unknown workflow field"})
			continue
		}
		unset = append(unset, k)
	}
	clean, err := definition.ValidateWorkflowFields(wf, sets)
	if err != nil {
		var ve *definition.ValidationError
		if !errors.As(err, &ve) {
			return nil, nil, err
		}
		problems = append(problems, ve.Problems...)
	}
	if len(problems) > 0 {
		sort.Slice(problems, func(i, j int) bool { return problems[i].Path < problems[j].Path })
		return nil, nil, &definition.ValidationError{Problems: problems}
	}
	for _, k := range unset {
		if _, had := merged[k]; had {
			delete(merged, k)
			changed[k] = nil
		}
	}
	for k, v := range clean {
		if old, had := merged[k]; !had || !reflect.DeepEqual(old, v) {
			merged[k] = v
			changed[k] = v
		}
	}
	return merged, changed, nil
}

// isEmpty reports whether a workflow field value counts as "not provided".
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}
