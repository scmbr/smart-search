package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	s "github.com/scmbr/smart-search/internal/domain/search"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Search interface {
	Search(context.Context, d.Identity, s.Request) (d.Record, error)
}
type requestDTO struct {
	Query   string  `json:"query"`
	Session string  `json:"session_id"`
	Limit   *int    `json:"limit"`
	Cursor  *string `json:"cursor"`
	Context *struct {
		Category *string `json:"category_id"`
		Segment  *string `json:"segment"`
		Origin   *string `json:"origin"`
		Profile  *string `json:"profile_id"`
	} `json:"context"`
	Filters *struct {
		Policy   *string `json:"availability_policy"`
		Platform *string `json:"platform"`
		Version  *string `json:"version"`
		MinUsers *int    `json:"min_users"`
		Kind     *string `json:"offer_kind"`
		License  *string `json:"license_type"`
	} `json:"filters"`
}

// uniqueJSON rejects ambiguous duplicate keys, nulls and excessively nested input.
func uniqueJSON(decoder *json.Decoder, depth int) (any, error) {
	if depth > 8 {
		return nil, d.ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
		return nil, d.ErrInvalid
	}
	if delim, ok := token.(json.Delim); ok {
		if delim != '{' {
			return nil, d.ErrInvalid
		}
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, d.ErrInvalid
			}
			name, ok := key.(string)
			if !ok {
				return nil, d.ErrInvalid
			}
			if _, exists := object[name]; exists {
				return nil, d.ErrInvalid
			}
			value, err := uniqueJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		if _, err = decoder.Token(); err != nil {
			return nil, d.ErrInvalid
		}
		return object, nil
	}
	return token, nil
}
func closedKeys(value any, allowed string) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for key := range object {
		if !slices.Contains(strings.Split(allowed, "|"), key) {
			return false
		}
	}
	return true
}
func decodeSearch(raw []byte, key string) (d.Identity, s.Request, error) {
	bad := func() (d.Identity, s.Request, error) { return d.Identity{}, s.Request{}, d.ErrInvalid }
	if !utf8.Valid(raw) {
		return bad()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := uniqueJSON(decoder, 0)
	if err != nil {
		return bad()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return bad()
	}
	if !closedKeys(value, "query|session_id|limit|cursor|context|filters") {
		return bad()
	}
	object := value.(map[string]any)
	if v, ok := object["context"]; ok && !closedKeys(v, "category_id|segment|origin|profile_id") {
		return bad()
	}
	if v, ok := object["filters"]; ok && !closedKeys(v, "availability_policy|platform|version|min_users|offer_kind|license_type") {
		return bad()
	}
	var dto requestDTO
	if json.Unmarshal(raw, &dto) != nil {
		return bad()
	}
	if dto.Cursor != nil {
		return bad()
	}
	if c := dto.Context; c != nil {
		if c.Category != nil || c.Profile != nil || (c.Segment != nil && *c.Segment != "anonymous") || (c.Origin != nil && *c.Origin != "unspecified") {
			return bad()
		}
	}
	request := s.Request{Query: dto.Query, Limit: 4}
	if dto.Limit != nil {
		request.Limit = *dto.Limit
	}
	if f := dto.Filters; f != nil {
		if f.License != nil {
			return bad()
		}
		if f.Policy != nil {
			request.Policy = *f.Policy
			if request.Policy != "priced_first" && request.Policy != "available_only" {
				return bad()
			}
		}
		if f.Platform != nil {
			request.Platform = *f.Platform
			if strings.TrimSpace(*f.Platform) == "" {
				return bad()
			}
		}
		if f.Version != nil {
			request.Version = *f.Version
			if strings.TrimSpace(*f.Version) == "" {
				return bad()
			}
		}
		if f.Kind != nil {
			request.Kind = *f.Kind
			if request.Kind == "" {
				return bad()
			}
		}
		if f.MinUsers != nil {
			if *f.MinUsers < 1 || *f.MinUsers > 1000000 {
				return bad()
			}
			request.MinUsers = *f.MinUsers
		}
	}
	prepared, err := s.Prepare(request)
	if err != nil || len(prepared.UnsupportedConstraints) > 0 {
		return bad()
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return bad()
	}
	hash := sha256.Sum256(canonical)
	id := d.Identity{Session: dto.Session, Key: key, BodyHash: hex.EncodeToString(hash[:])}
	if id.Validate() != nil {
		return bad()
	}
	return id, request, nil
}
func searchHandler(service Search) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeError(w, 405, "invalid_request", "Method not allowed")
			return
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
			writeError(w, 400, "invalid_request", "Expected UTF-8 application/json")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			var large *http.MaxBytesError
			if errors.As(err, &large) {
				writeError(w, 413, "payload_too_large", "Request body exceeds 16 KiB")
			} else {
				writeError(w, 400, "invalid_request", "Cannot read request body")
			}
			return
		}
		if len(r.Header.Values("Idempotency-Key")) != 1 {
			writeError(w, 400, "invalid_request", "A single Idempotency-Key is required")
			return
		}
		id, request, err := decodeSearch(raw, r.Header.Get("Idempotency-Key"))
		if err != nil {
			writeError(w, 400, "invalid_request", "Invalid or unsupported search parameters; cursor, personalization and license_type are not supported yet")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		record, err := service.Search(ctx, id, request)
		if err != nil {
			switch {
			case errors.Is(err, d.ErrConflict):
				writeError(w, 409, "idempotency_conflict", "Key was already used with different request content")
			case errors.Is(err, d.ErrInvalid):
				writeError(w, 400, "invalid_request", "Invalid or unsupported search parameters")
			default:
				writeError(w, 503, "service_unavailable", "Search temporarily unavailable")
			}
			return
		}
		response, err := mapSearch(record)
		if err != nil {
			writeError(w, 503, "service_unavailable", "Search result unavailable")
			return
		}
		writeJSON(w, 200, response)
	}
}
