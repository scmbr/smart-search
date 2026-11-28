package search

import (
	"net/url"
	"slices"
	"strings"
)

type NameSelection struct {
	CatalogID, DictionarySHA string
	Entity                   NameEntity
	Groups                   []string
}

// SelectName preserves parsed conditions and original input.
func SelectName(r Request, s NameSelection) Request {
	r.NamedSelection = &s
	r.Query = s.Entity.Name
	r.SemanticQuery = ""
	r.Plan = queryPlan(r.Query, r.Kind)
	r.Plan.ExactOnly = true
	r.Plan.Variants = nil
	return r
}
func NameAllowed(s *NameSelection, d Document) bool {
	if s == nil {
		return true
	}
	if d.CatalogID != s.CatalogID {
		return false
	}
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" || !strings.HasPrefix(u.Path, s.Entity.Scope) {
		return false
	}
	if s.Entity.Kind == "brand" {
		return nameNormalized(d.Vendor) == nameNormalized(s.Entity.Name)
	}
	return s.Entity.Kind == "product" && slices.Contains(s.Groups, d.GroupID) && nameNormalized(d.ProductName) == nameNormalized(s.Entity.Name)
}
