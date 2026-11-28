package elastic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	a "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"os"
	"testing"
)

func TestWindowsProductIntentFiltersBeforeCollapse(t *testing.T) {
	endpoint := os.Getenv("TEST_ELASTICSEARCH_URL")
	if endpoint == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(nonce[:])
	client, err := New(endpoint, "", "test-windows-"+id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	spec := a.NewSpec(a.CatalogInfo{ID: "windows-" + id, Offers: 6}, "families")
	t.Cleanup(func() {
		m, exists, err := client.meta(context.Background(), spec.Name)
		if err != nil {
			t.Error(err)
			return
		}
		if !exists {
			return
		}
		if m.Spec != spec {
			t.Error("foreign index cleanup refused")
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
		{OfferID: "desktop", VendorCode: "WIN-001", FamilyID: "windows-desktop", GroupID: "desktop", Name: "Microsoft Windows 11 Pro", ProductName: "Microsoft Windows 11 Pro", CategoryNames: []string{"Windows 11", "Операционные системы"}, ProductType: "software", Version: "11"},
		{OfferID: "server", FamilyID: "windows-server", GroupID: "server", Name: "Microsoft Windows Server 2022", ProductName: "Microsoft Windows Server 2022", CategoryNames: []string{"Операционные системы"}, ProductType: "software", Version: "2022"},
		{OfferID: "player", FamilyID: "player", GroupID: "player", Name: "Плеер для Windows", ProductName: "Плеер для Windows", CategoryNames: []string{"Мультимедиа"}, ProductType: "software"},
		{OfferID: "headset", FamilyID: "headset", GroupID: "headset", Name: "Headset", Description: "Microsoft Windows Windows Windows", CategoryNames: []string{"Аксессуары"}, ProductType: "hardware"},
		{OfferID: "other-os", FamilyID: "other-os", GroupID: "other-os", Name: "Other OS", ProductName: "Other OS", Description: "Microsoft Windows compatibility", CategoryNames: []string{"Операционные системы"}, ProductType: "software"},
		{OfferID: "old-server", FamilyID: "windows-server", GroupID: "old-server", Name: "Microsoft Windows Server 2019", ProductName: "Microsoft Windows Server 2019", CategoryNames: []string{"Операционные системы"}, ProductType: "software", Version: "2019"},
	}
	for i := range docs {
		doc := &docs[i]
		doc.CatalogID = spec.CatalogID
		doc.URL = "https://store.softline.ru/vendor/" + doc.OfferID + "/"
		doc.OfferKind = "new_license"
		if doc.OfferID != "old-server" {
			doc.PriceMinor = &price
			doc.HasPrice = true
			doc.Available = true
		}
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
	for _, query := range []string{"виндовс", "виндоус", "Windows", "Microsoft Windows"} {
		result, err := a.Search(ctx, client, d.Request{Query: query, Limit: 4})
		if err != nil || len(result.Hits) != 2 {
			t.Fatalf("%s: %+v %v", query, result, err)
		}
		for _, hit := range result.Hits {
			if hit.Document.OfferID != "desktop" && hit.Document.OfferID != "server" {
				t.Fatal("platform mention leaked", query, hit)
			}
		}
	}
	for _, tc := range []struct {
		query, offer string
		fallback     bool
	}{{"виндовс 11", "desktop", false}, {"плеер для виндовс", "player", false}, {"WIN-001", "desktop", false}, {"Windows Server 2019", "old-server", true}} {
		result, err := a.Search(ctx, client, d.Request{Query: tc.query, Limit: 4})
		if err != nil || len(result.Hits) != 1 || result.Hits[0].Document.OfferID != tc.offer || result.UnpricedFallbackUsed != tc.fallback {
			t.Fatalf("%s: %+v %v", tc.query, result, err)
		}
	}
}
