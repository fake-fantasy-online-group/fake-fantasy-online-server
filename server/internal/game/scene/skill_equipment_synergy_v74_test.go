package scene

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// Keep the frozen V5 fixture intact; explicitly apply the reviewed74 data delta
// to this independent test input. The PostgreSQL test below verifies actual SQL.
func equipmentSynergyTestCatalog(t *testing.T) *domain.SkillProgressionCatalog {
	t.Helper()
	b, err := os.ReadFile("../../testfixture/catalog/skill_progression.json")
	if err != nil {
		t.Fatal(err)
	}
	var in domain.SkillProgressionData
	if err = json.Unmarshal(b, &in); err != nil {
		t.Fatal(err)
	}
	levels := map[domain.SkillID]int32{}
	for _, d := range in.Identities {
		levels[d.ID] = d.UnlockLevel
	}
	for i, e := range in.Synergies {
		if e.RankMode != "hard" {
			continue
		}
		e.RankMode = "effective_training"
		if e.Effect == "heal_flat" {
			e.Effect = "heal_power"
		}
		e.PerPointBP = 300
		if levels[e.Target] >= 30 {
			e.PerPointBP = 400
		}
		if levels[e.Target] >= 50 {
			e.PerPointBP = 500
		}
		if levels[e.Target] >= 70 {
			e.PerPointBP = 600
		}
		in.Synergies[i] = e
	}
	c, err := domain.NewSkillProgressionCatalog(in)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func equipmentSynergyTestScene(t *testing.T) (*Scene, *entity.Entity) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "装备协同", 100, 100)
	p := s.players[1]
	s.skills, s.statuses = realV5SkillFixtures(t)
	domain.AttachSkillProgression(s.skills, equipmentSynergyTestCatalog(t))
	p.Player.Char.Level = 100
	p.Player.Char.SkillAllocationVersion = 5
	p.Player.Char.SkillPoints = 95
	p.Player.Char.Race = domain.Warlock
	return s, p
}

func TestEquipmentSynergyAllEdgesUseRawNonrecursiveSourceRanks(t *testing.T) {
	s, p := equipmentSynergyTestScene(t)
	checked := 0
	for _, edge := range s.skills.ProgressionCatalog().Synergies() {
		if !edge.IsTraining() {
			continue
		}
		checked++
		t.Run(fmt.Sprintf("%d-to-%d/%s", edge.Source, edge.Target, edge.Effect), func(t *testing.T) {
			hard := domain.Learned{edge.Source: 1, edge.Target: 1}
			raw := domain.Learned{edge.Source: 23, edge.Target: 80}
			before := raw.Clone()
			rows, _ := s.skillSynergyViews(edge.Target, hard, raw)
			found := false
			for _, row := range rows {
				if row.SourceID == edge.Source && row.Effect == edge.Effect {
					found = true
					if row.SourceEffectiveRank != "23" || row.InvestedHardPoints != 1 || !row.EquipmentApplies || row.ContributionValue != int64(edge.PerPointBP)*23 {
						t.Fatal(row)
					}
				}
			}
			if !found {
				t.Fatal("edge missing")
			}
			a, _ := s.skills.Get(edge.Target, 1)
			s.applyTrainingSynergiesWithLevels(p, &a, hard, raw)
			baseline, _ := s.skills.Get(edge.Target, 1)
			emptySource := raw.Clone()
			delete(emptySource, edge.Source)
			s.applyTrainingSynergiesWithLevels(p, &baseline, hard, emptySource)
			changed := a.TrainingDamageBP != baseline.TrainingDamageBP || a.TrainingEliteDamageBP != baseline.TrainingEliteDamageBP || a.MPCost != baseline.MPCost || a.Radius != baseline.Radius || a.Area != baseline.Area || !reflect.DeepEqual(a.Effects, baseline.Effects) || !reflect.DeepEqual(a.Statuses, baseline.Statuses)
			if !changed {
				t.Fatal("effective source did not alter consumed payload", edge)
			}
			b, _ := s.skills.Get(edge.Target, 1)
			s.applyTrainingSynergiesWithLevels(p, &b, hard, raw)
			if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(before, raw) {
				t.Fatal("recursive or mutating resolution")
			}
		})
	}
	if checked != 70 {
		t.Fatal(checked)
	}
}

