package httpapi

import (
	"context"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deadlineSearch struct {
	t     *testing.T
	short bool
}

func (f deadlineSearch) Search(ctx context.Context, _ d.Identity, _ s.Request) (d.Record, error) {
	deadline, ok := ctx.Deadline()
	remaining := time.Until(deadline)
	if !ok {
		f.t.Fatal("missing deadline")
	}
	if f.short {
		if remaining > time.Second {
			f.t.Fatal("parent deadline extended")
		}
	} else if remaining < 25*time.Second || remaining > 30*time.Second {
		f.t.Fatal("request cannot accommodate measured live latency")
	}
	return d.Record{}, d.ErrUnavailable
}
func TestSearchDeadlineAllowsLiveAndPreservesParent(t *testing.T) {
	for _, short := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader(`{"query":"photo editor","session_id":"s"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "key")
		if short {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
		New(nil, deadlineSearch{t, short}).ServeHTTP(httptest.NewRecorder(), r)
	}
}
