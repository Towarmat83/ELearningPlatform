package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Row is a minimal interface satisfied by *pgxpool.Row.
type Row interface {
	Scan(dest ...any) error
}

// Pool is the minimal DB interface used by handlers and middleware.
type Pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Adapter wraps *pgxpool.Pool to satisfy Pool.
type Adapter struct{ p *pgxpool.Pool }

// NewAdapter wraps a real pgxpool.Pool.
func NewAdapter(p *pgxpool.Pool) *Adapter { return &Adapter{p: p} }

func (a *Adapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return a.p.QueryRow(ctx, sql, args...)
}

func (a *Adapter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return a.p.Query(ctx, sql, args...)
}

func (a *Adapter) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return a.p.Exec(ctx, sql, args...)
}
