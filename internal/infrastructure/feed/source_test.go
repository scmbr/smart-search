package feed

import (
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	reader "github.com/scmbr/smart-search/internal/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/catalog"
)

func TestImmutableCopyAndMapping(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile("../../../testdata/catalog/minimal.xml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	original := filepath.Join(dir, "input.xml")
	if err := os.WriteFile(original, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, original, dir, reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	path := s.file.Name()
	if err := os.WriteFile(original, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	var first []domain.Record
	v, err := s.Read(ctx, func(_ context.Context, r domain.Record) error { first = append(first, r); return nil })
	if err != nil || !v.Eligible() || v.Accepted != 10 || v.Categories != 7 {
		t.Fatalf("read: %+v %v", v, err)
	}
	var second []domain.Record
	if _, err := s.Read(ctx, func(_ context.Context, r domain.Record) error { second = append(second, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("replay differs")
	}
	var unknown, repeats bool
	for _, r := range first {
		if r.Offer == nil {
			continue
		}
		if r.Offer.PriceMinor == nil && !r.Offer.Available {
			unknown = true
		}
		if len(r.Offer.Parameters) > 1 {
			repeats = true
		}
		if r.Offer.Attributes == nil || r.Offer.RawFields == nil {
			t.Fatal("lost source fields")
		}
	}
	if !unknown || !repeats {
		t.Fatal("lost price or parameters")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("spool retained")
	}
}

func TestSourceOptionsAndCancellation(t *testing.T) {
	path := "../../../testdata/catalog/minimal.xml"
	a, err := Open(context.Background(), path, t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(context.Background(), path, t.TempDir(), reader.Options{MaxIssues: 101})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.Identity().ID() == b.Identity().ID() {
		t.Fatal("options not versioned")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, path, t.TempDir(), reader.Options{}); err == nil {
		t.Fatal("ignored cancel")
	}
	if _, err := Open(context.Background(), path, t.TempDir(), reader.Options{MaxIssues: -1}); err == nil {
		t.Fatal("negative limit")
	}
}

func TestGzipIdentityAndSinkFailure(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile("../../../testdata/catalog/minimal.xml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "compressed.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	if _, err := z.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path, t.TempDir(), reader.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := s.Read(ctx, func(context.Context, domain.Record) error { return nil })
	if err != nil || !v.Eligible() {
		t.Fatalf("gzip: %+v %v", v, err)
	}
	sentinel := errors.New("sink disconnected")
	v, err = s.Read(ctx, func(context.Context, domain.Record) error { return sentinel })
	if !errors.Is(err, sentinel) || v.Eligible() {
		t.Fatalf("sink error: %+v %v", v, err)
	}
}
