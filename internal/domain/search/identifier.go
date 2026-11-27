package search

import (
	"strings"
	"unicode"
)

func ExactIdentifier(r Request) bool {
	return r.Plan.ExactOnly && r.Plan.LimitReason == "" && len(strings.Fields(r.Query)) == 1 && strings.ContainsFunc(r.Query, unicode.IsDigit)
}

func IdentifierAllowed(r Request, d Document) bool {
	return !ExactIdentifier(r) || d.OfferID == r.Query || strings.EqualFold(d.VendorCode, r.Query)
}
