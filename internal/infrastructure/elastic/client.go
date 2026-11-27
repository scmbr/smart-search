// Package elastic adapts the official Elastic transport to versioned lexical search.
package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	domain "github.com/scmbr/smart-search/internal/domain/search"
)

var indexPattern = regexp.MustCompile(`^smart-search-[a-f0-9]{64}$`)
var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,100}$`)

type Client struct {
	transport *elastictransport.Client
	alias     string
}
type metadata struct {
	State string
	Spec  app.Spec
}
type storedDocument domain.Document

func New(endpoint, key, alias string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" || !aliasPattern.MatchString(alias) {
		return nil, errors.New("invalid Elasticsearch endpoint or alias")
	}
	t, err := elastictransport.New(elastictransport.Config{URLs: []*url.URL{u}, APIKey: key, MaxRetries: 2, RetryOnStatus: []int{429, 502, 503, 504}, RetryBackoff: func(attempt int) time.Duration { return time.Duration(attempt+1) * 100 * time.Millisecond }, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second, MaxIdleConnsPerHost: 4}})
	if err != nil {
		return nil, err
	}
	return &Client{transport: t, alias: alias}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var raw []byte
	var err error
	if b, ok := body.([]byte); ok {
		raw = b
	} else if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	r.Header.Set("Content-Type", "application/json")
	if path == "/_bulk" {
		r.Header.Set("Content-Type", "application/x-ndjson")
	}
	response, err := c.transport.Perform(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, fmt.Errorf("%w: request failed", domain.ErrUnavailable)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024+1))
	if err != nil || len(data) > 16*1024*1024 {
		return nil, response.StatusCode, fmt.Errorf("%w: invalid response body", domain.ErrUnavailable)
	}
	if response.Header.Get("X-Elastic-Product") != "Elasticsearch" {
		return nil, response.StatusCode, fmt.Errorf("%w: unexpected backend", domain.ErrUnavailable)
	}
	if response.StatusCode == 404 {
		return data, 404, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, fmt.Errorf("%w: HTTP %d", domain.ErrUnavailable, response.StatusCode)
	}
	return data, response.StatusCode, nil
}

func (c *Client) meta(ctx context.Context, index string) (metadata, bool, error) {
	if !indexPattern.MatchString(index) {
		return metadata{}, false, errors.New("invalid physical search index")
	}
	raw, code, err := c.request(ctx, "GET", "/"+index+"/_mapping", nil)
	if err != nil || code == 404 {
		return metadata{}, false, err
	}
	var result map[string]struct {
		Mappings struct {
			Meta metadata `json:"_meta"`
		}
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return metadata{}, false, err
	}
	entry, ok := result[index]
	if !ok || entry.Mappings.Meta.Spec.Name != index || entry.Mappings.Meta.Spec.Name != "smart-search-"+entry.Mappings.Meta.Spec.Fingerprint || entry.Mappings.Meta.Spec.Count < 1 || entry.Mappings.Meta.Spec.CatalogID == "" {
		return metadata{}, false, errors.New("index metadata mismatch")
	}
	return entry.Mappings.Meta, true, nil
}

func (c *Client) Prepare(ctx context.Context, spec app.Spec) (bool, error) {
	m, exists, err := c.meta(ctx, spec.Name)
	if err != nil {
		return false, err
	}
	if exists {
		if m.Spec != spec {
			return false, errors.New("index fingerprint metadata mismatch")
		}
		return m.State == "ready", nil
	}
	_, code, err := c.request(ctx, "PUT", "/"+spec.Name, mapping(spec))
	if err != nil && ctx.Err() == nil {
		// A timed-out PUT may have created the index before transport retries it.
		// Adopt only the exact expected immutable specification, never a foreign index.
		m, exists, readErr := c.meta(ctx, spec.Name)
		if readErr == nil && exists && m.Spec == spec && (m.State == "staging" || m.State == "ready") {
			return m.State == "ready", nil
		}
	}
	if code == 404 && err == nil {
		err = domain.ErrUnavailable
	}
	return false, err
}

func (c *Client) Write(ctx context.Context, index string, docs []domain.Document) error {
	if !indexPattern.MatchString(index) {
		return errors.New("invalid bulk index")
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, d := range docs {
		if err := encoder.Encode(map[string]any{"index": map[string]string{"_index": index, "_id": d.OfferID}}); err != nil {
			return err
		}
		if err := encoder.Encode(storedDocument(d)); err != nil {
			return err
		}
	}
	raw, code, err := c.request(ctx, "POST", "/_bulk", body.Bytes())
	if err != nil {
		return err
	}
	if code == 404 {
		return domain.ErrUnavailable
	}
	var result struct {
		Errors *bool `json:"errors"`
		Items  []struct {
			Index struct {
				Status int `json:"status"`
			} `json:"index"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Errors == nil || *result.Errors || len(result.Items) != len(docs) {
		return errors.New("Elasticsearch bulk item failed; alias unchanged")
	}
	for _, item := range result.Items {
		if item.Index.Status < 200 || item.Index.Status >= 300 {
			return errors.New("Elasticsearch bulk status failed")
		}
	}
	return nil
}

