package postgres

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

func TestRuntimeSharedSlotsAndPersistentClaims(t *testing.T) {
	p := testPool(t)
	ctx := context.Background()
	s := NewEmbeddingStore(p)
	g1, err := s.TryGuard(ctx, []string{d.CacheKey("slot-one")})
	if err != nil {
		t.Fatal(err)
	}
	defer g1.Close()
	if _, err = s.TryGuard(ctx, []string{d.CacheKey("slot-one")}); !errors.Is(err, a.ErrBusy) {
		t.Fatal("same key acquired twice", err)
	}
	g2, err := NewEmbeddingStore(p).TryGuard(ctx, []string{d.CacheKey("slot-two")})
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()
	if _, err = s.TryGuard(ctx, []string{d.CacheKey("slot-three")}); !errors.Is(err, a.ErrBusy) {
		t.Fatal("third worker admitted", err)
	}
	if err = g1.Close(); err != nil {
		t.Fatal(err)
	}
	if err = g2.Close(); err != nil {
		t.Fatal(err)
	}
	key := d.CacheKey("uncertain-persisted")
	r := a.Reservation{ID: "runtime-uncertain", UpperNano: 100, CapNano: d.ProjectCapNano}
	if err = s.BeginAttempt(ctx, r, []string{key}); err != nil {
		t.Fatal(err)
	}
	// Simulated process restart: a new store sees the committed unresolved claim.
	r.ID = "must-not-reserve"
	if err = NewEmbeddingStore(p).BeginAttempt(ctx, r, []string{key}); !errors.Is(err, a.ErrUncertain) {
		t.Fatal(err)
	}
	b, err := s.Budget(ctx)
	if err != nil || b.UncertainNano != 100 {
		t.Fatal("blocked attempt consumed budget", b, err)
	}
	if err = s.Rejected(ctx, "runtime-uncertain"); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginAttempt(ctx, r, []string{key}); err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx)
	if err != nil || b.UncertainNano != 200 {
		t.Fatal("rejection refunded uncertain cost", b, err)
	}
}

type concurrentEmbedder struct {
	calls   atomic.Int32
	entered chan struct{}
	resume  chan struct{}
}
type observedStore struct {
	*EmbeddingStore
	busy chan struct{}
}

func (s observedStore) TryGuard(ctx context.Context, keys []string) (a.Guard, error) {
	g, err := s.EmbeddingStore.TryGuard(ctx, keys)
	if errors.Is(err, a.ErrBusy) {
		select {
		case s.busy <- struct{}{}:
		default:
		}
	}
	return g, err
}

func (p *concurrentEmbedder) Embed(ctx context.Context, texts []string) (a.Response, error) {
	p.calls.Add(1)
	p.entered <- struct{}{}
	select {
	case <-p.resume:
	case <-ctx.Done():
		return a.Response{}, ctx.Err()
	}
	r := a.Response{Model: d.Model, Tokens: int64(len(texts))}
	for range texts {
		v := make([]float64, d.Dimensions)
		v[0] = 1
		r.Vectors = append(r.Vectors, v)
	}
	return r, nil
}
func TestRuntimeConcurrentMissPaidOnceWithThreeConnections(t *testing.T) {
	p := testPool(t)
	if p.Config().MaxConns != 3 {
		t.Fatal("test requires the production pool limit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider := &concurrentEmbedder{entered: make(chan struct{}, 2), resume: make(chan struct{})}
	opts := a.Options{Live: true, RunCapNano: d.RunCapNano, ProjectCapNano: d.ProjectCapNano}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	busy := make(chan struct{}, 1)
	run := func() {
		defer wg.Done()
		_, err := (a.Runtime{Store: observedStore{NewEmbeddingStore(p), busy}, Provider: provider}).Ensure(ctx, []string{"concurrent-unique"}, opts)
		results <- err
	}
	wg.Add(1)
	go run()
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("first worker did not reach provider")
	}
	wg.Add(1)
	go run()
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("second worker did not observe shared lock")
	}
	close(provider.resume)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatal("duplicate paid miss", provider.calls.Load())
	}
	b, err := NewEmbeddingStore(p).Budget(ctx)
	if err != nil || b.UncertainNano != 0 || b.ActualNano != 20 {
		t.Fatal(b, err)
	}
}
