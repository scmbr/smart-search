package search

import (
	"fmt"
	"math"
	"sort"
)

const HybridVersion = "hybrid-v1"
const RankConstant = 60
const CandidateLimit = 20
const VectorWindow = 50
const MinimumCosine = 0.35

type GroupCandidate struct {
	GroupID, Vendor string
	Score           float64
}
type HybridTrace struct {
	Version, CorpusSHA, VectorStatus                 string
	CorpusDocuments, LexicalFamilies, VectorFamilies int
}

// Fuse gives at most one vote per family in each source. Lexical SKU wins ties.
func Fuse(lexical, vector []Hit, limit int) ([]Hit, error) {
	if limit < 1 || limit > CandidateLimit || len(lexical) > CandidateLimit || len(vector) > CandidateLimit {
		return nil, ErrInvalidQuery
	}
	byFamily := map[string]Hit{}
	offerFamilies := map[string]string{}
	catalog := ""
	for _, list := range [][]Hit{lexical, vector} {
		seen := map[string]bool{}
		rank := 0
		for _, h := range list {
			d := h.Document
			if catalog == "" {
				catalog = d.CatalogID
			}
			previous, exists := offerFamilies[d.OfferID]
			if d.FamilyID == "" || d.OfferID == "" || d.CatalogID == "" || d.CatalogID != catalog || math.IsNaN(h.Score) || math.IsInf(h.Score, 0) || (exists && previous != d.FamilyID) {
				return nil, fmt.Errorf("%w: invalid fusion identity", ErrUnavailable)
			}
			offerFamilies[d.OfferID] = d.FamilyID
			if seen[d.FamilyID] {
				continue
			}
			seen[d.FamilyID] = true
			rank++
			current, ok := byFamily[d.FamilyID]
			if !ok {
				current = h
				current.Score = 0
			}
			current.Score += 1.0 / float64(RankConstant+rank)
			byFamily[d.FamilyID] = current
		}
	}
	out := make([]Hit, 0, len(byFamily))
	for _, h := range byFamily {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Document.FamilyID < out[j].Document.FamilyID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
