package postgres

import (
	"context"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/readiness"
	"strings"
)

func (r *Repository) ActiveCatalog(ctx context.Context) (d.State, error) {
	var id, schema string
	err := r.pool.QueryRow(ctx, `SELECT s.id,s.schema_version FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready'`).Scan(&id, &schema)
	if err != nil {
		return d.State{}, d.ErrNotReady
	}
	return d.State{CatalogID: id, Ready: strings.HasSuffix(schema, "/"+catalog.NormalizationVersion)}, nil
}
