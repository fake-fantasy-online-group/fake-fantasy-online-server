package scene

import (
	"container/heap"
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 怪物行为的**执行**部分。决策在 game/ai（纯函数），这里只负责:
// 找目标（要 AOI）、真的挪一步、真的把攻击排进本帧队列、真的广播。
//
// 位置在帧序的第 2 步, **必须排在战斗之前** —— 反过来的话怪永远慢一帧才还手:
// 这一帧决定出手, 下一帧才结算, 攻击间隔凭空多出 100ms。

const (
	// aiScanEvery 每隔多少帧找一次目标。
	//
	// 不每帧扫是因为一张图最多 402 个刷怪点, 每帧每只怪都遍历九宫格是纯浪费 ——
	// 玩家 0.5 秒最多走 80 个单位, 索敌半径 300, 慢半拍完全感觉不到。
	// 用实体 id 错开, 免得 400 只怪挤在同一帧一起扫。
	aiScanEvery = 5

	// reviveGraceTicks 复活保护时长。
	//
	// **原地复活必须配保护, 否则就是死亡循环**: 玩家在打死他的怪旁边站起来,
	// 复活那一帧就被重新盯上。实测过: 帧 71 复活、帧 71 挨打、帧 91 再死。
	// 换成回城之后这个值还有用 —— 复活保护本身是常规机制。
	reviveGraceTicks = 30 // 3 秒

	// 怪物没有战斗目标时不会钉死，而是从当前位置开始走走停停。
	// 每一段路保持较短；位置不会重置，所以被拉到传送门的主动怪会留在那里活动。
	roamRadius = 120.0
	// 每到一个游荡点停 2~5 秒；出生后的第一次移动也按这个范围错峰。
	roamPauseMinTicks domain.Tick = 20
	roamPauseJitter               = 31

	// 闲置游荡不必每个服务端步长都同步；追击绕障则必须逐步同步，避免客户端
	// 将相邻路径拐点之间的稀疏快照插值成一条穿墙直线。
	monsterMoveBroadcastEvery domain.Tick = 5

	// 随机点可能落在狭窄岸边；多试几次，仍找不到完整可走的直线路径就原地等待。
	roamPointAttempts = 32
	roamPathSample    = 5.0

	// 怪物只在自己附近做 MASK 寻路。200 明显小于 400 的索敌范围和 600 的
	// 追击半径：能看见远处玩家，但隔着稍大的障碍就不肯继续绕，更不会全图寻路。
	monsterPathRadius = 200.0
	// 20px 网格兼顾 10px MASK 精度与在线怪物数量；每条边仍按 5px 采样，
	// 因而斜走也不能从碰撞格的角上穿过去。
	monsterPathGrid        = 20.0
	monsterPathMaxExpanded = 1024
	// 玩家只挪一点时继续沿当前拐点走；移动超过 40px 才重新规划。
	monsterPathTargetSlack             = 40.0
	monsterPathRetryTicks  domain.Tick = 20
)

type monsterPathCell struct{ x, y int }

type monsterPathQueueItem struct {
	cell monsterPathCell
	g, f float64
	seq  int
}

type monsterPathQueue []monsterPathQueueItem

func (q monsterPathQueue) Len() int { return len(q) }
func (q monsterPathQueue) Less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	return q[i].seq < q[j].seq
}
func (q monsterPathQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *monsterPathQueue) Push(v any)   { *q = append(*q, v.(monsterPathQueueItem)) }
func (q *monsterPathQueue) Pop() any {
	old := *q
	n := len(old) - 1
	v := old[n]
	*q = old[:n]
	return v
}

// stepAI 推进本图所有怪的行为。
func (s *Scene) stepAI() {
	if len(s.monsters) == 0 {
		return
	}
	for _, m := range s.monsters {
		if !m.Alive() || m.Monster == nil {
			continue
		}
		if m.Monster.ExpireAt > 0 && s.tick >= m.Monster.ExpireAt {
			s.despawnSummonedMonster(m)
			continue
		}
		if (uint64(s.tick)+uint64(m.ID))%uint64(aiScanEvery) == 0 {
			s.ensureMonsterPassives(m)
		}
		if !canAct(m) {
			continue // 昏迷/冰冻/石化的怪站着不动
		}
		if m.Monster.CastingSkill != 0 && s.tick < m.Monster.CastingUntil {
			continue
		}
		p := aiParams(m)
		if m.Monster.NoAttack && m.Monster.Target != 0 {
			s.dropTarget(m)
		}

		// 没目标就找一个。不主动的怪跳过 —— 它们只在挨打时被动上仇恨。
		if m.Monster.Target == 0 && p.Aggressive && s.shouldScan(m.ID) {
			s.acquire(m, p)
		}

		tgt := s.targetInfo(m)
		chaseTo, directPath := domain.Pos{}, true
		if tgt != nil {
			var pathOK bool
			chaseTo, directPath, pathOK = s.monsterChaseWaypoint(m, tgt.Pos, p.TraceDist)
			if !pathOK {
				s.dropTarget(m)
				continue
			}
		}
		if tgt == nil && m.Monster.Target == 0 && s.stepWander(m, p) {
			continue
		}
		// 怪物技能和普攻使用独立准入队列。只要选技节拍、技能 GCD 和单技能
		// 冷却允许，就先尝试技能；不能再等普攻 nextAt 转好才开始选技。
		// 隔墙时仍先寻路，避免独立选技重新引入穿墙施法。
		if tgt != nil && directPath && s.tryMonsterSkill(m, tgt) {
			continue
		}
		p.AttackDist = s.monsterEngageRange(m)
		action := ai.Decide(ai.Input{
			Params: p, Self: m.Pos, Home: m.Monster.Home,
			Target: tgt, CanAttack: m.ReadyToAttack(s.tick),
		})
		// 隔着墙即使几何距离已经进入攻击圈，也必须先沿局部路径绕到同侧；
		// 远程技能同样不能穿过 MASK 直接起手。
		if tgt != nil && !directPath && action == ai.Strike {
			action = ai.Chase
		}
		switch action {
		case ai.Strike:
			// 普通野怪中的真 caster 只使用技能。Boss 与精英无论模板是否法系
			// 都继续走下面的普攻；只有自 Buff 或没有远程技的误分类普通怪也保底普攻。
			if m.Monster.NoBasicAttack || s.monsterUsesSkillsOnly(m) {
				continue
			}
			// “不能攻击”状态允许移动和释放非伤害技能，但不能从技能路径落回
			// 普攻绕过。封印只影响施法，不会走到这里。
			if !canDealDamage(m) {
				continue
			}
			basicDist := float64(m.Monster.AI.BasicAttackDist)
			if basicDist <= 0 {
				basicDist = ai.MeleeRange
			}
			// 进入普攻距离后必须完成本轮普攻。
			if tgt != nil && sqDist(m.Pos, tgt.Pos) <= basicDist*basicDist {
				s.monsterSpeak(m, domain.MonsterSceneAttack)
				s.attacks = append(s.attacks, pendingAttack{src: m.ID, dst: m.Monster.Target})
				continue
			}
			// 只有远程技能的决策圈把怪物留在了普攻距离外；本轮技能
			// 未通过概率时原地等待，不伪造超远普攻。技能进入冷却后会重新追到普攻距离。
		case ai.Chase:
			to := ai.StepToward(m.Pos, chaseTo, p.MoveSpeed)
			if to != m.Pos && s.straightPathWalkable(m.Pos, to) {
				s.moveEntity(m, to)
			}
		case ai.DropTarget:
			s.dropTarget(m)
		}
	}
}

