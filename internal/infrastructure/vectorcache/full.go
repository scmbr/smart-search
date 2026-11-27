package vectorcache

import (
	"context"
	"sort"

	app "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/search"
	corpus "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
)

const QueryRunCapNano int64 = 1000000

type indexedDocument struct {
	document corpus.Document
	vector   []float64
}
type Full struct {
	manifest    corpus.Corpus
	runtime     app.Runtime
	liveQueries bool
	loading     chan struct{}
	documents   []indexedDocument
}

func NewFull(x corpus.Corpus, runtime app.Runtime, liveQueries bool) (*Full, error) {
	if err := corpus.Validate(x); err != nil {
		return nil, err
	}
	x.Documents = append([]corpus.Document(nil), x.Documents...)
	return &Full{manifest: x, runtime: runtime, liveQueries: liveQueries, loading: make(chan struct{}, 1)}, nil
}

// load keeps only a 512-component copy; partial cache never becomes a ready corpus.
func (c *Full) load(ctx context.Context) ([]indexedDocument, error) {
	select {
	case c.loading <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.loading }()
	if c.documents != nil {
		return c.documents, nil
	}
	runtime := c.runtime
	runtime.Provider = nil
	out := make([]indexedDocument, 0, len(c.manifest.Documents))
	for start := 0; start < len(c.manifest.Documents); start += 512 {
		batch := c.manifest.Documents[start:min(start+512, len(c.manifest.Documents))]
		texts := make([]string, len(batch))
		for i, doc := range batch {
			texts[i] = doc.Text
		}
		execution, err := runtime.Ensure(ctx, texts, app.Options{})
		if err != nil {
			return nil, err
		}
		for _, doc := range batch {
			v := execution.Vectors[embedding.CacheKey(doc.Text)]
			prefix := append([]float64(nil), v[:512]...)
			if _, err := embedding.Cosine(prefix, prefix); err != nil {
				return nil, err
			}
			out = append(out, indexedDocument{document: doc, vector: prefix})
		}
	}
	c.documents = out
	return out, nil
}
func (c *Full) Candidates(ctx context.Context, r d.Request) ([]d.GroupCandidate, error) {
	documents, err := c.load(ctx)
	if err != nil {
		return nil, err
	}
	query, err := c.runtime.Ensure(ctx, []string{r.OriginalQuery}, app.Options{Live: c.liveQueries, RunCapNano: QueryRunCapNano, ProjectCapNano: embedding.ProjectCapNano})
	if err != nil {
		return nil, err
	}
	q := query.Vectors[embedding.CacheKey(r.OriginalQuery)]
	var out []d.GroupCandidate
	for _, doc := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		score, err := embedding.Cosine(q[:512], doc.vector)
		if err != nil {
			return nil, err
		}
		if score >= d.MinimumCosine {
			out = append(out, d.GroupCandidate{GroupID: doc.document.GroupID, OfferID: doc.document.OfferID, Vendor: doc.document.Vendor, Score: score})
		}
	}
	key := func(c d.GroupCandidate) string {
		if c.OfferID != "" {
			return c.Vendor + "/offer/" + c.OfferID
		}
		return c.Vendor + "/group/" + c.GroupID
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return key(out[i]) < key(out[j])
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > d.VectorWindow {
		out = out[:d.VectorWindow]
	}
	return out, nil
}