func TestEquipmentSynergyScopesGrantsAndNoPhantomSource(t *testing.T) {
	c := equipmentSynergyTestCatalog(t)
	for _, tc := range []struct {
		name           string
		prof           int32
		source, target domain.SkillID
		bonus          domain.SkillRankBonus
	}{
		{"all", 5, 10401, 10402, domain.SkillRankBonus{Scope: "all"}},
		{"class", 5, 10401, 10402, domain.SkillRankBonus{Scope: "class", Profession: 5}},
		{"left", 5, 10401, 10402, domain.SkillRankBonus{Scope: "branch", Profession: 5, Branch: "left"}},
		{"right", 5, 10409, 10410, domain.SkillRankBonus{Scope: "branch", Profession: 5, Branch: "right"}},
		{"aux", 3, 10213, 10214, domain.SkillRankBonus{Scope: "branch", Profession: 3, Branch: "aux"}},
		{"school", 5, 10409, 10410, domain.SkillRankBonus{Scope: "school", School: "frost"}},
		{"single", 5, 10401, 10402, domain.SkillRankBonus{Scope: "single", SkillID: 10401}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.bonus.SourceID = "equipped"
			tc.bonus.Levels = 7
			for _, learned := range []bool{false, true} {
				hard := domain.Learned{tc.target: 1}
				if learned {
					hard[tc.source] = 2
				}
				ranks, err := domain.ResolveSkillLoadout(tc.prof, hard, []domain.SkillRankBonus{tc.bonus}, nil, c)
				if err != nil {
					t.Fatal(err)
				}
				raw := domain.Learned{}
				for id, r := range ranks {
					if r.CanCast {
						raw[id], err = r.EngineRank()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				want := int32(0)
				if learned {
					want = 9
				}
				for _, e := range c.Synergies() {
					if e.Target == tc.target && e.Source == tc.source && e.IsTraining() {
						if e.SourceRank(hard, raw) != want {
							t.Fatal("phantom or missing gear source", e, raw)
						}
					}
				}
			}
		})
	}
	for _, allows := range []bool{false, true} {
		hard := domain.Learned{10402: 1}
		ranks, err := domain.ResolveSkillLoadout(5, hard, []domain.SkillRankBonus{{SourceID: "all", Scope: "all", Levels: 7}}, []domain.SkillGrant{{SourceID: "explicit", SkillID: 10401, Rank: 3, Origin: "test_explicit_grant", BenefitsFromSkillRanks: allows}}, c)
		if err != nil {
			t.Fatal(err)
		}
		want := "3"
		if allows {
			want = "10"
		}
		if ranks[10401].EffectiveRank != want || ranks[10401].HardRank != 0 || ranks[10409].CanCast {
			t.Fatal(ranks[10401])
		}
	}
}

func TestEquipmentSynergyLargeContributionAndPersistentSourceLoss(t *testing.T) {
	s, p := equipmentSynergyTestScene(t)
	hard := domain.Learned{10401: 1, 10402: 1}
	raw := domain.Learned{10401: 1, 10402: math.MaxInt32}
	rows, _ := s.skillSynergyViews(10401, hard, raw)
	if rows[0].SourceEffectiveRank != "2147483647" || rows[0].ContributionValue != 644245094100 {
		t.Fatal(rows)
	}
	def, _ := s.skills.Get(10401, 1)
	s.applyTrainingSynergiesWithLevels(p, &def, hard, raw)
	if def.TrainingDamageBP != math.MaxInt32 {
		t.Fatal("engine overflow", def.TrainingDamageBP)
	}
	p.Player.Char.Skills = domain.Learned{10401: 1, 10402: 2}
	cast, _ := s.skills.Get(10401, 1)
	cast.SnapshotHardRanks = hard
	cast.SnapshotRawRanks = domain.Learned{10401: 1, 10402: 20}
	next, ok := s.lesserPersistentSkill(p, cast)
	if !ok || next.TrainingDamageBP != 600 || next.SnapshotRawRanks[10402] != 2 {
		t.Fatal("lost equipment persisted", ok, next.TrainingDamageBP)
	}
	// New gear or ranks never upgrade the contribution of a previously accepted cast.
	cast.SnapshotRawRanks[10402] = 1
	next, ok = s.lesserPersistentSkill(p, cast)
	if !ok || next.TrainingDamageBP != 300 {
		t.Fatal("old cast upgraded", next.TrainingDamageBP)
	}
}

func TestEquipmentSynergyPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_EQUIPMENT_SYNERGY_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration74 directory required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing running cluster")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	skills, _, err := data.LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	statuses, _, err := data.LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(skills.ProgressionCatalog().Synergies(), equipmentSynergyTestCatalog(t).Synergies()) {
		t.Fatal("SQL delta differs")
	}
	s, p := equipmentSynergyTestScene(t)
	s.skills, s.statuses = skills, statuses
	ch := p.Player.Char
	ch.Skills = domain.Learned{10401: 3, 10402: 20, 10421: 1, 10425: 1, 10405: 1, 10408: 1}
	ch.SkillPoints = 72
	if err := domain.ValidateSkillAllocation(ch.Level, int32(ch.Race)+1, ch.Skills, ch.SkillPoints, s.skills.ProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	ch.Base = domain.Base{STR: 1000, INT: 1000, SPI: 1000, VIT: 1000, AGI: 1000, DEX: 1000}
	ch.Employed, ch.EmploymentKnown = true, true
	p.Player.EndgameSkillBehavior = true
	u, _ := domain.UniqueForBase(2018, catalogfixture.EndgameCatalog())
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 2018, Equip: &domain.EquipDef{Slot: int32(u.Slot), Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{2018: item}
	st := domain.NewStack(item, 1)
	st.UID = 7402018
	st.Durability, st.MaxDurability = 100, 100
	st.Endgame = domain.EndgameItemState{UniqueID: 2018, UniqueRollBP: 10000, UniqueContentVersion: 6}
	// Specific-source equipment raises the received synergy while target rank stays20.
	evaluate := func() (int32, domain.SkillRankView) {
		ranks, e := s.skillRanksForDisplay(p)
		if e != nil {
			t.Fatal(e)
		}
		r := ranks[10402]
		def, _ := s.skills.Get(10402, 20)
		s.applyResolvedSkillModifiers(p, &def, ch.Skills, s.rawSkillLevels(p))
		p.Stats.MAtk = 100
		p.Stats.MCritRate = 0
		target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 100000, MaxHP: 100000, Monster: &entity.Monster{}}
		s.entities[999] = target
		def.Statuses = nil
		for i := range def.Effects {
			def.Effects[i].GuaranteedHit = true
		}
		if !s.applySkill(p, target, def, 0, false) {
			t.Fatal("real damage failed")
		}
		return target.MaxHP - target.HP, r
	}
	base, before := evaluate()
	p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
	equipped, view := evaluate()
	if before.EffectiveRank != "20" || view.EffectiveRank != "20" || view.TrainingSynergies[0].SourceEffectiveRank != "5" || view.TrainingSynergies[0].ContributionValue != 1500 || equipped <= base {
		t.Fatal(base, equipped, before, view)
	}
	ranks, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	response := domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: []domain.SkillRankView{ranks[10402], ranks[10405], ranks[10408], ranks[10410]}, Result: map[string]string{"formulaVersion": "5", "hardCap": "20", "playerCap": "100", "petCap": "80", "unspentPoints": fmt.Sprint(ch.SkillPoints)}}
	if out := os.Getenv("SKILL_EQUIPMENT_SYNERGY_FIXTURE_PATH"); out != "" {
		b, e := json.Marshal(response)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out, b, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(out+".wire.hex", []byte(hex.EncodeToString(protocol.EncodeEndgame(response))), 0600); e != nil {
			t.Fatal(e)
		}
	}
	st.Durability = 0
	p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
	broken, bv := evaluate()
	if broken != base || bv.TrainingSynergies[0].SourceEffectiveRank != "3" {
		t.Fatal("broken equipment retained synergy", broken, base)
	}
	p.Player.Worn = domain.NewEquipSet()
	removed, rv := evaluate()
	if removed != base || rv.TrainingSynergies[0].ContributionValue != 900 || ch.Skills[10401] != 3 {
		t.Fatal("unequip changed allocation or retained bonus", removed, base, rv)
	}
	t.Logf("actual migration74 LoadSkills + specific-source equipment: damage %d -> %d -> %d; source3 ->5 ->3, beneficiary remains20", base, equipped, removed)
}

