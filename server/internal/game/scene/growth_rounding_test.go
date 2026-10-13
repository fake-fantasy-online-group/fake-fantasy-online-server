package scene

import (
	"fmt"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func roundingPlayer(t *testing.T, id domain.SkillID, rank int32) (*Scene, *entity.Entity, UseSkill) {
	t.Helper()
	prof := int32((id-10000)/100 + 1)
	s, p := behaviorAcceptancePlayer(t, prof)
	p.Player.Char.Skills = domain.Learned{id: min(rank, 20)}
	// Synthetic equipment bonus drives the real equipment/loadout resolver. This
	// is a high-rank boundary fixture, not a claim that live gear grants +9936.
	if rank > 20 {
		input := catalogfixture.EndgameCatalogData()
		for i := range input.Uniques {
			if input.Uniques[i].Version == domain.UniqueContentVersion && input.Uniques[i].Definition.ID == 2032 {
				input.Uniques[i].Definition.SkillBonuses = []domain.SkillRankBonus{{Scope: "single", SkillID: id, Levels: int64(rank - 20)}}
				input.Uniques[i].Definition.TypedEffects = nil
				input.Uniques[i].Definition.Traits = nil
			}
		}
		catalog, err := domain.NewEndgameCatalog(input)
		if err != nil {
			t.Fatal(err)
		}
		s.endgameCatalog = catalog
		item := domain.ItemDef{ID: 2032, EndgameCatalog: catalog, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotHead), Durable: 100}}
		s.items = map[domain.ItemID]domain.ItemDef{2032: item}
		st := domain.NewStack(item, 1)
		st.UID = 2032001
		st.Durability, st.MaxDurability = 100, 100
		st.Endgame = domain.EndgameItemState{UniqueID: 2032, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
		p.Player.Worn.Set(domain.SlotHead, st)
	}
	if got := s.rawSkillLevels(p)[id]; got != rank {
		t.Fatal("equipped rank", got, rank)
	}
	p.MP, p.MaxMP = 1000000, 1000000
	cmd := UseSkill{ID: p.ID, Skill: id, Target: p.ID}
	if id == 10432 {
		at := domain.Pos{MapID: s.id.MapID, X: 110, Y: 100}
		cmd.At = &at
	}
	return s, p, cmd
}

func TestGrowthRoundingActualCastsConserveManaAndFrames(t *testing.T) {
	for _, tc := range []struct {
		id        domain.SkillID
		rank      int32
		mp, ticks int64
	}{{10018, 100, 828, 6897}, {10018, 101, 822, 6850}, {10432, 100, 4690, 6897}, {10432, 101, 4658, 6850}, {10018, 9955, 100, 100}, {10018, 9956, 100, 100}, {10432, 9956, 100, 100}} {
		t.Run(fmt.Sprintf("%d/%d", tc.id, tc.rank), func(t *testing.T) {
			s, p, cmd := roundingPlayer(t, tc.id, tc.rank)
			var mana, frames int64
			for i := 0; i < 100; i++ {
				d, ok := s.skills.Get(tc.id, tc.rank)
				if !ok {
					t.Fatal("missing skill")
				}
				s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
				ledger := p.Player.Char.Clone().SkillRoundingCredits
				once := d
				previewGrowthRounding(p, &d)
				if !reflect.DeepEqual(once, d) || !reflect.DeepEqual(ledger, p.Player.Char.SkillRoundingCredits) {
					t.Fatal("preview consumed or changed credit")
				}
				p.MP, p.MaxMP = 1000000, 1000000
				before := p.MP
				s.onUseSkill(cmd)
				if len(s.casts) != 1 || before-p.MP != d.MPCost {
					t.Fatal(i, before-p.MP, d.MPCost, len(s.casts))
				}
				ticks := domain.Ticks(int(s.casts[0].cooldownMS))
				if s.casts[0].def.CooldownMS != d.CooldownMS || ticks < 1 || d.MPCost < 1 || !p.Player.SkillReadyAt(tc.id, s.tick+ticks) || p.Player.SkillReadyAt(tc.id, s.tick+ticks-1) {
					t.Fatal("actual ready-at disagrees", i, d.CooldownMS, ticks)
				}
				mana += int64(before - p.MP)
				frames += int64(ticks)
				// A repeated packet in the same frame is rejected and cannot spend credit.
				after := p.Player.Char.Clone().SkillRoundingCredits
				mp := p.MP
				s.onUseSkill(cmd)
				if len(s.casts) != 1 || p.MP != mp || !reflect.DeepEqual(after, p.Player.Char.SkillRoundingCredits) {
					t.Fatal("duplicate spent credit")
				}
				s.stepCasts()
				s.casts = nil
				s.tick += ticks
			}
			if mana != tc.mp || frames != tc.ticks {
				t.Fatalf("100 actual casts: mana%d want%d; frames%d want%d", mana, tc.mp, frames, tc.ticks)
			}
			t.Logf("100 accepted casts paid %d mana, %d actual cooldown ticks", mana, frames)
		})
	}
}

