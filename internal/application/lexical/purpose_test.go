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
