package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// keyOrder fixes the order of mapping keys in pulled files so output is
// byte-stable and reads naturally. Keys not listed sort after these, alphabetically.
var keyOrder = []string{
	"slug", "key", "value", "type", "title", "label", "description", "workflow", "initial",
	"settings", "public", "submitLabel", "confirmationMessage", "required", "placeholder", "help",
	"options", "validation", "minLength", "maxLength", "pattern", "min", "max",
	"showIf", "field", "equals", "notEquals", "in",
	"from", "to", "guard", "roles", "requireFields", "actions",
	"url", "subject", "body", "user", "role", "color", "terminal",
	"states", "fields", "onSubmit", "transitions",
}

var keyRank = func() map[string]int {
	m := make(map[string]int, len(keyOrder))
	for i, k := range keyOrder {
		m[k] = i
	}
	return m
}()

// CanonicalYAML renders v (via its JSON form) as deterministic YAML.
func CanonicalYAML(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	node, err := toNode(tree)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range keys {
			kn, err := scalar(k)
			if err != nil {
				return nil, err
			}
			vn, err := toNode(t[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, kn, vn)
		}
		return n, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range t {
			c, err := toNode(item)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, c)
		}
		return n, nil
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return scalar(i)
		}
		f, err := t.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", t, err)
		}
		return scalar(f)
	default: // string, bool, nil
		return scalar(t)
	}
}

// scalar lets yaml.v3 pick tag and quoting, so "yes" or "123" stay strings.
func scalar(v any) (*yaml.Node, error) {
	n := &yaml.Node{}
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return n, nil
}

func lessKey(a, b string) bool {
	ra, oka := keyRank[a]
	rb, okb := keyRank[b]
	switch {
	case oka && okb:
		return ra < rb
	case oka:
		return true
	case okb:
		return false
	default:
		return a < b
	}
}
