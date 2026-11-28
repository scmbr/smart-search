package postgres

import (
	"context"
	"errors"
	app "github.com/scmbr/smart-search/internal/application/names"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"net/url"
	"strings"
)

// LoadNames reuses the pinned catalog reader and rejects a changed active snapshot.
// It only uses own product names and vendors, never descriptions or search labels.
func (s *CatalogSource) LoadNames(ctx context.Context) (result app.Snapshot, err error) {
	f, err := s.Open(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	result.CatalogID = f.Info().ID
	categories := map[string]string{}
	for _, c := range f.Categories() {
		categories[c.ID] = c.Name
	}
	seen := map[d.NameEntity]bool{}
	add := func(e d.NameEntity) {
		if strings.TrimSpace(e.Name) != "" && !seen[e] {
			seen[e] = true
			result.Entities = append(result.Entities, e)
		}
	}
	var after int64
	for {
		rows, e := f.Read(ctx, after, 1000)
		if e != nil {
			return app.Snapshot{}, e
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if r.Ordinal <= after {
				return app.Snapshot{}, errors.New("nonmonotonic catalog names page")
			}
			after = r.Ordinal
			u, e := url.Parse(r.Offer.URL)
			if e != nil || u.Scheme != "https" || u.Host != "store.softline.ru" {
				continue
			}
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) < 2 || parts[0] == "" {
				continue
			}
			scope := "/" + parts[0] + "/"
			add(d.NameEntity{Kind: "brand", Name: r.Offer.Vendor, Scope: scope})
			if name := categories[r.Offer.GroupID]; name != "" {
				add(d.NameEntity{Kind: "product", Name: name, Scope: scope})
			}
		}
	}
	current, e := f.Current(ctx)
	if e != nil {
		return app.Snapshot{}, e
	}
	if !current {
		return app.Snapshot{}, errors.New("catalog changed while loading names")
	}
	return result, nil
}
