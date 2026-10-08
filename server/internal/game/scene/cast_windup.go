package scene

import (
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// castWindup 由队列副本共享。技能结算中可能命中另一位仍在前摇的玩家，
// 不能原地删改正在遍历/压缩的 casts 切片，否则会漏结算或复活已取消的施法。
type castWindup struct {
	finished    bool
	interrupted bool
}

func (s *Scene) castInterruptChanceBP(caster *entity.Entity) int32 {
	reductionBP := s.passiveGlobalChanceBP(caster, domain.PassiveCastInterruptReductionPct)
	chance := s.combatTiming.CastInterruptBaseBP * (10000 - reductionBP) / 10000
	if chance < s.combatTiming.CastInterruptMinBP {
		chance = s.combatTiming.CastInterruptMinBP
	}
	return chance
}

// afterCastDamage 必须在真实伤害事件之后调用。普攻/技能实际扣血才判定打断；
// 持续掉血、反伤和完全吸收仅恢复客户端可能被 PlayHurt 清掉的剩余准备态。
// 未命中的伤害包不会触发受击表现，也不需要重发吟唱。
func (s *Scene) afterCastDamage(dealt event.DamageDealt, directHit bool) {
	caster := s.entities[dealt.Dst]
	if caster == nil || caster.Player == nil || !caster.Alive() || dealt.Flag.Has(event.DamageMiss) {
		return
	}
	now := s.now()
	for _, cast := range s.casts {
		if cast.src != caster.ID || cast.windup == nil || cast.windup.finished ||
			cast.released || !now.Before(cast.releaseAtWall) {
			continue
		}
		remaining := cast.releaseAtWall.Sub(now)
		if directHit && dealt.Amount > 0 {
			chance := s.castInterruptChanceBP(caster)
			roll := int32(s.rng.Intn(10000))
			interrupted := roll < chance
			s.log.Debug("受击吟唱判定", "char", caster.Name, "skill", cast.def.ID,
				"source", dealt.Src, "chance_bp", chance, "roll_bp", roll,
				"interrupted", interrupted, "remaining_ms", remaining.Milliseconds())
			if interrupted {
				cast.windup.finished, cast.windup.interrupted = true, true
				s.emitTo(caster.ID, event.ServerNotice{Who: caster.ID, Text: "技能吟唱已中断。"})
				return
			}
		}
		// 0x800e phase=0 是已有的吟唱协议，cooldown=0 不重开冷却。
		// 不携带 aim，避免重复创建位置型技能的持续特效；真实目标/落点留在队列。
		// 只续播剩余时间，不改 releaseAtWall、不再扣 MP/耐久、不重新入队。
		s.emit(event.SkillCastChanged{
			Caster: caster.ID, Skill: cast.def.ID, Phase: event.SkillCastChant,
			ChantHoldMS: int32((remaining + time.Millisecond - 1) / time.Millisecond),
		})
		return
	}
}

func (s *Scene) clearCastWindup(id domain.EntityID) {
	for _, cast := range s.casts {
		if cast.src == id && cast.windup != nil && !cast.windup.finished {
			cast.windup.finished, cast.windup.interrupted = true, true
		}
	}
}
