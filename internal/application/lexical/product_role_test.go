package lexical

import (
	"context"
	"errors"
	"testing"

	d "github.com/scmbr/smart-search/internal/domain/search"
)

func TestProductRoleCannotBeBypassedByBackend(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		forge       bool
		valid       bool
	}{
		{query: "диаграммы", name: "Diagram SDK"},
		{query: "диаграммы", name: "Дополнительные компоненты продукта"},
		{query: "диаграммы", name: "Diagram SDK", forge: true},
		{query: "библиотека диаграмм", name: "Diagram SDK", valid: true},
		{query: "библиотека диаграмм", name: "Неизвестный продукт"},
		{query: "SKU-1234", name: "Diagram SDK", valid: true},
		{query: "диаграммы", name: "Неизвестный продукт", valid: true},
	} {
		t.Run(tc.query+tc.name, func(t *testing.T) {
			price := int64(100)
			doc := d.Document{Name: tc.name, OfferID: "SKU-1234", FamilyID: "f", CatalogID: "c", PriceMinor: &price}
			d.SetProductRole(&doc)
			if tc.forge {
				doc.ProductRole = ""
				doc.ProductRoleRule = ""
				doc.ProductRoleSource = ""
				doc.ProductRoleText = ""
			}
			backend := &behaviorBackend{result: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{{Document: doc}}}}
			_, err := Search(context.Background(), backend, d.Request{Query: tc.query, Limit: 4})
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !errors.Is(err, d.ErrUnavailable) {
				t.Fatal("role bypass accepted", err)
			}
		})
	}
}
