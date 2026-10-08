package data

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

// 从数据库加载静态配置。
//
// 静态配置预先导入 PostgreSQL，服务启动时读取到内存，并在运行期间保持只读。
//
// 与 store 的分工看**可变性**, 不看存储介质:
//
//	data(gamedata)  静态配置。开服全量读进内存, 之后只读, 永不写
//	store           玩家状态。随时读写, 要事务, 要写回队列
//
// 两个包都连同一个 Postgres, 这不矛盾 —— 它们碰的是完全不同的表。

// Querier 是本包需要的全部数据库能力: 只读查询。
// 用最小接口而不是 *pgxpool.Pool, 是为了让 data **拿不到写的能力** ——
// 静态配置层不该有能力改任何东西, 这条约束靠类型保证比靠自觉可靠。
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// MonsterDefs 是全部怪物模板, 按配置 id 索引。
type MonsterDefs map[domain.MonsterID]domain.MonsterDef

type WardrobeStats struct {
	Rows, Loaded, DuplicateIDs, UnsupportedCategory, MissingItem, MissingAppearance int
}

// LoadHairRules 读取客户端理发店的两组八项配置。mode=0 使用发色表，
// 同时消耗 DyeItem×DyeCount 与 Money；mode=1 使用发型表，只消耗 Money。
func LoadHairRules(ctx context.Context, q Querier, items ItemDefs) (domain.HairRules, error) {
	out := domain.HairRules{
		domain.HairColor: make(map[uint8]domain.HairOption, 8),
		domain.HairStyle: make(map[uint8]domain.HairOption, 8),
	}
	styleRows, err := q.Query(ctx, `
		SELECT h.index,r.value
		  FROM gamedata.ov_hairstyle h
		  JOIN gamedata.ov_hairstyle_req_entry r ON r.row_no=h.row_no
		 WHERE r.req_type=1
		 ORDER BY h.index`)
	if err != nil {
		return nil, fmt.Errorf("data: 查发型费用: %w", err)
	}
	for styleRows.Next() {
		var id int32
		var money int64
		if err := styleRows.Scan(&id, &money); err != nil {
			styleRows.Close()
			return nil, err
		}
		if id < 1 || id > 255 || money <= 0 {
			styleRows.Close()
			return nil, fmt.Errorf("data: 发型配置非法 id=%d money=%d", id, money)
		}
		out[domain.HairStyle][uint8(id)] = domain.HairOption{
			Mode: domain.HairStyle, ID: uint8(id), Money: money,
		}
	}
	if err := styleRows.Err(); err != nil {
		styleRows.Close()
		return nil, err
	}
	styleRows.Close()

	colorRows, err := q.Query(ctx, `
		SELECT h.index,
		       max(r.value) FILTER (WHERE r.req_type=1) AS money,
		       max(r.value) FILTER (WHERE r.req_type=2) AS dye_item,
		       max(r.res) FILTER (WHERE r.req_type=2) AS dye_count
		  FROM gamedata.ov_haircolor h
		  JOIN gamedata.ov_haircolor_req_entry r ON r.row_no=h.row_no
		 GROUP BY h.index ORDER BY h.index`)
	if err != nil {
		return nil, fmt.Errorf("data: 查发色费用: %w", err)
	}
	defer colorRows.Close()
	for colorRows.Next() {
		var id, dyeCount int32
		var money int64
		var dye domain.ItemID
		if err := colorRows.Scan(&id, &money, &dye, &dyeCount); err != nil {
			return nil, err
		}
		def, exists := items[dye]
		if id < 1 || id > 255 || money <= 0 || dyeCount <= 0 || !exists || !def.Stackable {
			return nil, fmt.Errorf("data: 发色配置非法 id=%d money=%d dye=%d count=%d",
				id, money, dye, dyeCount)
		}
		out[domain.HairColor][uint8(id)] = domain.HairOption{
			Mode: domain.HairColor, ID: uint8(id), Money: money,
			DyeItem: dye, DyeCount: dyeCount,
		}
	}
	if err := colorRows.Err(); err != nil {
		return nil, err
	}
	if len(out[domain.HairColor]) != 8 || len(out[domain.HairStyle]) != 8 {
		return nil, fmt.Errorf("data: 理发店配置不完整 colors=%d styles=%d",
			len(out[domain.HairColor]), len(out[domain.HairStyle]))
	}
	return out, nil
}

// LoadStallOpenItems 从客户端权威物品表读取两类开店凭证。名称与原版用途一一
// 对应：黄金古币开出售摊，白银古币开收购摊；场景只持这份已校验映射。
func LoadStallOpenItems(ctx context.Context, q Querier, items ItemDefs) (domain.StallOpenItems, error) {
	rows, err := q.Query(ctx, `
		SELECT index,name
		  FROM gamedata.ov_item
		 WHERE name IN ('黄金古币','白银古币')
		 ORDER BY index`)
	if err != nil {
		return nil, fmt.Errorf("data: 查开店古币: %w", err)
	}
	defer rows.Close()
	out := make(domain.StallOpenItems, 2)
	for rows.Next() {
		var id domain.ItemID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		def, ok := items[id]
		if !ok || !def.Stackable || !def.InventoryTabKnown {
			return nil, fmt.Errorf("data: 开店古币无有效物品定义 id=%d name=%q", id, name)
		}
		var typ domain.StallType
		switch name {
		case "黄金古币":
			typ = domain.StallSell
		case "白银古币":
			typ = domain.StallBuy
		default:
			continue
		}
		if old, duplicate := out[typ]; duplicate && old != id {
			return nil, fmt.Errorf("data: %s存在多个物品号 %d/%d", name, old, id)
		}
		out[typ] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !out.Valid() {
		return nil, fmt.Errorf("data: 开店古币配置不完整 sell=%d buy=%d",
			out[domain.StallSell], out[domain.StallBuy])
	}
	return out, nil
}

// LoadMonsters 读 game_monsters。
//
// 只读**有等级的**行: level=0 的 88 行是没匹配上数据的占位, 刷出来会是一只
// 0 级 0 血、生下来就死的怪 —— 那种东西进不了世界比进去好。
func LoadMonsters(ctx context.Context, q Querier) (MonsterDefs, error) {
	profiles, err := loadMonsterColorProfiles(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT m.id, m.name, m.kind, m.elite_id, m.level, m.hp, m.exp, m.sprite,
		       m.atk_min, m.atk_max, m.def, m.hit, m.matk, m.mdef, m.crit_rate, m.mcrit_rate,
		       m.atk_speed, m.move_speed,
		       m.aggressive, m.can_attack, m.basic_attack_enabled, m.call_help,
		       a.id, a.view_dist, a.trace_dist, a.basic_attack_dist, a.keep_dist,
		       a.help_radius, a.skill_check_ms, a.global_cooldown_ms
		  FROM game_monsters m
		  JOIN game_monster_ai_templates a ON a.id = m.ai_template
		 WHERE m.level > 0 AND m.hp > 0`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_monsters: %w", err)
	}
	defer rows.Close()

	out := MonsterDefs{}
	for rows.Next() {
		var (
			d                                    domain.MonsterDef
			kind                                 string
			atkMin, atkMax, def, hit, matk, mdef int32
			critRate, mcritRate                  int32
			atkSpeed, moveSpeed, expRaw          int32
			skillCheckMS, globalCooldownMS       int32
			canAttack, basicAttackEnabled        bool
		)
		if err := rows.Scan(&d.ID, &d.Name, &kind, &d.EliteID, &d.Level, &d.HP, &expRaw, &d.Sprite,
			&atkMin, &atkMax, &def, &hit, &matk, &mdef, &critRate, &mcritRate, &atkSpeed, &moveSpeed,
			&d.Aggressive, &canAttack, &basicAttackEnabled, &d.CallHelp,
			&d.AI.ID, &d.AI.ViewDist, &d.AI.TraceDist, &d.AI.BasicAttackDist, &d.AI.KeepDist,
			&d.AI.HelpRadius, &skillCheckMS, &globalCooldownMS); err != nil {
			return nil, fmt.Errorf("data: 读 game_monsters 行: %w", err)
		}
		d.Kind = domain.ParseMonsterKind(kind)
		d.ColorProfile = colorProfileForKind(profiles, kind)
		d.NoAttack = !canAttack
		d.NoBasicAttack = !basicAttackEnabled
		d.Exp = int64(expRaw)
		d.Stats = domain.NewMonsterStatsRange(atkMin, atkMax, def, hit, matk, mdef, atkSpeed, moveSpeed)
		d.Stats.CritRate, d.Stats.MCritRate = critRate, mcritRate
		d.ViewDist, d.TraceDist = d.AI.ViewDist, d.AI.TraceDist
		d.AI.SkillCheckEvery = domain.Ticks(int(skillCheckMS))
		d.AI.GlobalCooldown = domain.Ticks(int(globalCooldownMS))
		out[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_monsters: %w", err)
	}
	rows.Close()

	if err := loadMonsterSkills(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// colorProfileForKind 取某个类别词对应的染色档位。认不出的类别词与空 kind 共用
// 兜底档，和 domain.ParseMonsterKind 把未知词按普通怪处理保持一致。
func colorProfileForKind(profiles map[string]domain.ColorProfile, kind string) domain.ColorProfile {
	if profile, ok := profiles[strings.TrimSpace(kind)]; ok {
		return profile
	}
	return profiles[""]
}

// loadMonsterColorProfiles 读 game_monsters.kind → 染色档位（属性条数权重档）的映射。
//
// 空字符串键是“没见过的类别词”的兜底：domain.ParseMonsterKind 把认不出的词按
// 普通怪处理，这里也要给它一个明确档位，不能让它静默落到默认档。
func loadMonsterColorProfiles(ctx context.Context, q Querier) (map[string]domain.ColorProfile, error) {
	rows, err := q.Query(ctx, `SELECT kind, profile_id FROM game_monster_color_profiles`)
	if err != nil {
		return nil, fmt.Errorf("data: 查怪物染色档位映射: %w", err)
	}
	defer rows.Close()
	out := make(map[string]domain.ColorProfile)
	for rows.Next() {
		var kind string
		var profile int16
		if err := rows.Scan(&kind, &profile); err != nil {
			return nil, fmt.Errorf("data: 读怪物染色档位映射: %w", err)
		}
		if profile <= 0 {
			return nil, fmt.Errorf("data: 怪物染色档位非法 kind=%q profile=%d", kind, profile)
		}
		if _, dup := out[kind]; dup {
			return nil, fmt.Errorf("data: 怪物染色档位重复 kind=%q", kind)
		}
		out[kind] = domain.ColorProfile(profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历怪物染色档位映射: %w", err)
	}
	if _, ok := out[""]; !ok {
		return nil, fmt.Errorf("data: 怪物染色档位缺少空 kind 兜底行")
	}
	return out, nil
}

// ValidateColorProfiles 校验怪物实际用到的染色档位都在随机属性条数表里有配置。
// 缺档位会让这些怪掉出的装备静默全白板，所以必须在启动前拦住。
func ValidateColorProfiles(defs MonsterDefs, roll domain.EquipmentRollTable) error {
	var missing []string
	for id, d := range defs {
		if _, ok := roll.Counts[d.ColorProfile]; ok {
			continue
		}
		if len(missing) < 5 {
			missing = append(missing, fmt.Sprintf("%d(%s) 档位%d", id, d.Name, d.ColorProfile))
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("data: 怪物的染色档位没有条数配置: %s", strings.Join(missing, "、"))
	}
	return nil
}

type monsterSkillEffectRow struct {
	kind                                    string
	powerPct, healMaxPct, flatValue         int32
	statusID, statusLevelValue, durationSec int32
	chanceBP                                int32
	hurtTypeOverride                        int32
	statusLevelMode                         string
	summonMonster, summonCount              int32
	guaranteedHit, overrideHurtType         bool
}

// loadMonsterSkills 把三层数据闭合成可执行规则：怪物引用、客户端技能定义、服务端
// 结果配置。运行时只读闭合后的 MonsterDef，不再查库或按名字猜行为。
func loadMonsterSkills(ctx context.Context, q Querier, monsters MonsterDefs) error {
	groundEffects, err := loadGroundSkillEffects(ctx, q)
	if err != nil {
		return err
	}
	effects := map[domain.SkillKey][]monsterSkillEffectRow{}
	erows, err := q.Query(ctx, `
		SELECT skill_id, skill_lv, effect_kind, power_pct, heal_max_hp_pct, flat_value,
		       status_id, status_level_mode, status_level_value, duration_sec,
		       summon_monster_id, summon_count, chance_bp, guaranteed_hit,
		       override_hurt_type, hurt_type_override
		  FROM game_monster_skill_effects
		 ORDER BY skill_id, skill_lv, seq`)
	if err != nil {
		return fmt.Errorf("data: 查怪物技能结果: %w", err)
	}
	for erows.Next() {
		var id domain.SkillID
		var level int32
		var e monsterSkillEffectRow
		if err := erows.Scan(&id, &level, &e.kind, &e.powerPct, &e.healMaxPct, &e.flatValue,
			&e.statusID, &e.statusLevelMode, &e.statusLevelValue, &e.durationSec,
			&e.summonMonster, &e.summonCount, &e.chanceBP, &e.guaranteedHit,
			&e.overrideHurtType, &e.hurtTypeOverride); err != nil {
			erows.Close()
			return fmt.Errorf("data: 读怪物技能结果行: %w", err)
		}
		effects[domain.SkillKey{ID: id, Level: level}] = append(
			effects[domain.SkillKey{ID: id, Level: level}], e)
	}
	if err := erows.Err(); err != nil {
		erows.Close()
		return fmt.Errorf("data: 遍历怪物技能结果: %w", err)
	}
	erows.Close()

	rows, err := q.Query(ctx, `
		SELECT ms.monster_id, ms.slot, ms.kind, ms.skill_id, ms.skill_lv, ms.name,
		       ms.priority, ms.chance_bp, ms.cooldown_ms, ms.initial_delay_ms,
		       ms.min_hp_pct, ms.max_hp_pct, ms.max_casts_per_life,
		       ms.fallback_damage_pct, ms.enabled,
		       COALESCE(sd.name, ms.name), COALESCE(sd.small_map,0),
		       COALESCE(sd.prof,0), COALESCE(sd.skill_type,1), COALESCE(sd.hurt_type,0),
		       COALESCE(sd.desc_,''), COALESCE(sd.dist,0), COALESCE(sd.radius,0),
		       COALESCE(sd.prepare,0), COALESCE(sd.sep,0),
		       COALESCE(sd.target_self,0), COALESCE(sd.target_mon,0),
		       COALESCE(sd.target_team,0)
		  FROM game_monster_skills ms
		  LEFT JOIN LATERAL (
		       SELECT sd.*
		         FROM gamedata.ov_skilldesc sd
		        WHERE sd.skill_id = ms.skill_id
		        ORDER BY CASE WHEN sd.skill_level = ms.skill_lv THEN 0 ELSE 1 END,
		                 CASE WHEN sd.prof = 0 THEN 0 ELSE 1 END,
		                 ABS(sd.skill_level - GREATEST(ms.skill_lv,1))
		        LIMIT 1
		  ) sd ON true
		 ORDER BY ms.monster_id, ms.kind, ms.priority DESC, ms.slot`)
	if err != nil {
		return fmt.Errorf("data: 查怪物技能规则: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			monsterID                  domain.MonsterID
			kind, sourceName, desc     string
			rule                       domain.MonsterSkillRule
			level, fallbackDamage      int32
			cooldownMS, initialDelayMS int32
			prof, skillType, hurtType  int64
			sep                        int32
			tSelf, tMon, tTeam         int32
		)
		if err := rows.Scan(&monsterID, &rule.Slot, &kind, &rule.Skill.ID, &level, &sourceName,
			&rule.Priority, &rule.ChanceBP, &cooldownMS, &initialDelayMS,
			&rule.MinHPPct, &rule.MaxHPPct, &rule.MaxCastsPerLife,
			&fallbackDamage, &rule.Enabled,
			&rule.Skill.Name, &rule.Skill.Icon, &prof, &skillType, &hurtType,
			&desc, &rule.Skill.Dist, &rule.Skill.Radius, &rule.Skill.PrepareMS, &sep,
			&tSelf, &tMon, &tTeam); err != nil {
			return fmt.Errorf("data: 读怪物技能规则行: %w", err)
		}
		d, ok := monsters[monsterID]
		if !ok {
			continue
		}
		if kind == "immune" {
			if d.ImmuneSkills == nil {
				d.ImmuneSkills = map[domain.SkillID]struct{}{}
			}
			d.ImmuneSkills[rule.Skill.ID] = struct{}{}
			monsters[d.ID] = d
			continue
		}
		if level <= 0 {
			level = 1
		}
		// 原始怪物表的“对敌/对己”有一批与客户端目标位冲突：普通怪和精英怪
		// 的狂暴术、钢铁之躯等自用技能被写成了 enemy。客户端明确只有
		// target_self 时以客户端为准；kind='self' 仍保留给天魔之怒这类
		// 自身中心、实际伤害敌人的范围技能。
		rule.Self = kind == "self" || tSelf != 0 && tMon == 0 && tTeam == 0
		rule.Skill.Level, rule.Skill.Prof = level, int32(prof)
		rule.Skill.Kind = domain.ParseSkillKind(int32(skillType))
		rule.Skill.GroundEffect = groundEffects[rule.Skill.ID]
		rule.Skill.HurtType = int32(hurtType)
		rule.Skill.Effect = parseEffect(desc)
		rule.Skill.Statuses = skillStatusApplications(rule.Skill.ID, level)
		rule.Skill.CooldownMS = sep * 100
		rule.Skill.TargetSelf = tSelf != 0 || rule.Self
		rule.Skill.TargetEnemy = tMon != 0 || !rule.Self && tSelf == 0 && tTeam == 0
		rule.Skill.TargetTeam = tTeam != 0
		rule.Cooldown = domain.Ticks(int(cooldownMS))
		rule.InitialDelay = domain.Ticks(int(initialDelayMS))
		if rule.Skill.Name == "" {
			rule.Skill.Name = sourceName
		}
		if rule.Skill.Icon == 0 && rule.Skill.ID == 12005 {
			rule.Skill.Icon = 7069
		}
		// level=0 是少量脏引用；精确等级缺行时已经取同技能最近等级的表现，
		// 但规则等级仍保持怪物表声明值，状态强度不会偷偷改成回退行的等级。
		applyMonsterSkillEffects(&rule, effects[domain.SkillKey{ID: rule.Skill.ID, Level: 0}])
		applyMonsterSkillEffects(&rule, effects[domain.SkillKey{ID: rule.Skill.ID, Level: level}])
		if rule.Skill.Effect.Kind == domain.EffectNone && len(rule.Skill.Statuses) == 0 &&
			len(rule.Extra) == 0 && fallbackDamage > 0 {
			rule.Skill.Effect = domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: fallbackDamage}
		}
		if rule.Skill.Dist <= 0 && !rule.Self {
			rule.Skill.Dist = d.AI.BasicAttackDist
		}
		d.Skills = append(d.Skills, rule)
		monsters[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历怪物技能规则: %w", err)
	}
	for id, d := range monsters {
		sort.SliceStable(d.Skills, func(i, j int) bool {
			if d.Skills[i].Priority != d.Skills[j].Priority {
				return d.Skills[i].Priority > d.Skills[j].Priority
			}
			return d.Skills[i].Slot < d.Skills[j].Slot
		})
		monsters[id] = d
	}
	return nil
}

func applyMonsterSkillEffects(rule *domain.MonsterSkillRule, rows []monsterSkillEffectRow) {
	if rule == nil {
		return
	}
	for _, e := range rows {
		if e.overrideHurtType {
			rule.Skill.HurtType = e.hurtTypeOverride
		}
		switch e.kind {
		case "damage":
			pct := e.powerPct
			if pct <= 0 {
				pct = 100
			}
			setMonsterDirectEffect(&rule.Skill, domain.SkillEffect{
				Kind: domain.EffectDamage, DamagePct: pct, DamageFlat: e.flatValue,
				GuaranteedHit: e.guaranteedHit,
			})
		case "damage_mp":
			setMonsterDirectEffect(&rule.Skill, domain.SkillEffect{
				Kind: domain.EffectDamageMP, SourceAttackPct: e.flatValue,
				RequiresPreviousLanded: true,
			})
		case "heal":
			setMonsterDirectEffect(&rule.Skill, domain.SkillEffect{
				Kind: domain.EffectHeal, HealPctOfMax: e.healMaxPct, HealFlat: e.flatValue,
			})
		case "status":
			level := rule.Skill.Level
			switch e.statusLevelMode {
			case "fixed":
				level = e.statusLevelValue
			case "offset":
				level += e.statusLevelValue
			}
			if level <= 0 {
				level = 1
			}
			app := domain.StatusApplication{
				ID: domain.StatusID(e.statusID), Level: level, DurationSec: e.durationSec,
				ChanceBP: e.chanceBP,
			}
			duplicate := false
			for i, old := range rule.Skill.Statuses {
				// 怪物专用的显式时长也覆盖通用技能映射，后者可能已挂接
				// 同一状态的其它等级；不能因状态已存在而丢掉怪物时长配置。
				if old.ID == app.ID && app.DurationSec > 0 {
					rule.Skill.Statuses[i].DurationSec = app.DurationSec
				}
				if old.ID == app.ID && old.Level == app.Level {
					duplicate = true
				}
			}
			if !duplicate {
				rule.Skill.Statuses = append(rule.Skill.Statuses, app)
			}
		case "random_status":
			level := rule.Skill.Level
			switch e.statusLevelMode {
			case "fixed":
				level = e.statusLevelValue
			case "offset":
				level += e.statusLevelValue
			}
			if level <= 0 {
				level = 1
			}
			app := domain.StatusApplication{
				ID: domain.StatusID(e.statusID), Level: level, DurationSec: e.durationSec,
				ChanceBP: e.chanceBP,
			}
			found := false
			for i := range rule.Extra {
				if rule.Extra[i].Kind == domain.MonsterExtraRandomStatus {
					rule.Extra[i].Statuses = append(rule.Extra[i].Statuses, app)
					found = true
					break
				}
			}
			if !found {
				rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{
					Kind: domain.MonsterExtraRandomStatus, Statuses: []domain.StatusApplication{app},
				})
			}
		case "dispel_beneficial":
			rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{Kind: domain.MonsterExtraDispelBeneficial})
		case "dispel_harmful":
			rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{Kind: domain.MonsterExtraDispelHarmful})
		case "clear_aggro":
			rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{Kind: domain.MonsterExtraClearAggro})
		case "despawn":
			rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{Kind: domain.MonsterExtraDespawn})
		case "summon":
			rule.Extra = append(rule.Extra, domain.MonsterSkillExtra{
				Kind: domain.MonsterExtraSummon, SummonMonster: domain.MonsterID(e.summonMonster),
				Count: e.summonCount, Lifetime: domain.Ticks(int(e.durationSec) * 1000),
			})
		}
	}
}

func setMonsterDirectEffect(skill *domain.SkillDef, effect domain.SkillEffect) {
	if skill == nil {
		return
	}
	for i := range skill.Effects {
		if skill.Effects[i].Kind == effect.Kind {
			skill.Effects[i] = effect
			skill.Effect = skill.Effects[0]
			return
		}
	}
	skill.Effects = append(skill.Effects, effect)
	skill.Effect = skill.Effects[0]
}

// SpawnTable 是全服刷怪点, 按地图 id 分组。
type SpawnTable map[int32][]domain.SpawnPoint

// SpawnStats 记录加载时丢了什么。**必须报出来** ——
// 45635 个点位里有相当一部分引用的怪不在 game_monsters 里, 静默跳过会变成
// "这张图怎么没怪", 而且查不出原因。
type SpawnStats struct {
	Total      int // 表里的总点位
	Loaded     int // 真正装进内存的
	NoMonster  int // 引用的怪没数据
	NoMap      int // map_file 在 map_defs 里查不到 id
	SkippedIDs map[domain.MonsterID]int
}

// LoadSpawns 读 map_monster_spawns, 按地图 id 分组。
//
// map_file 是文件名(qw0014), 场景用的是数字 id, 所以要过一遍 map_defs。
// 两边对不上的直接丢 —— 没有 id 的图, 场景根本创建不出来。
//
// ⚠️ **一个地图文件可以对应多个地图 id**, 所以返回的点位总数会**多于**
// map_monster_spawns 的行数。这不是重复读, 是同一张图被多个副本/分线复用:
// pw23800(荒雷岛) 有 6 个 id, qw0060(幻想小岛) 有 3 个。每个 id 都是独立场景,
// 各自要有自己那份怪。别把这个"多出来"当 bug 修掉。
func LoadSpawns(ctx context.Context, q Querier, defs MonsterDefs) (SpawnTable, SpawnStats, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id, s.spawn_id, s.monster, s.x, s.y, s.dir
		  FROM map_monster_spawns s
		  LEFT JOIN map_defs d ON d.file = s.map_file`)
	if err != nil {
		return nil, SpawnStats{}, fmt.Errorf("data: 查 map_monster_spawns: %w", err)
	}
	defer rows.Close()

	out := SpawnTable{}
	st := SpawnStats{SkippedIDs: map[domain.MonsterID]int{}}
	for rows.Next() {
		var (
			mapID   *int32 // LEFT JOIN 可能给 NULL
			p       domain.SpawnPoint
			x, y    int32
			monster int32
		)
		if err := rows.Scan(&mapID, &p.ID, &monster, &x, &y, &p.Dir); err != nil {
			return nil, st, fmt.Errorf("data: 读刷怪点行: %w", err)
		}
		st.Total++
		if mapID == nil {
			st.NoMap++
			continue
		}
		p.Monster = domain.MonsterID(monster)
		if _, ok := defs[p.Monster]; !ok {
			st.NoMonster++
			st.SkippedIDs[p.Monster]++
			continue
		}
		p.Pos = domain.Pos{MapID: *mapID, X: float64(x), Y: float64(y)}
		out[*mapID] = append(out[*mapID], p)
		st.Loaded++
	}
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历刷怪点: %w", err)
	}
	return out, st, nil
}

// TeleportTable 是全服传送门, 按**起点**地图 id 分组。
type TeleportTable map[int32][]domain.Portal

// TeleportStats 记录加载时丢了什么。
type TeleportStats struct {
	Total   int // 表里的总数
	Loaded  int // 装进内存的
	NoMap   int // 起点 map_file 查不到 id
	NoDest  int // 目标地图在 map_defs 里没有定义
	NoArea  int // 没有触发多边形, 踩不出来
	Dungeon int // 由已闭合 map_script 进入队伍副本的门
}

