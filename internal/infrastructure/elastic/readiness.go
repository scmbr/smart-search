package elastic

import (
	"context"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/readiness"
)

func (c *Client) ActiveIndex(ctx context.Context) (d.State, error) {
	indexes, err := c.aliases(ctx)
	if err != nil || len(indexes) != 1 {
		return d.State{}, d.ErrNotReady
	}
	m, exists, err := c.meta(ctx, indexes[0])
	if err != nil || !exists {
		return d.State{}, d.ErrNotReady
	}
	ready := m.State == "ready" && m.Spec.ValidIdentity() && m.Spec.NormalizationVersion == catalog.NormalizationVersion && m.Spec.DocumentVersion == app.DocumentSchemaVersion
	if !ready {
		return d.State{}, d.ErrNotReady
	}
	_, err = c.searchHits(ctx, indexes[0], map[string]any{"size": 0, "timeout": "2s", "track_total_hits": false, "query": map[string]any{"match_all": map[string]any{}}})
	if err != nil {
		return d.State{}, d.ErrNotReady
	}
	return d.State{CatalogID: m.Spec.CatalogID, Ready: true}, nil
}
