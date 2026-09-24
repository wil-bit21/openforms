package definition

import "sort"

// ValidateWorkflowFields validates a patch of workflow-managed field values
// against w.Fields. The result has the same keys as fields: a normalized value,
// or nil when the patch sets the field to null or a blank string ("unset").
// Callers merge it into a submission's fields: nil → delete the key,
// otherwise set it. Problems use paths "fields.<key>", sorted by key.
func ValidateWorkflowFields(w Workflow, fields map[string]any) (map[string]any, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(fields))
	var ps problems
	for _, k := range keys {
		path := "fields." + k
		wf, ok := w.Field(k)
		if !ok {
			ps.add(path, "unknown workflow field")
			continue
		}
		val, provided, msg := checkValue(wf.Type, wf.Options, nil, fields[k])
		switch {
		case msg != "":
			ps.add(path, msg)
		case !provided:
			out[k] = nil
		default:
			out[k] = val
		}
	}
	if err := ps.err(); err != nil {
		return nil, err
	}
	return out, nil
}
