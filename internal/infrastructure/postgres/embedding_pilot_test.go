package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

func TestEmbeddingLedgerSurvivesReopenAndSerializesReservations(t *testing.T) {
	p := testPool(t)
	ctx := context.Background()
	s := NewEmbeddingStore(p)
	if _, err := p.Exec(ctx, "UPDATE api_budget SET cap_nano=100"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			results <- s.Reserve(ctx, a.Reservation{ID: id, UpperNano: 80, CapNano: 100})
		}(id)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, a.ErrBudget) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("concurrent budget bypass")
	}
	b, err := NewEmbeddingStore(p).Budget(ctx)
	if err != nil || b.UncertainNano != 80 {
		t.Fatal("restart lost reservation", b, err)
	}
	var id string
	if err = p.QueryRow(ctx, "SELECT id FROM api_reservations").Scan(&id); err != nil {
		t.Fatal(err)
	}
	v := make([]float64, d.Dimensions)
	v[0] = 1
	r := a.Reservation{ID: id, UpperNano: 80, CapNano: 100}
	if err = s.Settle(ctx, r, 40, 2, []a.Entry{{Key: d.CacheKey("bad"), Vector: []float64{1}}}); err == nil {
		t.Fatal("invalid cache accepted")
	}
	if b, err = s.Budget(ctx); err != nil || b.UncertainNano != 80 || b.ActualNano != 0 {
		t.Fatal("failed cache write partially settled budget", b, err)
	}
	if err = s.Settle(ctx, r, 40, 2, []a.Entry{{Key: d.CacheKey("test"), Vector: v}}); err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx)
	if err != nil || b.ActualNano != 40 || b.UncertainNano != 0 {
		t.Fatal(b, err)
	}
	if err = s.Settle(ctx, r, 40, 2, nil); err == nil {
		t.Fatal("double settlement accepted")
	}
	cache, err := s.Get(ctx, []string{d.CacheKey("test")})
	if err != nil || len(cache) != 1 {
		t.Fatal(cache, err)
	}
}
