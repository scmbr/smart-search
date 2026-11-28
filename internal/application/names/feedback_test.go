package names

import (
	"context"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestAmbiguousNameDoesNotChooseOrCallPaidFallback(t *testing.T) {
	dictionary, err := d.NewNameDictionary("snapshot", []d.NameEntity{{Kind: "product", Name: "paint", Scope: "/a/"}, {Kind: "product", Name: "point", Scope: "/b/"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := SearchBackend{Dictionary: dictionary, CatalogID: "snapshot", Named: backendFunc(func(context.Context, d.Request) (d.Result, error) {
		return d.Result{CatalogID: "snapshot", IndexVersion: "index", Hits: []d.Hit{{Document: d.Document{OfferID: "arbitrary"}}}}, nil
	}), Fallback: backendFunc(func(context.Context, d.Request) (d.Result, error) {
		t.Fatal("ambiguous query reached fallback")
		return d.Result{}, nil
	})}
	r, err := b.Search(context.Background(), d.Request{Query: "peint", OriginalQuery: "peint"})
	if err != nil || len(r.Hits) != 0 || r.NameResolution == nil || r.NameResolution.Status != "ambiguous" || len(r.NameResolution.Candidates) != 2 {
		t.Fatalf("%+v %v", r, err)
	}
}
