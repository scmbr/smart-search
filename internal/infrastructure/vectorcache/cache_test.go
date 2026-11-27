package vectorcache

import (
	"context"
	app "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	search "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type cachedStore struct{ vectors map[string][]float64 }

func (s cachedStore) Get(context.Context, []string) (map[string][]float64, error) {
	return s.vectors, nil
}
func (cachedStore) Budget(context.Context) (app.Budget, error)     { panic("not needed for cache read") }
func (cachedStore) Reserve(context.Context, app.Reservation) error { panic("no paid calls") }
func (cachedStore) Settle(context.Context, app.Reservation, int64, int64, []app.Entry) error {
	panic("no paid calls")
}
func (cachedStore) TryGuard(context.Context, []string) (app.Guard, error) { panic("no paid calls") }
func (cachedStore) BeginAttempt(context.Context, app.Reservation, []string) error {
	panic("no paid calls")
}
func (cachedStore) Rejected(context.Context, string) error { panic("no paid calls") }
func dataset() d.Dataset {
	return d.Dataset{Version: d.TextVersion, Role: "development_tuning_pilot", SourceSHA: "27eaec9e77942c61d58b3c04dcaa8991c707dc0fcb64a6442a7408d9908cb148", Documents: []d.Document{{ID: "v/g", GroupID: "g", Vendor: "v", Text: "product", SourceURL: "https://store.softline.ru/v/product/"}}, Queries: []d.Query{{ID: "tune-1", Text: "task", Scope: "family", Split: "tuning", Groups: []string{"g"}}}}
}
func TestCachedCandidatesWithoutPaymentOrLabelLookup(t *testing.T) {
	v := make([]float64, d.Dimensions)
	v[0] = 1
	store := cachedStore{map[string][]float64{d.CacheKey("product"): v, d.CacheKey("task"): v}}
	corpus, err := New(dataset(), app.Runtime{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	got, err := corpus.Candidates(context.Background(), search.Request{OriginalQuery: "task"})
	if err != nil || len(got) != 1 || got[0].GroupID != "g" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = corpus.Candidates(context.Background(), search.Request{OriginalQuery: "not cached"}); err == nil {
		t.Fatal("missing query must abstain")
	}
	bad := dataset()
	bad.Documents[0].SourceURL = "https://store.softline.ru/other/product/"
	if _, err = New(bad, app.Runtime{Store: store}); err == nil {
		t.Fatal("vendor mismatch")
	}
}
