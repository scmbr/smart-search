package elastic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	a "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

func TestProductRolesFilterBeforeCollapseInEveryBranch(t *testing.T) {
	endpoint := os.Getenv("TEST_ELASTICSEARCH_URL")
	if endpoint == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(nonce[:])
	ctx := context.Background()
	client, err := New(endpoint, "", "test-roles-"+id)
	if err != nil {
		t.Fatal(err)
	}
	spec := a.NewSpec(a.CatalogInfo{ID: "roles-" + id, Offers: 9}, "families")
	t.Cleanup(func() {
		meta, exists, err := client.meta(context.Background(), spec.Name)
		if err != nil {
			t.Error(err)
			return
		}
		if !exists {
			return
		}
		if meta.Spec != spec {
			t.Error("refusing foreign cleanup")
			return
		}
		if _, _, err = client.request(context.Background(), "DELETE", "/"+spec.Name, nil); err != nil {
			t.Error(err)
		}
	})
	if _, err = client.Prepare(ctx, spec); err != nil {
		t.Fatal(err)
	}
	price := int64(100)
	docs := []d.Document{
		{OfferID: "base", FamilyID: "same", GroupID: "g", Name: "Diagram приложение", HasPrice: true, PriceMinor: &price},
		{OfferID: "addon", FamilyID: "same", GroupID: "g", Name: "Diagram Diagram Diagram, дополнительные компоненты", HasPrice: true, PriceMinor: &price},
		{OfferID: "SDK-1234", FamilyID: "library", GroupID: "lib", Name: "Diagram SDK библиотека", HasPrice: true, PriceMinor: &price},
		{OfferID: "media", FamilyID: "media", GroupID: "media", Name: "Diagram медиа-комплект", HasPrice: true, PriceMinor: &price},
		{OfferID: "unpriced", FamilyID: "unpriced", GroupID: "unpriced", Name: "Unique дистрибутив"},
		{OfferID: "capacity-base", FamilyID: "capacity", GroupID: "capacity", Name: "VulnScanner основная лицензия на 4 IP", HasPrice: true, PriceMinor: &price},
		{OfferID: "IP-1234", FamilyID: "capacity", GroupID: "capacity", Name: "VulnScanner VulnScanner на дополнительный IP к лицензии на 4 IP", HasPrice: true, PriceMinor: &price},
		{OfferID: "capacity-unpriced", FamilyID: "capacity-unpriced", GroupID: "capacity-unpriced", Name: "CapacityOnly на дополнительный узел"},
		{OfferID: "migration-1234", FamilyID: "capacity", GroupID: "capacity", Name: "VulnScanner VulnScanner лицензия перехода с OldScanner", HasPrice: true, PriceMinor: &price},
	}
	for i := range docs {
		docs[i].CatalogID = spec.CatalogID
		docs[i].URL = "https://store.softline.ru/vendor/" + docs[i].OfferID + "/"
		d.SetProductRole(&docs[i])
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
	for _, tc := range []struct {
		query, offer string
		fallback     bool
	}{
		{"Diagram", "base", false}, {"Diaram", "base", false},
		{"Diagram библиотека", "SDK-1234", false}, {"Diagram дополнительные компоненты", "addon", false},
		{"Diagram медиа-комплект", "media", false}, {"SDK-1234", "SDK-1234", false},
		{"Unique", "", false}, {"Unique дистрибутив", "unpriced", true},
		{"VulnScanner", "capacity-base", false}, {"VulnScanner дополнительный IP", "IP-1234", false},
		{"IP-1234", "IP-1234", false},
		{"migration-1234", "migration-1234", false},
		{"CapacityOnly", "", false}, {"CapacityOnly дополнительный узел", "capacity-unpriced", true},
	} {
		result, err := a.Search(ctx, client, d.Request{Query: tc.query, Limit: 4})
		if err != nil {
			t.Fatal(tc.query, err)
		}
		if tc.offer == "" {
			if len(result.Hits) != 0 {
				t.Fatal("unpriced bypass", result.Hits)
			}
			continue
		}
		if len(result.Hits) != 1 || result.Hits[0].Document.OfferID != tc.offer || result.UnpricedFallbackUsed != tc.fallback {
			t.Fatalf("%s: %+v", tc.query, result)
		}
	}
	r, _ := d.Prepare(d.Request{Query: "неименованная задача", Limit: 4})
	hits, err := client.Resolve(ctx, r, []d.GroupCandidate{{GroupID: "lib", Vendor: "vendor", Score: .9}, {GroupID: "g", Vendor: "vendor", Score: .8}}, true, spec.CatalogID, spec.Fingerprint)
	if err != nil || len(hits) != 1 || hits[0].Document.OfferID != "base" {
		t.Fatalf("resolve bypass: %+v %v", hits, err)
	}
	hits, err = client.Resolve(ctx, r, []d.GroupCandidate{{GroupID: "capacity", Vendor: "vendor", Score: .9}}, true, spec.CatalogID, spec.Fingerprint)
	if err != nil || len(hits) != 1 || hits[0].Document.OfferID != "capacity-base" {
		t.Fatalf("capacity collapse bypass: %+v %v", hits, err)
	}
}
