// Package searchsession executes a search once per persistent idempotency key.
package searchsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"time"
)

type Lease interface {
	Cached() *d.Record
	Complete(context.Context, d.Record) error
	Close() error
}
type Store interface {
	Begin(context.Context, d.Identity) (Lease, error)
}
type Engine func(context.Context, s.Request) (s.Result, error)
type Service struct {
	Store  Store
	Engine Engine
}

func (service Service) Search(ctx context.Context, id d.Identity, request s.Request) (d.Record, error) {
	if err := id.Validate(); err != nil {
		return d.Record{}, err
	}
	prepared, err := s.Prepare(request)
	if err != nil || len(prepared.UnsupportedConstraints) > 0 {
		return d.Record{}, d.ErrInvalid
	}
	if ctx.Err() != nil {
		return d.Record{}, ctx.Err()
	}
	if service.Store == nil || service.Engine == nil {
		return d.Record{}, d.ErrUnavailable
	}
	lease, err := service.Store.Begin(ctx, id)
	if err != nil {
		if errors.Is(err, d.ErrConflict) {
			return d.Record{}, d.ErrConflict
		}
		return d.Record{}, d.ErrUnavailable
	}
	if lease == nil {
		return d.Record{}, d.ErrUnavailable
	}
	defer lease.Close()
	if ctx.Err() != nil {
		return d.Record{}, ctx.Err()
	}
	if record := lease.Cached(); record != nil {
		return *record, nil
	}
	started := time.Now()
	result, err := service.Engine(ctx, request)
	if ctx.Err() != nil {
		return d.Record{}, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, s.ErrInvalidQuery) {
			return d.Record{}, d.ErrInvalid
		}
		return d.Record{}, d.ErrUnavailable
	}
	if result.CatalogID == "" || result.IndexVersion == "" {
		return d.Record{}, d.ErrUnavailable
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return d.Record{}, d.ErrUnavailable
	}
	record := d.Record{ID: hex.EncodeToString(token[:]), DurationMS: time.Since(started).Milliseconds(), Result: result}
	if err = lease.Complete(ctx, record); err != nil {
		return d.Record{}, d.ErrUnavailable
	}
	return record, nil
}
