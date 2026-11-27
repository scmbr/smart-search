// Package queryinterpretation validates untrusted model JSON at the boundary.
package queryinterpretation

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	d "github.com/scmbr/smart-search/internal/domain/search"
)

// Decode deliberately accepts only four distinct, exact, string-valued keys.
func Decode(raw []byte, source string) (d.Interpretation, error) {
	invalid := func() (d.Interpretation, error) { return d.Interpretation{}, d.ErrInvalidInterpretation }
	if len(raw) > 4096 || !utf8.Valid(raw) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return invalid()
	}
	fields := make(map[string]string, 4)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return invalid()
		}
		key, ok := token.(string)
		if !ok {
			return invalid()
		}
		switch key {
		case "version", "source_query", "status", "semantic_query":
		default:
			return invalid()
		}
		if _, exists := fields[key]; exists {
			return invalid()
		}
		token, err = decoder.Token()
		if err != nil {
			return invalid()
		}
		value, ok := token.(string)
		if !ok {
			return invalid()
		}
		fields[key] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || len(fields) != 4 {
		return invalid()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return invalid()
	}
	x := d.Interpretation{Version: fields["version"], SourceQuery: fields["source_query"], Status: fields["status"], SemanticQuery: fields["semantic_query"]}
	if err = d.ValidateInterpretation(source, x); err != nil {
		return invalid()
	}
	return x, nil
}
