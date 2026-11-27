package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	embedding "github.com/scmbr/smart-search/internal/application/embeddingpilot"
	a "github.com/scmbr/smart-search/internal/application/queryparser"
	p "github.com/scmbr/smart-search/internal/domain/queryparser"
)

type QueryParserStore struct{ pool *pgxpool.Pool }

func NewQueryParserStore(pool *pgxpool.Pool) *QueryParserStore { return &QueryParserStore{pool: pool} }
func (s *QueryParserStore) Get(ctx context.Context, key string) (a.Entry, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT entry FROM query_parser_cache WHERE cache_key=$1", key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return a.Entry{}, false, nil
	}
	if err != nil {
		return a.Entry{}, false, err
	}
	var entry a.Entry
	if len(raw) > 8192 || json.Unmarshal(raw, &entry) != nil || a.ValidateEntry(entry.Query, entry) != nil {
		return a.Entry{}, false, a.ErrOutput
	}
	plan, err := p.Plan(entry.Query)
	if err != nil || plan.Key != key {
		return a.Entry{}, false, a.ErrOutput
	}
	return entry, true, nil
}
func (s *QueryParserStore) TryGuard(ctx context.Context, key string) (a.Guard, error) {
	guard, err := NewEmbeddingStore(s.pool).TryGuard(ctx, []string{key})
	if errors.Is(err, embedding.ErrBusy) {
		return nil, a.ErrBusy
	}
	return guard, err
}
func validParserReservation(r a.Reservation) bool {
	raw, err := hex.DecodeString(r.Key)
	return err == nil && len(raw) == 32 && r.ID != "" && r.UpperNano > 0 && r.UpperNano <= p.QueryCapNano
}
func (s *QueryParserStore) Begin(ctx context.Context, r a.Reservation) error {
	if !validParserReservation(r) {
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
	if cap != p.ProjectCapNano || r.UpperNano > cap-used {
		return a.ErrBudget
	}
	var pending bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM query_parser_pending WHERE cache_key=$1)", r.Key).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return a.ErrUncertain
	}
	if _, err = tx.Exec(ctx, "INSERT INTO api_reservations(id,model,dimensions,upper_nano) VALUES($1,$2,0,$3)", r.ID, p.Model, r.UpperNano); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO query_parser_pending(cache_key,reservation_id) VALUES($1,$2)", r.Key, r.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *QueryParserStore) Settle(ctx context.Context, r a.Reservation, entry a.Entry, input, output int64) error {
	plan, err := p.Plan(entry.Query)
	if err != nil || !validParserReservation(r) || plan.Key != r.Key || plan.CostUpperNano != r.UpperNano || a.ValidateEntry(entry.Query, entry) != nil || input < 1 || input > plan.InputTokenUpper || output < 0 || output > p.MaxOutputTokens {
		return a.ErrBudget
	}
	actual := p.Cost(input, output)
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE api_reservations SET actual_nano=$1,input_tokens=$2,output_tokens=$3,settled_at=now(),outcome='settled'
 WHERE id=$4 AND model=$5 AND dimensions=0 AND upper_nano=$6 AND actual_nano IS NULL AND outcome='unresolved'
 AND EXISTS(SELECT 1 FROM query_parser_pending WHERE cache_key=$7 AND reservation_id=$4)`, actual, input, output, r.ID, p.Model, r.UpperNano, r.Key)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return a.ErrUncertain
	}
	if _, err = tx.Exec(ctx, "INSERT INTO query_parser_cache(cache_key,entry) VALUES($1,$2)", r.Key, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM query_parser_pending WHERE cache_key=$1 AND reservation_id=$2", r.Key, r.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *QueryParserStore) Rejected(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE api_reservations SET outcome='rejected' WHERE id=$1 AND model=$2 AND dimensions=0 AND actual_nano IS NULL AND outcome='unresolved'
 AND EXISTS(SELECT 1 FROM query_parser_pending WHERE reservation_id=$1)`, id, p.Model)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return a.ErrUncertain
	}
	if _, err = tx.Exec(ctx, "DELETE FROM query_parser_pending WHERE reservation_id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