// initializeMonster 初始化一次“出生”的技能冷却与被动状态。重生和召唤都走
// 这里，因此 initial_delay 永远相对本次出生，而不是相对场景创建时间。
func (s *Scene) initializeMonster(m *entity.Entity) {
	if m == nil || m.Monster == nil {
		return
	}
	m.Monster.SkillReady = map[domain.SkillID]domain.Tick{}
	m.Monster.SkillCasts = map[domain.SkillID]int32{}
	m.Monster.NextSkillAt = 0
	m.Monster.SpokenScenes = 0
	s.clearMonsterChasePath(m.Monster)
	m.Monster.ChasePathRetryAt = 0
	m.Monster.ChasePathRetryTarget = domain.Pos{}
	if s.defs == nil {
		return
	}
	def, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return
	}
	if m.Monster.AI.ID == "" {
		m.Monster.AI = def.AI
	}
	m.Monster.NextSkillCheck = s.tick + def.AI.SkillCheckEvery
	for _, rule := range def.Skills {
		m.Monster.SkillReady[rule.Skill.ID] = s.tick + rule.InitialDelay
	}
	s.ensureMonsterPassives(m)
}

// ensureMonsterPassives 把数据库声明的被动状态维持在怪物身上。状态表仍保留
// 自己的时长，便于客户端展示；到期或被驱散后，AI 节拍会重新挂回，而不是把
// “被动”错误地做成出生后只持续十几秒。
func (s *Scene) ensureMonsterPassives(m *entity.Entity) {
	if m == nil || m.Monster == nil || s.defs == nil || s.statuses == nil {
		return
	}
	def, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return
	}
	for _, rule := range def.Skills {
		if !rule.Enabled || rule.Skill.Kind.Active() {
			continue
		}
		for _, app := range rule.Skill.Statuses {
			if m.Status != nil && m.Status.Has(app.ID) {
				continue
			}
			statusDef, found := s.statuses.Get(app.ID, app.Level)
			if !found {
				continue
			}
			if app.DurationSec > 0 {
				statusDef.DurationSec = app.DurationSec
			}
			applyStatusSource(&statusDef, rule.Skill.Icon, int32(rule.Skill.ID))
			s.applyStatusDef(m, statusDef, m.ID)
		}
	}
}

// monsterEngageRange 返回这一帧怪物愿意停下尝试出手的最远距离。
// 只有已转好的对敌技能会扩大这个圆；技能冷却期间会继续追到普攻距离，
// 否则 KeepDist 远程模板会永远卡在“够不着普攻、也不再靠近”的区间。
func (s *Scene) monsterEngageRange(m *entity.Entity) float64 {
	if m == nil || m.Monster == nil {
		return ai.MeleeRange
	}
	base := float64(m.Monster.AI.BasicAttackDist)
	if base <= 0 {
		base = ai.MeleeRange
	}
	if s.defs == nil {
		return base
	}
	def, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return base
	}
	hpPct := monsterHPPct(m)
	skillsOnly := s.monsterUsesSkillsOnly(m)
	for _, rule := range def.Skills {
		if !rule.Usable() || !s.monsterRuleTargetsEnemies(rule) ||
			!skillsOnly && !monsterRuleInHPRange(rule, hpPct) {
			continue
		}
		if !skillsOnly && s.tick < m.Monster.SkillReady[rule.Skill.ID] {
			continue
		}
		d := monsterSkillRange(rule)
		if d > base {
			base = d
		}
	}
	return base
}

func monsterSkillRange(rule domain.MonsterSkillRule) float64 {
	d := float64(rule.Skill.Dist)
	if rule.Self && rule.Skill.Kind == domain.SkillArea && float64(rule.Skill.Radius) > d {
		d = float64(rule.Skill.Radius)
	}
	return d
}

