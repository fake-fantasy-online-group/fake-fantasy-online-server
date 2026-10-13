package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestV5TrainingGraphHasActualAttackChainsAcrossFiveProfessions(t *testing.T) {
	skills, _ := realV5SkillFixtures(t)
	attacks := map[int32]int{}
	targets := map[domain.SkillID]bool{}
	training, native := 0, 0
	for _, e := range catalogfixture.SkillProgressionCatalog().Synergies() {
		if e.RankMode != "hard" {
			native++
			continue
		}
		training++
		targets[e.Target] = true
		source, _ := skills.Get(e.Source, 1)
		target, _ := skills.Get(e.Target, 1)
		if !source.Kind.Active() || !target.Kind.Active() {
			t.Fatalf("training is not active to active: %+v", e)
		}
		if source.HasDamageEffect() && target.HasDamageEffect() && (e.Effect == "damage" || e.Effect == "elite_damage") {
			attacks[target.Prof]++
		}
	}
	if training != 70 || native != 58 || len(targets) != 56 {
		t.Fatal(training, native, len(targets))
	}
	for prof := int32(1); prof <= 5; prof++ {
		if attacks[prof] < 5 {
			t.Fatalf("profession%d missing meaningful attack training: %d", prof, attacks[prof])
		}
	}
	if err := domain.ValidateSkillSynergies(catalogfixture.SkillProgressionCatalog().Synergies(), catalogfixture.SkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
}

func TestV5EveryDamageTrainingEdgeChangesActualTargetHP(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, edge := range catalogfixture.SkillProgressionCatalog().Synergies() {
		if edge.RankMode != "hard" || (edge.Effect != "damage" && edge.Effect != "elite_damage") {
			continue
		}
		t.Run(fmt.Sprintf("%d-to-%d", edge.Source, edge.Target), func(t *testing.T) {
			strike := func(hard int32) int32 {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "训练", 100, 100)
				p := s.players[1]
				ident, _ := domain.V5SkillIdentity(edge.Target, catalogfixture.SkillProgressionCatalog())
				p.Player.Char.Race = domain.Race(ident.Profession - 1)
				p.Player.Char.Level = 100
				p.Player.Char.Skills = domain.Learned{edge.Target: 20, edge.Source: hard}
				p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk = 10000, 10000, 10000
				p.Stats.CritRate, p.Stats.MCritRate = 0, 0
				s.skills, s.statuses = skills, statuses
				s.rng = rand.New(rand.NewSource(7))
				def, _ := skills.Get(edge.Target, 20)
				s.applyTrainingSynergies(p, &def)
				s.applyPassiveSkillModifiers(p, &def)
				// Isolate the damage lane from RNG-only side effects; execute the real
				// scene applySkill/resolveStrikeStatuses/HP consumer with real curves.
				var damage []domain.SkillEffect
				for _, e := range def.DirectEffects() {
					if e.Kind == domain.EffectDamage {
						e.GuaranteedHit = true
						damage = append(damage, e)
					}
				}
				def.Effects, def.Statuses = damage, nil
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 2000000, MaxHP: 2000000, Monster: &entity.Monster{Kind: domain.MonsterBoss}}
				s.entities[target.ID] = target
				if !s.applySkill(p, target, def, 0, false) {
					t.Fatal("real strike failed")
				}
				return target.MaxHP - target.HP
			}
			base, trained := strike(0), strike(20)
			if base <= 0 || trained <= base {
				t.Fatalf("source hardpoints never changed actual damage: base%d trained%d edge%+v", base, trained, edge)
			}
		})
	}
}

