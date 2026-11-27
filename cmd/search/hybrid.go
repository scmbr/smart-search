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
	provider "github.com/scmbr/smart-search/internal/infrastructure/openai"
	"github.com/scmbr/smart-search/internal/infrastructure/postgres"
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

func fullHybrid(ctx context.Context, s localconfig.Settings, backend *elastic.Client, file string, live bool) (hybrid.Backend, func(), error) {
	var result hybrid.Backend
	x, err := vectorcache.LoadManifest(file)
	if err != nil {
		return result, nil, err
	}
	var p embedding.Provider
	if live {
		if os.Getenv("OPENAI_LIVE_ENABLED") != "true" || os.Getenv("OPENAI_API_KEY") == "" {
			return result, nil, errors.New("live-query requires OPENAI_LIVE_ENABLED=true and API key")
		}
		if model := os.Getenv("OPENAI_EMBEDDING_MODEL"); model != "" && model != x.Model {
			return result, nil, errors.New("query embedding model differs from manifest")
		}
		if budget := os.Getenv("OPENAI_BUDGET_USD"); budget != "" && budget != "5" && budget != "5.00" {
			return result, nil, errors.New("query budget differs from 5 USD")
		}
		p = provider.New(os.Getenv("OPENAI_API_KEY"))
	}
	pool, err := s.Pool(ctx)
	if err != nil {
		return result, nil, err
	}
	var catalogID, schema string
	err = pool.QueryRow(ctx, `SELECT s.id,s.schema_version FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready' AND s.source_sha256=$1`, x.SourceSHA).Scan(&catalogID, &schema)
	if err != nil || !strings.HasSuffix(schema, "/"+catalog.NormalizationVersion) {
		pool.Close()
		return result, nil, errors.New("vector corpus does not match active normalized feed")
	}
	vectors, err := vectorcache.NewFull(x, embedding.Runtime{Store: postgres.NewEmbeddingStore(pool), Provider: p}, live)
	if err != nil {
		pool.Close()
		return result, nil, err
	}
	return hybrid.Backend{Lexical: backend, Vectors: vectors, ExpectedCatalog: catalogID, CorpusSHA: x.SHA, CorpusDocuments: len(x.Documents)}, pool.Close, nil
}
