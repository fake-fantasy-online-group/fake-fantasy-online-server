package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Postgres 是 Store 的 PostgreSQL 实现(生产存储)。语义与 Memory 一致, 保证上层逻辑
// 在两种实现下行为相同。连接池由 pgxpool 管理; 事务用真实 DB 事务。
type Postgres struct {
	pool *pgxpool.Pool
}

// Open 建立连接池。dsn 例: postgres://fantasy:fantasy_dev@localhost:5433/fantasy
func Open(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 16
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() error { p.pool.Close(); return nil }

// ReadOnly 返回一个只读查询口, 给 data 层加载静态配置用。
//
// 刻意只暴露 Query 一个方法: 静态配置层拿不到写的能力, 这条约束由类型保证,
// 比靠自觉可靠。返回的是 *pgxpool.Pool, 但调用方看到的类型只有 Query。
func (p *Postgres) ReadOnly() interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
} {
	return p.pool
}

// querier 抽象 pool 与 tx, 让 CRUD 逻辑在事务内外复用同一份代码。
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ── 账号 ──

func (p *Postgres) AccountByName(ctx context.Context, username string) (*Account, error) {
	return accountByName(ctx, p.pool, username)
}
func accountByName(ctx context.Context, q querier, username string) (*Account, error) {
	var a Account
	var bannedUntil *time.Time
	var gmLevel int16
	err := q.QueryRow(ctx,
		`SELECT id, username, pass_hash, sec_hash, banned, banned_until, gm_level, caiyu FROM accounts WHERE username=$1`, username).
		Scan(&a.ID, &a.Username, &a.PassHash, &a.SecHash, &a.Banned, &bannedUntil, &gmLevel, &a.Caiyu)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if bannedUntil != nil {
		a.BannedUntil = *bannedUntil
	}
	a.GMLevel = GMLevel(gmLevel)
	return &a, nil
}

func (p *Postgres) SetAccountBanned(ctx context.Context, accountID int64, banned bool) error {
	return setAccountBanned(ctx, p.pool, accountID, banned)
}

func setAccountBanned(ctx context.Context, q querier, accountID int64, banned bool) error {
	ct, err := q.Exec(ctx,
		`UPDATE accounts SET banned=$2, banned_until=NULL WHERE id=$1`, accountID, banned)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	return nil
}

func (p *Postgres) SetAccountBannedUntil(ctx context.Context, accountID int64, until time.Time) error {
	return setAccountBannedUntil(ctx, p.pool, accountID, until)
}

func setAccountBannedUntil(ctx context.Context, q querier, accountID int64, until time.Time) error {
	ct, err := q.Exec(ctx,
		`UPDATE accounts SET banned=FALSE, banned_until=$2 WHERE id=$1`, accountID, until)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	return nil
}