// LoadTeleports 读 map_teleports。
//
// 三类会被丢掉, 而且**必须分开计数** —— 合成一个"丢了 N 个"没法判断是哪出了问题:
//
//	起点图没 id    这张图根本建不出场景
//	目标图没定义   送过去也是个不存在的地方(995 个里有 364 个如此)
//	没有多边形     踩不出来(144 个)。它们不是坏数据, 是靠点 NPC/用道具触发的门,
//	              等那两条路接进来时再单独加载
func LoadTeleports(ctx context.Context, q Querier, dungeons domain.DungeonTable) (TeleportTable, TeleportStats, error) {
	rows, err := q.Query(ctx, `
		SELECT src.id, t.proc_id, t.map_to, t.to_x, t.to_y, t.to_dir,
		       t.back_x, t.back_y, t.back_dir, t.poly, t.script,
		       dst.id IS NOT NULL
		  FROM map_teleports t
		  LEFT JOIN map_defs src ON src.file = t.map_file
		  LEFT JOIN map_defs dst ON dst.id   = t.map_to`)
	if err != nil {
		return nil, TeleportStats{}, fmt.Errorf("data: 查 map_teleports: %w", err)
	}
	defer rows.Close()

	out := TeleportTable{}
	st := TeleportStats{}
	for rows.Next() {
		var (
			srcID  *int32
			tp     domain.Teleport
			mapTo  int32
			tx, ty int32
			bx, by int32
			poly   []int32
			script string
			destOK bool
		)
		if err := rows.Scan(&srcID, &tp.ProcID, &mapTo, &tx, &ty, &tp.ToDir,
			&bx, &by, &tp.BackDir, &poly, &script, &destOK); err != nil {
			return nil, st, fmt.Errorf("data: 读传送门行: %w", err)
		}
		st.Total++
		if srcID == nil {
			st.NoMap++
			continue
		}
		if script == "Trap20502" {
			def, ok := dungeons[20502]
			if !ok || *srcID != 507 || mapTo != 0 {
				return nil, st, fmt.Errorf("data: 通天塔 Trap20502 与副本定义不一致 source=%d map_to=%d", *srcID, mapTo)
			}
			mapTo = def.Enter.MapID
			tx, ty = int32(def.Enter.X), int32(def.Enter.Y)
			tp.ToDir = 0
			tp.Dungeon = true
			destOK = true
			st.Dungeon++
		}
		if !destOK {
			st.NoDest++
			continue
		}
		if len(poly) < 6 || len(poly)%2 != 0 { // 至少三个点才围得成区域
			st.NoArea++
			continue
		}
		tp.To = domain.Pos{MapID: mapTo, X: float64(tx), Y: float64(ty)}
		tp.Back = domain.Pos{MapID: *srcID, X: float64(bx), Y: float64(by)}
		tp.Area = make(domain.Polygon, 0, len(poly)/2)
		for i := 0; i+1 < len(poly); i += 2 {
			tp.Area = append(tp.Area, domain.Pos{
				MapID: *srcID, X: float64(poly[i]), Y: float64(poly[i+1]),
			})
		}
		out[*srcID] = append(out[*srcID], domain.NewPortal(tp))
		st.Loaded++
	}
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历传送门: %w", err)
	}
	return out, st, nil
}

// LoadLevels 读 game_levels 的经验曲线。
//
// 只读 **level ≤ cap** 的行。理由不是省内存, 是数据在 61 级之后变了含义:
// `exp_accum` 从"累计值"变成了 `exp` 的副本(飞升后经验另起一套账)。
// 把飞升后的行混进来会让"多少经验算满级"这件事说不清。
func LoadLevels(ctx context.Context, q Querier, cap int32) (*domain.LevelTable, error) {
	rows, err := q.Query(ctx,
		`SELECT level, exp FROM game_levels WHERE level >= 1 AND level <= $1 ORDER BY level`, cap)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_levels: %w", err)
	}
	defer rows.Close()

	need := map[int32]int64{}
	for rows.Next() {
		var lv int32
		var exp int64
		if err := rows.Scan(&lv, &exp); err != nil {
			return nil, fmt.Errorf("data: 读 game_levels 行: %w", err)
		}
		need[lv] = exp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_levels: %w", err)
	}
	if len(need) == 0 {
		return nil, fmt.Errorf("data: game_levels 是空的")
	}
	return domain.NewLevelTable(need), nil
}

// ItemDefs 是全部物品与装备模板, 共用一个 id 空间。
type ItemDefs map[domain.ItemID]domain.ItemDef

