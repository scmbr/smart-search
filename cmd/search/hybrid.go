package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	embedding "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	hybrid "github.com/scmbr/smart-search/internal/application/hybrid"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
	"github.com/scmbr/smart-search/internal/infrastructure/elastic"
	"github.com/scmbr/smart-search/internal/infrastructure/localconfig"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
	"github.com/scmbr/smart-search/internal/infrastructure/searchruntime"
	"github.com/scmbr/smart-search/internal/infrastructure/vectorcache"
)

func cachedHybrid(ctx context.Context, s localconfig.Settings, backend *elastic.Client, file string) (hybrid.Backend, func(), error) {
	var result hybrid.Backend
	f, err := os.Open(file)
	if err != nil {
		return result, nil, errors.New("cannot open hybrid development corpus")
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) > 4*1024*1024 {
		return result, nil, errors.New("cannot read bounded hybrid corpus")
	}
	var x d.Dataset
	if err = json.Unmarshal(raw, &x); err != nil {
		return result, nil, errors.New("invalid hybrid corpus JSON")
	}
	if err = d.Validate(x); err != nil {
		return result, nil, err
	}
	pool, err := s.Pool(ctx)
	if err != nil {
		return result, nil, err
	}
	var catalogID, schema string
	err = pool.QueryRow(ctx, `SELECT s.id,s.schema_version FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready' AND s.source_sha256=$1`, x.SourceSHA).Scan(&catalogID, &schema)
	if err != nil || !strings.HasSuffix(schema, "/"+catalog.NormalizationVersion) {
		pool.Close()
		return result, nil, errors.New("hybrid corpus does not match active feed/current normalization")
	}
	vectors, err := vectorcache.New(x, embedding.Runtime{Store: postgres.NewEmbeddingStore(pool)})
	if err != nil {
		pool.Close()
		return result, nil, err
	}
	result = hybrid.Backend{Lexical: backend, Vectors: vectors, ExpectedCatalog: catalogID, CorpusSHA: d.Fingerprint(x), CorpusDocuments: len(x.Documents)}
	return result, pool.Close, nil
}

func fullHybrid(ctx context.Context, s localconfig.Settings, backend *elastic.Client, file string, live bool, parserMode string) (hybrid.Backend, func(), error) {
	return searchruntime.NewFull(ctx, s, backend, searchruntime.Options{Corpus: file, LiveQuery: live, ParserMode: parserMode})
}
