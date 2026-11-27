// Package lexical orchestrates versioned index preparation through external ports.
package lexical

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

const DocumentSchemaVersion = "lexical-v6"

var ErrCatalogChanged = errors.New("active catalog changed during index build")

type CatalogInfo struct {
	ID, SchemaVersion string
	Offers            int
}
type Row struct {
	Ordinal int64
	Offer   catalog.Offer
}
type Source interface {
	Open(context.Context) (Feed, error)
}
type Feed interface {
	Info() CatalogInfo
	Categories() []catalog.Category
	Read(ctx context.Context, after int64, size int) ([]Row, error)
	Current(context.Context) (bool, error)
	Close() error
}
type Spec struct {
	CatalogID, Name, Fingerprint, NormalizationVersion, FamilyVersion, DocumentVersion string
	Count                                                                              int
}
type Target interface {
	Prepare(context.Context, Spec) (bool, error)
	Write(context.Context, string, []domain.Document) error
	Finish(context.Context, Spec) error
	Activate(context.Context, Spec) error
}
type BuildResult struct {
	Spec              Spec
	Reused, Activated bool
}

func NewSpec(info CatalogInfo, familyVersion string) Spec {
	h := sha256.Sum256([]byte(info.ID + "\x00" + catalog.NormalizationVersion + "\x00" + familyVersion + "\x00" + DocumentSchemaVersion))
	fingerprint := hex.EncodeToString(h[:])
	return Spec{CatalogID: info.ID, Name: "smart-search-" + fingerprint, Fingerprint: fingerprint, NormalizationVersion: catalog.NormalizationVersion, FamilyVersion: familyVersion, DocumentVersion: DocumentSchemaVersion, Count: info.Offers}
}

func Build(ctx context.Context, source Source, target Target, families *domain.Families, familyVersion string, batchSize int) (result BuildResult, err error) {
	if batchSize < 1 || batchSize > 1000 || families == nil || familyVersion == "" {
		return result, errors.New("invalid index build options")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	feed, err := source.Open(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, feed.Close()) }()
	info := feed.Info()
	if info.ID == "" || info.Offers < 1 || !strings.HasSuffix(info.SchemaVersion, "/"+catalog.NormalizationVersion) {
		return result, errors.New("catalog requires current characteristics policy")
	}
	result.Spec = NewSpec(info, familyVersion)
	categories := make(map[string]catalog.Category)
	for _, category := range feed.Categories() {
		if _, exists := categories[category.ID]; exists {
			return result, errors.New("duplicate category in index source")
		}
		categories[category.ID] = category
	}
	result.Reused, err = target.Prepare(ctx, result.Spec)
	if err != nil {
		return result, err
	}
	if !result.Reused {
		var after int64
		count := 0
		for {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			rows, err := feed.Read(ctx, after, batchSize)
			if err != nil {
				return result, err
			}
			if len(rows) == 0 {
				break
			}
			if len(rows) > batchSize {
				return result, errors.New("source returned oversized batch")
			}
			docs := make([]domain.Document, 0, len(rows))
			for _, row := range rows {
				if row.Ordinal <= after {
					return result, errors.New("nonmonotonic catalog cursor")
				}
				after = row.Ordinal
				d, err := domain.BuildDocument(row.Offer, categories, families, info.ID)
				if err != nil {
					return result, fmt.Errorf("offer %s: %w", row.Offer.ID, err)
				}
				docs = append(docs, d)
			}
			if err := target.Write(ctx, result.Spec.Name, docs); err != nil {
				return result, err
			}
			count += len(docs)
			if count > info.Offers {
				return result, errors.New("index source count exceeded catalog count")
			}
		}
		if count != info.Offers {
			return result, errors.New("incomplete index source")
		}
	}
	if err := target.Finish(ctx, result.Spec); err != nil {
		return result, err
	}
	current, err := feed.Current(ctx)
	if err != nil {
		return result, err
	}
	if !current {
		return result, ErrCatalogChanged
	}
	if err := target.Activate(ctx, result.Spec); err != nil {
		return result, err
	}
	result.Activated = true
	return result, nil
}
