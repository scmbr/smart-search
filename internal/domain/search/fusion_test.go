package search

import (
	"math"
	"testing"
)

func fusionHit(f, offer string) Hit {
	return Hit{Document: Document{FamilyID: f, OfferID: offer, CatalogID: "snapshot"}, Score: 1}
}
func TestFuseFamilies(t *testing.T) {
	a, b, c := fusionHit("a", "a1"), fusionHit("b", "b1"), fusionHit("c", "c1")
	got, err := Fuse([]Hit{a, b, b}, []Hit{c, b}, 4)
	if err != nil || len(got) != 3 || got[0].Document.FamilyID != "b" || got[0].Document.OfferID != "b1" {
		t.Fatalf("%+v %v", got, err)
	}
	if got[1].Document.FamilyID != "a" {
		t.Fatal("ties must be stable")
	}
	want := 1.0/62 + 1.0/62
	if math.Abs(got[0].Score-want) > 1e-12 {
		t.Fatal("duplicate vote")
	}
}
func TestFuseRejectsMalformedIdentity(t *testing.T) {
	for _, bad := range []Hit{
		{Document: Document{FamilyID: "a", OfferID: "a1", CatalogID: "other"}},
		{Document: Document{FamilyID: "a", OfferID: "", CatalogID: "snapshot"}},
		{Document: Document{FamilyID: "a", OfferID: "a1", CatalogID: "snapshot"}, Score: math.NaN()},
		fusionHit("b", "a1"),
	} {
		if _, err := Fuse([]Hit{fusionHit("a", "a1")}, []Hit{bad}, 4); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
