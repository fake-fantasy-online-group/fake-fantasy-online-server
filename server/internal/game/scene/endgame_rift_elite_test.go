package scene

import (
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func riftEliteRulesFixture(t *testing.T) *domain.RiftRules {
	t.Helper()
	r := riftRulesFixture()
	r.Version = 6
	b, err := os.ReadFile("../../../docs/endgame/elite-skills-v6.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, r); err != nil {
		t.Fatal(err)
	}
	if err = r.Validate(); err != nil {
		t.Fatal(err)
	}
	return r
}
func riftEliteFixture(t *testing.T, ability int32) (*Scene, *entity.Entity, *entity.Entity, *int64) {
	t.Helper()
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	// Lifecycle tests isolate cadence from the newly magical hazard defense.
	p.Stats.MDef, p.Stats.MagicResist = 0, 0
	r := riftEliteRulesFixture(t)
	s.rift.data.Run.Rules = r
	s.rift.data.Run.RulesVersion = r.Version
	s.riftRules, _ = domain.NewRiftRuleTable(1, []domain.RiftRules{*riftRulesFixture(), *r}, lootRulesTableFixture(), balanceItemsFixture())
	s.rift.abilities = nil
	s.walkable = func(pos domain.Pos) bool {
		return pos.MapID == 7 && pos.X >= 0 && pos.Y >= 0 && pos.X < 3000 && pos.Y < 3000
	}
	m.Pos = domain.Pos{MapID: 7, X: 1000, Y: 1000}
	p.Pos = domain.Pos{MapID: 7, X: 1100, Y: 1000}
	m.Stats.MoveSpeed = 100
	s.rift.baseStats[m.ID] = m.Stats
	def := domain.MonsterDef{ID: 1, Name: "原怪", Kind: domain.MonsterElite, Sprite: 201, HP: m.MaxHP, Stats: m.Stats, AI: domain.MonsterAIProfile{ID: "native_caster"}, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}, InitialDelay: 7, Enabled: true}}, ImmuneSkills: map[domain.SkillID]struct{}{12001: {}}}
	s.defs = spawn.MapDefs{1: def}
	s.initializeMonster(m)
	s.registerEndgameRiftAbilities(m, 1, 0, 13, 1)
	state := s.rift.abilities[m.ID]
	state.pattern = []endgameRiftAbility{endgameRiftAbility(ability)}
	s.eliteSnapshot().Casters[state.eliteKey].Ready[ability] = *now
	return s, m, p, now
}
func TestRiftEliteSuccessfulIndependentCooldowns(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 3)
	state := s.rift.abilities[m.ID]
	state.pattern = []endgameRiftAbility{3, 4}
	c := s.eliteSnapshot().Casters[state.eliteKey]
	c.Ready[4] = *now
	m.Monster.CastingSkill = 12013
	m.Monster.CastingUntil = s.tick + 1
	s.stepEndgameRiftAbilities(*now)
	if len(s.eliteSnapshot().Effects) != 0 {
		t.Fatal("overlaid native cast")
	}
	m.Monster.CastingSkill = 0
	s.stepEndgameRiftAbilities(*now)
	if c.Ready[3] != *now+20000 || c.Ready[4] != *now || len(s.eliteSnapshot().Effects) != 1 {
		t.Fatal("cooldowns are coupled")
	}
	s.armRiftHazards(p, *now)
	*now += 650
	s.stepEndgameRiftAbilities(*now)
	if len(s.eliteSnapshot().Effects) != 2 || c.Ready[4] != *now+20000 {
		t.Fatal("second independent skill missing")
	}
	*now += 650
	if s.riftMonsterAbilityBusy(m.ID) {
		t.Fatal("persistent orb blocked native AI")
	}
}
func TestRiftEliteGoldenAndAgilityUseGenericBuckets(t *testing.T) {
	s, m, _, now := riftEliteFixture(t, 0)
	s.stepEndgameRiftAbilities(*now)
	if m.Status.DamageReductionPct(false) != 95 || m.Status.DamageReductionCapPct() != 95 || !m.Status.SuppressesHarmful() {
		t.Fatal("golden status semantics")
	}
	a := s.currentRiftRules().Abilities[5]
	for i := 0; i < 5; i++ {
		s.applyRiftEliteBuff(m, a)
	}
	if m.Stats.MoveSpeed != 200 {
		t.Fatal("agility compounded", m.Stats.MoveSpeed)
	}
}
func TestRiftEliteCloneNativeSkillsNoRewardsAndAtomicReplace(t *testing.T) {
	s, m, _, now := riftEliteFixture(t, 1)
	s.stepEndgameRiftAbilities(*now)
	if len(s.eliteSnapshot().Clones) != 3 {
		t.Fatal("missing three clones")
	}
	for id, c := range s.eliteSnapshot().Clones {
		clone := s.monsters[id]
		if c.Owner != m.ID || clone.MaxHP != 500 || clone.Stats.MaxAtk != m.Stats.MaxAtk || clone.Monster.SkillReady[12013] != s.tick+7 || clone.Monster.AI.ID != "native_caster" || clone.Monster.Summoner != m.ID || s.rift.data.Run.Monsters[id] != nil || s.rift.abilities[id] != nil {
			t.Fatal("bad native clone", clone)
		}
		clone.Monster.SkillCasts[12013] = 1
		if m.Monster.SkillCasts[12013] != 0 {
			t.Fatal("shared native runtime")
		}
	}
	s.walkable = func(domain.Pos) bool { return false }
	if s.spawnRiftClones(m, s.currentRiftRules().Abilities[1], *now+30000) || len(s.eliteSnapshot().Clones) != 3 {
		t.Fatal("failed replacement deleted old group")
	}
	*now += 20000
	s.stepRiftClones(*now)
	if len(s.eliteSnapshot().Clones) != 0 {
		t.Fatal("clone lifetime")
	}
}
func TestRiftEliteDeathLavaSurvivesArrivalAndIsExactlyOnce(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 2)
	s.rift.data.Run.Rules.GuardianArrivalMS = 5000
	m.HP = 0
	s.onEndgameRiftMonsterDeath(m, p)
	s.onEndgameRiftMonsterDeath(m, p)
	if len(s.eliteSnapshot().Effects) != 1 {
		t.Fatal("death did not create exactly one orb")
	}
	s.armRiftHazards(p, *now)
	p.Pos = m.Pos
	s.rift.data.Run.Phase = domain.RiftBossArriving
	s.clearRiftMonsters()
	if len(s.eliteSnapshot().Effects) != 1 {
		t.Fatal("arrival cleared death effect")
	}
	*now += 2999
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 10000 {
		t.Fatal("early explosion")
	}
	*now++
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 9840 || len(s.eliteSnapshot().Effects) != 0 {
		t.Fatal("death effect not resolved during arrival", p.HP)
	}
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 9840 {
		t.Fatal("replayed explosion")
	}
}
func TestRiftEliteLavaQueuedAcrossEconomicAggregate(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 2)
	old := s.rift.data.clone()
	s.rift.economicPending = true
	m.HP = 0
	s.onEndgameRiftMonsterDeath(m, p)
	if len(s.rift.delayedDeaths) != 1 || len(s.eliteSnapshot().Effects) != 0 {
		t.Fatal("mutated frozen aggregate")
	}
	s.rift.data = old
	s.rift.economicPending = false
	*now += 5000
	s.applyEndgameRiftDeath(s.rift.delayedDeaths[0])
	s.rift.delayedDeaths = nil
	if len(s.eliteSnapshot().Effects) != 1 {
		t.Fatal("lost authoritative death")
	}
	for _, h := range s.eliteSnapshot().Effects {
		if h.ExpiresMS != *now+3000 {
			t.Fatal("hidden warning shortened")
		}
	}
}
func TestRiftEliteLightningPulseDedupAndNoCatchup(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 3)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[3], *now)
	s.armRiftHazards(p, *now)
	start := *now
	for _, dt := range []int64{1000, 1100, 1200, 3000, 3100, 5000, 7000, 9000} {
		*now = start + dt
		s.stepRiftHazards(*now)
	}
	if p.HP != 9850 {
		t.Fatal("lightning did not hit once per ring", p.HP)
	}
	s, m, p, now = riftEliteFixture(t, 3)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[3], *now)
	s.armRiftHazards(p, *now)
	*now += 9500
	s.stepRiftHazards(*now)
	if p.HP < 9970 {
		t.Fatal("replayed missed rings", p.HP)
	}
}
func TestRiftElitePoisonTwentySecondsResetAndOverlap(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	a := s.currentRiftRules().Abilities[6]
	s.createRiftHazard(m, p.Pos, a, *now)
	s.createRiftHazard(m, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	for i := 0; i < 20; i++ {
		*now += 1000
		s.stepRiftHazards(*now)
	}
	if p.HP != 9755 || len(s.eliteSnapshot().Effects) != 0 {
		t.Fatal("poison duration/ramp/overlap", p.HP)
	}
	s, m, p, now = riftEliteFixture(t, 6)
	s.createRiftHazard(m, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	*now += 1000
	s.stepRiftHazards(*now)
	p.Pos.X += 121
	*now += 100
	s.stepRiftHazards(*now)
	p.Pos.X -= 121
	*now += 100
	s.stepRiftHazards(*now)
	*now += 1000
	s.stepRiftHazards(*now)
	if p.HP != 9990 {
		t.Fatal("leaving pool failed to reset", p.HP)
	}
}
func TestRiftEliteReconnectCannotReceiveHistoricalDamage(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now)
	s.armRiftHazards(p, *now)
	*now += 1000
	s.stepRiftHazards(*now)
	s.forgetRiftHazardPlayer(p.ID)
	*now += 4000
	s.stepRiftHazards(*now)
	if p.HP != 9995 {
		t.Fatal("unseen history damaged disconnected player")
	}
	s.armRiftHazards(p, *now)
	*now += 999
	s.stepRiftHazards(*now)
	if p.HP != 9995 {
		t.Fatal("reconnect warning bypassed")
	}
	*now++
	s.stepRiftHazards(*now)
	if p.HP != 9990 {
		t.Fatal("reconnect occupancy not fresh", p.HP)
	}
}
func TestRiftElitePersistenceAndAuthoritativeJSON(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 4)
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[4], *now)
	s.armRiftHazards(p, *now)
	record := s.rift.data.record(2)
	restored, ok := decodeEndgameRift(&record, s.riftRules)
	if !ok {
		t.Fatal("snapshot rejected")
	}
	if !reflect.DeepEqual(restored.Elite, s.rift.data.Elite) {
		t.Fatal("snapshot changed effects")
	}
	s.rift.data = restored
	s.restoreRiftElite()
	for _, h := range s.eliteSnapshot().Effects {
		if len(h.Viewers) != 0 || h.ExpiresMS != *now+3000 {
			t.Fatal("unsafe recovery")
		}
	}
	v := s.riftView(p, s.rift.data, true, true)
	if v.MapID != 7 || v.ServerNowMS != *now || len(v.Hazards) != 1 || v.Hazards[0].RemainingMS != int32(v.Hazards[0].ExpiresMS-v.ServerNowMS) {
		t.Fatal("bad JSON clock")
	}
}

