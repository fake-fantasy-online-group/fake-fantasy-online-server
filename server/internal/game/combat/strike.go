package combat

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Blow 描述一次出手是怎么打的。零值 = 普通物理平砍。
type Blow struct {
	// Additive critical multiplier BP: 2000 means 200% -> 220%.
	CriticalBonusBP                        int32
	SourceOffense, CurrentSourceOffense    *domain.CombatSourceSnapshotV5
	AttackStats                            domain.Stats
	AttackStatsSnapshot                    bool
	Origin                                 domain.DamageOrigin
	RootID                                 uint64
	ProcDepth                              uint8
	EquipmentOffense                       domain.EquipmentCombatStats
	EquipmentSnapshot                      bool
	SkillPctNumerator, SkillPctDenominator int64
	// PayloadScaleBP is used only by approved equipment proc payloads.
	PayloadScaleBP int32
	// PayloadScaleKnown distinguishes a deliberately zero coefficient from unset.
	PayloadScaleKnown bool

	// Cast-local context survives stealth consumption and trap delays.
	StartedInvisible bool
	Area             bool
	HurtType         int32
	SkillID          int32 // 0 = 普通攻击
	// Magic 表示这是法系技能。**法系技能 100% 命中**（法杖平砍算物理, 照样 miss）。
	Magic bool
	// SkillPct 技能倍率, 百分比。0 视为 100。来自 ov_skilldesc 的「130%伤害」。
	SkillPct int32
	// FlatDamage 只在基础攻击真实命中后追加；它绕过基础护甲/抗性，但仍会
	// 进入场景层的状态增减伤。
	FlatDamage int32
	// Trap 标记这次伤害由已触发的地面陷阱造成，供陷阱回避被动独立判定。
	Trap bool
	// IgnoreDefense 是「无视防御」词缀触发了。玩家实测把它读成「绝对命中率」——
	// 这条恰好是「防御=闪避」最硬的一条反证。
	IgnoreDefense bool
	// GuaranteedHit 是技能自身的必中结果，不消耗命中步进器。
	GuaranteedHit bool
}

// CalculateStrike evaluates the legacy hit/crit/native-defense formula without
// mutating HP. Scenes apply shields, elemental defenses and absorption before
// committing HP once; standalone callers can still use Strike below.
// The caller owns alive, target, range and cooldown admission.
func CalculateStrike(a, d *entity.Entity, tr *HitTracker, b Blow, rng Rand) event.DamageDealt {
	if b.AttackStatsSnapshot {
		snapshot := *a
		snapshot.Stats = b.AttackStats
		a = &snapshot
	}
	ev := event.DamageDealt{Src: a.ID, Dst: d.ID, SkillID: b.SkillID, Origin: b.Origin, RootID: b.RootID, ProcDepth: b.ProcDepth}

	if tr.Engage(d.ID, a.Stats.Hit+d.Stats.Def, rng) {
		ev.Flag |= event.DamageOpening // 交战第一击，只用于服务端识别一轮新交战
	}

	// 暴击先掷 —— 它是「必中」的三种例外之一, 所以必须排在命中判定**前面**。
	critRate := a.Stats.CritRate
	if b.Magic {
		critRate = a.Stats.MCritRate
	}
	crit := Crit(critRate, rng)
	if crit {
		ev.Flag |= event.DamageCrit
	}

	var dmg int32
	switch {
	case b.Magic:
		// 法系不判命中。
		ev.Flag |= event.DamageMagic
		dmg = RollMagicFixedDefense(a.Stats.MAtk, b.SkillPct, d.Stats.MDef, d.Stats.MagicResist)
		if b.SkillPctDenominator > 0 {
			dmg = RollMagicFixedDefenseFraction(a.Stats.MAtk, b.SkillPctNumerator, b.SkillPctDenominator, d.Stats.MDef, d.Stats.MagicResist)
		}
	default:
		mustHit := crit || b.IgnoreDefense || b.GuaranteedHit
		if !mustHit && !tr.Roll(a.Stats.Hit, d.Stats.Def) {
			ev.Flag |= event.DamageMiss
			ev.Amount = 0 // 未命中的伤害必须是 0, 抓包 22/22 如此
			ev.DstHP = d.HP
			return ev
		}
		if b.SkillPctDenominator > 0 {
			dmg = RollPhysicalSkillFraction(a.Stats.MinAtk, a.Stats.MaxAtk, d.Stats.PhysResist, b.SkillPctNumerator, b.SkillPctDenominator, rng)
		} else {
			dmg = RollPhysicalSkill(a.Stats.MinAtk, a.Stats.MaxAtk, d.Stats.PhysResist, b.SkillPct, rng)
		}
	}

	if crit {
		dmg = boundedDamage(int64(dmg) * (CritMultiplier*10000 + int64(b.CriticalBonusBP)) / 10000)
	}

	// 扣血。伤害可以大于剩余血(溢出斩杀), 抓包实测最后一击伤害 7 > 剩余 4。
	hp := max(int64(0), int64(d.HP)-int64(dmg))
	if hp <= 0 {
		ev.Flag |= event.DamageFatal
	}
	ev.Amount = dmg
	ev.DstHP = int32(hp)
	return ev
}

// Strike retains the standalone legacy API; scenes use CalculateStrike so shield,
// absorption, death and HP commit exactly once after all channels resolve.
func Strike(a, d *entity.Entity, tr *HitTracker, b Blow, rng Rand) event.DamageDealt {
	ev := CalculateStrike(a, d, tr, b, rng)
	d.HP = ev.DstHP
	return ev
}

// Restore 结算并**应用**一次治疗, 返回该发出去的事件。
//
// 与 Strike 同一个道理: 算和加血绑在一起, 就不可能出现"飘了绿字但没回血"。
func Restore(src, dst *entity.Entity, amount int32, skillID int32) event.HealDone {
	if amount < 0 {
		amount = 0
	}
	// 不能超过上限 —— 溢出的部分不算, 但事件里报**实际回了多少**,
	// 否则客户端会飘一个和血条对不上的数
	if room := dst.MaxHP - dst.HP; amount > room {
		amount = room
	}
	dst.HP += amount
	ev := event.HealDone{
		Src: src.ID, Dst: dst.ID, Amount: amount, DstHP: dst.HP, SkillID: skillID,
		DstKind: dst.Kind,
	}
	if dst.Kind == domain.KindPet && dst.Pet != nil {
		ev.DstOwner = dst.Pet.Owner
	}
	return ev
}
