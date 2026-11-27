// Package embeddingpilot orchestrates an isolated experiment through external ports.
package embeddingpilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

var ErrBudget = errors.New("embedding budget exhausted or invalid")

type Budget struct{ ActualNano, UncertainNano, CapNano int64 }
type Reservation struct {
	ID                 string
	UpperNano, CapNano int64
}
type Entry struct {
	Key    string
	Vector []float64
}
type Store interface {
	Get(context.Context, []string) (map[string][]float64, error)
	Budget(context.Context) (Budget, error)
	Reserve(context.Context, Reservation) error
	Settle(context.Context, Reservation, int64, int64, []Entry) error
}
type Response struct {
	Model   string
	Tokens  int64
	Vectors [][]float64
}
type Provider interface {
	Embed(context.Context, []string) (Response, error)
}
type Lexical interface {
	Rank(context.Context, d.Dataset) ([][]d.Rank, error)
}
type Options struct {
	Live, Evaluate             bool
	RunCapNano, ProjectCapNano int64
}
type Row struct {
	Query                                d.Query
	BM25, Vector1536, Vector512          []d.Rank
	BM25Hit, Vector1536Hit, Vector512Hit *bool
}
type Metrics struct {
	Split                                                                                     string
	PositiveCases, BM25Hits, Vector1536Hits, Vector512Hits, BrandCases, NegativeForcedMatches int
}
type Report struct {
	Mode, DatasetSHA, Model, Note                                                                            string
	Dimensions, Documents, Queries, UniqueInputs, CacheHits, CacheMisses, Requests                           int
	TokenUpperBound, CostUpperNano, ActualTokens, ActualNano, FullGroupCostUpperNano, FullOfferCostUpperNano int64
	BudgetBefore, BudgetAfter                                                                                Budget
	BatchLatencyMS                                                                                           []float64
	RankMillis                                                                                               int64
	Rows                                                                                                     []Row
	Metrics                                                                                                  []Metrics
}

