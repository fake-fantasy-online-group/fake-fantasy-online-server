package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrFamilyNotFound     = errors.New("家族不存在")
	ErrFamilyAlreadyIn    = errors.New("已经加入了家族")
	ErrFamilyNotIn        = errors.New("尚未加入家族")
	ErrFamilyNameTaken    = errors.New("家族名称已经存在")
	ErrFamilyFull         = errors.New("家族成员已满")
	ErrFamilyResisting    = errors.New("该家族暂不接受申请")
	ErrFamilyPermission   = errors.New("没有执行此操作的家族权限")
	ErrFamilyLeaderLeave  = errors.New("族长必须先转让职位才能退出家族")
	ErrFamilyInvalid      = errors.New("家族参数无效")
	ErrFamilyTarget       = errors.New("目标不是本家族成员")
	ErrFamilyLevelTooLow  = errors.New("角色等级不足")
	ErrFamilyPositionFull = errors.New("该职位人数已满")
)

func (p *Postgres) LoadFamily(ctx context.Context, charID int64) (*domain.Family, error) {
	return loadFamily(ctx, p.pool, charID)
}
func (t *pgTx) LoadFamily(ctx context.Context, charID int64) (*domain.Family, error) {
	return loadFamily(ctx, t.tx, charID)
}

func loadFamily(ctx context.Context, q querier, charID int64) (*domain.Family, error) {
	var f domain.Family
	var myPos int16
	if err := q.QueryRow(ctx, `
		SELECT f.id,f.name,f.level,l.max_member,f.proclaim,f.resist,f.reputation,f.wealth,
		       me.position_id,f.leader_char_id,f.created_at,
		       COALESCE(p.invite,0)<>0,COALESCE(p.send_fam_mail,0)<>0
		  FROM game_family_members me
		  JOIN game_families f ON f.id=me.family_id
		  JOIN gamedata.ov_famlevel l ON l.fam_level=f.level
		  LEFT JOIN gamedata.ov_famperm p ON p.perm_level=me.position_id
		 WHERE me.char_id=$1`, charID).Scan(&f.ID, &f.Name, &f.Level, &f.MemberCap,
		&f.Proclaim, &f.Resist, &f.Reputation, &f.Wealth, &myPos, &f.Leader,
		&f.CreatedAt, &f.CanInvite, &f.CanMail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: 读取家族: %w", err)
	}
	f.MyPosition = domain.FamilyPositionID(myPos)
	rows, err := q.Query(ctx, `
		SELECT c.id,c.name,c.level,c.race,m.position_id,
		       COALESCE(n.name,pos.name),m.contribution
		  FROM game_family_members m
		  JOIN characters c ON c.id=m.char_id
		  JOIN gamedata.ov_famposperm pos ON pos.fam_pos=m.position_id
		  LEFT JOIN game_family_position_names n
		    ON n.family_id=m.family_id AND n.position_id=m.position_id
		 WHERE m.family_id=$1
		 ORDER BY m.position_id,m.joined_at,c.id`, f.ID)
	if err != nil {
		return nil, fmt.Errorf("store: 查询家族成员: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.FamilyMember
		var race, pos int16
		if err := rows.Scan(&m.Char, &m.Name, &m.Level, &race, &pos,
			&m.PositionName, &m.Contribution); err != nil {
			return nil, fmt.Errorf("store: 读取家族成员: %w", err)
		}
		m.Race, m.Position = domain.Race(race), domain.FamilyPositionID(pos)
		f.Members = append(f.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: 遍历家族成员: %w", err)
	}
	return &f, nil
}

func (p *Postgres) BrowseFamilies(ctx context.Context) ([]domain.FamilyBrowseEntry, error) {
	return browseFamilies(ctx, p.pool)
}
func (t *pgTx) BrowseFamilies(ctx context.Context) ([]domain.FamilyBrowseEntry, error) {
	return browseFamilies(ctx, t.tx)
}

func browseFamilies(ctx context.Context, q querier) ([]domain.FamilyBrowseEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT f.id,f.name,f.level,count(m.char_id),l.max_member,f.proclaim,f.reputation,f.created_at
		  FROM game_families f
		  JOIN gamedata.ov_famlevel l ON l.fam_level=f.level
		  LEFT JOIN game_family_members m ON m.family_id=f.id
		 GROUP BY f.id,l.max_member
		 ORDER BY f.level DESC,f.reputation DESC,f.id
		 LIMIT 255`)
	if err != nil {
		return nil, fmt.Errorf("store: 查询家族列表: %w", err)
	}
	defer rows.Close()
	var out []domain.FamilyBrowseEntry
	for rows.Next() {
		var e domain.FamilyBrowseEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Level, &e.Members, &e.MemberCap,
			&e.Proclaim, &e.Reputation, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *Postgres) FamilyPositions(ctx context.Context, charID int64) ([]domain.FamilyPosition, error) {
	return familyPositions(ctx, p.pool, charID)
}
func (t *pgTx) FamilyPositions(ctx context.Context, charID int64) ([]domain.FamilyPosition, error) {
	return familyPositions(ctx, t.tx, charID)
}

func familyPositions(ctx context.Context, q querier, charID int64) ([]domain.FamilyPosition, error) {
	rows, err := q.Query(ctx, `
		SELECT pos.fam_pos,COALESCE(n.name,pos.name),pos.desc_,pos.perm_level
		  FROM game_family_members me
		  JOIN game_families f ON f.id=me.family_id
		  JOIN gamedata.ov_famlevel l ON l.fam_level=f.level
		  JOIN gamedata.ov_famposperm pos
		    ON pos.fam_pos IN (1,2,10)
		    OR pos.fam_pos BETWEEN 3 AND 2+l.max_custom_pos
		  LEFT JOIN game_family_position_names n
		    ON n.family_id=f.id AND n.position_id=pos.fam_pos
		 WHERE me.char_id=$1
		 ORDER BY pos.fam_pos`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FamilyPosition
	for rows.Next() {
		var p domain.FamilyPosition
		var id, perm int16
		if err := rows.Scan(&id, &p.Name, &p.Description, &perm); err != nil {
			return nil, err
		}
		p.ID, p.PermLevel = domain.FamilyPositionID(id), uint8(perm)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (p *Postgres) FamilyByName(ctx context.Context, name string) (int64, domain.CharID, bool, error) {
	return familyByName(ctx, p.pool, name)
}
func (t *pgTx) FamilyByName(ctx context.Context, name string) (int64, domain.CharID, bool, error) {
	return familyByName(ctx, t.tx, name)
}
func familyByName(ctx context.Context, q querier, name string) (int64, domain.CharID, bool, error) {
	var id, leader int64
	var resist bool
	err := q.QueryRow(ctx, `SELECT id,leader_char_id,resist FROM game_families WHERE lower(name)=lower($1)`, strings.TrimSpace(name)).Scan(&id, &leader, &resist)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, ErrFamilyNotFound
	}
	return id, domain.CharID(leader), resist, err
}

func (p *Postgres) CreateFamily(ctx context.Context, charID int64, name, proclaim string) error {
	return p.WithTx(ctx, func(raw Store) error {
		return raw.(FamilyStore).CreateFamily(ctx, charID, name, proclaim)
	})
}
func (t *pgTx) CreateFamily(ctx context.Context, charID int64, name, proclaim string) error {
	return createFamily(ctx, t.tx, charID, name, proclaim)
}
func createFamily(ctx context.Context, q querier, charID int64, name, proclaim string) error {
	name, proclaim = strings.TrimSpace(name), strings.TrimSpace(proclaim)
	if name == "" || len([]rune(name)) > 30 || len([]rune(proclaim)) > 500 {
		return ErrFamilyInvalid
	}
	var familyID int64
	err := q.QueryRow(ctx, `INSERT INTO game_families(name,proclaim,leader_char_id)
		VALUES($1,$2,$3) RETURNING id`, name, proclaim, charID).Scan(&familyID)
	if err != nil {
		if isDup(err) {
			return ErrFamilyNameTaken
		}
		return err
	}
	if _, err := q.Exec(ctx, `INSERT INTO game_family_members(family_id,char_id,position_id)
		VALUES($1,$2,1)`, familyID, charID); err != nil {
		if isDup(err) {
			return ErrFamilyAlreadyIn
		}
		return err
	}
	return nil
}

func (p *Postgres) JoinFamily(ctx context.Context, charID, familyID int64, invited bool) error {
	return p.WithTx(ctx, func(raw Store) error {
		return raw.(FamilyStore).JoinFamily(ctx, charID, familyID, invited)
	})
}
func (t *pgTx) JoinFamily(ctx context.Context, charID, familyID int64, invited bool) error {
	var level int32
	if err := t.tx.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1 FOR UPDATE`, charID).Scan(&level); err != nil {
		return ErrFamilyTarget
	}
	if level < domain.FamilyMinJoinLevel {
		return ErrFamilyLevelTooLow
	}
	var familyLevel int16
	var resist bool
	if err := t.tx.QueryRow(ctx, `SELECT level,resist FROM game_families WHERE id=$1 FOR UPDATE`, familyID).Scan(&familyLevel, &resist); err != nil {
		return ErrFamilyNotFound
	}
	var cap, count int
	if err := t.tx.QueryRow(ctx, `SELECT max_member FROM gamedata.ov_famlevel WHERE fam_level=$1`, familyLevel).Scan(&cap); err != nil {
		return err
	}
	if err := t.tx.QueryRow(ctx, `SELECT count(*) FROM game_family_members WHERE family_id=$1`, familyID).Scan(&count); err != nil {
		return err
	}
	if resist && !invited {
		return ErrFamilyResisting
	}
	if count >= cap {
		return ErrFamilyFull
	}
	if _, err := t.tx.Exec(ctx, `INSERT INTO game_family_members(family_id,char_id,position_id)
		VALUES($1,$2,10)`, familyID, charID); err != nil {
		if isDup(err) {
			return ErrFamilyAlreadyIn
		}
		return err
	}
	return nil
}

func (p *Postgres) LeaveFamily(ctx context.Context, charID int64) error {
	return leaveFamily(ctx, p.pool, charID)
}
func (t *pgTx) LeaveFamily(ctx context.Context, charID int64) error {
	return leaveFamily(ctx, t.tx, charID)
}
func leaveFamily(ctx context.Context, q querier, charID int64) error {
	var pos int16
	if err := q.QueryRow(ctx, `SELECT position_id FROM game_family_members WHERE char_id=$1`, charID).Scan(&pos); err != nil {
		return ErrFamilyNotIn
	}
	if pos == int16(domain.FamilyLeader) {
		return ErrFamilyLeaderLeave
	}
	_, err := q.Exec(ctx, `DELETE FROM game_family_members WHERE char_id=$1`, charID)
	return err
}

func familyActor(ctx context.Context, q querier, actorID int64) (familyID int64, position int16, err error) {
	err = q.QueryRow(ctx, `SELECT family_id,position_id FROM game_family_members WHERE char_id=$1`, actorID).Scan(&familyID, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrFamilyNotIn
	}
	return
}

func familyHasPermission(ctx context.Context, q querier, actorID int64, column string) (int64, int16, error) {
	familyID, pos, err := familyActor(ctx, q, actorID)
	if err != nil {
		return 0, 0, err
	}
	allowed := false
	query := `SELECT COALESCE(` + column + `,0)<>0 FROM gamedata.ov_famperm WHERE perm_level=$1`
	if err := q.QueryRow(ctx, query, pos).Scan(&allowed); err != nil || !allowed {
		return 0, 0, ErrFamilyPermission
	}
	return familyID, pos, nil
}

func (p *Postgres) SetFamilyProclaim(ctx context.Context, actorID int64, proclaim string) error {
	return setFamilyProclaim(ctx, p.pool, actorID, proclaim)
}
func (t *pgTx) SetFamilyProclaim(ctx context.Context, actorID int64, proclaim string) error {
	return setFamilyProclaim(ctx, t.tx, actorID, proclaim)
}
func setFamilyProclaim(ctx context.Context, q querier, actorID int64, proclaim string) error {
	proclaim = strings.TrimSpace(proclaim)
	if len([]rune(proclaim)) > 500 {
		return ErrFamilyInvalid
	}
	familyID, _, err := familyHasPermission(ctx, q, actorID, "chg_fam_announce")
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE game_families SET proclaim=$1 WHERE id=$2`, proclaim, familyID)
	return err
}

func (p *Postgres) SetFamilyResist(ctx context.Context, actorID int64, resist bool) error {
	return setFamilyResist(ctx, p.pool, actorID, resist)
}
func (t *pgTx) SetFamilyResist(ctx context.Context, actorID int64, resist bool) error {
	return setFamilyResist(ctx, t.tx, actorID, resist)
}
func setFamilyResist(ctx context.Context, q querier, actorID int64, resist bool) error {
	familyID, pos, err := familyActor(ctx, q, actorID)
	if err != nil || pos != int16(domain.FamilyLeader) {
		return ErrFamilyPermission
	}
	_, err = q.Exec(ctx, `UPDATE game_families SET resist=$1 WHERE id=$2`, resist, familyID)
	return err
}

func (p *Postgres) SetFamilyMemberPosition(ctx context.Context, actorID int64, targetName string, position domain.FamilyPositionID) (domain.CharID, error) {
	return setFamilyMemberPosition(ctx, p.pool, actorID, targetName, position)
}
func (t *pgTx) SetFamilyMemberPosition(ctx context.Context, actorID int64, targetName string, position domain.FamilyPositionID) (domain.CharID, error) {
	return setFamilyMemberPosition(ctx, t.tx, actorID, targetName, position)
}
func setFamilyMemberPosition(ctx context.Context, q querier, actorID int64, targetName string, position domain.FamilyPositionID) (domain.CharID, error) {
	familyID, actorPos, err := familyActor(ctx, q, actorID)
	if err != nil || actorPos > int16(domain.FamilyElder) || position < domain.FamilyElder || position > domain.FamilyOrdinary {
		return 0, ErrFamilyPermission
	}
	var target domain.CharID
	var level int32
	if err := q.QueryRow(ctx, `SELECT c.id,c.level FROM game_family_members m JOIN characters c ON c.id=m.char_id
		WHERE m.family_id=$1 AND lower(c.name)=lower($2)`, familyID, strings.TrimSpace(targetName)).Scan(&target, &level); err != nil {
		return 0, ErrFamilyTarget
	}
	var minLevel int32
	if err := q.QueryRow(ctx, `SELECT member_min_level FROM gamedata.ov_famposperm WHERE fam_pos=$1`, position).Scan(&minLevel); err != nil || level < minLevel {
		return 0, ErrFamilyLevelTooLow
	}
	if position == domain.FamilyElder {
		var current, cap int
		if err := q.QueryRow(ctx, `SELECT count(*) FILTER(WHERE m.position_id=2),l.max_presbyter
			FROM game_families f JOIN gamedata.ov_famlevel l ON l.fam_level=f.level
			LEFT JOIN game_family_members m ON m.family_id=f.id WHERE f.id=$1 GROUP BY l.max_presbyter`, familyID).Scan(&current, &cap); err != nil || current >= cap {
			return 0, ErrFamilyPositionFull
		}
	}
	if _, err := q.Exec(ctx, `UPDATE game_family_members SET position_id=$1 WHERE family_id=$2 AND char_id=$3`, position, familyID, target); err != nil {
		return 0, err
	}
	return target, nil
}

func (p *Postgres) TransferFamilyLeader(ctx context.Context, actorID int64, targetName string) (domain.CharID, error) {
	var target domain.CharID
	err := p.WithTx(ctx, func(raw Store) error {
		var err error
		target, err = raw.(FamilyStore).TransferFamilyLeader(ctx, actorID, targetName)
		return err
	})
	return target, err
}
func (t *pgTx) TransferFamilyLeader(ctx context.Context, actorID int64, targetName string) (domain.CharID, error) {
	familyID, pos, err := familyActor(ctx, t.tx, actorID)
	if err != nil || pos != int16(domain.FamilyLeader) {
		return 0, ErrFamilyPermission
	}
	var target domain.CharID
	if err := t.tx.QueryRow(ctx, `SELECT c.id FROM game_family_members m JOIN characters c ON c.id=m.char_id
		WHERE m.family_id=$1 AND lower(c.name)=lower($2) AND c.id<>$3 FOR UPDATE OF m`, familyID, strings.TrimSpace(targetName), actorID).Scan(&target); err != nil {
		return 0, ErrFamilyTarget
	}
	if _, err := t.tx.Exec(ctx, `UPDATE game_family_members SET position_id=CASE char_id WHEN $1 THEN 2 ELSE 1 END
		WHERE family_id=$2 AND char_id IN ($1,$3)`, actorID, familyID, target); err != nil {
		return 0, err
	}
	_, err = t.tx.Exec(ctx, `UPDATE game_families SET leader_char_id=$1 WHERE id=$2`, target, familyID)
	return target, err
}

func (p *Postgres) KickFamilyMember(ctx context.Context, actorID int64, targetName string) (domain.CharID, error) {
	return kickFamilyMember(ctx, p.pool, actorID, targetName)
}
func (t *pgTx) KickFamilyMember(ctx context.Context, actorID int64, targetName string) (domain.CharID, error) {
	return kickFamilyMember(ctx, t.tx, actorID, targetName)
}
func kickFamilyMember(ctx context.Context, q querier, actorID int64, targetName string) (domain.CharID, error) {
	familyID, actorPos, err := familyHasPermission(ctx, q, actorID, "dismiss_member")
	if err != nil {
		return 0, err
	}
	var target domain.CharID
	var targetPos int16
	if err := q.QueryRow(ctx, `SELECT c.id,m.position_id FROM game_family_members m JOIN characters c ON c.id=m.char_id
		WHERE m.family_id=$1 AND lower(c.name)=lower($2)`, familyID, strings.TrimSpace(targetName)).Scan(&target, &targetPos); err != nil || target == domain.CharID(actorID) || targetPos <= actorPos {
		return 0, ErrFamilyTarget
	}
	_, err = q.Exec(ctx, `DELETE FROM game_family_members WHERE family_id=$1 AND char_id=$2`, familyID, target)
	return target, err
}

func (p *Postgres) RenameFamilyPosition(ctx context.Context, actorID int64, position domain.FamilyPositionID, name string) error {
	return renameFamilyPosition(ctx, p.pool, actorID, position, name)
}
func (t *pgTx) RenameFamilyPosition(ctx context.Context, actorID int64, position domain.FamilyPositionID, name string) error {
	return renameFamilyPosition(ctx, t.tx, actorID, position, name)
}
func renameFamilyPosition(ctx context.Context, q querier, actorID int64, position domain.FamilyPositionID, name string) error {
	name = strings.TrimSpace(name)
	if position < 3 || position > 9 || name == "" || len([]rune(name)) > 20 {
		return ErrFamilyInvalid
	}
	familyID, pos, err := familyActor(ctx, q, actorID)
	if err != nil || pos != int16(domain.FamilyLeader) {
		return ErrFamilyPermission
	}
	var maxCustom int
	if err := q.QueryRow(ctx, `SELECT l.max_custom_pos FROM game_families f JOIN gamedata.ov_famlevel l ON l.fam_level=f.level WHERE f.id=$1`, familyID).Scan(&maxCustom); err != nil || int(position) > 2+maxCustom {
		return ErrFamilyInvalid
	}
	_, err = q.Exec(ctx, `INSERT INTO game_family_position_names(family_id,position_id,name)
		VALUES($1,$2,$3) ON CONFLICT(family_id,position_id) DO UPDATE SET name=EXCLUDED.name`, familyID, position, name)
	return err
}

func (p *Postgres) SendFamilyMail(ctx context.Context, actorID int64, senderName, title, body string, expiresAt time.Time) ([]domain.CharID, error) {
	var members []domain.CharID
	err := p.WithTx(ctx, func(raw Store) error {
		var err error
		members, err = raw.(FamilyStore).SendFamilyMail(ctx, actorID, senderName, title, body, expiresAt)
		return err
	})
	return members, err
}
func (t *pgTx) SendFamilyMail(ctx context.Context, actorID int64, senderName, title, body string, expiresAt time.Time) ([]domain.CharID, error) {
	return sendFamilyMail(ctx, t.tx, actorID, senderName, title, body, expiresAt)
}
func sendFamilyMail(ctx context.Context, q querier, actorID int64, senderName, title, body string, expiresAt time.Time) ([]domain.CharID, error) {
	if strings.TrimSpace(title) == "" || len([]rune(title)) > 60 || len([]rune(body)) > 1000 {
		return nil, ErrFamilyInvalid
	}
	familyID, _, err := familyHasPermission(ctx, q, actorID, "send_fam_mail")
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT char_id FROM game_family_members WHERE family_id=$1 ORDER BY char_id`, familyID)
	if err != nil {
		return nil, err
	}
	var members []domain.CharID
	for rows.Next() {
		var id domain.CharID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		members = append(members, id)
	}
	rows.Close()
	for _, id := range members {
		if _, err := q.Exec(ctx, `INSERT INTO character_mails
			(recipient_id,sender_id,sender_name,title,body,mail_type,expires_at)
			VALUES($1,$2,$3,$4,$5,1,$6)`, id, actorID, senderName, title, body, expiresAt); err != nil {
			return nil, err
		}
	}
	return members, nil
}

func (p *Postgres) FamilyMemberIDs(ctx context.Context, charID int64) ([]domain.CharID, error) {
	return familyMemberIDs(ctx, p.pool, charID)
}
func (t *pgTx) FamilyMemberIDs(ctx context.Context, charID int64) ([]domain.CharID, error) {
	return familyMemberIDs(ctx, t.tx, charID)
}
func familyMemberIDs(ctx context.Context, q querier, charID int64) ([]domain.CharID, error) {
	rows, err := q.Query(ctx, `SELECT peer.char_id FROM game_family_members me JOIN game_family_members peer ON peer.family_id=me.family_id WHERE me.char_id=$1 ORDER BY peer.char_id`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CharID
	for rows.Next() {
		var id domain.CharID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
