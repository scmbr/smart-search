// Package vectorcache ranks a bounded development corpus without new paid calls.
package vectorcache

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"

	app "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

var vendorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

type Corpus struct {
	dataset embedding.Dataset
	runtime app.Runtime
}

func New(x embedding.Dataset, runtime app.Runtime) (*Corpus, error) {
	if err := embedding.Validate(x); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, doc := range x.Documents {
		u, err := url.Parse(doc.SourceURL)
		key := doc.Vendor + "/" + doc.GroupID
		if err != nil || u.Scheme != "https" || u.Host != "store.softline.ru" || u.User != nil || !vendorPattern.MatchString(doc.Vendor) || !strings.HasPrefix(u.Path, "/"+doc.Vendor+"/") || seen[key] {
			return nil, d.ErrInvalidQuery
		}
		seen[key] = true
	}
	// Never permit a paid provider in this adapter, even if the caller supplied one.
	runtime.Provider = nil
	return &Corpus{dataset: x, runtime: runtime}, nil
}
func (c *Corpus) Candidates(ctx context.Context, r d.Request) ([]d.GroupCandidate, error) {
	query, err := c.runtime.Ensure(ctx, []string{r.OriginalQuery}, app.Options{})
	if err != nil {
		return nil, err
	}
	texts := make([]string, 0, len(c.dataset.Documents))
	for _, doc := range c.dataset.Documents {
		texts = append(texts, doc.Text)
	}
	execution, err := c.runtime.Ensure(ctx, texts, app.Options{})
	if err != nil {
		return nil, err
	}
	q := query.Vectors[embedding.CacheKey(r.OriginalQuery)]
	var out []d.GroupCandidate
	for _, doc := range c.dataset.Documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v := execution.Vectors[embedding.CacheKey(doc.Text)]
		score, err := embedding.Cosine(q[:512], v[:512])
		if err != nil {
			return nil, err
		}
		if score >= d.MinimumCosine {
			out = append(out, d.GroupCandidate{GroupID: doc.GroupID, Vendor: doc.Vendor, Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Vendor+"/"+out[i].GroupID < out[j].Vendor+"/"+out[j].GroupID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > d.VectorWindow {
		out = out[:d.VectorWindow]
	}
	return out, nil
}
