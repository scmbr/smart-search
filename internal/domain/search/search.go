// Package search owns lexical request, document, and family grouping rules.
package search

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
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
	for _, rule := range rules {
		u, err := url.Parse(rule.SourceURL)
		if rule.ID == "" || rule.Name == "" || len(rule.Groups) == 0 || !strings.HasPrefix(rule.VendorPath, "/") || !strings.HasSuffix(rule.VendorPath, "/") || rule.VendorPath == "/" || err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("invalid family rule")
		}
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
	OriginalQuery, Query, Policy, Version, Kind, Platform, ProductType string
	Limit, MinUsers                                                    int
}

func Prepare(r Request) (Request, error) {
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
	c := catalog.Normalize(catalog.Offer{Name: r.Query})
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
		switch strings.ToLower(r.Query) {
		case "сервер", "server":
			r.ProductType = "hardware"
		case "по для сервера", "программное обеспечение для сервера":
			r.ProductType = "software"
		}
	}
	return r, nil
}

type Document struct {
	OfferID, GroupID, FamilyID, FamilyName, FamilySource string
	FamilyConfirmed                                      bool
	Name, Vendor, VendorCode, URL, CategoryID, CatalogID string
	ProductName                                          string
	CategoryNames                                        []string
	Description, ProductType, OfferKind, Version         string
	RoleConflict                                         bool
	Platforms                                            []string
	UsersLimit                                           *int
	PriceMinor                                           *int64
	HasPrice, Available                                  bool
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
	if c.Platforms.Status == catalog.Known {
		d.Platforms = c.Platforms.Values
	}
	if c.UsersLimit.Status == catalog.Known {
		v := c.UsersLimit.Value
		d.UsersLimit = &v
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
	return d, nil
}

type Hit struct {
	Document Document
	Score    float64
}
type Result struct {
	Query, CatalogID, IndexVersion              string
	Hits                                        []Hit
	UnpricedFallbackUsed, FourFamiliesConfirmed bool
	ShortfallReason                             string
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
