package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrApprenticeLevel       = errors.New("师父必须高于30级，徒弟必须低于30级")
	ErrApprenticeHasMaster   = errors.New("该角色已经有师父")
	ErrApprenticeCapacity    = errors.New("师父的徒弟名额已满")
	ErrApprenticeInvalidPair = errors.New("师徒目标无效")
	ErrApprenticeNotFound    = errors.New("双方没有师徒关系")
)

func (p *Postgres) CheckApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	return checkApprenticeship(ctx, p.pool, masterID, apprenticeID)
}
func (t *pgTx) CheckApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	return checkApprenticeship(ctx, t.tx, masterID, apprenticeID)
}

// Request-time validation is read-only. CreateApprenticeship repeats every
// constraint under the transaction locks when the other player accepts.
func checkApprenticeship(ctx context.Context, q querier, masterID, apprenticeID int64) error {
	if masterID <= 0 || apprenticeID <= 0 || masterID == apprenticeID {
		return ErrApprenticeInvalidPair
	}
	var masterLevel, apprenticeLevel int32
	if err := q.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1`, masterID).Scan(&masterLevel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApprenticeInvalidPair
		}
		return err
	}
	if err := q.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1`, apprenticeID).Scan(&apprenticeLevel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApprenticeInvalidPair
		}
		return err
	}
	if masterLevel < domain.MasterMinLevel || apprenticeLevel > domain.ApprenticeMaxLevel {
		return ErrApprenticeLevel
	}
	var hasMaster bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM game_apprenticeships WHERE apprentice_char_id=$1)`, apprenticeID).Scan(&hasMaster); err != nil {
		return err
	}
	if hasMaster {
		return ErrApprenticeHasMaster
	}
	capacity := domain.DefaultApprentices
	if err := q.QueryRow(ctx, `SELECT capacity FROM game_master_profiles WHERE master_char_id=$1`, masterID).Scan(&capacity); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM game_apprenticeships WHERE master_char_id=$1`, masterID).Scan(&count); err != nil {
		return err
	}
	if count >= capacity {
		return ErrApprenticeCapacity
	}
	return nil
}

func (p *Postgres) LoadApprenticeSnapshot(ctx context.Context, charID int64) (domain.ApprenticeSnapshot, error) {
	return loadApprenticeSnapshot(ctx, p.pool, charID)
}
func (t *pgTx) LoadApprenticeSnapshot(ctx context.Context, charID int64) (domain.ApprenticeSnapshot, error) {
	return loadApprenticeSnapshot(ctx, t.tx, charID)
}

func loadApprenticeSnapshot(ctx context.Context, q querier, charID int64) (domain.ApprenticeSnapshot, error) {
	var out domain.ApprenticeSnapshot
	var master domain.SocialPerson
	err := q.QueryRow(ctx, `SELECT c.id,c.name,c.level FROM game_apprenticeships a
		JOIN characters c ON c.id=a.master_char_id WHERE a.apprentice_char_id=$1`, charID).
		Scan(&master.Char, &master.Name, &master.Level)
	if err == nil {
		out.Master = &master
		rows, err := q.Query(ctx, `SELECT c.id,c.name,c.level FROM game_apprenticeships mine
			JOIN game_apprenticeships peer ON peer.master_char_id=mine.master_char_id
			JOIN characters c ON c.id=peer.apprentice_char_id
			WHERE mine.apprentice_char_id=$1 AND peer.apprentice_char_id<>$1
			ORDER BY peer.created_at,c.id`, charID)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var peer domain.SocialPerson
			if err := rows.Scan(&peer.Char, &peer.Name, &peer.Level); err != nil {
				rows.Close()
				return out, err
			}
			out.Peers = append(out.Peers, peer)
		}
		rows.Close()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("store: 读取师父: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT c.id,c.name,c.level FROM game_apprenticeships a
		JOIN characters c ON c.id=a.apprentice_char_id WHERE a.master_char_id=$1
		ORDER BY a.created_at,c.id`, charID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var apprentice domain.SocialPerson
		if err := rows.Scan(&apprentice.Char, &apprentice.Name, &apprentice.Level); err != nil {
			return out, err
		}
		out.Apprentices = append(out.Apprentices, apprentice)
	}
	return out, rows.Err()
}

func (p *Postgres) CreateApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	return p.WithTx(ctx, func(raw Store) error {
		return raw.(ApprenticeStore).CreateApprenticeship(ctx, masterID, apprenticeID)
	})
}

func (p *Postgres) DeleteApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	return p.WithTx(ctx, func(raw Store) error {
		return raw.(ApprenticeStore).DeleteApprenticeship(ctx, masterID, apprenticeID)
	})
}

func (t *pgTx) DeleteApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	if masterID <= 0 || apprenticeID <= 0 || masterID == apprenticeID {
		return ErrApprenticeInvalidPair
	}
	// Match the creation lock order so concurrent establishment and removal
	// cannot observe a partially changed pair.
	for _, id := range []int64{masterID, apprenticeID} {
		var lockedID int64
		if err := t.tx.QueryRow(ctx, `SELECT id FROM characters WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrApprenticeInvalidPair
			}
			return err
		}
	}
	ct, err := t.tx.Exec(ctx, `DELETE FROM game_apprenticeships WHERE master_char_id=$1 AND apprentice_char_id=$2`, masterID, apprenticeID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return ErrApprenticeNotFound
	}
	return nil
}

func (t *pgTx) CreateApprenticeship(ctx context.Context, masterID, apprenticeID int64) error {
	if masterID <= 0 || apprenticeID <= 0 || masterID == apprenticeID {
		return ErrApprenticeInvalidPair
	}
	var masterLevel, apprenticeLevel int32
	if err := t.tx.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1 FOR UPDATE`, masterID).Scan(&masterLevel); err != nil {
		return ErrApprenticeInvalidPair
	}
	if err := t.tx.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1 FOR UPDATE`, apprenticeID).Scan(&apprenticeLevel); err != nil {
		return ErrApprenticeInvalidPair
	}
	if masterLevel < domain.MasterMinLevel || apprenticeLevel > domain.ApprenticeMaxLevel {
		return ErrApprenticeLevel
	}
	var hasMaster bool
	if err := t.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM game_apprenticeships WHERE apprentice_char_id=$1)`, apprenticeID).Scan(&hasMaster); err != nil {
		return err
	}
	if hasMaster {
		return ErrApprenticeHasMaster
	}
	if _, err := t.tx.Exec(ctx, `INSERT INTO game_master_profiles(master_char_id) VALUES($1) ON CONFLICT DO NOTHING`, masterID); err != nil {
		return err
	}
	var capacity, count int
	if err := t.tx.QueryRow(ctx, `SELECT p.capacity,count(a.apprentice_char_id)
		FROM game_master_profiles p LEFT JOIN game_apprenticeships a ON a.master_char_id=p.master_char_id
		WHERE p.master_char_id=$1 GROUP BY p.capacity`, masterID).Scan(&capacity, &count); err != nil {
		return err
	}
	if count >= capacity {
		return ErrApprenticeCapacity
	}
	_, err := t.tx.Exec(ctx, `INSERT INTO game_apprenticeships(master_char_id,apprentice_char_id) VALUES($1,$2)`, masterID, apprenticeID)
	if isDup(err) {
		return ErrApprenticeHasMaster
	}
	return err
}
