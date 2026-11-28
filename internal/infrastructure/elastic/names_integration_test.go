package elastic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	a "github.com/scmbr/smart-search/internal/application/lexical"
	n "github.com/scmbr/smart-search/internal/application/names"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"os"
	"testing"
)

func TestNamedProductScopeBeatsForeignPricedTypo(t *testing.T) {
	endpoint := os.Getenv("TEST_ELASTICSEARCH_URL")
	if endpoint == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(nonce[:])
	client, err := New(endpoint, "", "test-names-"+id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	spec := a.NewSpec(a.CatalogInfo{ID: "names-" + id, Offers: 4}, "families")
	t.Cleanup(func() {
		m, exists, err := client.meta(context.Background(), spec.Name)
		if err != nil {
			t.Error(err)
			return
		}
		if exists && m.Spec == spec {
			if _, _, err = client.request(context.Background(), "DELETE", "/"+spec.Name, nil); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err = client.Prepare(ctx, spec); err != nil {
		t.Fatal(err)
	}
	price := int64(100)
	docs := []d.Document{
		{OfferID: "target", GroupID: "editor", ProductName: "Adobe Photoshop", Vendor: "Adobe Systems", URL: "https://store.softline.ru/adobe/target/", Version: "2025"},
		{OfferID: "similar", GroupID: "slideshow", ProductName: "ФотоШОУ PRO", Vendor: "Other", URL: "https://store.softline.ru/other/similar/", PriceMinor: &price, HasPrice: true, Available: true},
		{OfferID: "plugin", GroupID: "plugin", ProductName: "Photoshop Plug-ins", Vendor: "Adobe Systems", URL: "https://store.softline.ru/adobe/plugin/", PriceMinor: &price, HasPrice: true, Available: true},
		{OfferID: "collision", GroupID: "editor", ProductName: "Adobe Photoshop", Vendor: "Other", URL: "https://store.softline.ru/other/collision/", PriceMinor: &price, HasPrice: true, Available: true},
	}
	for i := range docs {
		doc := &docs[i]
		doc.Name = doc.ProductName
		doc.CatalogID = spec.CatalogID
		doc.FamilyID = doc.OfferID
		doc.OfferKind = "new_license"
		doc.ProductType = "software"
		d.SetProductRole(doc)
	}
	if err = client.Write(ctx, spec.Name, docs); err != nil {
		t.Fatal(err)
	}
	if err = client.Finish(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err = client.Activate(ctx, spec); err != nil {
		t.Fatal(err)
	}
	entity := d.NameEntity{Kind: "product", Name: "Adobe Photoshop", Scope: "/adobe/"}
	dictionary, err := d.NewNameDictionary(spec.CatalogID, []d.NameEntity{entity}, []d.NameAlias{{Alias: "фотошоп", Name: entity.Name, Kind: "product"}})
	if err != nil {
		t.Fatal(err)
	}
	backend := n.SearchBackend{CatalogID: spec.CatalogID, Dictionary: dictionary, Groups: map[d.NameEntity][]string{entity: {"editor"}}, Named: client, Fallback: client}
	for _, query := range []string{"фотошоп", "фотошор"} {
		r, err := a.Search(ctx, backend, d.Request{Query: query, Limit: 4})
		if err != nil || len(r.Hits) != 1 || r.Hits[0].Document.OfferID != "target" || !r.UnpricedFallbackUsed {
			t.Fatalf("%s %+v %v", query, r, err)
		}
	}
	for _, request := range []d.Request{{Query: "фотошоп", Limit: 4, Policy: "available_only"}, {Query: "фотошоп", Limit: 4, Version: "2024"}} {
		r, err := a.Search(ctx, backend, request)
		if err != nil || len(r.Hits) != 0 {
			t.Fatalf("constraint relaxed: %+v %v", r, err)
		}
	}
}