func (p *Postgres) CreateAccount(ctx context.Context, username, passHash string) (*Account, error) {
	return createAccount(ctx, p.pool, username, passHash, "")
}
func (p *Postgres) CreateRegisteredAccount(ctx context.Context, username, passHash, secHash string) (*Account, error) {
	return createAccount(ctx, p.pool, username, passHash, secHash)
}
func createAccount(ctx context.Context, q querier, username, passHash, secHash string) (*Account, error) {
	var a Account
	a.Username, a.PassHash, a.SecHash = username, passHash, secHash
	err := q.QueryRow(ctx,
		`INSERT INTO accounts(username, pass_hash, sec_hash) VALUES($1,$2,$3) RETURNING id`,
		username, passHash, secHash).Scan(&a.ID)
	if isDup(err) {
		return nil, ErrDup
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (p *Postgres) UpdateAccountPasswordHash(ctx context.Context, accountID int64, passHash string) error {
	return updateAccountPasswordHash(ctx, p.pool, accountID, passHash)
}

func (p *Postgres) CompareAndSwapAccountCredentials(ctx context.Context, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error) {
	return compareAndSwapAccountCredentials(ctx, p.pool, accountID, oldPass, oldSec, newPass, newSec)
}

func compareAndSwapAccountCredentials(ctx context.Context, q querier, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error) {
	ct, err := q.Exec(ctx, `UPDATE accounts SET pass_hash=$4, sec_hash=$5
		WHERE id=$1 AND pass_hash=$2 AND sec_hash=$3`, accountID, oldPass, oldSec, newPass, newSec)
	return err == nil && ct.RowsAffected() == 1, err
}

func updateAccountPasswordHash(ctx context.Context, q querier, accountID int64, passHash string) error {
	ct, err := q.Exec(ctx, `UPDATE accounts SET pass_hash=$2 WHERE id=$1`, accountID, passHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	return nil
}

func (p *Postgres) UpdateAccountSecurityHash(ctx context.Context, accountID int64, securityHash string) error {
	return updateAccountSecurityHash(ctx, p.pool, accountID, securityHash)
}

func updateAccountSecurityHash(ctx context.Context, q querier, accountID int64, securityHash string) error {
	ct, err := q.Exec(ctx, `UPDATE accounts SET sec_hash=$2 WHERE id=$1`, accountID, securityHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return fmt.Errorf("store: account %d not found", accountID)
	}
	return nil
}

// ── 角色 ──

func scanChar(row pgx.Row) (*domain.Character, error) {
	var c domain.Character
	var sceneInstance int64
	var race, gender, hair, face int16
	var atkVariant, weaponCType int16
	var equipView []byte
	var glowMode int16
	var pkMode int16
	var hotbarIDs []int32
	var hotbarKinds []byte
	var savedStatuses []byte
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &race, &c.Level, &c.Exp,
		&c.Pos.MapID, &c.Pos.X, &c.Pos.Y, &gender, &hair, &face,
		&equipView, &c.CreatedAt, &c.LastLogin,
		&c.FreePoints, &c.BagSlots,
		&c.Base.STR, &c.Base.VIT, &c.Base.INT, &c.Base.SPI, &c.Base.AGI, &c.Base.DEX,
		&c.Money.Gold, &c.Money.Silver, &c.Money.Copper, &c.Honor, &c.Nianli,
		&c.Stamina, &c.StaminaDay,
		&atkVariant, &weaponCType, &c.Appear.AtkDist, &c.Attrs,
		&c.SkillPoints,
		&c.Appear.Title, &c.Appear.Aura, &c.Appear.SoulFX, &c.Appear.TitleFX,
		&glowMode, &c.Appear.GlowUnlocked, &c.Appear.EquipFXHideMask,
		&c.SmartCast, &c.PetViewMask, &hotbarIDs, &hotbarKinds, &c.HotbarExpanded,
		&c.Employed, &pkMode, &c.Caiyu, &sceneInstance, &c.OwnedTitles, &c.Trial.Day, &c.Trial.DailyCompleted, &c.Trial.Cycle, &c.Trial.MonsterLevel,
		&savedStatuses, &c.ExpBonusPct, &c.ExpBonusRemainingTicks, &c.PetExpBonusPct, &c.PetExpBonusRemainingTicks, &c.Trial.KeyReward, &c.Trial.CompletionID)
	if err != nil {
		return nil, err
	}
	if c.LastLogin.Equal(time.Unix(0, 0).UTC()) {
		c.LastLogin = time.Time{}
	}
	if err := json.Unmarshal(savedStatuses, &c.SavedStatuses); err != nil {
		return nil, fmt.Errorf("store: 角色 %d 状态快照无效: %w", c.ID, err)
	}
	if sceneInstance < 0 || sceneInstance > int64(^uint32(0)) {
		return nil, fmt.Errorf("store: 角色 %d 的副本实例号非法: %d", c.ID, sceneInstance)
	}
	c.SceneInstance = uint32(sceneInstance)
	c.Race = domain.Race(race)
	c.Appear.Gender, c.Appear.Hair, c.Appear.Head = uint8(gender), uint8(hair), uint8(face)
	c.Appear.AtkVariant, c.Appear.WeaponCType = uint8(atkVariant), uint8(weaponCType)
	c.Appear.GlowMode = uint8(glowMode)
	c.PKMode = uint8(pkMode)
	c.Appear.GlowModeKnown = true
	c.SkillPointsKnown = true
	c.EmploymentKnown = true
	c.NianliKnown = true
	for i := 0; i < 6 && i*2+1 < len(equipView); i++ {
		c.Appear.EquipView[i] = uint16(equipView[i*2]) | uint16(equipView[i*2+1])<<8
	}
	for i := range c.Hotbar {
		if i < len(hotbarIDs) {
			c.Hotbar[i].ID = hotbarIDs[i]
		}
		if i < len(hotbarKinds) {
			c.Hotbar[i].Kind = hotbarKinds[i]
		}
	}
	return &c, nil
}

const charCols = `id, account_id, slot, name, race, level, exp, map_id, pos_x, pos_y,
	gender, hair, face, equip_view, created_at, last_login,
	free_points, bag_slots, base_str, base_vit, base_int, base_spi, base_agi, base_dex,
	money_gold, money_silver, money_copper, honor, nianli, stamina, stamina_day,
	atk_variant, weapon_ctype, atk_dist, attrs,
	skill_points, title, aura, soul_fx, title_fx, glow_mode, glow_unlocked,
	equip_fx_hide_mask, smart_cast, pet_view_mask, hotbar_ids, hotbar_kinds, hotbar_expanded,
	employed, pk_mode, (SELECT a.caiyu FROM accounts a WHERE a.id=characters.account_id),
	scene_instance,
		ARRAY(SELECT ct.title FROM character_titles ct WHERE ct.char_id=characters.id ORDER BY ct.title), trial_day, trial_daily_completed, trial_cycle, trial_monster_level,
		saved_statuses, exp_bonus_pct, exp_bonus_remaining_ticks, pet_exp_bonus_pct, pet_exp_bonus_remaining_ticks, trial_key_reward, trial_completion_id`

func (p *Postgres) CharsByAccount(ctx context.Context, accountID int64) ([]*domain.Character, error) {
	return charsByAccount(ctx, p.pool, accountID)
}
func charsByAccount(ctx context.Context, q querier, accountID int64) ([]*domain.Character, error) {
	rows, err := q.Query(ctx,
		`SELECT `+charCols+` FROM characters WHERE account_id=$1 ORDER BY slot`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Character
	for rows.Next() {
		c, err := scanChar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *Postgres) CharByName(ctx context.Context, name string) (*domain.Character, error) {
	return charByName(ctx, p.pool, name)
}
func charByName(ctx context.Context, q querier, name string) (*domain.Character, error) {
	c, err := scanChar(q.QueryRow(ctx, `SELECT `+charCols+` FROM characters WHERE name=$1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCharNotFound
	}
	return c, err
}

func equipViewBytes(c *domain.Character) []byte {
	b := make([]byte, 12)
	for i := 0; i < 6; i++ {
		b[i*2] = byte(c.Appear.EquipView[i])
		b[i*2+1] = byte(c.Appear.EquipView[i] >> 8)
	}
	return b
}

func (p *Postgres) CreateChar(ctx context.Context, c *domain.Character) error {
	return createChar(ctx, p.pool, c)
}

// characters.last_login is NOT NULL; UTC epoch is the persisted form of the
// domain's zero time (never entered the world). Real login times pass through.
func lastLoginForStore(lastLogin time.Time) time.Time {
	if lastLogin.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return lastLogin
}
func createChar(ctx context.Context, q querier, c *domain.Character) error {
	err := q.QueryRow(ctx,
		`INSERT INTO characters(account_id, slot, name, race, level, exp, map_id, pos_x, pos_y,
			gender, hair, face, equip_view, created_at, last_login,
			free_points, bag_slots, base_str, base_vit, base_int, base_spi, base_agi, base_dex,
			money_gold, money_silver, money_copper, honor, nianli, stamina, stamina_day,
			atk_variant, weapon_ctype, atk_dist, attrs, employed, pk_mode, scene_instance)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		        $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37) RETURNING id`,
		c.AccountID, c.Slot, c.Name, int16(c.Race), c.Level, c.Exp,
		c.Pos.MapID, c.Pos.X, c.Pos.Y,
		int16(c.Appear.Gender), int16(c.Appear.Hair), int16(c.Appear.Head),
		equipViewBytes(c), c.CreatedAt, lastLoginForStore(c.LastLogin),
		c.FreePoints, bagSlotsOf(c),
		c.Base.STR, c.Base.VIT, c.Base.INT, c.Base.SPI, c.Base.AGI, c.Base.DEX,
		c.Money.Gold, c.Money.Silver, c.Money.Copper, c.Honor, c.EffectiveNianli(),
		c.Stamina, c.StaminaDay,
		int16(c.Appear.AtkVariant), int16(c.Appear.WeaponCType), c.Appear.AtkDist,
		attrsOf(c), c.Employed, int16(c.PKMode), int64(c.SceneInstance)).Scan(&c.ID)
	if constraint := duplicateConstraint(err); constraint != "" {
		if constraint == "characters_account_id_slot_key" {
			return domain.ErrSlotTaken
		}
		return domain.ErrNameTaken
	}
	return err
}

func (p *Postgres) SaveChar(ctx context.Context, c *domain.Character) error {
	return saveChar(ctx, p.pool, c)
}

func (p *Postgres) RenameCharacter(ctx context.Context, charID int64, newName string) error {
	ct, err := p.pool.Exec(ctx, `UPDATE characters SET name=$2 WHERE id=$1`, charID, newName)
	if constraint := duplicateConstraint(err); constraint != "" {
		return domain.ErrNameTaken
	}
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCharNotFound
	}
	return nil
}

// attrsOf 取属性数组。nil 要变成空数组: INT[] 列是 NOT NULL, 写 nil 会被拒。
func attrsOf(c *domain.Character) []int32 {
	if c.Attrs == nil {
		return []int32{}
	}
	return c.Attrs
}

// bagSlotsOf 取角色的背包格数, 0 视为默认值 —— 存 0 会让下次加载建出一个 0 格的包。
func bagSlotsOf(c *domain.Character) int32 {
	if c.BagSlots <= 0 {
		return domain.DefaultBagSlots
	}
	return c.BagSlots
}

func hotbarIDsOf(c *domain.Character) []int32 {
	ids := make([]int32, len(c.Hotbar))
	for i := range c.Hotbar {
		ids[i] = c.Hotbar[i].ID
	}
	return ids
}

func hotbarKindsOf(c *domain.Character) []byte {
	kinds := make([]byte, len(c.Hotbar))
	for i := range c.Hotbar {
		kinds[i] = c.Hotbar[i].Kind
	}
	return kinds
}

func saveChar(ctx context.Context, q querier, c *domain.Character) error {
	statuses := c.SavedStatuses
	if statuses == nil {
		statuses = []domain.SavedStatus{}
	}
	statusJSON, err := json.Marshal(statuses)
	if err != nil {
		return fmt.Errorf("store: 编码角色 %d 状态快照: %w", c.ID, err)
	}
	ct, err := q.Exec(ctx,
		`UPDATE characters SET level=$2, exp=$3, map_id=$4, pos_x=$5, pos_y=$6,
			gender=$7, hair=$8, face=$9, equip_view=$10, last_login=$11,
			free_points=$12, bag_slots=$13,
			base_str=$14, base_vit=$15, base_int=$16, base_spi=$17, base_agi=$18, base_dex=$19,
			money_gold=$20, money_silver=$21, money_copper=$22, honor=$23, nianli=$24,
			stamina=$25, stamina_day=$26,
			atk_variant=$27, weapon_ctype=$28, atk_dist=$29, attrs=$30,
			skill_points=$31,
			title=$32, aura=$33, soul_fx=$34, title_fx=$35,
			glow_mode=$36, glow_unlocked=$37, equip_fx_hide_mask=$38,
			smart_cast=$39, pet_view_mask=$40,
			hotbar_ids=$41, hotbar_kinds=$42, hotbar_expanded=$43,
			pk_mode=$44, employed=$45, scene_instance=$46,
			trial_day=$47, trial_daily_completed=$48, trial_cycle=$49, trial_monster_level=$50,
			saved_statuses=$51::jsonb, exp_bonus_pct=$52, exp_bonus_remaining_ticks=$53,
			pet_exp_bonus_pct=$54, pet_exp_bonus_remaining_ticks=$55, trial_key_reward=$56, trial_completion_id=$57
		 WHERE id=$1`,
		c.ID, c.Level, c.Exp, c.Pos.MapID, c.Pos.X, c.Pos.Y,
		int16(c.Appear.Gender), int16(c.Appear.Hair), int16(c.Appear.Head),
		equipViewBytes(c), lastLoginForStore(c.LastLogin),
		c.FreePoints, bagSlotsOf(c),
		c.Base.STR, c.Base.VIT, c.Base.INT, c.Base.SPI, c.Base.AGI, c.Base.DEX,
		c.Money.Gold, c.Money.Silver, c.Money.Copper, c.Honor, c.EffectiveNianli(),
		c.Stamina, c.StaminaDay,
		int16(c.Appear.AtkVariant), int16(c.Appear.WeaponCType), c.Appear.AtkDist,
		attrsOf(c), c.SkillPoints,
		c.Appear.Title, c.Appear.Aura, c.Appear.SoulFX, c.Appear.TitleFX,
		int16(c.Appear.GlowMode), c.Appear.GlowUnlocked, c.Appear.EquipFXHideMask,
		c.SmartCast, c.PetViewMask, hotbarIDsOf(c), hotbarKindsOf(c), c.HotbarExpanded,
		int16(c.PKMode), c.Employed, int64(c.SceneInstance), c.Trial.Day, c.Trial.DailyCompleted, c.Trial.Cycle, c.Trial.MonsterLevel,
		string(statusJSON), c.ExpBonusPct, c.ExpBonusRemainingTicks, c.PetExpBonusPct, c.PetExpBonusRemainingTicks, c.Trial.KeyReward, c.Trial.CompletionID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCharNotFound
	}
	return nil
}

func (p *Postgres) DeleteChar(ctx context.Context, id int64) error {
	ct, err := p.pool.Exec(ctx, `DELETE FROM characters WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCharNotFound
	}
	return nil
}

func (p *Postgres) LoadSeenTips(ctx context.Context, charID int64) ([]int32, error) {
	return loadSeenTips(ctx, p.pool, charID)
}

func loadSeenTips(ctx context.Context, q querier, charID int64) ([]int32, error) {
	rows, err := q.Query(ctx,
		`SELECT tip_id FROM character_seen_tips WHERE char_id=$1 ORDER BY tip_id`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var tipID int32
		if err := rows.Scan(&tipID); err != nil {
			return nil, err
		}
		out = append(out, tipID)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkTipSeen(ctx context.Context, charID int64, tipID int32) error {
	return markTipSeen(ctx, p.pool, charID, tipID)
}

func markTipSeen(ctx context.Context, q querier, charID int64, tipID int32) error {
	_, err := q.Exec(ctx,
		`INSERT INTO character_seen_tips(char_id, tip_id) VALUES($1,$2)
		 ON CONFLICT (char_id, tip_id) DO NOTHING`, charID, tipID)
	return err
}

// WithTx 在真实 DB 事务内执行 fn。fn 返回 error 则回滚。
func (p *Postgres) WithTx(ctx context.Context, fn func(Store) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(&pgTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// pgTx 是事务内的 Store 视图, 所有操作走同一个 tx。
type pgTx struct{ tx pgx.Tx }

func (t *pgTx) AccountByName(ctx context.Context, u string) (*Account, error) {
	return accountByName(ctx, t.tx, u)
}
func (t *pgTx) CreateAccount(ctx context.Context, u, h string) (*Account, error) {
	return createAccount(ctx, t.tx, u, h, "")
}
func (t *pgTx) CreateRegisteredAccount(ctx context.Context, u, h, s string) (*Account, error) {
	return createAccount(ctx, t.tx, u, h, s)
}
func (t *pgTx) UpdateAccountPasswordHash(ctx context.Context, accountID int64, passHash string) error {
	return updateAccountPasswordHash(ctx, t.tx, accountID, passHash)
}

func (t *pgTx) CompareAndSwapAccountCredentials(ctx context.Context, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error) {
	return compareAndSwapAccountCredentials(ctx, t.tx, accountID, oldPass, oldSec, newPass, newSec)
}

func (t *pgTx) UpdateAccountSecurityHash(ctx context.Context, accountID int64, securityHash string) error {
	return updateAccountSecurityHash(ctx, t.tx, accountID, securityHash)
}
func (t *pgTx) SetAccountBanned(ctx context.Context, accountID int64, banned bool) error {
	return setAccountBanned(ctx, t.tx, accountID, banned)
}
func (t *pgTx) SetAccountBannedUntil(ctx context.Context, accountID int64, until time.Time) error {
	return setAccountBannedUntil(ctx, t.tx, accountID, until)
}
func (t *pgTx) CharsByAccount(ctx context.Context, id int64) ([]*domain.Character, error) {
	return charsByAccount(ctx, t.tx, id)
}
func (t *pgTx) CharByName(ctx context.Context, n string) (*domain.Character, error) {
	return charByName(ctx, t.tx, n)
}
func (t *pgTx) CreateChar(ctx context.Context, c *domain.Character) error {
	return createChar(ctx, t.tx, c)
}
func (t *pgTx) SaveChar(ctx context.Context, c *domain.Character) error {
	return saveChar(ctx, t.tx, c)
}
func (t *pgTx) DeleteChar(ctx context.Context, id int64) error {
	ct, err := t.tx.Exec(ctx, `DELETE FROM characters WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCharNotFound
	}
	return nil
}
func (t *pgTx) LoadSeenTips(ctx context.Context, charID int64) ([]int32, error) {
	return loadSeenTips(ctx, t.tx, charID)
}
func (t *pgTx) MarkTipSeen(ctx context.Context, charID int64, tipID int32) error {
	return markTipSeen(ctx, t.tx, charID, tipID)
}
func (t *pgTx) WithTx(ctx context.Context, fn func(Store) error) error { return fn(t) } // 已在事务内
func (t *pgTx) Close() error                                           { return nil }

// isDup 判断是否唯一约束冲突(Postgres 错误码 23505)。
func isDup(err error) bool {
	return duplicateConstraint(err) != ""
}

func duplicateConstraint(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return ""
	}
	return pgErr.ConstraintName
}
