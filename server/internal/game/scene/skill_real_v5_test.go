package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func realV5SkillFixtures(t *testing.T) (domain.SkillTable, domain.StatusTable) {
	t.Helper()
	b, e := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []domain.SkillDef
	if e = json.Unmarshal(b, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 149*5 {
		t.Fatalf("original fixture rows = %d, want 745", len(rows))
	}
	skills := domain.SkillTable{}
	for _, d := range rows {
		key := domain.SkillKey{ID: d.ID, Level: d.Level}
		if _, exists := skills[key]; exists {
			t.Fatalf("duplicate original skill rank: %+v", key)
		}
		skills[key] = d
	}
	if e = domain.InstallV5SkillProgression(skills); e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile("../../../../docs/daoist-endgame/v5/skill-status-anchors.json")
	if e != nil {
		t.Fatal(e)
	}
	var states []domain.StatusDef
	if e = json.Unmarshal(b, &states); e != nil {
		t.Fatal(e)
	}
	statuses := domain.StatusTable{}
	for _, d := range states {
		statuses[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	return skills, statuses
}
func TestV5StealthAndAdvancedStealthRetainOriginalPercentageAtHit(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, rank := range []int32{1, 2, 3, 4, 5, 20, 21, 100} {
		t.Run(fmt.Sprint(rank), func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "刺客", 100, 100)
			p := s.players[1]
			p.Player.Char.Race = domain.Assassin
			p.Player.Char.Level = 100
			p.Player.Char.Skills = domain.Learned{10208: min(rank, 20), 10207: 1}
			s.skills, s.statuses = skills, statuses
			def, ok := skills.Get(10208, rank)
			if !ok {
				t.Fatal("unresolved stealth rank", rank)
			}
			s.applyPassiveSkillModifiersWithLevels(p, &def, domain.Learned{10207: 1})
			// This seam supplies an accepted effective-rank snapshot. Source-loss
			// reconciliation is covered separately, including actual equipped items.
			s.skillProgressionRefreshing = true
			if !s.applySkill(p, p, def, 0, false) {
				t.Fatal("stealth failed")
			}
			s.skillProgressionRefreshing = false
			blow := combat.Blow{SkillPct: 100, GuaranteedHit: true}
			s.prepareBasicAttackStatuses(p, &blow)
			wantPct := int32(100) + 35*rank + 5 // original stealth and advanced-stealth slopes
			if blow.FlatDamage != 0 || blow.SkillPct != wantPct {
				t.Fatal("stealth must retain original percentage without invented flat damage", rank, blow, wantPct)
			}
			if p.Status.Invisible() {
				t.Fatal("stealth failed to consume")
			}
			p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.CritRate = 100, 100, 0
			target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
			s.entities[target.ID] = target
			ev, _, _ := s.resolveStrikeStatuses(p, target, &combat.HitTracker{}, blow)
			if ev.Amount != wantPct {
				t.Fatal("consumed percentage did not reach real strike", rank, ev, wantPct)
			}
			next := combat.Blow{SkillPct: 100, GuaranteedHit: true}
			s.prepareBasicAttackStatuses(p, &next)
			if next.SkillPct != 100 || next.FlatDamage != 0 {
				t.Fatal("stealth bonus applied twice", next)
			}
		})
	}
}

func TestV5OneShotSpellsRetainOriginalEffectAtConsumption(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, id := range []domain.SkillID{10419, 10420, 10328, 10329} {
		for _, rank := range []int32{1, 2, 3, 4, 5, 20, 21, 100} {
			t.Run(fmt.Sprintf("%d/%d", id, rank), func(t *testing.T) {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "术者", 100, 100)
				p := s.players[1]
				ident, _ := domain.V5SkillIdentity(id)
				p.Player.Char.Race = domain.Race(ident.Profession - 1)
				p.Player.Char.Level = 100
				p.Player.Char.Skills = domain.Learned{id: min(rank, 20)}
				s.skills, s.statuses = skills, statuses
				buff, ok := skills.Get(id, rank)
				if !ok {
					t.Fatal("unresolved one-shot rank", id, rank)
				}
				s.skillProgressionRefreshing = true
				if !s.applySkill(p, p, buff, 0, false) {
					t.Fatal("one-shot cast failed", id, rank)
				}
				s.skillProgressionRefreshing = false
				attack := domain.SkillDef{ID: 10401, PrepareMS: 1000, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}, HurtType: domain.HurtFire}
				s.prepareSkillStatuses(p, &attack)
				wantPct, wantPrepare := int32(100), int32(1000)
				if id == 10419 || id == 10328 {
					wantPct += 40 + 15*(rank-1)
				} else {
					wantPrepare -= wantPrepare * min(int32(100), 60+10*(rank-1)) / 100
				}
				if attack.Effect.DamageFlat != 0 || attack.Effect.DamagePct != wantPct || attack.PrepareMS != wantPrepare {
					t.Fatal("one-shot changed original effect category or magnitude", attack, wantPct, wantPrepare)
				}
				if p.Status.Has(buff.Statuses[0].ID) {
					t.Fatal("one-shot status not consumed")
				}
				p.Stats.MAtk, p.Stats.MCritRate = 100, 0
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
				s.entities[target.ID] = target
				if !s.applySkill(p, target, attack, 0, false) || target.HP != 1000000-wantPct {
					t.Fatal("one-shot percentage not consumed by real strike", id, rank, target.HP, wantPct)
				}
				next := domain.SkillDef{PrepareMS: 1000, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
				s.prepareSkillStatuses(p, &next)
				if next.Effect.DamagePct != 100 || next.Effect.DamageFlat != 0 || next.PrepareMS != 1000 {
					t.Fatal("one-shot applied twice", next)
				}
			})
		}
	}
}

