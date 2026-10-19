package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func behaviorFixture(t *testing.T) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	d := 被动树妖
	d.CallHelp = true
	d.Flee = true
	d.AI = domain.MonsterAIProfile{ID: "behavior-test", ViewDist: 400, TraceDist: 600, HelpRadius: 400}
	s, id := aiScene(t, d, 290, 100)
	peer := d
	peer.ID = 1002
	s.defs = spawn.MapDefs{d.ID: d, peer.ID: peer}
	join(t, s, 1, 100, "甲", 250, 100)
	m := s.entities[id]
	copyMonster := *m.Monster
	copyMonster.TypeID = 1002
	other := *m
	other.ID = 90001
	other.Monster = &copyMonster
	other.Pos.X = 610
	s.entities[other.ID] = &other
	s.monsters[other.ID] = &other
	s.aoi.Enter(&other)
	return s, m, &other
}

func TestAuditedHelpRecruitsOtherTypeAcrossAOICell(t *testing.T) {
	s, m, other := behaviorFixture(t)
	m.Monster.Target = 1 // acquired before being hit
	s.aggro(m, 1)
	if other.Monster.Target != 1 {
		t.Fatal("already-engaged caller did not recruit nearby other type")
	}
	if m.Monster.Target != 1 {
		t.Fatal("caller target changed")
	}
}
func TestAuditedHelpDoesNotStealFightOrRecruitProps(t *testing.T) {
	s, m, other := behaviorFixture(t)
	other.Monster.Target = 33
	s.aggro(m, 1)
	if other.Monster.Target != 33 {
		t.Fatal("stole existing fight")
	}
	s.tick += 10
	other.Monster.Target = 0
	other.Monster.NoAttack = true
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("recruited nonattacking object")
	}
}
func TestAuditedHelpRateLimitAndFatalHit(t *testing.T) {
	s, m, other := behaviorFixture(t)
	s.aggro(m, 1)
	other.Monster.Target = 0
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("repeated hit bypassed rate limit")
	}
	s.tick += 10
	m.HP = 0
	s.aggro(m, 1)
	if other.Monster.Target != 1 {
		t.Fatal("fatal hit failed to alert companions")
	}
}
func TestAbove60HelpKeepsPreviousRules(t *testing.T) {
	s, m, other := behaviorFixture(t)
	d, _ := s.defs.Def(m.Monster.TypeID)
	d.Level = 61
	s.defs = spawn.MapDefs{d.ID: d}
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("high-level caller recruited different type")
	}
	other.Monster.TypeID = m.Monster.TypeID
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("high-level caller changed already-targeted semantics")
	}
}
func TestMonsterFleeBoundedAndCooldown(t *testing.T) {
	s, m, _ := behaviorFixture(t)
	m.Monster.Target = 1
	m.HP = m.MaxHP / 5
	before := m.Pos
	if !s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
		t.Fatal("low HP did not flee")
	}
	if m.Pos.X <= before.X {
		t.Fatal("monster did not move away from attacker")
	}
	if m.Monster.FleeUntil != 30 || m.Monster.NextFleeAt != 100 {
		t.Fatal("unexpected flee defaults")
	}
	s.tick = 30
	if s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
		t.Fatal("fled past duration")
	}
	s.tick = 99
	if s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
		t.Fatal("flee bypassed cooldown")
	}
	s.tick = 100
	if !s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
		t.Fatal("flee did not resume after cooldown")
	}
	s.dropTarget(m)
	if m.Monster.FleeUntil != 0 {
		t.Fatal("lost target retained flee state")
	}
}
func TestMonsterFleeUsesExplicitTemplateOnly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		level   int32
		enabled bool
		hp      int32
	}{{"unlisted", 14, false, 1}, {"over60", 61, true, 1}, {"healthy", 14, true, 246}} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, _ := behaviorFixture(t)
			d, _ := s.defs.Def(m.Monster.TypeID)
			d.Level = tc.level
			d.Flee = tc.enabled
			s.defs = spawn.MapDefs{d.ID: d}
			m.Monster.Target = 1
			m.HP = tc.hp
			if s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
				t.Fatal("unqualified monster fled")
			}
		})
	}
}
func TestMonsterFleeConfiguredTiming(t *testing.T) {
	s, m, _ := behaviorFixture(t)
	d, _ := s.defs.Def(m.Monster.TypeID)
	d.AI.FleeHPPercent = 50
	d.AI.FleeDurationTicks = 5
	d.AI.FleeCooldownTicks = 40
	s.defs = spawn.MapDefs{d.ID: d}
	m.Monster.Target = 1
	m.HP = m.MaxHP / 2
	if !s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) || m.Monster.FleeUntil != 5 || m.Monster.NextFleeAt != 40 {
		t.Fatal("configured flee policy ignored")
	}
}

