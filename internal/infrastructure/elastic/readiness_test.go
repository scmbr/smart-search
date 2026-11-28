package elastic

import (
	"context"
	"encoding/json"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadinessRequiresSearchableReadyIndex(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "failed shard"}[failed], func(t *testing.T) {
			spec := app.NewSpec(app.CatalogInfo{ID: "catalog", Offers: 1}, "families")
			searches := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				switch r.URL.Path {
				case "/_alias/test-health":
					json.NewEncoder(w).Encode(map[string]any{spec.Name: map[string]any{}})
				case "/" + spec.Name + "/_mapping":
					json.NewEncoder(w).Encode(map[string]any{spec.Name: map[string]any{"mappings": map[string]any{"_meta": metadata{State: "ready", Spec: spec}}}})
				case "/" + spec.Name + "/_search":
					searches++
					shards := 0
					if failed {
						shards = 1
					}
					json.NewEncoder(w).Encode(map[string]any{"timed_out": false, "_shards": map[string]int{"failed": shards}, "hits": map[string]any{"hits": []any{}}})
				default:
					t.Error("unexpected path", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer s.Close()
			c, err := New(s.URL, "", "test-health")
			if err != nil {
				t.Fatal(err)
			}
			state, err := c.ActiveIndex(context.Background())
			if searches != 1 || (state.Ready && err == nil) == failed {
				t.Fatalf("state=%+v err=%v searches=%d", state, err, searches)
			}
		})
	}
}
