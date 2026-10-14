package scene

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func TestEliteRuntimeReviewExpiredHazardCannotArmInvisibleReconnect(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 2)
	if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[2], *now) {
		t.Fatal("fixture did not create lava orb")
	}
	s.armRiftHazards(p, *now)
	s.forgetRiftHazardPlayer(p.ID)
	s.rift.economicPending = true
	*now += 4000
	if len(s.riftView(p, s.rift.data, true, true).Hazards) != 0 {
		t.Fatal("fixture expected expired orb omitted from reconnect view")
	}
	s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
	*now += 1000
	s.rift.economicPending = false
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 10000 {
		t.Fatalf("reconnecting player took invisible expired-orb damage: hp=%d", p.HP)
	}
}

func TestEliteRuntimeReviewEconomicReloadCannotResurrectPoisonOccupancy(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now) {
		t.Fatal("fixture did not create poison")
	}
	s.armRiftHazards(p, *now)
	for i := 0; i < 5; i++ {
		*now += 1000
		s.stepRiftHazards(*now)
	}
	record := s.rift.data.record(s.rift.revision + 1)
	s.rift.economicPending = true
	// Death/revival retains the actor ID; its transport and poison history
	// was explicitly cleared while an earlier economic aggregate was pending.
	s.forgetRiftHazardPlayer(p.ID)
	*now += 5000
	if !s.acceptEndgameRiftReload(domain.EndgameRunLoadResult{Run: &record}) {
		t.Fatal("fixture economic reload rejected")
	}
	s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
	before := p.HP
	s.stepRiftHazards(*now)
	if p.HP != before {
		t.Fatalf("earlier economic aggregate restored dead actor occupancy and bypassed fresh warning: before=%d after=%d", before, p.HP)
	}
	*now += 1000
	s.stepRiftHazards(*now)
	if got := before - p.HP; got != 5 {
		t.Fatalf("revived actor inherited old poison ramp instead of first tick: damage=%d", got)
	}
}

func TestEliteRuntimeReviewFinalProgressCollectionKeepsLava(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 2)
	r := s.rift.data.Run
	r.Rules.GuardianArrivalMS = 5000
	r.Progress = r.Rules.ProgressMax - r.Rules.GlobeProgress
	p.Pos = m.Pos
	m.HP = 0
	s.onEndgameRiftMonsterDeath(m, p)
	if len(s.eliteSnapshot().Effects) != 1 || len(r.Globes) == 0 {
		t.Fatal("final elite death did not generate lava and progress globes")
	}
	s.stepEndgameRift()
	if r.Phase != domain.RiftBossArriving || len(s.monsters) != 0 || len(s.eliteSnapshot().Effects) != 1 {
		t.Fatalf("real progress transition lost lava: phase=%s monsters=%d effects=%d", r.Phase, len(s.monsters), len(s.eliteSnapshot().Effects))
	}
	*now += 3000
	s.stepEndgameRift()
	if p.HP != 9840 || len(s.eliteSnapshot().Effects) != 0 {
		t.Fatalf("arrival failed exact-once lava resolution: hp=%d effects=%d", p.HP, len(s.eliteSnapshot().Effects))
	}
	s.stepEndgameRift()
	if p.HP != 9840 {
		t.Fatal("replayed final-progress lava")
	}
}

func TestEliteRuntimeReviewCloneNativeCastAndScaledBaseSurviveRefresh(t *testing.T) {
	s, owner, p, now := riftEliteFixture(t, 1)
	defs := s.defs.(spawn.MapDefs)
	def := defs[owner.Monster.TypeID]
	def.Skills = []domain.MonsterSkillRule{{Enabled: true, ChanceBP: 10000, MaxHPPct: 100, Cooldown: 40,
		Skill: domain.SkillDef{ID: 12013, Level: 1, Kind: domain.SkillSingle, Dist: 450, TargetEnemy: true, HurtType: domain.HurtFire,
			Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}}}
	defs[owner.Monster.TypeID] = def
	base := s.rift.baseStats[owner.ID]
	base.MAtk, base.Def, base.MoveSpeed = 100, 77, 100
	s.rift.baseStats[owner.ID] = base
	// The current owner can have temporary offensive/movement changes; clones
	// must only inherit the scaled, unbuffed base and their own native runtime.
	owner.Stats.MAtk, owner.Stats.MoveSpeed = 999, 999
	owner.Monster.SkillReady[12013] = s.tick + 999
	owner.Monster.SkillCasts[12013] = 3
	if !s.spawnRiftClones(owner, s.currentRiftRules().Abilities[1], *now) {
		t.Fatal("spawn clones")
	}
	before := p.HP
	for id := range s.eliteSnapshot().Clones {
		clone := s.monsters[id]
		s.refreshEntityStats(clone)
		if clone.MaxHP != 500 || clone.Stats.MaxHP != 500 || clone.Stats.MAtk != 100 || clone.Stats.Def != 77 || clone.Stats.MoveSpeed != 100 {
			t.Fatalf("clone reverted to template or inherited current buffs: hp=%d stats=%+v", clone.MaxHP, clone.Stats)
		}
		if !s.tryMonsterSkill(clone, s.targetInfo(clone)) || clone.Monster.SkillCasts[12013] != 1 || clone.Monster.SkillReady[12013] != s.tick+40 {
			t.Fatal("clone cannot independently execute its native spell")
		}
	}
	s.stepCasts()
	if p.HP >= before {
		t.Fatal("accepted native clone casts never damaged their target")
	}
	if owner.Monster.SkillReady[12013] != s.tick+999 || owner.Monster.SkillCasts[12013] != 3 {
		t.Fatal("clone changed owner skill runtime")
	}
	progress, phase, xp, globes, drops := s.rift.data.Run.Progress, s.rift.data.Run.Phase, p.Player.Char.Exp, len(s.rift.data.Run.Globes), len(s.ground)
	for id := range s.eliteSnapshot().Clones {
		clone := s.monsters[id]
		clone.HP = 0
		s.onDeath(clone, p.ID)
	}
	if s.rift.data.Run.Progress != progress || s.rift.data.Run.Phase != phase || p.Player.Char.Exp != xp || len(s.rift.data.Run.Globes) != globes || len(s.ground) != drops {
		t.Fatal("killed clones produced experience, drops or run progression")
	}
}

