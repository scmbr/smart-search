// Package openai adapts the official SDK without exposing SDK types to inner layers.
package openai

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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
			kind := "uncertain"
			if api.StatusCode == 429 {
				if api.Code == "insufficient_quota" || api.Type == "insufficient_quota" {
					kind = "quota"
				} else if api.Code == "rate_limit_exceeded" || (api.Code == "" && api.Type == "rate_limit_error") {
					kind = "rate_limit"
				}
			} else if api.StatusCode >= 400 && api.StatusCode < 500 && api.StatusCode != 408 {
				kind = "rejected"
			}
			var after time.Duration
			if api.Response != nil {
				after = retryAfter(api.Response.Header.Get("Retry-After"), time.Now())
			}
			return a.Response{}, &a.Failure{Kind: kind, RetryAfter: after}
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

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > 3600 {
			return time.Hour
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
