package hybrid

import (
	"context"
	"errors"
	lexical "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type recoveryCatalog struct {
	base                d.Result
	baseErr, resolveErr error
	cancel              context.CancelFunc
	cancelAt            string
	resolves            int
}

func TestFallbackStillValidatesOriginalCapacity(t *testing.T) {
	hit := h("base", true)
	n := 1
	hit.Document.DevicesLimit = &n
	b := &recoveryCatalog{base: d.Result{CatalogID: "c", IndexVersion: "i", Hits: []d.Hit{hit}}}
	combined := Backend{Lexical: b, Vectors: &recoveryVectors{err: d.ErrUnavailable}}
	_, err := lexical.Search(context.Background(), combined, d.Request{Query: "товар", Limit: 4, MinDevices: 5})
	if !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("fallback dropped capacity constraint", err)
	}
}

func (b *recoveryCatalog) Search(context.Context, d.Request) (d.Result, error) {
	if b.cancelAt == "lexical" {
		b.cancel()
	}
	return b.base, b.baseErr
}
func (b *recoveryCatalog) Resolve(_ context.Context, _ d.Request, _ []d.GroupCandidate, priced bool, _, _ string) ([]d.Hit, error) {
	b.resolves++
	if b.cancelAt == "resolve" {
		b.cancel()
	}
	if !priced || b.resolveErr != nil {
		return nil, b.resolveErr
	}
	return nil, nil
}

type recoveryVectors struct {
	calls  int
	cancel context.CancelFunc
	err    error
}

func (v *recoveryVectors) Candidates(context.Context, d.Request) ([]d.GroupCandidate, error) {
	v.calls++
	if v.cancel != nil {
		v.cancel()
	}
	return []d.GroupCandidate{{GroupID: "g", Vendor: "vendor", Score: .8}}, v.err
}

func TestCancellationStopsFollowingPorts(t *testing.T) {
	for _, at := range []string{"lexical", "vectors", "resolve"} {
		ctx, cancel := context.WithCancel(context.Background())
		b := &recoveryCatalog{base: d.Result{CatalogID: "c", IndexVersion: "i"}, cancel: cancel, cancelAt: at}
		v := &recoveryVectors{}
		if at == "vectors" {
			v.cancel = cancel
		}
		result, err := (Backend{Lexical: b, Vectors: v}).Search(ctx, d.Request{Query: "товар", Limit: 4})
		cancel()
		if !errors.Is(err, context.Canceled) || len(result.Hits) > 0 {
			t.Fatalf("%s: %+v %v", at, result, err)
		}
		if at == "lexical" && v.calls != 0 {
			t.Fatal("vectors called after cancellation")
		}
		if at == "vectors" && b.resolves != 0 {
			t.Fatal("resolve called after cancellation")
		}
		if at == "resolve" && b.resolves != 1 {
			t.Fatal("unpriced resolve called after cancellation")
		}
	}
}

func TestRecoveryPreservesBaselineAndPriceTier(t *testing.T) {
	for _, unpriced := range []bool{false, true} {
		for _, stage := range []string{"vectors", "resolve", "lexical"} {
			base := d.Result{CatalogID: "c", IndexVersion: "i", SearchMode: "exact", UnpricedFallbackUsed: unpriced, Hits: []d.Hit{h("base", !unpriced)}}
			b := &recoveryCatalog{base: base}
			v := &recoveryVectors{}
			switch stage {
			case "vectors":
				v.err = d.ErrUnavailable
			case "resolve":
				b.resolveErr = d.ErrNotReady
			case "lexical":
				b.baseErr = d.ErrUnavailable
			}
			result, err := (Backend{Lexical: b, Vectors: v}).Search(context.Background(), d.Request{Query: "товар", Limit: 4})
			if stage == "lexical" {
				if !errors.Is(err, d.ErrUnavailable) || v.calls != 0 {
					t.Fatal("lexical outage hidden", err)
				}
				continue
			}
			if err != nil || len(result.Hits) != 1 || result.Hits[0].Document.OfferID != "base" || result.UnpricedFallbackUsed != unpriced || result.CatalogID != "c" || result.IndexVersion != "i" || result.SearchMode != "exact" || result.Hybrid.VectorStatus != "fallback" {
				t.Fatalf("%s/%v: %+v %v", stage, unpriced, result, err)
			}
		}
	}
}
