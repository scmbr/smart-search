package contracts_test

import (
	"context"
	"errors"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/scmbr/smart-search/internal/presentation/httpapi"
	"net/http/httptest"
	"testing"
)

type healthChecker struct{ fail bool }

func (c healthChecker) Check(context.Context) error {
	if c.fail {
		return errors.New("unavailable")
	}
	return nil
}

func TestActualHealthResponsesMatchOpenAPI(t *testing.T) {
	doc := loadContract(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		fail   bool
		status int
	}{
		{"/api/v1/health/live", true, 200}, {"/api/v1/health/ready", false, 200}, {"/api/v1/health/ready", true, 503},
	} {
		req := httptest.NewRequest("GET", "http://localhost:8080"+tc.path, nil)
		w := httptest.NewRecorder()
		httpapi.New(healthChecker{tc.fail}).ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(w.Code, tc.status)
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
	}
}