// monsterUsesSkillsOnly 只识别普通野怪中的真远程 caster。标签本身不够：人虎、
// 骑兵蚁等会因为自用“术”被数据归为 caster，仙符等则只有远程控制技能。
// 只有具备远程对敌伤害技能的普通怪才禁用普攻，其余仍靠普攻输出。
// Boss 与精英始终返回 false，保证技能间隙存在普攻。
func (s *Scene) monsterUsesSkillsOnly(m *entity.Entity) bool {
	if m == nil || m.Monster == nil || m.Monster.Kind != domain.MonsterNormal ||
		m.Monster.AI.ID != "caster" || s.defs == nil {
		return false
	}
	def, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return false
	}
	basicDist := float64(m.Monster.AI.BasicAttackDist)
	if basicDist <= 0 {
		basicDist = ai.MeleeRange
	}
	for _, rule := range def.Skills {
		if rule.Usable() && rule.Skill.HasDamageEffect() &&
			s.monsterRuleTargetsEnemies(rule) && !monsterSkillHeals(rule) &&
			monsterSkillRange(rule) > basicDist {
			return true
		}
	}
	return false
}

// tryMonsterSkill 按规则优先级、血线和独立冷却选择技能。仅靠技能攻击的普通
// caster 每帧检查伤害技能，不受选技节拍和概率限制；其余技能仍按模板选技。
// 返回 true 表示已经起手；普通攻击必须让路。
func (s *Scene) tryMonsterSkill(m *entity.Entity, target *ai.Target) bool {
	if m == nil || m.Monster == nil || s.defs == nil ||
		s.tick < m.Monster.NextSkillAt || !canCast(m) {
		return false
	}
	skillsOnly := s.monsterUsesSkillsOnly(m)
	checkDue := s.tick >= m.Monster.NextSkillCheck
	if !checkDue && !skillsOnly {
		return false
	}
	if checkDue {
		checkEvery := m.Monster.AI.SkillCheckEvery
		if checkEvery == 0 {
			checkEvery = domain.Ticks(1000)
		}
		m.Monster.NextSkillCheck = s.tick + checkEvery
	}
	def, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return false
	}
	hpPct := monsterHPPct(m)
	for _, rule := range def.Skills {
		damageOnReady := skillsOnly && rule.Skill.HasDamageEffect() &&
			s.monsterRuleTargetsEnemies(rule) && !monsterSkillHeals(rule)
		if !checkDue && !damageOnReady {
			continue
		}
		if !rule.Usable() || !monsterRuleInHPRange(rule, hpPct) ||
			s.tick < m.Monster.SkillReady[rule.Skill.ID] ||
			(rule.MaxCastsPerLife > 0 && m.Monster.SkillCasts[rule.Skill.ID] >= rule.MaxCastsPerLife) {
			continue
		}
		if rule.Skill.Effect.Kind == domain.EffectDamage && !canDealDamage(m) {
			continue
		}
		targets := s.resolveMonsterTargets(m, rule, target)
		if len(targets) == 0 {
			continue
		}
		// 自身增益避免无意义刷新；对敌减益仍可按各怪物的冷却与概率再次施放。
		if !s.monsterRuleTargetsEnemies(rule) && rule.Skill.Effect.Kind == domain.EffectNone && len(rule.Extra) == 0 &&
			s.monsterTargetsAlreadyHaveStatuses(targets, rule.Skill.Statuses) {
			continue
		}
		if !damageOnReady && rule.ChanceBP < 10000 && int32(s.aiRng.Intn(10000)) >= rule.ChanceBP {
			continue
		}
		s.startMonsterSkill(m, rule, targets)
		return true
	}
	return false
}

func monsterHPPct(m *entity.Entity) int32 {
	if m == nil || m.MaxHP <= 0 {
		return 0
	}
	return m.HP * 100 / m.MaxHP
}

func monsterRuleInHPRange(rule domain.MonsterSkillRule, hpPct int32) bool {
	return hpPct >= rule.MinHPPct && hpPct <= rule.MaxHPPct
}

func entityAlreadyHasStatuses(e *entity.Entity, apps []domain.StatusApplication) bool {
	if len(apps) == 0 || e == nil || e.Status == nil {
		return false
	}
	for _, app := range apps {
		if !e.Status.Has(app.ID) {
			return false
		}
	}
	return true
}

func (s *Scene) monsterTargetsAlreadyHaveStatuses(targets []domain.EntityID, apps []domain.StatusApplication) bool {
	if len(targets) == 0 || len(apps) == 0 {
		return false
	}
	for _, targetID := range targets {
		if !entityAlreadyHasStatuses(s.entities[targetID], apps) {
			return false
		}
	}
	return true
}

func (s *Scene) resolveMonsterTargets(caster *entity.Entity, rule domain.MonsterSkillRule, target *ai.Target) []domain.EntityID {
	if caster == nil || caster.Monster == nil {
		return nil
	}
	if monsterSkillHeals(rule) {
		return s.resolveMonsterHealTargets(caster, rule)
	}
	if !s.monsterRuleTargetsEnemies(rule) {
		return []domain.EntityID{caster.ID}
	}
	targetID := caster.Monster.Target
	t, ok := s.entities[targetID]
	if target == nil || !ok || !s.validMonsterTarget(t) {
		return nil
	}
	castRange := float64(rule.Skill.Dist)
	if castRange <= 0 {
		castRange = float64(caster.Monster.AI.BasicAttackDist)
	}
	if castRange <= 0 {
		castRange = ai.MeleeRange
	}
	center := t.Pos
	if rule.Self && rule.Skill.Kind == domain.SkillArea {
		center = caster.Pos
		if float64(rule.Skill.Radius) > castRange {
			castRange = float64(rule.Skill.Radius)
		}
	}
	if sqDist(caster.Pos, t.Pos) > castRange*castRange {
		return nil
	}
	if rule.Skill.Kind == domain.SkillSingle || rule.Skill.Radius <= 0 {
		return []domain.EntityID{targetID}
	}
	radiusSq := float64(rule.Skill.Radius) * float64(rule.Skill.Radius)
	out := make([]domain.EntityID, 0, 4)
	s.aoi.AroundPos(center, func(candidate *entity.Entity) {
		if !s.validMonsterTarget(candidate) || sqDist(center, candidate.Pos) > radiusSq {
			return
		}
		out = append(out, candidate.ID)
	})
	return sortEntityIDs(out)
}

