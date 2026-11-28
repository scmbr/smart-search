package searchsession

import (
	s "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

func TestSummaryCountsOnlyConfirmedDistinctFamilies(t *testing.T) {
	result := s.Result{Hits: []s.Hit{
		{Document: s.Document{OfferID: "unknown", FamilyID: "provisional"}},
		{Document: s.Document{OfferID: "a", FamilyID: "confirmed", FamilyConfirmed: true}},
		{Document: s.Document{OfferID: "duplicate", FamilyID: "confirmed", FamilyConfirmed: true}},
		{Document: s.Document{OfferID: "b", FamilyID: "second", FamilyConfirmed: true}},
	}}
	view := Summarize(result)
	if len(view.TopOfferIDs) != 2 || view.TopOfferIDs[0] != "a" || view.TopOfferIDs[1] != "b" || view.Shortfall != "verification_incomplete" {
		t.Fatal(view)
	}
}

func TestSummaryCompleteAndEmpty(t *testing.T) {
	result := s.Result{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		result.Hits = append(result.Hits, s.Hit{Document: s.Document{OfferID: id, FamilyID: id, FamilyConfirmed: true}})
	}
	view := Summarize(result)
	if len(view.TopOfferIDs) != 4 || view.TopOfferIDs[3] != "d" || view.Shortfall != "" || view.ExactTotal != nil {
		t.Fatal(view)
	}
	view = Summarize(s.Result{})
	if view.ExactTotal == nil || *view.ExactTotal != 0 || view.Shortfall == "" {
		t.Fatal(view)
	}
}