func TestV5PureControlsRetainStatusWithoutInventedDamageOrMiss(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, id := range []domain.SkillID{10016, 10029, 10128, 10224} {
		for _, rank := range []int32{1, 2, 3, 4, 5, 20, 21, 100} {
			t.Run(fmt.Sprintf("%d/%d", id, rank), func(t *testing.T) {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "控制", 100, 100)
				p := s.players[1]
				ident, _ := domain.V5SkillIdentity(id)
				p.Player.Char.Race = domain.Race(ident.Profession - 1)
				p.Player.Char.Level = 100
				p.Player.Char.Skills = domain.Learned{id: min(rank, 20)}
				s.skills, s.statuses = skills, statuses
				p.Stats.Hit, p.Stats.CritRate = -100000, 0
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 10000, MaxHP: 10000, Stats: domain.Stats{Def: 100000}, Monster: &entity.Monster{}}
				s.entities[target.ID] = target
				def, ok := skills.Get(id, rank)
				if !ok || def.HasDamageEffect() || def.ProgressionShieldFlat != 0 {
					t.Fatal("pure control gained a damage/shield carrier", id, rank, def)
				}
				if !s.applySkill(p, target, def, 0, false) {
					t.Fatal("pure control incorrectly introduced MISS", id, rank)
				}
				if target.HP != 10000 {
					t.Fatal("pure control dealt invented damage", id, rank, target.HP)
				}
				for _, app := range def.Statuses {
					if app.ChanceBP < 0 || app.ChanceBP > 10000 {
						t.Fatal("unbounded status probability", app)
					}
					if target.Status == nil || !target.Status.Has(app.ID) {
						t.Fatal("original control lost", id, rank, app)
					}
				}
			})
		}
	}
}

func TestV5FreezeSynergyUsesRealStatusDuration(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, tc := range []struct{ rank, baseDuration, synergyDuration, basePressure, synergyPressure int32 }{
		{5, 30, 33, 0, 0}, {6, 34, 37, 0, 0}, {7, 38, 41, 0, 0}, {11, 54, 59, 0, 0},
		{12, 58, 60, 0, 0}, {13, 60, 60, 0, 0}, {20, 60, 60, 7, 7},
	} {
		t.Run(fmt.Sprint(tc.rank), func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "刺客", 100, 100)
			p := s.players[1]
			p.Player.Char.Race = domain.Assassin
			p.Player.Char.Level = 100
			s.skills, s.statuses = skills, statuses
			for _, synergy := range []bool{false, true} {
				p.Player.Char.Skills = domain.Learned{10216: tc.rank}
				if synergy {
					p.Player.Char.Skills[10217] = 20
				}
				def, ok := skills.Get(10216, tc.rank)
				if !ok {
					t.Fatal("freeze rank unavailable", tc.rank)
				}
				if def.Statuses[0].DurationSec != 0 {
					t.Fatal("fixture no longer exercises table-backed duration", def.Statuses[0])
				}
				s.applyTrainingSynergies(p, &def)
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
				s.entities[target.ID] = target
				if !s.applySkill(p, target, trapPayload(def), 0, true) {
					t.Fatal("freeze did not land")
				}
				var got domain.StatusDef
				target.Status.Each(func(a domain.Active) {
					if a.Def.ID == domain.StatusFreeze {
						got = a.Def
					}
				})
				wantDuration, wantPressure := tc.baseDuration, tc.basePressure
				if synergy {
					wantDuration, wantPressure = tc.synergyDuration, tc.synergyPressure
				}
				if got.DurationSec != wantDuration || got.ControlPower != wantPressure {
					t.Fatal("hard synergy must scale actual duration before cap, with no intermediate rounding", synergy, got.DurationSec, got.ControlPower, wantDuration, wantPressure)
				}
			}
		})
	}
}

