package elastic

import (
	"context"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

var vendorPathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

// Resolve pins the physical BM25 index, filters individual SKUs, then collapses.
func (c *Client) Resolve(ctx context.Context, r d.Request, groups []d.GroupCandidate, priced bool, catalogID, fingerprint string) ([]d.Hit, error) {
	index := "smart-search-" + fingerprint
	if !indexPattern.MatchString(index) || len(groups) < 1 || len(groups) > d.VectorWindow {
		return nil, d.ErrNotReady
	}
	m, exists, err := c.meta(ctx, index)
	if err != nil {
		return nil, err
	}
	if !exists || m.State != "ready" || m.Spec.CatalogID != catalogID || m.Spec.Fingerprint != fingerprint || m.Spec.DocumentVersion != app.DocumentSchemaVersion || m.Spec.NormalizationVersion != catalog.NormalizationVersion {
		return nil, d.ErrNotReady
	}
	body := searchBody(r, priced, false)
	query := body["query"].(map[string]any)["bool"].(map[string]any)
	clauses := make([]any, 0, len(groups))
	ranks := map[string]int{}
	for i, g := range groups {
		key := g.Vendor + "/group/" + g.GroupID
		field, value := "GroupID", g.GroupID
		if g.OfferID != "" {
			key = g.Vendor + "/offer/" + g.OfferID
			field, value = "OfferID", g.OfferID
		}
		if !vendorPathPattern.MatchString(g.Vendor) || (g.GroupID == "") == (g.OfferID == "") || len(g.GroupID) > 512 || len(g.OfferID) > 512 || math.IsNaN(g.Score) || math.IsInf(g.Score, 0) || g.Score < d.MinimumCosine || (i > 0 && g.Score > groups[i-1].Score) {
			return nil, d.ErrInvalidQuery
		}
		if _, ok := ranks[key]; ok {
			return nil, d.ErrInvalidQuery
		}
		ranks[key] = i
		filter := map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"term": map[string]any{field: value}},
			map[string]any{"prefix": map[string]any{"URL": "https://store.softline.ru/" + g.Vendor + "/"}},
		}}}
		clauses = append(clauses, map[string]any{"constant_score": map[string]any{"filter": filter, "boost": 1000.0 / float64(i+1)}})
	}
	query["must"] = []any{map[string]any{"bool": map[string]any{"should": clauses, "minimum_should_match": 1}}}
	delete(query, "should") // SKU role preference must not reorder semantic groups.
	body["size"] = d.CandidateLimit
	hits, err := c.searchHits(ctx, index, body)
	if err != nil {
		return nil, err
	}
	rankOf := func(h d.Hit) (int, bool) {
		u, err := url.Parse(h.Document.URL)
		if err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" {
			return 0, false
		}
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) < 2 {
			return 0, false
		}
		rank, ok := ranks[parts[0]+"/group/"+h.Document.GroupID]
		if !ok {
			rank, ok = ranks[parts[0]+"/offer/"+h.Document.OfferID]
		}
		return rank, ok
	}
	for _, h := range hits {
		if _, ok := rankOf(h); !ok || h.Document.CatalogID != catalogID {
			return nil, d.ErrUnavailable
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { a, _ := rankOf(hits[i]); b, _ := rankOf(hits[j]); return a < b })
	return hits, nil
}
