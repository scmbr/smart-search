// Package httpapi maps application results into the public HTTP contract.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

type Readiness interface{ Check(context.Context) error }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var b [16]byte
	id := "request-unavailable"
	if _, err := rand.Read(b[:]); err == nil {
		id = hex.EncodeToString(b[:])
	}
	writeJSON(w, status, struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}{id, code, message})
}

func New(ready Readiness) http.Handler {
	mux := http.NewServeMux()
	health := func(check bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				writeError(w, http.StatusMethodNotAllowed, "invalid_request", "Method not allowed")
				return
			}
			if check {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				if ready == nil || ready.Check(ctx) != nil {
					writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Required search resources are not ready")
					return
				}
			}
			writeJSON(w, http.StatusOK, struct {
				Status string `json:"status"`
			}{"ok"})
		}
	}
	mux.HandleFunc("/api/v1/health/live", health(false))
	mux.HandleFunc("/api/v1/health/ready", health(true))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	})
	return mux
}
