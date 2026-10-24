package scene

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

// nativeRepairTables is a bounded, source-backed scene fixture, NOT a database
// loader test. It preserves the authored costs, windups, projectiles, geometry,
// damage and controls. The old snapshots need explicit migration corrections:
// 62 status restoration/source metadata, 66 scoped controls, 68 radii, 69
// Gangqi knockback, 70 theft, 71 Exorcism families, 73 Backstab stun and 74
// equipment-inclusive synergies. Later migrations through 104 do not replace
// these skill mechanics. PostgreSQL-dependent tests remain separate.
func nativeRepairTables(t *testing.T) (domain.SkillTable, domain.StatusTable) {
	t.Helper()
	skills, statuses := nativeRestoredSceneFixtures(t)
	raw, err := testfixture.ReadMigrationJSON("000000_init-62.sql", "$skill_config$")
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		States       []domain.StatusDef
		Applications []struct {
			SkillID domain.SkillID
			Rank    int32
			App     domain.StatusApplication
		}
	}
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, d := range restored.States {
		statuses[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	apps := map[domain.SkillKey][]domain.StatusApplication{}
	for _, row := range restored.Applications {
		row.App.SourceSkill, row.App.ChanceExplicit = row.SkillID, true
		key := domain.SkillKey{ID: row.SkillID, Level: row.Rank}
		apps[key] = append(apps[key], row.App)
	}
	for key, values := range apps {
		d := skills[key]
		d.Statuses = values
		skills[key] = d
	}
	for rank := int32(1); rank <= 5; rank++ {
		for id, radius := range map[domain.SkillID]int32{10028: 100, 10127: 100, 10313: 150} {
			key := domain.SkillKey{ID: id, Level: rank}
			d := skills[key]
			d.Radius, d.Area.Radius = radius, radius
			skills[key] = d
		}
		key := domain.SkillKey{ID: 10102, Level: rank}
		d := skills[key]
		d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectKnockback, KnockbackDistance: 100, ChanceBP: 10000, ChanceExplicit: true, RequiresPreviousLanded: true}}
		skills[key] = d
		key = domain.SkillKey{ID: 10212, Level: rank}
		d = skills[key]
		d.Name = "偷窃术"
		d.Effects = []domain.SkillEffect{{Kind: domain.EffectStealItem, ChanceBP: 5000 + (rank-1)*600, ChanceExplicit: true}}
		d.Effect = d.Effects[0]
		skills[key] = d
		key = domain.SkillKey{ID: 10312, Level: rank}
		d = skills[key]
		d.TargetFamilies = []domain.MonsterFamily{domain.MonsterFamilyUndead, domain.MonsterFamilyDemon}
		skills[key] = d
		key = domain.SkillKey{ID: 10201, Level: rank}
		d = skills[key]
		d.PassiveTriggers = []domain.PassiveTriggerStatus{{TargetSkill: 10209, Status: 1002, StatusLevel: 27, ChanceBP: 10000, ChanceExplicit: true, SourceSkill: 10201, DurationSec: 3}}
		skills[key] = d
		key = domain.SkillKey{ID: 10317, Level: rank}
		d = skills[key]
		d.Statuses = []domain.StatusApplication{
			{ID: 1008, Level: rank + 1, ChanceBP: 10000, ChanceExplicit: true, SourceSkill: 10317, DurationSec: 60, TargetType: domain.SkillStatusTargetNonPlayer},
			{ID: 1008, Level: rank + 20, ChanceBP: 10000, ChanceExplicit: true, SourceSkill: 10317, DurationSec: 8, TargetType: domain.SkillStatusTargetPlayer},
		}
		skills[key] = d
		statuses[domain.StatusKey{ID: 1008, Level: rank + 20}] = domain.StatusDef{ID: 1008, Level: rank + 20, Name: "致盲", Type: domain.StatusBad, TypeKnown: true, DurationSec: 8, Affixes: []domain.Affix{{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: -(8 + rank*2)}}}
		key = domain.SkillKey{ID: 10326, Level: rank}
		d = skills[key]
		d.Statuses = []domain.StatusApplication{
			{ID: 1034, Level: rank + 30, ChanceBP: 3000, ChanceExplicit: true, SourceSkill: 10326, DurationSec: 5 + rank, TargetType: domain.SkillStatusTargetNonPlayer},
			{ID: 1034, Level: 41, ChanceBP: 1000, ChanceExplicit: true, SourceSkill: 10326, DurationSec: 3, TargetType: domain.SkillStatusTargetPlayer},
		}
		skills[key] = d
		statuses[domain.StatusKey{ID: 1034, Level: rank + 30}] = domain.StatusDef{ID: 1034, Level: rank + 30, Name: "定身", Type: domain.StatusBad, TypeKnown: true, DurationSec: 5 + rank, BlockMove: true, Immobile: 1}
	}
	statuses[domain.StatusKey{ID: 1034, Level: 41}] = domain.StatusDef{ID: 1034, Level: 41, Name: "定身", Type: domain.StatusBad, TypeKnown: true, DurationSec: 3, BlockMove: true, Immobile: 1}
	statuses[domain.StatusKey{ID: 1002, Level: 27}] = domain.StatusDef{ID: 1002, Level: 27, Name: "昏迷", Type: domain.StatusBad, TypeKnown: true, DurationSec: 3, Control: domain.ControlStun}
	for key, d := range skills {
		for i := range d.Statuses {
			if !d.Statuses[i].ChanceExplicit && d.Statuses[i].ChanceBP == 0 {
				d.Statuses[i].ChanceBP = 10000
			}
			d.Statuses[i].SourceSkill = d.ID
			d.Statuses[i].ChanceExplicit = true
		}
		skills[key] = d
	}
	// Use a fresh catalog rather than mutate the shared historical fixture.
	raw, err = os.ReadFile("../../testfixture/catalog/skill_progression.json")
	if err != nil {
		t.Fatal(err)
	}
	var progression domain.SkillProgressionData
	if err = json.Unmarshal(raw, &progression); err != nil {
		t.Fatal(err)
	}
	for i, e := range progression.Synergies {
		if e.RankMode != "hard" {
			continue
		}
		e.RankMode = "effective_training"
		if e.Effect == "heal_flat" {
			e.Effect = "heal_power"
		}
		unlock := skills[domain.SkillKey{ID: e.Target, Level: 1}].LevelNeed
		e.PerPointBP = 300
		if unlock >= 30 {
			e.PerPointBP = 400
		}
		if unlock >= 50 {
			e.PerPointBP = 500
		}
		if unlock >= 70 {
			e.PerPointBP = 600
		}
		progression.Synergies[i] = e
	}
	policy := progression.Policies[10212]
	policy.EfficiencyCapRank = 10
	progression.Policies[10212] = policy
	catalog, err := domain.NewSkillProgressionCatalog(progression)
	if err != nil {
		t.Fatal(err)
	}
	if err = domain.InstallV5SkillProgression(skills, catalog); err != nil {
		t.Fatal(err)
	}
	return skills, statuses
}

