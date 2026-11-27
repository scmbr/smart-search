package lexical

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestRecoveryDoesNotExposePartialErrorOrWrongIdentifier(t *testing.T) {
	price := int64(100)
	doc := d.Document{OfferID: "other", FamilyID: "f", CatalogID: "c", Description: "compatible with AB-1234", PriceMinor: &price}
	d.SetProductRole(&doc)
	b := &behaviorBackend{result: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{{Document: doc}}}}
	if _, err := Search(context.Background(), b, d.Request{Query: "AB-1234", Limit: 4}); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("identifier bypass", err)
	}
	b.err = d.ErrUnavailable
	result, err := Search(context.Background(), b, d.Request{Query: "товар", Limit: 4})
	if !errors.Is(err, d.ErrUnavailable) || len(result.Hits) != 0 || result.IndexVersion != "" {
		t.Fatal("partial error result exposed", result, err)
	}
}

func TestIndexVersionFieldsAreBoundToFingerprint(t *testing.T) {
	spec := NewSpec(CatalogInfo{ID: "snapshot", Offers: 1}, "families")
	if !spec.ValidIdentity() {
		t.Fatal("canonical spec invalid")
	}
	for _, field := range []string{"catalog", "family", "document", "normalization", "name", "count"} {
		changed := spec
		switch field {
		case "catalog":
			changed.CatalogID = "other"
		case "family":
			changed.FamilyVersion = "other"
		case "document":
			changed.DocumentVersion = "other"
		case "normalization":
			changed.NormalizationVersion = "other"
		case "name":
			changed.Name = "other"
		case "count":
			changed.Count = 0
		}
		if changed.ValidIdentity() {
			t.Fatal("corrupt spec accepted", field)
		}
	}
}
