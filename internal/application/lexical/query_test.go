package lexical

import (
	"context"
	domain "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type queryBackend struct {
	result  domain.Result
	request domain.Request
}

func (b *queryBackend) Search(_ context.Context, r domain.Request) (domain.Result, error) {
	b.request = r
	return b.result, nil
}
func TestSearchRechecksOfferConstraints(t *testing.T) {
	price := int64(100)
	d := domain.Document{OfferID: "a", FamilyID: "family", CatalogID: "catalog", OfferKind: "new_license", Version: "5.0", PriceMinor: &price}
	for _, kind := range []string{"valid", "wrong version", "support", "duplicate family", "mixed catalog", "unpriced"} {
		t.Run(kind, func(t *testing.T) {
			doc := d
			b := &queryBackend{result: domain.Result{CatalogID: "catalog", IndexVersion: "index"}}
			switch kind {
			case "wrong version":
				doc.Version = "4.0"
			case "support":
				doc.OfferKind = "support"
			case "mixed catalog":
				doc.CatalogID = "other"
			case "unpriced":
				doc.PriceMinor = nil
			}
			b.result.Hits = []domain.Hit{{Document: doc}}
			if kind == "duplicate family" {
				second := doc
				second.OfferID = "b"
				b.result.Hits = append(b.result.Hits, domain.Hit{Document: second})
			}
			_, err := Search(context.Background(), b, domain.Request{Query: "КриптоПро CSP 5.0", Limit: 4})
			if (err == nil) != (kind == "valid") {
				t.Fatalf("kind=%s err=%v", kind, err)
			}
		})
	}
}