func TestRiftEliteNativeGuardianPreservedAndWhitelistEnforced(t *testing.T) {
	s, plan := seededRiftLayoutFixture(t, 1)
	r := riftEliteRulesFixture(t)
	s.rift.data.Run.Rules = r
	s.rift.data.Run.RulesVersion = 6
	native := domain.MonsterDef{ID: 1011, Name: "刑天", Kind: domain.MonsterBoss, Sprite: 211, Level: 90, HP: 9999, Stats: domain.Stats{MaxHP: 9999, MinAtk: 700, MaxAtk: 800, MAtk: 900, MoveSpeed: 77, Def: 333}, AI: domain.MonsterAIProfile{ID: "native_xingtian"}, Skills: []domain.MonsterSkillRule{{Skill: domain.SkillDef{ID: 12013}, InitialDelay: 7, Enabled: true}}, ImmuneSkills: map[domain.SkillID]struct{}{12001: {}}}
	base := s.defs.(spawn.MapDefs)
	base[1011] = native
	plan.Guardian.Point.Monster = 1011
	s.rift.data.Floor = &plan
	s.rift.data.Run.Phase = domain.RiftBoss
	s.populateEndgameRift(true)
	if len(s.monsters) != 1 {
		t.Fatal("missing approved guardian")
	}
	for id, m := range s.monsters {
		d, _ := s.defs.Def(m.Monster.TypeID)
		if m.Name != native.Name || m.Look.ModelID != native.Sprite || m.Monster.AI.ID != native.AI.ID || !reflect.DeepEqual(d.Skills, native.Skills) || !reflect.DeepEqual(d.ImmuneSkills, native.ImmuneSkills) || m.Stats.Def != 333 || m.Stats.MoveSpeed != 115 {
			t.Fatal("native guardian changed", m, d)
		}
		if !reflect.DeepEqual(s.riftMonsterAbilityIDs(id), []int32{0, 3, 6}) {
			t.Fatal("guardian affixes changed")
		}
	}
	s.clearRiftMonsters()
	s.rift.data.Floor.Guardian.Point.Monster = 501
	s.populateEndgameRift(true)
	if len(s.monsters) != 0 || s.rift.data.Run.Phase != domain.RiftFailed {
		t.Fatal("ordinary guardian fallback")
	}
}
func TestRiftEliteWallAtomicLifetimeAndDeath(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 7)
	s.stepEndgameRiftAbilities(*now)
	if len(s.temporaryWalls) != 3 || len(s.eliteSnapshot().Effects) != 1 {
		t.Fatal("wall cast failed")
	}
	m.HP = 0
	s.onEndgameRiftMonsterDeath(m, p)
	if len(s.temporaryWalls) != 3 {
		t.Fatal("caster death removed wall")
	}
	*now += 15000
	s.stepEndgameRiftAbilities(*now)
	if len(s.temporaryWalls) != 0 || len(s.eliteSnapshot().Effects) != 0 {
		t.Fatal("wall expiration")
	}
}
func TestRiftEliteFreezeUsesResistanceAndImmunity(t *testing.T) {
	for _, immune := range []bool{false, true} {
		s, m, p, now := riftEliteFixture(t, 4)
		s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[4], *now)
		s.armRiftHazards(p, *now)
		if immune {
			s.applyStatusDef(p, domain.StatusDef{ID: 9999, Level: 1, TypeKnown: true, Type: domain.StatusGood, BlockHarmful: true}, p.ID)
		}
		*now += 3000
		s.stepRiftHazards(*now)
		if p.Status.Has(domain.StatusFreeze) == immune {
			t.Fatal("freeze immunity bypass", immune)
		}
	}
}
func TestRiftEliteWireFixture(t *testing.T) {
	path := os.Getenv("RIFT_ELITE_WIRE_FIXTURE")
	if path == "" {
		t.Skip("set RIFT_ELITE_WIRE_FIXTURE to regenerate authoritative parser fixture")
	}
	baseScene, baseMonster, basePlayer, baseNow := riftEliteFixture(t, 3)
	_, basePlan := seededRiftLayoutFixture(t, 1)
	basePlan.Entry = domain.Pos{MapID: 7, X: 100, Y: 100}
	basePlan.Exit = domain.Pos{MapID: 7, X: 2500, Y: 2500}
	basePlan.Spawns = nil
	baseScene.rift.data.Floor = &basePlan
	for _, id := range []int{2, 3, 4, 6, 7} {
		if !baseScene.createRiftHazard(baseMonster, basePlayer.Pos, baseScene.currentRiftRules().Abilities[id], *baseNow) {
			t.Fatal("basic fixture", id)
		}
	}
	baseResponse := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: baseScene.riftView(basePlayer, baseScene.rift.data, true, true)}
	writeRiftEliteFixture(t, path, baseResponse)
	path = strings.TrimSuffix(path, ".json") + "-metadata.json"
	s, m, p, now := riftEliteFixture(t, 3)
	_, plan := seededRiftLayoutFixture(t, 1)
	// Use a real generated, native-map floor identity while leaving a clear
	// test combat arena so all five hazard mechanisms fit in one fixture.
	plan.Entry = domain.Pos{MapID: 7, X: 100, Y: 100}
	plan.Exit = domain.Pos{MapID: 7, X: 2500, Y: 2500}
	plan.Spawns = nil
	s.rift.data.Floor = &plan
	state := s.rift.abilities[m.ID]
	state.pattern = []endgameRiftAbility{0}
	s.eliteSnapshot().Casters[state.eliteKey].Ready[0] = *now
	s.stepRiftElite(*now)
	if !s.spawnRiftClones(m, s.currentRiftRules().Abilities[1], *now) {
		t.Fatal("fixture clones")
	}
	for _, id := range []int{2, 3, 4, 6, 7} {
		if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[id], *now) {
			t.Fatal("fixture hazard", id)
		}
	}
	s.armRiftHazards(p, *now)
	*now += 3000
	s.stepRiftHazards(*now)
	for _, id := range []int{2, 4} {
		if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[id], *now) {
			t.Fatal("fixture replacement", id)
		}
	}
	response := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: s.riftView(p, s.rift.data, true, true)}
	writeRiftEliteFixture(t, path, response)
}
func writeRiftEliteFixture(t *testing.T, path string, response domain.EndgameResponse) {
	t.Helper()
	b, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wire := protocol.EncodeEndgame(response)
	if len(wire) == 0 {
		t.Fatal("empty wire encoding")
	}
	if err = os.WriteFile(strings.TrimSuffix(path, ".json")+".wire.hex", []byte(hex.EncodeToString(wire)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRiftElitePoisonStaggeredOverlapSharesOneCadence(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	a := s.currentRiftRules().Abilities[6]
	s.createRiftHazard(m, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	*now += 500
	m.Stats.MinAtk = 200
	m.Stats.MaxAtk = 200
	s.createRiftHazard(m, p.Pos, a, *now)
	s.armRiftHazards(p, *now)
	*now += 500
	s.stepRiftHazards(*now)
	if p.HP != 9995 {
		t.Fatal("first mature source", p.HP)
	}
	*now += 500
	s.stepRiftHazards(*now)
	if p.HP != 9995 {
		t.Fatal("staggered pools stacked", p.HP)
	}
	*now += 500
	s.stepRiftHazards(*now)
	if p.HP != 9985 {
		t.Fatal("strongest mature source not selected", p.HP)
	}
}

func TestRiftEliteConfirmedImpactsAndActiveAuraMetadata(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 0)
	s.stepRiftElite(*now)
	if s.riftGoldenUntil(m.ID, *now) != *now+5000 {
		t.Fatal("missing golden deadline")
	}
	if !s.spawnRiftClones(m, s.currentRiftRules().Abilities[1], *now) {
		t.Fatal("clones")
	}
	v := s.riftView(p, s.rift.data, true, true)
	n := 0
	for _, a := range v.Auras {
		if a.CloneOf != "" {
			n++
			if a.Class != 0 || len(a.AbilityIDs) != 0 || len(a.Affixes) != 0 {
				t.Fatal("clone inherited elite metadata")
			}
		}
	}
	if n != 3 {
		t.Fatal("clone markers", n)
	}
	s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[2], *now)
	s.armRiftHazards(p, *now)
	*now += 3000
	s.stepRiftHazards(*now)
	impacts := s.riftImpactViews(p, *now)
	if len(impacts) != 1 || impacts[0].AtMS != *now || impacts[0].ExpiresMS != *now+700 {
		t.Fatal("unconfirmed/missing burst receipt")
	}
	s.forgetRiftHazardPlayer(p.ID)
	if len(s.riftImpactViews(p, *now)) != 0 {
		t.Fatal("late connection replayed burst")
	}
}
func TestRiftEliteExpiredFreezeAndLavaCannotResumeHiddenDamage(t *testing.T) {
	for _, id := range []int32{2, 3, 4} {
		s, m, p, now := riftEliteFixture(t, id)
		s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[id], *now)
		s.armRiftHazards(p, *now)
		s.rift.economicPending = true
		s.stepRiftElite(*now)
		*now += 12000
		s.rift.economicPending = false
		s.stepRiftElite(*now)
		if p.HP != 10000 {
			t.Fatal("expired paused effect hit", id, p.HP)
		}
	}
}

func TestRiftEliteBudgetFailureRetriesWithoutFormalCooldown(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 3)
	a := s.currentRiftRules().Abilities[3]
	for i := int32(0); i < s.currentRiftRules().Elite.HazardMaxConcurrent; i++ {
		if !s.createRiftHazard(m, p.Pos, a, *now) {
			t.Fatal("fill budget")
		}
	}
	state := s.rift.abilities[m.ID]
	clock := s.eliteSnapshot().Casters[state.eliteKey]
	s.stepRiftElite(*now)
	if clock.Ready[3] != *now+1000 || clock.RecoverUntil != 0 || len(s.eliteSnapshot().Effects) != 6 {
		t.Fatal("budget failure consumed cast", clock)
	}
	for id := range s.eliteSnapshot().Effects {
		delete(s.eliteSnapshot().Effects, id)
		break
	}
	*now += 1000
	s.stepRiftElite(*now)
	if clock.Ready[3] != *now+20000 || len(s.eliteSnapshot().Effects) != 6 {
		t.Fatal("cast did not retry when capacity freed")
	}
}
func TestRiftElitePoisonPreviewDoesNotConsumeHPShieldOrRNG(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 6)
	s.applyStatusDef(p, domain.StatusDef{ID: 9997, Level: 1, Name: "shield", TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, p.ID)
	if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[6], *now) {
		t.Fatal("hazard")
	}
	var h *endgameRiftHazard
	for _, h = range s.eliteSnapshot().Effects {
		break
	}
	before := p.Status.Clone()
	hp := p.HP
	s.rng.Seed(93)
	want := s.rng.Intn(1000000)
	s.rng.Seed(93)
	for i := 0; i < 10; i++ {
		s.riftPoisonNominalDamage(h, p, 15)
	}
	if p.HP != hp || !reflect.DeepEqual(before, p.Status) || s.rng.Intn(1000000) != want {
		t.Fatal("preview consumed combat state")
	}
}
func TestRiftEliteTerminalCleanupRemovesAllTemporaryActors(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 7)
	if !s.spawnRiftClones(m, s.currentRiftRules().Abilities[1], *now) {
		t.Fatal("clones")
	}
	if !s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[7], *now) {
		t.Fatal("walls")
	}
	s.rift.data.Run.Phase = domain.RiftAborted
	s.stepRiftElite(*now)
	if len(s.eliteSnapshot().Effects) != 0 || len(s.eliteSnapshot().Clones) != 0 || len(s.temporaryWalls) != 0 {
		t.Fatal("terminal residue")
	}
}

func TestRiftEliteFatalImpactOnlyForWarnedDeadConnection(t *testing.T) {
	for _, change := range []string{"dead", "revived", "reconnected", "map"} {
		s, m, p, now := riftEliteFixture(t, 2)
		p.HP = 1
		s.createRiftHazard(m, p.Pos, s.currentRiftRules().Abilities[2], *now)
		s.armRiftHazards(p, *now)
		*now += 3000
		s.stepRiftHazards(*now)
		if p.Alive() || len(s.riftImpactViews(p, *now)) != 1 {
			t.Fatal("fatal victim lost confirmed explosion")
		}
		switch change {
		case "revived":
			p.HP = 100
		case "reconnected":
			s.forgetRiftHazardPlayer(p.ID)
		case "map":
			p.Pos.MapID++
		}
		if change != "dead" && len(s.riftImpactViews(p, *now)) != 0 {
			t.Fatal("replayed fatal receipt after life/context change", change)
		}
		if change == "dead" {
			*now += 700
			if len(s.riftImpactViews(p, *now)) != 0 {
				t.Fatal("expired fatal receipt")
			}
		}
	}
}
