// Package openai adapts the official SDK without exposing SDK types to inner layers.
package openai

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

type Embedder struct{ client sdk.Client }

func New(key string) *Embedder { return newEmbedder(key) }
func newEmbedder(key string, opts ...option.RequestOption) *Embedder {
	base := []option.RequestOption{option.WithAPIKey(key), option.WithBaseURL("https://api.openai.com/v1"), option.WithMaxRetries(0), option.WithRequestTimeout(30 * time.Second)}
	return &Embedder{client: sdk.NewClient(append(base, opts...)...)}
}
func (e *Embedder) Embed(ctx context.Context, texts []string) (a.Response, error) {
	if len(texts) < 1 || len(texts) > 32 {
		return a.Response{}, errors.New("invalid embedding batch")
	}
	for _, text := range texts {
		if _, err := d.TokenBound(text); err != nil {
			return a.Response{}, err
		}
	}
	r, err := e.client.Embeddings.New(ctx, sdk.EmbeddingNewParams{Model: d.Model, Dimensions: sdk.Int(d.Dimensions), EncodingFormat: sdk.EmbeddingNewParamsEncodingFormatFloat, Input: sdk.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts}})
	if err != nil {
		var api *sdk.Error
		if errors.As(err, &api) {
			return a.Response{}, fmt.Errorf("embedding provider HTTP %d (details redacted)", api.StatusCode)
		}
		return a.Response{}, errors.New("embedding provider unavailable (details redacted)")
	}
	if r == nil || r.Model != d.Model || len(r.Data) != len(texts) || !r.JSON.Usage.Valid() || !r.Usage.JSON.TotalTokens.Valid() || r.Usage.TotalTokens != r.Usage.PromptTokens {
		return a.Response{}, errors.New("invalid embedding response metadata")
	}
	out := a.Response{Model: r.Model, Tokens: r.Usage.TotalTokens, Vectors: make([][]float64, len(texts))}
	seen := map[int64]bool{}
	for _, v := range r.Data {
		if !v.JSON.Index.Valid() || v.Index < 0 || v.Index >= int64(len(texts)) || seen[v.Index] {
			return a.Response{}, errors.New("invalid embedding response index")
		}
		seen[v.Index] = true
		if err = d.ValidateVector(v.Embedding); err != nil {
			return a.Response{}, err
		}
		out.Vectors[v.Index] = v.Embedding
	}
	return out, nil
}