func TestV5EveryTrainingEdgeChangesConsumedPayload(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, edge := range domain.V5SkillSynergies {
		if edge.RankMode != "hard" {
			continue
		}
		t.Run(fmt.Sprintf("%d-to-%d", edge.Source, edge.Target), func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "协同", 100, 100)
			p := s.players[1]
			ident, _ := domain.V5SkillIdentity(edge.Target)
			p.Player.Char.Race = domain.Race(ident.Profession - 1)
			p.Player.Char.Level = 100
			// Bounded control duration can saturate; prove the duration edge
			// below its cap without inventing a compensating effect above it.
			targetRank := int32(20)
			if edge.Effect == "freeze_duration" {
				targetRank = 5
			}
			p.Player.Char.Skills = domain.Learned{edge.Target: targetRank}
			s.skills = skills
			s.statuses = statuses
			a, _ := skills.Get(edge.Target, targetRank)
			s.applyTrainingSynergies(p, &a)
			s.applyPassiveSkillModifiers(p, &a)
			p.Player.Char.Skills[edge.Source] = 20
			b, _ := skills.Get(edge.Target, targetRank)
			s.applyTrainingSynergies(p, &b)
			s.applyPassiveSkillModifiers(p, &b)
			// Every retained hard edge must alter an actually consumed payload field.
			changed := !reflect.DeepEqual(a.DirectEffects(), b.DirectEffects()) || !reflect.DeepEqual(a.Statuses, b.Statuses) || a.MPCost != b.MPCost || a.PrepareMS != b.PrepareMS || a.Area != b.Area || a.Radius != b.Radius || a.TrainingEliteDamageBP != b.TrainingEliteDamageBP
			if !changed {
				t.Fatal("advertised but inert synergy", edge)
			}
		})
	}
}

