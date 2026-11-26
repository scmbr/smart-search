package elastic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func TestQueryUsesLiteralTextAndFiltersBeforeCollapse(t *testing.T) {
	r, _ := domain.Prepare(domain.Request{Query: `literal "query"`, Limit: 4, Version: "5.0", MinUsers: 5})
	body, err := json.Marshal(queryBody(r, true))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, part := range []string{`"collapse":{"field":"FamilyID"}`, `"HasPrice":true`, `"Version":"5.0"`, `"UsersLimit":{"gte":5}`, `"RoleConflict":false`} {
		if !strings.Contains(s, part) {
			t.Fatalf("missing %s in %s", part, s)
		}
	}
	if strings.Contains(s, "query_string") {
		t.Fatal("input interpreted as DSL")
	}
}

func TestBulkItemFailureIsNotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.URL.Path != "/_bulk" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"errors":true}`))
	}))
	defer server.Close()
	c, err := New(server.URL, "", "test-search")
	if err != nil {
		t.Fatal(err)
	}
	spec := app.NewSpec(app.CatalogInfo{ID: "snapshot", Offers: 1}, "families")
	if err := c.Write(context.Background(), spec.Name, []domain.Document{{OfferID: "a"}}); err == nil {
		t.Fatal("bulk failure ignored")
	}
}

func TestMissingAliasAndServerFailuresAreExplicit(t *testing.T) {
	for _, code := range []int{404, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{}`))
		}))
		c, err := New(server.URL, "", "test-search")
		if err != nil {
			t.Fatal(err)
		}
		r, _ := domain.Prepare(domain.Request{Query: "product", Limit: 4})
		_, err = c.Search(context.Background(), r)
		if code == 404 && !errors.Is(err, domain.ErrNotReady) {
			t.Fatalf("404=%v", err)
		}
		if code == 503 && !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("503=%v", err)
		}
		server.Close()
	}
}
