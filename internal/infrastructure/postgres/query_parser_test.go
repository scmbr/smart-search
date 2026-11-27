package postgres

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	e "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	a "github.com/scmbr/smart-search/internal/application/queryparser"
	p "github.com/scmbr/smart-search/internal/domain/queryparser"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

func TestParserSharedBudgetClaimsAndSettlement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := NewQueryParserStore(pool)
	query := "нужно помогать сотрудникам с их компьютерами"
	plan, _ := p.Plan(query)
	reservation := a.Reservation{ID: "parser-one", Key: plan.Key, UpperNano: plan.CostUpperNano}
	if err := s.Begin(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	other := reservation
	other.ID = "parser-two"
	if err := NewQueryParserStore(pool).Begin(ctx, other); !errors.Is(err, a.ErrUncertain) {
		t.Fatalf("unresolved repeated: %v", err)
	}
	ledger := NewEmbeddingStore(pool)
	b, err := ledger.Budget(ctx)
	if err != nil || b.UncertainNano != plan.CostUpperNano {
		t.Fatal(b, err)
	}
	entry := a.Entry{Query: query, Value: d.Interpretation{Version: d.InterpretationVersion, SourceQuery: query, Status: "resolved", SemanticQuery: "программа удаленной поддержки"}}
	bad := reservation
	bad.Key = "wrong"
	if err = s.Settle(ctx, bad, entry, 100, 40); err == nil {
		t.Fatal("wrong claim settled")
	}
	if _, found, err := s.Get(ctx, plan.Key); err != nil || found {
		t.Fatal("partial cache write", err)
	}
	if err = s.Settle(ctx, reservation, entry, 100, 40); err != nil {
		t.Fatal(err)
	}
	if got, found, err := s.Get(ctx, plan.Key); err != nil || !found || got != entry {
		t.Fatal(got, found, err)
	}
	b, err = ledger.Budget(ctx)
	if err != nil || b.ActualNano != p.Cost(100, 40) || b.UncertainNano != 0 {
		t.Fatal(b, err)
	}
	if err = s.Settle(ctx, reservation, entry, 100, 40); err == nil {
		t.Fatal("double settlement")
	}
	// Embeddings and parser reserve against exactly the same locked budget row.
	if err = ledger.Reserve(ctx, e.Reservation{ID: "fill-budget", UpperNano: p.ProjectCapNano - b.ActualNano, CapNano: p.ProjectCapNano}); err != nil {
		t.Fatal(err)
	}
	other.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = s.Begin(ctx, other); !errors.Is(err, a.ErrBudget) {
		t.Fatalf("separate budget: %v", err)
	}
}

type parserProvider struct {
	calls           atomic.Int32
	entered, resume chan struct{}
}

func (f *parserProvider) Generate(ctx context.Context, query string) (a.Response, error) {
	f.calls.Add(1)
	f.entered <- struct{}{}
	select {
	case <-f.resume:
	case <-ctx.Done():
		return a.Response{}, ctx.Err()
	}
	return a.Response{Model: p.Model, InputTokens: 100, OutputTokens: 40, Entry: a.Entry{Query: query, Value: d.Interpretation{Version: d.InterpretationVersion, SourceQuery: query, Status: "resolved", SemanticQuery: "программа удаленной поддержки"}}}, nil
}

type observedParserStore struct {
	*QueryParserStore
	busy chan struct{}
}

func (s observedParserStore) TryGuard(ctx context.Context, key string) (a.Guard, error) {
	g, err := s.QueryParserStore.TryGuard(ctx, key)
	if errors.Is(err, a.ErrBusy) {
		select {
		case s.busy <- struct{}{}:
		default:
		}
	}
	return g, err
}
func TestParserConcurrentMissPaidOnceWithThreeConnections(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider := &parserProvider{entered: make(chan struct{}, 2), resume: make(chan struct{})}
	busy := make(chan struct{}, 1)
	results := make(chan error, 2)
	run := func() {
		_, err := (a.Service{Store: observedParserStore{NewQueryParserStore(pool), busy}, Provider: provider, Live: true}).Run(ctx, "нужно помогать сотрудникам с компьютерами")
		results <- err
	}
	go run()
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("first request", ctx.Err())
	}
	go run()
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("second caller did not see lock", ctx.Err())
	}
	close(provider.resume)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatal("duplicate payment", provider.calls.Load())
	}
	budget, err := NewEmbeddingStore(pool).Budget(ctx)
	if err != nil || budget.ActualNano != p.Cost(100, 40) || budget.UncertainNano != 0 {
		t.Fatal(budget, err)
	}
}

func TestParserSettlementRollsBackOnCacheConflict(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewQueryParserStore(pool)
	query := "нужно помогать сотрудникам с компьютерами"
	plan, _ := p.Plan(query)
	r := a.Reservation{ID: "rollback-parser", Key: plan.Key, UpperNano: plan.CostUpperNano}
	if err := store.Begin(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO query_parser_cache(cache_key,entry) VALUES($1,'{}')`, plan.Key); err != nil {
		t.Fatal(err)
	}
	if err := store.Settle(ctx, r, a.Entry{Query: query, Failure: "invalid"}, 100, 40); err == nil {
		t.Fatal("cache collision accepted")
	}
	b, err := NewEmbeddingStore(pool).Budget(ctx)
	if err != nil || b.ActualNano != 0 || b.UncertainNano != plan.CostUpperNano {
		t.Fatal("partial ledger settlement", b, err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM query_parser_pending WHERE cache_key=$1", plan.Key).Scan(&count); err != nil || count != 1 {
		t.Fatal("claim removed by rollback", count, err)
	}
}

func TestParserSharesAPISlotsAndRejectionDoesNotRefund(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := NewQueryParserStore(pool)
	emb := NewEmbeddingStore(pool)
	plan, _ := p.Plan("нужно помочь сотрудникам с компьютерами")
	g1, err := s.TryGuard(ctx, plan.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer g1.Close()
	if _, err = s.TryGuard(ctx, plan.Key); !errors.Is(err, a.ErrBusy) {
		t.Fatal("duplicate key lock", err)
	}
	g2, err := emb.TryGuard(ctx, []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()
	if _, err = s.TryGuard(ctx, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"); !errors.Is(err, a.ErrBusy) {
		t.Fatal("third API slot", err)
	}
	if err = g1.Close(); err != nil {
		t.Fatal(err)
	}
	if err = g2.Close(); err != nil {
		t.Fatal(err)
	}
	r := a.Reservation{ID: "rejected-parser", Key: plan.Key, UpperNano: plan.CostUpperNano}
	if err = s.Begin(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err = s.Rejected(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	r.ID = "retry-parser"
	if err = s.Begin(ctx, r); err != nil {
		t.Fatal(err)
	}
	b, err := emb.Budget(ctx)
	if err != nil || b.UncertainNano != 2*plan.CostUpperNano {
		t.Fatal("refund", b, err)
	}
}
