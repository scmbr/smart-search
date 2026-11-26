// Package catalogimport orchestrates resumable snapshot imports through ports.
package catalogimport

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/scmbr/smart-search/internal/domain/catalog"
)

type Source interface {
	Identity() domain.Identity
	Read(context.Context, func(context.Context, domain.Record) error) (domain.Validation, error)
}

type Result struct {
	Snapshot domain.Snapshot
	Reused   bool
}

func Run(ctx context.Context, source Source, repo domain.Repository, batchSize int) (result Result, err error) {
	if batchSize < 1 || batchSize > 5000 {
		return result, fmt.Errorf("batch size must be 1..5000")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	session, err := repo.Begin(ctx, source.Identity())
	if err != nil {
		return result, fmt.Errorf("begin import: %w", err)
	}
	defer func() {
		result.Snapshot = session.Snapshot()
		err = errors.Join(err, session.Close())
	}()
	state := session.Snapshot()
	switch state.Status {
	case domain.Ready:
		result.Reused = true
		return result, nil
	case domain.Invalid:
		return result, domain.ErrInvalidCatalog
	case domain.Staging:
	default:
		return result, fmt.Errorf("unknown snapshot status %q", state.Status)
	}
	var seen int64
	batch := make([]domain.Record, 0, batchSize)
	flush := func(ctx context.Context) error {
		if len(batch) == 0 {
			return nil
		}
		if err := session.Save(ctx, seen-int64(len(batch))+1, batch); err != nil {
			return err
		}
		clear(batch)
		batch = batch[:0]
		return nil
	}
	v, err := source.Read(ctx, func(ctx context.Context, record domain.Record) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen++
		if seen <= state.Checkpoint {
			return nil
		}
		batch = append(batch, record)
		if len(batch) == batchSize {
			return flush(ctx)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read catalog: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if seen < state.Checkpoint {
		return result, domain.ErrCheckpoint
	}
	if v.Eligible() {
		if seen != int64(v.Categories)+int64(v.Accepted) {
			return result, domain.ErrCheckpoint
		}
		if err := flush(ctx); err != nil {
			return result, fmt.Errorf("flush catalog: %w", err)
		}
	}
	if err := session.Finish(ctx, v); err != nil {
		return result, fmt.Errorf("finish catalog: %w", err)
	}
	if !v.Eligible() {
		return result, domain.ErrInvalidCatalog
	}
	return result, nil
}
