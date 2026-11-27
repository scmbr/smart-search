package openai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/option"
	p "github.com/scmbr/smart-search/internal/domain/queryparser"
)

func TestParserPromptVersionIsPinned(t *testing.T) {
	sum := sha256.Sum256([]byte(parserPrompt + "\x00" + parserSchema))
	if p.PromptVersion != "parser-prompt-v1" || hex.EncodeToString(sum[:]) != "2fa0e8e3af916b0de026e2fcd07fb4481206742a2e26148709040307756298e8" {
		t.Fatal("prompt/schema changed: bump prompt version and update reviewed fingerprint")
	}
}

func TestParserSDKContractAndSafeFailures(t *testing.T) {
	query := "нужно помогать сотрудникам с их компьютерами"
	for _, scenario := range []string{"valid", "invalid", "refusal", "truncated", "missing_usage", "wrong_model", "error", "server_error"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				var body map[string]any
				if err = json.Unmarshal(raw, &body); err != nil {
					t.Error(err)
				}
				plan, _ := p.Plan(query)
				if r.URL.Path != "/v1/chat/completions" || body["model"] != p.Model || body["max_completion_tokens"] != float64(p.MaxOutputTokens) || body["reasoning_effort"] != "none" || body["service_tier"] != "default" || body["store"] != false || int64(len(raw)) > plan.InputTokenUpper {
					t.Errorf("request contract: %s", raw)
				}
				format, _ := body["response_format"].(map[string]any)
				schema, _ := format["json_schema"].(map[string]any)
				if format["type"] != "json_schema" || schema["strict"] != true || body["tools"] != nil {
					t.Error("unstructured/tools request")
				}
				if scenario == "error" || scenario == "server_error" {
					code := 429
					if scenario == "server_error" {
						code = 500
					}
					w.WriteHeader(code)
					fmt.Fprint(w, `{"error":{"message":"test-secret","type":"rate_limit_error"}}`)
					return
				}
				content := `{"version":"interpretation-v1","source_query":"` + query + `","status":"resolved","semantic_query":"программа удаленного доступа"}`
				if scenario == "invalid" {
					content = `{"offer_id":"invented"}`
				}
				message := map[string]any{"role": "assistant", "content": content}
				finish := "stop"
				if scenario == "truncated" {
					finish = "length"
				}
				if scenario == "refusal" {
					message["refusal"] = "test-secret"
				}
				response := map[string]any{"id": "test", "object": "chat.completion", "model": p.Model, "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 40, "total_tokens": 140}}
				if scenario == "missing_usage" {
					delete(response, "usage")
				}
				if scenario == "wrong_model" {
					response["model"] = "different"
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			provider := newQueryParser("test-secret", option.WithBaseURL(server.URL+"/v1"))
			response, err := provider.Generate(context.Background(), query)
			wantError := scenario == "missing_usage" || scenario == "wrong_model" || scenario == "error" || scenario == "server_error"
			if (err != nil) != wantError {
				t.Fatalf("%+v %v", response, err)
			}
			if calls != 1 {
				t.Fatal("SDK retried", calls)
			}
			if err != nil && strings.Contains(err.Error(), "test-secret") {
				t.Fatal("secret exposed")
			}
			if !wantError {
				wantFailure := ""
				switch scenario {
				case "invalid":
					wantFailure = "invalid"
				case "refusal":
					wantFailure = "refused"
				case "truncated":
					wantFailure = "truncated"
				}
				if response.Entry.Failure != wantFailure {
					t.Fatalf("%+v", response)
				}
			}
		})
	}
}