// LoadItems 读 game_items + game_equipment, 合成一张表。
//
// 为什么合成一张: **掉落表里同一列既可能指向普通物品也可能指向装备**
// (item_src = 'item' / 'arm'), 上层不该为了查一个 id 去猜该查哪张表。
//
// 堆叠标记来自 ov_item.can_pile(真数据, 10629 能堆 / 1912 不能)。
// 装备一律不可堆叠 —— 每件的耐久不同, 堆起来就分不出谁是谁了。
func LoadItems(ctx context.Context, q Querier) (ItemDefs, error) {
	out := ItemDefs{}

	// 1. 普通物品。can_pile、weight 与使用标志都在客户端表里，原始
	// 描述来自 ov_desc；四个背包桶不能拿 category 直接当 tab。
	rows, err := q.Query(ctx, `
		SELECT i.id, i.name, i.level, i.price, COALESCE(c.sell_price, 0),
		       COALESCE(c.level_need, 0), COALESCE(c.can_pile, 0),
		       EXISTS(
		           SELECT 1 FROM gamedata.ov_card card
		            WHERE card.index = i.id AND card.item_id = i.id
		       ) AS physical_socket_card,
		       COALESCE(c.category, -1),
		       COALESCE(c.self_use, 0), COALESCE(c.other_use, 0), COALESCE(c.use_waste, 0),
		       COALESCE(c.weight, 0), COALESCE(c.can_mail,0), COALESCE(c.can_deal,0),
		       COALESCE(d.desc_, ''), COALESCE(d.small_map, 0),
		       COALESCE(cd.cool_time, 0), COALESCE(cd.item_type, 0)
		  FROM game_items i
		  LEFT JOIN gamedata.ov_item c ON c.index = i.id
		  LEFT JOIN gamedata.ov_itemcool cd ON cd.item_index = i.id
		  LEFT JOIN (
		       SELECT DISTINCT ON (index) index, desc_, small_map
		         FROM gamedata.ov_desc
		        ORDER BY index, row_no
		  ) d ON d.index = i.id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_items: %w", err)
	}
	for rows.Next() {
		var d domain.ItemDef
		var physicalSocketCard bool
		var pile, category, selfUse, otherUse, useWaste, sellEnabled, canMail, canTrade int32
		var cooldownSec, cooldownGroup int64
		if err := rows.Scan(&d.ID, &d.Name, &d.Level, &d.Price, &sellEnabled,
			&d.UseLevel, &pile, &physicalSocketCard, &category,
			&selfUse, &otherUse, &useWaste, &d.Weight, &canMail, &canTrade, &d.Description, &d.Icon,
			&cooldownSec, &cooldownGroup); err != nil {
			rows.Close()
			return nil, fmt.Errorf("data: 读 game_items 行: %w", err)
		}
		if cooldownSec < 0 || cooldownSec > math.MaxInt32 ||
			cooldownGroup < 0 || cooldownGroup > math.MaxInt32 {
			rows.Close()
			return nil, fmt.Errorf("data: 物品 %d 冷却配置越界: seconds=%d group=%d",
				d.ID, cooldownSec, cooldownGroup)
		}
		d.Stackable = pile != 0
		if physicalSocketCard {
			d.InstanceKind = domain.ItemInstanceSocketCard
			d.CardDefaults.Initialized = true
			d.Stackable = false
		}
		d.ConsumeOnUse = useWaste != 0
		d.CanMail = canMail != 0
		d.CanTrade = canTrade != 0
		d.CooldownSec, d.CooldownGroup = int32(cooldownSec), int32(cooldownGroup)
		d.SellPrice = npcSellPrice(d.Price, sellEnabled)
		d.InventoryTab, d.InventoryTabKnown = itemInventoryTab(category, selfUse, otherUse)
		if physicalSocketCard {
			d.InventoryTab, d.InventoryTabKnown = 1, true
		}
		out[d.ID] = d
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_items: %w", err)
	}

	// 2. 装备。覆盖同 id 的普通物品条目(装备表更具体)。
	//
	// 穿戴门槛来自客户端表 ov_arm：等级、六维、性别与职业是四组
	// 独立条件。职业开关同时合并基础/飞升列，不能拿其中一组代替另一组。
	// 固有属性、需求属性、weight、duration 和 max_weight 必须分组直接读
	// ov_arm：
	//   min_atk..hit_rate / str..dex       是装备主表的固有属性；
	//   level_need / str_need..spi_need     是穿戴门槛；
	//   game_equip_extra                    才是附加词条子表。
	// game_equipment.strength/... 是早期按 +117..+125 导入的“需求属性”，
	// 不能再当成装备加成；game_equipment.durable 同样曾误导入 +101 重量。
	// duration/300 才是 0x8006 展示耐久，小剑 1200→4、初行者装
	// 5100→17 已由真实样本闭合。
	erows, err := q.Query(ctx, `
		SELECT e.id, e.name, e.price, COALESCE(a.sell_price, 0), e.level_req, e.slot,
		       COALESCE(a.weight,0), COALESCE(a.duration,0) / 300,
		       COALESCE(a.attack_consume,0), COALESCE(a.be_hit_consume,0),
		       COALESCE(a.dead_consume,0), COALESCE(a.repair_consume,0),
		       COALESCE(a.spec_repair_consume,0), COALESCE(a.can_repair,0),
		       COALESCE(a.can_spec_repair,0), COALESCE(a.disappear_if_zero,0),
		       COALESCE(a.no_limit_duration,0), COALESCE(a.max_weight,0),
		       COALESCE(a.min_atk,0), COALESCE(a.max_atk,0), COALESCE(a.def,0),
		       COALESCE(a.matk,0), COALESCE(a.mdef,0), COALESCE(a.hit_rate,0),
		       COALESCE(a.str,0), COALESCE(a.vit,0), COALESCE(a.int_,0),
		       COALESCE(a.spi,0), COALESCE(a.agi,0), COALESCE(a.dex,0),
		       COALESCE(a.level_need,0), COALESCE(a.str_need,0), COALESCE(a.vit_need,0),
		       COALESCE(a.int_need,0), COALESCE(a.spi_need,0), COALESCE(a.agi_need,0),
		       COALESCE(a.dex_need,0), COALESCE(a.sex,0),
		       COALESCE(a.newbie,0), COALESCE(a.warrior,0), COALESCE(a.swordman,0), COALESCE(a.stabber,0),
		       COALESCE(a.druggist,0), COALESCE(a.magician,0),
		       COALESCE(a.sr_warrior,0), COALESCE(a.sr_swordman,0), COALESCE(a.sr_stabber,0),
		       COALESCE(a.sr_druggist,0), COALESCE(a.sr_magician,0),
		       COALESCE(ao.avatar,d.avatar,''), COALESCE(d.category,-1), COALESCE(d.type,0),
		       COALESCE(d.atk_dist,0), COALESCE(d.atk_type,0),
		       COALESCE(d.desc_,''), COALESCE(d.small_map,0),
		       COALESCE(a.position_,0), COALESCE(a.level,0), COALESCE(a.no_type_drop,1),
		       COALESCE(a.refine_limit,0), COALESCE(a.enchase_limit,0),
		       COALESCE(a.binding_skill,0), COALESCE(a.can_mail,0), COALESCE(a.can_deal,0), COALESCE(bs.skill_id,0),
		       COALESCE(bs.name,''), COALESCE(bs.desc_,'')
		  FROM game_equipment e
		  LEFT JOIN gamedata.ov_arm a ON a.index = e.id
		  LEFT JOIN game_equipment_appearance_overrides ao ON ao.item_id = e.id
		  LEFT JOIN (
		       SELECT DISTINCT ON (index)
		              index, avatar, category, type, atk_dist, atk_type, desc_, small_map
		         FROM gamedata.ov_desc
		        ORDER BY index, row_no
		       ) d ON d.index = e.id
		  LEFT JOIN LATERAL (
		       SELECT sd.skill_id, sd.name, sd.desc_
		         FROM gamedata.ov_skilldesc sd
		        WHERE sd.skill_level = 1
		          AND sd.skill_id = a.binding_skill
		          AND sd.binding_arm = e.id
		        ORDER BY sd.row_no
		        LIMIT 1
		  ) bs ON TRUE`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_equipment: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var (
			d                                                domain.ItemDef
			e                                                domain.EquipDef
			atkMin, atkMax, def, matk, mdef, hit             int32
			str, vit, wis, spi, agi, dex                     int32
			nLv, nStr, nVit, nInt, nSpi                      int64
			nAgi, nDex, sex                                  int64
			newbie, warrior, swordman, stabber               int64
			druggist, magician                               int64
			srWarrior, srSwordman, srStabber                 int64
			srDruggist, srMagician                           int64
			maxWeight                                        int32
			avatar                                           string
			avatarCategory, avatarType                       int32
			avatarAtkDist, avatarAtkType                     int32
			position, resourceLevel, noTypeDrop              int32
			refineLimit, socketLimit                         int32
			bindingSkill, canMail, canTrade, reciprocalSkill int64
			bindingSkillName, bindingSkillDesc               string
			sellEnabled                                      int32
			canRepair, canSpecialRepair                      int64
			disappearIfZero, noLimitDurability               int64
		)
		if err := erows.Scan(&d.ID, &d.Name, &d.Price, &sellEnabled,
			&e.LevelReq, &e.Slot, &d.Weight, &e.Durable, &e.AttackDurabilityCostRaw,
			&e.BeHitDurabilityCostRaw, &e.DeathDurabilityCostRaw,
			&e.RepairDurabilityCostRaw, &e.SpecRepairDurabilityCostRaw,
			&canRepair, &canSpecialRepair, &disappearIfZero, &noLimitDurability, &maxWeight,
			&atkMin, &atkMax, &def, &matk, &mdef, &hit,
			&str, &vit, &wis, &spi, &agi, &dex,
			&nLv, &nStr, &nVit, &nInt, &nSpi, &nAgi, &nDex, &sex,
			&newbie, &warrior, &swordman, &stabber, &druggist, &magician,
			&srWarrior, &srSwordman, &srStabber, &srDruggist, &srMagician,
			&avatar, &avatarCategory, &avatarType, &avatarAtkDist, &avatarAtkType,
			&d.Description, &d.Icon, &position, &resourceLevel, &noTypeDrop,
			&refineLimit, &socketLimit,
			&bindingSkill, &canMail, &canTrade, &reciprocalSkill,
			&bindingSkillName, &bindingSkillDesc); err != nil {
			return nil, fmt.Errorf("data: 读 game_equipment 行: %w", err)
		}
		if bindingSkill != 0 {
			if bindingSkill != reciprocalSkill || bindingSkillName == "" {
				return nil, fmt.Errorf("data: 法宝 %d 的绑定技能 %d 缺少 ov_skilldesc 双向关联", d.ID, bindingSkill)
			}
			e.Skill = &domain.EquipmentSkill{
				ID: domain.SkillID(bindingSkill), Name: bindingSkillName, Description: bindingSkillDesc,
			}
		}
		e.Need = domain.Requirement{
			Level: int32(nLv), Sex: uint8(sex),
			Base: domain.Base{STR: int32(nStr), VIT: int32(nVit), INT: int32(nInt),
				SPI: int32(nSpi), AGI: int32(nAgi), DEX: int32(nDex)},
			BeginnerAllowed: newbie != 0,
			ProfessionRestricted: newbie != 0 || warrior != 0 || swordman != 0 ||
				stabber != 0 || druggist != 0 || magician != 0 ||
				srWarrior != 0 || srSwordman != 0 || srStabber != 0 ||
				srDruggist != 0 || srMagician != 0,
			Professions: professionMask(
				warrior != 0 || srWarrior != 0,
				swordman != 0 || srSwordman != 0,
				stabber != 0 || srStabber != 0,
				druggist != 0 || srDruggist != 0,
				magician != 0 || srMagician != 0,
			),
		}
		e.ProfessionNames = professionNames(newbie, warrior, swordman, stabber, druggist, magician,
			srWarrior, srSwordman, srStabber, srDruggist, srMagician)
		d.Level = e.LevelReq
		d.SellPrice = npcSellPrice(d.Price, sellEnabled)
		d.Stackable = false // 装备永远不堆叠
		d.InstanceKind = domain.ItemInstanceEquipment
		d.CanMail = canMail != 0
		d.CanTrade = canTrade != 0
		// 初行者衣(category=8)与小剑(category=4)两种装备均由真实包证明
		// 走 tab=2；客户端同一装备值类型承载其余槽位。
		d.InventoryTab, d.InventoryTabKnown = 2, true
		e.Bonus = domain.Stats{
			MinAtk: atkMin, MaxAtk: atkMax, Def: def,
			MAtk: matk, MDef: mdef, Hit: hit, MaxWeight: maxWeight,
		}
		e.Base = domain.Base{STR: str, VIT: vit, INT: wis, SPI: spi, AGI: agi, DEX: dex}
		e.CanRepair = canRepair != 0
		e.CanSpecialRepair = canSpecialRepair != 0
		e.DisappearIfZero = disappearIfZero != 0
		e.NoLimitDurability = noLimitDurability != 0
		e.Category, e.Type = avatarCategory, avatarType
		// 旧导入arm_level读取了offset72的两个字节，混入套装号；正确等级
		// 是单字节level，不能再将“99 + 套装号*256”当成配方等级。
		e.Position, e.Tier = position, resourceLevel
		e.ResourceLevel, e.NoTypeDrop = resourceLevel, noTypeDrop != 0
		e.RefineLimit, e.SocketLimit = refineLimit, socketLimit
		e.Appearance = parseEquipAppearance(d.ID, e.Slot, avatar, avatarCategory,
			avatarType, avatarAtkDist, avatarAtkType)
		d.Equip = &e
		out[d.ID] = d
	}
	if err := erows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_equipment: %w", err)
	}

	if _, err := loadAffixes(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadSocketAffixes(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadEquipmentSuits(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadRefineEffects(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemUseStatuses(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemResourceRestores(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemStatusRemovals(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemReturns(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemSkillBooks(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemRevives(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetResourceRestores(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPlayerStatusCleanse(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetResetItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadCaptureBoostItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadCharacterStatResetItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadFixedItemRewards(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadGachaBoxes(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemCurrencyRewards(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemTitleUnlocks(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemExperienceBoosts(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPlayerExperienceItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemAvatarFusions(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemEquipmentSouls(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadDragonFusions(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetTransmogItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetCarrierItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetEggItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetExperienceItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetPPItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadPetRewardItems(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadSaddles(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadItemUseAudit(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

func loadItemUseAudit(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT item_id,disabled_reason FROM game_item_use_audit
		 WHERE source='self' AND NOT enabled ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查物品使用台账: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		var reason string
		if err := rows.Scan(&itemRaw, &reason); err != nil {
			return fmt.Errorf("data: 读物品使用台账: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || reason == "" {
			return fmt.Errorf("data: 物品使用台账无效 item=%d reason=%q", id, reason)
		}
		def.UseDisabledReason = reason
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历物品使用台账: %w", err)
	}
	return nil
}

// loadSocketAffixes 只读取实体卡自身的定义。其它共享 item_id 的行属于
// 词条定义，不能按物品号汇总成一张卡的效果。
func loadSocketAffixes(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
 SELECT c.item_id,e.idx,COALESCE(e.op_type,0),COALESCE(e.attr_id,0),
 COALESCE(e.mode,0),COALESCE(e.prob,0),COALESCE(e.value,0)
 FROM gamedata.ov_card c JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no
 WHERE c.index=c.item_id ORDER BY c.item_id,e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查卡片初始效果: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.ItemID
		var idx int
		var effect domain.CardEffect
		if err := rows.Scan(&item, &idx, &effect.Op, &effect.Attr, &effect.Mode, &effect.Probability, &effect.Value); err != nil {
			return err
		}
		def, ok := items[item]
		if !ok || def.InstanceKind != domain.ItemInstanceSocketCard {
			continue
		}
		if idx < 0 || idx >= len(def.CardDefaults.Effects) {
			return fmt.Errorf("data: 卡片 %d 效果索引越界 %d", item, idx)
		}
		def.CardDefaults.Initialized = true
		def.CardDefaults.Effects[idx] = effect
		if effect.Op != 0 {
			def.CardDefaults.Count = uint8(idx + 1)
		}
		items[item] = def
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for id, def := range items {
		if def.InstanceKind != domain.ItemInstanceSocketCard {
			continue
		}
		if !def.CardDefaults.Initialized {
			return fmt.Errorf("data: 卡片 %d 缺少原始效果", id)
		}
		def.SocketAffixes = def.CardDefaults.StaticAffixes()
		items[id] = def
	}
	return nil
}

// npcSellPrice 复现已抓到的普通物品 NPC 回收口径。sell_price 在两张原表中
// 大量为 0/1，真实背包却分别下发 0 或 floor(buy_price/2)，因此它只裁决能否卖，
// 不能作为金额。价格为 1 的活动物品仍至少回收 1 铜币。
func npcSellPrice(buy int64, enabled int32) int64 {
	if enabled <= 0 || buy <= 0 {
		return 0
	}
	price := buy / 2
	if price < 1 {
		price = 1
	}
	return price
}

// loadItemUseStatuses 把普通物品效果脚本中已经闭合的状态操作挂回物品模板。
// 常规 mode=4 是“状态操作”，attr_id 是状态号，value 是状态等级；肉片
// (3001→补充生命 1023/1) 与豆奶(3003→补充法力 1024/1)可逐字段对上。另有7件
// 同系列五秒恢复药把 mode 写成0，但状态号、等级、状态详情和物品描述仍完全闭合。
// 只为1023/1024接受这一原表变体，不把其他未知 mode=0 效果泛化成状态。
func loadItemUseStatuses(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, e.attr_id, e.value
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no = i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND e.op_type = 1
		   AND (e.mode = 4 OR (e.mode = 0 AND e.attr_id IN (1023,1024)))
		   AND e.prob = 100
		   AND e.attr_id >= 1000
		   AND e.value > 0
		   AND EXISTS (
		       SELECT 1
		         FROM gamedata.ov_exceptdetail d
		         JOIN gamedata.ov_exceptdetail_entry x ON x.row_no = d.row_no
		        WHERE d.type = e.attr_id AND x.except_level = e.value)
		 ORDER BY i.index, e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查物品状态效果: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, statusRaw, level int32
		if err := rows.Scan(&itemRaw, &statusRaw, &level); err != nil {
			return fmt.Errorf("data: 读物品状态效果行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		d, ok := items[id]
		if !ok || d.Equip != nil {
			continue
		}
		d.UseStatuses = append(d.UseStatuses, domain.StatusApplication{
			ID: domain.StatusID(statusRaw), Level: level,
		})
		items[id] = d
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历物品状态效果: %w", err)
	}
	return nil
}

// loadItemResourceRestores 装入23件已经由效果行与物品描述双向闭合的即时药品。
// op_type=1 是对自己生效；attr 33/35 分别是当前生命/法力；mode 0 是固定点数，
// mode 1 是最大值百分比。只接受 prob=100，随机效果留给独立模块处理。
func loadItemResourceRestores(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, e.attr_id, e.mode, e.value
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no = i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND e.op_type = 1
		   AND e.attr_id IN (33,35)
		   AND e.mode IN (0,1)
		   AND e.prob = 100
		   AND e.value > 0
		 ORDER BY i.index, e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查即时恢复物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, attr, mode, value int32
		if err := rows.Scan(&itemRaw, &attr, &mode, &value); err != nil {
			return fmt.Errorf("data: 读即时恢复物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		target := &def.UseRestore.HPFlat
		if attr == 35 {
			target = &def.UseRestore.MPFlat
		}
		if mode == 1 {
			target = &def.UseRestore.HPPct
			if attr == 35 {
				target = &def.UseRestore.MPPct
			}
		}
		if value > math.MaxInt32-*target {
			return fmt.Errorf("data: 即时恢复物品%d效果溢出", id)
		}
		*target += value
		if !def.UseRestore.Valid() {
			return fmt.Errorf("data: 即时恢复物品%d配置无效: %+v", id, def.UseRestore)
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历即时恢复物品: %w", err)
	}
	return nil
}

// loadItemStatusRemovals 装入解毒草与观音符。两件物品的描述分别写明解除
// 中毒/诅咒，效果行也精确给出 attr=87、value=状态号。
func loadItemStatusRemovals(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, e.value
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no = i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND e.op_type = 1
		   AND e.attr_id = 87
		   AND e.mode = 0
		   AND e.prob = 100
		   AND e.value >= 1000
		   AND EXISTS (SELECT 1 FROM gamedata.ov_exceptdesc d WHERE d.type=e.value)
		 ORDER BY i.index, e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查解除状态物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, statusRaw int32
		if err := rows.Scan(&itemRaw, &statusRaw); err != nil {
			return fmt.Errorf("data: 读解除状态物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		def.UseRemoveStatuses = append(def.UseRemoveStatuses, domain.StatusID(statusRaw))
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历解除状态物品: %w", err)
	}
	return nil
}

// loadItemReturns 装入回城卷轴与无限次回城书。两者都使用同一个 attr=94
// 效果，是否消耗由物品自己的 use_waste 决定。
func loadItemReturns(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no = i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND e.op_type = 1
		   AND e.attr_id = 94
		   AND e.mode = 0
		   AND e.prob = 100
		   AND e.value = 0
		 ORDER BY i.index`)
	if err != nil {
		return fmt.Errorf("data: 查回城物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		if err := rows.Scan(&itemRaw); err != nil {
			return fmt.Errorf("data: 读回城物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		def.UseReturn = true
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历回城物品: %w", err)
	}
	return nil
}

// loadItemSkillBooks 装入能与职业技能定义闭合的技能书。生活技能书的
// 110xx 编号由 0x1022 的生活技能流程处理，不能混进 Character.Skills。
func loadItemSkillBooks(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, i.skill_id
		  FROM gamedata.ov_item i
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		   AND i.skill_id > 0
		   AND i.skill_id = i.skill_id_3
		   AND i.skill_id NOT BETWEEN 11000 AND 11999
		   AND EXISTS (SELECT 1 FROM gamedata.ov_skilldesc s WHERE s.skill_id=i.skill_id)
		 ORDER BY i.index`)
	if err != nil {
		return fmt.Errorf("data: 查职业技能书: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, skillRaw int32
		if err := rows.Scan(&itemRaw, &skillRaw); err != nil {
			return fmt.Errorf("data: 读职业技能书行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		def.UseSkillBook = domain.SkillID(skillRaw)
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历职业技能书: %w", err)
	}
	return nil
}

// loadItemRevives 装入死亡面板使用的复活道具：两种无地图限制的替身娃娃
// 对应原地复活，回魂娃娃对应回城复活并免除损失。它们都不走普通 UseItem。
// 柏奚娃娃只限荒雷岛，现有数据没有给出这一名称对应的精确地图集合，因此
// 不在这里扩大适用范围。
func loadItemRevives(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index
		  FROM gamedata.ov_item i
		 WHERE i.index > 0
		   AND i.name IN ('替身娃娃','回魂娃娃')
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		 ORDER BY i.index`)
	if err != nil {
		return fmt.Errorf("data: 查原地复活物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		if err := rows.Scan(&itemRaw); err != nil {
			return fmt.Errorf("data: 读原地复活物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil || !def.ConsumeOnUse {
			continue
		}
		if def.Name == "回魂娃娃" {
			def.UseCityRevive = true
		} else {
			def.UseRevive = true
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历原地复活物品: %w", err)
	}
	return nil
}

// loadPetResourceRestores 装入活血丹、回魂散和宝宝糖果。op_type=2 表示
// 宠物目标，attr 33/35 分别恢复宠物生命/法力，mode=1 是最大值百分比。
func loadPetResourceRestores(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, e.attr_id, e.value
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		   AND e.op_type = 2
		   AND e.attr_id IN (33,35)
		   AND e.mode = 1
		   AND e.prob = 100
		   AND e.value BETWEEN 1 AND 100
		 ORDER BY i.index,e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查宠物恢复物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, attr, value int32
		if err := rows.Scan(&itemRaw, &attr, &value); err != nil {
			return fmt.Errorf("data: 读宠物恢复物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		if attr == 33 {
			def.UsePetRestore.HPPct += value
		} else {
			def.UsePetRestore.MPPct += value
		}
		if !def.UsePetRestore.Valid() {
			return fmt.Errorf("data: 宠物恢复物品%d配置无效: %+v", id, def.UsePetRestore)
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历宠物恢复物品: %w", err)
	}
	return nil
}

// loadPlayerStatusCleanse 装入醒梦铃和圣水。虽然效果表与宠物药共用
// op_type=2，但两件物品的描述语义明确作用于人物：醒梦铃删除睡眠1002，
// 圣水的 attr=88 表示清除人物全部当前效果。
func loadPlayerStatusCleanse(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, i.name, e.attr_id, e.value
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		   AND e.op_type = 2
		   AND e.mode = 0
		   AND e.prob = 100
		   AND ((i.name='醒梦铃' AND e.attr_id=87 AND e.value=1002)
		     OR (i.name='圣水' AND e.attr_id=88 AND e.value=0))
		 ORDER BY i.index,e.idx`)
	if err != nil {
		return fmt.Errorf("data: 查人物状态解除物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, attr, statusRaw int32
		var name string
		if err := rows.Scan(&itemRaw, &name, &attr, &statusRaw); err != nil {
			return fmt.Errorf("data: 读人物状态解除物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		if attr == 88 {
			def.UseClearStatuses = true
		} else {
			def.UseRemoveStatuses = append(def.UseRemoveStatuses, domain.StatusID(statusRaw))
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历人物状态解除物品: %w", err)
	}
	return nil
}

// loadPetResetItems 装入两种洗髓丸。attr 455 是保留前缀与天生技能的普通
// 重置，attr 458 是回到未孵化状态、重新抽取前缀的高级重置。
func loadPetResetItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index, e.attr_id
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		   AND e.op_type = 1
		   AND e.attr_id IN (455,458)
		   AND e.mode = 1
		   AND e.prob = 100
		   AND e.value = 100
		 ORDER BY i.index`)
	if err != nil {
		return fmt.Errorf("data: 查宠物洗髓物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, attr int32
		if err := rows.Scan(&itemRaw, &attr); err != nil {
			return fmt.Errorf("data: 读宠物洗髓物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		def.UsePetReset = domain.PetResetKeepInnate
		if attr == 458 {
			def.UsePetReset = domain.PetResetRehatch
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历宠物洗髓物品: %w", err)
	}
	return nil
}

// loadCaptureBoostItems 装入两种万能绳索。attr 438 的200表示特殊前缀概率
// 翻倍，attr 468 的200表示捕捉成功率翻倍；两者的说明都明确可替代普通
// 捕捉工具捕捉任何可捕捉宠物。
func loadCaptureBoostItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT i.index,
		       max(e.value) FILTER (WHERE e.attr_id=438) AS prefix_rate_pct,
		       COALESCE(max(e.value) FILTER (WHERE e.attr_id=468),100) AS success_rate_pct
		  FROM gamedata.ov_item i
		  JOIN gamedata.ov_item_entry e ON e.row_no=i.row_no
		 WHERE i.index > 0
		   AND i.self_use <> 0
		   AND i.use_waste <> 0
		   AND e.op_type = 1
		   AND e.attr_id IN (438,468)
		   AND e.mode = 1
		   AND e.prob = 100
		 GROUP BY i.index
		HAVING count(*) FILTER (WHERE e.attr_id=438)=1
		 ORDER BY i.index`)
	if err != nil {
		return fmt.Errorf("data: 查增强捕捉绳索: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, prefixPct, successPct int32
		if err := rows.Scan(&itemRaw, &prefixPct, &successPct); err != nil {
			return fmt.Errorf("data: 读增强捕捉绳索行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil {
			continue
		}
		def.CaptureBoost = domain.CaptureToolBoost{
			Universal: true, SuccessRatePct: successPct, PrefixRatePct: prefixPct,
		}
		if !def.CaptureBoost.Valid() {
			return fmt.Errorf("data: 增强捕捉绳索%d配置无效: %+v", id, def.CaptureBoost)
		}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历增强捕捉绳索: %w", err)
	}
	return nil
}

// loadCharacterStatResetItems 装入六种单属性豹胎易筋丸和重生水晶。
// script 是客户端表里稳定的一对一处理器名，比从中文描述截取属性名更可靠。
func loadCharacterStatResetItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT index,script
		  FROM gamedata.ov_item
		 WHERE self_use<>0 AND use_waste<>0
		   AND script IN ('Item_attr01','Item_attr02','Item_attr03','Item_attr04',
		                  'Item_attr05','Item_attr06','Item_rebirth')
		 ORDER BY index`)
	if err != nil {
		return fmt.Errorf("data: 查人物洗点物品: %w", err)
	}
	defer rows.Close()
	byScript := map[string]domain.StatResetKind{
		"Item_attr01":  domain.StatResetSTR,
		"Item_attr02":  domain.StatResetVIT,
		"Item_attr03":  domain.StatResetINT,
		"Item_attr04":  domain.StatResetSPI,
		"Item_attr05":  domain.StatResetAGI,
		"Item_attr06":  domain.StatResetDEX,
		"Item_rebirth": domain.StatResetAll,
	}
	for rows.Next() {
		var itemRaw int32
		var script string
		if err := rows.Scan(&itemRaw, &script); err != nil {
			return fmt.Errorf("data: 读人物洗点物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		kind := byScript[script]
		if !ok || def.Equip != nil || kind == domain.StatResetNone {
			continue
		}
		def.StatReset = kind
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历人物洗点物品: %w", err)
	}
	return nil
}

// loadFixedItemRewards 读取已人工闭合的固定礼包。随机礼包不进入该表；调用方
// 因此可以一次性预演全部奖励并做原子扣包/发放，不需要在场景里猜概率。
func loadFixedItemRewards(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT source_item_id,reward_item_id,quantity
		  FROM game_item_use_rewards
		 ORDER BY source_item_id,seq`)
	if err != nil {
		return fmt.Errorf("data: 查固定礼包奖励: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sourceRaw, rewardRaw, quantity int32
		if err := rows.Scan(&sourceRaw, &rewardRaw, &quantity); err != nil {
			return fmt.Errorf("data: 读固定礼包奖励行: %w", err)
		}
		sourceID, rewardID := domain.ItemID(sourceRaw), domain.ItemID(rewardRaw)
		source, sourceOK := items[sourceID]
		_, rewardOK := items[rewardID]
		if !sourceOK || source.Equip != nil || !rewardOK || quantity <= 0 {
			return fmt.Errorf("data: 固定礼包配置无效 source=%d reward=%d quantity=%d",
				sourceID, rewardID, quantity)
		}
		source.UseRewards = append(source.UseRewards, domain.RewardItem{Item: rewardID, Qty: quantity})
		items[sourceID] = source
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历固定礼包奖励: %w", err)
	}
	return nil
}

// loadGachaBoxes 读取概率礼包配置。原服开箱脚本没有随静态数据提供，概率与
// 数量区间完全由 game_item_gacha_boxes / game_item_gacha_rewards 决定；同一
// 物品要么是固定礼包要么是概率礼包，禁止双登记（init-43 已做 SQL 校验，
// 这里再对加载到的物品做一次内存断言）。
func loadGachaBoxes(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT source_item_id,key_item_id,key_quantity,min_free_slots
		  FROM game_item_gacha_boxes
		 ORDER BY source_item_id`)
	if err != nil {
		return fmt.Errorf("data: 查概率礼包: %w", err)
	}
	type boxRow struct {
		keyItem  domain.ItemID
		keyQty   int32
		minSlots int32
	}
	boxes := map[domain.ItemID]boxRow{}
	for rows.Next() {
		var sourceRaw, keyRaw, keyQty, minSlots int32
		if err := rows.Scan(&sourceRaw, &keyRaw, &keyQty, &minSlots); err != nil {
			rows.Close()
			return fmt.Errorf("data: 读概率礼包行: %w", err)
		}
		id := domain.ItemID(sourceRaw)
		source, ok := items[id]
		if !ok || source.Equip != nil {
			rows.Close()
			return fmt.Errorf("data: 概率礼包本体无效 item=%d", id)
		}
		if (keyRaw == 0) != (keyQty == 0) || minSlots < 0 {
			rows.Close()
			return fmt.Errorf("data: 概率礼包钥匙/门槛配置无效 item=%d key=%d qty=%d slots=%d",
				id, keyRaw, keyQty, minSlots)
		}
		if keyRaw > 0 {
			if _, keyOK := items[domain.ItemID(keyRaw)]; !keyOK {
				rows.Close()
				return fmt.Errorf("data: 概率礼包钥匙物品未闭合 item=%d key=%d", id, keyRaw)
			}
		}
		boxes[id] = boxRow{domain.ItemID(keyRaw), keyQty, minSlots}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历概率礼包: %w", err)
	}

	rewardRows, err := q.Query(ctx, `
		SELECT source_item_id,reward_item_id,min_quantity,max_quantity,chance_bp
		  FROM game_item_gacha_rewards
		 ORDER BY source_item_id,seq`)
	if err != nil {
		return fmt.Errorf("data: 查概率礼包奖励: %w", err)
	}
	defer rewardRows.Close()
	type rewardRow struct {
		reward           int32
		min, max, chance int32
	}
	rewards := map[domain.ItemID][]rewardRow{}
	for rewardRows.Next() {
		var sourceRaw, rewardRaw, minQty, maxQty, chance int32
		if err := rewardRows.Scan(&sourceRaw, &rewardRaw, &minQty, &maxQty, &chance); err != nil {
			return fmt.Errorf("data: 读概率礼包奖励行: %w", err)
		}
		sourceID := domain.ItemID(sourceRaw)
		if _, ok := boxes[sourceID]; !ok {
			return fmt.Errorf("data: 概率礼包奖励行无对应礼包 source=%d", sourceID)
		}
		if _, ok := items[domain.ItemID(rewardRaw)]; !ok {
			return fmt.Errorf("data: 概率礼包奖励未闭合 source=%d reward=%d", sourceID, rewardRaw)
		}
		if minQty <= 0 || maxQty < minQty || chance < 1 || chance > 10000 {
			return fmt.Errorf("data: 概率礼包奖励数值无效 source=%d reward=%d min=%d max=%d chance=%d",
				sourceID, rewardRaw, minQty, maxQty, chance)
		}
		rewards[sourceID] = append(rewards[sourceID], rewardRow{rewardRaw, minQty, maxQty, chance})
	}
	if err := rewardRows.Err(); err != nil {
		return fmt.Errorf("data: 遍历概率礼包奖励: %w", err)
	}

	for id, box := range boxes {
		def := items[id]
		boxDef := &domain.GachaBoxDef{
			KeyItem: box.keyItem, KeyQuantity: box.keyQty, MinFreeSlots: box.minSlots,
		}
		for _, r := range rewards[id] {
			boxDef.Rewards = append(boxDef.Rewards, domain.GachaBoxReward{
				Item: domain.ItemID(r.reward), MinQty: r.min, MaxQty: r.max, ChanceBP: r.chance,
			})
		}
		def.GachaBox = boxDef
		items[id] = def
	}
	return nil
}

func loadItemCurrencyRewards(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,caiyu,copper FROM game_item_currency_rewards ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查物品货币奖励: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		var caiyu, copper int64
		if err := rows.Scan(&itemRaw, &caiyu, &copper); err != nil {
			return fmt.Errorf("data: 读物品货币奖励: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil || caiyu < 0 || copper < 0 || (caiyu > 0) == (copper > 0) {
			return fmt.Errorf("data: 物品货币奖励配置无效 item=%d caiyu=%d", id, caiyu)
		}
		def.UseCaiyu = caiyu
		def.UseCopper = copper
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历物品货币奖励: %w", err)
	}
	return nil
}

func loadItemTitleUnlocks(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `SELECT item_id,title FROM game_item_title_unlocks ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查称号石配置: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		var title string
		if err := rows.Scan(&itemRaw, &title); err != nil {
			return fmt.Errorf("data: 读称号石配置行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil || title == "" {
			return fmt.Errorf("data: 称号石配置无效 item=%d title=%q", id, title)
		}
		def.UseTitle = title
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历称号石配置: %w", err)
	}
	return nil
}

func loadItemExperienceBoosts(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT item_id,player_bonus_pct,pet_bonus_pct,duration_sec
		  FROM game_item_experience_boosts ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查脚本型经验物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		var boost domain.ItemExperienceBoost
		if err := rows.Scan(&itemRaw, &boost.PlayerBonusPct, &boost.PetBonusPct,
			&boost.DurationSec); err != nil {
			return fmt.Errorf("data: 读脚本型经验物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.Equip != nil || !boost.Valid() {
			return fmt.Errorf("data: 脚本型经验物品%d配置无效: %+v", id, boost)
		}
		def.ExperienceBoost = boost
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历脚本型经验物品: %w", err)
	}
	return nil
}

func loadItemAvatarFusions(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT item_id,appearance_part,model
		  FROM game_item_avatar_fusions ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查装备换形物品: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw, partRaw, modelRaw int32
		if err := rows.Scan(&itemRaw, &partRaw, &modelRaw); err != nil {
			return fmt.Errorf("data: 读装备换形物品行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		part := domain.AppearancePart(partRaw)
		if !ok || def.Equip != nil || partRaw < 0 || partRaw > int32(domain.AppearanceFace) ||
			modelRaw < 0 || modelRaw > math.MaxUint16 {
			return fmt.Errorf("data: 装备换形物品%d配置无效 part=%d model=%d", id, partRaw, modelRaw)
		}
		def.AvatarFusion = &domain.AvatarFusionDef{Appearance: domain.EquipAppearance{
			Part: part, Model: uint16(modelRaw), Known: true,
		}}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历装备换形物品: %w", err)
	}
	rows.Close()
	affixRows, err := q.Query(ctx, `
		SELECT item_id,attr_id,value,mode
		  FROM game_item_avatar_fusion_affixes ORDER BY item_id,seq`)
	if err != nil {
		return fmt.Errorf("data: 查装备换形属性: %w", err)
	}
	defer affixRows.Close()
	for affixRows.Next() {
		var itemRaw int32
		var affix domain.Affix
		if err := affixRows.Scan(&itemRaw, &affix.Attr, &affix.Value, &affix.Mode); err != nil {
			return fmt.Errorf("data: 读装备换形属性行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.AvatarFusion == nil || !knownAttr(affix.Attr) ||
			(affix.Mode != domain.ModeAbsolute && affix.Mode != domain.ModePercent) {
			return fmt.Errorf("data: 装备换形属性无效 item=%d affix=%+v", id, affix)
		}
		def.AvatarFusion.Affixes = append(def.AvatarFusion.Affixes, affix)
		items[id] = def
	}
	if err := affixRows.Err(); err != nil {
		return fmt.Errorf("data: 遍历装备换形属性: %w", err)
	}
	return nil
}

func loadItemEquipmentSouls(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT item_id,target_slot
		  FROM game_item_equipment_souls ORDER BY item_id`)
	if err != nil {
		return fmt.Errorf("data: 查装备灵物品: %w", err)
	}
	for rows.Next() {
		var itemRaw, slotRaw int32
		if err := rows.Scan(&itemRaw, &slotRaw); err != nil {
			rows.Close()
			return fmt.Errorf("data: 读装备灵物品: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		slot := domain.EquipSlot(slotRaw)
		if !ok || def.Equip == nil || slot <= 0 || slot > domain.SlotTwoHand {
			rows.Close()
			return fmt.Errorf("data: 装备灵物品%d目标槽%d无效", id, slot)
		}
		def.EquipmentSoul = &domain.EquipmentSoulDef{TargetSlot: slot}
		items[id] = def
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("data: 遍历装备灵物品: %w", err)
	}
	rows.Close()

	affixes, err := q.Query(ctx, `
		SELECT item_id,attr_id,value,mode
		  FROM game_item_equipment_soul_affixes ORDER BY item_id,seq`)
	if err != nil {
		return fmt.Errorf("data: 查装备灵属性: %w", err)
	}
	defer affixes.Close()
	for affixes.Next() {
		var itemRaw int32
		var affix domain.Affix
		if err := affixes.Scan(&itemRaw, &affix.Attr, &affix.Value, &affix.Mode); err != nil {
			return fmt.Errorf("data: 读装备灵属性: %w", err)
		}
		id := domain.ItemID(itemRaw)
		def, ok := items[id]
		if !ok || def.EquipmentSoul == nil || !knownAttr(affix.Attr) ||
			(affix.Mode != domain.ModeAbsolute && affix.Mode != domain.ModePercent) {
			return fmt.Errorf("data: 装备灵属性无效 item=%d affix=%+v", id, affix)
		}
		def.EquipmentSoul.Affixes = append(def.EquipmentSoul.Affixes, affix)
		items[id] = def
	}
	if err := affixes.Err(); err != nil {
		return fmt.Errorf("data: 遍历装备灵属性: %w", err)
	}
	return nil
}

func loadPetTransmogItems(ctx context.Context, q Querier, items ItemDefs) error {
	rows, err := q.Query(ctx, `
		SELECT t.item_id,t.target_monster_id,c.name,t.model,t.duration_sec,t.require_unmounted
		  FROM game_item_pet_transmogs t
		  JOIN gamedata.ov_cmon c ON c.index=t.target_monster_id
		 ORDER BY t.item_id`)
	if err != nil {
		return fmt.Errorf("data: 查宠物幻化书: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemRaw int32
		var def domain.PetTransmogDef
		if err := rows.Scan(&itemRaw, &def.TargetMonster, &def.TargetName, &def.Model,
			&def.DurationSec, &def.RequireUnmounted); err != nil {
			return fmt.Errorf("data: 读宠物幻化书行: %w", err)
		}
		id := domain.ItemID(itemRaw)
		item, ok := items[id]
		if !ok || item.Equip != nil || !def.Valid() {
			return fmt.Errorf("data: 宠物幻化书%d配置无效: %+v", id, def)
		}
		item.PetTransmog = &def
		items[id] = item
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历宠物幻化书: %w", err)
	}
	return nil
}

func professionMask(allowed ...bool) uint8 {
	var mask uint8
	for i, ok := range allowed {
		if ok {
			mask |= 1 << uint8(i)
		}
	}
	return mask
}

func professionNames(flags ...int64) []string {
	base := [...]string{"初行者", "战士", "剑客", "刺客", "药师", "术士"}
	ascended := [...][2]string{
		{"飞天武者", "天武战神"},
		{"御剑圣者", "轩辕剑神"},
		{"无双影者", "秘影邪神"},
		{"圣光使者", "妙法灵仙"},
		{"冰火行者", "诛魔法仙"},
	}
	out := make([]string, 0, 16)
	for i, name := range base {
		if i < len(flags) && flags[i] != 0 {
			out = append(out, name)
		}
	}
	for i, names := range ascended {
		if 6+i < len(flags) && flags[6+i] != 0 {
			out = append(out, names[:]...)
		}
	}
	return out
}

// itemInventoryTab 把普通物品的客户端定义映射到 InventoryDlg 的四个桶：
// 0 medicine（可使用物品）、1 ware（材料/收集品）、2 equip、3 special。
// 装备由调用方固定走 2；这里处理 ov_item 的三种特殊 category。
//
// 真实客户端 TabAssets 的顺序已经运行时闭合为
// medicinebtn/warebtn/equipbtn/specialbtn；当前背包流量同时证明普通材料走 1。
func itemInventoryTab(category, selfUse, otherUse int32) (uint8, bool) {
	switch category {
	case 0:
		if selfUse != 0 || otherUse != 0 {
			return 0, true
		}
		return 1, true
	case 100, 112, 113:
		return 3, true
	default:
		return 0, false
	}
}

var (
	bodyAvatarPattern     = regexp.MustCompile(`^(?:A|M|N|m|body)([0-9]+)$`)
	capAvatarPattern      = regexp.MustCompile(`^(?:cap|cape)([0-9]+)$`)
	backpackAvatarPattern = regexp.MustCompile(`^backpack([0-9]+)$`)
	faceAvatarPattern     = regexp.MustCompile(`^face([0-9]+)$`)
	weaponRAvatarPattern  = regexp.MustCompile(`^weaponr([0-9]+)$`)
)

// parseEquipAppearance 严格解析 ov_desc.avatar 的可见装备资源名，
// 并要求装备槽与资源部位同时吻合。
//
// weaponr 是资源命名空间，真正落到 0x800a 的左/右手由装备槽决定：
// 单手武器(4)、双手武器(13) -> apWeaponR，盾(5) -> apWeaponL。
// 项链、手套、戒指、鞋等 ov_desc.avatar 为空的部位本来就没有角色模型，
// 不应用物品 id 伪造。24 行 face/slot=10 是旧表槽位异常，仍然拒绝跨槽填充。
func parseEquipAppearance(_ domain.ItemID, slot int32, avatar string, _ int32, typ, atkDist, atkType int32) domain.EquipAppearance {
	avatar = strings.TrimSpace(avatar)
	var (
		part  domain.AppearancePart
		match []string
	)
	switch {
	case slot == int32(domain.SlotBody) && bodyAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceBody, bodyAvatarPattern.FindStringSubmatch(avatar)
	case slot == int32(domain.SlotHead) && capAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceCap, capAvatarPattern.FindStringSubmatch(avatar)
	case slot == int32(domain.SlotBag) && backpackAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceBackpack, backpackAvatarPattern.FindStringSubmatch(avatar)
	case slot == int32(domain.SlotFace) && faceAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceFace, faceAvatarPattern.FindStringSubmatch(avatar)
	case (slot == int32(domain.SlotWeapon) || slot == int32(domain.SlotTwoHand)) && weaponRAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceWeaponR, weaponRAvatarPattern.FindStringSubmatch(avatar)
	case slot == int32(domain.SlotShield) && weaponRAvatarPattern.MatchString(avatar):
		part, match = domain.AppearanceWeaponL, weaponRAvatarPattern.FindStringSubmatch(avatar)
	default:
		return domain.EquipAppearance{}
	}
	if match == nil {
		return domain.EquipAppearance{}
	}
	model, err := strconv.ParseUint(match[1], 10, 16)
	if err != nil || model == 0 {
		return domain.EquipAppearance{}
	}
	out := domain.EquipAppearance{Part: part, Model: uint16(model), Known: true}

	// 0x800a 的战斗外观字段与 ov_desc 是同一组定义。weaponCType 直接使用
	// 武器 type，atkVariant 则是客户端四种协议动作族，不等于常量 1：
	// 单手剑=1，枪/双手剑=2，拳刃/暗器=3，杖=4。客户端运行时已经逐类
	// 验证；把暗器等武器也写成 1 会让部分角色模型取不到攻击帧。
	// atk_type 在武器行为为 0，不写入其他字段。盾只提供左手模型。
	if (slot == int32(domain.SlotWeapon) || slot == int32(domain.SlotTwoHand)) &&
		atkDist >= 0 && atkDist <= math.MaxUint16 && atkType == 0 {
		variant, known := weaponAttackVariant(typ)
		if !known {
			return out
		}
		out.AtkVariant = variant
		out.WeaponCType = uint8(typ)
		out.AtkDist = uint16(atkDist)
		out.AttackKnown = true
	}
	return out
}

// weaponAttackVariant 把 ov_desc.type 的武器位标志收敛为客户端协议动作族。
// type 是 1/2/4/8/16/32 六种互斥武器大类；只接受闭合过的精确值，避免未来
// 出现未知组合位时静默选择错误动作。
func weaponAttackVariant(typ int32) (uint8, bool) {
	switch typ {
	case 2: // 单手剑
		return 1, true
	case 1, 4: // 枪、双手剑
		return 2, true
	case 8, 32: // 拳刃、暗器/飞刀
		return 3, true
	case 16: // 杖
		return 4, true
	default:
		return 0, false
	}
}

// LoadItemsVerbose 与 LoadItems 相同, 另外报告跳过了多少条词条。
func LoadItemsVerbose(ctx context.Context, q Querier) (ItemDefs, int, error) {
	out, err := LoadItems(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	var skipped int
	if err := func() error {
		var e error
		skipped, e = countUnnamedAffixes(ctx, q)
		return e
	}(); err != nil {
		return out, 0, err
	}
	return out, skipped, nil
}

// LoadWardrobeDefs 把变装卡闭合到客户端五类衣柜与 0x800a 模型。
// ov_avatararm.index 是背包里的变装卡，avatar_index 指向真正提供模型的装备；
// 同一 item id 出现多份定义的两条脏数据不猜优先级，整项跳过。
func LoadWardrobeDefs(ctx context.Context, q Querier, items ItemDefs) (domain.WardrobeTable, WardrobeStats, error) {
	rows, err := q.Query(ctx, `
		WITH unique_avatar AS (
			SELECT index, min(category) AS category, min(avatar_index) AS avatar_index,
			       min(cannot_attach) AS cannot_attach
			  FROM gamedata.ov_avatararm
			 GROUP BY index
			HAVING count(*) = 1
		), duplicate_avatar AS (
			SELECT count(*) AS count
			  FROM (SELECT index FROM gamedata.ov_avatararm GROUP BY index HAVING count(*) > 1) d
		)
		SELECT u.index, u.category, u.avatar_index, u.cannot_attach,
		       COALESCE(d.avatar,''), COALESCE(a.sex,0),
		       (SELECT count FROM duplicate_avatar)
		  FROM unique_avatar u
		  LEFT JOIN (
		       SELECT DISTINCT ON (index) index,avatar
		         FROM gamedata.ov_desc ORDER BY index,row_no
		  ) d ON d.index = u.avatar_index
		  LEFT JOIN (
		       SELECT DISTINCT ON (index) index,sex
		         FROM gamedata.ov_arm ORDER BY index,row_no
		  ) a ON a.index = u.avatar_index
		 ORDER BY u.index`)
	if err != nil {
		return nil, WardrobeStats{}, fmt.Errorf("data: 查衣柜变装映射: %w", err)
	}
	defer rows.Close()

	out := domain.WardrobeTable{}
	var stats WardrobeStats
	for rows.Next() {
		var (
			item, appearanceItem domain.ItemID
			slot                 int32
			cannotAttach         int16
			avatar               string
			sex                  int16
			duplicates           int
		)
		if err := rows.Scan(&item, &slot, &appearanceItem, &cannotAttach, &avatar, &sex, &duplicates); err != nil {
			return nil, stats, fmt.Errorf("data: 读衣柜变装映射行: %w", err)
		}
		stats.Rows++
		stats.DuplicateIDs = duplicates
		itemDef, ok := items[item]
		if !ok || itemDef.Equip != nil {
			stats.MissingItem++
			continue
		}
		appearance := parseEquipAppearance(appearanceItem, slot, avatar, 0, 0, 0, 0)
		category, ok := domain.WardrobeCategoryOfAppearance(appearance.Part)
		if !ok {
			stats.UnsupportedCategory++
			continue
		}
		if !appearance.Known {
			stats.MissingAppearance++
			continue
		}
		out[item] = domain.WardrobeDef{
			ID: item, Category: category, Appearance: appearance,
			Name: itemDef.Name, Description: itemDef.Description,
			CannotAttach: cannotAttach != 0, Sex: uint8(sex),
		}
		stats.Loaded++
	}
	if err := rows.Err(); err != nil {
		return nil, stats, fmt.Errorf("data: 遍历衣柜变装映射: %w", err)
	}
	return out, stats, nil
}

func countUnnamedAffixes(ctx context.Context, q Querier) (int, error) {
	rows, err := q.Query(ctx,
		`SELECT count(*) FROM game_equip_extra WHERE prob < 100 OR attr_name = ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}

// loadAffixes 把 game_equip_extra 的词条挂到装备上。
//
// **Stats 里的暴击率与各种抗性只从这里来** —— 没有词条那些字段永远是零,
// 整套战斗公式就有一半参数拿不到值。
//
// 跳掉两类行, 都有明确依据:
//
//	prob < 100    "有几率触发"的词条(全表只有 2 条, 30% 的爆击率)。
//	              当成固定加成算等于白送 —— 宁可不给, 也别给错。
//	attr_name=''  **导入器没能解出名字的行**(352 条, 18 种 attr_id, 其中 266 条值是 0,
//	              还有 attr_id=0 / 772 / 1405 这种明显不是属性号的)。
//	              带名字的 17365 条覆盖 **24 种 attr_id, 与 domain 里的字典一一对上**,
//	              所以"有没有名字"就是这份数据自带的质量信号, 比我们另立一套白名单可靠。
func loadAffixes(ctx context.Context, q Querier, items ItemDefs) (skipped int, err error) {
	rows, err := q.Query(ctx, `
		SELECT arm_id, attr_id, value, mode, attr_name <> '' AS named
		  FROM game_equip_extra WHERE prob >= 100 ORDER BY arm_id, seq`)
	if err != nil {
		return 0, fmt.Errorf("data: 查 game_equip_extra: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var armID, attr, value, mode int32
		var named bool
		if err := rows.Scan(&armID, &attr, &value, &mode, &named); err != nil {
			return skipped, fmt.Errorf("data: 读词条行: %w", err)
		}
		if !named {
			skipped++
			continue
		}
		d, ok := items[domain.ItemID(armID)]
		if !ok || d.Equip == nil {
			skipped++ // 词条挂在一件不存在的装备上
			continue
		}
		d.Equip.Affixes = append(d.Equip.Affixes,
			domain.Affix{Attr: attr, Value: value, Mode: mode})
		items[domain.ItemID(armID)] = d
	}
	return skipped, rows.Err()
}

// DropTable 是怪物掉落表, 按怪物配置 id 索引。
type DropTable map[domain.MonsterID][]domain.DropEntry

// DropStats 记录加载时丢了什么。
type DropStats struct {
	Total          int // 表里的总行数
	Loaded         int // 指定了具体物品并成功加载的行
	ByKind         int // 只给了类别(长剑/法杖…)而没有具体 id 的行
	KindLoaded     int // ByKind 中成功解析出候选池的行
	KindUnresolved int // ByKind 中暂时找不到可靠候选池的行
	NoItem         int // 给了 id 但物品表里没有
	BadRate        int // 概率不合法
}

// LoadEquipmentRollTable 加载已经离线预计算好的装备随机属性份额。
// 普通类型掉落已经正式启用随机属性，空配置必须阻止启动；否则服务会看似正常，
// 实际却把所有普通装备静默产成白板。
//
// 条数权重按染色档位分组：档位决定“这次掉几属性”，数值池（Options）各档共用。
func LoadEquipmentRollTable(ctx context.Context, q Querier) (domain.EquipmentRollTable, error) {
	out := domain.EquipmentRollTable{
		Counts:  make(map[domain.ColorProfile][]domain.WeightedAffixCount),
		Options: make(map[domain.EquipmentRollKey][]domain.EquipmentRollOption),
	}
	rows, err := q.Query(ctx, `
		SELECT profile_id, attr_count, weight
		  FROM game_equipment_roll_counts
		 ORDER BY profile_id, attr_count`)
	if err != nil {
		return out, fmt.Errorf("data: 查装备随机属性条数: %w", err)
	}
	for rows.Next() {
		var profile, count int16
		var weight int64
		if err := rows.Scan(&profile, &count, &weight); err != nil {
			rows.Close()
			return out, fmt.Errorf("data: 读装备随机属性条数: %w", err)
		}
		if profile <= 0 || count < 0 || count > 5 || weight <= 0 {
			rows.Close()
			return out, fmt.Errorf("data: 装备随机属性条数配置非法 profile=%d count=%d weight=%d",
				profile, count, weight)
		}
		key := domain.ColorProfile(profile)
		out.Counts[key] = append(out.Counts[key],
			domain.WeightedAffixCount{Count: uint8(count), Weight: weight})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, fmt.Errorf("data: 遍历装备随机属性条数: %w", err)
	}
	rows.Close()

	options, err := q.Query(ctx, `
		SELECT equipment_level, equipment_type, card_id, attr_id, value, mode, weight
		  FROM game_equipment_roll_options
		 ORDER BY equipment_level, equipment_type, card_id, attr_id, value`)
	if err != nil {
		return out, fmt.Errorf("data: 查装备随机属性矩阵: %w", err)
	}
	defer options.Close()
	for options.Next() {
		var key domain.EquipmentRollKey
		var option domain.EquipmentRollOption
		if err := options.Scan(&key.Level, &key.Type, &option.CardID, &option.Affix.Attr,
			&option.Affix.Value, &option.Affix.Mode, &option.Weight); err != nil {
			return out, fmt.Errorf("data: 读装备随机属性矩阵: %w", err)
		}
		if key.Level < 0 || key.Type < 0 || option.CardID <= 0 || option.Affix.Attr <= 0 ||
			(option.Affix.Mode != domain.ModeAbsolute && option.Affix.Mode != domain.ModePercent) ||
			option.Weight <= 0 {
			return out, fmt.Errorf("data: 装备随机属性矩阵非法 key=%+v option=%+v", key, option)
		}
		out.Options[key] = append(out.Options[key], option)
	}
	if err := options.Err(); err != nil {
		return out, fmt.Errorf("data: 遍历装备随机属性矩阵: %w", err)
	}
	if len(out.Counts) == 0 {
		return out, fmt.Errorf("data: 装备随机属性条数档位为空")
	}
	// 默认档（精英/BOSS）必须覆盖 0..5 全部六档，保证精英与原行为一致。
	defaultCounts := out.Counts[domain.DefaultColorProfile]
	if len(defaultCounts) != 6 {
		return out, fmt.Errorf("data: 默认染色档条数不完整: got=%d want=6", len(defaultCounts))
	}
	for count, option := range defaultCounts {
		if int(option.Count) != count {
			return out, fmt.Errorf("data: 默认染色档缺少条数 %d: got=%d", count, option.Count)
		}
	}
	// 同一档位内部不允许重复条数，否则权重会被重复累加。
	for profile, counts := range out.Counts {
		seen := make(map[uint8]struct{}, len(counts))
		for _, option := range counts {
			if _, dup := seen[option.Count]; dup {
				return out, fmt.Errorf("data: 染色档 %d 的属性条数 %d 重复", profile, option.Count)
			}
			seen[option.Count] = struct{}{}
		}
	}
	if !out.Enabled() {
		return out, fmt.Errorf("data: 装备随机属性矩阵为空")
	}
	return out, nil
}

// LoadBossFixedDropTable 加载固定装备怪的保底阶段。该阶段先按各行概率
// 正常掷，再按 Minimum 用权重池补足；完成后场景才执行原有完整掉落表。
func LoadBossFixedDropTable(ctx context.Context, q Querier, items ItemDefs) (domain.BossFixedDropTable, error) {
	out := domain.BossFixedDropTable{}
	rows, err := q.Query(ctx, `
		SELECT map_id, monster_id, minimum, item_id, rate_pct, weight, roll_affixes
		  FROM game_boss_fixed_equipment_pool
		 ORDER BY map_id, monster_id, seq`)
	if err != nil {
		return nil, fmt.Errorf("data: 查副本BOSS固定装备池: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key domain.BossFixedDropKey
		var minimum int16
		var choice domain.BossFixedDropChoice
		if err := rows.Scan(&key.MapID, &key.Monster, &minimum, &choice.Item,
			&choice.RatePct, &choice.Weight, &choice.RollAffixes); err != nil {
			return nil, fmt.Errorf("data: 读副本BOSS固定装备池: %w", err)
		}
		def, ok := items[choice.Item]
		if key.MapID <= 0 || key.Monster <= 0 || minimum < 1 || minimum > 2 ||
			!ok || def.Equip == nil || choice.RatePct < 0 || choice.RatePct > 100 || choice.Weight <= 0 ||
			choice.RollAffixes && def.Equip.NoTypeDrop {
			return nil, fmt.Errorf("data: 副本BOSS固定装备池非法 key=%+v minimum=%d choice=%+v",
				key, minimum, choice)
		}
		rule := out[key]
		if rule.Minimum != 0 && rule.Minimum != uint8(minimum) {
			return nil, fmt.Errorf("data: 副本BOSS固定装备池最低件数冲突 key=%+v", key)
		}
		rule.Minimum = uint8(minimum)
		rule.Pool = append(rule.Pool, choice)
		out[key] = rule
	}
	return out, rows.Err()
}

// LoadDrops 读 game_monster_drops。
//
// item_id = 0 的行只给类别名与等级区间，意思是“从该类物品里按等级随机挑
// 一件”。装备分类直接来自 ov_arm.query_type，卡片与宝石来自 ov_item 的
// query_type/category；不能用装备槽或名称猜类别。
//
// rate 是**百分比, 每条独立掷**, 不是瓜分 100%。实测单怪合计能到 3200,
// 也有 rate=100 的必掉项 —— 按"瓜分"理解就全错了。
func LoadDrops(ctx context.Context, q Querier, items ItemDefs) (DropTable, DropStats, error) {
	levels, err := loadDropChoiceLevels(ctx, q, items)
	if err != nil {
		return nil, DropStats{}, err
	}
	type poolKey struct {
		kind     string
		min, max int32
	}
	pools := make(map[poolKey][]domain.ItemID)
	choicePool := func(kind string, minLevel, maxLevel int32) []domain.ItemID {
		key := poolKey{kind: kind, min: minLevel, max: maxLevel}
		if pool, ok := pools[key]; ok {
			return pool
		}
		if minLevel > maxLevel || maxLevel-minLevel > 1000 {
			pools[key] = nil
			return nil
		}
		var pool []domain.ItemID
		for level := minLevel; level <= maxLevel; level++ {
			pool = append(pool, levels[kind][level]...)
		}
		pools[key] = pool
		return pool
	}

	rows, err := q.Query(ctx, `
		SELECT monster_id, item_id, rate, kind_name, lv_min, lv_max
		  FROM game_monster_drops
		 ORDER BY monster_id, seq`)
	if err != nil {
		return nil, DropStats{}, fmt.Errorf("data: 查 game_monster_drops: %w", err)
	}
	defer rows.Close()

	out := DropTable{}
	st := DropStats{}
	for rows.Next() {
		var mid, iid, minLevel, maxLevel int32
		var kind string
		var rate float32
		if err := rows.Scan(&mid, &iid, &rate, &kind, &minLevel, &maxLevel); err != nil {
			return nil, st, fmt.Errorf("data: 读掉落行: %w", err)
		}
		st.Total++
		if rate <= 0 || rate > 100 {
			st.BadRate++
			continue
		}
		if iid == 0 {
			st.ByKind++
			pool := choicePool(strings.TrimSpace(kind), minLevel, maxLevel)
			if len(pool) == 0 {
				st.KindUnresolved++
				continue
			}
			out[domain.MonsterID(mid)] = append(out[domain.MonsterID(mid)],
				domain.DropEntry{Item: pool[0], Choices: pool, RatePct: float64(rate), RollAffixes: true})
			st.KindLoaded++
			continue
		}
		if _, ok := items[domain.ItemID(iid)]; !ok {
			st.NoItem++
			continue
		}
		out[domain.MonsterID(mid)] = append(out[domain.MonsterID(mid)],
			domain.DropEntry{Item: domain.ItemID(iid), RatePct: float64(rate)})
		st.Loaded++
	}
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历掉落表: %w", err)
	}
	return out, st, nil
}

// loadDropChoiceLevels 把客户端静态表中的明确分类建成“类别 → 掉落等级 → 物品”。
// 只收 LoadItems 已加载的 id，保证掷中后一定能构造地面实体并进入背包。
func loadDropChoiceLevels(ctx context.Context, q Querier, items ItemDefs) (map[string]map[int32][]domain.ItemID, error) {
	out := make(map[string]map[int32][]domain.ItemID)
	add := func(kind string, level, rawID int32) {
		id := domain.ItemID(rawID)
		if kind == "" || level < 0 {
			return
		}
		if _, ok := items[id]; !ok {
			return
		}
		if out[kind] == nil {
			out[kind] = make(map[int32][]domain.ItemID)
		}
		out[kind][level] = append(out[kind][level], id)
	}

	armRows, err := q.Query(ctx, `
		SELECT DISTINCT ON (index) index, query_type, position_, level
		  FROM gamedata.ov_arm
		 WHERE index <> 0 AND no_type_drop = 0
		 ORDER BY index, row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查随机装备候选: %w", err)
	}
	for armRows.Next() {
		var id, queryType, position, level int32
		if err := armRows.Scan(&id, &queryType, &position, &level); err != nil {
			armRows.Close()
			return nil, fmt.Errorf("data: 读随机装备候选: %w", err)
		}
		kind := equipmentDropKind(queryType)
		if kind == "" {
			switch position {
			case 114:
				kind = "耳环"
			case 116:
				kind = "腰带"
			}
		}
		add(kind, level, id)
	}
	if err := armRows.Err(); err != nil {
		armRows.Close()
		return nil, fmt.Errorf("data: 遍历随机装备候选: %w", err)
	}
	armRows.Close()

	itemRows, err := q.Query(ctx, `
		SELECT DISTINCT ON (index) index, query_type, category, level
		  FROM gamedata.ov_item
		 WHERE index <> 0
		 ORDER BY index, row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查随机物品候选: %w", err)
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var id, queryType, category, level int32
		if err := itemRows.Scan(&id, &queryType, &category, &level); err != nil {
			return nil, fmt.Errorf("data: 读随机物品候选: %w", err)
		}
		kind := ""
		switch {
		case queryType >= 28 && queryType <= 34:
			kind = "卡片"
		case category == 112:
			kind = "宝石"
		}
		add(kind, level, id)
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历随机物品候选: %w", err)
	}
	return out, nil
}

func equipmentDropKind(queryType int32) string {
	return map[int32]string{
		1: "长枪", 2: "长剑", 3: "双手剑", 4: "法杖", 5: "双刃",
		6: "暗器", 7: "盾牌", 8: "服装", 9: "头盔", 10: "鞋子",
		11: "手套", 12: "戒指", 13: "背包", 14: "项链", 15: "面具",
		49: "耳环", 51: "腰带",
	}[queryType]
}

// SkillStats 记录技能加载的情况。
type SkillStats struct {
	Total              int // 读到的行数
	Loaded             int
	Passive            int // 被动技能(加载了, 但没有主动效果)
	WithStatus         int // 至少挂上一条已闭合状态的技能等级
	StructuredEffects  int // 使用 PostgreSQL 显式直接效果的技能等级
	StructuredStatuses int // 使用 PostgreSQL 显式状态映射的技能等级
	NoEffect           int // 主动技能但直接效果/状态均未闭合
}

func loadSkillAreaCenters(ctx context.Context, q Querier) (map[domain.SkillID]domain.SkillAreaCenter, error) {
	rows, err := q.Query(ctx, `
		SELECT skill_id, center
		  FROM game_skill_area_rules
		 ORDER BY skill_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查技能范围中心规则: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillID]domain.SkillAreaCenter)
	for rows.Next() {
		var id domain.SkillID
		var center string
		if err := rows.Scan(&id, &center); err != nil {
			return nil, fmt.Errorf("data: 读技能范围中心规则: %w", err)
		}
		switch center {
		case "caster":
			out[id] = domain.SkillAreaCenterCaster
		case "target":
			out[id] = domain.SkillAreaCenterTarget
		default:
			return nil, fmt.Errorf("data: 技能 %d 的范围中心 %q 未实现", id, center)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历技能范围中心规则: %w", err)
	}
	return out, nil
}

func skillHasIndependentTargeting(d domain.SkillDef) bool {
	for _, effect := range d.DirectEffects() {
		switch effect.Kind {
		case domain.EffectTeleportPoint, domain.EffectTeleportTarget,
			domain.EffectPlaceTrap, domain.EffectDetectTrap, domain.EffectDisarmTrap,
			domain.EffectRevealInvisible:
			return true
		}
	}
	return false
}

func loadAreaSkillHitEffects(ctx context.Context, q Querier) (map[domain.SkillID]string, error) {
	rows, err := q.Query(ctx, `
		SELECT s.skill_id, s.be_h_show_id, s.r_be_h_show_id, s.be_h_show_pos
		  FROM gamedata.ov_skillshow s
		 WHERE (s.be_h_show_id > 0 OR s.r_be_h_show_id > 0)
		   AND EXISTS (
		       SELECT 1 FROM gamedata.ov_skilldesc d
		        WHERE d.skill_id = s.skill_id
		          AND d.prof BETWEEN 1 AND 5 AND d.skill_type = 2
		   )
		 ORDER BY s.row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查职业范围技能受击特效: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillID]string)
	for rows.Next() {
		var id domain.SkillID
		var singleID, rangeID, singlePos int32
		if err := rows.Scan(&id, &singleID, &rangeID, &singlePos); err != nil {
			return nil, fmt.Errorf("data: 读职业范围技能受击特效: %w", err)
		}
		showID, phase := singleID, 30
		if rangeID > 0 {
			showID, phase = rangeID, 50
		} else if singlePos == 4 {
			// 位置命中特效由0x800e的落点播放一次，不能按每个受击实体复制。
			continue
		}
		if showID <= 0 {
			continue
		}
		name := fmt.Sprintf("TFX%03d_%d", showID, phase)
		if old, exists := out[id]; exists && old != name {
			return nil, fmt.Errorf("data: 职业范围技能 %d 存在冲突的受击特效 %q/%q", id, old, name)
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历职业范围技能受击特效: %w", err)
	}
	return out, nil
}

func loadStructuredSkillStatuses(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.StatusApplication, error) {
	rows, err := q.Query(ctx, `
		SELECT s.skill_id, s.skill_level, s.status_order,
		       s.status_id, s.status_level, s.chance_bp, s.duration_sec, s.description_override,
		       s.instant_death_immune,
		       EXISTS (
		           SELECT 1
		             FROM gamedata.ov_exceptdetail d
		             JOIN gamedata.ov_exceptdetail_entry e ON e.row_no = d.row_no
		            WHERE d.type = s.status_id
		              AND e.except_level = s.status_level
		              AND e.last_time > 0
		       ) OR EXISTS (
		           SELECT 1
		             FROM game_custom_statuses c
		            WHERE c.status_id = s.status_id
		              AND c.status_level = s.status_level
		       )
		  FROM game_skill_statuses s
		 ORDER BY s.skill_id, s.skill_level, s.status_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查结构化技能状态: %w", err)
	}
	defer rows.Close()

	out := make(map[domain.SkillKey][]domain.StatusApplication)
	for rows.Next() {
		var (
			key      domain.SkillKey
			order    int32
			app      domain.StatusApplication
			statusOK bool
		)
		if err := rows.Scan(&key.ID, &key.Level, &order,
			&app.ID, &app.Level, &app.ChanceBP, &app.DurationSec, &app.Description,
			&app.InstantDeathImmune, &statusOK); err != nil {
			return nil, fmt.Errorf("data: 读结构化技能状态: %w", err)
		}
		if !statusOK {
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个状态引用不存在的定义 %d/%d",
				key.ID, key.Level, order, app.ID, app.Level)
		}
		out[key] = append(out[key], app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历结构化技能状态: %w", err)
	}
	return out, nil
}

func loadStructuredSkillEffects(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.SkillEffect, error) {
	rows, err := q.Query(ctx, `
		SELECT skill_id, skill_level, effect_order, effect_kind,
		       power_pct, max_resource_pct, flat_value, guaranteed_hit, chance_bp,
		       effect_target, requires_previous_landed, weapon_durability_pct,
		       loss_refund_pct
		  FROM game_skill_effects
		 ORDER BY skill_id, skill_level, effect_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查结构化技能效果: %w", err)
	}
	defer rows.Close()

	type effectOrderKey struct {
		Skill domain.SkillKey
		Order int32
	}
	out := make(map[domain.SkillKey][]domain.SkillEffect)
	positions := make(map[effectOrderKey]int)
	for rows.Next() {
		var (
			key                   domain.SkillKey
			order                 int32
			kind                  string
			powerPct, resourcePct int32
			flat                  int32
			guaranteedHit         bool
			chanceBP              int32
			target                string
			requiresPrevious      bool
			weaponDurabilityPct   int32
			lossRefundPct         int32
		)
		if err := rows.Scan(&key.ID, &key.Level, &order, &kind,
			&powerPct, &resourcePct, &flat, &guaranteedHit, &chanceBP, &target, &requiresPrevious,
			&weaponDurabilityPct, &lossRefundPct); err != nil {
			return nil, fmt.Errorf("data: 读结构化技能效果: %w", err)
		}
		effect := domain.SkillEffect{
			GuaranteedHit:           guaranteedHit,
			RequiresPreviousLanded:  requiresPrevious,
			WeaponDurabilityCostPct: weaponDurabilityPct,
			LossRefundPct:           lossRefundPct,
		}
		if lossRefundPct != 0 && kind != "resurrect" {
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个非复活效果不能配置死亡损失返还", key.ID, key.Level, order)
		}
		if weaponDurabilityPct != 0 && kind != "damage" {
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个非伤害效果不能消耗武器耐久", key.ID, key.Level, order)
		}
		switch target {
		case "target":
			effect.Target = domain.EffectTargetResolved
		case "caster":
			effect.Target = domain.EffectTargetCaster
		default:
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个效果目标 %q 未实现", key.ID, key.Level, order, target)
		}
		switch kind {
		case "damage":
			if powerPct <= 0 || resourcePct != 0 || effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 damage 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.DamagePct, effect.DamageFlat = domain.EffectDamage, powerPct, flat
		case "heal":
			if powerPct != 0 || resourcePct < 0 || flat < 0 || resourcePct+flat == 0 || guaranteedHit {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 heal 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind = domain.EffectHeal
			effect.HealPctOfMax, effect.HealFlat = resourcePct, flat
		case "restore_mp":
			if resourcePct != 0 || powerPct < 0 || flat < 0 || powerPct+flat <= 0 ||
				(powerPct > 0 && flat > 0) || guaranteedHit {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 restore_mp 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.RestoreMPFlat = domain.EffectRestoreMP, flat
			effect.SourceAttackPct = powerPct
		case "restore_hp":
			if powerPct <= 0 || resourcePct != 0 || flat != 0 || guaranteedHit || effect.Target != domain.EffectTargetCaster {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 restore_hp 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.SourceAttackPct = domain.EffectRestoreHP, powerPct
		case "damage_mp":
			if powerPct <= 0 || resourcePct != 0 || flat != 0 || guaranteedHit || effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 damage_mp 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.SourceAttackPct = domain.EffectDamageMP, powerPct
		case "damage_mp_max":
			if powerPct != 0 || resourcePct <= 0 || resourcePct > 100 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 damage_mp_max 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.MaxResourcePct = domain.EffectDamageMPMax, resourcePct
		case "taunt":
			if powerPct != 0 || resourcePct != 0 || flat <= 0 || guaranteedHit || chanceBP != 10000 ||
				effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 taunt 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.TauntPower = domain.EffectTaunt, flat
		case "teleport_point":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetCaster {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 teleport_point 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.ChanceBP = domain.EffectTeleportPoint, chanceBP
		case "teleport_target":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit || chanceBP != 10000 ||
				effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 teleport_target 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.ChanceBP = domain.EffectTeleportTarget, chanceBP
		case "place_trap":
			if powerPct != 0 || resourcePct != 0 || flat <= 0 || guaranteedHit || chanceBP != 10000 ||
				effect.Target != domain.EffectTargetCaster {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 place_trap 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.TrapRadius = domain.EffectPlaceTrap, flat
		case "detect_trap":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetCaster {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 detect_trap 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.ChanceBP = domain.EffectDetectTrap, chanceBP
		case "disarm_trap":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetResolved {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 disarm_trap 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.ChanceBP = domain.EffectDisarmTrap, chanceBP
		case "reveal_invisible":
			if powerPct != 0 || resourcePct != 0 || flat <= 0 || guaranteedHit || chanceBP != 10000 ||
				effect.Target != domain.EffectTargetCaster {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 reveal_invisible 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.RevealRadius = domain.EffectRevealInvisible, flat
		case "instant_death":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetResolved || lossRefundPct != 0 {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 instant_death 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.ChanceBP = domain.EffectInstantDeath, chanceBP
		case "resurrect":
			if powerPct != 0 || resourcePct <= 0 || resourcePct > 100 || flat != 0 || guaranteedHit ||
				chanceBP != 10000 || effect.Target != domain.EffectTargetResolved ||
				lossRefundPct < 0 || lossRefundPct > 100 {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 resurrect 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind = domain.EffectResurrect
			effect.HealPctOfMax = resourcePct
		case "knockback":
			if powerPct != 0 || resourcePct != 0 || flat <= 0 || guaranteedHit || chanceBP != 10000 ||
				effect.Target != domain.EffectTargetResolved || lossRefundPct != 0 {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 knockback 效果参数非法", key.ID, key.Level, order)
			}
			effect.Kind, effect.KnockbackDistance = domain.EffectKnockback, flat
		case "clear_harmful":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit || chanceBP != 10000 || lossRefundPct != 0 {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 clear_harmful 效果参数非法", key.ID, key.Level, order)
			}
			effect.ClearHarmful = true
		case "remove_statuses":
			if powerPct != 0 || resourcePct != 0 || flat != 0 || guaranteedHit ||
				effect.Target != domain.EffectTargetResolved || lossRefundPct != 0 {
				return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个 remove_statuses 效果参数非法", key.ID, key.Level, order)
			}
			effect.DispelChanceBP = chanceBP
		default:
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个效果类型 %q 未实现", key.ID, key.Level, order, kind)
		}
		positions[effectOrderKey{Skill: key, Order: order}] = len(out[key])
		out[key] = append(out[key], effect)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历结构化技能效果: %w", err)
	}
	rows.Close()

	dispelRows, err := q.Query(ctx, `
		SELECT d.skill_id, d.skill_level, d.effect_order, d.status_id,
		       EXISTS (SELECT 1 FROM gamedata.ov_exceptdetail x WHERE x.type = d.status_id)
		       OR EXISTS (SELECT 1 FROM game_custom_statuses x WHERE x.status_id = d.status_id)
		  FROM game_skill_effect_remove_statuses d
		 ORDER BY d.skill_id, d.skill_level, d.effect_order, d.status_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查选择性驱散状态: %w", err)
	}
	defer dispelRows.Close()
	for dispelRows.Next() {
		var key domain.SkillKey
		var order int32
		var statusID domain.StatusID
		var statusOK bool
		if err := dispelRows.Scan(&key.ID, &key.Level, &order, &statusID, &statusOK); err != nil {
			return nil, fmt.Errorf("data: 读选择性驱散状态: %w", err)
		}
		position, ok := positions[effectOrderKey{Skill: key, Order: order}]
		if !ok || position >= len(out[key]) || out[key][position].DispelChanceBP == 0 {
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个驱散状态没有 remove_statuses 主效果", key.ID, key.Level, order)
		}
		if !statusOK {
			return nil, fmt.Errorf("data: 技能 %d/%d 第 %d 个驱散引用不存在的状态 %d", key.ID, key.Level, order, statusID)
		}
		effect := out[key][position]
		effect.RemoveStatusIDs = append(effect.RemoveStatusIDs, statusID)
		out[key][position] = effect
	}
	if err := dispelRows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历选择性驱散状态: %w", err)
	}

	for key, effects := range out {
		count := 0
		special, placeTrap := 0, 0
		for _, effect := range effects {
			if effect.WeaponDurabilityCostPct > 0 {
				count++
			}
			switch effect.Kind {
			case domain.EffectTeleportPoint, domain.EffectTeleportTarget,
				domain.EffectPlaceTrap, domain.EffectDetectTrap, domain.EffectDisarmTrap,
				domain.EffectRevealInvisible:
				special++
				if effect.Kind == domain.EffectPlaceTrap {
					placeTrap++
				}
			}
			if effect.DispelChanceBP > 0 && len(effect.RemoveStatusIDs) == 0 {
				return nil, fmt.Errorf("data: 技能 %d/%d 的选择性驱散没有状态白名单", key.ID, key.Level)
			}
		}
		if count > 1 {
			return nil, fmt.Errorf("data: 技能 %d/%d 重复配置武器耐久消耗", key.ID, key.Level)
		}
		if special > 1 || special == 1 && placeTrap == 0 && len(effects) != 1 {
			return nil, fmt.Errorf("data: 技能 %d/%d 的场景机制效果配置冲突", key.ID, key.Level)
		}
		if placeTrap == 1 && len(effects) < 2 {
			return nil, fmt.Errorf("data: 技能 %d/%d 的陷阱缺少触发结果", key.ID, key.Level)
		}
	}
	return out, nil
}

func loadSkillPassiveStats(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveStatModifier, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.modifier_order,
		       p.attr_id,p.value,p.mode,p.required_equip_type,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       )
		  FROM game_skill_passive_stat_modifiers p
		 ORDER BY p.passive_skill_id,p.passive_skill_level,p.modifier_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动属性修正: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveStatModifier)
	for rows.Next() {
		var key domain.SkillKey
		var order int32
		var modifier domain.PassiveStatModifier
		var passiveOK bool
		if err := rows.Scan(&key.ID, &key.Level, &order, &modifier.Attr, &modifier.Value,
			&modifier.Mode, &modifier.RequiredEquipType, &passiveOK); err != nil {
			return nil, fmt.Errorf("data: 读被动属性修正: %w", err)
		}
		if !passiveOK || modifier.Attr <= 0 ||
			(modifier.Mode != domain.ModeAbsolute && modifier.Mode != domain.ModePercent) {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条属性修正非法",
				key.ID, key.Level, order)
		}
		out[key] = append(out[key], modifier)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动属性修正: %w", err)
	}
	return out, nil
}

func loadSkillPassiveStatusResists(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveStatusResist, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.resist_order,
		       p.status_id,p.resist_pct,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       ),
		       EXISTS (SELECT 1 FROM gamedata.ov_exceptdesc x WHERE x.type=p.status_id)
		  FROM game_skill_passive_status_resists p
		 ORDER BY p.passive_skill_id,p.passive_skill_level,p.resist_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动状态抗性: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveStatusResist)
	for rows.Next() {
		var key domain.SkillKey
		var order int32
		var resist domain.PassiveStatusResist
		var passiveOK, statusOK bool
		if err := rows.Scan(&key.ID, &key.Level, &order, &resist.Status, &resist.Pct,
			&passiveOK, &statusOK); err != nil {
			return nil, fmt.Errorf("data: 读被动状态抗性: %w", err)
		}
		if !passiveOK || !statusOK || resist.Pct <= 0 || resist.Pct > 100 {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条状态抗性非法",
				key.ID, key.Level, order)
		}
		out[key] = append(out[key], resist)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动状态抗性: %w", err)
	}
	return out, nil
}

func loadSkillPassiveTriggerStatuses(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveTriggerStatus, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.trigger_order,
		       p.target_skill_id,p.status_id,p.status_level,p.chance_bp,p.duration_sec,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       ),
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.target_skill_id AND d.prof BETWEEN 1 AND 5
		       ),
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_exceptdetail d
		           JOIN gamedata.ov_exceptdetail_entry e ON e.row_no=d.row_no
		            WHERE d.type=p.status_id AND e.except_level=p.status_level
		           UNION ALL
		           SELECT 1 FROM game_custom_statuses c
		            WHERE c.status_id=p.status_id AND c.status_level=p.status_level
		       )
		  FROM game_skill_passive_trigger_statuses p
		 ORDER BY p.passive_skill_id,p.passive_skill_level,p.trigger_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动触发状态: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveTriggerStatus)
	for rows.Next() {
		var key domain.SkillKey
		var order int32
		var trigger domain.PassiveTriggerStatus
		var passiveOK, targetOK, statusOK bool
		if err := rows.Scan(&key.ID, &key.Level, &order, &trigger.TargetSkill,
			&trigger.Status, &trigger.StatusLevel, &trigger.ChanceBP, &trigger.DurationSec,
			&passiveOK, &targetOK, &statusOK); err != nil {
			return nil, fmt.Errorf("data: 读被动触发状态: %w", err)
		}
		if !passiveOK || !targetOK || !statusOK || trigger.ChanceBP <= 0 ||
			trigger.ChanceBP > 10000 || trigger.DurationSec < 0 {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条触发状态非法",
				key.ID, key.Level, order)
		}
		out[key] = append(out[key], trigger)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动触发状态: %w", err)
	}
	return out, nil
}