func TestEliteRuntimeReviewPoisonOverlapChoosesHighestDamage(t *testing.T) {
	sample := func(physical, magical bool) int32 {
		s, m, p, now := riftEliteFixture(t, 6)
		p.Stats.MDef = 10000
		if physical {
			m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
			s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now)
		}
		if magical {
			m.Stats = domain.Stats{MAtk: 101}
			s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now)
		}
		s.armRiftHazards(p, *now)
		*now += 1000
		before := p.HP
		s.stepRiftHazards(*now)
		return before - p.HP
	}
	physical, magical, overlap := sample(true, false), sample(false, true), sample(true, true)
	if overlap != max(physical, magical) {
		t.Fatalf("overlap selected raw offense rather than strongest resulting damage: physical=%d magical=%d overlap=%d", physical, magical, overlap)
	}
}

func TestEliteRuntimeReviewPoisonComparisonHasNoCombatSideEffects(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	s.refreshEntityStats(p)
	p.HP = p.MaxHP
	p.Stats.PhysResist = 0
	p.Status.Apply(domain.StatusDef{ID: 99981, Level: 1, TypeKnown: true, Type: domain.StatusGood, DamageShield: 3, DurationSec: 20}, s.tick, p.ID)
	p.Status.Apply(domain.StatusDef{ID: 99982, Level: 1, TypeKnown: true, Type: domain.StatusBad, BreakOnDamage: true, DurationSec: 20}, s.tick, m.ID)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now)
	var h *endgameRiftHazard
	for _, hazard := range s.eliteSnapshot().Effects {
		h = hazard
	}
	hp, mp, events, statuses := p.HP, p.MP, len(s.outbox), p.Status.Clone()
	s.rng = rand.New(rand.NewSource(77451))
	expectRNG := rand.New(rand.NewSource(77451))
	for i := 0; i < 20; i++ {
		if got := s.riftPoisonNominalDamage(h, p, 5); got != 2 {
			t.Fatalf("nominal damage with shield: %d", got)
		}
	}
	if p.HP != hp || p.MP != mp || !reflect.DeepEqual(statuses, p.Status) || len(s.outbox) != events || s.rng.Int63() != expectRNG.Int63() {
		t.Fatal("nominal comparison consumed RNG, HP, shields, statuses or emitted combat events")
	}
	s.armRiftHazards(p, *now)
	*now += 1000
	s.stepRiftHazards(*now)
	if p.HP != hp-2 || p.Status.Has(99981) || p.Status.Has(99982) {
		t.Fatalf("winner was not resolved once through shield consumption and break-on-damage: hp=%d", p.HP)
	}
}

func TestEliteRuntimeReviewSixHazardBudgetRetriesWithoutFormalCooldown(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 3)
	if got := s.currentRiftRules().Elite.HazardMaxConcurrent; got != 6 {
		t.Fatalf("expected approved budget 6, got %d", got)
	}
	for i := 0; i < 6; i++ {
		if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[4], *now) {
			t.Fatal("fill active budget")
		}
	}
	state := s.rift.abilities[m.ID]
	caster := s.eliteSnapshot().Casters[state.eliteKey]
	sequence := s.eliteSnapshot().Sequence
	s.stepRiftElite(*now)
	if len(s.eliteSnapshot().Effects) != 6 || s.eliteSnapshot().Sequence != sequence || caster.Ready[3] != *now+1000 || caster.RecoverUntil != 0 || p.HP != 10000 {
		t.Fatal("capacity failure generated an effect, damaged player, or consumed formal cooldown/action recovery")
	}
	*now += 1000
	s.stepRiftElite(*now)
	if caster.Ready[3] != *now+1000 || len(s.eliteSnapshot().Effects) != 6 {
		t.Fatal("capacity retry timing")
	}
	for id := range s.eliteSnapshot().Effects {
		delete(s.eliteSnapshot().Effects, id)
		break
	}
	*now += 1000
	s.stepRiftElite(*now)
	if len(s.eliteSnapshot().Effects) != 6 || s.eliteSnapshot().Sequence != sequence+1 || caster.Ready[3] != *now+20000 {
		t.Fatal("released capacity did not admit exactly one normal-cooldown cast")
	}
	// Death lava has its own reserved capacity and cannot disappear because
	// active area effects already consume all six normal slots.
	state.pattern = []endgameRiftAbility{2}
	m.HP = 0
	s.onEndgameRiftMonsterDeath(m, p)
	if len(s.eliteSnapshot().Effects) != 7 {
		t.Fatal("active budget silently dropped the reserved death effect")
	}
}