func TestV5TrainingSynergyRowsAreAuthoritativeIncludingUnlearnedSources(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "术士", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race, ch.Level, ch.SkillAllocationVersion, ch.SkillPoints = domain.Warlock, 100, 5, 98
	ch.Skills = domain.Learned{10401: 1}
	s.skills, s.statuses = skills, statuses
	views, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	row := views[10401]
	if len(row.TrainingSynergies) != 1 {
		t.Fatal(row)
	}
	x := row.TrainingSynergies[0]
	if x.SourceID != 10402 || x.SourceName != "火球术" || x.PerPointValue != 100 || x.ContributionValue != 0 || x.InvestedHardPoints != 0 || x.EquipmentApplies || x.Unit != "percent_bp" {
		t.Fatal(x)
	}
	if views[10410].TrainingSynergies[0].SourceID != 10409 || views[10410].CanCast {
		t.Fatal("unlearned target lost planning row")
	}
	if strings.Contains(strings.Join(row.CurrentEffectLines, ""), "有效等级") || row.NextEffectiveRank != "2" || row.NextRequiredLevel != 11 {
		t.Fatal(row)
	}
	if !strings.HasPrefix(row.CurrentEffectLines[len(row.CurrentEffectLines)-3], "消耗: ") || !strings.HasPrefix(row.CurrentEffectLines[len(row.CurrentEffectLines)-2], "技能吟唱: ") || !strings.HasPrefix(row.CurrentEffectLines[len(row.CurrentEffectLines)-1], "技能间隔: ") {
		t.Fatal(row.CurrentEffectLines)
	}
	ch.Skills[10402] = 3
	ch.Skills[10426] = 2
	ch.SkillPoints = 93
	views, err = s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	row = views[10401]
	if x = row.TrainingSynergies[0]; x.InvestedHardPoints != 3 || x.ContributionValue != 300 {
		t.Fatal(x)
	}
	if !strings.Contains(strings.Join(row.NativeModifierLines, ""), "已学被动使本技能伤害提升8%") {
		t.Fatal(row.NativeModifierLines)
	}
	before := ch.Skills.Clone()
	for i := 0; i < 3; i++ {
		if _, err = s.skillRanksForDisplay(p); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, ch.Skills) {
		t.Fatal("display changed hard allocation")
	}
	response := domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: []domain.SkillRankView{row, views[10410]}, Result: map[string]string{"formulaVersion": "5", "hardCap": "20", "playerCap": "100", "petCap": "80", "unspentPoints": fmt.Sprint(ch.SkillPoints)}}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if protocol.EncodeEndgame(response) == nil {
		t.Fatal("production encoder rejected DTO")
	}
	if path := os.Getenv("SKILL_SYNERGY_FIXTURE_PATH"); path != "" {
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(strings.TrimSuffix(path, ".json")+".wire.hex", []byte(hex.EncodeToString(protocol.EncodeEndgame(response))), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestV5TrainingViewsIgnoreSourceSoftRanksAndKeepNativeDistinct(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	hard := domain.Learned{10401: 20, 10402: 3, 10426: 2}
	a, na := s.skillSynergyViews(10401, hard, hard)
	raw := hard.Clone()
	raw[10402] = 100
	raw[10426] = 7
	b, nb := s.skillSynergyViews(10401, hard, raw)
	if !reflect.DeepEqual(a, b) || a[0].ContributionValue != 300 {
		t.Fatal("source softpoints multiplied training", a, b)
	}
	if reflect.DeepEqual(na, nb) {
		t.Fatal("native effective-rank modifier stopped accepting soft levels")
	}
	ident, _ := domain.V5SkillIdentity(10401, catalogfixture.SkillProgressionCatalog())
	bonus := []domain.SkillRankBonus{{SourceID: "same", Scope: "single", SkillID: 10401, Levels: 3}, {SourceID: "same", Scope: "single", SkillID: 10401, Levels: 3}}
	if next := firstSkillInvestmentRank("1", ident, bonus); next.String() != "5" {
		t.Fatal("first hardpoint did not enable eligible equipment exactly once", next)
	}
	grants := []domain.SkillGrant{{SourceID: "treasure", SkillID: 10401, Rank: 1, Origin: "treasure_grant", BenefitsFromSkillRanks: false}}
	before, err := domain.ResolveSkillLoadout(5, domain.Learned{}, bonus, grants, catalogfixture.SkillProgressionCatalog())
	if err != nil {
		t.Fatal(err)
	}
	after, err := domain.ResolveSkillLoadout(5, domain.Learned{10401: 1}, bonus, grants, catalogfixture.SkillProgressionCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if before[10401].EffectiveRank != "1" || before[10401].SoftRank != "0" || after[10401].EffectiveRank != "5" || firstSkillInvestmentRank(before[10401].GrantRank, ident, bonus).String() != after[10401].EffectiveRank {
		t.Fatal("first-grant preview diverged from actual loadout resolution", before[10401], after[10401])
	}

}

func TestV5TrainingAttackSynergiesThroughAcceptedCasts(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, pair := range [][2]domain.SkillID{{10009, 10002}, {10111, 10102}, {10221, 10205}, {10317, 10319}, {10410, 10409}} {
		t.Run(fmt.Sprint(pair[0]), func(t *testing.T) {
			cast := func(h int32) int32 {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "施术", 100, 100)
				p := s.players[1]
				ident, _ := domain.V5SkillIdentity(pair[0], catalogfixture.SkillProgressionCatalog())
				p.Player.Char.Race = domain.Race(ident.Profession - 1)
				p.Player.Char.Level = 100
				p.Player.Char.SkillAllocationVersion = 5
				p.Player.Char.Skills = domain.Learned{pair[0]: 20, pair[1]: h}
				p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk, p.Stats.Hit = 10000, 10000, 10000, 1000000
				p.Stats.CritRate, p.Stats.MCritRate = 0, 0
				p.MP, p.MaxMP = 100000, 100000
				s.skills, s.statuses = skills, statuses
				s.rng = rand.New(rand.NewSource(7))
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, Pos: domain.Pos{MapID: p.Pos.MapID, X: 110, Y: 100}, HP: 2000000, MaxHP: 2000000, Monster: &entity.Monster{Kind: domain.MonsterBoss}}
				s.entities[target.ID] = target
				s.monsters[target.ID] = target
				s.aoi.Enter(target)
				s.onUseSkill(UseSkill{ID: p.ID, Skill: pair[0], Target: target.ID})
				if len(s.casts) != 1 || p.MP >= 100000 {
					t.Fatal("cast rejected or did not consume MP", len(s.casts), p.MP)
				}
				s.stepCastsAt(s.now().Add(time.Minute), false)
				s.stepCastsAt(s.now().Add(2*time.Minute), false)
				if len(s.casts) != 0 {
					t.Fatal("cast never resolved")
				}
				return target.MaxHP - target.HP
			}
			base, trained := cast(0), cast(20)
			if base <= 0 || trained <= base {
				t.Fatalf("accepted cast has no synergy effect: %d -> %d", base, trained)
			}
		})
	}
}

func TestV5All149ReadableSkillViewsFixture(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	all := []domain.SkillRankView{}
	for prof := int32(1); prof <= 5; prof++ {
		s := newTestScene(t, nil)
		join(t, s, 1, 100, "面板", 100, 100)
		p := s.players[1]
		ch := p.Player.Char
		ch.Race, ch.Level, ch.SkillAllocationVersion = domain.Race(prof-1), 100, 5
		ch.Skills = domain.Learned{}
		ch.SkillPoints = 99
		for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
			ident, _ := domain.V5SkillIdentity(id, catalogfixture.SkillProgressionCatalog())
			if ident.Profession == prof {
				ch.Skills[id] = 3
				ch.SkillPoints -= 3
			}
		}
		s.skills, s.statuses = skills, statuses
		views, err := s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
			if v, ok := views[id]; ok {
				all = append(all, v)
			}
		}
	}
	if len(all) != 149 {
		t.Fatal(len(all))
	}
	for _, row := range all {
		for _, line := range append(append([]string{}, row.CurrentEffectLines...), row.NextEffectLines...) {
			if strings.Contains(line, "固定伤害+0") || strings.Contains(line, "长0") || strings.Contains(line, "宽0") || strings.Contains(line, "角度0") {
				t.Fatal("debug/zero clutter", row.ID, line)
			}
		}
	}
	if path := os.Getenv("SKILL_ALL_VIEWS_FIXTURE_PATH"); path != "" {
		data, err := json.Marshal(all)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestV5PassiveTooltipGroupsExactTargetMembershipWithoutLoss(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for _, id := range []domain.SkillID{10001, 10101} {
		def, _ := skills.Get(id, 3)
		lines := s.skillEffectLines(def)
		if len(lines) != 2 {
			t.Fatal("weapon mastery should share its target list once", id, lines)
		}
		text := strings.Join(lines, "\n")
		seen := map[domain.SkillID]bool{}
		for _, m := range def.PassiveModifiers {
			if !seen[m.TargetSkill] && strings.Count(text, skillTargetName(m.TargetSkill, catalogfixture.SkillProgressionCatalog())) != 1 {
				t.Fatal("lost or duplicated exact target", id, m.TargetSkill, lines)
			}
			seen[m.TargetSkill] = true
			if !strings.Contains(text, skillPassiveModifierText(m)) {
				t.Fatal("lost modifier", id, m, lines)
			}
		}
	}
}

func TestV5EquippedSourceSoftPointDoesNotIncreaseTrainingDamage(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "刺客", 100, 100)
	p := s.players[1]
	p.Player.Char.Race, p.Player.Char.Level = domain.Assassin, 100
	p.Player.Char.Skills = domain.Learned{10205: 20, 10221: 3}
	p.Stats.MinAtk, p.Stats.MaxAtk = 10000, 10000
	p.Stats.CritRate = 0
	s.skills, s.statuses = skills, statuses
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 1905, Equip: &domain.EquipDef{Slot: 13, Type: 32, Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{1905: item}
	st := domain.NewStack(item, 1)
	st.UID, st.Durability, st.MaxDurability = 19050001, 100, 100
	// Keep this historical V5 contract independent of the current item catalog.
	st.Endgame = domain.EndgameItemState{UniqueID: 1905, UniqueRollBP: 10000, UniqueContentVersion: 5}
	strike := func() int32 {
		rank, _ := s.skillLevelForUse(p, 10205)
		if rank != 20 {
			t.Fatal("fixture changed target rank", rank)
		}
		def, _ := skills.Get(10205, rank)
		s.applyTrainingSynergies(p, &def)
		s.applyPassiveSkillModifiers(p, &def)
		for i := range def.Effects {
			if def.Effects[i].Kind == domain.EffectDamage {
				def.Effects[i].GuaranteedHit = true
			}
		}
		target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 2000000, MaxHP: 2000000, Monster: &entity.Monster{}}
		s.entities[target.ID] = target
		if !s.applySkill(p, target, def, 0, false) {
			t.Fatal("strike failed")
		}
		return target.MaxHP - target.HP
	}
	base := strike()
	p.Player.Worn.Set(domain.SlotTwoHand, st)
	if rank, _ := s.skillLevelForUse(p, 10221); rank != 4 {
		t.Fatal("equipment did not provide source soft rank", rank)
	}
	if equipped := strike(); equipped != base {
		t.Fatal("equipment source rank multiplied training", base, equipped)
	}
	views, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	if row := views[10205].TrainingSynergies[0]; row.SourceID != 10221 || row.InvestedHardPoints != 3 || row.ContributionValue != 300 {
		t.Fatal(row)
	}
	p.Player.Worn.Set(domain.SlotTwoHand, domain.Stack{})
	if removed := strike(); removed != base {
		t.Fatal("source removal retained soft-point training", removed, base)
	}
	p.Player.Char.Skills[10221] = 4
	if invested := strike(); invested <= base {
		t.Fatal("actual fourth hardpoint gave no damage", invested, base)
	}
}

func TestV5StatusTooltipNamesAndDurationsAreDeterministic(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for i := 0; i < 100; i++ {
		if got := s.skillStatusName(1038); got != "冰之盾" {
			t.Fatal("dispel picked unrelated state level", got)
		}
	}
	for _, id := range []domain.SkillID{10012, 10019, 10110, 10423, 10427, 10430} {
		def, _ := skills.Get(id, 3)
		for _, line := range s.skillEffectLines(def) {
			if strings.Contains(line, "持续0秒") {
				t.Fatal("table duration missing", id, line)
			}
		}
		for _, trigger := range def.PassiveTriggers {
			state, ok := statuses.Get(trigger.Status, trigger.StatusLevel)
			if !ok {
				t.Fatal("missing fixture", trigger)
			}
			if state.DurationSec > 0 && !strings.Contains(strings.Join(s.skillEffectLines(def), ""), fmt.Sprintf("持续%d秒", state.DurationSec)) {
				t.Fatal("actual duration not rendered", id, state.DurationSec)
			}
		}
	}
}
