package names

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type backendFunc func(context.Context, d.Request) (d.Result, error)

func (f backendFunc) Search(c context.Context, r d.Request) (d.Result, error) { return f(c, r) }
func TestSearchPinsNamedIdentityAndPreservesConditions(t *testing.T) {
	entity := d.NameEntity{Kind: "product", Name: "Adobe Photoshop", Scope: "/adobe/"}
	dictionary, err := d.NewNameDictionary("snapshot", []d.NameEntity{entity}, []d.NameAlias{{Alias: "фотошоп", Name: entity.Name, Kind: "product"}})
	if err != nil {
		t.Fatal(err)
	}
	fallbackCalls := 0
	b := SearchBackend{Dictionary: dictionary, CatalogID: "snapshot", Groups: map[d.NameEntity][]string{entity: {"group"}}, Fallback: backendFunc(func(context.Context, d.Request) (d.Result, error) { fallbackCalls++; return d.Result{}, nil })}
	document := d.Document{CatalogID: "snapshot", GroupID: "group", ProductName: entity.Name, URL: "https://store.softline.ru/adobe/product/"}
	b.Named = backendFunc(func(_ context.Context, r d.Request) (d.Result, error) {
		if r.OriginalQuery != "фотошоп" || r.Query != entity.Name || r.Policy != "available_only" || r.Version != "2025" || !r.Plan.ExactOnly || r.NamedSelection == nil {
			t.Fatalf("conditions changed: %+v", r)
		}
		return d.Result{CatalogID: "snapshot", Hits: []d.Hit{{Document: document}}}, nil
	})
	request := d.Request{Query: "фотошоп", OriginalQuery: "фотошоп", Policy: "available_only", Version: "2025"}
	if _, err = b.Search(context.Background(), request); err != nil || fallbackCalls != 0 {
		t.Fatalf("%v %d", err, fallbackCalls)
	}
	document.ProductName = "Photoshop Plug-ins"
	if _, err = b.Search(context.Background(), request); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("foreign product accepted", err)
	}
	request.Query = "FQC-10572"
	if _, err = b.Search(context.Background(), request); err != nil || fallbackCalls != 1 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = b.Search(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