func TestV5TrapExpansionPersistsOnlyWhileSourceEquipped(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "刺客", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Assassin
	p.Player.Char.Level = 100
	p.Player.Char.Skills = domain.Learned{10213: 20}
	s.skills = skills
	s.statuses = statuses
	item := domain.ItemDef{ID: 1908, Equip: &domain.EquipDef{Slot: 13, Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{1908: item}
	st := domain.NewStack(item, 1)
	st.UID = 1908001
	st.MaxDurability = 100
	st.Durability = 100
	st.Endgame = domain.EndgameItemState{UniqueID: 1908, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
	p.Player.Worn.Set(domain.SlotTwoHand, st)
	rank, _ := s.skillLevelForUse(p, 10213)
	def, _ := skills.Get(10213, rank)
	s.applyTrainingSynergies(p, &def)
	s.applyPassiveSkillModifiers(p, &def)
	s.applyUniqueSkillModifiers(p, &def)
	def.SnapshotRawRanks = s.rawSkillLevels(p)
	def.SnapshotHardRanks = p.Player.Char.Skills.Clone()
	radius := int32(0)
	for _, e := range def.DirectEffects() {
		if e.Kind == domain.EffectPlaceTrap {
			radius = e.TrapRadius
		}
	}
	base, _ := skills.Get(10213, 20)
	native := int32(0)
	for _, e := range base.DirectEffects() {
		if e.Kind == domain.EffectPlaceTrap {
			native = e.TrapRadius
		}
	}
	if radius <= native {
		t.Fatal("fixture lacks expansion", radius, native)
	}
	current, ok := s.lesserPersistentSkill(p, def)
	if !ok {
		t.Fatal("lost equipped trap")
	}
	currentRadius := int32(0)
	for _, e := range current.DirectEffects() {
		if e.Kind == domain.EffectPlaceTrap {
			currentRadius = e.TrapRadius
		}
	}
	if currentRadius != radius {
		t.Fatal("still equipped source vanished", currentRadius, radius)
	}
	trap := &entity.Entity{ID: 9000, Kind: domain.KindTrap, Pos: p.Pos, Trap: &entity.Trap{Owner: p.ID, Def: current, Radius: currentRadius}}
	target := &entity.Entity{ID: 999, Kind: domain.KindMonster, Pos: domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + float64(native+1), Y: p.Pos.Y}, HP: 10000, MaxHP: 10000, Monster: &entity.Monster{Kind: domain.MonsterNormal}}
	s.entities[999] = target
	s.monsters[999] = target
	if len(s.trapTargets(trap)) != 1 {
		t.Fatal("expanded ring enemy not targetable")
	}
	p.Player.Worn.Set(domain.SlotTwoHand, domain.Stack{})
	current, ok = s.lesserPersistentSkill(p, current)
	if !ok {
		t.Fatal("hard20 trap lost on unequip")
	}
	for _, e := range current.DirectEffects() {
		if e.Kind == domain.EffectPlaceTrap {
			trap.Trap.Radius = min(trap.Trap.Radius, e.TrapRadius)
		}
	}
	if trap.Trap.Radius != native || len(s.trapTargets(trap)) != 0 {
		t.Fatal("unequipped expansion retained", trap.Trap.Radius, native)
	}
}

func TestV5DetachedBuffRevokedAcrossTransferAndLogout(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "施术者", 100, 100)
	join(t, s, 2, 101, "队友", 110, 100)
	src, dst := s.players[1], s.players[2]
	dst.Player.Char.Race = domain.Healer
	dst.Player.Char.Skills = domain.Learned{10321: 1}
	own := domain.StatusDef{ID: 95001, Level: 1, DurationSec: 30, Type: domain.StatusGood, TypeKnown: true, SkillSnapshotID: 10321, SkillSnapshotRank: 1}
	external := own
	external.ID = 95002
	legacy := domain.StatusDef{ID: 95003, Level: 1, DurationSec: 30, Type: domain.StatusGood, TypeKnown: true}
	dst.Status = domain.NewStatusSet()
	dst.Status.Apply(own, s.tick, dst.ID)
	dst.Status.Apply(external, s.tick, src.ID)
	dst.Status.Apply(legacy, s.tick, src.ID)
	saved := dst.Status.SaveForLogout(s.tick, time.Now(), dst.ID)
	if len(saved) != 2 {
		t.Fatal("detached V5 status saved", saved)
	}
	restored := domain.RestoreSavedStatuses(saved, 10, time.Now(), 9)
	if restored.Has(external.ID) || !restored.Has(legacy.ID) || !restored.Has(own.ID) {
		t.Fatal("logout restore did not preserve proper boundary")
	}
	rt := dst.CapturePlayerRuntime(s.tick)
	copy := &entity.Entity{ID: 9, Player: &entity.Player{}, MaxHP: 100, MaxMP: 100}
	copy.RestorePlayerRuntime(rt, 50)
	if copy.Status.Has(external.ID) || !copy.Status.Has(legacy.ID) || !copy.Status.Has(own.ID) {
		t.Fatal("cross-scene lost boundary")
	}
	copy.Status.Each(func(a domain.Active) {
		if a.Def.ID == own.ID && a.Source != 9 {
			t.Fatal("self source not rebound", a)
		}
	})
	s.revokeDepartingSkillSource(src)
	if dst.Status.Has(external.ID) || !dst.Status.Has(legacy.ID) || !dst.Status.Has(own.ID) {
		t.Fatal("caster departure left detached V5 effect")
	}
	// Also reject an older saved record that already lost its source handle.
	old := domain.SavedStatus{Def: external, RemainingTicks: 100, SourceSelf: false}
	if domain.RestoreSavedStatuses([]domain.SavedStatus{old}, 1, time.Now(), 9).Count() != 0 {
		t.Fatal("detached old record resurrected")
	}
}

func TestV5StatusRequirementExpiryInvalidatesSkillCache(t *testing.T) {
	for _, version := range []int32{5, domain.UniqueContentVersion} {
		t.Run(fmt.Sprintf("content%d", version), func(t *testing.T) {
			skills, statuses := realV5SkillFixtures(t)
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "药师", 100, 100)
			p := s.players[1]
			p.Player.Char.Level = 100
			p.Player.Char.Race = domain.Healer
			p.Player.Char.Base = domain.Base{STR: 10, VIT: 10, INT: 10, SPI: 10, AGI: 10, DEX: 10}
			p.Player.Char.Skills = domain.Learned{10301: 20}
			s.skills = skills
			s.statuses = statuses
			item := domain.ItemDef{ID: 2032, Equip: &domain.EquipDef{Slot: 2, Durable: 100, Need: domain.Requirement{Base: domain.Base{STR: 100}}}}
			s.items = map[domain.ItemID]domain.ItemDef{2032: item}
			st := domain.NewStack(item, 1)
			st.UID = 2032001
			st.MaxDurability = 100
			st.Durability = 100
			st.Endgame = domain.EndgameItemState{UniqueID: 2032, UniqueRollBP: 10000, UniqueContentVersion: version}
			p.Player.Worn.Set(domain.SlotHead, st)
			unique, ok := domain.UniqueOf(st)
			if !ok {
				t.Fatal("missing versioned unique fixture", version)
			}
			bonuses := append([]domain.SkillRankBonus(nil), unique.SkillBonuses...)
			for i := range bonuses {
				bonuses[i].SourceID = fmt.Sprintf("fixture:%d", i)
			}
			ranks, err := domain.ResolveSkillLoadout(int32(p.Player.Char.Race)+1, p.Player.Char.Skills, bonuses, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantRank, err := ranks[10301].EngineRank()
			if err != nil {
				t.Fatal(err)
			}
			if wantRank <= 20 || version == 5 && wantRank != 21 {
				t.Fatal("fixture must retain versioned soft bonus", version, wantRank)
			}
			buff := domain.StatusDef{ID: 95111, Level: 1, DurationSec: 1, Type: domain.StatusGood, TypeKnown: true, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 100, Mode: domain.ModeAbsolute}}}
			if !s.applyStatusDef(p, buff, p.ID) {
				t.Fatal("strength buff failed")
			}
			if rank, _ := s.skillLevelForUse(p, 10301); rank != wantRank {
				t.Fatal("eligible gear rank", rank)
			}
			// Confirm the versioned soft rank is cached before real status removal.
			if rank, _ := s.skillLevelForUse(p, 10301); rank != wantRank {
				t.Fatal(rank)
			}
			removed := p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == buff.ID })
			s.statusesRemoved(p, removed)
			if !s.functionalWorn(p).At(domain.SlotHead).Empty() {
				t.Fatal("fixture gear still usable")
			}
			if rank, _ := s.skillLevelForUse(p, 10301); rank != 20 {
				t.Fatal("expired strength retained cached soft rank", rank)
			}
			if p.Player.Char.Skills[10301] != 20 {
				t.Fatal("hard rank mutated")
			}
		})
	}
}

