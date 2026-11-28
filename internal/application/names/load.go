// Package names loads a complete snapshot-owned catalog vocabulary.
package names

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

type Snapshot struct {
	CatalogID string
	Entities  []d.NameEntity
}
type Source interface {
	LoadNames(context.Context) (Snapshot, error)
}

func Load(ctx context.Context, s Source, aliases []d.NameAlias) (*d.NameDictionary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("missing catalog names source")
	}
	snapshot, err := s.LoadNames(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if len(snapshot.Entities) == 0 {
		return nil, errors.New("empty catalog names")
	}
	return d.NewNameDictionary(snapshot.CatalogID, snapshot.Entities, aliases)
}
