package elastic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

func TestPilotCleanupRefusesForeignOwner(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			w.WriteHeader(400)
			return
		}
		if r.Method == "DELETE" {
			deleted = true
			return
		}
		name := r.URL.Path[1 : len(r.URL.Path)-len("/_mapping")]
		_ = json.NewEncoder(w).Encode(map[string]any{name: map[string]any{"mappings": map[string]any{"_meta": map[string]string{"Owner": "foreign"}}}})
	}))
	defer server.Close()
	c, err := New(server.URL, "", "test-search")
	if err != nil {
		t.Fatal(err)
	}
	x := d.Dataset{Version: d.TextVersion, Role: "development_tuning_pilot", SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Documents: []d.Document{{ID: "v/a", GroupID: "a", Vendor: "v", Text: "text"}}, Queries: []d.Query{{ID: "probe-01", Text: "task", Scope: "family", Split: "pilot_probe", Groups: []string{"a"}}}}
	if _, err := (PilotRank{Client: c}).Rank(context.Background(), x); err == nil || deleted {
		t.Fatal("foreign pilot index deleted or accepted", err)
	}
}

func TestPilotBM25IsIsolatedAndOwnedIndexIsRemoved(t *testing.T) {
	endpoint := os.Getenv("TEST_ELASTICSEARCH_URL")
	if endpoint == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set")
	}
	c, err := New(endpoint, "", "smart-search-active")
	if err != nil {
		t.Fatal(err)
	}
	x := d.Dataset{Version: d.TextVersion, Role: "development_tuning_pilot", SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Documents: []d.Document{{ID: "v/a", GroupID: "a", Vendor: "v", Text: "резервное копирование серверов"}, {ID: "v/b", GroupID: "b", Vendor: "v", Text: "почтовые сообщения"}}, Queries: []d.Query{{ID: "probe-01", Text: "резервные копии сервера", Split: "pilot_probe", Scope: "family", Groups: []string{"a"}}}}
	r, err := (PilotRank{Client: c}).Rank(context.Background(), x)
	if err != nil || len(r) != 1 || len(r[0]) == 0 || r[0][0].ID != "v/a" {
		t.Fatalf("ranks=%+v err=%v", r, err)
	}
}