func nativeRepairFixture(t *testing.T, race domain.Race, learned domain.Learned, powers ...string) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s := newCombatScene(t)
	s.skills, s.statuses = nativeRepairTables(t)
	s.alloc = domain.NewEntityAlloc()
	join(t, s, 1, 100, "native combination", 100, 100)
	p := s.players[1]
	p.Player.Char.Race, p.Player.Char.Level = race, 100
	p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Skills = learned
	p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 100000}
	s.items = map[domain.ItemID]domain.ItemDef{}
	nativeRepairEquip(t, s, p, powers...)
	m := putMonster(s, 900, 100000, domain.Stats{}, 130, 100)
	m.Status = domain.NewStatusSet()
	s.monsters[m.ID] = m
	return s, p, m
}

func nativeRepairEquip(t *testing.T, s *Scene, p *entity.Entity, powers ...string) {
	t.Helper()
	slots := []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead, domain.SlotRing, domain.SlotNeck, domain.SlotGlove, domain.SlotFace, domain.SlotBag}
	// Utility class powers have only a weapon slot; allocate restrictive powers first.
	ordered := append([]string(nil), powers...)
	for i, id := range ordered {
		if id[:2] != "TY" && id[3] == 'U' {
			ordered[0], ordered[i] = ordered[i], ordered[0]
		}
	}
	for i, id := range ordered {
		power, ok := domain.LegendaryPower(id)
		if !ok {
			t.Fatal("unknown power", id)
		}
		found := false
		for _, slot := range slots {
			if p.Player.Worn.At(slot).Item != 0 {
				continue
			}
			item := domain.ItemID(970000 + i)
			d := domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1 << uint8(p.Player.Char.Race)}}}
			if !domain.LegendaryPowerEligible(power, d) {
				continue
			}
			s.items[item] = d
			p.Player.Worn.Set(slot, domain.Stack{UID: int64(item), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
			found = true
			break
		}
		if !found {
			t.Fatal("no eligible free slot for combination", powers, id)
		}
	}
	s.invalidateCombatAttributes(p)
	if got := domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char); len(got) != len(powers) {
		t.Fatal("inactive combination", powers, got)
	}
}

// No SkillDef fields are rewritten to speed up the test. Advance the normal
// wall-clock settlement owner twice to cover native windup plus projectile time.
func nativeRepairCast(t *testing.T, s *Scene, p *entity.Entity, skill domain.SkillID, target domain.EntityID) {
	t.Helper()
	s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: target})
	if len(s.casts) == 0 {
		t.Fatalf("native cast %d rejected: %+v", skill, s.outbox)
	}
	now := s.now()
	s.stepCastsAt(now.Add(time.Minute), false)
	s.stepCastsAt(now.Add(2*time.Minute), false)
	if len(s.casts) != 0 {
		t.Fatalf("native cast %d did not finish", skill)
	}
}

func TestLegendaryRepairNativeFixtureProvenance(t *testing.T) {
	skills, statuses := nativeRepairTables(t)
	for _, id := range []domain.SkillID{10009, 10206, 10213, 10301, 10317, 10321, 10402, 10428} {
		d, ok := skills.Get(id, 1)
		if !ok || d.FormulaVersion != domain.SkillFormulaVersion || d.Progression == nil {
			t.Fatal("missing V5 native definition", id)
		}
		for _, a := range d.Statuses {
			if a.SourceSkill != id || !a.ChanceExplicit {
				t.Fatal("missing native source/chance metadata", id, a)
			}
		}
	}
	if d := statuses[domain.StatusKey{ID: 10009, Level: 1}]; len(d.Affixes) == 0 || d.Affixes[0].Attr != domain.AttrDef || d.Affixes[0].Value >= 0 {
		t.Fatal("armor fixture is not native defense reduction", d)
	}
	if d := statuses[domain.StatusKey{ID: 10321, Level: 1}]; d.PhysicalDamageReductionPct != 10 || d.MagicDamageReductionPct != 10 {
		t.Fatal("defense fixture predates migration 62", d)
	}
	if d, _ := skills.Get(10127, 1); d.Dist != 0 || d.Radius != 100 {
		t.Fatal("taunt range fixture is synthetic", d)
	}
}
