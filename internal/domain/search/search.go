// Package search owns lexical request, document, and family grouping rules.
package search

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
)

var ErrInvalidQuery = errors.New("invalid search query")
var ErrUnavailable = errors.New("search backend unavailable")
var ErrNotReady = errors.New("search index not ready")

type FamilyRule struct {
	ID, Name, VendorPath, SourceURL string
	Groups                          []string
}
type Family struct {
	ID, Name, Source string
	Confirmed        bool
}
type Families struct{ groups map[string]FamilyRule }

func NewFamilies(rules []FamilyRule) (*Families, error) {
	r := &Families{groups: make(map[string]FamilyRule)}
	identities := make(map[string]FamilyRule)
	for _, rule := range rules {
		u, err := url.Parse(rule.SourceURL)
		if rule.ID == "" || rule.Name == "" || len(rule.Groups) == 0 || !strings.HasPrefix(rule.VendorPath, "/") || !strings.HasSuffix(rule.VendorPath, "/") || rule.VendorPath == "/" || err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("invalid family rule")
		}
		if previous, exists := identities[rule.ID]; exists && (previous.Name != rule.Name || previous.VendorPath != rule.VendorPath || previous.SourceURL != rule.SourceURL) {
			return nil, errors.New("conflicting family identity")
		}
		identities[rule.ID] = rule
		for _, group := range rule.Groups {
			if _, exists := r.groups[group]; exists || group == "" {
				return nil, errors.New("duplicate/empty family group")
			}
			r.groups[group] = rule
		}
	}
	return r, nil
}

func (r *Families) Resolve(o catalog.Offer) Family {
	u, _ := url.Parse(o.URL)
	path := ""
	vendor := ""
	if u != nil {
		path = u.Path
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) > 0 {
			vendor = u.Host + "/" + parts[0]
		}
	}
	if rule, ok := r.groups[o.GroupID]; ok && u != nil && u.Hostname() == "store.softline.ru" && strings.HasPrefix(path, rule.VendorPath) {
		return Family{ID: rule.ID, Name: rule.Name, Source: rule.SourceURL, Confirmed: true}
	}
	key := vendor + "/group/" + o.GroupID
	if o.GroupID == "" {
		key = vendor + "/offer/" + o.ID
	}
	h := sha256.Sum256([]byte(key))
	return Family{ID: "provisional-" + hex.EncodeToString(h[:]), Name: o.Name}
}

type Request struct {
	NamedSelection                                                     *NameSelection
	NamedLibrary                                                       bool
	RequestedProductRole                                               string
	SemanticQuery                                                      string
	FamilyID                                                           string
	ProductIntents                                                     []ProductIntent
	OriginalQuery, Query, Policy, Version, Kind, Platform, ProductType string
	Limit, MinUsers                                                    int
	Plan                                                               QueryPlan
	Edition, TermKind, Deployment                                      string
	TermMonths                                                         int
	MinDevices, MinIPAddresses, MinParticipants                        int
	Constraints, UnsupportedConstraints                                []QueryConstraint
}

var queryCSPVersion = regexp.MustCompile(`(?:^|\s)(?:csp|цсп)\s*(?:v)?([0-9]{1,4}(?:\.[0-9]{1,4}){1,3})\b`)
var queryDecimal = regexp.MustCompile(`(?:^|\s)v?[0-9]{1,4}(?:\.[0-9]{1,4}){1,3}(?:$|\s)`)

