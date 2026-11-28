// Package readiness checks mandatory ports without depending on HTTP or SDKs.
package readiness

import (
	"context"
	d "github.com/scmbr/smart-search/internal/domain/readiness"
)

type Catalog interface {
	ActiveCatalog(context.Context) (d.State, error)
}
type Index interface {
	ActiveIndex(context.Context) (d.State, error)
}
type Service struct {
	Catalog Catalog
	Index   Index
}

func (s Service) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Catalog == nil || s.Index == nil {
		return d.ErrNotReady
	}
	catalog, err := s.Catalog.ActiveCatalog(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !catalog.Ready {
		return d.ErrNotReady
	}
	index, err := s.Index.ActiveIndex(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return d.ErrNotReady
	}
	if err = d.Check(catalog, index); err != nil {
		return err
	}
	current, err := s.Catalog.ActiveCatalog(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return d.ErrNotReady
	}
	return d.Check(current, index)
}
