// Package postgres persists catalog snapshots and their transactional checkpoints.
package postgres

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_catalog.sql
var catalogSchema string

//go:embed migrations/002_embedding_pilot.sql
var embeddingSchema string

//go:embed migrations/003_embedding_runtime.sql
var embeddingRuntimeSchema string

//go:embed migrations/004_query_parser.sql
var queryParserSchema string

//go:embed migrations/005_search_sessions.sql
var searchSessionsSchema string

const migrationLock int64 = 7362784101
const importLock int64 = 7362784102

// Migrate is forward-only and refuses modifications to an applied migration.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, sha256 text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for i, schema := range []string{catalogSchema, embeddingSchema, embeddingRuntimeSchema, queryParserSchema, searchSessionsSchema} {
		version := i + 1
		h := sha256.Sum256([]byte(schema))
		checksum := hex.EncodeToString(h[:])
		var existing string
		err = tx.QueryRow(ctx, "SELECT sha256 FROM schema_migrations WHERE version=$1", version).Scan(&existing)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx, schema); err != nil {
				return fmt.Errorf("migration %d: %w", version, err)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,sha256) VALUES($1,$2)", version, checksum); err != nil {
				return err
			}
		case err != nil:
			return err
		case existing != checksum:
			return fmt.Errorf("applied migration %d checksum differs", version)
		}
	}
	return tx.Commit(ctx)
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx) // No-op after Commit; best effort on a broken connection.
}