func loadSkillPassiveStatusAffixes(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveStatusAffix, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.modifier_order,
		       p.target_skill_id,p.status_id,p.attr_id,p.value,p.mode,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       ),
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.target_skill_id AND d.prof BETWEEN 1 AND 5
		       )
		  FROM game_skill_passive_status_affixes p
		 ORDER BY p.passive_skill_id,p.passive_skill_level,p.modifier_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动状态属性: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveStatusAffix)
	for rows.Next() {
		var key domain.SkillKey
		var order int32
		var modifier domain.PassiveStatusAffix
		var passiveOK, targetOK bool
		if err := rows.Scan(&key.ID, &key.Level, &order, &modifier.TargetSkill,
			&modifier.Status, &modifier.Affix.Attr, &modifier.Affix.Value,
			&modifier.Affix.Mode, &passiveOK, &targetOK); err != nil {
			return nil, fmt.Errorf("data: 读被动状态属性: %w", err)
		}
		if !passiveOK || !targetOK || modifier.Status <= 0 || modifier.Affix.Attr <= 0 ||
			(modifier.Affix.Mode != domain.ModeAbsolute && modifier.Affix.Mode != domain.ModePercent) {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条状态属性非法",
				key.ID, key.Level, order)
		}
		out[key] = append(out[key], modifier)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动状态属性: %w", err)
	}
	return out, nil
}

