package search

import "strings"

// identityMatch uses catalog identity, never a mention in a description.
func identityMatch(r Request, document Document) bool {
	if document.ProductName == "" {
		return false
	}
	names := []string{canonicalQuery(document.ProductName)}
	names = append(names, canonicalQuery(protectedTransform(names[0], toLatin)))
	for _, variant := range r.Plan.Variants {
		if len(variant.Terms) == 0 {
			continue
		}
		for _, name := range names {
			words := strings.Fields(name)
			if len(variant.Terms) == 1 && len(words) != 1 {
				continue
			}
			matched := true
			for _, term := range variant.Terms {
				found := false
				for _, word := range words {
					if word == term.Text {
						found = true
						break
					}
				}
				if !found {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	return false
}

// FuseRequest preserves lexical identity matches before semantic-only neighbors.
// Inputs must already have passed the same SKU/price filters.
func FuseRequest(r Request, lexical, vector []Hit, limit int) ([]Hit, int, error) {
	if limit < 1 || limit > CandidateLimit {
		return nil, 0, ErrInvalidQuery
	}
	fused, err := Fuse(lexical, vector, CandidateLimit)
	if err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	out := make([]Hit, 0, len(fused))
	for _, hit := range lexical {
		if !seen[hit.Document.FamilyID] && identityMatch(r, hit.Document) {
			seen[hit.Document.FamilyID] = true
			hit.Score = 1 + 1.0/float64(RankConstant+len(out)+1)
			out = append(out, hit)
		}
	}
	anchors := len(out)
	for _, hit := range fused {
		if !seen[hit.Document.FamilyID] {
			seen[hit.Document.FamilyID] = true
			out = append(out, hit)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, anchors, nil
}
