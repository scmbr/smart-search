// Package verification loads trusted policy and evidence before domain evaluation.
package verification

import (
	"context"
	"time"

	d "github.com/scmbr/smart-search/internal/domain/verification"
)

type Request struct{ PolicyID, AnchorID, RecommendedID, Catalog, ContextID string }

// Store must return a complete server-owned policy and confirmed scoped evidence.
// ContextID identifies trusted customer context, not an arbitrary rights assertion.
type Store interface {
	Load(context.Context, Request) (d.Input, error)
}
type Service struct{ Store Store }

func (s Service) Verify(ctx context.Context, q Request) (d.Report, error) {
	if err := ctx.Err(); err != nil {
		return d.Report{}, err
	}
	if s.Store == nil || q.PolicyID == "" || q.AnchorID == "" || q.RecommendedID == "" || q.Catalog == "" {
		return d.Report{}, d.ErrInvalid
	}
	i, err := s.Store.Load(ctx, q)
	if ctx.Err() != nil {
		return d.Report{}, ctx.Err()
	}
	if err != nil {
		return d.Report{}, err
	}
	if i.Policy.ID != q.PolicyID || i.AnchorID != q.AnchorID || i.RecommendedID != q.RecommendedID || i.Catalog != q.Catalog || i.Context.ID != q.ContextID {
		return d.Report{}, d.ErrInvalid
	}
	return d.Evaluate(i, time.Now())
}