func Prepare(r Request) (Request, error) {
	r.SemanticQuery = "" // Only the validated interpreter may set a retrieval hypothesis.
	if len(r.FamilyID) > 128 || (r.FamilyID != "" && !familyIdentifier.MatchString(r.FamilyID)) {
		return r, ErrInvalidQuery
	}
	if !utf8.ValidString(r.Query) || utf8.RuneCountInString(r.Query) > 256 || strings.TrimSpace(r.Query) == "" || r.Limit < 4 || r.Limit > 20 || r.MinUsers < 0 || r.MinUsers > 1000000 || utf8.RuneCountInString(r.Version) > 80 || utf8.RuneCountInString(r.Platform) > 80 {
		return r, ErrInvalidQuery
	}
	r.OriginalQuery = r.Query
	r.Query = strings.Join(strings.Fields(r.Query), " ")
	if r.Policy == "" {
		r.Policy = "priced_first"
	}
	if r.Policy != "priced_first" && r.Policy != "available_only" {
		return r, ErrInvalidQuery
	}
	switch r.Kind {
	case "", "new_license", "renewal", "upgrade", "support", "unknown":
	default:
		return r, ErrInvalidQuery
	}
	switch r.ProductType {
	case "", "hardware", "software":
	default:
		return r, ErrInvalidQuery
	}
	r.Platform = strings.ToLower(strings.TrimSpace(r.Platform))
	if err := parseConstraints(&r); err != nil {
		return r, err
	}
	canonical := canonicalQuery(r.Query)
	c := catalog.Normalize(catalog.Offer{Name: canonical})
	if c.Version.Status == catalog.Conflict || c.OfferKind.Status == catalog.Conflict {
		return r, ErrInvalidQuery
	}
	for _, match := range queryCSPVersion.FindAllStringSubmatch(canonical, -1) {
		if r.Version != "" && r.Version != match[1] {
			return r, fmt.Errorf("%w: conflicting versions", ErrInvalidQuery)
		}
		r.Version = match[1]
	}
	if c.Version.Status == catalog.Known {
		if r.Version != "" && r.Version != c.Version.Value {
			return r, fmt.Errorf("%w: conflicting versions", ErrInvalidQuery)
		}
		r.Version = c.Version.Value
	}
	if c.OfferKind.Status == catalog.Known {
		if r.Kind != "" && r.Kind != c.OfferKind.Value {
			return r, fmt.Errorf("%w: conflicting roles", ErrInvalidQuery)
		}
		r.Kind = c.OfferKind.Value
	}
	if r.ProductType == "" {
		switch strings.ToLower(strings.Join(strings.Fields(r.OriginalQuery), " ")) {
		case "сервер", "server":
			r.ProductType = "hardware"
		case "по для сервера", "программное обеспечение для сервера":
			r.ProductType = "software"
		}
		if head := canonicalQuery(r.OriginalQuery); head == "операционная система" || strings.HasPrefix(head, "операционная система ") {
			r.ProductType = "software"
		}
	}
	r.Plan = queryPlan(r.Query, r.Kind)
	if r.Version == "" && r.Plan.LimitReason == "" && queryDecimal.MatchString(canonical) {
		r.Plan.ExactOnly = true
		r.Plan.Variants = nil
		r.Plan.LimitReason = "unparsed_numeric_constraint"
	}
	if err := prepareProductRole(&r); err != nil {
		return r, err
	}
	if r.RequestedProductRole == "addon" && (r.MinUsers > 0 || r.MinDevices > 0 || r.MinIPAddresses > 0 || r.MinParticipants > 0) {
		r.UnsupportedConstraints = append(r.UnsupportedConstraints, QueryConstraint{Field: "additional_capacity_quantity", Text: r.OriginalQuery, Source: "query"})
	}
	r.Plan.ProductRoleVersion = ProductRolePolicyVersion
	r.Plan.RequestedProductRole = r.RequestedProductRole
	r.Plan.ExcludedProductRoles = ExcludedProductRoles(r)
	r.Plan.Purpose = requestedPurpose(r)
	return r, nil
}

type Document struct {
	ProductRole, ProductRoleRule, ProductRoleSource, ProductRoleText string
	OfferID, GroupID, FamilyID, FamilyName, FamilySource             string
	FamilyConfirmed                                                  bool
	Name, Vendor, VendorCode, URL, CategoryID, CatalogID             string
	ProductName                                                      string
	CategoryNames                                                    []string
	Description, ProductType, OfferKind, Version                     string
	RoleConflict                                                     bool
	Platforms                                                        []string
	UsersLimit                                                       *int
	DevicesLimit, IPAddressesLimit, ParticipantsLimit                *int
	PriceMinor                                                       *int64
	HasPrice, Available                                              bool
	Edition, TermKind, Deployment                                    string
	TermMonths                                                       *int
}

func prefix(s string, n int) string {
	count := 0
	for index := range s {
		if count == n {
			return s[:index]
		}
		count++
	}
	return s
}

