package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	a "github.com/scmbr/smart-search/internal/application/searchsession"
	d "github.com/scmbr/smart-search/internal/domain/searchsession"
	"time"
)

type SearchStore struct{ pool *pgxpool.Pool }

func NewSearchStore(pool *pgxpool.Pool) *SearchStore { return &SearchStore{pool: pool} }

func (s *SearchStore) CheckStore(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "SELECT session_id,idempotency_key,body_sha256,search_id,result,created_at,expires_at FROM search_sessions LIMIT 0")
	return err
}

type searchLease struct {
	conn     *pgxpool.Conn
	identity d.Identity
	key      int64
	record   *d.Record
	closed   bool
}

func (l *searchLease) Cached() *d.Record { return l.record }
func (l *searchLease) Close() error {
	if l.closed {
		return nil
	}
	l.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var unlocked bool
	err := l.conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", l.key).Scan(&unlocked)
	if err != nil || !unlocked {
		_ = l.conn.Conn().Close(ctx)
	}
	l.conn.Release()
	return err
}
func (s *SearchStore) Begin(ctx context.Context, id d.Identity) (a.Lease, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, d.ErrUnavailable
	}
	h := sha256.Sum256([]byte("search-session\x00" + id.Session + "\x00" + id.Key))
	key := int64(binary.BigEndian.Uint64(h[:8]))
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		// A failed query can have an uncertain outcome; discard that connection.
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = conn.Conn().Close(closeCtx)
			cancel()
		}
		conn.Release()
		return nil, d.ErrUnavailable
	}
	l := &searchLease{conn: conn, identity: id, key: key}
	fail := func(err error) (a.Lease, error) { _ = l.Close(); return nil, err }
	if _, err = conn.Exec(ctx, "DELETE FROM search_sessions WHERE session_id=$1 AND idempotency_key=$2 AND expires_at<=now()", id.Session, id.Key); err != nil {
		return fail(d.ErrUnavailable)
	}
	var hash string
	var raw []byte
	err = conn.QueryRow(ctx, "SELECT body_sha256,result FROM search_sessions WHERE session_id=$1 AND idempotency_key=$2", id.Session, id.Key).Scan(&hash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, nil
	}
	if err != nil {
		return fail(d.ErrUnavailable)
	}
	if hash != id.BodyHash {
		return fail(d.ErrConflict)
	}
	var record d.Record
	if json.Unmarshal(raw, &record) != nil || !d.ValidID(record.ID) || record.Result.CatalogID == "" || record.Result.IndexVersion == "" {
		return fail(d.ErrUnavailable)
	}
	l.record = &record
	return l, nil
}
func (l *searchLease) Complete(ctx context.Context, record d.Record) error {
	if l.closed || l.record != nil || !d.ValidID(record.ID) || record.Result.CatalogID == "" || record.Result.IndexVersion == "" || record.DurationMS < 0 {
		return d.ErrUnavailable
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > 1<<20 {
		return d.ErrUnavailable
	}
	_, err = l.conn.Exec(ctx, `INSERT INTO search_sessions(session_id,idempotency_key,body_sha256,search_id,result) VALUES($1,$2,$3,$4,$5)`, l.identity.Session, l.identity.Key, l.identity.BodyHash, record.ID, raw)
	if err == nil {
		l.record = &record
	}
	return err
}
