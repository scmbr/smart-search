package lexical

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestUnverifiedExtensionQuantityNeverCallsBackend(t *testing.T) {
	for _, query := range []string{"2 дополнительных IP", "дополнительный IP к лицензии на 4 IP", "лицензия на 10 узлов"} {
		backend := &behaviorBackend{}
		result, err := Search(context.Background(), backend, d.Request{Query: query, Limit: 4})
		if err != nil || backend.calls != 0 || result.ShortfallReason != "unsupported_query_constraints" {
			t.Fatalf("%s: %+v %v calls=%d", query, result, err, backend.calls)
		}
	}
}

func TestBackendCannotReturnTransitionAsBaseLicense(t *testing.T) {
	price := int64(100)
	doc := d.Document{Name: "XSpider Pro лицензия перехода с XSpider", OfferID: "540220", FamilyID: "f", CatalogID: "c", PriceMinor: &price}
	d.SetProductRole(&doc)
	backend := &behaviorBackend{result: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{{Document: doc}}}}
	if _, err := Search(context.Background(), backend, d.Request{Query: "сканер уязвимостей", Limit: 4}); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("dependent license bypass", err)
	}
}
