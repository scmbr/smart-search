package names

import (
	"context"
	"fmt"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

type Backend interface {
	Search(context.Context, d.Request) (d.Result, error)
}

// SearchBackend uses the strict lexical port for a resolved name and keeps the
// original configured backend for task queries. It accepts prepared requests.
type SearchBackend struct {
	Dictionary      *d.NameDictionary
	CatalogID       string
	Groups          map[d.NameEntity][]string
	Named, Fallback Backend
}

func (b SearchBackend) Search(ctx context.Context, r d.Request) (d.Result, error) {
	if err := ctx.Err(); err != nil {
		return d.Result{}, err
	}
	if b.Dictionary == nil || b.Named == nil || b.Fallback == nil {
		return d.Result{}, d.ErrNotReady
	}
	resolution := b.Dictionary.ResolveName(r.Query)
	if resolution.Status != "resolved" {
		return b.Fallback.Search(ctx, r)
	}
	entity := resolution.Candidates[0].Entity
	selection := d.NameSelection{CatalogID: b.CatalogID, DictionarySHA: b.Dictionary.Fingerprint(), Entity: entity, Groups: b.Groups[entity]}
	if entity.Kind == "product" && len(selection.Groups) == 0 {
		return d.Result{}, d.ErrNotReady
	}
	result, err := b.Named.Search(ctx, d.SelectName(r, selection))
	if ctx.Err() != nil {
		return d.Result{}, ctx.Err()
	}
	if err != nil {
		return d.Result{}, err
	}
	if result.CatalogID != b.CatalogID {
		return d.Result{}, d.ErrNotReady
	}
	for _, hit := range result.Hits {
		if !d.NameAllowed(&selection, hit.Document) {
			return d.Result{}, fmt.Errorf("%w: named result scope mismatch", d.ErrUnavailable)
		}
	}
	return result, nil
}
