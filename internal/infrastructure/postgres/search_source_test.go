package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/scmbr/smart-search/internal/application/catalogimport"
	reader "github.com/scmbr/smart-search/internal/catalog"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
)

func TestIndexSourcePinsSnapshotAndSerializesBuilders(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	input, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	first, err := catalogimport.Run(ctx, input, New(pool), 4)
	if err != nil {
		t.Fatal(err)
	}
	source := NewCatalogSource(pool)
	f, err := source.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Info().ID != first.Snapshot.ID || f.Info().Offers != 10 || len(f.Categories()) != 7 {
		t.Fatalf("source=%+v", f.Info())
	}
	if _, err := source.Open(ctx); !errors.Is(err, catalog.ErrBusy) {
		t.Fatalf("parallel builder: %v", err)
	}
	count := 0
	var after int64
	for {
		rows, err := f.Read(ctx, after, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if row.Ordinal <= after || row.Offer.Characteristics == nil {
				t.Fatal("invalid cursor or lost characteristics")
			}
			after = row.Ordinal
			count++
		}
	}
	if count != 10 {
		t.Fatalf("offers=%d", count)
	}
	if current, err := f.Current(ctx); err != nil || !current {
		t.Fatalf("current=%v err=%v", current, err)
	}
	newer, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{MaxIssues: 101})
	if err != nil {
		t.Fatal(err)
	}
	defer newer.Close()
	if _, err := catalogimport.Run(ctx, newer, New(pool), 4); err != nil {
		t.Fatal(err)
	}
	if current, err := f.Current(ctx); err != nil || current || f.Info().ID != first.Snapshot.ID {
		t.Fatalf("catalog change/pinning: %v %v", current, err)
	}
}
