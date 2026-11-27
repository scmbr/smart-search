package search

import "testing"

func TestIdentifierRequiresMatchingSKUOrArticle(t *testing.T) {
	r, err := Prepare(Request{Query: "AB-1234", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		doc     Document
		allowed bool
	}{
		{Document{OfferID: "AB-1234"}, true},
		{Document{OfferID: "other", VendorCode: "ab-1234"}, true},
		{Document{OfferID: "other", Description: "compatible with AB-1234"}, false},
	} {
		if IdentifierAllowed(r, tc.doc) != tc.allowed {
			t.Fatal(tc)
		}
	}
}
