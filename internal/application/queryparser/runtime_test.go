package queryparser

import (
	"context"
	"errors"
	"testing"

	p "github.com/scmbr/smart-search/internal/domain/queryparser"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

const testQuery = "нужно помогать сотрудникам с их компьютерами"

type memoryStore struct {
	entry                 Entry
	cached, pending       bool
	begins, settles       int
	reserveErr, settleErr error
}

func (s *memoryStore) Get(context.Context, string) (Entry, bool, error) {
	return s.entry, s.cached, nil
}

type guard struct{}

func (guard) Close() error                                             { return nil }
func (s *memoryStore) TryGuard(context.Context, string) (Guard, error) { return guard{}, nil }
func (s *memoryStore) Begin(_ context.Context, _ Reservation) error {
	if s.reserveErr != nil {
		return s.reserveErr
	}
	if s.pending {
		return ErrUncertain
	}
	s.pending = true
	s.begins++
	return nil
}
func (s *memoryStore) Settle(_ context.Context, _ Reservation, e Entry, _, _ int64) error {
	if s.settleErr != nil {
		return s.settleErr
	}
	s.entry = e
	s.cached = true
	s.pending = false
	s.settles++
	return nil
}
func (s *memoryStore) Rejected(context.Context, string) error { s.pending = false; return nil }

type fakeProvider struct {
	calls    int
	response Response
	err      error
	cancel   context.CancelFunc
}

func (f *fakeProvider) Generate(context.Context, string) (Response, error) {
	f.calls++
	if f.cancel != nil {
		f.cancel()
	}
	return f.response, f.err
}

func TestRuntimeSettlesKnownUsageAfterClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &memoryStore{}
	provider := &fakeProvider{response: validResponse(), cancel: cancel}
	r, err := (Service{Store: store, Provider: provider, Live: true}).Run(ctx, testQuery)
	if !errors.Is(err, context.Canceled) || store.pending || store.settles != 1 || r.AccountedNano != p.Cost(100, 40) {
		t.Fatalf("%+v %v pending=%v", r, err, store.pending)
	}
	r, err = (Service{Store: store}).Run(context.Background(), testQuery)
	if err != nil || !r.CacheHit {
		t.Fatal("paid answer lost after cancellation", err)
	}
}
func validResponse() Response {
	return Response{Model: p.Model, InputTokens: 100, OutputTokens: 40, Entry: Entry{Query: testQuery, Value: d.Interpretation{Version: d.InterpretationVersion, SourceQuery: testQuery, Status: "resolved", SemanticQuery: "программа удаленного доступа к компьютерам"}}}
}

func TestRuntimeCacheAndBudget(t *testing.T) {
	s := &memoryStore{}
	f := &fakeProvider{response: validResponse()}
	service := Service{Store: s, Provider: f}
	if _, err := service.Run(context.Background(), testQuery); !errors.Is(err, ErrCacheMiss) || f.calls != 0 {
		t.Fatal(err)
	}
	service.Live = true
	service.RunCapNano = 1
	if _, err := service.Run(context.Background(), testQuery); !errors.Is(err, ErrBudget) || f.calls != 0 {
		t.Fatal(err)
	}
	service.RunCapNano = p.QueryCapNano
	r, err := service.Run(context.Background(), testQuery)
	if err != nil || r.Requests != 1 || r.AccountedNano != p.Cost(100, 40) || s.pending {
		t.Fatalf("%+v %v", r, err)
	}
	service.Live = false
	service.Provider = nil
	r, err = service.Run(context.Background(), testQuery)
	if err != nil || !r.CacheHit || r.Requests != 0 || f.calls != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestRuntimeFailureAccountingAndPersistentClaims(t *testing.T) {
	for _, kind := range []string{"invalid", "refused", "truncated", "bad_value", "bad_usage", "timeout", "settle", "budget", "rejected"} {
		t.Run(kind, func(t *testing.T) {
			s := &memoryStore{}
			f := &fakeProvider{response: validResponse()}
			switch kind {
			case "invalid", "refused", "truncated":
				f.response.Entry = Entry{Query: testQuery, Failure: kind}
			case "bad_value":
				f.response.Entry.Value.SourceQuery = "wrong"
			case "bad_usage":
				f.response.OutputTokens = p.MaxOutputTokens + 1
			case "timeout":
				f.err = context.DeadlineExceeded
			case "settle":
				s.settleErr = errors.New("database error")
			case "budget":
				s.reserveErr = ErrBudget
			case "rejected":
				f.err = ErrRejected
			}
			service := Service{Store: s, Provider: f, Live: true, RunCapNano: p.QueryCapNano}
			if _, err := service.Run(context.Background(), testQuery); err == nil {
				t.Fatal("accepted failure")
			}
			if kind == "budget" {
				if f.calls != 0 {
					t.Fatal("spent over budget")
				}
				return
			}
			if kind == "rejected" {
				if s.pending {
					t.Fatal("definitive rejection retained claim")
				}
				return
			}
			if s.pending {
				service.Store = s // A fresh service must see the persisted unresolved claim.
				if _, err := service.Run(context.Background(), testQuery); !errors.Is(err, ErrUncertain) || f.calls != 1 {
					t.Fatalf("duplicate payment: %v calls=%d", err, f.calls)
				}
			} else {
				if s.settles != 1 || !s.cached {
					t.Fatal("invalid output not accounted/cached")
				}
				if _, err := service.Run(context.Background(), testQuery); err == nil || f.calls != 1 {
					t.Fatal("failed output paid again")
				}
			}
		})
	}
}
func TestRuntimeRejectsCorruptCacheAndCancelledInput(t *testing.T) {
	s := &memoryStore{cached: true, entry: Entry{Query: "wrong"}}
	if _, err := (Service{Store: s}).Run(context.Background(), testQuery); err == nil {
		t.Fatal("corrupt cache accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Service{Store: s}).Run(ctx, testQuery); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
