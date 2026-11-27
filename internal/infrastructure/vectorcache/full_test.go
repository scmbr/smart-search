package vectorcache

import (
	"context"
	app "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	search "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
	"math"
	"strings"
	"testing"
)

type countedStore struct {
	cachedStore
	reads int
}

func TestFullRejectsWeakNeighborsInsteadOfPaddingResults(t *testing.T) {
	x := fullManifest()
	q := make([]float64, embedding.Dimensions)
	q[0] = 1
	makeVector := func(score float64) []float64 {
		v := make([]float64, embedding.Dimensions)
		v[0] = score
		v[1] = math.Sqrt(1 - score*score)
		return v
	}
	s := cachedStore{map[string][]float64{embedding.CacheKey("product"): makeVector(0.44), embedding.CacheKey("sku"): makeVector(0.46), embedding.CacheKey("task"): q}}
	c, err := NewFull(x, app.Runtime{Store: s}, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Candidates(context.Background(), search.Request{OriginalQuery: "task"})
	if err != nil || len(got) != 1 || got[0].OfferID != "s" {
		t.Fatalf("weak unrelated candidate accepted: %+v %v", got, err)
	}
}

func (c *countedStore) Get(ctx context.Context, keys []string) (map[string][]float64, error) {
	c.reads++
	return c.cachedStore.Get(ctx, keys)
}
func fullManifest() d.Corpus {
	x := d.Corpus{Version: d.Version, TextVersion: embedding.TextVersion, Model: embedding.Model, Dimensions: embedding.Dimensions, SourceSHA: strings.Repeat("a", 64), Offers: 2, Documents: []d.Document{
		{ID: "v/group/g", GroupID: "g", Vendor: "v", Text: "product", SourceURL: "https://store.softline.ru/v/product/"},
		{ID: "v/offer/s", OfferID: "s", Vendor: "v", Text: "sku", SourceURL: "https://store.softline.ru/v/sku/"},
	}}
	x.SHA = d.Fingerprint(x)
	return x
}

type forbiddenProvider struct{}

func (forbiddenProvider) Embed(context.Context, []string) (app.Response, error) {
	panic("incomplete corpus cannot pay for queries")
}

func TestFullCorpusLoadedOnceAndSKUIdentityPreserved(t *testing.T) {
	x := fullManifest()
	v := make([]float64, embedding.Dimensions)
	v[0] = 1
	s := &countedStore{cachedStore: cachedStore{map[string][]float64{embedding.CacheKey("product"): v, embedding.CacheKey("sku"): v, embedding.CacheKey("task"): v}}}
	corpus, err := NewFull(x, app.Runtime{Store: s}, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := corpus.Candidates(context.Background(), search.Request{OriginalQuery: "task"})
		if err != nil || len(got) != 2 || got[1].OfferID != "s" || got[1].GroupID != "" {
			t.Fatalf("%+v %v", got, err)
		}
	}
	if s.reads != 3 {
		t.Fatalf("expected one corpus read and two query reads, got %d", s.reads)
	}
	if _, err = corpus.Candidates(context.Background(), search.Request{OriginalQuery: "missing"}); err == nil {
		t.Fatal("cache miss cannot pay")
	}
	delete(s.vectors, embedding.CacheKey("sku"))
	incomplete, _ := NewFull(x, app.Runtime{Store: s, Provider: forbiddenProvider{}}, true)
	if _, err = incomplete.Candidates(context.Background(), search.Request{OriginalQuery: "task"}); err == nil || incomplete.documents != nil {
		t.Fatal("partial corpus became ready")
	}
}
