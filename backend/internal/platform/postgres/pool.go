package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Options struct {
	URL            string
	ConnectTimeout time.Duration
	MaxConnections int32
}

// Pool owns the PostgreSQL connection lifecycle for a process.
type Pool struct {
	orm *gorm.DB
	sql *sql.DB
}

func Open(ctx context.Context, options Options) (*Pool, error) {
	config, err := pgx.ParseConfig(options.URL)
	if err != nil {
		return nil, errors.New("parse PostgreSQL pool configuration")
	}
	config.ConnectTimeout = options.ConnectTimeout

	sqlDatabase := stdlib.OpenDB(*config)
	sqlDatabase.SetMaxOpenConns(int(options.MaxConnections))
	maxIdleConnections := int(options.MaxConnections) / 2
	if maxIdleConnections < 1 {
		maxIdleConnections = 1
	}
	sqlDatabase.SetMaxIdleConns(maxIdleConnections)

	orm, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDatabase}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDatabase.Close()
		return nil, fmt.Errorf("initialize GORM PostgreSQL adapter: %w", err)
	}
	return &Pool{orm: orm.WithContext(ctx), sql: sqlDatabase}, nil
}

func (p *Pool) Check(ctx context.Context) error {
	if err := p.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}

func (p *Pool) ORM() *gorm.DB {
	return p.orm
}

func (p *Pool) Close() {
	_ = p.sql.Close()
}
