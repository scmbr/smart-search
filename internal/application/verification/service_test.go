package verification

import (
	"context"
	"errors"
	"testing"

	d "github.com/scmbr/smart-search/internal/domain/verification"
)

type storeFunc func(context.Context, Request) (d.Input, error)

func (f storeFunc) Load(c context.Context, q Request) (d.Input, error) { return f(c, q) }
func TestVerifyTrustedInputs(t *testing.T) {
	q := Request{PolicyID: "synthetic-v1", AnchorID: "a", RecommendedID: "b", Catalog: "snapshot"}
	input := func() d.Input {
		f := func(id string) d.Fact {
			return d.Fact{Status: d.Known, Values: []string{"windows"}, Confirmed: true, Source: "synthetic", OfferID: id, Catalog: q.Catalog}
		}
		return d.Input{AnchorID: q.AnchorID, RecommendedID: q.RecommendedID, Catalog: q.Catalog, Policy: d.Policy{ID: q.PolicyID, Rules: []d.Rule{{ID: "platform", Operator: d.Equals, AnchorField: "platform", RecommendedField: "platform"}}}, Anchor: map[string]d.Fact{"platform": f("a")}, Recommended: map[string]d.Fact{"platform": f("b")}}
	}
	s := Service{Store: storeFunc(func(context.Context, Request) (d.Input, error) { return input(), nil })}
	r, err := s.Verify(context.Background(), q)
	if err != nil || r.Status != d.Passed || !d.Publishable(input(), r) {
		t.Fatalf("%+v %v", r, err)
	}
	for _, mutate := range []func(*d.Input){func(i *d.Input) { i.AnchorID = "other" }, func(i *d.Input) { i.RecommendedID = "other" }, func(i *d.Input) { i.Catalog = "old" }, func(i *d.Input) { i.Context.ID = "another-customer" }, func(i *d.Input) { i.Policy.ID = "other" }, func(i *d.Input) { i.Policy.Rules = nil }} {
		s.Store = storeFunc(func(context.Context, Request) (d.Input, error) { i := input(); mutate(&i); return i, nil })
		if _, err = s.Verify(context.Background(), q); !errors.Is(err, d.ErrInvalid) {
			t.Fatalf("substitution accepted: %v", err)
		}
	}
	sentinel := errors.New("store unavailable")
	s.Store = storeFunc(func(context.Context, Request) (d.Input, error) { return d.Input{}, sentinel })
	if _, err = s.Verify(context.Background(), q); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Store = storeFunc(func(context.Context, Request) (d.Input, error) {
		t.Fatal("called after cancellation")
		return d.Input{}, nil
	})
	if _, err = s.Verify(ctx, q); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	s.Store = storeFunc(func(context.Context, Request) (d.Input, error) { cancel(); return input(), nil })
	if _, err = s.Verify(ctx, q); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = (Service{}).Verify(context.Background(), q); !errors.Is(err, d.ErrInvalid) {
		t.Fatal(err)
	}
}