func (c *Client) Finish(ctx context.Context, spec app.Spec) error {
	if !indexPattern.MatchString(spec.Name) {
		return errors.New("invalid index")
	}
	if _, code, err := c.request(ctx, "POST", "/"+spec.Name+"/_refresh", nil); err != nil {
		return err
	} else if code == 404 {
		return domain.ErrNotReady
	}
	raw, code, err := c.request(ctx, "GET", "/"+spec.Name+"/_count", nil)
	if err != nil {
		return err
	}
	if code == 404 {
		return domain.ErrNotReady
	}
	var count struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(raw, &count); err != nil {
		return err
	}
	if count.Count != spec.Count {
		return errors.New("search index count differs from catalog")
	}
	if _, code, err := c.request(ctx, "PUT", "/"+spec.Name+"/_mapping", map[string]any{"_meta": metadata{State: "ready", Spec: spec}}); err != nil {
		return err
	} else if code == 404 {
		return domain.ErrNotReady
	}
	_, code, err = c.request(ctx, "PUT", "/"+spec.Name+"/_settings", map[string]any{"index": map[string]any{"blocks.write": true, "refresh_interval": "1s"}})
	if code == 404 && err == nil {
		return domain.ErrNotReady
	}
	return err
}

func (c *Client) aliases(ctx context.Context) ([]string, error) {
	raw, code, err := c.request(ctx, "GET", "/_alias/"+c.alias, nil)
	if err != nil {
		return nil, err
	}
	if code == 404 {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	var names []string
	for index := range entries {
		if !indexPattern.MatchString(index) {
			return nil, errors.New("alias contains unmanaged index")
		}
		names = append(names, index)
	}
	return names, nil
}

func (c *Client) Activate(ctx context.Context, spec app.Spec) error {
	m, exists, err := c.meta(ctx, spec.Name)
	if err != nil {
		return err
	}
	if !exists || m.State != "ready" || m.Spec != spec {
		return domain.ErrNotReady
	}
	old, err := c.aliases(ctx)
	if err != nil {
		return err
	}
	var actions []any
	for _, index := range old {
		if index == spec.Name && len(old) == 1 {
			return nil
		}
		actions = append(actions, map[string]any{"remove": map[string]any{"index": index, "alias": c.alias, "must_exist": false}})
	}
	actions = append(actions, map[string]any{"add": map[string]string{"index": spec.Name, "alias": c.alias}})
	_, code, err := c.request(ctx, "POST", "/_aliases", map[string]any{"actions": actions})
	if code == 404 && err == nil {
		return domain.ErrNotReady
	}
	return err
}

func (c *Client) Search(ctx context.Context, r domain.Request) (domain.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := domain.Result{Query: r.OriginalQuery}
	indexes, err := c.aliases(ctx)
	if err != nil {
		return result, err
	}
	if len(indexes) != 1 {
		return result, domain.ErrNotReady
	}
	m, exists, err := c.meta(ctx, indexes[0])
	if err != nil {
		return result, err
	}
	if !exists || m.State != "ready" || m.Spec.DocumentVersion != app.DocumentSchemaVersion || m.Spec.NormalizationVersion != catalog.NormalizationVersion {
		return result, domain.ErrNotReady
	}
	result.CatalogID = m.Spec.CatalogID
	result.IndexVersion = m.Spec.Fingerprint
	result.SearchMode = "none"
	for _, priced := range []bool{true, false} {
		for _, expanded := range []bool{false, true} {
			if expanded && (r.Plan.ExactOnly || len(r.Plan.Variants) == 0) {
				continue
			}
			hits, searchErr := c.hits(ctx, indexes[0], r, priced, expanded)
			if searchErr != nil {
				return result, searchErr
			}
			mode := "exact"
			if expanded {
				mode = "expanded"
			}
			result.SearchAttempts = append(result.SearchAttempts, domain.SearchAttempt{Mode: mode, Priced: priced, FamiliesReturned: len(hits)})
			if len(hits) > 0 {
				result.Hits = hits
				result.SearchMode = mode
				result.UnpricedFallbackUsed = !priced
				break
			}
		}
		if len(result.Hits) > 0 {
			break
		}
	}
	for _, hit := range result.Hits {
		if hit.Document.CatalogID != result.CatalogID {
			return domain.Result{}, fmt.Errorf("%w: mixed catalog versions", domain.ErrUnavailable)
		}
	}
	domain.Describe(&result)
	return result, nil
}

func (c *Client) hits(ctx context.Context, index string, r domain.Request, priced, expanded bool) ([]domain.Hit, error) {
	return c.searchHits(ctx, index, searchBody(r, priced, expanded))
}

func (c *Client) searchHits(ctx context.Context, index string, body map[string]any) ([]domain.Hit, error) {
	raw, code, err := c.request(ctx, "POST", "/"+index+"/_search", body)
	if err != nil {
		return nil, err
	}
	if code == 404 {
		return nil, domain.ErrNotReady
	}
	var result struct {
		TimedOut *bool `json:"timed_out"`
		Shards   *struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits []struct {
				Score  float64        `json:"_score"`
				Source storedDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.TimedOut == nil || result.Shards == nil || result.Hits == nil || *result.TimedOut || result.Shards.Failed > 0 {
		return nil, domain.ErrUnavailable
	}
	var hits []domain.Hit
	for _, hit := range result.Hits.Hits {
		hits = append(hits, domain.Hit{Document: domain.Document(hit.Source), Score: hit.Score})
	}
	return hits, nil
}
