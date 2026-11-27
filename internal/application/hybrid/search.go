// Package hybrid combines ranked families through ports; it never calls an SDK.
package hybrid

import (
	"context"
	"fmt"
	"strings"

	d "github.com/scmbr/smart-search/internal/domain/search"
)

type Catalog interface {
	Search(context.Context, d.Request) (d.Result, error)
	Resolve(context.Context, d.Request, []d.GroupCandidate, bool, string, string) ([]d.Hit, error)
}
type Vectors interface {
	Candidates(context.Context, d.Request) ([]d.GroupCandidate, error)
}
type Parser interface {
	Interpret(context.Context, string) (d.Interpretation, error)
}
type Backend struct {
	Parser                     Parser
	Lexical                    Catalog
	Vectors                    Vectors
	CorpusSHA, ExpectedCatalog string
	CorpusDocuments            int
}

// Search accepts a prepared request. Wrap it with lexical.Search to validate output.
func (b Backend) Search(ctx context.Context, r d.Request) (d.Result, error) {
	if err := ctx.Err(); err != nil {
		return d.Result{}, err
	}
	limit := r.Limit
	if limit < 4 || limit > d.CandidateLimit {
		return d.Result{}, d.ErrInvalidQuery
	}
	if b.Lexical == nil {
		return d.Result{}, d.ErrUnavailable
	}
	r.Limit = d.CandidateLimit
	result, err := b.Lexical.Search(ctx, r)
	if err != nil {
		return result, err
	}
	trace := &d.HybridTrace{Version: d.HybridVersion, CorpusSHA: b.CorpusSHA, CorpusDocuments: b.CorpusDocuments, LexicalFamilies: len(result.Hits), VectorStatus: "fallback", InterpretationStatus: "not_selected"}
	result.Hybrid = trace
	finish := func() (d.Result, error) {
		if err := ctx.Err(); err != nil {
			return d.Result{}, err
		}
		if len(result.Hits) > limit {
			result.Hits = result.Hits[:limit]
		}
		d.Describe(&result)
		return result, nil
	}
	if r.Plan.ExactOnly || r.Plan.LimitReason != "" {
		trace.VectorStatus = "skipped_exact"
		return finish()
	}
	if b.ExpectedCatalog != "" && b.ExpectedCatalog != result.CatalogID {
		trace.VectorStatus = "catalog_mismatch"
		return finish()
	}
	if b.Vectors == nil {
		return finish()
	}
	vectorRequest := r
	vectorRequest.SemanticQuery = ""
	if len(result.Hits) == 0 && len(strings.Fields(r.OriginalQuery)) >= 5 && r.FamilyID == "" && len(r.ProductIntents) == 0 {
		trace.InterpretationStatus = "disabled"
		if b.Parser != nil {
			x, parseErr := b.Parser.Interpret(ctx, r.OriginalQuery)
			if err := ctx.Err(); err != nil {
				return d.Result{}, err
			}
			switch {
			case parseErr != nil:
				trace.InterpretationStatus = "fallback"
			case d.ValidateInterpretation(r.OriginalQuery, x) != nil:
				trace.InterpretationStatus = "invalid"
			case x.Status == "clarify":
				trace.InterpretationStatus = "clarification_required"
				trace.VectorStatus = "skipped_interpretation"
				return finish()
			case x.Status == "unsupported":
				trace.InterpretationStatus = "unsupported"
				trace.VectorStatus = "skipped_interpretation"
				return finish()
			default:
				trace.InterpretationStatus = "used"
				trace.SemanticQuery = x.SemanticQuery
				vectorRequest.SemanticQuery = x.SemanticQuery
			}
		}
	}
	groups, err := b.Vectors.Candidates(ctx, vectorRequest)
	if err != nil {
		return finish()
	}
	if len(groups) == 0 {
		trace.VectorStatus = "no_candidates"
		return finish()
	}
	if len(groups) > d.VectorWindow {
		return d.Result{}, fmt.Errorf("%w: vector window exceeded", d.ErrUnavailable)
	}
	trace.VectorCandidates = groups
	hits, err := b.Lexical.Resolve(ctx, r, groups, true, result.CatalogID, result.IndexVersion)
	if err != nil {
		return finish()
	}
	lexical := result.Hits
	if len(hits) > 0 && result.UnpricedFallbackUsed {
		lexical = nil
		result.UnpricedFallbackUsed = false
	}
	if len(hits) == 0 && (len(lexical) == 0 || result.UnpricedFallbackUsed) {
		hits, err = b.Lexical.Resolve(ctx, r, groups, false, result.CatalogID, result.IndexVersion)
		if err != nil {
			return finish()
		}
		if len(hits) > 0 {
			result.UnpricedFallbackUsed = true
		}
	}
	for _, h := range hits {
		if h.Document.CatalogID != result.CatalogID {
			return d.Result{}, fmt.Errorf("%w: vector catalog mismatch", d.ErrUnavailable)
		}
	}
	result.Hits, err = d.Fuse(lexical, hits, limit)
	if err != nil {
		return d.Result{}, err
	}
	trace.VectorFamilies = len(hits)
	for _, hit := range hits {
		trace.VectorFamilyIDs = append(trace.VectorFamilyIDs, hit.Document.FamilyID)
	}
	trace.VectorStatus = "used"
	result.SearchMode = "hybrid"
	return finish()
}
