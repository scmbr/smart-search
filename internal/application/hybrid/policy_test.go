package hybrid

import (
	"context"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestHybridUnpricedOnlyAfterBothPricedBranchesEmpty(t *testing.T) {
	b := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i"}, unpriced: []d.Hit{h("vector", false)}}
	r, err := (Backend{Lexical: b, Vectors: &fakeVectors{}}).Search(context.Background(), d.Request{Limit: 4})
	if err != nil || len(r.Hits) != 1 || !r.UnpricedFallbackUsed || len(b.priced) != 2 || !b.priced[0] || b.priced[1] {
		t.Fatalf("%+v %v", r, err)
	}
	b = &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{h("lexical", true)}}, unpriced: []d.Hit{h("vector", false)}}
	r, err = (Backend{Lexical: b, Vectors: &fakeVectors{}}).Search(context.Background(), d.Request{Limit: 4})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Document.FamilyID != "lexical" || len(b.priced) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHybridNeverMixesCatalogsOrDuplicateFamilies(t *testing.T) {
	b := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{h("same", true)}}, resolved: []d.Hit{h("same", true)}}
	r, err := (Backend{Lexical: b, Vectors: &fakeVectors{}}).Search(context.Background(), d.Request{Limit: 4})
	if err != nil || len(r.Hits) != 1 || r.FourFamiliesConfirmed {
		t.Fatalf("%+v %v", r, err)
	}
	b.resolved[0].Document.CatalogID = "other"
	if _, err = (Backend{Lexical: b, Vectors: &fakeVectors{}}).Search(context.Background(), d.Request{Limit: 4}); err == nil {
		t.Fatal("mixed catalogs")
	}
	v := &fakeVectors{}
	r, err = (Backend{Lexical: b, Vectors: v, ExpectedCatalog: "other"}).Search(context.Background(), d.Request{Limit: 4})
	if err != nil || v.calls != 0 || r.Hybrid.VectorStatus != "catalog_mismatch" {
		t.Fatalf("%+v %v", r, err)
	}
}
