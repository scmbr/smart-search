package httpapi

import (
	"context"
	"encoding/json"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"net/http/httptest"
	"strings"
	"testing"
)

type searchFake struct {
	calls  int
	hash   string
	result d.Record
	err    error
}

func (f *searchFake) Search(_ context.Context, id d.Identity, r s.Request) (d.Record, error) {
	f.calls++
	f.hash = id.BodyHash
	return f.result, f.err
}
func TestSearchRequestRejectsUnsupportedAndInvalidBeforeBackend(t *testing.T) {
	for _, body := range []string{
		`{"query":"server","session_id":"s","unknown":1}`,
		`{"query":"server","session_id":"s","query|session_id":"unknown"}`,
		`{"query":"server","query":"other","session_id":"s"}`,
		`{"query":"server","session_id":"s","limit":null}`,
		`{"Query":"server","session_id":"s"}`,
		`{"query":"server","session_id":"s","context":{"segment":"enterprise"}}`,
		`{"query":"server","session_id":"s","filters":{"license_type":"OEM"}}`,
		`{"query":"server","session_id":"s","cursor":"page"}`,
		`{"query":"server","session_id":"s","limit":3}`,
		`{"query":"server","session_id":"s","filters":{"min_users":0}}`,
	} {
		f := &searchFake{}
		handler := New(nil, f)
		r := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "k")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 || f.calls != 0 {
			t.Fatalf("body=%s code=%d calls=%d", body, w.Code, f.calls)
		}
	}
}
func TestSearchMapsCatalogAndCanonicalizesBody(t *testing.T) {
	price := int64(500)
	f := &searchFake{result: d.Record{ID: "search", Result: s.Result{Query: "product", CatalogID: "catalog", IndexVersion: "index", Hits: []s.Hit{{Document: s.Document{OfferID: "offer", GroupID: "g", FamilyID: "family", FamilyConfirmed: true, ProductName: "Product", Name: "License", CategoryID: "cat", URL: "https://store.softline.ru/vendor/offer/", OfferKind: "new_license", PriceMinor: &price}}}}}}
	var hash string
	for _, body := range []string{`{"query":"product","session_id":"s"}`, `{ "session_id":"s", "query":"product" }`} {
		r := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "k")
		w := httptest.NewRecorder()
		New(nil, f).ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if hash != "" && hash != f.hash {
			t.Fatal("JSON order changed canonical hash")
		}
		hash = f.hash
		var response map[string]any
		if json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatal("invalid JSON")
		}
		if response["total_matches"] != nil || len(response["top4"].([]any)) != 1 || response["shortfall_reason"] == nil {
			t.Fatal(response)
		}
	}
}

func TestSearchRejectsOversizeAndMissingKey(t *testing.T) {
	f := &searchFake{}
	for _, tc := range []struct {
		body, key string
		status    int
	}{
		{strings.Repeat(" ", 16385), "k", 413},
		{`{"query":"product","session_id":"s"}`, "", 400},
	} {
		r := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		if tc.key != "" {
			r.Header.Set("Idempotency-Key", tc.key)
		}
		w := httptest.NewRecorder()
		New(nil, f).ServeHTTP(w, r)
		if w.Code != tc.status || f.calls != 0 {
			t.Fatal(w.Code, tc.status, f.calls)
		}
	}
}
func TestUnconfirmedFamilyIsNotPublishedInTop4(t *testing.T) {
	price := int64(100)
	result := d.Record{ID: "search", Result: s.Result{Query: "product", CatalogID: "catalog", IndexVersion: "index", Hits: []s.Hit{{Document: s.Document{OfferID: "offer", FamilyID: "provisional", CategoryID: "cat", Name: "License", URL: "https://store.softline.ru/vendor/offer/", OfferKind: "unknown", PriceMinor: &price}}}}}
	response, err := mapSearch(result)
	if err != nil || len(response.Matches) != 1 || response.Matches[0].Confirmed || len(response.Top4) != 0 || response.Shortfall == nil {
		t.Fatal(response, err)
	}
}

func TestUnicodeEscapesPreserveSearchIdentity(t *testing.T) {
	a, ar, err := decodeSearch([]byte(`{"query":"адоб","session_id":"s"}`), "k")
	if err != nil {
		t.Fatal(err)
	}
	b, br, err := decodeSearch([]byte(`{"session_id":"s","query":"\u0430\u0434\u043e\u0431"}`), "k")
	if err != nil || a.BodyHash != b.BodyHash || ar.Query != br.Query {
		t.Fatal(a, b, ar.Query, br.Query, err)
	}
}