func loadSkillPassiveTriggerEffects(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveTriggerEffect, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.trigger_order,
		       p.target_skill_id,p.effect_kind,p.flat_value,p.chance_bp,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       ),
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.target_skill_id AND d.prof BETWEEN 1 AND 5
		       )
		  FROM game_skill_passive_trigger_effects p
		 ORDER BY p.passive_skill_id,p.passive_skill_level,p.trigger_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动触发效果: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveTriggerEffect)
	for rows.Next() {
		var key domain.SkillKey
		var order int32
		var kind string
		var trigger domain.PassiveTriggerEffect
		var passiveOK, targetOK bool
		if err := rows.Scan(&key.ID, &key.Level, &order, &trigger.TargetSkill,
			&kind, &trigger.Effect.KnockbackDistance, &trigger.Effect.ChanceBP,
			&passiveOK, &targetOK); err != nil {
			return nil, fmt.Errorf("data: 读被动触发效果: %w", err)
		}
		if kind != "knockback" || !passiveOK || !targetOK ||
			trigger.Effect.KnockbackDistance <= 0 || trigger.Effect.ChanceBP <= 0 ||
			trigger.Effect.ChanceBP > 10000 {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条触发效果非法",
				key.ID, key.Level, order)
		}
		trigger.Effect.Kind = domain.EffectKnockback
		trigger.Effect.RequiresPreviousLanded = true
		out[key] = append(out[key], trigger)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动触发效果: %w", err)
	}
	return out, nil
}

