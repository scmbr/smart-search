package postgres

import (
	"context"
	"github.com/scmbr/smart-search/internal/application/catalogimport"
	reader "github.com/scmbr/smart-search/internal/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
	"testing"
)

func TestReadinessRequiresActiveNormalizedCatalog(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := New(pool)
	if state, err := repo.ActiveCatalog(ctx); err == nil || state.Ready {
		t.Fatal("empty catalog ready", state, err)
	}
	input, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	result, err := catalogimport.Run(ctx, input, repo, 4)
	if err != nil {
		t.Fatal(err)
	}
	state, err := repo.ActiveCatalog(ctx)
	if err != nil || !state.Ready || state.CatalogID != result.Snapshot.ID {
		t.Fatal(state, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE catalog_snapshots SET schema_version='legacy' WHERE id=$1`, state.CatalogID); err != nil {
		t.Fatal(err)
	}
	state, err = repo.ActiveCatalog(ctx)
	if err != nil || state.Ready {
		t.Fatal("old normalization ready", state, err)
	}
}
