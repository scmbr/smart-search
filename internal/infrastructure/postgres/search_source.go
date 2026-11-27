package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/scmbr/smart-search/internal/application/lexical"
	catalog "github.com/scmbr/smart-search/internal/domain/catalog"
	search "github.com/scmbr/smart-search/internal/domain/search"
)

const indexLock int64 = 7362784103

type CatalogSource struct{ pool *pgxpool.Pool }

func NewCatalogSource(pool *pgxpool.Pool) *CatalogSource { return &CatalogSource{pool: pool} }

type catalogFeed struct {
	conn       *pgxpool.Conn
	info       app.CatalogInfo
	categories []catalog.Category
	validation catalog.Validation
}

func (s *CatalogSource) openMetadata(ctx context.Context) (_ *catalogFeed, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	locked := false
	defer func() {
		if err != nil {
			if locked {
				_ = releaseLock(conn, indexLock)
			} else {
				conn.Release()
			}
		}
	}()
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", indexLock).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, catalog.ErrBusy
	}
	f := &catalogFeed{conn: conn}
	var raw []byte
	err = conn.QueryRow(ctx, `SELECT s.id,s.schema_version,s.validation FROM catalog_snapshots s JOIN catalog_active a ON a.snapshot_id=s.id WHERE s.status='ready'`).Scan(&f.info.ID, &f.info.SchemaVersion, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, search.ErrNotReady
	}
	if err != nil {
		return nil, err
	}
	var validation storedValidation
	if err = json.Unmarshal(raw, &validation); err != nil {
		return nil, err
	}
	f.info.Offers = validation.Accepted
	f.validation = catalog.Validation(validation)
	return f, nil
}

func (s *CatalogSource) Open(ctx context.Context) (_ app.Feed, err error) {
	f, err := s.openMetadata(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, f.Close())
		}
	}()
	conn := f.conn
	rows, err := conn.Query(ctx, "SELECT payload FROM catalog_records WHERE snapshot_id=$1 AND kind='category' ORDER BY ordinal", f.info.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		var c storedCategory
		if err = rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(payload, &c); err != nil {
			return nil, err
		}
		f.categories = append(f.categories, catalog.Category(c))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return f, nil
}
func (f *catalogFeed) Info() app.CatalogInfo          { return f.info }
func (f *catalogFeed) Categories() []catalog.Category { return f.categories }
func (f *catalogFeed) Read(ctx context.Context, after int64, size int) ([]app.Row, error) {
	if size < 1 || size > 1000 {
		return nil, errors.New("invalid catalog page size")
	}
	rows, err := f.conn.Query(ctx, "SELECT ordinal,payload FROM catalog_records WHERE snapshot_id=$1 AND kind='offer' AND ordinal>$2 ORDER BY ordinal LIMIT $3", f.info.ID, after, size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []app.Row
	for rows.Next() {
		var row app.Row
		var payload []byte
		var offer storedOffer
		if err := rows.Scan(&row.Ordinal, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &offer); err != nil {
			return nil, err
		}
		row.Offer = catalog.Offer(offer)
		result = append(result, row)
	}
	return result, rows.Err()
}
func (f *catalogFeed) Current(ctx context.Context) (bool, error) {
	var current bool
	err := f.conn.QueryRow(ctx, "SELECT snapshot_id=$1 FROM catalog_active", f.info.ID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return current, err
}
func (f *catalogFeed) Close() error { return releaseLock(f.conn, indexLock) }
