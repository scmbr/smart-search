package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scmbr/smart-search/internal/application/catalogimport"
	reader "github.com/scmbr/smart-search/internal/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/catalog"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set: PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	schema := fmt.Sprintf("test_catalog_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		if err != nil {
			t.Error(err)
		}
	})
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestPostgresBatchRollbackAndWriterLock(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := New(pool)
	a, err := repo.Begin(ctx, domain.Identity{SourceSHA256: "a", SchemaVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Begin(ctx, domain.Identity{SourceSHA256: "b", SchemaVersion: "v1"}); !errors.Is(err, domain.ErrBusy) {
		t.Fatalf("lock: %v", err)
	}
	category := domain.Record{Category: &domain.Category{ID: "cat", Name: "category"}}
	if err := a.Save(ctx, 1, []domain.Record{category, category}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if a.Snapshot().Checkpoint != 0 {
		t.Fatal("checkpoint advanced on failed transaction")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM catalog_records").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch: %d %v", count, err)
	}
	if err := a.Save(ctx, 1, []domain.Record{category}); err != nil {
		t.Fatal(err)
	}
	if err := a.Finish(ctx, domain.Validation{Complete: true, Categories: 1, Offers: 1, Accepted: 1}); !errors.Is(err, domain.ErrCheckpoint) {
		t.Fatalf("inconsistent activation: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := repo.Begin(ctx, domain.Identity{SourceSHA256: "a", SchemaVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Snapshot().Checkpoint != 1 {
		t.Fatal("checkpoint lost")
	}
}

type interruptedSource struct {
	source    *feed.Source
	stopAfter int
}

func (s interruptedSource) Identity() domain.Identity { return s.source.Identity() }
func (s interruptedSource) Read(ctx context.Context, emit func(context.Context, domain.Record) error) (domain.Validation, error) {
	seen := 0
	return s.source.Read(ctx, func(ctx context.Context, r domain.Record) error {
		seen++
		if s.stopAfter > 0 && seen > s.stopAfter {
			return errors.New("simulated connection loss")
		}
		return emit(ctx, r)
	})
}

func TestPostgresImportResumeAndActivation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := New(pool)
	source, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	partial, err := catalogimport.Run(ctx, interruptedSource{source: source, stopAfter: 9}, repo, 4)
	if err == nil || partial.Snapshot.Status != domain.Staging || partial.Snapshot.Checkpoint != 8 {
		t.Fatalf("partial: %+v %v", partial, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM catalog_active").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial activated")
	}
	complete, err := catalogimport.Run(ctx, source, repo, 4)
	if err != nil || complete.Snapshot.Status != domain.Ready || complete.Snapshot.Checkpoint != 17 {
		t.Fatalf("resume: %+v %v", complete, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM catalog_records").Scan(&count); err != nil || count != 17 {
		t.Fatalf("rows: %d %v", count, err)
	}
	var payload []byte
	if err := pool.QueryRow(ctx, "SELECT payload FROM catalog_records WHERE kind='offer' AND payload->>'PriceMinor' IS NULL").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var persisted storedOffer
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.ID != "unavailable-zero" || persisted.PriceMinor != nil || persisted.Available || persisted.RawFields == nil {
		t.Fatalf("lost fields: %+v", persisted)
	}
	if err := pool.QueryRow(ctx, "SELECT payload FROM catalog_records WHERE kind='offer' AND source_id='os-new'").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var priced storedOffer
	if err := json.Unmarshal(payload, &priced); err != nil {
		t.Fatal(err)
	}
	if priced.PriceMinor == nil || *priced.PriceMinor != 120050 || !priced.Available || len(priced.Parameters) != 4 || priced.Parameters[2].RawValue != "Русский" || priced.Parameters[3].RawValue != "Английский" {
		t.Fatalf("lost exact price or repeated parameters: %+v", priced)
	}
	reused, err := catalogimport.Run(ctx, source, repo, 4)
	if err != nil || !reused.Reused || reused.Snapshot.ID != complete.Snapshot.ID {
		t.Fatalf("repeat: %+v %v", reused, err)
	}
	// A new source version can become active. Repeating an older ready version
	// must not revert the active pointer.
	newer, err := feed.Open(ctx, "../../../testdata/catalog/minimal.xml", t.TempDir(), reader.Options{MaxIssues: 101})
	if err != nil {
		t.Fatal(err)
	}
	defer newer.Close()
	latest, err := catalogimport.Run(ctx, newer, repo, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalogimport.Run(ctx, source, repo, 4); err != nil {
		t.Fatal(err)
	}
	assertActive(t, pool, latest.Snapshot.ID)

	// Invalid completed data and a late XML error preserve the active catalog.
	bad, err := feed.Open(ctx, "../../../testdata/catalog/invalid.xml", t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	invalid, err := catalogimport.Run(ctx, bad, repo, 3)
	if !errors.Is(err, domain.ErrInvalidCatalog) || invalid.Snapshot.Status != domain.Invalid {
		t.Fatalf("invalid: %+v %v", invalid, err)
	}
	assertActive(t, pool, latest.Snapshot.ID)
	data, err := os.ReadFile("../../../testdata/catalog/minimal.xml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "late.xml")
	if err := os.WriteFile(path, append(data, []byte("<broken>")...), 0600); err != nil {
		t.Fatal(err)
	}
	late, err := feed.Open(ctx, path, t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	failed, err := catalogimport.Run(ctx, late, repo, 3)
	if err == nil || failed.Snapshot.Status != domain.Staging {
		t.Fatalf("late error: %+v %v", failed, err)
	}
	assertActive(t, pool, latest.Snapshot.ID)
}

func assertActive(t *testing.T, pool *pgxpool.Pool, want string) {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), "SELECT snapshot_id FROM catalog_active").Scan(&id); err != nil || id != want {
		t.Fatalf("active=%s want=%s err=%v", id, want, err)
	}
}

func TestPostgresMigrationIdempotencyAndChecksum(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET sha256='modified'"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err == nil {
		t.Fatal("modified migration accepted")
	}
}
