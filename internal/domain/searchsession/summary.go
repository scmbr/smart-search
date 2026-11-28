package searchsession

import s "github.com/scmbr/smart-search/internal/domain/search"

type Summary struct {
	TopOfferIDs []string
	Shortfall   string
	ExactTotal  *int
}

// Summarize never counts provisional identities as confirmed product families.
func Summarize(result s.Result) Summary {
	view := Summary{TopOfferIDs: []string{}}
	if result.NameResolution != nil && result.NameResolution.Status == "ambiguous" {
		return view
	}
	seen := map[string]bool{}
	for _, hit := range result.Hits {
		doc := hit.Document
		if doc.FamilyConfirmed && doc.FamilyID != "" && doc.OfferID != "" && !seen[doc.FamilyID] && len(view.TopOfferIDs) < 4 {
			seen[doc.FamilyID] = true
			view.TopOfferIDs = append(view.TopOfferIDs, doc.OfferID)
		}
	}
	if len(view.TopOfferIDs) < 4 {
		view.Shortfall = "insufficient_relevant_families"
		if len(result.Hits) >= 4 {
			view.Shortfall = "verification_incomplete"
		}
	}
	if len(result.Hits) == 0 && (result.NameResolution == nil || result.NameResolution.Status != "ambiguous") {
		zero := 0
		view.ExactTotal = &zero
	}
	return view
}
