package postgres

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	a "github.com/scmbr/smart-search/internal/application/embeddingpilot"
)

type embeddingGuard struct{ tx pgx.Tx }

func (g *embeddingGuard) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := g.tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}
func (s *EmbeddingStore) TryGuard(ctx context.Context, keys []string) (a.Guard, error) {
	if len(keys) == 0 || len(keys) > 32 {
		return nil, errors.New("invalid embedding lock keys")
	}
	ordered := append([]string(nil), keys...)
	sort.Strings(ordered)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			rollback(tx)
		}
	}()
	for _, key := range ordered {
		raw, e := hex.DecodeString(key)
		if e != nil || len(raw) != 32 {
			return nil, errors.New("invalid embedding lock key")
		}
		// Collision only serializes unrelated keys; the persistent claim uses the full SHA.
		var locked bool
		if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(7362785,$1)", int32(binary.BigEndian.Uint32(raw[:4]))).Scan(&locked); err != nil {
			return nil, err
		}
		if !locked {
			return nil, a.ErrBusy
		}
	}
	for slot := 0; slot < 2; slot++ {
		var locked bool
		if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(7362786,$1)", slot).Scan(&locked); err != nil {
			return nil, err
		}
		if locked {
			success = true
			return &embeddingGuard{tx: tx}, nil
		}
	}
	return nil, a.ErrBusy
}
func (s *EmbeddingStore) BeginAttempt(ctx context.Context, r a.Reservation, keys []string) error {
	if len(keys) == 0 || len(keys) > 32 {
		return errors.New("invalid claimed input count")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = reserveEmbedding(ctx, tx, r); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM embedding_pending WHERE cache_key=ANY($1)", keys).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return a.ErrUncertain
	}
	for _, key := range keys {
		if _, err = tx.Exec(ctx, "INSERT INTO embedding_pending(cache_key,reservation_id) VALUES($1,$2)", key, r.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *EmbeddingStore) Rejected(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, "UPDATE api_reservations SET outcome='rejected' WHERE id=$1 AND actual_nano IS NULL", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("rejected reservation missing or settled")
	}
	if _, err = tx.Exec(ctx, "DELETE FROM embedding_pending WHERE reservation_id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
