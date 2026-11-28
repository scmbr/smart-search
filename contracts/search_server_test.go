package contracts_test

import (
	"context"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"github.com/scmbr/smart-search/internal/presentation/httpapi"
	"net/http/httptest"
	"strings"
	"testing"
)

type searchServerFake struct {
	record d.Record
	err    error
}

func (f searchServerFake) Search(context.Context, d.Identity, s.Request) (d.Record, error) {
	return f.record, f.err
}
func TestActualSearchResponsesMatchOpenAPI(t *testing.T) {
	doc := loadContract(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	price := int64(100)
	hit := s.Hit{Document: s.Document{OfferID: "offer", FamilyID: "family", FamilyConfirmed: true, GroupID: "group", CategoryID: "category", Name: "License", URL: "https://store.softline.ru/vendor/offer/", OfferKind: "new_license", PriceMinor: &price}}
	for _, tc := range []struct {
		name   string
		hits   []s.Hit
		err    error
		status int
	}{
		{"match", []s.Hit{hit}, nil, 200}, {"empty", nil, nil, 200}, {"conflict", nil, d.ErrConflict, 409}, {"unavailable", nil, d.ErrUnavailable, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := searchServerFake{record: d.Record{ID: "search", Result: s.Result{Query: "product", CatalogID: "catalog", IndexVersion: "index", Hits: tc.hits}}, err: tc.err}
			req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/search", strings.NewReader(`{"query":"product","session_id":"session"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "key")
			w := httptest.NewRecorder()
			httpapi.New(nil, fake).ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			route, params, err := router.FindRoute(req)
			if err != nil {
				t.Fatal(err)
			}
			response := w.Result()
			defer response.Body.Close()
			input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: response.StatusCode, Header: response.Header, Body: response.Body, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
			if err = openapi3filter.ValidateResponse(context.Background(), input); err != nil {
				t.Fatal(err)
			}
		})
	}
}
