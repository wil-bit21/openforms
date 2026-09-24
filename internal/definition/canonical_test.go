package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCanonicalSortsKeysAndCompacts(t *testing.T) {
	in := map[string]any{"b": 1, "a": map[string]any{"d": true, "c": "<x> & y"}}
	got, hash, err := Canonical(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"c":"<x> & y","d":true},"b":1}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	sum := sha256.Sum256([]byte(want))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %s does not match sha256 of canonical bytes", hash)
	}
}

func TestCanonicalStructAndMapAgree(t *testing.T) {
	f := Form{Slug: "contact", Title: "Contact", Fields: []Field{{Key: "name", Type: FieldText, Label: "Name"}}}
	m := map[string]any{
		"title":    "Contact",
		"fields":   []any{map[string]any{"label": "Name", "type": "text", "key": "name"}},
		"slug":     "contact",
		"settings": map[string]any{"public": false},
	}
	b1, h1, err := Canonical(f)
	if err != nil {
		t.Fatal(err)
	}
	b2, h2, err := Canonical(m)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch:\n%s\n%s", b1, b2)
	}
}

func TestCanonicalPreservesNumbers(t *testing.T) {
	got, _, err := Canonical(map[string]any{"n": 0.1, "i": 42, "big": 12345678901234567})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"big":12345678901234567,"i":42,"n":0.1}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCanonicalRejectsUnmarshalable(t *testing.T) {
	if _, _, err := Canonical(map[string]any{"f": func() {}}); err == nil {
		t.Fatal("expected error for a func value")
	}
}
