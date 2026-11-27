package elastic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdentifierDoesNotSearchDescriptions(t *testing.T) {
	r, err := d.Prepare(d.Request{Query: "AB-1234", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(queryBody(r, true))
	if strings.Contains(string(raw), "multi_match") || strings.Contains(string(raw), "Description") {
		t.Fatal("identifier widened into product text", string(raw))
	}
}

func TestCorruptVersionIdentityRejectedBeforeSearch(t *testing.T) {
	for _, field := range []string{"catalog", "family", "normalization", "document", "old_document", "old_normalization"} {
		t.Run(field, func(t *testing.T) {
			spec := app.NewSpec(app.CatalogInfo{ID: "catalog", Offers: 1}, "families")
			stored := spec
			switch field {
			case "catalog":
				stored.CatalogID = "other"
			case "family":
				stored.FamilyVersion = "other"
			case "normalization":
				stored.NormalizationVersion = "other"
			case "document":
				stored.DocumentVersion = "other"
			case "old_document":
				stored.DocumentVersion = "lexical-old"
			case "old_normalization":
				stored.NormalizationVersion = "characteristics-old"
			}
			physical := spec.Name
			if strings.HasPrefix(field, "old_") {
				sum := sha256.Sum256([]byte(strings.Join([]string{stored.CatalogID, stored.NormalizationVersion, stored.FamilyVersion, stored.DocumentVersion}, "\x00")))
				stored.Fingerprint = hex.EncodeToString(sum[:])
				stored.Name = "smart-search-" + stored.Fingerprint
				physical = stored.Name
				if !stored.ValidIdentity() {
					t.Fatal("invalid historical fixture")
				}
			}
			searches := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				switch r.URL.Path {
				case "/_alias/test-recovery":
					json.NewEncoder(w).Encode(map[string]any{physical: map[string]any{}})
				case "/" + physical + "/_mapping":
					json.NewEncoder(w).Encode(map[string]any{physical: map[string]any{"mappings": map[string]any{"_meta": metadata{State: "ready", Spec: stored}}}})
				default:
					searches++
					w.Write([]byte(`{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[]}}`))
				}
			}))
			defer server.Close()
			c, err := New(server.URL, "", "test-recovery")
			if err != nil {
				t.Fatal(err)
			}
			r, _ := d.Prepare(d.Request{Query: "товар", Limit: 4})
			if _, err = c.Search(context.Background(), r); err == nil || searches != 0 {
				t.Fatalf("corrupt %s accepted: %v searches=%d", field, err, searches)
			}
			if _, err = c.Resolve(context.Background(), r, []d.GroupCandidate{{GroupID: "g", Vendor: "vendor", Score: .8}}, true, stored.CatalogID, strings.TrimPrefix(physical, "smart-search-")); err == nil || searches != 0 {
				t.Fatalf("invalid resolve accepted: %v searches=%d", err, searches)
			}
		})
	}
}

func TestPartialSearchIsFailure(t *testing.T) {
	for _, body := range []string{`{"timed_out":true,"_shards":{"failed":0},"hits":{"hits":[]}}`, `{"timed_out":false,"_shards":{"failed":1},"hits":{"hits":[]}}`, `{"hits":{"hits":[]}}`, `{}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.Write([]byte(body))
		}))
		c, err := New(server.URL, "", "test-recovery")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.searchHits(context.Background(), "test-index", map[string]any{})
		server.Close()
		if !errors.Is(err, d.ErrUnavailable) {
			t.Fatalf("partial response accepted: %s %v", body, err)
		}
	}
}
