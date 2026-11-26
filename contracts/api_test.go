package contracts_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
)

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(filepath.Join("..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load OpenAPI: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI: %v", err)
	}
	return doc
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func TestOpenAPIContract(t *testing.T) {
	loadContract(t)
}

func TestSchemaExamples(t *testing.T) {
	doc := loadContract(t)
	var fixture struct {
		Synthetic bool `json:"synthetic"`
		Cases     []struct {
			Name   string `json:"name"`
			Schema string `json:"schema"`
			Valid  bool   `json:"valid"`
			Value  any    `json:"value"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join("..", "testdata", "api", "cases.json"), &fixture)
	if !fixture.Synthetic || len(fixture.Cases) == 0 {
		t.Fatal("API examples must be explicitly synthetic and nonempty")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			schema, exists := doc.Components.Schemas[tc.Schema]
			if !exists {
				t.Fatalf("schema %q does not exist", tc.Schema)
			}
			err := schema.Value.VisitJSON(tc.Value)
			if tc.Valid && err != nil {
				t.Fatalf("valid example rejected: %v", err)
			}
			if !tc.Valid && err == nil {
				t.Fatal("invalid example accepted")
			}
		})
	}
}

func TestSearchRequestContract(t *testing.T) {
	doc := loadContract(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		body  string
		key   string
		valid bool
	}{
		{"normal", `{"query":"сервер","session_id":"s-1"}`, "request-1", true},
		{"version", `{"query":"CSP 5.0","session_id":"s-1","filters":{"version":"5.0"}}`, "request-2", true},
		{"missing idempotency key", `{"query":"сервер","session_id":"s-1"}`, "", false},
		{"blank query", `{"query":"   ","session_id":"s-1"}`, "request-3", false},
		{"too long query", `{"query":"` + strings.Repeat("я", 257) + `","session_id":"s-1"}`, "request-4", false},
		{"unicode limit counts characters", `{"query":"` + strings.Repeat("я", 256) + `","session_id":"s-1"}`, "request-5", true},
		{"invalid filter", `{"query":"сервер","session_id":"s-1","filters":{"min_users":0}}`, "request-6", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/search", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.key != "" {
				req.Header.Set("Idempotency-Key", tc.key)
			}
			route, params, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("find route: %v", err)
			}
			err = openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{
				Request: req, PathParams: params, Route: route,
			})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, validation error=%v", tc.valid, err)
			}
		})
	}
}

func TestSearchResponseContract(t *testing.T) {
	doc := loadContract(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name   string          `json:"name"`
			Schema string          `json:"schema"`
			Valid  bool            `json:"valid"`
			Value  json.RawMessage `json:"value"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join("..", "testdata", "api", "cases.json"), &fixture)
	for _, tc := range fixture.Cases {
		if !tc.Valid || (tc.Schema != "SearchResponse" && tc.Name != "infrastructure failure") {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/search", nil)
			route, params, err := router.FindRoute(req)
			if err != nil {
				t.Fatal(err)
			}
			status := http.StatusOK
			if tc.Schema == "Error" {
				status = http.StatusServiceUnavailable
			}
			input := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request: req, PathParams: params, Route: route,
				},
				Status:  status,
				Header:  http.Header{"Content-Type": []string{"application/json"}},
				Body:    io.NopCloser(strings.NewReader(string(tc.Value))),
				Options: &openapi3filter.Options{IncludeResponseStatus: true},
			}
			if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
				t.Fatalf("HTTP %d response does not match contract: %v", status, err)
			}
		})
	}
}

// These invariants supplement schema validation; they do not test a search engine.
func TestSearchExampleInvariants(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name   string        `json:"name"`
			Schema string        `json:"schema"`
			Valid  bool          `json:"valid"`
			Value  searchExample `json:"value"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join("..", "testdata", "api", "cases.json"), &fixture)
	for _, tc := range fixture.Cases {
		if !tc.Valid || tc.Schema != "SearchResponse" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			checkSearchExample(t, tc.Value)
		})
	}
}

type exampleCard struct {
	FamilyID  string `json:"family_id"`
	ProductID string `json:"product_id"`
	Offer     struct {
		ID    string `json:"offer_id"`
		Price struct {
			Status string `json:"status"`
			Amount *int64 `json:"amount_minor"`
		} `json:"price"`
	} `json:"selected_offer"`
}

type searchExample struct {
	Matches         []exampleCard `json:"matches"`
	Alternatives    []exampleCard `json:"alternatives"`
	Recommendations []struct {
		Card         exampleCard `json:"card"`
		Verification struct {
			Status        string `json:"status"`
			AnchorID      string `json:"anchor_offer_id"`
			RecommendedID string `json:"recommended_offer_id"`
			Checks        []struct {
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"verification"`
	} `json:"recommendations"`
	Top4 []struct {
		Section   string `json:"section"`
		ProductID string `json:"product_id"`
		FamilyID  string `json:"family_id"`
	} `json:"top4"`
	Total     int     `json:"total_matches"`
	Fallback  bool    `json:"unpriced_fallback_used"`
	Shortfall *string `json:"shortfall_reason"`
}

func checkSearchExample(t *testing.T, response searchExample) {
	t.Helper()
	cards := make(map[string]exampleCard)
	anchors := make(map[string]bool)
	priced, unpriced := 0, 0
	for _, card := range response.Matches {
		cards["matches/"+card.ProductID] = card
		anchors[card.Offer.ID] = true
		if card.Offer.Price.Status == "known" {
			priced++
		} else {
			unpriced++
		}
	}
	for _, card := range response.Alternatives {
		cards["alternatives/"+card.ProductID] = card
		anchors[card.Offer.ID] = true
	}
	for _, recommendation := range response.Recommendations {
		card, report := recommendation.Card, recommendation.Verification
		cards["recommendations/"+card.ProductID] = card
		if report.Status != "passed" || len(report.Checks) == 0 || !anchors[report.AnchorID] || report.RecommendedID != card.Offer.ID {
			t.Errorf("recommendation verification does not match the selected pair: %s", card.ProductID)
		}
		for _, check := range report.Checks {
			if check.Status != "passed" {
				t.Errorf("unverified characteristic in %s", card.ProductID)
			}
		}
	}
	seen := make(map[string]bool)
	for _, ref := range response.Top4 {
		card, exists := cards[ref.Section+"/"+ref.ProductID]
		if !exists || card.FamilyID != ref.FamilyID {
			t.Errorf("top4 reference is unresolved: %+v", ref)
		}
		if seen[ref.FamilyID] {
			t.Errorf("top4 repeats family %s", ref.FamilyID)
		}
		seen[ref.FamilyID] = true
	}
	if len(response.Top4) < 4 && response.Shortfall == nil {
		t.Error("short top4 without explanation")
	}
	if len(response.Top4) == 4 && response.Shortfall != nil {
		t.Error("complete top4 marked as shortfall")
	}
	if response.Total < len(response.Matches) {
		t.Error("total_matches smaller than page")
	}
	if response.Fallback != (unpriced > 0) || priced > 0 && unpriced > 0 {
		t.Error("unpriced fallback must be explicit and used only without priced matches")
	}
	for _, card := range cards {
		p := card.Offer.Price
		if p.Status == "known" && (p.Amount == nil || *p.Amount <= 0) {
			t.Errorf("invalid known price: %s", card.ProductID)
		}
		if p.Status == "unknown" && p.Amount != nil {
			t.Errorf("unknown price is not null: %s", card.ProductID)
		}
	}
}
