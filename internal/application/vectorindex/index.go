// Package vectorindex plans and executes a complete manifest through bounded ports.
package vectorindex

import (
	"context"
	"errors"
	"time"

	embedding "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/vectorcorpus"
)

var ErrBudget = errors.New("vector indexing budget exceeded")
var ErrIncomplete = errors.New("vector corpus cache incomplete")

type Budget struct{ ActualNano, ReservedNano, CapNano int64 }
type Cache interface {
	Get(context.Context, []string) (map[string][]float64, error)
	Budget(context.Context) (Budget, error)
}
type Execution struct {
	Requests                         int
	Tokens, ActualNano, ReservedNano int64
}
type Executor interface {
	Ensure(context.Context, []string, int64) (Execution, error)
}
type Report struct {
	Mode, CorpusSHA, SourceSHA                                                      string
	Documents, GroupDocuments, OfferDocuments, UniqueInputs, CacheHits, CacheMisses int
	TokenUpperBound, CostUpperNano, RunCapNano, ActualNano, ReservedNano, Tokens    int64
	Requests                                                                        int
	Complete, BudgetFits                                                            bool
	BudgetBefore, BudgetAfter                                                       Budget
}

func inspect(ctx context.Context, x d.Corpus, s Cache) (hits int, missing []string, tokens int64, err error) {
	unique := []string{}
	seen := map[string]bool{}
	for _, doc := range x.Documents {
		key := embedding.CacheKey(doc.Text)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, doc.Text)
		}
	}
	for start := 0; start < len(unique); start += 32 {
		if err = ctx.Err(); err != nil {
			return
		}
		batch := unique[start:min(start+32, len(unique))]
		keys := make([]string, len(batch))
		for i, t := range batch {
			keys[i] = embedding.CacheKey(t)
		}
		var cached map[string][]float64
		cached, err = s.Get(ctx, keys)
		if err != nil {
			return
		}
		for i, key := range keys {
			if v, ok := cached[key]; ok {
				if err = embedding.ValidateVector(v); err != nil {
					return
				}
				hits++
			} else {
				missing = append(missing, batch[i])
				n, _ := embedding.TokenBound(batch[i])
				tokens += n
			}
		}
	}
	return
}
func Run(ctx context.Context, x d.Corpus, mode string, s Cache, e Executor, maxNano int64) (r Report, err error) {
	if mode != "plan" && mode != "live" && mode != "replay" {
		return r, errors.New("invalid vector indexing mode")
	}
	if err = d.Validate(x); err != nil {
		return
	}
	r = Report{Mode: mode, CorpusSHA: x.SHA, SourceSHA: x.SourceSHA, Documents: len(x.Documents), RunCapNano: maxNano}
	for _, doc := range x.Documents {
		if doc.OfferID == "" {
			r.GroupDocuments++
		} else {
			r.OfferDocuments++
		}
	}
	r.BudgetBefore, err = s.Budget(ctx)
	if err != nil {
		return
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if b, e := s.Budget(c); e == nil {
			r.BudgetAfter = b
		}
	}()
	var missing []string
	r.CacheHits, missing, r.TokenUpperBound, err = inspect(ctx, x, s)
	if err != nil {
		return
	}
	r.CacheMisses = len(missing)
	r.UniqueInputs = r.CacheHits + r.CacheMisses
	r.CostUpperNano = embedding.Cost(r.TokenUpperBound)
	r.Complete = len(missing) == 0
	b := r.BudgetBefore
	r.BudgetFits = b.CapNano > 0 && b.CapNano <= embedding.ProjectCapNano && b.ActualNano >= 0 && b.ReservedNano >= 0 && b.ActualNano <= b.CapNano && b.ReservedNano <= b.CapNano-b.ActualNano && r.CostUpperNano <= b.CapNano-b.ActualNano-b.ReservedNano
	if mode == "plan" {
		return
	}
	if mode == "replay" {
		if !r.Complete {
			err = ErrIncomplete
		}
		return
	}
	if e == nil || maxNano < 1 || maxNano > d.MaxRunNano || r.CostUpperNano > maxNano || !r.BudgetFits {
		err = ErrBudget
		return
	}
	for start := 0; start < len(missing); start += 32 {
		if err = ctx.Err(); err != nil {
			return
		}
		remaining := maxNano - r.ReservedNano
		batch := missing[start:min(start+32, len(missing))]
		var upper int64
		for _, text := range batch {
			n, _ := embedding.TokenBound(text)
			upper += embedding.Cost(n)
		}
		if remaining < upper {
			err = ErrBudget
			return
		}
		var execution Execution
		execution, err = e.Ensure(ctx, batch, min(remaining, embedding.RunCapNano))
		r.Requests += execution.Requests
		r.Tokens += execution.Tokens
		r.ActualNano += execution.ActualNano
		r.ReservedNano += execution.ReservedNano
		if err != nil {
			return
		}
		if execution.ReservedNano < 0 || execution.ReservedNano > remaining || execution.ActualNano < 0 || execution.ActualNano > execution.ReservedNano {
			err = ErrBudget
			return
		}
	}
	var afterMissing []string
	_, afterMissing, _, err = inspect(ctx, x, s)
	if err == nil && len(afterMissing) > 0 {
		err = ErrIncomplete
	}
	r.Complete = err == nil
	return
}
