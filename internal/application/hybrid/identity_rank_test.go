package hybrid

import (
	"context"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestHybridKeepsLexicalProductIdentityAndSKU(t *testing.T) {
	r, err := d.Prepare(d.Request{Query: "Vector Studio", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	a, b := h("software", true), h("hardware", true)
	a.Document.ProductName = "Vector Studio"
	b.Document.ProductName = "Sheet scanner"
	backend := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{a, b}}, resolved: []d.Hit{b}}
	got, err := (Backend{Lexical: backend, Vectors: &fakeVectors{}}).Search(context.Background(), r)
	if err != nil || got.Hybrid.IdentityFamilies != 1 || got.Hits[0].Document.OfferID != a.Document.OfferID || got.Hybrid.Version != d.HybridVersion {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPricedVectorStillReplacesUnpricedIdentity(t *testing.T) {
	r, _ := d.Prepare(d.Request{Query: "Vector Studio", Limit: 4})
	a, b := h("software", false), h("hardware", true)
	a.Document.ProductName = "Vector Studio"
	backend := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{a}, UnpricedFallbackUsed: true}, resolved: []d.Hit{b}}
	got, err := (Backend{Lexical: backend, Vectors: &fakeVectors{}}).Search(context.Background(), r)
	if err != nil || len(got.Hits) != 1 || got.Hits[0].Document.OfferID != b.Document.OfferID || got.Hybrid.IdentityFamilies != 0 || got.UnpricedFallbackUsed {
		t.Fatalf("%+v %v", got, err)
	}
}
