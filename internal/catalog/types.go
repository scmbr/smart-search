// Package catalog streams and validates YML snapshots without activating them.
package catalog

import (
	"context"
	"errors"
)

var (
	ErrRecordTooLarge   = errors.New("XML record or token exceeds limit")
	ErrDocumentTooLarge = errors.New("decoded XML document exceeds limit")
	ErrInvalidStructure = errors.New("invalid YML structure")
)

type Options struct {
	MaxRecordBytes   int64
	MaxDocumentBytes int64
	MaxIssues        int
}

type Category struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Name     string `json:"name"`
}

type Parameter struct {
	Name     string `json:"name"`
	RawValue string `json:"raw_value"`
	Value    string `json:"value"`
	Unit     string `json:"unit,omitempty"`
	Known    bool   `json:"known"`
}

type Offer struct {
	ID               string              `json:"id"`
	GroupID          string              `json:"group_id,omitempty"`
	Name             string              `json:"name"`
	Vendor           string              `json:"vendor,omitempty"`
	VendorURL        string              `json:"vendor_url,omitempty"`
	VendorCode       string              `json:"vendor_code,omitempty"`
	URL              string              `json:"url"`
	CategoryID       string              `json:"category_id"`
	Available        bool                `json:"available"`
	RawPrice         string              `json:"raw_price"`
	PriceMinor       *int64              `json:"price_minor"`
	RawOldPrice      string              `json:"raw_old_price,omitempty"`
	OldPriceMinor    *int64              `json:"old_price_minor"`
	OriginalCurrency string              `json:"original_currency"`
	Currency         string              `json:"currency"`
	Description      string              `json:"description,omitempty"`
	PictureURLs      []string            `json:"picture_urls,omitempty"`
	Parameters       []Parameter         `json:"parameters"`
	Attributes       map[string]string   `json:"attributes"`
	RawFields        map[string][]string `json:"raw_fields"`
}

// Sink receives provisional records. A later error can invalidate the snapshot;
// implementations must stage writes rather than mutate the active catalog.
type Sink interface {
	WriteCategory(context.Context, Category) error
	WriteOffer(context.Context, Offer) error
}

type Issue struct {
	Code     string `json:"code"`
	EntityID string `json:"entity_id,omitempty"`
	Message  string `json:"message"`
}

type Report struct {
	Format          string  `json:"format"`
	CatalogDate     string  `json:"catalog_date"`
	CategoriesSeen  int     `json:"categories_seen"`
	OffersSeen      int     `json:"offers_seen"`
	Accepted        int     `json:"accepted"`
	Rejected        int     `json:"rejected"`
	AvailableOffers int     `json:"available_accepted_offers"`
	KnownPrices     int     `json:"known_prices_accepted_offers"`
	UnknownPrices   int     `json:"unknown_prices_accepted_offers"`
	DecodedBytes    int64   `json:"decoded_bytes"`
	IssueCount      int     `json:"issue_count"`
	OmittedIssues   int     `json:"omitted_issues"`
	Issues          []Issue `json:"issues"`
	Complete        bool    `json:"complete"`
	CanActivate     bool    `json:"can_activate"`
}

func (r *Report) addIssue(limit int, code, id, message string) {
	r.IssueCount++
	if len(r.Issues) < limit {
		r.Issues = append(r.Issues, Issue{Code: code, EntityID: id, Message: message})
	} else {
		r.OmittedIssues++
	}
}