func TestGrowthRoundingRejectedCastsAndLifecycle(t *testing.T) {
	s, p, cmd := roundingPlayer(t, 10432, 101)
	s.onUseSkill(cmd)
	s.stepCasts()
	s.casts = nil
	s.tick += 1000
	saved := p.Player.Char.Clone().SkillRoundingCredits
	p.MP = 0
	s.onUseSkill(cmd)
	if len(s.casts) != 0 || !reflect.DeepEqual(saved, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("not-enough-mana spent credit")
	}
	p.MP = 100000
	bad := cmd
	at := domain.Pos{MapID: s.id.MapID, X: 100000, Y: 100000}
	bad.At = &at
	s.onUseSkill(bad)
	if len(s.casts) != 0 || !reflect.DeepEqual(saved, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("bad target spent credit")
	}
	// Snapshot clone is the shared persistence/transfer contract; it must not alias.
	snap := s.snapshotOf(p)
	runtime := p.CapturePlayerRuntime(s.tick)
	want := snap.Char.Clone().SkillRoundingCredits
	s.onLeave(Leave{ID: p.ID})
	s.exec(Enter{ID: 2, Char: snap.Char, Bag: snap.Bag, Worn: snap.Worn, Runtime: &runtime, Sink: &fakeSink{}})
	p = s.players[2]
	if p == nil || !reflect.DeepEqual(want, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("transfer lost credit")
	}
	// Re-enter from a saved character without runtime (normal fresh login).
	relog := s.snapshotOf(p)
	s.onLeave(Leave{ID: p.ID})
	s.exec(Enter{ID: 3, Char: relog.Char, Bag: relog.Bag, Worn: relog.Worn, Sink: &fakeSink{}})
	p = s.players[3]
	if p == nil || !reflect.DeepEqual(want, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("relogin lost credit")
	}
	p.Player.Char.Skills = domain.Learned{10432: 5}
	d, _ := s.skills.Get(10432, 5)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, p.Player.Char.Skills)
	before := p.Player.Char.Clone().SkillRoundingCredits
	commitGrowthRounding(p, d)
	if !reflect.DeepEqual(before, p.Player.Char.SkillRoundingCredits) || d.MPCost != 136 || d.CooldownMS != 20000 {
		t.Fatal("native rank consumed credit")
	}
	other := p.Player.Char.Clone()
	other.SkillRoundingCredits[10432] = domain.SkillRoundingState{}
	if !reflect.DeepEqual(want, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("snapshot aliases live ledger")
	}
}

func TestGrowthRoundingBurnActualStepStatusAdjacentTotals(t *testing.T) {
	for _, targetRank := range []int32{1, 5, 20} {
		for _, passiveRank := range []int32{15, 16, 20, 21, 22, 100, 101} {
			s, p := behaviorAcceptancePlayer(t, 5)
			p.Player.Char.Skills = domain.Learned{10421: min(targetRank, 20), 10422: min(passiveRank, 20)}
			d, ok := s.skills.Get(10421, targetRank)
			if !ok {
				t.Fatal(targetRank)
			}
			s.applyPassiveSkillModifiersWithLevels(p, &d, domain.Learned{10422: passiveRank})
			target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
			s.entities[target.ID] = target
			if !s.applySkill(p, target, d, 0, false) || target.Status == nil {
				t.Fatal("burn missing")
			}
			before := target.HP
			for i := 0; i < 10; i++ {
				s.tick += 20
				s.stepStatus()
			}
			base := int64(20)
			if targetRank == 20 {
				base = 35
			}
			bonus := int64(max(int32(0), passiveRank-15)) * 2
			want := base * (100 + bonus) * 10 / 100
			if int64(before-target.HP) != want {
				t.Fatalf("burn%d passive%d 10 true ticks=%d want%d", targetRank, passiveRank, before-target.HP, want)
			}
			t.Logf("burn%d passive%d 10 ticks=%d", targetRank, passiveRank, before-target.HP)
		}
	}
}

func TestGrowthRoundingCastReceiptUsesActualFrameCooldown(t *testing.T) {
	s, p, cmd := roundingPlayer(t, 10018, 101)
	sink := p.Player.Sink.(*fakeSink)
	for i, want := range []int32{6900, 6800, 6900, 6800} {
		sink.take()
		s.onUseSkill(cmd)
		s.flush()
		found, sidecar := false, false
		for _, e := range sink.take() {
			if update, ok := e.(event.EndgameMessage); ok && update.Message.Kind == "skill.ranks" {
				for _, row := range update.Message.SkillRanks {
					if row.ID == 10018 && row.Contract != nil && row.Contract.Current != nil {
						next, _ := s.skills.Get(10018, 101)
						s.applyResolvedSkillModifiers(p, &next, p.Player.Char.Skills, s.rawSkillLevels(p))
						if row.Contract.Current.MPCost != next.MPCost || row.Contract.Current.CooldownMS != next.CooldownMS {
							t.Fatal("sidecar stale next cost", row.Contract.Current, next.MPCost, next.CooldownMS)
						}
						sidecar = true
					}
				}
			}
			if cast, ok := e.(event.SkillCastChanged); ok && cast.Phase == event.SkillCastChant {
				found = true
				if cast.CooldownMS != want {
					t.Fatal(i, cast.CooldownMS, want)
				}
			}
		}
		if !found || !sidecar {
			t.Fatal("phase0 or changed-cost sidecar missing", found, sidecar)
		}
		s.casts = nil
		s.tick += domain.Ticks(int(want))
	}
}

