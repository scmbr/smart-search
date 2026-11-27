package elastic

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	a "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

// PilotRank uses the same text as embeddings in an isolated, owner-checked index.
type PilotRank struct{ Client *Client }

type pilotMetadata struct{ Owner, DatasetSHA string }

func (c *Client) pilotMetadata(ctx context.Context, name string) (pilotMetadata, bool, error) {
	raw, code, err := c.request(ctx, "GET", "/"+name+"/_mapping", nil)
	if err != nil || code == 404 {
		return pilotMetadata{}, false, err
	}
	var reply map[string]struct {
		Mappings struct {
			Meta pilotMetadata `json:"_meta"`
		}
	}
	if err = json.Unmarshal(raw, &reply); err != nil {
		return pilotMetadata{}, false, err
	}
	entry, ok := reply[name]
	if !ok {
		return pilotMetadata{}, false, errors.New("missing pilot index metadata")
	}
	return entry.Mappings.Meta, true, nil
}

func (p PilotRank) Rank(ctx context.Context, x d.Dataset) (result [][]d.Rank, err error) {
	if err = d.Validate(x); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	owner := hex.EncodeToString(nonce[:])
	name := "smart-search-pilot-" + owner
	expected := pilotMetadata{Owner: owner, DatasetSHA: d.Fingerprint(x)}
	c := p.Client
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		meta, exists, e := c.pilotMetadata(clean, name)
		if e != nil {
			err = errors.Join(err, e)
			return
		}
		if !exists {
			return
		}
		if meta != expected {
			err = errors.Join(err, errors.New("refusing to delete foreign pilot index"))
			return
		}
		_, _, e = c.request(clean, "DELETE", "/"+name, nil)
		err = errors.Join(err, e)
	}()
	settings := mapping(a.Spec{})["settings"]
	body := map[string]any{"settings": settings, "mappings": map[string]any{"dynamic": "strict", "_meta": map[string]string{"Owner": owner, "DatasetSHA": d.Fingerprint(x)}, "properties": map[string]any{"Text": map[string]string{"type": "text", "analyzer": "lexical"}}}}
	if _, _, err = c.request(ctx, "PUT", "/"+name, body); err != nil {
		// A timed-out PUT may have succeeded before transport retried it.
		meta, exists, readErr := c.pilotMetadata(ctx, name)
		if readErr != nil || !exists || meta != expected {
			return nil, errors.Join(err, readErr)
		}
		err = nil
	}
	var bulk bytes.Buffer
	encoder := json.NewEncoder(&bulk)
	for _, doc := range x.Documents {
		if err = encoder.Encode(map[string]any{"index": map[string]string{"_index": name, "_id": doc.ID}}); err != nil {
			return nil, err
		}
		if err = encoder.Encode(map[string]string{"Text": doc.Text}); err != nil {
			return nil, err
		}
	}
	raw, _, err := c.request(ctx, "POST", "/_bulk", bulk.Bytes())
	if err != nil {
		return nil, err
	}
	var ack struct {
		Errors bool
		Items  []map[string]struct{ Status int }
	}
	if err = json.Unmarshal(raw, &ack); err != nil {
		return nil, err
	}
	if ack.Errors || len(ack.Items) != len(x.Documents) {
		return nil, errors.New("pilot bulk incomplete")
	}
	for _, item := range ack.Items {
		if item["index"].Status != 200 && item["index"].Status != 201 {
			return nil, errors.New("pilot bulk item failed")
		}
	}
	if _, _, err = c.request(ctx, "POST", "/"+name+"/_refresh", nil); err != nil {
		return nil, err
	}
	for _, q := range x.Queries {
		body := map[string]any{"size": 4, "timeout": "5s", "query": map[string]any{"match": map[string]any{"Text": map[string]string{"query": q.Text, "operator": "or"}}}, "sort": []any{map[string]string{"_score": "desc"}}}
		raw, _, err = c.request(ctx, "POST", "/"+name+"/_search", body)
		if err != nil {
			return nil, err
		}
		var reply struct {
			TimedOut bool                 `json:"timed_out"`
			Shards   struct{ Failed int } `json:"_shards"`
			Hits     struct {
				Hits []struct {
					ID    string  `json:"_id"`
					Score float64 `json:"_score"`
				}
			}
		}
		if err = json.Unmarshal(raw, &reply); err != nil {
			return nil, err
		}
		if reply.TimedOut || reply.Shards.Failed > 0 {
			return nil, errors.New("pilot BM25 partial result")
		}
		var ranks []d.Rank
		for _, h := range reply.Hits.Hits {
			ranks = append(ranks, d.Rank{ID: h.ID, Score: h.Score})
		}
		result = append(result, ranks)
	}
	return result, nil
}
