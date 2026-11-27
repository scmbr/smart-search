package postgres

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/scmbr/smart-search/internal/application/catalogimport"
	a "github.com/scmbr/smart-search/internal/application/lexical"
	reader "github.com/scmbr/smart-search/internal/catalog"
	c "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"github.com/scmbr/smart-search/internal/infrastructure/feed"
)

func readIndexDocuments(t *testing.T, f a.Feed) []d.Document {
	t.Helper()
	categories := map[string]c.Category{}
	for _, category := range f.Categories() {
		categories[category.ID] = category
	}
	families, _ := d.NewFamilies(nil)
	var result []d.Document
	var after int64
	for {
		rows, err := f.Read(context.Background(), after, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			doc, err := d.BuildDocument(row.Offer, categories, families, f.Info().ID)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, doc)
			after = row.Ordinal
		}
	}
	return result
}

func TestFileIndexSourceProducesIdenticalDocumentsAndCleansPrivateFiles(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	path := "../../../testdata/catalog/minimal.xml"
	input, err := feed.Open(ctx, path, t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	imported, err := catalogimport.Run(ctx, input, New(pool), 4)
	if err != nil {
		t.Fatal(err)
	}
	database, err := NewCatalogSource(pool).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := readIndexDocuments(t, database)
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	local, err := NewCatalogFileSource(pool, path, cache).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if local.Info().ID != imported.Snapshot.ID {
		t.Fatal("snapshot changed")
	}
	got := readIndexDocuments(t, local)
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("feed and PostgreSQL index documents differ")
	}
	if current, err := local.Current(ctx); err != nil || !current {
		t.Fatal(current, err)
	}
	if err = local.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(cache)
	if err != nil || len(files) != 0 {
		t.Fatal("private spool leaked", files, err)
	}
	wrong := filepath.Join(t.TempDir(), "different.xml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(wrong, append(raw, []byte("\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = NewCatalogFileSource(pool, wrong, cache).Open(ctx); err == nil {
		t.Fatal("different SHA accepted")
	}
	files, err = os.ReadDir(cache)
	if err != nil || len(files) != 0 {
		t.Fatal("failed open leaked files", files, err)
	}
	reopened, err := NewCatalogSource(pool).Open(ctx)
	if err != nil {
		t.Fatal("failed open leaked builder lock", err)
	}
	defer reopened.Close()
}

func TestFileIndexSourceRejectsLateInvalidXMLDespiteMatchingMetadata(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cache := t.TempDir()
	raw, err := os.ReadFile("../../../testdata/catalog/minimal.xml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "late-invalid.xml")
	if err = os.WriteFile(path, append(raw, []byte("<broken")...), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := feed.Open(ctx, path, t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	identity := input.Identity()
	identity.SchemaVersion += "/" + c.NormalizationVersion
	// Deliberately inconsistent ready metadata: the adapter must still validate the entire file.
	if _, err = pool.Exec(ctx, `INSERT INTO catalog_snapshots(id,source_sha256,schema_version,status,validation) VALUES($1,$2,$3,'ready','{"Complete":true,"Accepted":10,"Categories":7}')`, identity.ID(), identity.SourceSHA256, identity.SchemaVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO catalog_active(singleton,snapshot_id) VALUES(true,$1)", identity.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = NewCatalogFileSource(pool, path, cache).Open(ctx); err == nil {
		t.Fatal("late invalid XML accepted")
	}
	files, err := os.ReadDir(cache)
	if err != nil || len(files) != 0 {
		t.Fatal("failed validation leaked spool", files, err)
	}
}

func TestIndexSpoolBoundAndCancellation(t *testing.T) {
	var output bytes.Buffer
	w := boundedIndexWriter{writer: &output, remaining: 2}
	if _, err := w.Write([]byte("abc")); err == nil || output.Len() != 0 {
		t.Fatal("spool overflow")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&fileIndexFeed{}).Read(ctx, 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