func TestEquipmentSynergyLayeredEconomyKeepsPositiveManaFloor(t *testing.T) {
	s, p := equipmentSynergyTestScene(t)
	p.Player.Char.Race = domain.Race(0)
	p.Player.Char.Base = domain.Base{STR: 1000, INT: 1000, SPI: 1000, VIT: 1000, AGI: 1000, DEX: 1000}
	p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
	u, ok := domain.UniqueForBase(2704, catalogfixture.EndgameCatalog())
	if !ok {
		t.Fatal("missing actual economy gear")
	}
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 2704, Equip: &domain.EquipDef{Slot: int32(u.Slot), Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{2704: item}
	st := domain.NewStack(item, 1)
	st.UID = 742704
	st.Durability, st.MaxDurability = 100, 100
	st.Endgame = domain.EndgameItemState{UniqueID: 2704, UniqueRollBP: 10000, UniqueContentVersion: 6}
	p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
	hard := domain.Learned{10002: 1, 10013: 20}
	raw := domain.Learned{10002: 1, 10013: 27}
	p.Player.Char.Skills = hard
	def, ok := s.skills.Get(10002, 1)
	if !ok || def.MPCost != 8 {
		t.Fatal(def.MPCost, ok)
	}
	s.applyResolvedSkillModifiers(p, &def, hard, raw)
	if def.MPCost != 1 || s.skillBehaviorPayload(p, def).MPCost != 1 {
		t.Fatal("layered economy made a positive cost free", def.MPCost)
	}
	free, _ := s.skills.Get(10002, 1)
	free.MPCost = 0
	s.applyResolvedSkillModifiers(p, &free, hard, raw)
	if free.MPCost != 0 {
		t.Fatal("genuinely free skill gained a cost", free.MPCost)
	}
}

func TestEquipmentSynergyExplicitTreasureGrantWithoutHardSkills(t *testing.T) {
	s, p := equipmentSynergyTestScene(t)
	p.Player.Char.Skills = domain.Learned{}
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 1999999999, Equip: &domain.EquipDef{Slot: int32(domain.SlotTreasure), Durable: 100, Skill: &domain.EquipmentSkill{ID: 10401}}}
	s.items = map[domain.ItemID]domain.ItemDef{item.ID: item}
	st := domain.NewStack(item, 1)
	st.UID = 741234
	st.Durability, st.MaxDurability = 100, 100
	p.Player.Worn.Set(domain.SlotTreasure, st)
	raw := s.rawSkillLevels(p)
	if raw[10401] != 1 || raw[10409] != 0 {
		t.Fatal("explicit grant was lost or phantom skill added", raw)
	}
	rows, _ := s.skillSynergyViews(10402, p.Player.Char.Skills, raw)
	if rows[0].InvestedHardPoints != 0 || rows[0].SourceEffectiveRank != "1" || rows[0].ContributionValue != 300 {
		t.Fatal(rows)
	}
	p.Player.Worn = domain.NewEquipSet()
	raw = s.rawSkillLevels(p)
	if raw[10401] != 0 {
		t.Fatal("removed grant survived", raw)
	}
}