func TestMonsterBehaviorDoesNotLeakAcrossSpawnOrElite(t *testing.T) {
	s, m, _ := behaviorFixture(t)
	d, _ := s.defs.Def(m.Monster.TypeID)
	elite := d
	elite.ID = 9002
	elite.Kind = domain.MonsterElite
	elite.Flee = false
	d.EliteID = elite.ID
	defs := spawn.MapDefs{d.ID: d, elite.ID: elite}
	s.defs = defs
	// Summons look up their own template and receive fresh runtime state.
	summoned := spawn.Summon(91000, m.Pos, d, m.ID)
	s.initializeMonster(summoned)
	summoned.Monster.Target = 1
	summoned.HP = 1
	if !s.stepMonsterFlee(summoned, s.targetInfo(summoned), aiParams(summoned)) {
		t.Fatal("summon lost configured flee trait")
	}
	summoned.Monster.HelpReadyAt = 77
	s.initializeMonster(summoned)
	if summoned.Monster.NextFleeAt != 0 || summoned.Monster.FleeUntil != 0 || summoned.Monster.HelpReadyAt != 0 {
		t.Fatal("initialization leaked behavior runtime state")
	}
	// A separately configured elite does not inherit the normal monster's trait.
	em := spawn.Summon(91001, m.Pos, elite, m.ID)
	s.initializeMonster(em)
	em.Monster.Target = 1
	em.HP = 1
	if s.stepMonsterFlee(em, s.targetInfo(em), aiParams(em)) {
		t.Fatal("elite inherited normal template flee")
	}
	sp, _ := spawn.New([]domain.SpawnPoint{{ID: 1, Monster: d.ID, Pos: m.Pos}}, defs, domain.NewEntityAlloc())
	first := sp.Respawn(1, 99)
	first.Monster.NextFleeAt = 999
	sp.Release(1)
	second := sp.Respawn(1, 99)
	if second == nil || second.Monster.NextFleeAt != 0 || second.Monster.TypeID != d.ID {
		t.Fatal("respawn did not receive a fresh behavior state")
	}
}

func TestMonsterFleeBlockedDoesNotFreezeCombat(t *testing.T) {
	s, m, _ := behaviorFixture(t)
	m.Monster.Target = 1
	m.HP = 1
	s.walkable = func(domain.Pos) bool { return false }
	before := m.Pos
	if s.stepMonsterFlee(m, s.targetInfo(m), aiParams(m)) {
		t.Fatal("blocked flee consumed combat tick")
	}
	if m.Pos != before || m.Monster.FleeUntil != 0 || m.Monster.NextFleeAt == 0 {
		t.Fatal("blocked flee state incorrect")
	}
}
func TestAuditedHelpDoesNotCrossWallOrRadius(t *testing.T) {
	s, m, other := behaviorFixture(t)
	s.walkable = func(p domain.Pos) bool { return p.X < 400 || p.X > 420 }
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("help crossed wall")
	}
	s.walkable = nil
	s.tick += 10
	s.aoi.Move(other, domain.Pos{MapID: 14, X: 800, Y: 100})
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("help crossed radius")
	}
}

func TestAuditedHelpDoesNotChangeAbove60Recipient(t *testing.T) {
	s, m, other := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	d := defs[other.Monster.TypeID]
	d.Level = 61
	defs[d.ID] = d
	s.aggro(m, 1)
	if other.Monster.Target != 0 {
		t.Fatal("audited caller changed higher-level recipient behavior")
	}
}

func TestMonsterFleePreemptsAICombat(t *testing.T) {
	s, m, _ := behaviorFixture(t)
	m.Monster.Target = 1
	m.HP = 1
	before := m.Pos
	s.stepAI()
	if m.Pos.X <= before.X || m.Monster.FleeUntil == 0 {
		t.Fatal("AI did not route low health monster through flee")
	}
	for _, attack := range s.attacks {
		if attack.src == m.ID {
			t.Fatal("fleeing monster still queued attack")
		}
	}
}