func BuildDocument(o catalog.Offer, categories map[string]catalog.Category, families *Families, snapshot string) (Document, error) {
	if o.Characteristics == nil || o.Characteristics.PolicyVersion != catalog.NormalizationVersion || len(o.ID) > 512 || len(o.URL) > 8192 || (o.PriceMinor != nil && *o.PriceMinor < 1) {
		return Document{}, errors.New("unsupported offer characteristics or index field size")
	}
	f := families.Resolve(o)
	c := o.Characteristics
	d := Document{OfferID: o.ID, GroupID: o.GroupID, FamilyID: f.ID, FamilyName: prefix(f.Name, 512), FamilySource: f.Source, FamilyConfirmed: f.Confirmed,
		Name: prefix(o.Name, 512), Vendor: prefix(o.Vendor, 128), VendorCode: prefix(o.VendorCode, 256), URL: o.URL, CategoryID: o.CategoryID, CatalogID: snapshot,
		Description: prefix(o.Description, 2048), OfferKind: "unknown", RoleConflict: c.OfferKind.Status == catalog.Conflict,
		PriceMinor: o.PriceMinor, HasPrice: o.PriceMinor != nil, Available: o.Available}
	if c.ProductType.Status == catalog.Known {
		d.ProductType = c.ProductType.Value
	}
	if c.OfferKind.Status == catalog.Known {
		d.OfferKind = c.OfferKind.Value
	}
	if c.Version.Status == catalog.Known {
		d.Version = c.Version.Value
	}
	if c.Edition.Status == catalog.Known {
		d.Edition = c.Edition.Value
	}
	if c.TermKind.Status == catalog.Known {
		d.TermKind = c.TermKind.Value
	}
	if c.TermMonths.Status == catalog.Known {
		n := c.TermMonths.Value
		d.TermMonths = &n
	}
	if c.Deployment.Status == catalog.Known {
		d.Deployment = c.Deployment.Value
	}
	if c.Platforms.Status == catalog.Known {
		d.Platforms = c.Platforms.Values
	}
	if c.UsersLimit.Status == catalog.Known {
		v := c.UsersLimit.Value
		d.UsersLimit = &v
	}
	for _, pair := range []struct {
		source catalog.IntegerAttribute
		target **int
	}{
		{c.DevicesLimit, &d.DevicesLimit}, {c.IPAddressesLimit, &d.IPAddressesLimit}, {c.ParticipantsLimit, &d.ParticipantsLimit},
	} {
		if pair.source.Status == catalog.Known {
			n := pair.source.Value
			*pair.target = &n
		}
	}
	if product, ok := categories[o.GroupID]; ok {
		d.ProductName = prefix(product.Name, 512)
	}
	id := o.CategoryID
	seen := map[string]bool{}
	for id != "" {
		category, ok := categories[id]
		if !ok || seen[id] {
			return Document{}, errors.New("missing/cyclic document category")
		}
		seen[id] = true
		d.CategoryNames = append(d.CategoryNames, category.Name)
		id = category.ParentID
	}
	SetProductRole(&d)
	return d, nil
}

type Hit struct {
	Document Document
	Score    float64
}
type Result struct {
	Hybrid                                      *HybridTrace
	ProductIntents                              []ProductIntent
	Query, CatalogID, IndexVersion              string
	Hits                                        []Hit
	UnpricedFallbackUsed, FourFamiliesConfirmed bool
	ShortfallReason                             string
	QueryPlan                                   QueryPlan
	SearchMode                                  string
	SearchAttempts                              []SearchAttempt
	AppliedConstraints, UnsupportedConstraints  []QueryConstraint
}

type SearchAttempt struct {
	Mode             string
	Priced           bool
	FamiliesReturned int
}

func Describe(result *Result) {
	result.ShortfallReason = ""
	result.FourFamiliesConfirmed = len(result.Hits) >= 4
	for i, h := range result.Hits {
		if i < 4 && !h.Document.FamilyConfirmed {
			result.FourFamiliesConfirmed = false
		}
	}
	if !result.FourFamiliesConfirmed {
		result.ShortfallReason = "insufficient_confirmed_families"
	}
}
