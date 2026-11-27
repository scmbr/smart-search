package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/option"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

func TestEmbeddingResponseValidationOrderingAndRedaction(t *testing.T) {
	for _, scenario := range []string{"valid", "duplicate index", "missing usage", "wrong dimension", "error"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				calls++
				if r.Header.Get("Authorization") != "Bearer test-secret" || r.URL.Path != "/v1/embeddings" {
					t.Error("invalid request")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != d.Model || body["dimensions"] != float64(1536) {
					t.Error("model/dimensions lost")
				}
				if scenario == "error" {
					w.WriteHeader(429)
					fmt.Fprint(w, `{"error":{"message":"test-secret should never escape","type":"rate_limit_error"}}`)
					return
				}
				v := make([]float64, 1536)
				v[0] = 1
				if scenario == "wrong dimension" {
					v = v[:2]
				}
				second := 1
				if scenario == "duplicate index" {
					second = 0
				}
				other := append([]float64(nil), v...)
				if len(other) > 1 {
					other[0], other[1] = 0, 1
				}
				data := map[string]any{"object": "list", "model": d.Model, "data": []any{map[string]any{"index": second, "embedding": other}, map[string]any{"index": 0, "embedding": v}}, "usage": map[string]int{"prompt_tokens": 2, "total_tokens": 2}}
				if scenario == "missing usage" {
					delete(data, "usage")
				}
				_ = json.NewEncoder(w).Encode(data)
			}))
			defer server.Close()
			client := newEmbedder("test-secret", option.WithBaseURL(server.URL+"/v1"))
			response, err := client.Embed(context.Background(), []string{"one", "two"})
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: %v", scenario, err)
			}
			if calls != 1 {
				t.Fatal("SDK silently retried paid request")
			}
			if err == nil && (response.Vectors[0][0] != 1 || response.Vectors[1][1] != 1) {
				t.Fatal("response index ordering lost")
			}
			if err != nil && strings.Contains(err.Error(), "test-secret") {
				t.Fatal("provider error exposed secret")
			}
		})
	}
}
