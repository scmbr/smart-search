package lexical

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestPurposeCannotBeBypassedByBackend(t *testing.T) {
	for _, group := range []string{"165644", "360395", "unknown"} {
		price := int64(100)
		doc := d.Document{OfferID: "fixture", GroupID: group, URL: "https://store.softline.ru/redsoft/fixture/", FamilyID: "f", CatalogID: "c", PriceMinor: &price, ProductType: "software"}
		d.SetProductRole(&doc)
		backend := &behaviorBackend{result: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{{Document: doc}}}}
		_, err := Search(context.Background(), backend, d.Request{Query: "защищенная российская операционная система", Limit: 4})
		if group == "165644" && err != nil {
			t.Fatal(err)
		}
		if group != "165644" && !errors.Is(err, d.ErrUnavailable) {
			t.Fatal("purpose bypass", err)
		}
	}
}

func TestWindowsPurposeCannotBeBypassedByBackend(t *testing.T) {
	price := int64(100)
	for _, valid := range []bool{false, true} {
		doc := d.Document{OfferID: "fixture", FamilyID: "f", CatalogID: "c", URL: "https://store.softline.ru/microsoft/windows/", PriceMinor: &price, HasPrice: true, Available: true, ProductType: "software", ProductName: "Player for Windows", CategoryNames: []string{"Multimedia"}}
		if valid {
			doc.ProductName = "Microsoft Windows 11 Pro"
			doc.CategoryNames = []string{"Операционные системы"}
		}
		d.SetProductRole(&doc)
		backend := &behaviorBackend{result: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{{Document: doc}}}}
		result, err := Search(context.Background(), backend, d.Request{Query: "виндовс", Limit: 4})
		if valid && (err != nil || len(result.Hits) != 1) {
			t.Fatalf("Windows rejected: %+v %v", result, err)
		}
		if !valid && (!errors.Is(err, d.ErrUnavailable) || len(result.Hits) != 0) {
			t.Fatalf("platform mention bypassed application: %+v %v", result, err)
		}
	}
}