func TestGrowthRoundingBurnReequipmentCannotBankHigherRank(t *testing.T) {
	s, p, _ := roundingPlayer(t, 10422, 101)
	p.Player.Char.Skills[10421] = 5
	target := putMonster(s, 999, 1000000, domain.Stats{}, 110, 100)
	cmd := UseSkill{ID: p.ID, Skill: 10421, Target: target.ID}
	s.onUseSkill(cmd)
	if len(s.casts) != 1 {
		t.Fatal("burn cast rejected")
	}
	s.stepCasts()
	s.tick += 20
	before := target.HP
	s.stepStatus()
	if before-target.HP != 54 {
		t.Fatal("first fractional burn", before-target.HP)
	}
	head := p.Player.Worn.At(domain.SlotHead)
	p.Player.Worn.Set(domain.SlotHead, domain.Stack{})
	s.refreshStats(p)
	for i := 0; i < 3; i++ {
		s.tick += 20
		before = target.HP
		s.stepStatus()
		if before-target.HP != 22 {
			t.Fatal("removed gear retained high fraction", i, before-target.HP)
		}
	}
	p.Player.Worn.Set(domain.SlotHead, head)
	s.refreshStats(p)
	s.tick += 20
	before = target.HP
	s.stepStatus()
	if before-target.HP != 22 {
		t.Fatal("reequipment upgraded old burn", before-target.HP)
	}
}

func TestGrowthRoundingAllSixAcceptedCostStreams(t *testing.T) {
	for _, tc := range []struct {
		id   domain.SkillID
		rank int32
	}{{10018, 101}, {10227, 101}, {10432, 101}, {10301, 101}, {10305, 101}, {10309, 101}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, p, cmd := roundingPlayer(t, tc.id, tc.rank)
			join(t, s, 2, 200, "rounding recipient", 110, 100)
			target := s.players[2]
			p.Player.Party, target.Player.Party = 7, 7
			if tc.id == 10227 || tc.id == 10305 {
				cmd.Target = target.ID
			}
			initial, _ := s.skills.Get(tc.id, tc.rank)
			s.applyResolvedSkillModifiers(p, &initial, p.Player.Char.Skills, s.rawSkillLevels(p))
			var mana, frames int64
			const casts int64 = 20
			for i := int64(0); i < casts; i++ {
				if tc.id == 10305 {
					target.HP = 0
				}
				p.MP, p.MaxMP = 1000000, 1000000
				before := p.MP
				s.onUseSkill(cmd)
				if len(s.casts) != 1 {
					t.Fatal("accepted cast missing", i)
				}
				elapsed := domain.Ticks(int(s.casts[0].cooldownMS))
				mana += int64(before - p.MP)
				frames += int64(elapsed)
				s.stepCasts()
				s.casts = nil
				s.tick += max(domain.Tick(1), elapsed)
			}
			ceilTotal := func(n, d int64) int64 {
				if n <= 0 || d <= 0 {
					return 0
				}
				n = max(n, d)
				return (casts*n + d - 1) / d
			}
			wantMP := ceilTotal(initial.ManaCostFractionNumerator, initial.ManaCostFractionDenominator)
			wantCD := ceilTotal(initial.CooldownFractionNumeratorMS, initial.CooldownFractionDenominator*domain.TickMS)
			if mana != wantMP || frames != wantCD {
				t.Fatal("accepted stream differs from exact rate", mana, wantMP, frames, wantCD)
			}
		})
	}
}

func TestGrowthRoundingInterruptedCastKeepsExactlyOneDebit(t *testing.T) {
	s, p, cmd := roundingPlayer(t, 10305, 101)
	join(t, s, 2, 200, "revive recipient", 110, 100)
	target := s.players[2]
	p.Player.Party, target.Player.Party = 7, 7
	target.HP = 0
	cmd.Target = target.ID
	s.onUseSkill(cmd)
	if len(s.casts) != 1 || s.casts[0].windup == nil {
		t.Fatal("expected admitted resurrection windup")
	}
	credit := p.Player.Char.Clone().SkillRoundingCredits
	paidMP := p.MP
	cooldown := domain.Ticks(int(s.casts[0].cooldownMS))
	s.clearCastWindup(p.ID)
	s.stepCasts()
	if len(s.casts) != 0 || p.MP != paidMP || !reflect.DeepEqual(credit, p.Player.Char.SkillRoundingCredits) {
		t.Fatal("interruption refunded or double-consumed credit")
	}
	if cooldown > 0 && p.Player.SkillReadyAt(10305, s.tick+cooldown-1) {
		t.Fatal("interruption erased cooldown")
	}
	if len(target.Player.Char.SkillRoundingCredits) != 0 {
		t.Fatal("recipient inherited caster credit")
	}
}