func Run(ctx context.Context, x d.Dataset, o Options, s Store, p Provider, l Lexical) (r Report, err error) {
	if err = d.Validate(x); err != nil {
		return r, err
	}
	r = Report{Mode: "plan", DatasetSHA: d.Fingerprint(x), Model: d.Model, Dimensions: d.Dimensions, Documents: len(x.Documents), Queries: len(x.Queries), FullGroupCostUpperNano: d.Cost(x.FullGroupTokenBound), FullOfferCostUpperNano: d.Cost(x.FullOfferTokenBound), Note: "Candidate-only development pilot; no SKU eligibility, independent relevance or production SLO. No holdout."}
	texts, keys := []string{}, []string{}
	seen := map[string]bool{}
	add := func(text string) {
		key := d.CacheKey(text)
		if !seen[key] {
			texts = append(texts, text)
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, doc := range x.Documents {
		add(doc.Text)
	}
	for _, q := range x.Queries {
		add(q.Text)
	}
	r.UniqueInputs = len(keys)
	vectors, e := s.Get(ctx, keys)
	if e != nil {
		return r, e
	}
	if vectors == nil {
		vectors = map[string][]float64{}
	}
	var missing []string
	for i, key := range keys {
		if v, ok := vectors[key]; ok {
			if err = d.ValidateVector(v); err != nil {
				return r, err
			}
			r.CacheHits++
		} else {
			missing = append(missing, texts[i])
			n, _ := d.TokenBound(texts[i])
			r.TokenUpperBound += n
		}
	}
	r.CacheMisses = len(missing)
	r.CostUpperNano = d.Cost(r.TokenUpperBound)
	r.BudgetBefore, err = s.Budget(ctx)
	if err != nil {
		return r, err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if b, e := s.Budget(c); e == nil {
			r.BudgetAfter = b
		}
	}()
	if !o.Live && !o.Evaluate {
		return r, nil
	}
	r.Mode = "replay"
	if o.Live {
		r.Mode = "live"
	}
	if len(missing) > 0 && !o.Live {
		return r, errors.New("cache miss in replay; no API call made")
	}
	if o.Live && (p == nil || o.ProjectCapNano < 1 || o.ProjectCapNano > d.ProjectCapNano || o.RunCapNano < 1 || o.RunCapNano > d.RunCapNano || r.CostUpperNano > o.RunCapNano) {
		return r, ErrBudget
	}
	for start := 0; start < len(missing); start += 32 {
		batch := missing[start:min(start+32, len(missing))]
		var bound int64
		for _, text := range batch {
			n, _ := d.TokenBound(text)
			bound += n
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return r, err
		}
		reservation := Reservation{ID: hex.EncodeToString(nonce[:]), UpperNano: d.Cost(bound), CapNano: o.ProjectCapNano}
		if err = s.Reserve(ctx, reservation); err != nil {
			return r, err
		}
		r.Requests++
		started := time.Now()
		response, e := p.Embed(ctx, batch)
		r.BatchLatencyMS = append(r.BatchLatencyMS, float64(time.Since(started).Microseconds())/1000)
		if e != nil {
			return r, e
		}
		if response.Model != d.Model || response.Tokens < 1 || response.Tokens > bound || len(response.Vectors) != len(batch) {
			return r, errors.New("invalid embedding response; reservation retained")
		}
		var entries []Entry
		for i, v := range response.Vectors {
			if err = d.ValidateVector(v); err != nil {
				return r, err
			}
			entries = append(entries, Entry{Key: d.CacheKey(batch[i]), Vector: v})
		}
		actual := d.Cost(response.Tokens)
		if err = s.Settle(ctx, reservation, actual, response.Tokens, entries); err != nil {
			return r, err
		}
		r.ActualTokens += response.Tokens
		r.ActualNano += actual
		for _, entry := range entries {
			vectors[entry.Key] = entry.Vector
		}
	}
	var lexical [][]d.Rank
	if l != nil {
		lexical, err = l.Rank(ctx, x)
		if err != nil {
			return r, err
		}
		if len(lexical) != len(x.Queries) {
			return r, errors.New("invalid lexical result count")
		}
	}
	start := time.Now()
	byID := map[string]d.Document{}
	for _, doc := range x.Documents {
		byID[doc.ID] = doc
	}
	hit := func(q d.Query, ranks []d.Rank) *bool {
		if q.Scope == "empty" {
			return nil
		}
		yes := false
		for _, rank := range ranks {
			doc, ok := byID[rank.ID]
			if !ok {
				continue
			}
			if q.Scope == "brand" {
				yes = yes || doc.Vendor == q.Vendor
			} else {
				for _, g := range q.Groups {
					yes = yes || g == doc.GroupID
				}
			}
		}
		return &yes
	}
	for i, q := range x.Queries {
		row := Row{Query: q}
		row.Vector1536, err = d.RankVectors(x.Documents, vectors, q.Text, 1536)
		if err != nil {
			return r, err
		}
		row.Vector512, err = d.RankVectors(x.Documents, vectors, q.Text, 512)
		if err != nil {
			return r, err
		}
		if l != nil {
			row.BM25 = lexical[i]
			row.BM25Hit = hit(q, row.BM25)
		}
		row.Vector1536Hit = hit(q, row.Vector1536)
		row.Vector512Hit = hit(q, row.Vector512)
		r.Rows = append(r.Rows, row)
	}
	r.RankMillis = time.Since(start).Milliseconds()
	for _, split := range []string{"tuning", "pilot_probe"} {
		m := Metrics{Split: split}
		for _, row := range r.Rows {
			if row.Query.Split != split {
				continue
			}
			if row.Query.Scope == "empty" {
				if len(row.Vector1536) > 0 {
					m.NegativeForcedMatches++
				}
				continue
			}
			if row.Query.Scope == "brand" {
				m.BrandCases++
				continue
			}
			m.PositiveCases++
			if row.BM25Hit != nil && *row.BM25Hit {
				m.BM25Hits++
			}
			if *row.Vector1536Hit {
				m.Vector1536Hits++
			}
			if *row.Vector512Hit {
				m.Vector512Hits++
			}
		}
		r.Metrics = append(r.Metrics, m)
	}
	return r, nil
}

func Summary(r Report) string {
	return fmt.Sprintf("%s: %d documents, %d queries, %d requests, usage cost %.8f USD", r.Mode, r.Documents, r.Queries, r.Requests, float64(r.ActualNano)/1e9)
}
