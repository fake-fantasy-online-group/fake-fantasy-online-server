package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"reflect"
	"testing"
)

// Frozen pre-shortcut execution path checks state, queued attacks and movement events.
func (s *Scene) stepAIWithoutIdleShortcutReference() {
	if s.rift != nil && !s.riftCombatReady() {
		return
	}
	if len(s.monsters) == 0 {
		return
	}
	for _, m := range s.monsters {
		if s.riftMonsterAbilityBusy(m.ID) {
			continue
		}
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

func TestIdleAIShortcutParity(t *testing.T) {
	for _, mode := range []string{"waiting", "wandering", "stationary", "missing-target", "acquiring", "passive"} {
		t.Run(mode, func(t *testing.T) {
			def := 主动树妖
			if mode == "stationary" {
				def = 食人花
			}
			if mode == "passive" {
				def = 被动树妖
			}
			old, oldID := aiScene(t, def, 100, 100)
			got, gotID := aiScene(t, def, 100, 100)
			oldSink := join(t, old, 1, 100, "observer", 1000, 1000)
			gotSink := join(t, got, 1, 100, "observer", 1000, 1000)
			for _, s := range []*Scene{old, got} {
				var mID domain.EntityID
				for id := range s.monsters {
					mID = id
				}
				m := s.entities[mID]
				m.Monster.NextRoamAt = 1000
				if mode == "wandering" {
					m.Monster.NextRoamAt = 1
				}
				if mode == "missing-target" {
					m.Monster.Target = 999999
				}
				if mode == "acquiring" || mode == "passive" {
					s.aoi.Move(s.entities[1], domain.Pos{MapID: 14, X: 150, Y: 100})
				}
			}
			oldSink.take()
			gotSink.take()
			for tick := domain.Tick(1); tick <= 100; tick++ {
				old.tick, got.tick = tick, tick
				old.stepAIWithoutIdleShortcutReference()
				got.stepAI()
				if !reflect.DeepEqual(old.entities[oldID], got.entities[gotID]) {
					t.Fatalf("tick %d: monster state diverged", tick)
				}
				if !reflect.DeepEqual(old.attacks, got.attacks) {
					t.Fatalf("tick %d: attacks diverged", tick)
				}
				if want, actual := oldSink.take(), gotSink.take(); !reflect.DeepEqual(want, actual) {
					t.Fatalf("tick %d: events differ: want %+v, got %+v", tick, want, actual)
				}
			}
		})
	}
}