func monsterSkillHeals(rule domain.MonsterSkillRule) bool {
	for _, effect := range rule.Skill.DirectEffects() {
		if effect.Kind == domain.EffectHeal {
			return true
		}
	}
	return false
}

// resolveMonsterHealTargets 让治疗怪选择真实需要治疗的怪物。单体治疗选择施法
// 距离内血量比例最低者；群疗覆盖自身周围的受伤友军。怪物阵营当前统一，场景
// 物件和采集物不参与治疗，避免治疗技能浪费在机关与采集点上。
func (s *Scene) resolveMonsterHealTargets(caster *entity.Entity, rule domain.MonsterSkillRule) []domain.EntityID {
	castRange := float64(rule.Skill.Dist)
	if castRange <= 0 {
		castRange = float64(caster.Monster.AI.BasicAttackDist)
	}
	if castRange <= 0 {
		castRange = ai.MeleeRange
	}
	radius := float64(rule.Skill.Radius)
	area := rule.Skill.Kind == domain.SkillArea || rule.Skill.TargetTeam || radius > 0
	if area && radius > 0 {
		castRange = radius
	}
	rangeSq := castRange * castRange
	candidates := make([]*entity.Entity, 0, 4)
	s.aoi.AroundPos(caster.Pos, func(candidate *entity.Entity) {
		if candidate == nil || candidate.Monster == nil || !candidate.Alive() ||
			!candidate.Monster.Kind.Hostile() || candidate.HP >= candidate.MaxHP ||
			sqDist(caster.Pos, candidate.Pos) > rangeSq {
			return
		}
		candidates = append(candidates, candidate)
	})
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := int64(candidates[i].HP) * int64(candidates[j].MaxHP)
		right := int64(candidates[j].HP) * int64(candidates[i].MaxHP)
		if left != right {
			return left < right
		}
		leftDist, rightDist := sqDist(caster.Pos, candidates[i].Pos), sqDist(caster.Pos, candidates[j].Pos)
		if leftDist != rightDist {
			return leftDist < rightDist
		}
		return candidates[i].ID < candidates[j].ID
	})
	if !area {
		return []domain.EntityID{candidates[0].ID}
	}
	out := make([]domain.EntityID, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.ID)
	}
	return sortEntityIDs(out)
}

// monsterRuleTargetsEnemies 用已闭合的结果裁决作用阵营。kind='self' 表示技能
// 从怪物自身起手/定中心，不代表暴雪、战争恐惧这类自身中心范围技要打自己。
func (s *Scene) monsterRuleTargetsEnemies(rule domain.MonsterSkillRule) bool {
	if monsterSkillHeals(rule) {
		return false
	}
	// 客户端有少量“自己+队伍”增益被原始怪物表归在 enemy。只要技能没有
	// 伤害、明确能作用自身，且挂接状态全部为增益，就应落在施法怪自身。
	if !rule.Skill.HasDamageEffect() && rule.Skill.TargetSelf && len(rule.Skill.Statuses) > 0 {
		harmful := false
		for _, app := range rule.Skill.Statuses {
			if s.statuses != nil {
				if def, ok := s.statuses.Get(app.ID, app.Level); ok && def.Harmful() {
					harmful = true
					break
				}
			}
		}
		if !harmful {
			return false
		}
	}
	if !rule.Self || rule.Skill.Effect.Kind == domain.EffectDamage {
		return true
	}
	for _, extra := range rule.Extra {
		if extra.Kind == domain.MonsterExtraDispelBeneficial {
			return true
		}
	}
	for _, app := range rule.Skill.Statuses {
		if s.statuses != nil {
			if def, ok := s.statuses.Get(app.ID, app.Level); ok && def.Harmful() {
				return true
			}
		}
	}
	if len(rule.Skill.Statuses) > 0 && rule.Skill.TargetEnemy && !rule.Skill.TargetSelf {
		return true
	}
	return false
}

func (s *Scene) validMonsterTarget(target *entity.Entity) bool {
	// 隐身不等于伤害免疫。怪物不会把隐身玩家作为仇恨目标，但已经起手的
	// 技能，或以另一名玩家为中心结算的范围效果，仍能波及并命中他。
	if target == nil || !target.Alive() || s.entityMapLoading(target) {
		return false
	}
	if target.Kind == domain.KindPlayer {
		return target.Player == nil || !target.Player.Protected(s.tick)
	}
	return false
}

