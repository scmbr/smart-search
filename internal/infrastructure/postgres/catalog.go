package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/scmbr/smart-search/internal/domain/catalog"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ domain.Repository = (*Repository)(nil)

type session struct {
	conn  *pgxpool.Conn
	state domain.Snapshot
}

// Persistence DTOs deliberately belong to the adapter. Their version is part
// of the source schema identity; they are not HTTP contracts.
type storedCategory domain.Category
type storedOffer domain.Offer
type storedValidation domain.Validation

func (r *Repository) Begin(ctx context.Context, identity domain.Identity) (_ domain.ImportSession, err error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	locked := false
	defer func() {
		if err != nil {
			if locked {
				_ = release(conn)
			} else {
				conn.Release()
			}
		}
	}()
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", importLock).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, domain.ErrBusy
	}
	id := identity.ID()
	if _, err := conn.Exec(ctx, `INSERT INTO catalog_snapshots(id,source_sha256,schema_version,status)
		VALUES($1,$2,$3,'staging') ON CONFLICT(id) DO NOTHING`, id, identity.SourceSHA256, identity.SchemaVersion); err != nil {
		return nil, err
	}
	s := &session{conn: conn}
	var validation []byte
	if err := conn.QueryRow(ctx, "SELECT id,status,checkpoint,validation FROM catalog_snapshots WHERE id=$1", id).
		Scan(&s.state.ID, &s.state.Status, &s.state.Checkpoint, &validation); err != nil {
		return nil, err
	}
	if validation != nil {
		var v storedValidation
		if err := json.Unmarshal(validation, &v); err != nil {
			return nil, err
		}
		s.state.Validation = domain.Validation(v)
	}
	return s, nil
}

func (s *session) Snapshot() domain.Snapshot { return s.state }

func (s *session) Save(ctx context.Context, first int64, records []domain.Record) error {
	if s.state.Status != domain.Staging || first != s.state.Checkpoint+1 || len(records) == 0 {
		return domain.ErrCheckpoint
	}
	rows := make([][]any, 0, len(records))
	for i, record := range records {
		var kind, id string
		var value any
		switch {
		case record.Category != nil && record.Offer == nil:
			kind, id, value = "category", record.Category.ID, storedCategory(*record.Category)
		case record.Offer != nil && record.Category == nil:
			kind, id, value = "offer", record.Offer.ID, storedOffer(*record.Offer)
		default:
			return errors.New("record must contain exactly one category or offer")
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		rows = append(rows, []any{s.state.ID, first + int64(i), kind, id, payload})
	}
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"catalog_records"}, []string{"snapshot_id", "ordinal", "kind", "source_id", "payload"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy records: %w", err)
	}
	next := first + int64(len(records)) - 1
	tag, err := tx.Exec(ctx, `UPDATE catalog_snapshots SET checkpoint=$1 WHERE id=$2 AND status='staging' AND checkpoint=$3`, next, s.state.ID, s.state.Checkpoint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrCheckpoint
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.state.Checkpoint = next
	return nil
}

func (s *session) Finish(ctx context.Context, v domain.Validation) error {
	if s.state.Status != domain.Staging {
		return errors.New("snapshot is not staging")
	}
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	status := domain.Invalid
	if v.Eligible() {
		var categories, offers, total, highest int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='category'), count(*) FILTER(WHERE kind='offer'), count(*), coalesce(max(ordinal),0)
			FROM catalog_records WHERE snapshot_id=$1`, s.state.ID).Scan(&categories, &offers, &total, &highest); err != nil {
			return err
		}
		if categories != int64(v.Categories) || offers != int64(v.Accepted) || total != s.state.Checkpoint || highest != total {
			return domain.ErrCheckpoint
		}
		status = domain.Ready
	}
	payload, err := json.Marshal(storedValidation(v))
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE catalog_snapshots SET status=$1,validation=$2,completed_at=now() WHERE id=$3 AND status='staging' AND checkpoint=$4`, status, payload, s.state.ID, s.state.Checkpoint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrCheckpoint
	}
	if status == domain.Ready {
		if _, err := tx.Exec(ctx, `INSERT INTO catalog_active(singleton,snapshot_id) VALUES(true,$1)
			ON CONFLICT(singleton) DO UPDATE SET snapshot_id=excluded.snapshot_id,activated_at=now()`, s.state.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.state.Status, s.state.Validation = status, v
	return nil
}

func release(conn *pgxpool.Conn) error {
	return releaseLock(conn, importLock)
}

func releaseLock(conn *pgxpool.Conn, lock int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", lock).Scan(&unlocked)
	if err != nil || !unlocked {
		// Never put a connection still holding the session lock back in the pool.
		closeErr := conn.Conn().Close(ctx)
		err = errors.Join(err, closeErr)
		if err == nil {
			err = errors.New("import lock was not held")
		}
	}
	conn.Release()
	return err
}

func (s *session) Close() error { return release(s.conn) }