func TestV5UnlearnedViewHasFirstPointAndSoftPreview(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "药师", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race = domain.Healer
	ch.Level = 100
	ch.SkillPoints = 99
	ch.SkillAllocationVersion = 5
	ch.Skills = domain.Learned{}
	s.skills = skills
	s.statuses = statuses
	item := domain.ItemDef{ID: 2032, Equip: &domain.EquipDef{Slot: 2}}
	s.items = map[domain.ItemID]domain.ItemDef{2032: item}
	st := domain.NewStack(item, 1)
	st.UID = 2032002
	st.Endgame = domain.EndgameItemState{UniqueID: 2032, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
	p.Player.Worn.Set(domain.SlotHead, st)
	view, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	v := view[10301]
	if v.CanCast || !v.CanInvest || v.SoftRank != "1" || len(v.CurrentEffectLines) != 0 || len(v.NextEffectLines) == 0 || v.NextEffectiveRank != "2" || v.NextRequiredLevel != 10 {
		t.Fatal(v)
	}
	ch.Skills = domain.Learned{10305: 1}
	view, err = s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range view[10305].CurrentEffectLines {
		if strings.Contains(line, "成功率0%") || strings.Contains(line, "/10000") {
			t.Fatal("misleading probability", line)
		}
	}
	for _, v := range view {
		for _, line := range append(v.CurrentEffectLines, v.NextEffectLines...) {
			if strings.Contains(line, "属性13") || strings.Contains(line, "/19%") {
				t.Fatal("raw technical label", line)
			}
		}
	}
}
