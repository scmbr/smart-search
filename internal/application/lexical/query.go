package lexical

import (
	"context"
	"fmt"
	"slices"

	domain "github.com/scmbr/smart-search/internal/domain/search"
)

type Backend interface {
	Search(context.Context, domain.Request) (domain.Result, error)
}

func Search(ctx context.Context, backend Backend, input domain.Request) (domain.Result, error) {
	r, err := domain.Prepare(input)
	if err != nil {
		return domain.Result{}, err
	}
	if len(r.UnsupportedConstraints) > 0 {
		return domain.Result{Query: r.OriginalQuery, QueryPlan: r.Plan, SearchMode: "not_searched", ShortfallReason: "unsupported_query_constraints", AppliedConstraints: r.Constraints, UnsupportedConstraints: r.UnsupportedConstraints}, nil
	}
	result, err := backend.Search(ctx, r)
	if err != nil {
		return result, err
	}
	if result.CatalogID == "" || result.IndexVersion == "" || len(result.Hits) > r.Limit {
		return domain.Result{}, fmt.Errorf("%w: invalid result metadata", domain.ErrUnavailable)
	}
	families := map[string]bool{}
	offers := map[string]bool{}
	for _, hit := range result.Hits {
		d := hit.Document
		invalid := d.OfferID == "" || d.FamilyID == "" || families[d.FamilyID] || offers[d.OfferID] || d.CatalogID != result.CatalogID || d.RoleConflict
		invalid = invalid || (r.Kind != "" && d.OfferKind != r.Kind) || (r.Kind == "" && slices.Contains([]string{"renewal", "upgrade", "support"}, d.OfferKind))
		invalid = invalid || (r.Version != "" && d.Version != r.Version) || (r.Platform != "" && !slices.Contains(d.Platforms, r.Platform)) || (r.ProductType != "" && d.ProductType != r.ProductType)
		invalid = invalid || (r.MinUsers > 0 && (d.UsersLimit == nil || *d.UsersLimit < r.MinUsers)) || (r.Policy == "available_only" && !d.Available)
		invalid = invalid || (r.Edition != "" && d.Edition != r.Edition) || (r.TermKind != "" && d.TermKind != r.TermKind) || (r.Deployment != "" && d.Deployment != r.Deployment)
		invalid = invalid || (r.TermMonths > 0 && (d.TermMonths == nil || *d.TermMonths != r.TermMonths))
		for _, capacity := range []struct {
			required int
			actual   *int
		}{{r.MinDevices, d.DevicesLimit}, {r.MinIPAddresses, d.IPAddressesLimit}, {r.MinParticipants, d.ParticipantsLimit}} {
			invalid = invalid || (capacity.required > 0 && (capacity.actual == nil || *capacity.actual < capacity.required))
		}
		invalid = invalid || (result.UnpricedFallbackUsed && d.PriceMinor != nil) || (!result.UnpricedFallbackUsed && (d.PriceMinor == nil || *d.PriceMinor < 1))
		if invalid {
			return domain.Result{}, fmt.Errorf("%w: result violates request constraints", domain.ErrUnavailable)
		}
		families[d.FamilyID] = true
		offers[d.OfferID] = true
	}
	result.Query = r.OriginalQuery
	result.QueryPlan = r.Plan
	result.AppliedConstraints = r.Constraints
	domain.Describe(&result)
	if len(result.Hits) == 0 && (len(r.Constraints) > 0 || r.Edition != "" || r.TermKind != "" || r.Deployment != "" || r.MinUsers > 0 || r.MinDevices > 0 || r.MinIPAddresses > 0 || r.MinParticipants > 0) {
		result.ShortfallReason = "no_verified_offer_for_constraints"
	}
	return result, nil
}
