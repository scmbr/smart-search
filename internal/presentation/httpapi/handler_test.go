package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type checkerFake struct {
	calls    int
	fail     bool
	deadline bool
}

func (f *checkerFake) Check(ctx context.Context) error {
	f.calls++
	_, f.deadline = ctx.Deadline()
	if f.fail {
		return errors.New("secret password and key")
	}
	return nil
}
func TestHealthRoutesAndSafeErrors(t *testing.T) {
	f := &checkerFake{}
	handler := New(f)
	for _, tc := range []struct {
		method, path  string
		fail          bool
		status, calls int
	}{
		{"GET", "/api/v1/health/live", true, 200, 0},
		{"GET", "/api/v1/health/ready", false, 200, 1},
		{"GET", "/api/v1/health/ready", true, 503, 1},
		{"POST", "/api/v1/health/live", false, 405, 0},
		{"GET", "/api/v1/unknown", false, 404, 0},
	} {
		f.calls = 0
		f.fail = tc.fail
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || f.calls != tc.calls || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%+v code=%d calls=%d body=%s", tc, w.Code, f.calls, w.Body.String())
		}
		if tc.path == "/api/v1/health/ready" && !f.deadline {
			t.Fatal("missing deadline")
		}
	}
}
