package definition

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCheckValue(t *testing.T) {
	opts := []Option{{Value: "go", Label: "Go"}, {Value: "ts", Label: "TypeScript"}}
	cases := []struct {
		name     string
		typ      FieldType
		v        *Validation
		raw      any
		want     any
		provided bool
		msg      string
	}{
		{"nil", FieldText, nil, nil, nil, false, ""},
		{"trim", FieldText, nil, "  hi ", "hi", true, ""},
		{"blank", FieldText, nil, "   ", nil, false, ""},
		{"not string", FieldText, nil, 5.0, nil, false, "must be a string"},
		{"object for text", FieldTextarea, nil, map[string]any{"a": 1.0}, nil, false, "must be a string"},
		{"min length runes", FieldText, &Validation{MinLength: intPtr(3)}, "éé", nil, false, "must be at least 3 characters"},
		{"max length runes", FieldText, &Validation{MaxLength: intPtr(2)}, "👍👍", "👍👍", true, ""},
		{"max length exceeded", FieldText, &Validation{MaxLength: intPtr(2)}, "abc", nil, false, "must be at most 2 characters"},
		{"pattern anchored", FieldText, &Validation{Pattern: "[A-Z]{3}"}, "ABCD", nil, false, "must match the required format"},
		{"pattern ok", FieldText, &Validation{Pattern: "[A-Z]{3}"}, "ABC", "ABC", true, ""},
		{"bad pattern ignored", FieldText, &Validation{Pattern: "("}, "x", "x", true, ""},
		{"email ok", FieldEmail, nil, "ada@example.com", "ada@example.com", true, ""},
		{"email display name", FieldEmail, nil, "Ada <ada@example.com>", nil, false, "must be a valid email address"},
		{"email angle brackets", FieldEmail, nil, "<ada@example.com>", nil, false, "must be a valid email address"},
		{"email no at", FieldEmail, nil, "ada", nil, false, "must be a valid email address"},
		{"url ok", FieldURL, nil, "https://example.com", "https://example.com", true, ""},
		{"url no scheme", FieldURL, nil, "example.com", nil, false, "must be an absolute http(s) URL"},
		{"url ftp", FieldURL, nil, "ftp://example.com", nil, false, "must be an absolute http(s) URL"},
		{"date ok", FieldDate, nil, "2024-02-29", "2024-02-29", true, ""},
		{"date impossible", FieldDate, nil, "2023-02-29", nil, false, "must be a date in YYYY-MM-DD format"},
		{"date short", FieldDate, nil, "2024-2-3", nil, false, "must be a date in YYYY-MM-DD format"},
		{"number float", FieldNumber, nil, 3.5, 3.5, true, ""},
		{"number int normalized", FieldNumber, nil, 42, 42.0, true, ""},
		{"number json.Number", FieldNumber, nil, json.Number("7"), 7.0, true, ""},
		{"number string", FieldNumber, nil, "5", nil, false, "must be a number"},
		{"number bool", FieldNumber, nil, true, nil, false, "must be a number"},
		{"number below min", FieldNumber, &Validation{Min: floatPtr(1)}, 0.5, nil, false, "must be at least 1"},
		{"number above max", FieldNumber, &Validation{Max: floatPtr(10)}, 10.5, nil, false, "must be at most 10"},
		{"number at max", FieldNumber, &Validation{Max: floatPtr(10)}, 10.0, 10.0, true, ""},
		{"checkbox false", FieldCheckbox, nil, false, false, true, ""},
		{"checkbox string", FieldCheckbox, nil, "true", nil, false, "must be true or false"},
		{"select ok", FieldSelect, nil, "go", "go", true, ""},
		{"select trimmed", FieldSelect, nil, " go ", "go", true, ""},
		{"select bad", FieldSelect, nil, "rust", nil, false, "must be one of the options"},
		{"multi ok", FieldMultiselect, nil, []any{"ts", "go"}, []any{"ts", "go"}, true, ""},
		{"multi []string", FieldMultiselect, nil, []string{"go"}, []any{"go"}, true, ""},
		{"multi empty", FieldMultiselect, nil, []any{}, nil, false, ""},
		{"multi dup", FieldMultiselect, nil, []any{"go", "go"}, nil, false, "must not contain duplicates"},
		{"multi bad option", FieldMultiselect, nil, []any{"rust"}, nil, false, "must only contain valid options"},
		{"multi string", FieldMultiselect, nil, "go", nil, false, "must be a list of options"},
		{"multi non-string item", FieldMultiselect, nil, []any{1.0}, nil, false, "must be a list of options"},
		{"unknown type", FieldType("color"), nil, "x", nil, false, "unsupported field type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, provided, msg := checkValue(tc.typ, opts, tc.v, tc.raw)
			if msg != tc.msg {
				t.Fatalf("msg = %q, want %q", msg, tc.msg)
			}
			if provided != tc.provided {
				t.Fatalf("provided = %v, want %v", provided, tc.provided)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("value = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestJSONEqual(t *testing.T) {
	if !jsonEqual(18, 18.0) {
		t.Error("int and float64 18 should be equal")
	}
	if !jsonEqual([]string{"a"}, []any{"a"}) {
		t.Error("[]string and []any should be equal")
	}
	if jsonEqual("18", 18.0) {
		t.Error("string and number must differ")
	}
	if !jsonEqual(map[string]any{"a": []any{1}}, map[string]any{"a": []any{1.0}}) {
		t.Error("nested normalization failed")
	}
}
