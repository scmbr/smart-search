package search

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const InterpretationVersion = "interpretation-v1"

var ErrInvalidInterpretation = errors.New("invalid query interpretation")

// Interpretation is a retrieval hypothesis, never a source of SKU filters.
type Interpretation struct {
	Version, SourceQuery, Status, SemanticQuery string
}

func ValidateInterpretation(source string, x Interpretation) error {
	if x.Version != InterpretationVersion || x.SourceQuery != source || !safeInterpretationText(source) {
		return ErrInvalidInterpretation
	}
	switch x.Status {
	case "resolved":
		if !safeInterpretationText(x.SemanticQuery) || !slices.Equal(numericTerms(source), numericTerms(x.SemanticQuery)) {
			return ErrInvalidInterpretation
		}
	case "clarify", "unsupported":
		if x.SemanticQuery != "" {
			return ErrInvalidInterpretation
		}
	default:
		return ErrInvalidInterpretation
	}
	return nil
}

func safeInterpretationText(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= 256 && strings.TrimSpace(s) != "" && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

func numericTerms(s string) []string {
	var out []string
	for _, term := range queryTokens(s) {
		if strings.ContainsFunc(term, unicode.IsDigit) {
			out = append(out, term)
		}
	}
	return out
}

// VectorQuery leaves the original user text available for filters and diagnostics.
func (r Request) VectorQuery() string {
	if r.SemanticQuery != "" {
		return r.SemanticQuery
	}
	return r.OriginalQuery
}
