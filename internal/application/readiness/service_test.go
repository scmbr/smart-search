package readiness

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/readiness"
	"testing"
)

type catalogFake struct {
	calls   int
	changed bool
	err     error
	cancel  context.CancelFunc
}

func (f *catalogFake) ActiveCatalog(context.Context) (d.State, error) {
	f.calls++
	if f.cancel != nil {
		f.cancel()
	}
	id := "a"
	if f.changed && f.calls > 1 {
		id = "b"
	}
	return d.State{CatalogID: id, Ready: true}, f.err
}

type indexFake struct {
	calls int
	err   error
}

func (f *indexFake) ActiveIndex(context.Context) (d.State, error) {
	f.calls++
	return d.State{CatalogID: "a", Ready: true}, f.err
}

func TestReadinessChecksSnapshotsAndRequiredFailures(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		changed, dbFail, indexFail bool
		want                       bool
	}{
		{"ready", false, false, false, true}, {"changed", true, false, false, false},
		{"db failure", false, true, false, false}, {"index failure", false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &catalogFake{changed: tc.changed}
			i := &indexFake{}
			if tc.dbFail {
				c.err = errors.New("secret DSN")
			}
			if tc.indexFail {
				i.err = errors.New("secret key")
			}
			err := (Service{Catalog: c, Index: i}).Check(context.Background())
			if (err == nil) != tc.want {
				t.Fatal(err)
			}
			if err != nil && !errors.Is(err, d.ErrNotReady) {
				t.Fatal("must return safe classification", err)
			}
		})
	}
}
func TestCanceledReadinessDoesNotCallNextPort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &catalogFake{cancel: cancel}
	i := &indexFake{}
	err := (Service{Catalog: c, Index: i}).Check(ctx)
	if !errors.Is(err, context.Canceled) || i.calls != 0 {
		t.Fatal(err, i.calls)
	}
}
