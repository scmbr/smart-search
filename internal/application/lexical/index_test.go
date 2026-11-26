package lexical

import (
	"context"
	"errors"
	"testing"

	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

type testFeed struct {
	rows             []Row
	closed, outdated bool
}

func (f *testFeed) Info() CatalogInfo {
	return CatalogInfo{ID: "snapshot", SchemaVersion: "reader/" + catalog.NormalizationVersion, Offers: len(f.rows)}
}
func (f *testFeed) Categories() []catalog.Category {
	return []catalog.Category{{ID: "c", Name: "Catalog"}}
}
func (f *testFeed) Read(_ context.Context, after int64, size int) ([]Row, error) {
	var rows []Row
	for _, row := range f.rows {
		if row.Ordinal > after {
			rows = append(rows, row)
			if len(rows) == size {
				break
			}
		}
	}
	return rows, nil
}
func (f *testFeed) Current(context.Context) (bool, error) { return !f.outdated, nil }
func (f *testFeed) Close() error                          { f.closed = true; return nil }
func (f *testFeed) Open(context.Context) (Feed, error)    { return f, nil }

type testTarget struct {
	docs                                     []domain.Document
	writeFail, finishFail, reused, activated bool
}

func (f *testTarget) Prepare(context.Context, Spec) (bool, error) { return f.reused, nil }
func (f *testTarget) Write(_ context.Context, _ string, docs []domain.Document) error {
	if f.writeFail {
		return errors.New("bulk failed")
	}
	f.docs = append(f.docs, docs...)
	return nil
}
func (f *testTarget) Finish(context.Context, Spec) error {
	if f.finishFail {
		return errors.New("count mismatch")
	}
	return nil
}
func (f *testTarget) Activate(context.Context, Spec) error { f.activated = true; return nil }
func fixtureFeed() *testFeed {
	f := &testFeed{}
	for _, id := range []string{"a", "b", "c"} {
		o := catalog.Offer{ID: id, CategoryID: "c", Name: id, URL: "https://store.softline.ru/v/" + id}
		c := catalog.Normalize(o)
		o.Characteristics = &c
		f.rows = append(f.rows, Row{Ordinal: int64(len(f.rows) + 2), Offer: o})
	}
	return f
}

func TestBuildBatchesAndVersion(t *testing.T) {
	f := fixtureFeed()
	target := &testTarget{}
	families, _ := domain.NewFamilies(nil)
	r, err := Build(context.Background(), f, target, families, "families-v1", 2)
	if err != nil || !target.activated || !f.closed || len(target.docs) != 3 || r.Spec.Count != 3 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	changed := NewSpec(f.Info(), "families-v2")
	if changed.Name == r.Spec.Name {
		t.Fatal("mapping version absent from index identity")
	}
}

func TestBuildFailureKeepsAlias(t *testing.T) {
	for _, kind := range []string{"bulk", "count", "catalog changed"} {
		t.Run(kind, func(t *testing.T) {
			f := fixtureFeed()
			target := &testTarget{}
			switch kind {
			case "bulk":
				target.writeFail = true
			case "count":
				target.finishFail = true
			case "catalog changed":
				f.outdated = true
			}
			families, _ := domain.NewFamilies(nil)
			if _, err := Build(context.Background(), f, target, families, "v1", 2); err == nil || target.activated || !f.closed {
				t.Fatalf("failure activated index: %v", err)
			}
		})
	}
}

func TestReadyIndexReuse(t *testing.T) {
	f := fixtureFeed()
	target := &testTarget{reused: true}
	families, _ := domain.NewFamilies(nil)
	r, err := Build(context.Background(), f, target, families, "v1", 2)
	if err != nil || !r.Reused || !target.activated || len(target.docs) != 0 {
		t.Fatalf("reuse=%+v err=%v", r, err)
	}
}
