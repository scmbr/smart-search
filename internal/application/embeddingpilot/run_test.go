package embeddingpilot

import (
	"context"
	"errors"
	"testing"

	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

type memoryStore struct {
	cache            map[string][]float64
	reserved, actual int64
	deny             bool
}
type noopGuard struct{}

func (noopGuard) Close() error                                           { return nil }
func (s *memoryStore) TryGuard(context.Context, []string) (Guard, error) { return noopGuard{}, nil }
func (s *memoryStore) BeginAttempt(ctx context.Context, r Reservation, _ []string) error {
	return s.Reserve(ctx, r)
}
func (s *memoryStore) Rejected(context.Context, string) error { return nil }

func (s *memoryStore) Get(context.Context, []string) (map[string][]float64, error) {
	return s.cache, nil
}
func (s *memoryStore) Budget(context.Context) (Budget, error) {
	return Budget{ActualNano: s.actual, UncertainNano: s.reserved}, nil
}
func (s *memoryStore) Reserve(_ context.Context, r Reservation) error {
	if s.deny {
		return ErrBudget
	}
	s.reserved += r.UpperNano
	return nil
}
func (s *memoryStore) Settle(_ context.Context, r Reservation, n, tokens int64, entries []Entry) error {
	s.reserved -= r.UpperNano
	s.actual += n
	for _, e := range entries {
		s.cache[e.Key] = e.Vector
	}
	return nil
}

type fakeProvider struct {
	store *memoryStore
	calls int
	fail  bool
}

func (p *fakeProvider) Embed(_ context.Context, texts []string) (Response, error) {
	if p.store.reserved == 0 {
		return Response{}, errors.New("network before reservation")
	}
	p.calls++
	if p.fail {
		return Response{}, errors.New("unknown request outcome")
	}
	r := Response{Model: d.Model, Tokens: int64(len(texts))}
	for range texts {
		v := make([]float64, d.Dimensions)
		v[0] = 1
		r.Vectors = append(r.Vectors, v)
	}
	return r, nil
}
func fixture() d.Dataset {
	return d.Dataset{Version: d.TextVersion, Role: "development_tuning_pilot", SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Documents: []d.Document{{ID: "v/g", GroupID: "g", Vendor: "v", Text: "Product"}}, Queries: []d.Query{{ID: "probe-01", Text: "task", Scope: "family", Split: "pilot_probe", Groups: []string{"g"}}}}
}
func TestReserveCacheReplayAndUnknownOutcome(t *testing.T) {
	ctx := context.Background()
	s := &memoryStore{cache: map[string][]float64{}}
	p := &fakeProvider{store: s}
	r, err := Run(ctx, fixture(), Options{Live: true, RunCapNano: 50000000, ProjectCapNano: 5000000000}, s, p, nil)
	if err != nil || p.calls != 1 || r.Requests != 1 || s.actual == 0 || s.reserved != 0 {
		t.Fatalf("%+v %+v %v", r, s, err)
	}
	_, err = Run(ctx, fixture(), Options{Evaluate: true}, s, nil, nil)
	if err != nil || p.calls != 1 {
		t.Fatal("cache replay made a paid call", err)
	}
	s = &memoryStore{cache: map[string][]float64{}}
	p = &fakeProvider{store: s, fail: true}
	_, err = Run(ctx, fixture(), Options{Live: true, RunCapNano: 50000000, ProjectCapNano: 5000000000}, s, p, nil)
	if err == nil || s.reserved == 0 || len(s.cache) != 0 {
		t.Fatal("uncertain cost lost or bad response cached")
	}
}
func TestDryRunBudgetAndHoldoutGuard(t *testing.T) {
	ctx := context.Background()
	s := &memoryStore{cache: map[string][]float64{}, deny: true}
	p := &fakeProvider{store: s}
	if _, err := Run(ctx, fixture(), Options{}, s, p, nil); err != nil || p.calls != 0 {
		t.Fatal("dry run used network", err)
	}
	if _, err := Run(ctx, fixture(), Options{Live: true, RunCapNano: 50000000, ProjectCapNano: 5000000000}, s, p, nil); !errors.Is(err, ErrBudget) || p.calls != 0 {
		t.Fatal("budget bypass", err)
	}
	x := fixture()
	x.Queries[0].Split = "holdout"
	if _, err := Run(ctx, x, Options{}, s, p, nil); err == nil {
		t.Fatal("holdout accepted")
	}
}
