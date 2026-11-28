package postgres

import (
	"context"
	"github.com/scmbr/smart-search/internal/application/catalogimport"
	reader "github.com/scmbr/smart-search/internal/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
	"testing"
)

func TestNamesOnlyUseTrustedCatalogSources(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	input, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	imported, err := catalogimport.Run(ctx, input, New(pool), 4)
	if err != nil {
		t.Fatal(err)
	}
	source := NewCatalogSource(pool)
	snapshot, err := source.LoadNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CatalogID != imported.Snapshot.ID || len(snapshot.Entities) != 0 {
		t.Fatalf("untrusted fixture URLs accepted: %+v", snapshot)
	}
	// Modify only this test's isolated schema to represent a synthetic trusted source.
	_, err = pool.Exec(ctx, `UPDATE catalog_records SET payload=jsonb_set(payload,'{URL}',to_jsonb('https://store.softline.ru/synthetic/product/'::text)) WHERE kind='offer'`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = source.LoadNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entities) == 0 {
		t.Fatal("no vendors loaded")
	}
	for _, e := range snapshot.Entities {
		if e.Scope != "/synthetic/" || e.Kind != "brand" {
			t.Fatalf("unexpected entity: %+v", e)
		}
	}
}
