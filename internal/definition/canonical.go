package definition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Canonical returns v as canonical JSON (object keys sorted, no insignificant
// whitespace, HTML characters unescaped) and the lowercase hex sha256 of it.
// Two definitions that differ only in key order or formatting hash equally.
func Canonical(v any) ([]byte, string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", fmt.Errorf("canonical: marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep numbers exactly as marshalled
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, "", fmt.Errorf("canonical: decode: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(generic); err != nil { // maps encode with sorted keys
		return nil, "", fmt.Errorf("canonical: encode: %w", err)
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}