func (s *Scene) startMonsterSkill(m *entity.Entity, rule domain.MonsterSkillRule, targets []domain.EntityID) {
	if s.monsterRuleTargetsEnemies(rule) {
		s.monsterSpeak(m, domain.MonsterSceneAttack)
	}
	def := rule.Skill
	statusDurationPct := s.prepareSkillStatuses(m, &def)
	cooldown := rule.Cooldown
	if fromSkill := domain.Ticks(int(def.CooldownMS)); fromSkill > cooldown {
		cooldown = fromSkill
	}
	if cooldown == 0 {
		cooldown = m.Monster.AI.GlobalCooldown
	}
	m.Monster.SkillReady[def.ID] = s.tick + cooldown
	gcd := m.Monster.AI.GlobalCooldown
	if gcd == 0 {
		gcd = domain.Ticks(500)
	}
	m.Monster.NextSkillAt = s.tick + gcd
	m.Monster.Roaming = false
	s.log.Debug("怪物施法", "怪物", m.Name, "实体", m.ID, "技能", def.Name,
		"技能号", def.ID, "目标", targets[0], "吟唱毫秒", def.PrepareMS,
		"冷却毫秒", cooldown.Millis())

	cast := pendingCast{
		src: m.ID, def: def, targets: targets, statusDurationPct: statusDurationPct,
		cooldownMS:   int32(cooldown.Millis()),
		visualTarget: targets[0],
		extras:       append([]domain.MonsterSkillExtra(nil), rule.Extra...),
	}
	s.setMonsterGroundVisual(m, rule, &cast)
	if def.PrepareMS <= 0 {
		m.Monster.SkillCasts[def.ID]++
		s.casts = append(s.casts, cast)
		return
	}

	m.Monster.CastSerial++
	serial := m.Monster.CastSerial
	m.Monster.CastingSkill = def.ID
	m.Monster.CastingUntil = s.tick + domain.Ticks(int(def.PrepareMS))
	s.emit(event.SkillCastChanged{
		Caster: m.ID, Skill: def.ID, Phase: event.SkillCastChant,
		ChantHoldMS: def.PrepareMS, CooldownMS: int32(cooldown.Millis()), Target: cast.visualTarget, Aim: cast.visualAim,
	})
	s.timer.after(s.tick, domain.Ticks(int(def.PrepareMS)), func() {
		caster, ok := s.monsters[m.ID]
		if !ok || !caster.Alive() || caster.Monster == nil || caster.Monster.CastSerial != serial {
			return
		}
		caster.Monster.CastingSkill = 0
		caster.Monster.CastingUntil = 0
		if !canAct(caster) || !canCast(caster) {
			return
		}
		freshTargets := s.resolveMonsterTargets(caster, rule, s.targetInfo(caster))
		if len(freshTargets) == 0 {
			return
		}
		// timer 在 stepAI 之前推进。释放完成后重新起算独立技能 GCD，避免同帧
		// 再选中另一项技能。普攻队列只挡这一帧，下一帧即可按自身攻速继续。
		gcd := caster.Monster.AI.GlobalCooldown
		if gcd == 0 {
			gcd = domain.Ticks(500)
		}
		caster.Monster.NextSkillAt = s.tick + gcd
		caster.DelayAttackUntil(s.tick + 1)
		caster.Monster.SkillCasts[def.ID]++
		cast.targets = freshTargets
		cast.visualTarget = freshTargets[0]
		s.setMonsterGroundVisual(caster, rule, &cast)
		s.casts = append(s.casts, cast)
	})
}

func (s *Scene) setMonsterGroundVisual(caster *entity.Entity, rule domain.MonsterSkillRule, cast *pendingCast) {
	if !cast.def.GroundEffect {
		return
	}
	target := cast.visualTarget
	if rule.Skill.Kind == domain.SkillArea && (rule.Self || monsterSkillHeals(rule)) {
		target = caster.ID
	} else if s.monsterRuleTargetsEnemies(rule) {
		// 范围受击列表按ID排序，不一定以当前仇恨目标开头。
		target = caster.Monster.Target
	} else if !monsterSkillHeals(rule) {
		target = caster.ID
	}
	cast.visualTarget, cast.visualAim = s.skillVisualTarget(caster, cast.def, UseSkill{Target: target}, cast.targets)
}

// stepWander 推进一只无目标怪物的闲逛状态。
//
// 返回 true 表示这一帧已经由游荡逻辑处理（包括停下来等下一次出发）。
func (s *Scene) stepWander(m *entity.Entity, p ai.Params) bool {
	d := m.Monster
	if d == nil || p.MoveSpeed <= 0 || !d.Kind.Hostile() {
		return false // 花草、箱子和天生移速为 0 的怪保持静止
	}

	if !d.Roaming {
		if d.NextRoamAt == 0 {
			s.scheduleNextRoam(d)
			return true
		}
		if s.tick < d.NextRoamAt {
			return true
		}
		to, ok := s.randomRoamPoint(m.Pos)
		if !ok {
			s.scheduleNextRoam(d)
			return true
		}
		d.RoamTo = to
		d.Roaming = true
		// 客户端拿目标点和 Speed 自己平滑播放整段路。游走开始时发一次即可，
		// 不能把服务端每个 100ms 插值点都向全图广播。
		s.emitEntityMove(m, d.RoamTo)
	}

	to := ai.StepToward(m.Pos, d.RoamTo, p.MoveSpeed)
	s.moveEntity(m, to)
	if to == d.RoamTo {
		d.Roaming = false
		s.scheduleNextRoam(d)
	}
	return true
}

func (s *Scene) scheduleNextRoam(m *entity.Monster) {
	m.NextRoamAt = s.tick + roamPauseMinTicks + domain.Tick(s.aiRng.Intn(roamPauseJitter))
}

func (s *Scene) randomRoamPoint(from domain.Pos) (domain.Pos, bool) {
	for attempt := 0; attempt < roamPointAttempts; attempt++ {
		// sqrt 让点在圆内按面积均匀分布，不会全挤在中心。
		r := roamRadius * math.Sqrt(s.aiRng.Float64())
		a := 2 * math.Pi * s.aiRng.Float64()
		to := domain.Pos{MapID: from.MapID, X: from.X + r*math.Cos(a), Y: from.Y + r*math.Sin(a)}
		if s.straightPathWalkable(from, to) {
			return to, true
		}
	}
	return from, false
}

