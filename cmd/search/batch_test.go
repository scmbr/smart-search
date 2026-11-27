package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	d "github.com/scmbr/smart-search/internal/domain/search"
)

type batchBackend struct{ calls int }

func (b *batchBackend) Search(_ context.Context, r d.Request) (d.Result, error) {
	b.calls++
	return d.Result{CatalogID: "snapshot", IndexVersion: "index"}, nil
}
func TestBatchPreservesQueryOrderAndValidatesBeforeSearch(t *testing.T) {
	b := &batchBackend{}
	var output bytes.Buffer
	err := searchBatch(context.Background(), b, d.Request{Limit: 4}, strings.NewReader("\"первый запрос\"\n\"второй запрос\"\n"), &output)
	if err != nil || b.calls != 2 {
		t.Fatal(err, b.calls)
	}
	decoder := json.NewDecoder(&output)
	for _, want := range []string{"первый запрос", "второй запрос"} {
		var result d.Result
		if err = decoder.Decode(&result); err != nil || result.Query != want {
			t.Fatal(result.Query, err)
		}
	}
	for _, bad := range []string{"{}\n", "\"\"\n", strings.Repeat("x", 4097) + "\n"} {
		b.calls = 0
		if err = searchBatch(context.Background(), b, d.Request{Limit: 4}, strings.NewReader(bad), &output); err == nil || b.calls != 0 {
			t.Fatalf("accepted invalid input %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = searchBatch(ctx, b, d.Request{Limit: 4}, strings.NewReader("\"запрос\"\n"), &output); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
