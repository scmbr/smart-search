package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"strings"
	"testing"
)

func TestSearchSessionsPersistReplayAndExcludeConcurrentExecution(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewSearchStore(pool)
	if err := store.CheckStore(ctx); err != nil {
		t.Fatal(err)
	}
	id := d.Identity{Session: "s", Key: "key", BodyHash: strings.Repeat("a", 64)}
	lease, err := store.Begin(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if lease.Cached() != nil {
		t.Fatal("new key was cached")
	}
	if other, err := store.Begin(ctx, id); !errors.Is(err, d.ErrUnavailable) {
		if other != nil {
			other.Close()
		}
		t.Fatal("concurrent execution allowed", err)
	}
	record := d.Record{ID: "saved", DurationMS: 12, Result: s.Result{Query: "query", CatalogID: "catalog", IndexVersion: "index"}}
	if err = lease.Complete(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	// New pool/store has no in-memory knowledge of the original request.
	otherPool, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer otherPool.Close()
	replayed, err := NewSearchStore(otherPool).Begin(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := replayed.Cached(); got == nil || got.ID != record.ID || got.DurationMS != 12 || got.Result.IndexVersion != "index" {
		t.Fatal("replay changed", got)
	}
	replayed.Close()
	changed := id
	changed.BodyHash = strings.Repeat("b", 64)
	if _, err = store.Begin(ctx, changed); !errors.Is(err, d.ErrConflict) {
		t.Fatal("changed body did not conflict", err)
	}
	changed.Session = "another"
	other, err := store.Begin(ctx, changed)
	if err != nil {
		t.Fatal("session scope broken", err)
	}
	other.Close()
	if _, err = pool.Exec(ctx, "UPDATE search_sessions SET created_at=now()-interval '25 hours',expires_at=now()-interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	changed.Session = id.Session
	fresh, err := store.Begin(ctx, changed)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if fresh.Cached() != nil {
		t.Fatal("expired record still cached")
	}
}
