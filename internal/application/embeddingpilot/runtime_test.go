package embeddingpilot

import (
	"context"
	"errors"
	"testing"
	"time"

	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

type runtimeMemory struct {
	memoryStore
	pending        map[string]string
	guards, closed int
	busy           bool
}
type memoryGuard struct{ s *runtimeMemory }

func (g memoryGuard) Close() error { g.s.closed++; return nil }
func (s *runtimeMemory) TryGuard(context.Context, []string) (Guard, error) {
	if s.busy {
		return nil, ErrBusy
	}
	s.guards++
	return memoryGuard{s}, nil
}
func (s *runtimeMemory) BeginAttempt(ctx context.Context, r Reservation, keys []string) error {
	for _, key := range keys {
		if s.pending[key] != "" {
			return ErrUncertain
		}
	}
	if err := s.Reserve(ctx, r); err != nil {
		return err
	}
	for _, key := range keys {
		s.pending[key] = r.ID
	}
	return nil
}
func (s *runtimeMemory) Rejected(_ context.Context, id string) error {
	for k, v := range s.pending {
		if v == id {
			delete(s.pending, k)
		}
	}
	return nil
}
func (s *runtimeMemory) Settle(ctx context.Context, r Reservation, n, tokens int64, entries []Entry) error {
	if err := s.memoryStore.Settle(ctx, r, n, tokens, entries); err != nil {
		return err
	}
	return s.Rejected(ctx, r.ID)
}

type providerFunc func(context.Context, []string) (Response, error)

func (f providerFunc) Embed(ctx context.Context, texts []string) (Response, error) {
	return f(ctx, texts)
}
func runtimeFixture() (*runtimeMemory, Options) {
	return &runtimeMemory{memoryStore: memoryStore{cache: map[string][]float64{}}, pending: map[string]string{}}, Options{Live: true, RunCapNano: d.RunCapNano, ProjectCapNano: d.ProjectCapNano}
}
func goodResponse(texts []string) Response {
	r := Response{Model: d.Model, Tokens: int64(len(texts))}
	for range texts {
		v := make([]float64, d.Dimensions)
		v[0] = 1
		r.Vectors = append(r.Vectors, v)
	}
	return r
}
func TestRuntimeDedupCacheAndUnknownOutcome(t *testing.T) {
	s, o := runtimeFixture()
	calls := 0
	p := providerFunc(func(_ context.Context, texts []string) (Response, error) {
		calls++
		if len(texts) != 2 || len(s.pending) != 2 || s.reserved == 0 {
			t.Fatal("dedup/reserve before network failed")
		}
		return goodResponse(texts), nil
	})
	r, err := (Runtime{s, p}).Ensure(context.Background(), []string{"one", "two", "one"}, o)
	if err != nil || calls != 1 || len(r.Vectors) != 2 || s.guards != s.closed {
		t.Fatal(r, err)
	}
	if _, err = (Runtime{Store: s}).Ensure(context.Background(), []string{"two", "one"}, Options{}); err != nil {
		t.Fatal("cache required provider", err)
	}
	s, o = runtimeFixture()
	p = providerFunc(func(context.Context, []string) (Response, error) {
		calls++
		return Response{}, errors.New("network unknown")
	})
	_, err = (Runtime{s, p}).Ensure(context.Background(), []string{"new"}, o)
	if err == nil || len(s.pending) != 1 || s.reserved == 0 || s.guards != s.closed {
		t.Fatal("unknown outcome not retained")
	}
	before := calls
	if _, err = (Runtime{s, p}).Ensure(context.Background(), []string{"new"}, o); !errors.Is(err, ErrUncertain) || calls != before {
		t.Fatal("unknown request repeated", err)
	}
}
func TestRuntimeRetriesAreSelectiveAndBudgeted(t *testing.T) {
	for _, kind := range []string{"rate_limit", "quota", "rejected", "uncertain"} {
		t.Run(kind, func(t *testing.T) {
			s, o := runtimeFixture()
			calls := 0
			p := providerFunc(func(context.Context, []string) (Response, error) { calls++; return Response{}, &Failure{Kind: kind} })
			r, err := (Runtime{s, p}).Ensure(context.Background(), []string{"one"}, o)
			want := 1
			if kind == "rate_limit" {
				want = 3
			}
			if err == nil || calls != want || r.Requests != want || s.reserved != r.ReservedNano || s.guards != s.closed {
				t.Fatal(r, calls, err)
			}
			if (len(s.pending) > 0) != (kind == "uncertain") {
				t.Fatal("wrong persistent claim")
			}
		})
	}
	s, o := runtimeFixture()
	o.RunCapNano = d.Cost(19)
	calls := 0
	p := providerFunc(func(context.Context, []string) (Response, error) {
		calls++
		return Response{}, &Failure{Kind: "rate_limit"}
	})
	if _, err := (Runtime{s, p}).Ensure(context.Background(), []string{"one"}, o); !errors.Is(err, ErrBudget) || calls != 1 {
		t.Fatal("retry bypassed run cap", err)
	}
}
func TestRuntimeCancellationAndValidation(t *testing.T) {
	s, o := runtimeFixture()
	s.busy = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Runtime{Store: s, Provider: providerFunc(func(context.Context, []string) (Response, error) { t.Fatal("network"); return Response{}, nil })}).Ensure(ctx, []string{"one"}, o); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := (Runtime{Store: s}).Ensure(context.Background(), []string{"one"}, Options{}); !errors.Is(err, ErrCacheMiss) {
		t.Fatal(err)
	}
	if err := wait(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s, o = runtimeFixture()
	calls := 0
	p := providerFunc(func(context.Context, []string) (Response, error) { calls++; return Response{}, nil })
	if _, err := (Runtime{s, p}).Ensure(context.Background(), []string{"valid", ""}, o); err == nil || calls != 0 {
		t.Fatal("invalid tail sent valid prefix")
	}
}

func TestRuntimeRetrySuccessAndCancellation(t *testing.T) {
	s, o := runtimeFixture()
	calls := 0
	p := providerFunc(func(_ context.Context, texts []string) (Response, error) {
		calls++
		if calls == 1 {
			return Response{}, &Failure{Kind: "rate_limit"}
		}
		return goodResponse(texts), nil
	})
	r, err := (Runtime{s, p}).Ensure(context.Background(), []string{"one"}, o)
	if err != nil || calls != 2 || r.ActualNano != 20 || s.reserved != d.Cost(19) || len(s.pending) != 0 {
		t.Fatal(r, err)
	}
	s, o = runtimeFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p = providerFunc(func(context.Context, []string) (Response, error) {
		cancel()
		return Response{}, &Failure{Kind: "rate_limit"}
	})
	if r, err = (Runtime{s, p}).Ensure(ctx, []string{"one"}, o); !errors.Is(err, context.Canceled) || r.Requests != 1 || s.guards != s.closed || len(s.pending) != 0 {
		t.Fatal(r, err)
	}
	s, o = runtimeFixture()
	p = providerFunc(func(context.Context, []string) (Response, error) {
		return Response{}, &Failure{Kind: "rate_limit", RetryAfter: time.Minute}
	})
	if r, err = (Runtime{s, p}).Ensure(context.Background(), []string{"one"}, o); err == nil || r.Requests != 1 {
		t.Fatal("long Retry-After ignored", r, err)
	}
}

func TestRuntimeInvalidResponseKeepsClaim(t *testing.T) {
	for _, scenario := range []string{"tokens", "model", "vectors"} {
		t.Run(scenario, func(t *testing.T) {
			s, o := runtimeFixture()
			p := providerFunc(func(_ context.Context, texts []string) (Response, error) {
				r := goodResponse(texts)
				switch scenario {
				case "tokens":
					r.Tokens = 1000000
				case "model":
					r.Model = "other"
				case "vectors":
					r.Vectors[0] = []float64{1}
				}
				return r, nil
			})
			r, err := (Runtime{s, p}).Ensure(context.Background(), []string{"one"}, o)
			if err == nil || r.Requests != 1 || len(s.pending) != 1 || len(s.cache) != 0 || s.actual != 0 {
				t.Fatal(r, err)
			}
		})
	}
}
