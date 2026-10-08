package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// entityInvisible 是服务端隐身状态的唯一入口。隐身不是“无法受到攻击”：
// 已经起手的攻击、周期伤害和陷阱仍可正常命中；它只驱动客户端表现与
// 怪物索敌/仇恨语义。
func entityInvisible(e *entity.Entity) bool {
	return e != nil && e.Status != nil && e.Status.Invisible()
}

// updatePlayerVisibility 把状态集合的隐身边沿翻译成客户端原生远端隐身开关。
// 本人由 0x8017 展示隐身 Buff；其他玩家走 0x805d RemotePlayer.SetInvis。
func (s *Scene) updatePlayerVisibility(e *entity.Entity, wasInvisible bool) {
	if e == nil || e.Kind != domain.KindPlayer || e.Player == nil {
		return
	}
	nowInvisible := entityInvisible(e)
	if nowInvisible == wasInvisible {
		return
	}
	if nowInvisible {
		// 隐身生效的瞬间就清掉所有怪物对该玩家的仇恨，不等各怪物
		// 下一次 AI 帧再被动发现。已排入结算队列的攻击不撤回，所以仍可
		// 命中并按 BreakOnDamage 使玩家显形。
		s.dropMonsterAggroForPlayer(e.ID)
	}
	s.emitExcept(event.PlayerVisibilityChanged{Who: e.ID, Invisible: nowInvisible}, e.ID)
}

func (s *Scene) removedStatusesIncludedInvisible(ids []domain.StatusID) bool {
	for _, id := range ids {
		for key, def := range s.statuses {
			if key.ID == id && def.Invisible {
				return true
			}
		}
	}
	return false
}

// revealInvisible 执行鹰眼的场景语义。PVP 尚未开放，因此这里唯一可证明的
// “敌方玩家”边界是非本人、非队友；不会把友方刺客从隐身中误拉出来。
func (s *Scene) revealInvisible(caster *entity.Entity, def domain.SkillDef, radius int32, statusDurationPct int32) {
	if caster == nil || radius <= 0 {
		return
	}
	radiusSq := float64(radius) * float64(radius)
	for _, target := range s.players {
		if target.ID == caster.ID || sameParty(caster, target) || !target.Alive() ||
			!entityInvisible(target) || sqDist(caster.Pos, target.Pos) > radiusSq {
			continue
		}
		removed := target.Status.RemoveWhere(func(existing domain.StatusDef) bool {
			return existing.Invisible
		})
		s.statusesRemoved(target, removed)
		for _, app := range def.Statuses {
			statusDef, ok := s.statuses.Get(app.ID, app.Level)
			if !ok {
				continue
			}
			if app.DurationSec > 0 {
				statusDef.DurationSec = app.DurationSec
			}
			if statusDurationPct > 0 {
				statusDef.DurationSec += statusDef.DurationSec * statusDurationPct / 100
			}
			if app.Description != "" {
				statusDef.Desc = app.Description
			}
			applyStatusSource(&statusDef, def.Icon, int32(def.ID))
			s.applyStatusDef(target, statusDef, caster.ID)
		}
	}
}
