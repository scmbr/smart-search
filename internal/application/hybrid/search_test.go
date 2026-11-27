package hybrid

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type fakeBackend struct {
	base     d.Result
	resolved []d.Hit
	unpriced []d.Hit
	priced   []bool
	err      error
}

func (f *fakeBackend) Search(context.Context, d.Request) (d.Result, error) { return f.base, nil }
func (f *fakeBackend) Resolve(_ context.Context, _ d.Request, _ []d.GroupCandidate, priced bool, catalog, index string) ([]d.Hit, error) {
	f.priced = append(f.priced, priced)
	if !priced {
		return f.unpriced, f.err
	}
	return f.resolved, f.err
}

type fakeVectors struct {
	calls int
	err   error
}

func (f *fakeVectors) Candidates(context.Context, d.Request) ([]d.GroupCandidate, error) {
	f.calls++
	return []d.GroupCandidate{{GroupID: "g", Vendor: "vendor", Score: 0.8}}, f.err
}
func h(family string, priced bool) d.Hit {
	var price *int64
	if priced {
		n := int64(100)
		price = &n
	}
	return d.Hit{Document: d.Document{FamilyID: family, OfferID: family, CatalogID: "c", PriceMinor: price}, Score: 1}
}
func TestHybridCombinesAndUsesGlobalPricePriority(t *testing.T) {
	for _, unpriced := range []bool{false, true} {
		b := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{h("lexical", !unpriced)}, UnpricedFallbackUsed: unpriced}, resolved: []d.Hit{h("vector", true)}}
		v := &fakeVectors{}
		r, err := (Backend{Lexical: b, Vectors: v}).Search(context.Background(), d.Request{Limit: 4})
		expected := 2
		if unpriced {
			expected = 1
		}
		if err != nil || len(r.Hits) != expected || r.UnpricedFallbackUsed || r.Hybrid.VectorStatus != "used" || len(b.priced) != 1 || !b.priced[0] {
			t.Fatalf("%+v %v", r, err)
		}
	}
}
func TestHybridSafetyAndFallback(t *testing.T) {
	for _, tc := range []struct {
		plan               d.QueryPlan
		vecErr, resolveErr error
		want               string
		calls              int
	}{
		{plan: d.QueryPlan{ExactOnly: true}, want: "skipped_exact", calls: 0},
		{plan: d.QueryPlan{LimitReason: "unparsed_numeric_constraint"}, want: "skipped_exact", calls: 0},
		{vecErr: errors.New("secret backend detail"), want: "fallback", calls: 1},
		{resolveErr: errors.New("stale index"), want: "fallback", calls: 1},
	} {
		b := &fakeBackend{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{h("lexical", true)}}, err: tc.resolveErr}
		v := &fakeVectors{err: tc.vecErr}
		r, err := (Backend{Lexical: b, Vectors: v}).Search(context.Background(), d.Request{Limit: 4, Plan: tc.plan})
		if err != nil || len(r.Hits) != 1 || r.Hybrid.VectorStatus != tc.want || v.calls != tc.calls {
			t.Fatalf("%+v %v", r, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Backend{}).Search(ctx, d.Request{Limit: 4}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
