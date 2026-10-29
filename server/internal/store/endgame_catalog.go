package store

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

// SetEndgameCatalog is startup-only dependency injection into one store instance.
// Calling it with a different snapshot is forbidden; there is no hot reload.
func SetEndgameCatalog(s Store, c *domain.EndgameCatalog) error {
	if c == nil {
		return fmt.Errorf("store: missing endgame catalog")
	}
	set := func(dst **domain.EndgameCatalog) error {
		if *dst != nil && *dst != c {
			return fmt.Errorf("store: endgame catalog already configured")
		}
		*dst = c
		return nil
	}
	switch x := s.(type) {
	case *Postgres:
		return set(&x.endgameCatalog)
	case *Memory:
		return set(&x.endgameCatalog)
	case *pgTx:
		if err := set(&x.endgameCatalog); err != nil {
			return err
		}
		x.tx = &endgameCatalogTx{Tx: x.tx, catalog: c, items: x.itemDefinitions}
		return nil
	case *txView:
		return set(&x.m.endgameCatalog)
	}
	return fmt.Errorf("store: unsupported catalog injection")
}

type endgameCatalogQuerier struct {
	querier
	catalog *domain.EndgameCatalog
	items   map[domain.ItemID]domain.ItemDef
}

func (q endgameCatalogQuerier) EndgameCatalog() *domain.EndgameCatalog { return q.catalog }

type endgameCatalogTx struct {
	pgx.Tx
	catalog *domain.EndgameCatalog
	items   map[domain.ItemID]domain.ItemDef
}

func (q *endgameCatalogTx) EndgameCatalog() *domain.EndgameCatalog { return q.catalog }
func catalogOfQuerier(q querier) *domain.EndgameCatalog {
	if p, ok := q.(interface{ EndgameCatalog() *domain.EndgameCatalog }); ok {
		return p.EndgameCatalog()
	}
	return nil
}
func catalogOfStore(s Store) *domain.EndgameCatalog {
	switch x := s.(type) {
	case *Postgres:
		return x.endgameCatalog
	case *Memory:
		return x.endgameCatalog
	case *pgTx:
		return x.endgameCatalog
	case *txView:
		return x.m.endgameCatalog
	}
	return nil
}
func (p *Postgres) catalogQuery() querier {
	return endgameCatalogQuerier{p.pool, p.endgameCatalog, p.itemDefinitions}
}
func (p *Postgres) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &endgameCatalogTx{Tx: tx, catalog: p.endgameCatalog, items: p.itemDefinitions}, nil
}
func (p *Postgres) beginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := p.pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &endgameCatalogTx{Tx: tx, catalog: p.endgameCatalog, items: p.itemDefinitions}, nil
}