func loadSkillPassiveCounterattacks(ctx context.Context, q Querier) (map[domain.SkillKey]*domain.PassiveCounterAttack, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id,p.passive_skill_level,p.chance_bp,p.damage_pct,
		       p.required_equip_type,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id=p.passive_skill_id
		              AND d.skill_level=p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type=18
		       )
		  FROM game_skill_passive_counterattacks p
		 ORDER BY p.passive_skill_id,p.passive_skill_level`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动反击: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey]*domain.PassiveCounterAttack)
	for rows.Next() {
		var key domain.SkillKey
		var counter domain.PassiveCounterAttack
		var passiveOK bool
		if err := rows.Scan(&key.ID, &key.Level, &counter.ChanceBP, &counter.DamagePct,
			&counter.RequiredEquipType, &passiveOK); err != nil {
			return nil, fmt.Errorf("data: 读被动反击: %w", err)
		}
		if !passiveOK || counter.ChanceBP <= 0 || counter.ChanceBP > 10000 ||
			counter.DamagePct <= 0 || counter.RequiredEquipType < 0 {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 的反击规则非法", key.ID, key.Level)
		}
		copy := counter
		out[key] = &copy
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动反击: %w", err)
	}
	return out, nil
}

func loadSkillPassiveModifiers(ctx context.Context, q Querier) (map[domain.SkillKey][]domain.PassiveModifier, error) {
	rows, err := q.Query(ctx, `
		SELECT p.passive_skill_id, p.passive_skill_level, p.modifier_order,
		       p.modifier_kind, p.target_skill_id, p.effect_kind, p.value,
		       EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id = p.passive_skill_id
		              AND d.skill_level = p.passive_skill_level
		              AND d.prof BETWEEN 1 AND 5 AND d.skill_type = 18
		       ),
		       p.target_skill_id = 0 OR EXISTS (
		           SELECT 1 FROM gamedata.ov_skilldesc d
		            WHERE d.skill_id = p.target_skill_id AND d.prof BETWEEN 1 AND 5
		       )
		  FROM game_skill_passive_modifiers p
		 ORDER BY p.passive_skill_id, p.passive_skill_level, p.modifier_order`)
	if err != nil {
		return nil, fmt.Errorf("data: 查被动技能修正: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.SkillKey][]domain.PassiveModifier)
	for rows.Next() {
		var (
			key                 domain.SkillKey
			order               int32
			kind, effectKind    string
			m                   domain.PassiveModifier
			passiveOK, targetOK bool
		)
		if err := rows.Scan(&key.ID, &key.Level, &order, &kind, &m.TargetSkill,
			&effectKind, &m.Value, &passiveOK, &targetOK); err != nil {
			return nil, fmt.Errorf("data: 读被动技能修正: %w", err)
		}
		if !passiveOK || !targetOK {
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条修正引用不存在的被动或目标技能",
				key.ID, key.Level, order)
		}
		switch kind {
		case "skill_damage_pct":
			m.Kind = domain.PassiveSkillDamagePct
		case "effect_source_attack_pct":
			m.Kind = domain.PassiveEffectSourceAttackPct
			switch effectKind {
			case "damage_mp":
				m.EffectKind = domain.EffectDamageMP
			case "restore_hp":
				m.EffectKind = domain.EffectRestoreHP
			case "restore_mp":
				m.EffectKind = domain.EffectRestoreMP
			default:
				return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条效果类型 %q 未实现",
					key.ID, key.Level, order, effectKind)
			}
		case "cast_time_reduction_pct":
			m.Kind = domain.PassiveCastTimeReductionPct
		case "cast_interrupt_reduction_pct":
			m.Kind = domain.PassiveCastInterruptReductionPct
		case "incoming_magic_half_chance":
			m.Kind = domain.PassiveIncomingMagicHalfChance
		case "stealth_duration_sec":
			m.Kind = domain.PassiveStealthDurationSec
		case "stealth_move_speed_pct":
			m.Kind = domain.PassiveStealthMoveSpeedPct
		case "stealth_damage_pct":
			m.Kind = domain.PassiveStealthDamagePct
		case "preserve_invisible_chance":
			m.Kind = domain.PassivePreserveInvisibleChance
		case "skill_durability_reduction_pct":
			m.Kind = domain.PassiveSkillDurabilityReductionPct
		case "attack_durability_reduction_pct":
			m.Kind = domain.PassiveAttackDurabilityReductionPct
		case "trap_half_damage_chance":
			m.Kind = domain.PassiveTrapHalfDamageChance
		case "status_physical_damage_taken_pct":
			m.Kind = domain.PassiveStatusPhysicalDamageTakenPct
		default:
			return nil, fmt.Errorf("data: 被动技能 %d/%d 第 %d 条类型 %q 未实现",
				key.ID, key.Level, order, kind)
		}
		out[key] = append(out[key], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历被动技能修正: %w", err)
	}
	return out, nil
}

// 从 desc_ 里抠效果。
//
// 两种句式各有变体, 正则要都吃下:
//
//	「对敌人造成130%伤害」「对...范围内的敌人造成95%的伤害」  → 伤害倍率
//	「目标恢复最大生命的10%+100点当前生命」                  → 治疗
var (
	reDamage = regexp.MustCompile(`造成([0-9]+)%的?伤害`)
	reHeal   = regexp.MustCompile(`恢复最大生命的([0-9]+)%\+([0-9]+)点`)
)

func parseEffect(desc string) domain.SkillEffect {
	if m := reHeal.FindStringSubmatch(desc); m != nil {
		return domain.SkillEffect{
			Kind:         domain.EffectHeal,
			HealPctOfMax: atoi32s(m[1]),
			HealFlat:     atoi32s(m[2]),
		}
	}
	if m := reDamage.FindStringSubmatch(desc); m != nil {
		return domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: atoi32s(m[1])}
	}
	return domain.SkillEffect{Kind: domain.EffectNone}
}

func atoi32s(s string) int32 {
	n, _ := strconv.Atoi(s)
	return int32(n)
}

// mpCost 解 sp_chg_start。
//
// **[实证]** sp_chg_control 决定怎么读:
//
//	0 → 直接就是消耗值(515 行, 0~554; 那 190 个 0 正好是全部被动技能)
//	1 → **低 16 位**才是消耗(235 行, 23~1092); 高 16 位(13~90)含义未知, 不用
func mpCost(raw int64, control int32) int32 {
	if control == 0 {
		return int32(raw)
	}
	return int32(raw & 0xFFFF)
}

// skillStatusApplications 是从客户端解包的技能描述与状态明细闭合出的交叉表。
//
// 客户端包没有 ov_skillresult.bin，ov_skilldesc.except_ 也全零，所以这里不能做
// “读一个外键”式加载。只收下面两类证据足够闭合的项：
//   - 技能五级描述中的状态数值/时长，与状态五级定义逐项相同；
//   - 同一状态的某段等级恰好逐项匹配技能五级。
//
// 若数值逐级一致、但技能文本明确给出不同持续时间，则在 StatusApplication 上
// 单独覆盖（祝福术就是这一类），不污染物品复用的基础状态时长。
//
// 缺概率（神威术）、必须覆盖状态时长（燃烧/时之封印）或仅名称相同的技能
// 不在这里猜，等各自参数闭合后再加入。
func skillStatusApplications(id domain.SkillID, level int32) []domain.StatusApplication {
	status := func(id domain.StatusID, level int32) []domain.StatusApplication {
		return []domain.StatusApplication{{ID: id, Level: level}}
	}
	switch id {
	case 14002, 14003, 14007, 14008, 14009, 14010, 14011, 14012,
		14014, 14015, 14016, 14017, 14018, 14019:
		// 法宝状态使用技能号作为稳定身份；具体数值由 LoadStatuses 从同一条
		// ov_skilldesc 记录构造，避免把法宝临时技能写进角色永久技能表。
		return status(domain.StatusID(id), 1)
	case domain.SkillBeginnerGuard: // 守护术：30 秒内物理/魔法伤害抗性均 +10%
		return status(domain.StatusBeginnerGuard, 1)
	case 10010: // 狂暴术 60..100% ↔ 狂暴 1013/6..10，均 30 秒
		return []domain.StatusApplication{{
			ID: 1013, Level: level + 5,
			BlockHarmful: true, Control: domain.ControlSilence,
		}}
	case 10011, 10130: // 怒火杀意 / 雷霆之怒 ↔ 杀意 1025/1..5
		return status(1025, level)
	case 10027, 10126: // 重击 / 强击：昏迷 6..10 秒
		return status(1002, level+10)
	case 10029, 10128: // 震慑 / 煞气：昏迷 5..9 秒
		return status(1002, [...]int32{0, 16, 11, 12, 13, 14}[level])
	case 10208: // 隐身术：状态表保存身份/时长，技能说明保存精确移速与攻击增幅。
		movePct := [...]int32{0, -25, -20, -15, -5, 0}[level]
		return []domain.StatusApplication{{
			ID: domain.StatusStealth, Level: level, DurationSec: 28 + level*2,
			ExtraAffixes: []domain.Affix{{
				Attr: domain.AttrMoveSpeed, Value: movePct, Mode: domain.ModePercent,
			}},
			StealthDamagePct: level * 35,
		}}
	case 10211: // 淬毒术先给自身“使毒”；普攻触发中毒属于后续结算语义
		return status(1018, level)
	case 10224: // 烟雾弹同时降低命中与魔攻 30/35/40/45/50%
		return []domain.StatusApplication{{ID: 1008, Level: level + 10}, {ID: 1007, Level: level + 10}}
	case 10307: // 祝福术：攻击 +10/12/14/16/18%；技能文本明确持续 5 分钟
		// 状态样式由客户端通过技能号取得，位置由 BuffSnapshot 统一指定为环绕人物；
		// 这里不再单独硬编码祝福术特效，避免只有强化被特殊处理。
		return []domain.StatusApplication{{
			ID: 1014, Level: level, DurationSec: 300,
			ExtraAffixes: []domain.Affix{{
				Attr: domain.AttrMAtk, Value: level * 5, Mode: domain.ModePercent,
			}},
			Description: fmt.Sprintf("攻击力上升%d%%；魔法攻击力上升%d%%。", 8+level*2, level*5),
		}}
	case 10315: // 瞬击术：一分钟内攻速 +25/27/29/31/33%
		return status(1020, level)
	case 10322, 10418: // 神术/秘术：集中
		return status(1026, level)
	case 10328, 10419: // 神术/秘术：释能
		return status(1027, level)
	case 10329, 10420: // 神术/秘术：瞬发
		return status(1028, level)
	case 10425: // 火盾术：减伤与反射比例逐级一致
		return status(1037, level)
	case 10435: // 冰盾术：减伤与冻结概率逐级一致
		return status(1038, level)
	default:
		return nil
	}
}

// LoadSkills 读五职业技能、两个初行者技能，以及由 ov_arm/ov_skilldesc 双向
// 绑定的法宝技能。其余 prof=0 行仍是宠物/怪物技能，不能混入玩家技能表。
func LoadSkills(ctx context.Context, q Querier) (domain.SkillTable, SkillStats, error) {
	groundEffects, err := loadGroundSkillEffects(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	areaCenters, err := loadSkillAreaCenters(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	areaHitEffects, err := loadAreaSkillHitEffects(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	structured, err := loadStructuredSkillEffects(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	structuredStatuses, err := loadStructuredSkillStatuses(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveModifiers, err := loadSkillPassiveModifiers(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveStats, err := loadSkillPassiveStats(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveResists, err := loadSkillPassiveStatusResists(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveTriggers, err := loadSkillPassiveTriggerStatuses(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveStatusAffixes, err := loadSkillPassiveStatusAffixes(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveTriggerEffects, err := loadSkillPassiveTriggerEffects(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	passiveCounters, err := loadSkillPassiveCounterattacks(ctx, q)
	if err != nil {
		return nil, SkillStats{}, err
	}
	rows, err := q.Query(ctx, `
		SELECT sd.skill_id, sd.skill_level, sd.name, sd.small_map, sd.prof, sd.skill_type,
		       sd.level_need, COALESCE(sd.book_id,0), COALESCE(sd.learn_money,0),
		       COALESCE(sd.depend1_id,0), COALESCE(sd.depend1_qty,0),
		       COALESCE(sd.depend2_id,0), COALESCE(sd.depend2_qty,0),
		       COALESCE(sd.depend3_id,0), COALESCE(sd.depend3_qty,0),
		       COALESCE(sd.depend4_id,0), COALESCE(sd.depend4_qty,0), sd.hurt_type,
		       COALESCE(sd.desc_,''), sd.dist, sd.radius, sd.prepare, sd.sep,
		       sd.sp_chg_start, sd.sp_chg_control,
		       sd.target_self, sd.target_mon, sd.target_team, COALESCE(sd.binding_arm,0),
		       COALESCE(sd.arm_type,0), COALESCE(sd.range_type,0),
		       COALESCE(sd.angle,0), COALESCE(sd.width,0), COALESCE(sd.height,0),
		       COALESCE(sshow.fly_show_id,0), COALESCE(sshow.fly_show_delay,0),
		       COALESCE(sshow.atk_show_delay,0), COALESCE(sshow.be_h_show_delay,0),
		       COALESCE(sshow.r_be_h_show_delay,0)
		  FROM gamedata.ov_skilldesc sd
		  LEFT JOIN LATERAL (
		       SELECT fly_show_id, fly_show_delay, atk_show_delay,
		              be_h_show_delay, r_be_h_show_delay
		         FROM gamedata.ov_skillshow ss
		        WHERE ss.skill_id = sd.skill_id
		        ORDER BY ss.row_no
		        LIMIT 1
		  ) sshow ON TRUE
		 WHERE (
		       sd.prof BETWEEN 1 AND 5
		       OR (sd.prof = 0 AND sd.skill_id IN (13601,13602))
		       OR (sd.prof = 0 AND sd.skill_id IN
		           (11001,11002,11003,11004,11006,11007,11008,11009,11010))
		       OR (sd.prof = 0 AND sd.binding_arm > 0 AND EXISTS (
		           SELECT 1 FROM gamedata.ov_arm a
		            WHERE a.index = sd.binding_arm AND a.binding_skill = sd.skill_id
		       ))
		   )
		   AND sd.skill_id > 0 AND sd.skill_level > 0`)
	if err != nil {
		return nil, SkillStats{}, fmt.Errorf("data: 查 ov_skilldesc: %w", err)
	}
	defer rows.Close()

	out := domain.SkillTable{}
	st := SkillStats{}
	seenAreaCenters := make(map[domain.SkillID]bool, len(areaCenters))
	for rows.Next() {
		var (
			d                          domain.SkillDef
			skillType, hurtType        int64
			desc                       string
			spStart                    int64
			spControl                  int32
			sep                        int32
			tSelf, tMon, tTeam         int32
			prof, level                int32
			bindingArm                 int64
			bookID, learnMoney         int64
			dependencyIDs              [4]int64
			dependencyQtys             [4]int32
			rangeType, angle           int32
			rangeWidth, rangeHeight    int32
			flyShowID, flyDelayMS      int32
			attackDelayMS              int32
			hitDelayMS, areaHitDelayMS int32
		)
		if err := rows.Scan(&d.ID, &level, &d.Name, &d.Icon, &prof, &skillType, &d.LevelNeed,
			&bookID, &learnMoney,
			&dependencyIDs[0], &dependencyQtys[0], &dependencyIDs[1], &dependencyQtys[1],
			&dependencyIDs[2], &dependencyQtys[2], &dependencyIDs[3], &dependencyQtys[3],
			&hurtType, &desc, &d.Dist, &d.Radius, &d.PrepareMS, &sep,
			&spStart, &spControl, &tSelf, &tMon, &tTeam, &bindingArm, &d.RequiredWeaponType,
			&rangeType, &angle, &rangeWidth, &rangeHeight, &flyShowID, &flyDelayMS,
			&attackDelayMS, &hitDelayMS, &areaHitDelayMS); err != nil {
			return nil, st, fmt.Errorf("data: 读技能行: %w", err)
		}
		d.Level, d.Prof = level, prof
		d.Book, d.LearnMoney = domain.ItemID(bookID), learnMoney
		seenDependencies := map[domain.SkillID]struct{}{}
		for i, rawID := range dependencyIDs {
			if rawID == 0 {
				continue
			}
			if dependencyQtys[i] != 0 {
				return nil, st, fmt.Errorf("data: 技能 %d/%d 前置技能 %d 的 qty=%d 语义未闭合",
					d.ID, d.Level, rawID, dependencyQtys[i])
			}
			dependency := domain.SkillID(rawID)
			if dependency == d.ID {
				return nil, st, fmt.Errorf("data: 技能 %d/%d 不能依赖自身", d.ID, d.Level)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				continue
			}
			seenDependencies[dependency] = struct{}{}
			d.Prerequisites = append(d.Prerequisites, dependency)
		}
		d.Description = desc
		d.RequiresInvisible = strings.Contains(desc, "只能在隐身状态下使用")
		d.BindingArm = domain.ItemID(bindingArm)
		d.Kind = domain.ParseSkillKind(int32(skillType))
		d.GroundEffect = groundEffects[d.ID]
		if d.Kind == domain.SkillArea {
			d.HitEffect = areaHitEffects[d.ID]
		}
		d.HurtType = int32(hurtType)
		d.Effect = parseEffect(desc)
		if d.ID == 14001 || d.ID == 14007 || d.ID >= 14016 && d.ID <= 14019 {
			d.Effect.ClearHarmful = true
		}
		key := domain.SkillKey{ID: d.ID, Level: d.Level}
		if modifiers := passiveModifiers[key]; len(modifiers) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动修正", d.ID, d.Level)
			}
			d.PassiveModifiers = append([]domain.PassiveModifier(nil), modifiers...)
			delete(passiveModifiers, key)
		}
		if modifiers := passiveStats[key]; len(modifiers) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动属性", d.ID, d.Level)
			}
			d.PassiveStats = append([]domain.PassiveStatModifier(nil), modifiers...)
			delete(passiveStats, key)
		}
		if resists := passiveResists[key]; len(resists) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动状态抗性", d.ID, d.Level)
			}
			d.PassiveResists = append([]domain.PassiveStatusResist(nil), resists...)
			delete(passiveResists, key)
		}
		if triggers := passiveTriggers[key]; len(triggers) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动触发状态", d.ID, d.Level)
			}
			d.PassiveTriggers = append([]domain.PassiveTriggerStatus(nil), triggers...)
			delete(passiveTriggers, key)
		}
		if modifiers := passiveStatusAffixes[key]; len(modifiers) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动状态属性", d.ID, d.Level)
			}
			d.PassiveStatusAffixes = append([]domain.PassiveStatusAffix(nil), modifiers...)
			delete(passiveStatusAffixes, key)
		}
		if effects := passiveTriggerEffects[key]; len(effects) > 0 {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动触发效果", d.ID, d.Level)
			}
			d.PassiveTriggerEffects = append([]domain.PassiveTriggerEffect(nil), effects...)
			delete(passiveTriggerEffects, key)
		}
		if counter := passiveCounters[key]; counter != nil {
			if d.Kind != domain.SkillPassive {
				return nil, st, fmt.Errorf("data: 主动技能 %d/%d 不能配置被动反击", d.ID, d.Level)
			}
			copy := *counter
			d.PassiveCounter = &copy
			delete(passiveCounters, key)
		}
		d.Statuses = skillStatusApplications(d.ID, d.Level)
		if statuses := structuredStatuses[key]; len(statuses) > 0 {
			if len(d.Statuses) > 0 {
				return nil, st, fmt.Errorf("data: 技能 %d/%d 同时存在旧状态映射与结构化状态配置", d.ID, d.Level)
			}
			d.Statuses = append([]domain.StatusApplication(nil), statuses...)
			delete(structuredStatuses, key)
			if d.Prof != 0 {
				st.StructuredStatuses++
			}
		}
		if effects := structured[key]; len(effects) > 0 {
			d.Effects = append([]domain.SkillEffect(nil), effects...)
			// Effect 在全部调用点迁到有序链之前保留首效果镜像；正式结算只读 Effects。
			d.Effect = d.Effects[0]
			delete(structured, key)
		}
		d.CooldownMS = sep * 100
		if flyShowID > 0 && flyDelayMS > 0 && d.Dist > 0 {
			d.ProjectileSpeedPXPerSec = int32(math.Round(float64(d.Dist) * 1000 / float64(flyDelayMS)))
			if d.ProjectileSpeedPXPerSec < 1 {
				d.ProjectileSpeedPXPerSec = 1
			}
		} else {
			d.ImpactDelayMS = attackDelayMS
			if hitDelayMS > d.ImpactDelayMS {
				d.ImpactDelayMS = hitDelayMS
			}
			if areaHitDelayMS > d.ImpactDelayMS {
				d.ImpactDelayMS = areaHitDelayMS
			}
		}
		d.MPCost = mpCost(spStart, spControl)
		d.TargetSelf, d.TargetEnemy, d.TargetTeam = tSelf != 0, tMon != 0, tTeam != 0
		if center, ok := areaCenters[d.ID]; ok {
			d.Area = domain.SkillAreaTargeting{
				Shape: domain.ParseSkillAreaShape(rangeType), Center: center,
				Radius: d.Radius, Angle: angle, Length: rangeWidth, Width: rangeHeight,
			}
			if !d.Area.Valid() {
				return nil, st, fmt.Errorf("data: 技能 %d/%d 的范围几何非法: type=%d radius=%d angle=%d width=%d height=%d",
					d.ID, d.Level, rangeType, d.Radius, angle, rangeWidth, rangeHeight)
			}
			seenAreaCenters[d.ID] = true
		} else if d.Prof >= 1 && d.Prof <= 5 && d.Kind == domain.SkillArea &&
			!skillHasIndependentTargeting(d) {
			return nil, st, fmt.Errorf("data: 职业范围技能 %d/%d 缺少中心规则", d.ID, d.Level)
		}

		// SkillStats 保持统计五职业的 750 行；两个初行者技能另行装表，
		// 不混进历史容量/可用率指标。
		if d.Prof != 0 {
			st.Total++
			if len(d.Effects) > 0 {
				st.StructuredEffects++
			}
			switch {
			case !d.Kind.Active():
				st.Passive++
			case len(d.Statuses) > 0:
				st.WithStatus++
			case len(d.DirectEffects()) == 0:
				st.NoEffect++
			}
			st.Loaded++
		}
		out[key] = d
	}
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历技能: %w", err)
	}
	for key := range structured {
		return nil, st, fmt.Errorf("data: 结构化技能效果引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range structuredStatuses {
		return nil, st, fmt.Errorf("data: 结构化技能状态引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveModifiers {
		return nil, st, fmt.Errorf("data: 被动修正引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveStats {
		return nil, st, fmt.Errorf("data: 被动属性引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveResists {
		return nil, st, fmt.Errorf("data: 被动状态抗性引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveTriggers {
		return nil, st, fmt.Errorf("data: 被动触发状态引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveStatusAffixes {
		return nil, st, fmt.Errorf("data: 被动状态属性引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveTriggerEffects {
		return nil, st, fmt.Errorf("data: 被动触发效果引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for key := range passiveCounters {
		return nil, st, fmt.Errorf("data: 被动反击引用不存在或非玩家技能的定义 %d/%d", key.ID, key.Level)
	}
	for id := range areaCenters {
		if !seenAreaCenters[id] {
			return nil, st, fmt.Errorf("data: 技能范围中心规则引用不存在或非职业范围技能 %d", id)
		}
	}
	for key, def := range out {
		for _, dependency := range def.Prerequisites {
			if out.MaxLevelOf(dependency) == 0 {
				return nil, st, fmt.Errorf("data: 技能 %d/%d 引用了不存在的前置技能 %d",
					key.ID, key.Level, dependency)
			}
		}
	}
	return out, st, nil
}

// StatusStats 记录状态加载的情况。
type StatusStats struct {
	Defs     int // 装入的状态×等级数
	Statuses int // 涉及多少个状态
	BadRows  int // 被过滤掉的效果行
}

// LoadStatuses 读状态定义。三张表拼:
//
//	ov_exceptdesc                名字(type = 状态号)
//	ov_exceptdetail_entry        每级的持续时间与周期
//	ov_exceptdetail_effect_entry 每级的属性影响
//
// ⚠️ **只认 op_type=1、mode∈{0,1}、attr_id 在字典里、prob=100 的确定效果行。**
// 3195 行里只有 603 行(82 个状态)过得了这一关。其余是 mode=87/176/219、
// value=25600 这种明显错位的残渣 —— 和装备词条里"没有 attr_name"的那批同类。
// 过滤掉的行会计数上报, 不静默丢。
func LoadStatuses(ctx context.Context, q Querier) (domain.StatusTable, StatusStats, error) {
	out := domain.StatusTable{}
	st := StatusStats{}

	// 1. 每级的时长与周期
	rows, err := q.Query(ctx, `
		SELECT d.type,
		       COALESCE(to_jsonb(x)->>('lv' || e.except_level::text || '_name'), x.lv1_name, ''),
		       COALESCE(to_jsonb(x)->>('lv' || e.except_level::text || '_desc'), x.lv1_desc, ''),
		       e.except_level, e.last_time, e.interval, e.no_inform_client, e.except_type
		  FROM gamedata.ov_exceptdetail d
		  JOIN gamedata.ov_exceptdetail_entry e ON e.row_no = d.row_no
		  LEFT JOIN gamedata.ov_exceptdesc x ON x.type = d.type
		 WHERE d.type > 0 AND e.last_time > 0`)
	if err != nil {
		return nil, st, fmt.Errorf("data: 查状态时长: %w", err)
	}
	for rows.Next() {
		var (
			typ              int64
			level            int64
			last, interval   int32
			name, desc       string
			hidden, typClass int16
		)
		if err := rows.Scan(&typ, &name, &desc, &level, &last, &interval, &hidden, &typClass); err != nil {
			rows.Close()
			return nil, st, fmt.Errorf("data: 读状态时长行: %w", err)
		}
		if typClass < int16(domain.StatusGood) || typClass > int16(domain.StatusGoodNew) {
			rows.Close()
			return nil, st, fmt.Errorf("data: 状态 %d 等级 %d 的 except_type=%d 越界", typ, level, typClass)
		}
		id := domain.StatusID(typ)
		d := domain.StatusDef{
			ID: id, Level: int32(level), Name: name, Desc: desc,
			Type: domain.StatusType(typClass), TypeKnown: true,
			DurationSec: last, IntervalSec: interval, Hidden: hidden != 0,
		}
		if id == 1032 { // 隐身：状态说明明确，且 0x8017 有独立 invisible 位
			d.Invisible = true
		}
		out[domain.StatusKey{ID: id, Level: d.Level}] = d
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历状态时长: %w", err)
	}

	// 2. 每级的属性影响
	erows, err := q.Query(ctx, `
		SELECT d.type, e.idx, e.attr_id, e.mode, e.value, e.op_type, e.prob
		  FROM gamedata.ov_exceptdetail d
		  JOIN gamedata.ov_exceptdetail_effect_entry e ON e.row_no = d.row_no
		 WHERE d.type > 0`)
	if err != nil {
		return nil, st, fmt.Errorf("data: 查状态效果: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var typ int64
		var idx, attr, value, opType int32
		var mode, prob int16
		if err := erows.Scan(&typ, &idx, &attr, &mode, &value, &opType, &prob); err != nil {
			return nil, st, fmt.Errorf("data: 读状态效果行: %w", err)
		}
		// idx 是 0 基的等级, 时长表里的 except_level 是 1 基
		key := domain.StatusKey{ID: domain.StatusID(typ), Level: idx + 1}
		d, ok := out[key]
		if !ok {
			st.BadRows++ // 效果挂在一个没有时长的等级上
			continue
		}
		// attr 83/84 是控制操作，不是可叠加数值属性。它们在表中的
		// value 可为 0，所以必须在通用属性白名单前识别。本地全表交叉：
		//   83 = 昏迷/冰冻/石化/定身/冰柩/梦魇/严寒，84 = 封印。
		if opType == 1 && prob == 100 {
			switch attr {
			case 83:
				d.Control = domain.ControlStun
				d.Immobile = 1
				out[key] = d
				continue
			case 84:
				d.Control = domain.ControlSilence
				out[key] = d
				continue
			case domain.StatusAttrCurrentHP:
				if mode == domain.ModeAbsolute || mode == domain.ModePercent {
					d.HPChanges = append(d.HPChanges, domain.ResourceChange{Value: value, Mode: int32(mode)})
					out[key] = d
					continue
				}
			case domain.StatusAttrCurrentMP:
				if mode == domain.ModeAbsolute || mode == domain.ModePercent {
					d.MPChanges = append(d.MPChanges, domain.ResourceChange{Value: value, Mode: int32(mode)})
					out[key] = d
					continue
				}
			}
		}
		// 状态表沿用了另一套“护甲”属性号；在运行时仍落到本作的防御字段。
		if opType == 1 && prob == 100 && attr == 124 &&
			(mode == domain.ModeAbsolute || mode == domain.ModePercent) {
			d.Affixes = append(d.Affixes, domain.Affix{
				Attr: domain.AttrDef, Value: value, Mode: int32(mode),
			})
			out[key] = d
			continue
		}
		if key.ID == 1068 && opType == 1 && prob == 100 &&
			attr == 104 && mode == domain.ModeAbsolute && value < 0 {
			d.OutgoingMagicDamageFlat = value
			d.MagicDamageReductionFlat = -value
			out[key] = d
			continue
		}
		if opType == 1 && prob == 100 && attr == 43 && mode == domain.ModePercent {
			d.ExperienceBonusPct = value
			out[key] = d
			continue
		}
		// 这些不是面板属性，而是一次攻击结算里的操作。此前统一白名单把它们
		// 丢掉，导致状态能显示、实际却没有反射/下一击/吸血效果。
		if mode == domain.ModePercent && prob > 0 {
			handled := true
			switch {
			case key.ID == 1010 && attr == 49 && opType == 2:
				d.PhysicalReflectPct = value
			case key.ID == 1011 && attr == 104 && opType == 2:
				// 魔法反弹 1..5 级的 30/35/40/45/50 与魔镜术逐级
				// “成功几率”完全一致：成功时折回整次伤害，不是每次固定反伤该比例。
				d.MagicFullReflectChancePct = value
			case attr == 97 && opType == 1 && prob == 100:
				d.NextBasicAttackPct = value
			case attr == 98 && opType == 1 && prob == 100:
				d.NextStatusDurationPct = value
			case attr == 99 && opType == 1 && prob == 100:
				d.NextSkillDamagePct = value
			case attr == 100 && opType == 1 && prob == 100:
				if value < 0 {
					d.NextCastTimeReductionPct = -value
				}
			case key.ID == 1030 && attr == 33 && opType == 1:
				d.LifeStealPct, d.LifeStealChance = value, int32(prob)
			case key.ID == 1031 && attr == 35 && opType == 1:
				d.ManaStealPct, d.ManaStealChance = value, int32(prob)
			default:
				handled = false
			}
			if handled {
				out[key] = d
				continue
			}
		}
		if opType != 1 || prob != 100 || (mode != 0 && mode != 1) || !knownAttr(attr) {
			st.BadRows++
			continue
		}
		d.Affixes = append(d.Affixes, domain.Affix{
			Attr: attr, Value: value, Mode: int32(mode)})
		out[key] = d
	}
	if err := erows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历状态效果: %w", err)
	}
	erows.Close()
	for key, d := range out {
		completeKnownStatusSemantics(&d)
		out[key] = d
	}

	// 守护术没有单独的状态表记录，效果只存在于客户端技能描述中：
	// “30秒内伤害抗性提高10%，就职后消失”。伤害抗性同时覆盖物理与魔法，
	// 才不会出现文字说通用抗性、运行时却只减一种伤害的分叉。
	out[domain.StatusKey{ID: domain.StatusBeginnerGuard, Level: 1}] = domain.StatusDef{
		ID: domain.StatusBeginnerGuard, Level: 1, Name: "守护术",
		Desc: "30秒内伤害抗性提高10%", Type: domain.StatusGood, TypeKnown: true,
		DurationSec: 30, PhysicalDamageReductionPct: 10, MagicDamageReductionPct: 10,
	}
	if err := loadMagicTreasureStatuses(ctx, q, out); err != nil {
		return nil, st, err
	}
	if err := loadCustomStatuses(ctx, q, out); err != nil {
		return nil, st, err
	}

	seen := map[domain.StatusID]bool{}
	for k := range out {
		seen[k.ID] = true
	}
	st.Defs, st.Statuses = len(out), len(seen)
	return out, st, nil
}

// loadCustomStatuses 读取技能描述中参数完整、但官方状态表没有等价等级的状态。
// 技能只引用这里的 status_id/level；所有数值与展示文本仍以 PostgreSQL 为准。
func loadCustomStatuses(ctx context.Context, q Querier, out domain.StatusTable) error {
	rows, err := q.Query(ctx, `
		SELECT status_id, status_level, name, description, status_type,
		       duration_sec, interval_sec, hp_change_value, hp_change_mode,
		       physical_damage_reduction_pct, magic_damage_reduction_pct,
		       physical_damage_taken_pct, magic_damage_taken_pct,
		       damage_shield, attack_cap, prevent_invisible
		  FROM game_custom_statuses
		 ORDER BY status_id, status_level`)
	if err != nil {
		return fmt.Errorf("data: 查自定义状态: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			d          domain.StatusDef
			statusType int16
			hpValue    int32
			hpMode     int32
			attackCap  *int32
		)
		if err := rows.Scan(&d.ID, &d.Level, &d.Name, &d.Desc, &statusType,
			&d.DurationSec, &d.IntervalSec, &hpValue, &hpMode,
			&d.PhysicalDamageReductionPct, &d.MagicDamageReductionPct,
			&d.PhysicalDamageTakenPct, &d.MagicDamageTakenPct,
			&d.DamageShield, &attackCap, &d.PreventInvisible); err != nil {
			return fmt.Errorf("data: 读自定义状态: %w", err)
		}
		if statusType < int16(domain.StatusGood) || statusType > int16(domain.StatusGoodNew) {
			return fmt.Errorf("data: 自定义状态 %d/%d 的 status_type=%d 越界", d.ID, d.Level, statusType)
		}
		d.Type, d.TypeKnown = domain.StatusType(statusType), true
		if hpValue != 0 {
			d.HPChanges = []domain.ResourceChange{{Value: hpValue, Mode: hpMode}}
		}
		if attackCap != nil {
			d.AttackCapKnown, d.AttackCap = true, *attackCap
		}
		key := domain.StatusKey{ID: d.ID, Level: d.Level}
		if _, exists := out[key]; exists {
			return fmt.Errorf("data: 自定义状态 %d/%d 与已有状态定义冲突", d.ID, d.Level)
		}
		out[key] = d
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历自定义状态: %w", err)
	}

	affixRows, err := q.Query(ctx, `
		SELECT status_id, status_level, affix_order, attr_id, value, mode
		  FROM game_custom_status_affixes
		 ORDER BY status_id, status_level, affix_order`)
	if err != nil {
		return fmt.Errorf("data: 查自定义状态属性: %w", err)
	}
	defer affixRows.Close()
	for affixRows.Next() {
		var key domain.StatusKey
		var order int32
		var affix domain.Affix
		if err := affixRows.Scan(&key.ID, &key.Level, &order,
			&affix.Attr, &affix.Value, &affix.Mode); err != nil {
			return fmt.Errorf("data: 读自定义状态属性: %w", err)
		}
		d, ok := out[key]
		if !ok {
			return fmt.Errorf("data: 自定义状态属性引用不存在的定义 %d/%d", key.ID, key.Level)
		}
		d.Affixes = append(d.Affixes, affix)
		out[key] = d
	}
	if err := affixRows.Err(); err != nil {
		return fmt.Errorf("data: 遍历自定义状态属性: %w", err)
	}
	return nil
}

// loadMagicTreasureStatuses 从 PostgreSQL 中双向闭合的法宝技能行构造持续效果。
// 这里只实现说明给出完整数值且现有战斗模型能够精确表达的效果；九极、瀚月、
// 灼热之箭和灵犀一指仍保持不可用，不能用猜测参数制造“看起来有效”。
func loadMagicTreasureStatuses(ctx context.Context, q Querier, out domain.StatusTable) error {
	rows, err := q.Query(ctx, `
		SELECT sd.skill_id, sd.name, COALESCE(sd.desc_,''), sd.small_map
		  FROM gamedata.ov_skilldesc sd
		  JOIN gamedata.ov_arm a
		    ON a.index = sd.binding_arm AND a.binding_skill = sd.skill_id
		 WHERE sd.prof = 0 AND sd.skill_level = 1
		 ORDER BY sd.skill_id`)
	if err != nil {
		return fmt.Errorf("data: 查法宝状态技能: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		var name, desc string
		var icon int32
		if err := rows.Scan(&id, &name, &desc, &icon); err != nil {
			return fmt.Errorf("data: 读法宝状态技能: %w", err)
		}
		def, ok := magicTreasureStatusDef(domain.SkillID(id), name, desc, icon)
		if ok {
			out[domain.StatusKey{ID: def.ID, Level: def.Level}] = def
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("data: 遍历法宝状态技能: %w", err)
	}
	return nil
}

func magicTreasureStatusDef(id domain.SkillID, name, desc string, icon int32) (domain.StatusDef, bool) {
	def := domain.StatusDef{
		ID: domain.StatusID(id), Level: 1, Name: name, Desc: desc,
		Icon: icon, SourceSkillID: int32(id), Type: domain.StatusGood, TypeKnown: true,
	}
	attackBuff := func(value, mode int32) {
		def.DurationSec = 10
		def.Affixes = []domain.Affix{
			{Attr: domain.AttrAtk, Value: value, Mode: mode},
			{Attr: domain.AttrMAtk, Value: value, Mode: mode},
		}
	}
	invulnerable := func(seconds int32) {
		def.DurationSec = seconds
		def.Invulnerable = true
		def.CannotAttack = true
	}
	fireReflect := func() {
		def.DurationSec = 30
		def.ClearHarmful = true
		def.PhysicalReflectFlat = 3000
		def.MagicReflectFlat = 3000
		def.ReflectFlatAsMagic = true
	}

	switch id {
	case 14002: // 轮回之力：魔法攻击 +100%，10 秒。
		def.DurationSec = 10
		def.Affixes = []domain.Affix{{Attr: domain.AttrMAtk, Value: 100, Mode: domain.ModePercent}}
	case 14003: // 乾坤之盾：15 秒无敌，同时不能攻击。
		invulnerable(15)
	case 14007, 14016, 14017, 14018, 14019:
		fireReflect()
	case 14008: // 碧落红尘佩：物攻、魔攻 +300，10 秒。
		attackBuff(300, domain.ModeAbsolute)
	case 14009: // 紫金霹雳冠：物攻、魔攻 +1000，10 秒。
		attackBuff(1000, domain.ModeAbsolute)
	case 14010, 14012: // 浩瀚烟波意 / 五岳独尊梭：12 秒无敌且不能攻击。
		invulnerable(12)
	case 14011: // 玉狐芭蕉扇：物攻、魔攻 +100%，10 秒。
		attackBuff(100, domain.ModePercent)
	case 14014: // 紫霞吸血镜：吸收 20000 点伤害，10 秒。
		def.DurationSec = 10
		def.DamageShield = 20000
	case 14015: // 青霞玄冰镜：物理、法术伤害均反射 50%，15 秒。
		def.DurationSec = 15
		def.PhysicalReflectPct = 50
		def.MagicReflectPct = 50
	default:
		return domain.StatusDef{}, false
	}
	return def, true
}

// completeKnownStatusSemantics 补齐客户端表中不是普通面板词条的状态机制。
// 这里只按已逐级对上的状态号处理；不从名字相似性外推到别的状态。
func completeKnownStatusSemantics(d *domain.StatusDef) {
	if d == nil {
		return
	}
	switch d.ID {
	case domain.StatusFreeze:
		d.BreakOnDamage = true
	case domain.StatusDampen:
		mirrorStatusAffix(d, domain.AttrMAtk, domain.AttrMDef)
	case domain.StatusIronSkin:
		if v, ok := removeStatusAffix(d, domain.AttrPhysRes); ok {
			d.PhysicalDamageReductionPct = v
		}
	case domain.StatusBerserk:
		if d.Level >= 1 && d.Level <= 10 {
			ensureStatusAffix(d, domain.AttrDef, -10*d.Level, domain.ModePercent)
			ensureStatusAffix(d, domain.AttrMoveSpeed, 50, domain.ModePercent)
		}
	case 1015: // 增幅：数据残片只保留了魔攻，描述逐级同时给出魔防。
		mirrorStatusAffix(d, domain.AttrMAtk, domain.AttrMDef)
	case 1017: // 心眼：命中与防御使用相同比例。
		mirrorStatusAffix(d, domain.AttrHit, domain.AttrDef)
	case domain.StatusPoisonHit:
		// 淬毒术五级描述：30/35/40/45/50% 命中后施加中毒 21..25 级。
		if d.Level >= 1 && d.Level <= 5 {
			d.PoisonOnHitChance = 25 + d.Level*5
			d.PoisonOnHitLevel = 20 + d.Level
		}
	case domain.StatusDamageShield:
		if d.Level >= 1 && d.Level <= 10 {
			d.DamageShield = 500 + d.Level*200
		} else if d.Level >= 11 && d.Level <= 20 {
			// 11..20 是神圣庇护使用的第二组：500/700/.../2300。
			d.DamageShield = 500 + (d.Level-11)*200
		}
	case domain.StatusSpiritLock:
		d.BlockHarmful = true
	case domain.StatusChaos:
		d.BlockBeneficial = true
	case domain.StatusStealth:
		d.BreakOnAttack = true
		d.BreakOnDamage = true
	case 1055: // 咒法·褪
		d.ClearHarmful = true
	case 1070: // 每 1 点法力转化 3 点生命。
		d.ManaToHealthRate = 3
	case 1074: // 表值为 +15，但字段语义负数才是提速。
		for i := range d.Affixes {
			if d.Affixes[i].Attr == domain.AttrAtkSpeed && d.Affixes[i].Mode == domain.ModePercent {
				d.Affixes[i].Value = -d.Affixes[i].Value
			}
		}
	case 1095:
		d.DamageShield = 1700
	case 1152: // 完全回避：官方状态说明明确为完全回避物理攻击。
		d.PhysicalDamageReductionPct = 100
	case 1034: // 定身的效果残片漏了防御 -100%，说明文字明确保留。
		if strings.Contains(d.Desc, "防御力下降100%") {
			ensureStatusAffix(d, domain.AttrDef, -100, domain.ModePercent)
		}
	case domain.StatusFireShield:
		if v, ok := removeStatusAffix(d, domain.AttrPhysRes); ok {
			d.PhysicalDamageReductionPct = v
		}
		if d.Level >= 1 && d.Level <= 5 {
			d.PhysicalReflectPct = d.Level * 5
		}
	case domain.StatusIceShield:
		if v, ok := removeStatusAffix(d, domain.AttrPhysRes); ok {
			d.PhysicalDamageReductionPct = v
			d.FreezeAttackerChance = v
		}
		// 第 6 级是怪物用的独立定义：吸收全部魔法并反射 50%。
		if d.Level == 6 {
			d.MagicDamageReductionPct = 100
			d.MagicReflectPct = 50
		}
	case 1174:
		d.DamageShield = 4000
	case 1175: // 狮王之力：物攻、魔攻同时 +50%。
		mirrorStatusAffix(d, domain.AttrAtk, domain.AttrMAtk)
	case 1178:
		d.Invulnerable = true
		d.CannotAttack = true
	case 1180:
		d.DamageShield = 15000
	case 1181:
		removeStatusAffixAny(d, domain.AttrMaxHP)
		d.MaxHPFloor = 15000
	case 1208:
		if v, ok := removeStatusAffix(d, domain.AttrPhysRes); ok {
			d.PhysicalDamageReductionPct = v
		}
	case 1209:
		if v, ok := removeStatusAffix(d, domain.AttrMagicRes); ok {
			d.MagicDamageReductionPct = v
		}
	}
}

func mirrorStatusAffix(d *domain.StatusDef, from, to int32) {
	for _, a := range d.Affixes {
		if a.Attr == from {
			ensureStatusAffix(d, to, a.Value, a.Mode)
			return
		}
	}
}

func ensureStatusAffix(d *domain.StatusDef, attr, value, mode int32) {
	for _, a := range d.Affixes {
		if a.Attr == attr {
			return
		}
	}
	d.Affixes = append(d.Affixes, domain.Affix{Attr: attr, Value: value, Mode: mode})
}

func removeStatusAffix(d *domain.StatusDef, attr int32) (int32, bool) {
	for i, a := range d.Affixes {
		if a.Attr != attr || a.Mode != domain.ModePercent {
			continue
		}
		d.Affixes = append(d.Affixes[:i], d.Affixes[i+1:]...)
		return a.Value, true
	}
	return 0, false
}

func removeStatusAffixAny(d *domain.StatusDef, attr int32) (int32, bool) {
	for i, a := range d.Affixes {
		if a.Attr != attr {
			continue
		}
		d.Affixes = append(d.Affixes[:i], d.Affixes[i+1:]...)
		return a.Value, true
	}
	return 0, false
}

// StatusOverlayStats 记录状态叠加矩阵的实际尺寸。
type StatusOverlayStats struct {
	Existing int // 有规则的现有状态（矩阵行）
	Incoming int // 可作为新状态的状态列
	Cells    int // 装入的规则格数
}

// LoadStatusOverlay 读 ov_exceptoverlay 的状态相互作用矩阵。
//
// 原始记录是 type 后紧跟 201 个状态列；导入 PG 时列名保留了客户端英文名，
// 并没有把状态号写进列名。因此按 information_schema 的稳定列序还原：
// 第 3 列是状态 1000，最后一列是状态 1200。矩阵方向是行=已有状态、列=新状态。
func LoadStatusOverlay(ctx context.Context, q Querier) (domain.StatusOverlay, StatusOverlayStats, error) {
	out := domain.StatusOverlay{}
	st := StatusOverlayStats{}
	rows, err := q.Query(ctx, `
		SELECT o.type,
		       (997 + c.ordinal_position)::bigint AS incoming_type,
		       (to_jsonb(o)->>c.column_name)::smallint AS rule
		  FROM gamedata.ov_exceptoverlay o
		 CROSS JOIN information_schema.columns c
		 WHERE c.table_schema = 'gamedata'
		   AND c.table_name = 'ov_exceptoverlay'
		   AND c.ordinal_position >= 3
		   AND o.type > 0
		 ORDER BY o.type, c.ordinal_position`)
	if err != nil {
		return nil, st, fmt.Errorf("data: 查状态叠加矩阵: %w", err)
	}
	defer rows.Close()

	existingSeen := map[domain.StatusID]struct{}{}
	incomingSeen := map[domain.StatusID]struct{}{}
	for rows.Next() {
		var existingRaw, incomingRaw int64
		var ruleRaw int16
		if err := rows.Scan(&existingRaw, &incomingRaw, &ruleRaw); err != nil {
			return nil, st, fmt.Errorf("data: 读状态叠加矩阵行: %w", err)
		}
		if existingRaw < 1000 || existingRaw > 1200 || incomingRaw < 1000 || incomingRaw > 1200 {
			return nil, st, fmt.Errorf("data: 状态叠加矩阵坐标越界: existing=%d incoming=%d", existingRaw, incomingRaw)
		}
		if ruleRaw < int16(domain.OverlayBlock) || ruleRaw > int16(domain.OverlayMustReplace) {
			return nil, st, fmt.Errorf("data: 状态叠加规则越界: existing=%d incoming=%d rule=%d", existingRaw, incomingRaw, ruleRaw)
		}
		existing := domain.StatusID(existingRaw)
		incoming := domain.StatusID(incomingRaw)
		out[domain.OverlayKey{Existing: existing, Incoming: incoming}] = domain.OverlayRule(ruleRaw)
		existingSeen[existing] = struct{}{}
		incomingSeen[incoming] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, st, fmt.Errorf("data: 遍历状态叠加矩阵: %w", err)
	}
	st.Existing = len(existingSeen)
	st.Incoming = len(incomingSeen)
	st.Cells = len(out)
	return out, st, nil
}

// knownAttr 报告这个 attr_id 是不是 domain 认识的那 24 种。
//
// 与装备词条共用同一套字典 —— 状态和装备本来就用同一张属性表,
// 各留一份白名单迟早会分叉。
func knownAttr(a int32) bool {
	switch a {
	case domain.AttrSTR, domain.AttrINT, domain.AttrVIT, domain.AttrAGI,
		domain.AttrDEX, domain.AttrSPI, domain.AttrDef, domain.AttrMAtk,
		domain.AttrMDef, domain.AttrHit, domain.AttrCrit, domain.AttrAtkSpeed,
		domain.AttrMoveSpeed, domain.AttrMinAtk, domain.AttrMaxAtk,
		domain.AttrMaxHP, domain.AttrMaxMP, domain.AttrHPRegen,
		domain.AttrMaxWeight, domain.AttrAtk, domain.AttrMCrit,
		domain.AttrPhysRes, domain.AttrMagicRes, domain.AttrStatusRes:
		return true
	}
	return false
}

// LoadQuests 读任务定义与奖励。
//
// ⚠️ **不读 game_tasks.exp** —— 那一列全表只有 4 个取值(10000/20000/32000/8000),
// 是按等级段填的占位。真正的每任务经验在 game_task_rewards.exp 里,
// 奖励数值根据任务奖励展示文本整理；无法确认的奖励不应猜测填入。
func LoadQuests(ctx context.Context, q Querier) (domain.QuestTable, error) {
	rows, err := q.Query(ctx, `
		SELECT t.id, t.name, t.npc, t.map, t.level_min, t.level_max,
		       ARRAY(SELECT p.prerequisite_task_id
		               FROM game_task_prerequisites p
		              WHERE p.task_id = t.id
		              ORDER BY p.seq),
		       t.goal_item, t.goal_qty, t.repeatable, t.descr, t.reward,
		       COALESCE(r.exp,0), COALESCE(r.honor,0),
		       COALESCE(r.gold,0), COALESCE(r.silver,0), COALESCE(r.copper,0),
		       ARRAY[COALESCE(o.trace_item1_id,0)::int, COALESCE(o.trace_item2_id,0)::int,
		             COALESCE(o.trace_item3_id,0)::int, COALESCE(o.trace_item4_id,0)::int,
		             COALESCE(o.trace_item5_id,0)::int],
		       ARRAY[COALESCE(o.trace_item1_qty,0)::int, COALESCE(o.trace_item2_qty,0)::int,
		             COALESCE(o.trace_item3_qty,0)::int, COALESCE(o.trace_item4_qty,0)::int,
		             COALESCE(o.trace_item5_qty,0)::int],
		       ARRAY[COALESCE(o.trace_monster1_id,0)::int, COALESCE(o.trace_monster2_id,0)::int,
		             COALESCE(o.trace_monster3_id,0)::int, COALESCE(o.trace_monster4_id,0)::int,
		             COALESCE(o.trace_monster5_id,0)::int],
		       ARRAY[COALESCE(o.trace_monster1_qty,0)::int, COALESCE(o.trace_monster2_qty,0)::int,
		             COALESCE(o.trace_monster3_qty,0)::int, COALESCE(o.trace_monster4_qty,0)::int,
		             COALESCE(o.trace_monster5_qty,0)::int]
		  FROM game_tasks t
		  LEFT JOIN game_task_rewards r ON r.task_id = t.id
		  LEFT JOIN gamedata.ov_task o ON o.task_id = t.id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_tasks: %w", err)
	}
	defer rows.Close()

	out := domain.QuestTable{}
	for rows.Next() {
		var d domain.QuestDef
		var goalItem int32
		var prerequisites, itemIDs, itemQty, monsterIDs, monsterQty []int32
		if err := rows.Scan(&d.ID, &d.Name, &d.NPC, &d.Map, &d.LevelMin, &d.LevelMax,
			&prerequisites, &goalItem, &d.GoalQty, &d.Repeatable, &d.Desc, &d.RewardText,
			&d.Reward.Exp, &d.Reward.Honor,
			&d.Reward.Money.Gold, &d.Reward.Money.Silver, &d.Reward.Money.Copper,
			&itemIDs, &itemQty, &monsterIDs, &monsterQty); err != nil {
			return nil, fmt.Errorf("data: 读任务行: %w", err)
		}
		d.Prerequisites = make([]domain.QuestID, len(prerequisites))
		for i, prerequisite := range prerequisites {
			d.Prerequisites[i] = domain.QuestID(prerequisite)
		}
		d.GoalItem = domain.ItemID(goalItem)
		if npc, career := domain.CareerQuestNPC(d.ID); career {
			d.NPC = npc
		}
		step := domain.QuestStep{Kind: domain.QuestStepObjective, To: d.NPC, Text: d.Desc}
		for i, id := range itemIDs {
			if id != 0 {
				step.Collect = append(step.Collect, domain.QuestItemGoal{
					Item: domain.ItemID(id), Qty: itemQty[i],
				})
			}
		}
		for i, id := range monsterIDs {
			if id != 0 {
				step.Kill = append(step.Kill, domain.QuestMonsterGoal{
					Monster: domain.MonsterID(id), Qty: monsterQty[i],
				})
			}
		}
		if len(step.Collect)+len(step.Kill) == 0 {
			// 数据库中的任务分步定义会覆盖这一步；若定义缺失则保持 unknown，绝不把
			// 数据缺失重新降级成“对话发布者即可完成”。
			step.Kind = domain.QuestStepUnknown
		}
		d.Steps = []domain.QuestStep{step}
		out[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历任务: %w", err)
	}

	irows, err := q.Query(ctx,
		`SELECT task_id, item_id, qty FROM game_task_reward_items ORDER BY task_id, seq`)
	if err != nil {
		return nil, fmt.Errorf("data: 查任务奖励物品: %w", err)
	}
	defer irows.Close()
	for irows.Next() {
		var tid, iid, qty int32
		if err := irows.Scan(&tid, &iid, &qty); err != nil {
			return nil, fmt.Errorf("data: 读奖励物品行: %w", err)
		}
		d, ok := out[domain.QuestID(tid)]
		if !ok {
			continue
		}
		d.Reward.Items = append(d.Reward.Items,
			domain.RewardItem{Item: domain.ItemID(iid), Qty: qty})
		out[domain.QuestID(tid)] = d
	}
	if err := irows.Err(); err != nil {
		return nil, err
	}
	irows.Close()
	if err := applyQuestExchanges(ctx, q, out); err != nil {
		return nil, err
	}
	if err := loadQuestProfessionRewards(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// NPCPlacement 是一个 NPC 在地图上的落位。
type NPCPlacement struct {
	MapID    int32
	NPCID    int32
	Name     string
	Sprite   int32
	Pos      domain.Pos
	Dir      int32
	Script   string // 对话脚本号; NPC 落位包要它
	Portrait string // PostgreSQL 配置的已核实立绘资源名
	Sell     int32  // 商店表 id; 0 表示不是商人
	Trans    int32  // 传送表 id; 0 表示没有传送菜单
	// Greeting 来自 game_map_npc_hints 的无条件 Hints，启动时补齐。
	Greeting string
}

// NPCTable 是全服 NPC 落位, 按地图 id 分组。
type NPCTable map[int32][]NPCPlacement

// LoadNPCs 读 map_npcs, 按地图 id 分组。
// 未指定立绘时复用同模型唯一的已配置立绘；有冲突的模型不自动选择。
//
// 与刷怪点一样要过 map_defs 把文件名翻成地图 id;
// 同一个地图文件对应多个地图 id 时, 每个 id 各拿一份(副本分线各自要有 NPC)。
func LoadNPCs(ctx context.Context, q Querier) (NPCTable, int, error) {
	rows, err := q.Query(ctx, `
		WITH model_portraits AS (
			SELECT sprite, MIN(portrait) AS portrait
			  FROM map_npcs
			 WHERE sprite > 0 AND portrait <> ''
			 GROUP BY sprite
			HAVING COUNT(DISTINCT portrait) = 1
		)
		SELECT d.id, n.npc_id, n.name, n.sprite, n.x, n.y, n.dir, n.script,
		       n.sell_list, n.trans_list,
		       COALESCE(NULLIF(n.portrait, ''), p.portrait, '')
		  FROM map_npcs n
		  LEFT JOIN map_defs d ON d.file = n.map_file
		  LEFT JOIN model_portraits p ON p.sprite = n.sprite
		 WHERE n.hide = false`)
	if err != nil {
		return nil, 0, fmt.Errorf("data: 查 map_npcs: %w", err)
	}
	defer rows.Close()

	out := NPCTable{}
	skipped := 0
	for rows.Next() {
		var mapID *int32
		var p NPCPlacement
		var x, y int32
		if err := rows.Scan(&mapID, &p.NPCID, &p.Name, &p.Sprite, &x, &y, &p.Dir, &p.Script,
			&p.Sell, &p.Trans, &p.Portrait); err != nil {
			return nil, skipped, fmt.Errorf("data: 读 NPC 行: %w", err)
		}
		if mapID == nil {
			skipped++ // 地图文件在 map_defs 里没有 id, 那张图建不出场景
			continue
		}
		p.MapID = *mapID
		p.Pos = domain.Pos{MapID: *mapID, X: float64(x), Y: float64(y)}
		out[*mapID] = append(out[*mapID], p)
	}
	return out, skipped, rows.Err()
}

// LoadTransports 读取 NPC TransList 对应的飞空艇目的地。
// destination_index 与正式客户端 TransportUI.sel 同为零基且必须连续；任何缺口
// 都会让客户端展示顺序和确认选择错位，因此整张表拒绝加载而不是跳过坏行。
func LoadTransports(ctx context.Context, q Querier, dungeons domain.DungeonTable) (domain.TransportTable, int, error) {
	rows, err := q.Query(ctx, `
		SELECT trans_list, destination_index, source_map_id,
		       name, map_id, x, y, price, is_dungeon, require_clear
		  FROM game_transport_destinations
		 ORDER BY trans_list, destination_index`)
	if err != nil {
		return nil, 0, fmt.Errorf("data: 查 game_transport_destinations: %w", err)
	}
	defer rows.Close()

	out := domain.TransportTable{}
	count := 0
	for rows.Next() {
		var listID, index, sourceMapID, mapID, x, y, price int32
		var name string
		var dungeon, requireClear bool
		if err := rows.Scan(&listID, &index, &sourceMapID, &name, &mapID, &x, &y, &price,
			&dungeon, &requireClear); err != nil {
			return nil, count, fmt.Errorf("data: 读传送目的地行: %w", err)
		}
		if listID <= 0 || sourceMapID <= 0 || mapID <= 0 || name == "" || x < 0 || y < 0 {
			return nil, count, fmt.Errorf("data: 传送列表 %d 索引 %d 配置无效", listID, index)
		}
		// 货币三面额的线上进率尚未闭合。当前飞空艇航线在数据库中明确为免费；
		// 非零价格不能只展示不扣款，也不能按猜测进率扣款，因此启动时拒绝。
		if price != 0 {
			return nil, count, fmt.Errorf("data: 传送列表 %d 索引 %d 的非零价格尚不支持", listID, index)
		}
		_, definedDungeon := dungeons[mapID]
		if dungeon != definedDungeon {
			return nil, count, fmt.Errorf("data: 传送列表 %d 的地图 %d 副本标记与定义不一致", listID, mapID)
		}
		if dungeon {
			def, exists := dungeons[mapID]
			if !exists {
				return nil, count, fmt.Errorf("data: 副本入口列表 %d 引用未定义地图 %d", listID, mapID)
			}
			if int32(def.Enter.X) != x || int32(def.Enter.Y) != y {
				return nil, count, fmt.Errorf("data: 副本入口列表 %d 的落点 (%d,%d) 与副本定义 (%d,%d) 不一致",
					listID, x, y, int32(def.Enter.X), int32(def.Enter.Y))
			}
		}
		if requireClear && !dungeon {
			return nil, count, fmt.Errorf("data: 传送列表 %d 只有副本目标才能要求清场", listID)
		}
		list, exists := out[listID]
		if !exists {
			list = domain.TransportList{ID: listID, SourceMapID: sourceMapID}
		} else if list.SourceMapID != sourceMapID {
			return nil, count, fmt.Errorf("data: 传送列表 %d 混入多个起点地图", listID)
		}
		if index != int32(len(list.Destinations)) {
			return nil, count, fmt.Errorf("data: 传送列表 %d 索引不连续: 得 %d, 期望 %d",
				listID, index, len(list.Destinations))
		}
		list.Destinations = append(list.Destinations, domain.TransportDestination{
			Index: index, Name: name,
			Pos: domain.Pos{MapID: mapID, X: float64(x), Y: float64(y)}, Price: price,
			Dungeon: dungeon, RequireClear: requireClear,
		})
		out[listID] = list
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, count, fmt.Errorf("data: 遍历传送目的地: %w", err)
	}
	return out, count, nil
}

// LoadWork 读打工定义与耐力规则。
//
// unit_sec 已经是 ÷5 之后的值(migration 0022 落库时就换算好了);
// unit_sec_v 留着原值只为对照, 服务端不读它。
func LoadWork(ctx context.Context, q Querier) (domain.WorkTable, domain.StaminaRule, error) {
	rule := domain.DefaultStaminaRule()
	rows, err := q.Query(ctx, `
		SELECT work_id, title, work_type, skill_id, pet_skill_id, pet_skill_add, lv_min, lv_max, unit_sec, exp, honor, coin, coin_prob,
		       roll_mode, practise_min, practise_max
		  FROM game_work WHERE unit_sec > 0`)
	if err != nil {
		return nil, rule, fmt.Errorf("data: 查 game_work: %w", err)
	}
	defer rows.Close()

	out := domain.WorkTable{}
	for rows.Next() {
		var d domain.WorkDef
		var exp, honor, coin, rollMode int32
		if err := rows.Scan(&d.ID, &d.Title, &d.WorkType, &d.RequiredSkill, &d.PetSkill, &d.PetSkillAddPct,
			&d.LevelMin, &d.LevelMax, &d.UnitSec,
			&exp, &honor, &coin, &d.CoinProb,
			&rollMode, &d.PractiseMin, &d.PractiseMax); err != nil {
			return nil, rule, fmt.Errorf("data: 读 game_work 行: %w", err)
		}
		d.Exp, d.Honor, d.Coin = int64(exp), int64(honor), int64(coin)
		d.WeightedPick = rollMode == 1
		out[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, rule, fmt.Errorf("data: 遍历 game_work: %w", err)
	}

	// 物品产出。**按 seq 排序不是为了好看** —— 权重单选是按顺序逐段扣的,
	// 顺序变了同一个随机数就会落到另一条鱼上, 掉落分布不再可复现。
	irows, err := q.Query(ctx, `
		SELECT work_id, item_id, qty, prob, practise_gain_max
		  FROM game_work_items ORDER BY work_id, seq`)
	if err != nil {
		return nil, rule, fmt.Errorf("data: 查 game_work_items: %w", err)
	}
	defer irows.Close()
	for irows.Next() {
		var id domain.WorkID
		var it domain.WorkItem
		if err := irows.Scan(&id, &it.Item, &it.Qty, &it.Prob, &it.PractiseGainMax); err != nil {
			return nil, rule, fmt.Errorf("data: 读 game_work_items 行: %w", err)
		}
		d, ok := out[id]
		if !ok {
			continue // 工种被 unit_sec>0 滤掉了, 它的产出也就无处可去
		}
		d.Items = append(d.Items, it)
		out[id] = d
	}
	if err := irows.Err(); err != nil {
		return nil, rule, fmt.Errorf("data: 遍历 game_work_items: %w", err)
	}

	// 生活工作装备是按 work_type 共用的可替代集合。不能只认普通锄头/鱼竿，
	// 也不能靠名字包含“矿镐”临时猜；全部可用档位由 PostgreSQL 明确列出。
	requirements := map[int32]domain.WorkRequirements{}
	erows, err := q.Query(ctx, `
		SELECT work_type, requirement_kind, equip_slot, item_id
		  FROM game_work_equipment_requirements
		 ORDER BY work_type, requirement_kind, item_id`)
	if err != nil {
		return nil, rule, fmt.Errorf("data: 查打工装备要求: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var workType, slot int32
		var kindName string
		var item domain.ItemID
		if err := erows.Scan(&workType, &kindName, &slot, &item); err != nil {
			return nil, rule, fmt.Errorf("data: 读打工装备要求: %w", err)
		}
		var kind domain.WorkRequirementKind
		switch kindName {
		case "outfit":
			kind = domain.WorkRequirementOutfit
		case "tool":
			kind = domain.WorkRequirementTool
		default:
			return nil, rule, fmt.Errorf("data: 工种 %d 的装备要求类型 %q 无效", workType, kindName)
		}
		r := requirements[workType]
		found := false
		for i := range r.Equipment {
			if r.Equipment[i].Kind == kind {
				if r.Equipment[i].Slot != domain.EquipSlot(slot) {
					return nil, rule, fmt.Errorf("data: 工种 %d 的 %q 要求混入多个槽位", workType, kindName)
				}
				r.Equipment[i].Items = append(r.Equipment[i].Items, item)
				found = true
				break
			}
		}
		if !found {
			r.Equipment = append(r.Equipment, domain.WorkEquipmentRequirement{
				Kind: kind, Slot: domain.EquipSlot(slot), Items: []domain.ItemID{item},
			})
		}
		requirements[workType] = r
	}
	if err := erows.Err(); err != nil {
		return nil, rule, fmt.Errorf("data: 遍历打工装备要求: %w", err)
	}

	crows, err := q.Query(ctx, `
		SELECT work_type, item_id, qty_per_yield
		  FROM game_work_consumable_requirements ORDER BY work_type`)
	if err != nil {
		return nil, rule, fmt.Errorf("data: 查打工消耗品要求: %w", err)
	}
	defer crows.Close()
	for crows.Next() {
		var workType int32
		var item domain.ItemID
		var qty int32
		if err := crows.Scan(&workType, &item, &qty); err != nil {
			return nil, rule, fmt.Errorf("data: 读打工消耗品要求: %w", err)
		}
		r := requirements[workType]
		r.Consumable, r.ConsumablePerYield = item, qty
		requirements[workType] = r
	}
	if err := crows.Err(); err != nil {
		return nil, rule, fmt.Errorf("data: 遍历打工消耗品要求: %w", err)
	}
	for id, d := range out {
		d.Requirements = requirements[d.WorkType]
		if d.WorkType == 10 || d.WorkType == 11 {
			_, hasOutfit := d.Requirements.EquipmentOf(domain.WorkRequirementOutfit)
			_, hasTool := d.Requirements.EquipmentOf(domain.WorkRequirementTool)
			if !hasOutfit || !hasTool || (d.WorkType == 11 && d.Requirements.ConsumablePerYield <= 0) {
				return nil, rule, fmt.Errorf("data: 生活工种 %d(work_id=%d) 的装备或消耗品规则不完整", d.WorkType, id)
			}
		}
		out[id] = d
	}

	rrows, err := q.Query(ctx,
		`SELECT max_points, refill_daily, cost_per_min FROM game_stamina_rule WHERE id = 1`)
	if err != nil {
		return out, rule, fmt.Errorf("data: 查 game_stamina_rule: %w", err)
	}
	defer rrows.Close()
	if rrows.Next() {
		if err := rrows.Scan(&rule.Max, &rule.RefillDaily, &rule.CostPerMin); err != nil {
			return out, rule, fmt.Errorf("data: 读耐力规则: %w", err)
		}
	}
	return out, rule, rrows.Err()
}

// LoadRestRule 读取坐下休息的单行恢复规则。
func LoadRestRule(ctx context.Context, q Querier) (domain.RestRule, error) {
	var rule domain.RestRule
	rows, err := q.Query(ctx, `
		SELECT interval_ms, heal_max_hp_pct, heal_flat
		  FROM game_rest_rule WHERE id = 1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_rest_rule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return rule, fmt.Errorf("data: 读 game_rest_rule: %w", err)
		}
		return rule, fmt.Errorf("data: game_rest_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.IntervalMS, &rule.HealMaxHPPct, &rule.HealFlat); err != nil {
		return domain.RestRule{}, fmt.Errorf("data: 读 game_rest_rule 行: %w", err)
	}
	if !rule.Enabled() {
		return domain.RestRule{}, fmt.Errorf("data: game_rest_rule 配置无效")
	}
	return rule, rows.Err()
}

// LoadCombatTimingRule 读取命中时序、远程弹道速度与前摇受击打断概率。
func LoadCombatTimingRule(ctx context.Context, q Querier) (domain.CombatTimingRule, error) {
	var rule domain.CombatTimingRule
	rows, err := q.Query(ctx, `
		SELECT melee_hit_delay_ms, ranged_basic_speed_px_per_sec,
		       cast_interrupt_base_bp, cast_interrupt_min_bp
		  FROM game_combat_timing_rule WHERE id = 1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_combat_timing_rule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return rule, fmt.Errorf("data: 读 game_combat_timing_rule: %w", err)
		}
		return rule, fmt.Errorf("data: game_combat_timing_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.MeleeHitDelayMS, &rule.RangedBasicSpeedPXPerSec,
		&rule.CastInterruptBaseBP, &rule.CastInterruptMinBP); err != nil {
		return domain.CombatTimingRule{}, fmt.Errorf("data: 读 game_combat_timing_rule 行: %w", err)
	}
	if !rule.Valid() || !rule.ValidCastInterrupt() {
		return domain.CombatTimingRule{}, fmt.Errorf("data: game_combat_timing_rule 配置无效")
	}
	return rule, rows.Err()
}

func LoadMailRule(ctx context.Context, q Querier) (domain.MailRule, error) {
	var rule domain.MailRule
	rows, err := q.Query(ctx, `SELECT capacity,expire_days,max_attach FROM game_mail_rule WHERE id=1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_mail_rule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return rule, fmt.Errorf("data: game_mail_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.Capacity, &rule.ExpireDays, &rule.MaxAttach); err != nil {
		return domain.MailRule{}, fmt.Errorf("data: 读 game_mail_rule: %w", err)
	}
	if !rule.Valid() {
		return domain.MailRule{}, fmt.Errorf("data: game_mail_rule 配置无效")
	}
	return rule, rows.Err()
}

func LoadChatChannelRules(ctx context.Context, q Querier) (domain.ChatChannelRules, error) {
	rows, err := q.Query(ctx, `SELECT channel,scope,cooldown_ms,max_shares FROM game_chat_channel_rules ORDER BY channel`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_chat_channel_rules: %w", err)
	}
	defer rows.Close()
	out := domain.ChatChannelRules{}
	for rows.Next() {
		var raw int16
		var scope string
		var rule domain.ChatChannelRule
		if err := rows.Scan(&raw, &scope, &rule.CooldownMS, &rule.MaxShares); err != nil {
			return nil, err
		}
		rule.Channel = uint8(raw)
		switch scope {
		case "map":
			rule.Scope = domain.ChatScopeMap
		case "party":
			rule.Scope = domain.ChatScopeParty
		case "family":
			rule.Scope = domain.ChatScopeFamily
		case "world":
			rule.Scope = domain.ChatScopeWorld
		default:
			return nil, fmt.Errorf("data: 聊天频道 %d 范围非法 %q", raw, scope)
		}
		out[rule.Channel] = rule
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: 聊天频道规则为空")
	}
	return out, nil
}

func LoadWardrobeRule(ctx context.Context, q Querier) (domain.WardrobeRule, error) {
	var rule domain.WardrobeRule
	rows, err := q.Query(ctx, `SELECT key_item_id,open_key_cost,open_capacity,max_capacity FROM game_wardrobe_rule WHERE id=1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_wardrobe_rule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return rule, fmt.Errorf("data: game_wardrobe_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.KeyItem, &rule.OpenKeyCost, &rule.OpenCapacity, &rule.MaxCapacity); err != nil {
		return domain.WardrobeRule{}, fmt.Errorf("data: 读 game_wardrobe_rule: %w", err)
	}
	if rule.KeyItem == 0 || rule.OpenKeyCost <= 0 || rule.OpenCapacity <= 0 ||
		rule.MaxCapacity < rule.OpenCapacity || rule.MaxCapacity > 1<<16-1 {
		return domain.WardrobeRule{}, fmt.Errorf("data: game_wardrobe_rule 基础配置无效")
	}
	rule.UnlockCosts = make(map[int32]int32, rule.MaxCapacity-rule.OpenCapacity)
	costRows, err := q.Query(ctx, `
		SELECT wardrobe_entry_no,cat
		  FROM gamedata.ov_magicwardrobe
		 WHERE wardrobe_entry_no>$1 AND wardrobe_entry_no<=$2
		 ORDER BY wardrobe_entry_no`, rule.OpenCapacity, rule.MaxCapacity)
	if err != nil {
		return domain.WardrobeRule{}, fmt.Errorf("data: 查魔法衣橱逐格费用: %w", err)
	}
	defer costRows.Close()
	for costRows.Next() {
		var capacity, cost int32
		if err := costRows.Scan(&capacity, &cost); err != nil {
			return domain.WardrobeRule{}, fmt.Errorf("data: 读魔法衣橱逐格费用: %w", err)
		}
		rule.UnlockCosts[capacity] = cost
	}
	if err := costRows.Err(); err != nil {
		return domain.WardrobeRule{}, err
	}
	if !rule.Valid() {
		return domain.WardrobeRule{}, fmt.Errorf("data: game_wardrobe_rule 配置无效")
	}
	return rule, rows.Err()
}

// LoadDeathRule 读取普通场景死亡损失的单行规则。
func LoadDeathRule(ctx context.Context, q Querier) (domain.DeathRule, error) {
	var rule domain.DeathRule
	rows, err := q.Query(ctx, `
		SELECT min_level, exp_loss_bp, money_loss_bp
		  FROM game_death_rule WHERE id = 1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_death_rule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return rule, fmt.Errorf("data: 读 game_death_rule: %w", err)
		}
		return rule, fmt.Errorf("data: game_death_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.MinLevel, &rule.ExpLossBP, &rule.MoneyLossBP); err != nil {
		return domain.DeathRule{}, fmt.Errorf("data: 读 game_death_rule 行: %w", err)
	}
	if !rule.Enabled() {
		return domain.DeathRule{}, fmt.Errorf("data: game_death_rule 配置无效")
	}
	return rule, rows.Err()
}

// LoadLevelPenalty 读跨级打怪的等级差惩罚单行规则（game_level_penalty）。
//
// 该表只有一行（id=1）；缺配置按错误处理，不允许静默关掉惩罚。
func LoadLevelPenalty(ctx context.Context, q Querier) (domain.LevelPenalty, error) {
	var rule domain.LevelPenalty
	rows, err := q.Query(ctx, `
		SELECT exp_full_above, exp_step_bp, exp_below_start,
		       drop_full, drop_step_bp, drop_floor_bp
		  FROM game_level_penalty WHERE id = 1`)
	if err != nil {
		return rule, fmt.Errorf("data: 查 game_level_penalty: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return rule, fmt.Errorf("data: 读 game_level_penalty: %w", err)
		}
		return rule, fmt.Errorf("data: game_level_penalty 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.ExpFullAbove, &rule.ExpStepBP, &rule.ExpBelowStart,
		&rule.DropFull, &rule.DropStepBP, &rule.DropFloorBP); err != nil {
		return domain.LevelPenalty{}, fmt.Errorf("data: 读 game_level_penalty 行: %w", err)
	}
	if !rule.Enabled() {
		return domain.LevelPenalty{}, fmt.Errorf("data: game_level_penalty 配置无效")
	}
	return rule, rows.Err()
}

// LoadPets 读宠物定义。
//
// **直读 `gamedata.ov_petgrow`**，不经 `game_pets` —— 后者只抄了 5 列，
// 六维成长那 12 列不在里面。技能与状态也是这么读的，不是特例。
//
// 两个已实测的坑，都在这里挡掉：
//
//	pet_id 不唯一   10944 有两行(黄金凤凰 / 凤舞傲九天), 六维与生命完全一致,
//	                是同一只坐骑的两个皮肤名。按 row_no 取先出现的那行, 结果稳定。
//	                (`game_pets` 只有 503 行就是被这一行并掉的。)
//	生命成长列全零   hp_min/hp_max/vit_hp_add/sp_min/sp_max/int_sp_add 504 行全零,
//	                别去读它们。生命怎么长见 domain.PetDef.MaxHPAt。
func LoadPets(ctx context.Context, q Querier) (domain.PetTable, error) {
	rows, err := q.Query(ctx, `
		SELECT pet_id, name, g.eats1_id, COALESCE(h.habit, 0),
		       COALESCE((SELECT m.sprite FROM game_monsters m WHERE m.id = g.pet_id), 0),
		       capture_level, capture_tool,
		       base_capture_succ, max_capture_succ,
		       init_str, init_vit, init_int, init_spi, init_agi, init_dex,
		       init_hp, init_sp,
		       str_odd, vit_odd, int_odd, spi_odd, agi_odd, dex_odd,
		       str_even, vit_even, int_even, spi_even, agi_even, dex_even,
		       max_pet_level, init_trust, init_starve,
		       horse_base_spd, horse_spd_add, horse_spd_limit,
		       starve_add1, starve_internal1, starve_add2, starve_internal2,
		       starve_add3, starve_internal3, starve_add4, starve_internal4,
		       online_add, online_internal, pickup_trust,
		       init_skills1_id, init_skills1_qty, init_skills2_id, init_skills2_qty,
		       init_skills3_id, init_skills3_qty, init_skills4_id, init_skills4_qty,
		       init_skills5_id, init_skills5_qty, init_skills6_id, init_skills6_qty
		  FROM gamedata.ov_petgrow g
		  LEFT JOIN game_pet_food_habits h ON h.food_item_id = g.eats1_id
		 WHERE pet_id > 0
		 ORDER BY row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 ov_petgrow: %w", err)
	}
	defer rows.Close()

	out := domain.PetTable{}
	for rows.Next() {
		var d domain.PetDef
		var raw [4]domain.StarveTier
		var online domain.StarveTier
		var pickup int32
		var foodID, habit int32
		var initial [6]domain.PetSkill
		if err := rows.Scan(&d.ID, &d.Name, &foodID, &habit, &d.Model, &d.CaptureLevel, &d.CaptureTool,
			&d.BaseCaptureRate, &d.MaxCaptureRate,
			&d.Init.STR, &d.Init.VIT, &d.Init.INT, &d.Init.SPI, &d.Init.AGI, &d.Init.DEX,
			&d.InitHP, &d.InitSP,
			&d.GrowthOdd.STR, &d.GrowthOdd.VIT, &d.GrowthOdd.INT,
			&d.GrowthOdd.SPI, &d.GrowthOdd.AGI, &d.GrowthOdd.DEX,
			&d.GrowthEven.STR, &d.GrowthEven.VIT, &d.GrowthEven.INT,
			&d.GrowthEven.SPI, &d.GrowthEven.AGI, &d.GrowthEven.DEX,
			&d.MaxLevel, &d.InitTrust, &d.InitStarve,
			&d.HorseBaseSpeed, &d.HorseSpeedAdd, &d.HorseSpeedLimit,
			&raw[0].Delta, &raw[0].Interval, &raw[1].Delta, &raw[1].Interval,
			&raw[2].Delta, &raw[2].Interval, &raw[3].Delta, &raw[3].Interval,
			&online.Delta, &online.Interval, &pickup,
			&initial[0].ID, &initial[0].Level, &initial[1].ID, &initial[1].Level,
			&initial[2].ID, &initial[2].Level, &initial[3].ID, &initial[3].Level,
			&initial[4].ID, &initial[4].Level, &initial[5].ID, &initial[5].Level); err != nil {
			return nil, fmt.Errorf("data: 读 ov_petgrow 行: %w", err)
		}
		if foodID != 0 && habit == 0 {
			return nil, fmt.Errorf("data: 宠物 %d 的食物 %d 未配置食性", d.ID, foodID)
		}
		d.Habit = uint8(habit)
		// **原表用 u8 存负数**: 255 就是 −1。不还原的话"每 3 分钟饿 255 点",
		// 一帧就能把饥渴度顶到上限
		for i := range raw {
			d.Starve.Tiers[i] = domain.StarveTier{
				Delta: signedByte(raw[i].Delta), Interval: raw[i].Interval}
		}
		d.Starve.Online = domain.StarveTier{
			Delta: signedByte(online.Delta), Interval: online.Interval}
		d.CanPickUp = pickup != 0
		for _, skill := range initial {
			if skill.ID == 0 {
				continue
			}
			if skill.Level <= 0 {
				skill.Level = 1
			}
			d.InitialSkills = append(d.InitialSkills, skill)
		}
		// 四档全零的那 28 行不是宠物, 是坐骑与幻化书
		d.RealPet = d.Starve.Tiers[0].Delta != 0
		// 先出现的那行赢 —— ORDER BY row_no 保证这件事是确定的
		if _, dup := out[d.ID]; !dup {
			out[d.ID] = d
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 ov_petgrow: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: ov_petgrow 是空的")
	}
	rows.Close()

	// pets.json 的模型、天生技能和“不学技能”已经落进以下 PostgreSQL
	// 配置。ov_petskillno 对海龟等种族与该文件正面冲突，不能再把它当学习池。
	profileRows, err := q.Query(ctx, `
		SELECT pet_id, model_id, blessing_slots
		  FROM game_pet_profiles ORDER BY pet_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物种族技能配置: %w", err)
	}
	profiled := 0
	for profileRows.Next() {
		var id domain.PetID
		var model, blessing int32
		if err := profileRows.Scan(&id, &model, &blessing); err != nil {
			profileRows.Close()
			return nil, fmt.Errorf("data: 读宠物种族技能配置: %w", err)
		}
		d, ok := out[id]
		if !ok {
			profileRows.Close()
			return nil, fmt.Errorf("data: pets.json 宠物 %d 在 ov_petgrow 中不存在", id)
		}
		if model > 0 {
			d.Model = model
		}
		d.BlessingSlots = blessing
		// 有 profile 就由 pets.json 全量覆盖，尤其要允许“天生技能为空”。
		d.InitialSkills = nil
		d.Learnable = nil
		out[id] = d
		profiled++
	}
	if err := profileRows.Err(); err != nil {
		profileRows.Close()
		return nil, fmt.Errorf("data: 遍历宠物种族技能配置: %w", err)
	}
	profileRows.Close()
	if profiled == 0 {
		return nil, fmt.Errorf("data: game_pet_profiles 是空的")
	}

	initialRows, err := q.Query(ctx, `
		SELECT pet_id, skill_id FROM game_pet_initial_skills ORDER BY pet_id, skill_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物天生技能: %w", err)
	}
	for initialRows.Next() {
		var id domain.PetID
		var skill domain.SkillID
		if err := initialRows.Scan(&id, &skill); err != nil {
			initialRows.Close()
			return nil, fmt.Errorf("data: 读宠物天生技能: %w", err)
		}
		d, ok := out[id]
		if !ok {
			initialRows.Close()
			return nil, fmt.Errorf("data: 宠物 %d 的天生技能没有种族定义", id)
		}
		d.InitialSkills = append(d.InitialSkills, domain.PetSkill{ID: skill, Level: 1})
		out[id] = d
	}
	if err := initialRows.Err(); err != nil {
		initialRows.Close()
		return nil, fmt.Errorf("data: 遍历宠物天生技能: %w", err)
	}
	initialRows.Close()

	learnRows, err := q.Query(ctx, `
		SELECT pet_id, skill_id FROM game_pet_learnable_skills ORDER BY pet_id, skill_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物可领悟技能: %w", err)
	}
	defer learnRows.Close()
	for learnRows.Next() {
		var id domain.PetID
		var skill domain.SkillID
		if err := learnRows.Scan(&id, &skill); err != nil {
			return nil, fmt.Errorf("data: 读宠物可领悟技能: %w", err)
		}
		d, ok := out[id]
		if !ok {
			return nil, fmt.Errorf("data: 宠物 %d 的可领悟技能没有种族定义", id)
		}
		d.Learnable = append(d.Learnable, skill)
		out[id] = d
	}
	if err := learnRows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历宠物可领悟技能: %w", err)
	}
	return out, nil
}

// LoadPetPrefixes 读取官方五种孵化前缀与每级成长修正。
func LoadPetPrefixes(ctx context.Context, q Querier) (domain.PetPrefixTable, error) {
	rows, err := q.Query(ctx, `
		SELECT nick_id, nick_name, get_rate, status_point,
		       str_odd, vit_odd, int_odd, spi_odd, agi_odd, dex_odd,
		       str_even, vit_even, int_even, spi_even, agi_even, dex_even
		  FROM gamedata.ov_petnickgrow ORDER BY row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 ov_petnickgrow: %w", err)
	}
	defer rows.Close()
	var out domain.PetPrefixTable
	for rows.Next() {
		var d domain.PetPrefixDef
		if err := rows.Scan(&d.ID, &d.Name, &d.GetRate, &d.FreePointsPerLevel,
			&d.GrowthOdd.STR, &d.GrowthOdd.VIT, &d.GrowthOdd.INT, &d.GrowthOdd.SPI,
			&d.GrowthOdd.AGI, &d.GrowthOdd.DEX,
			&d.GrowthEven.STR, &d.GrowthEven.VIT, &d.GrowthEven.INT, &d.GrowthEven.SPI,
			&d.GrowthEven.AGI, &d.GrowthEven.DEX); err != nil {
			return nil, fmt.Errorf("data: 读 ov_petnickgrow 行: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 ov_petnickgrow: %w", err)
	}
	return out, nil
}

// LoadPetSkills 只加载宠物技能的展示信息和最高等级；效果本期不执行。
func LoadPetSkills(ctx context.Context, q Querier) (domain.PetSkillTable, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (d.skill_id) d.skill_id, d.name, COALESCE(d.desc_,''),
		       d.small_map, d.is_combat, COALESCE(p.active, false),
		       p.skill_id IS NOT NULL, d.level_need, d.max_level_pet
		  FROM gamedata.ov_skilldesc d
		  LEFT JOIN game_pet_skill_profiles p ON p.skill_id = d.skill_id
		 WHERE d.skill_id > 0 AND d.skill_level = 1 AND d.max_level_pet > 0
		 ORDER BY d.skill_id, d.row_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物技能: %w", err)
	}
	defer rows.Close()
	out := domain.PetSkillTable{}
	for rows.Next() {
		var d domain.PetSkillDef
		var fight int32
		var configured bool
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Icon, &fight,
			&d.Active, &configured,
			&d.LearnLevel, &d.MaxLevel); err != nil {
			return nil, fmt.Errorf("data: 读宠物技能行: %w", err)
		}
		if !configured {
			return nil, fmt.Errorf("data: 宠物技能 %d(%s) 缺少 game_pet_skill_profiles 主动/被动配置", d.ID, d.Name)
		}
		d.Fight = fight != 0
		out[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历宠物技能: %w", err)
	}
	rows.Close()
	if err := loadPetSkillEffects(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadPetRule 读取用户确认的单行宠物规则。
func LoadPetRule(ctx context.Context, q Querier) (domain.PetRule, error) {
	rows, err := q.Query(ctx, `
		SELECT natural_learn_chance_bp, initial_skill_min, initial_skill_max,
		       low_trust_refuse_bp, zero_trust_recall_sec, hunger_interval_sec, trust_happy_sec,
		       trust_content_sec, deploy_hunger_add, death_trust_loss,
		       trade_trust_required, trade_trust_loss, max_life_skills, max_fight_skills
		  FROM game_pet_rule WHERE id = 1`)
	if err != nil {
		return domain.PetRule{}, fmt.Errorf("data: 查 game_pet_rule: %w", err)
	}
	defer rows.Close()
	var rule domain.PetRule
	if !rows.Next() {
		return rule, fmt.Errorf("data: game_pet_rule 缺少 id=1 配置")
	}
	if err := rows.Scan(&rule.NaturalLearnChanceBP, &rule.InitialSkillMin, &rule.InitialSkillMax,
		&rule.LowTrustRefuseBP, &rule.ZeroTrustRecallSec, &rule.HungerIntervalSec, &rule.TrustHappySec,
		&rule.TrustContentSec, &rule.DeployHungerAdd, &rule.DeathTrustLoss,
		&rule.TradeTrustRequired, &rule.TradeTrustLoss, &rule.MaxLifeSkills, &rule.MaxFightSkills); err != nil {
		return domain.PetRule{}, fmt.Errorf("data: 读 game_pet_rule: %w", err)
	}
	if !rule.Valid() || rule.MaxLifeSkills <= 0 || rule.MaxFightSkills <= 0 {
		return domain.PetRule{}, fmt.Errorf("data: game_pet_rule 配置无效")
	}
	return rule, rows.Err()
}

// LoadPetLearningItems 读取下一次升级领悟药丸。
func LoadPetLearningItems(ctx context.Context, q Querier,
	skills domain.PetSkillTable) (map[domain.ItemID]domain.PetLearningItem, error) {
	rows, err := q.Query(ctx, `
		SELECT r.item_id, i.name, r.skill_id, r.chance_bp
		  FROM game_pet_learning_items r
		  JOIN game_items i ON i.id = r.item_id
		 ORDER BY r.item_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物领悟道具: %w", err)
	}
	defer rows.Close()
	out := map[domain.ItemID]domain.PetLearningItem{}
	for rows.Next() {
		var item domain.PetLearningItem
		if err := rows.Scan(&item.Item, &item.Name, &item.Skill, &item.ChanceBP); err != nil {
			return nil, fmt.Errorf("data: 读宠物领悟道具行: %w", err)
		}
		if _, ok := skills[item.Skill]; !ok || item.ChanceBP <= 0 || item.ChanceBP > 10000 {
			return nil, fmt.Errorf("data: 宠物领悟道具 %d 配置非法", item.Item)
		}
		out[item.Item] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历宠物领悟道具: %w", err)
	}
	return out, nil
}

// LoadPetLevels 读宠物经验曲线。
//
// ⚠️ **只读 exp_need，绝不碰 exp_total** —— 后者从 59 级起全是 2147483647
// (int32 饱和)。拿它当累计值会在 59 级上凭空立一道墙。
func LoadPetLevels(ctx context.Context, q Querier, cap int32) (*domain.PetLevelTable, error) {
	rows, err := q.Query(ctx, `
		SELECT level, exp_need FROM gamedata.ov_petlevelexp
		 WHERE level >= 1 AND level <= $1 ORDER BY level`, cap)
	if err != nil {
		return nil, fmt.Errorf("data: 查 ov_petlevelexp: %w", err)
	}
	defer rows.Close()

	need := map[int32]int64{}
	for rows.Next() {
		var lv int32
		var exp int64
		if err := rows.Scan(&lv, &exp); err != nil {
			return nil, fmt.Errorf("data: 读 ov_petlevelexp 行: %w", err)
		}
		need[lv] = exp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 ov_petlevelexp: %w", err)
	}
	if len(need) == 0 {
		return nil, fmt.Errorf("data: ov_petlevelexp 是空的")
	}
	return domain.NewPetLevelTable(need), nil
}

// signedByte 把"用 u8 存的有符号数"还原回去。
//
// `ov_petgrow.starve_add*` 里 255 表示 −1 —— 原始 .bin 那一列是 1 字节，
// 解析成无符号之后负数就翻到了高位。doccheck 有一条断言盯着这个哨兵值。
// 不还原的结果很好认: 饥渴度一跳就满。
func signedByte(v int32) int32 {
	if v >= 128 && v <= 255 {
		return v - 256
	}
	return v
}

// LoadPetFoods 读宠物食物。
//
// 数量与适用区间**不在 game_items 里**，只在描述文本里 ——
// 而 `ov_petgrow.eats*_qty` 恰好是同一组数（3/2/1/5，482 行逐个吻合），
// 所以数量取表、区间取文本约定，两边互证。
//
// 区间是固定的三档（食物描述里写死的）：0~50 / 51~75 / 76~100。
func LoadPetFoods(ctx context.Context, q Querier) (map[domain.ItemID]domain.PetFood, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (v.item) v.item, v.qty, i.name
		  FROM (
		    SELECT eats1_id AS item, eats1_qty AS qty, 1 AS tier FROM gamedata.ov_petgrow WHERE eats1_id>0
		    UNION ALL SELECT eats2_id, eats2_qty, 2 FROM gamedata.ov_petgrow WHERE eats2_id>0
		    UNION ALL SELECT eats3_id, eats3_qty, 3 FROM gamedata.ov_petgrow WHERE eats3_id>0
		    UNION ALL SELECT eats4_id, eats4_qty, 4 FROM gamedata.ov_petgrow WHERE eats4_id>0
		  ) v
		  JOIN game_items i ON i.id = v.item
		 ORDER BY v.item, v.tier`)
	if err != nil {
		return nil, fmt.Errorf("data: 查宠物食物: %w", err)
	}
	defer rows.Close()

	out := map[domain.ItemID]domain.PetFood{}
	for rows.Next() {
		var f domain.PetFood
		if err := rows.Scan(&f.Item, &f.Reduce, &f.Name); err != nil {
			return nil, fmt.Errorf("data: 读宠物食物行: %w", err)
		}
		// 档位由**降低量**反推, 而不是由 id 段猜:
		// 降 3 = 低饥渴档、降 2 = 中档、降 1 = 高档、降 5 = 无尽淳(任何时候)
		switch f.Reduce {
		case 3:
			f.Min, f.Max = 0, 50
		case 2:
			f.Min, f.Max = 51, 75
		case 1:
			f.Min, f.Max = 76, 100
		case 5:
			f.Anytime, f.Trust = true, 2 // 无尽淳: 顺带 +2 信赖
		default:
			continue // 认不出来的不放进表, 好过按错的区间发下去
		}
		out[f.Item] = f
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历宠物食物: %w", err)
	}
	return out, nil
}
