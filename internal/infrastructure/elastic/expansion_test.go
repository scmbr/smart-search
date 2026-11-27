package elastic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

func TestExpandedDSLNeverFuzzesVersionsOrArticles(t *testing.T) {
	r, err := domain.Prepare(domain.Request{Query: "криптопор csp 5.0", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	exact, _ := json.Marshal(queryBody(r, true))
	if strings.Contains(string(exact), "fuzziness") {
		t.Fatal("exact stage became fuzzy")
	}
	body := searchBody(r, true, true)
	var inspect func(any)
	inspect = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if match, ok := v["multi_match"].(map[string]any); ok {
				query := match["query"].(string)
				if _, fuzzy := match["fuzziness"]; fuzzy {
					if strings.ContainsAny(query, "0123456789") {
						t.Fatalf("numeric token made fuzzy: %s", query)
					}
					if match["max_expansions"] != 3 || match["prefix_length"] != 1 {
						t.Fatal("fuzzy work unbounded")
					}
					for _, field := range match["fields"].([]string) {
						if strings.Contains(field, "VendorCode") || strings.Contains(field, "Description") {
							t.Fatal("fuzzy on article/description")
						}
					}
				}
			}
			for _, child := range v {
				inspect(child)
			}
		case []any:
			for _, child := range v {
				inspect(child)
			}
		}
	}
	inspect(body)
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"Version":"5.0"`) || strings.Contains(string(raw), "query_string") {
		t.Fatal("filter lost or input used as DSL")
	}
}

func TestSearchStagesStopOnFirstRelevantPriceTier(t *testing.T) {
	for _, scenario := range []struct {
		name                  string
		foundAt, wantRequests int
		wantMode              string
		fallback              bool
	}{
		{"exact wins", 1, 1, "exact", false},
		{"expanded priced", 2, 2, "expanded", false},
		{"exact unpriced", 3, 3, "exact", true},
		{"expanded unpriced", 4, 4, "expanded", true},
		{"empty", 0, 4, "none", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			spec := app.NewSpec(app.CatalogInfo{ID: "catalog", Offers: 1}, "test")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				var response any
				switch r.URL.Path {
				case "/_alias/test-search":
					response = map[string]any{spec.Name: map[string]any{}}
				case "/" + spec.Name + "/_mapping":
					response = map[string]any{spec.Name: map[string]any{"mappings": map[string]any{"_meta": metadata{State: "ready", Spec: spec}}}}
				case "/" + spec.Name + "/_search":
					requests++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					raw, _ := json.Marshal(body)
					if strings.Contains(string(raw), `"HasPrice":true`) != (requests <= 2) {
						t.Error("incorrect price stage order")
					}
					if strings.Contains(string(raw), `"fuzziness"`) != (requests%2 == 0) {
						t.Error("incorrect expansion order")
					}
					for _, field := range []string{"DevicesLimit", "IPAddressesLimit", "ParticipantsLimit"} {
						if !strings.Contains(string(raw), `"`+field+`":{"gte":5}`) {
							t.Errorf("%s filter lost in stage %d", field, requests)
						}
					}
					hits := []any{}
					if requests == scenario.foundAt {
						hits = append(hits, map[string]any{"_score": 1, "_source": domain.Document{OfferID: "sku", FamilyID: "family", CatalogID: "catalog"}})
					}
					response = map[string]any{"timed_out": false, "_shards": map[string]int{"failed": 0}, "hits": map[string]any{"hits": hits}}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			c, err := New(server.URL, "", "test-search")
			if err != nil {
				t.Fatal(err)
			}
			r, _ := domain.Prepare(domain.Request{Query: "product", Limit: 4, MinDevices: 5, MinIPAddresses: 5, MinParticipants: 5})
			result, err := c.Search(context.Background(), r)
			if err != nil || requests != scenario.wantRequests || result.SearchMode != scenario.wantMode || result.UnpricedFallbackUsed != scenario.fallback || len(result.SearchAttempts) != requests {
				t.Fatalf("result=%+v requests=%d err=%v", result, requests, err)
			}
		})
	}
}

func TestPrepareRecoversOnlyMatchingIndexAfterLostCreateAcknowledgement(t *testing.T) {
	for _, matching := range []bool{true, false} {
		spec := app.NewSpec(app.CatalogInfo{ID: "test-catalog", Offers: 1}, "test")
		created := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			if r.Method == "PUT" {
				created = true
				w.WriteHeader(400) // Original PUT succeeded; retry received already-exists.
				_, _ = w.Write([]byte(`{"error":{"type":"resource_already_exists_exception"}}`))
				return
			}
			if !created {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{}`))
				return
			}
			stored := spec
			if !matching {
				stored.CatalogID = "different-catalog"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{spec.Name: map[string]any{"mappings": map[string]any{"_meta": metadata{State: "staging", Spec: stored}}}})
		}))
		c, err := New(server.URL, "", "test-search")
		if err != nil {
			t.Fatal(err)
		}
		ready, err := c.Prepare(context.Background(), spec)
		server.Close()
		if ready || (err == nil) != matching {
			t.Fatalf("matching=%v ready=%v err=%v", matching, ready, err)
		}
	}
}
