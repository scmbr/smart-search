package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	d "github.com/scmbr/smart-search/internal/domain/embeddingpilot"
)

type EmbeddingStore struct{ pool *pgxpool.Pool }

func NewEmbeddingStore(p *pgxpool.Pool) *EmbeddingStore { return &EmbeddingStore{pool: p} }
func (s *EmbeddingStore) Get(ctx context.Context, keys []string) (map[string][]float64, error) {
	rows, err := s.pool.Query(ctx, "SELECT cache_key,vector FROM embedding_cache WHERE cache_key=ANY($1)", keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]float64{}
	for rows.Next() {
		var key string
		var raw []byte
		var vector []float64
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &vector); err != nil {
			return nil, err
		}
		out[key] = vector
	}
	return out, rows.Err()
}
func (s *EmbeddingStore) Budget(ctx context.Context) (a.Budget, error) {
	var b a.Budget
	err := s.pool.QueryRow(ctx, `SELECT cap_nano,coalesce((SELECT sum(actual_nano) FROM api_reservations),0),coalesce((SELECT sum(upper_nano) FROM api_reservations WHERE actual_nano IS NULL),0) FROM api_budget WHERE singleton`).Scan(&b.CapNano, &b.ActualNano, &b.UncertainNano)
	return b, err
}
func (s *EmbeddingStore) Reserve(ctx context.Context, r a.Reservation) error {
	if r.CapNano < 1 || r.CapNano > d.ProjectCapNano || r.UpperNano < 1 || r.ID == "" {
		return a.ErrBudget
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var cap, used int64
	if err = tx.QueryRow(ctx, "SELECT cap_nano FROM api_budget WHERE singleton FOR UPDATE").Scan(&cap); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT coalesce(sum(coalesce(actual_nano,upper_nano)),0) FROM api_reservations").Scan(&used); err != nil {
		return err
	}
	if cap != r.CapNano || r.UpperNano > cap-used {
		return a.ErrBudget
	}
	if _, err = tx.Exec(ctx, "INSERT INTO api_reservations(id,model,dimensions,upper_nano) VALUES($1,$2,$3,$4)", r.ID, d.Model, d.Dimensions, r.UpperNano); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *EmbeddingStore) Settle(ctx context.Context, r a.Reservation, actual, tokens int64, entries []a.Entry) error {
	if actual < 1 || tokens < 1 || actual != d.Cost(tokens) || actual > r.UpperNano {
		return a.ErrBudget
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE api_reservations SET actual_nano=$1,input_tokens=$2,settled_at=now() WHERE id=$3 AND upper_nano=$4 AND actual_nano IS NULL`, actual, tokens, r.ID, r.UpperNano)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("reservation missing or already settled")
	}
	for _, e := range entries {
		if err = d.ValidateVector(e.Vector); err != nil {
			return err
		}
		raw, err := json.Marshal(e.Vector)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO embedding_cache(cache_key,vector) VALUES($1,$2) ON CONFLICT(cache_key) DO NOTHING", e.Key, raw); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
