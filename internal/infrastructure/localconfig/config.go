// Package localconfig loads process-local settings without exposing credentials.
package localconfig

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Settings struct{ PostgresDSN, ElasticsearchURL, ElasticsearchKey string }

func Load() (Settings, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return Settings{}, errors.New("cannot read .env")
	}
	s := Settings{PostgresDSN: os.Getenv("POSTGRES_DSN"), ElasticsearchURL: os.Getenv("ELASTICSEARCH_URL"), ElasticsearchKey: os.Getenv("ELASTICSEARCH_API_KEY")}
	if s.ElasticsearchURL == "" {
		s.ElasticsearchURL = "http://localhost:9200"
	}
	return s, nil
}
func (s Settings) Pool(ctx context.Context) (*pgxpool.Pool, error) {
	if s.PostgresDSN == "" {
		return nil, errors.New("POSTGRES_DSN is required")
	}
	c, err := pgxpool.ParseConfig(s.PostgresDSN)
	if err != nil {
		return nil, errors.New("invalid POSTGRES_DSN")
	}
	c.MaxConns = 3
	c.MinConns = 0
	c.ConnConfig.ConnectTimeout = 5 * time.Second
	c.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errors.New("cannot create PostgreSQL pool")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Ping(check); err != nil {
		p.Close()
		return nil, errors.New("PostgreSQL connection failed")
	}
	return p, nil
}
