package searchsession

import (
	"context"
	"errors"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"testing"
)

type fakeLease struct {
	cached   *d.Record
	complete int
	fail     bool
	closed   bool
}

func (f *fakeLease) Cached() *d.Record { return f.cached }
func (f *fakeLease) Complete(context.Context, d.Record) error {
	f.complete++
	if f.fail {
		return errors.New("storage secret")
	}
	return nil
}
func (f *fakeLease) Close() error { f.closed = true; return nil }

type fakeStore struct {
	lease *fakeLease
	err   error
}

func (f fakeStore) Begin(context.Context, d.Identity) (Lease, error) { return f.lease, f.err }
func identity() d.Identity {
	return d.Identity{Session: "s", Key: "k", BodyHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
}
func TestReplayDoesNotExecuteSearch(t *testing.T) {
	l := &fakeLease{cached: &d.Record{ID: "previous"}}
	service := Service{Store: fakeStore{lease: l}, Engine: func(context.Context, s.Request) (s.Result, error) {
		t.Fatal("replayed search executed")
		return s.Result{}, nil
	}}
	r, err := service.Search(context.Background(), identity(), s.Request{Query: "product", Limit: 4})
	if err != nil || r.ID != "previous" || l.complete != 0 || !l.closed {
		t.Fatal(r, err, l)
	}
}
func TestSearchErrorsAndStorageFailuresDoNotBecomeSuccess(t *testing.T) {
	for _, engineFail := range []bool{false, true} {
		l := &fakeLease{fail: !engineFail}
		calls := 0
		service := Service{Store: fakeStore{lease: l}, Engine: func(context.Context, s.Request) (s.Result, error) {
			calls++
			if engineFail {
				return s.Result{}, errors.New("backend secret")
			}
			return s.Result{CatalogID: "catalog", IndexVersion: "index"}, nil
		}}
		_, err := service.Search(context.Background(), identity(), s.Request{Query: "product", Limit: 4})
		if !errors.Is(err, d.ErrUnavailable) || calls != 1 || !l.closed {
			t.Fatal(err, calls, l)
		}
	}
}

func TestCancellationPreventsSavingAndReleasesLease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &fakeLease{}
	service := Service{Store: fakeStore{lease: l}, Engine: func(context.Context, s.Request) (s.Result, error) {
		cancel()
		return s.Result{CatalogID: "catalog", IndexVersion: "index"}, nil
	}}
	_, err := service.Search(ctx, identity(), s.Request{Query: "product", Limit: 4})
	if !errors.Is(err, context.Canceled) || l.complete != 0 || !l.closed {
		t.Fatal(err, l)
	}
}
