package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
)

func TestFailureClassificationAndRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		status          int
		code, typ, want string
	}{
		{429, "rate_limit_exceeded", "rate_limit_error", "rate_limit"},
		{429, "insufficient_quota", "rate_limit_error", "quota"},
		{429, "unknown", "unknown", "uncertain"},
		{401, "", "", "rejected"}, {500, "", "", "uncertain"}, {408, "", "", "uncertain"},
	} {
		t.Run(fmt.Sprint(tc.status, tc.code), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"error":{"code":%q,"type":%q,"message":"secret"}}`, tc.code, tc.typ)
			}))
			defer server.Close()
			_, err := newEmbedder("test", option.WithBaseURL(server.URL)).Embed(context.Background(), []string{"one"})
			var failure *a.Failure
			if !errors.As(err, &failure) || failure.Kind != tc.want || failure.RetryAfter != 2*time.Second || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
	now := time.Now().UTC().Truncate(time.Second)
	if retryAfter(now.Add(3*time.Second).Format(http.TimeFormat), now) != 3*time.Second || retryAfter("invalid", now) != 0 || retryAfter("-1", now) != 0 {
		t.Fatal("Retry-After parsing")
	}
}