// straightPathWalkable 验证怪物正在尝试的直线路段。游荡只用一段；追击先走
// 直线，遇到障碍后由局部寻路拆成若干同样经过这里验证的拐点段。
func (s *Scene) straightPathWalkable(from, to domain.Pos) bool {
	if s.walkable == nil {
		return true // 无客户端资源的纯内存测试保持原行为
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	steps := int(math.Ceil(math.Hypot(dx, dy) / roamPathSample))
	if steps < 1 {
		steps = 1
	}
	for i := 1; i <= steps; i++ {
		p := domain.Pos{
			MapID: from.MapID,
			X:     from.X + dx*float64(i)/float64(steps),
			Y:     from.Y + dy*float64(i)/float64(steps),
		}
		if !s.walkable(p) {
			return false
		}
	}
	return true
}

// monsterChaseWaypoint 返回本帧该朝哪个点走。无遮挡时永远直追目标；有 MASK
// 时复用局部 A* 的拐点。第三个返回值为 false 表示限定半径内无路，应当脱战。
func (s *Scene) monsterChaseWaypoint(m *entity.Entity, target domain.Pos, traceDist float64) (domain.Pos, bool, bool) {
	if m == nil || m.Monster == nil {
		return domain.Pos{}, false, false
	}
	if s.straightPathWalkable(m.Pos, target) {
		s.clearMonsterChasePath(m.Monster)
		m.Monster.ChasePathRetryAt = 0
		return target, true, true
	}

	radius := monsterPathRadius
	if traceDist > 0 && traceDist < radius {
		radius = traceDist
	}
	if radius <= 0 || math.Hypot(target.X-m.Pos.X, target.Y-m.Pos.Y) > radius {
		s.clearMonsterChasePath(m.Monster)
		return domain.Pos{}, false, false
	}

	d := m.Monster
	if s.tick < d.ChasePathRetryAt &&
		math.Hypot(target.X-d.ChasePathRetryTarget.X, target.Y-d.ChasePathRetryTarget.Y) <= monsterPathTargetSlack {
		return domain.Pos{}, false, false
	}
	if d.ChasePathNext < len(d.ChasePath) &&
		math.Hypot(target.X-d.ChasePathTarget.X, target.Y-d.ChasePathTarget.Y) <= monsterPathTargetSlack {
		for d.ChasePathNext < len(d.ChasePath) && m.Pos == d.ChasePath[d.ChasePathNext] {
			d.ChasePathNext++
		}
		if d.ChasePathNext < len(d.ChasePath) && s.straightPathWalkable(m.Pos, d.ChasePath[d.ChasePathNext]) {
			return d.ChasePath[d.ChasePathNext], false, true
		}
	}

	path, ok := s.localMonsterPath(m.Pos, target, radius)
	if !ok || len(path) == 0 {
		s.clearMonsterChasePath(d)
		d.ChasePathRetryAt = s.tick + monsterPathRetryTicks
		d.ChasePathRetryTarget = target
		return domain.Pos{}, false, false
	}
	d.ChasePath = path
	d.ChasePathNext = 0
	d.ChasePathTarget = target
	d.ChasePathRetryAt = 0
	return d.ChasePath[0], false, true
}

func (s *Scene) clearMonsterChasePath(m *entity.Monster) {
	if m == nil {
		return
	}
	m.ChasePath = nil
	m.ChasePathNext = 0
	m.ChasePathTarget = domain.Pos{}
}

var monsterPathNeighbours = [...]monsterPathCell{
	{x: 1}, {x: -1}, {y: 1}, {y: -1},
	{x: 1, y: 1}, {x: 1, y: -1}, {x: -1, y: 1}, {x: -1, y: -1},
}

// localMonsterPath 在以 from 为圆心的固定半径内做 A*。搜索节点和展开数都有
// 硬上限；找不到就返回 false，调用方会让怪物放弃，而不是继续扩大到全图。
func (s *Scene) localMonsterPath(from, target domain.Pos, radius float64) ([]domain.Pos, bool) {
	if s.walkable == nil || !s.walkable(from) || !s.walkable(target) || radius <= 0 {
		return nil, false
	}
	start := monsterPathCell{}
	world := func(c monsterPathCell) domain.Pos {
		return domain.Pos{MapID: from.MapID,
			X: from.X + float64(c.x)*monsterPathGrid,
			Y: from.Y + float64(c.y)*monsterPathGrid}
	}
	heuristic := func(p domain.Pos) float64 { return math.Hypot(target.X-p.X, target.Y-p.Y) }

	open := monsterPathQueue{{cell: start, f: heuristic(from)}}
	heap.Init(&open)
	gScore := map[monsterPathCell]float64{start: 0}
	cameFrom := make(map[monsterPathCell]monsterPathCell)
	closed := make(map[monsterPathCell]bool)
	radiusSq := radius * radius
	sequence, expanded := 1, 0
	var goal monsterPathCell
	found := false

	for open.Len() > 0 && expanded < monsterPathMaxExpanded {
		item := heap.Pop(&open).(monsterPathQueueItem)
		best, ok := gScore[item.cell]
		if !ok || item.g != best || closed[item.cell] {
			continue
		}
		closed[item.cell] = true
		expanded++
		at := world(item.cell)
		if heuristic(at) <= monsterPathGrid*1.5 && s.straightPathWalkable(at, target) {
			goal, found = item.cell, true
			break
		}

		for _, delta := range monsterPathNeighbours {
			next := monsterPathCell{x: item.cell.x + delta.x, y: item.cell.y + delta.y}
			if closed[next] {
				continue
			}
			nextAt := world(next)
			dx, dy := nextAt.X-from.X, nextAt.Y-from.Y
			if dx*dx+dy*dy > radiusSq || !s.straightPathWalkable(at, nextAt) {
				continue
			}
			step := monsterPathGrid
			if delta.x != 0 && delta.y != 0 {
				step *= math.Sqrt2
			}
			candidate := item.g + step
			if previous, seen := gScore[next]; seen && candidate >= previous {
				continue
			}
			gScore[next] = candidate
			cameFrom[next] = item.cell
			heap.Push(&open, monsterPathQueueItem{
				cell: next, g: candidate, f: candidate + heuristic(nextAt), seq: sequence,
			})
			sequence++
		}
	}
	if !found {
		return nil, false
	}

	cells := []monsterPathCell{goal}
	for cells[len(cells)-1] != start {
		previous, ok := cameFrom[cells[len(cells)-1]]
		if !ok {
			return nil, false
		}
		cells = append(cells, previous)
	}
	points := make([]domain.Pos, 0, len(cells))
	for i := len(cells) - 2; i >= 0; i-- { // 去掉起点，按正向恢复
		points = append(points, world(cells[i]))
	}
	points = append(points, target)
	return s.simplifyMonsterPath(from, points), true
}

// simplifyMonsterPath 把 20px 的 A* 折线压成少量可直达拐点。客户端收到的路径
// 更平滑，服务端也不用让怪物在每个网格中心轻微转向。
func (s *Scene) simplifyMonsterPath(from domain.Pos, points []domain.Pos) []domain.Pos {
	if len(points) < 2 {
		return points
	}
	out := make([]domain.Pos, 0, len(points))
	at := from
	for first := 0; first < len(points); {
		furthest := first
		for i := len(points) - 1; i > first; i-- {
			if s.straightPathWalkable(at, points[i]) {
				furthest = i
				break
			}
		}
		out = append(out, points[furthest])
		at = points[furthest]
		first = furthest + 1
	}
	return out
}

// shouldScan 按实体 id 把索敌错开到不同帧, 避免 400 只怪同帧一起扫九宫格。
func (s *Scene) shouldScan(id domain.EntityID) bool {
	return (uint64(s.tick)+uint64(id))%aiScanEvery == 0
}

// aiParams 从实体上取行为参数。
//
// 视野/追击距离取数据库 AI 模板；没有模板的旧实体使用 400/600 兜底。
func aiParams(m *entity.Entity) ai.Params {
	d := m.Monster
	view, trace := float64(d.AI.ViewDist), float64(d.AI.TraceDist)
	attackDist := float64(d.AI.BasicAttackDist)
	if d.AI.ID == "" {
		view, trace = float64(d.ViewDist), float64(d.TraceDist)
	}
	if view <= 0 && d.Kind.Hostile() {
		view = 400
	}
	if trace <= 0 && d.Kind.Hostile() {
		trace = 600
	}
	if attackDist <= 0 {
		attackDist = ai.MeleeRange
	}
	return ai.Params{
		ViewDist:   view,
		TraceDist:  trace,
		AttackDist: attackDist,
		MoveSpeed:  float64(m.Stats.MoveSpeed),
		Aggressive: d.Aggressive && !d.NoAttack,
	}
}

// acquire 在视野里挑一个玩家当目标。挑**最近的那个** ——
// 挑随机的会让怪的行为看起来像抽风, 挑第一个则受 map 遍历顺序影响, 不可复现。
func (s *Scene) acquire(m *entity.Entity, p ai.Params) {
	type candidate struct {
		entity *entity.Entity
		distSq float64
	}
	candidates := make([]candidate, 0, 4)
	s.aoi.AroundPos(m.Pos, func(o *entity.Entity) {
		if o.Kind != domain.KindPlayer || !o.Alive() || entityInvisible(o) || s.entityMapLoading(o) {
			return
		}
		if o.Player != nil && o.Player.Protected(s.tick) {
			return // 复活保护中, 别再扑上去
		}
		if !ai.CanAcquire(p, m.Pos, m.Monster.Home, o.Pos) {
			return
		}
		candidates = append(candidates, candidate{entity: o, distSq: sqDist(m.Pos, o.Pos)})
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distSq != candidates[j].distSq {
			return candidates[i].distSq < candidates[j].distSq
		}
		return candidates[i].entity.ID < candidates[j].entity.ID
	})
	for _, cand := range candidates {
		if _, _, ok := s.monsterChaseWaypoint(m, cand.entity.Pos, p.TraceDist); !ok {
			continue
		}
		m.Monster.Target = cand.entity.ID
		m.Monster.Roaming = false
		s.monsterSpeak(m, domain.MonsterSceneEnemy)
		return
	}
}

// targetInfo 把当前目标翻成决策要的那点信息。目标已经不在场景里就当没有目标。
func (s *Scene) targetInfo(m *entity.Entity) *ai.Target {
	id := m.Monster.Target
	if id == 0 {
		return nil
	}
	t, ok := s.entities[id]
	if !ok || entityInvisible(t) || s.entityMapLoading(t) {
		s.dropTarget(m) // 目标下线/传送离图：原地脱战，不回出生点
		return nil
	}
	return &ai.Target{Pos: t.Pos, Alive: t.Alive()}
}

// dropTarget 脱战。除了清目标, 还要清命中步进器 ——
// 不清的话下次打新目标会被当成同一场交战, 累加器不重置。
func (s *Scene) dropTarget(m *entity.Entity) {
	// 服务端虽然已经原地脱战，客户端仍可能沿最后一次追击/拉距路径插值，
	// 因此显式下发权威坐标把真实移动路径停住。
	if m.Monster.CombatPathActive {
		s.emit(event.EntityMoved{ID: m.ID, To: m.Pos, Snap: true})
	}
	m.Monster.CombatPathActive = false
	m.Monster.Target = 0
	m.Monster.TauntPower = 0
	m.Monster.Roaming = false
	s.clearMonsterChasePath(m.Monster)
	s.scheduleNextRoam(m.Monster)
	if tr := s.hits[m.ID]; tr != nil {
		tr.Reset()
	}
}

// dropMonsterAggroForPlayer 实现隐身的核心语义：怪物失去对该玩家的仇恨。
// 当前怪物模型只有一个权威 Target，没有另一张威胁值表，因此清掉 Target
// 就是完整清仇恨。不从 attacks/casts 队列删除已起手效果。
func (s *Scene) dropMonsterAggroForPlayer(player domain.EntityID) {
	if player == 0 {
		return
	}
	dropped := 0
	for _, monster := range s.monsters {
		if monster == nil || monster.Monster == nil || monster.Monster.Target != player {
			continue
		}
		s.dropTarget(monster)
		dropped++
	}
	if dropped > 0 {
		s.log.Debug("隐身清除怪物仇恨", "玩家", player, "怪物数", dropped)
	}
}

// moveEntity 把一个非玩家实体挪到新位置并广播。AOI 只更新服务端空间索引；
// 客户端实体按整张地图常驻，跨格不再触发出场/消失。
//
// 与玩家移动共用同一套 AOI 逻辑, 但**不发给自己**这条不适用 ——
// 怪和宠物都没有自己的客户端, 所有看得见它的玩家都要收到。
//
// 怪与宠物共用它: 这段代码从头到尾没碰过 m.Monster, 本来就是通用的。
func (s *Scene) moveEntity(m *entity.Entity, to domain.Pos) {
	if to == m.Pos {
		return
	}
	s.aoi.Move(m, to)
	if m.Kind == domain.KindMonster && m.Monster != nil && m.Monster.Roaming {
		if (uint64(s.tick)+uint64(m.ID))%uint64(monsterMoveBroadcastEvery) != 0 {
			return // 游荡直线已在起步前经 MASK 校验，500ms 同步足够
		}
	}
	// 追击与绕障不能复用游荡的 500ms 节流：客户端会把两个权威点直接插值；
	// A* 路径在这段时间内拐弯时，视觉轨迹会切过墙体。困难副本的怪移动更快，
	// 因而最容易暴露。每个追击步长在调用前均已通过 straightPathWalkable。
	s.emitEntityMove(m, to)
	if m.Kind == domain.KindMonster && m.Monster != nil {
		m.Monster.CombatPathActive = true
	}
}

func (s *Scene) emitEntityMove(m *entity.Entity, to domain.Pos) {
	moved := event.EntityMoved{ID: m.ID, To: to, Dir: m.Dir}
	if (m.Kind == domain.KindMonster && m.Monster != nil) ||
		(m.Kind == domain.KindPet && m.Pet != nil) {
		moved.Speed = m.Stats.MoveSpeed
	}
	s.emit(moved)
}

// aggro 让被打的一方把攻击者记成仇恨目标。
//
// **不主动的怪靠这条还手** —— 它们不会去找人, 但挨了打一定回击。
// 已经有目标就不改, 免得两个人轮流打一只怪时它在中间来回抽风。
//
// 辅助宠物不是战斗单位，不能成为仇恨来源。
func (s *Scene) aggro(victim *entity.Entity, attacker domain.EntityID) {
	if victim.Monster == nil || victim.Monster.NoAttack || victim.Monster.Target != 0 || attacker == 0 {
		return
	}
	a, ok := s.entities[attacker]
	if !ok || a.Kind != domain.KindPlayer || entityInvisible(a) {
		return
	}
	victim.Monster.Target = attacker
	victim.Monster.Roaming = false
	s.monsterSpeak(victim, domain.MonsterSceneEnemy)
	s.callMonsterHelp(victim, attacker)
}

// taunt 强制怪物把施法者设为当前目标。客户端资料没有保存旧服的绝对仇恨值，
// 只明确“每次升级效果更佳”，所以把技能等级作为覆盖优先级：普通受击仇恨为
// 0，高等级嘲讽可覆盖低等级嘲讽，反向则不能抢走目标。
func (s *Scene) taunt(victim *entity.Entity, attacker domain.EntityID, power int32) bool {
	if victim == nil || victim.Monster == nil || victim.Monster.NoAttack || attacker == 0 || power <= 0 {
		return false
	}
	a, ok := s.entities[attacker]
	if !ok || a.Kind != domain.KindPlayer || !a.Alive() {
		return false
	}
	if victim.Monster.Target != 0 && victim.Monster.Target != attacker && power < victim.Monster.TauntPower {
		return false
	}
	if victim.Monster.Target != attacker {
		s.dropTarget(victim)
		victim.Monster.Target = attacker
		s.monsterSpeak(victim, domain.MonsterSceneEnemy)
	}
	victim.Monster.TauntPower = power
	victim.Monster.Roaming = false
	return true
}

// callMonsterHelp 只唤醒同模板、在帮助半径内且当前没有目标的同伴。直接赋目标
// 而不递归调用 aggro，避免一只怪把整张地图连锁唤醒。
func (s *Scene) callMonsterHelp(victim *entity.Entity, attacker domain.EntityID) {
	if victim == nil || victim.Monster == nil || s.defs == nil || victim.Monster.AI.HelpRadius <= 0 {
		return
	}
	def, ok := s.defs.Def(victim.Monster.TypeID)
	if !ok || !def.CallHelp {
		return
	}
	radiusSq := float64(victim.Monster.AI.HelpRadius) * float64(victim.Monster.AI.HelpRadius)
	s.aoi.AroundPos(victim.Pos, func(other *entity.Entity) {
		if other == nil || other.ID == victim.ID || !other.Alive() || other.Monster == nil ||
			other.Monster.NoAttack ||
			other.Monster.TypeID != victim.Monster.TypeID || other.Monster.Target != 0 ||
			sqDist(victim.Pos, other.Pos) > radiusSq {
			return
		}
		other.Monster.Target = attacker
		other.Monster.Roaming = false
		s.monsterSpeak(other, domain.MonsterSceneEnemy)
	})
}

func sqDist(a, b domain.Pos) float64 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx + dy*dy
}
