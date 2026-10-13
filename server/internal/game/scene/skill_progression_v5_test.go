package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"errors"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func v5TestSkillTable(id domain.SkillID, prof int32, fx domain.SkillEffect) domain.SkillTable {
	// Transaction tests use complete synthetic rows; combat tests below use the
	// authoritative 745-row fixture so original effect growth is independently covered.
	table := domain.SkillTable{}
	for rank := int32(1); rank <= 5; rank++ {
		table[domain.SkillKey{ID: id, Level: rank}] = domain.SkillDef{ID: id, Prof: prof, Level: rank, FormulaVersion: 5, Kind: domain.SkillSingle, Effect: fx, TargetEnemy: true, LevelNeed: 10 + rank - 1}
	}
	domain.AttachSkillProgression(table, catalogfixture.SkillProgressionCatalog())
	return table
}
func TestV5UpgradeUsesReceiptAtomicAndReplay(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	s.skills = v5TestSkillTable(10401, 5, domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 208})
	ch.Level = 100
	ch.Race = domain.Warlock
	ch.Skills = domain.Learned{}
	ch.SkillPoints = 99
	ch.SkillPointsKnown = true
	ch.SkillAllocationVersion = 5
	p.Player.EndgameGrowthV5 = true
	b := newDispatchBackend()
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	dispatchMessages(s, sink)
	r := domain.EndgameRequest{Op: "skill.upgrade", SkillID: 10401, RequestID: "skill-upgrade-000001", ExpectedRevision: "0"}
	s.onEndgame(EndgameRequest{ID: 1, Request: r})
	b.receipt <- domain.EndgameReceiptLoadResult{}
	s.finishEndgame(false)
	if b.commits != 1 || ch.Skills[10401] != 0 || ch.SkillPoints != 99 || !p.Player.TradeBusy {
		t.Fatal("published before commit", b.commits, ch)
	}
	plan := b.plan
	if plan.Snapshot.Char.Skills[10401] != 1 || plan.Snapshot.Char.SkillPoints != 98 {
		t.Fatal(plan)
	}
	b.commit <- domain.EndgameCommitResult{State: plan.State, Receipt: plan.Receipt}
	s.finishEndgame(false)
	if ch.Skills[10401] != 1 || ch.SkillPoints != 98 {
		t.Fatal(ch)
	}
	s.onEndgame(EndgameRequest{ID: 1, Request: r})
	b.receipt <- domain.EndgameReceiptLoadResult{Receipt: &plan.Receipt}
	s.finishEndgame(false)
	if b.commits != 1 || ch.Skills[10401] != 1 || ch.SkillPoints != 98 {
		t.Fatal("replay spent twice")
	}
	r.RequestID = "skill-upgrade-000002"
	r.ExpectedRevision = "1"
	s.onEndgame(EndgameRequest{ID: 1, Request: r})
	b.receipt <- domain.EndgameReceiptLoadResult{}
	s.finishEndgame(false)
	b.commit <- domain.EndgameCommitResult{Err: errors.New("rollback")}
	s.finishEndgame(false)
	if ch.Skills[10401] != 1 || ch.SkillPoints != 98 {
		t.Fatal("failed transaction spent points")
	}
}
func TestV5LegacyUpgradeAndBooksDoNotMintPoints(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	s.skills = v5TestSkillTable(10401, 5, domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 208})
	p.Player.Char.Skills = domain.Learned{}
	p.Player.Char.SkillPoints = 99
	s.onUpgradeSkill(UpgradeSkill{ID: 1, Skill: 10401})
	s.useSkillBook(p, 0, domain.ItemDef{UseSkillBook: 10401}, func(event.RejectReason) { t.Fatal("book wrongly consumed/rejected legacy") })
	if p.Player.Char.Skills[10401] != 0 || p.Player.Char.SkillPoints != 99 {
		t.Fatal("legacy path minted/spent")
	}
	_ = sink
}
func TestV5HardWindowAndPrerequisiteIgnoresSoft(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race = domain.Warlock
	ch.Level = 10
	ch.Skills = domain.Learned{10401: 1}
	s.skills = v5TestSkillTable(10401, 5, domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 208})
	if _, _, _, ok := s.skillUpgradeTarget(p, 10401); ok {
		t.Fatal("learned second hard point before level11")
	}
	ch.Level = 11
	if _, n, _, ok := s.skillUpgradeTarget(p, 10401); !ok || n != 2 {
		t.Fatal(n, ok)
	}
	ch.Level = 100
	ch.Skills[10401] = 20
	if _, _, r, ok := s.skillUpgradeTarget(p, 10401); ok || r != event.RejectMaxLevel {
		t.Fatal(r, ok)
	}
}
func TestV5ExactDamageAndHardSynergyUseRealStrike(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race = domain.Warlock
	ch.Skills = domain.Learned{10401: 20, 10402: 20}
	s.skills, s.statuses = realV5SkillFixtures(t)
	def, _ := s.skills.Get(10401, 20)
	s.applyTrainingSynergies(p, &def)
	s.applyPassiveSkillModifiers(p, &def)
	if def.Effect.DamagePct != 432 || def.Effect.DamageFlat != 0 {
		t.Fatal(def.Effect)
	}
	p.Stats.MAtk = 1000
	p.Stats.MCritRate = 0
	target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 10000, MaxHP: 10000, Monster: &entity.Monster{}}
	s.entities[999] = target
	if !s.applySkill(p, target, def, 0, false) || target.HP != 5680 {
		t.Fatal("expected original 360% times 1.20 hard synergy = 4320 damage", target.HP)
	}
}

func TestV5CatalogSkillGearChangesRealHealingAndRemoval(t *testing.T) {
	for _, hardRank := range []int32{4, 20} {
		for _, id := range []domain.ItemID{2032, 2056, 2409, 2559, 2899, 1310} {
			t.Run(fmt.Sprintf("%d/rank%d", id, hardRank), func(t *testing.T) {
				s := newTestScene(t, nil)
				join(t, s, 1, 100, "药师", 100, 100)
				p := s.players[1]
				ch := p.Player.Char
				ch.Race = domain.Healer
				ch.Level = 100
				ch.Skills = domain.Learned{10301: hardRank}
				s.skills, s.statuses = realV5SkillFixtures(t)
				u, ok := domain.UniqueForBase(id, catalogfixture.EndgameCatalog())
				if !ok {
					t.Fatal(id)
				}
				item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Equip: &domain.EquipDef{Slot: int32(u.Slot), Durable: 100}}
				s.items = map[domain.ItemID]domain.ItemDef{id: item}
				st := domain.NewStack(item, 1)
				st.UID = 900000 + int64(id)
				// Keep this historical V5 contract independent of the current item catalog.
				st.Endgame = domain.EndgameItemState{UniqueID: int32(id), UniqueRollBP: 10000, UniqueContentVersion: 5}
				st.Durability = 100
				st.MaxDurability = 100
				p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
				lv, _ := s.skillLevelForUse(p, 10301)
				want := hardRank + 1
				if id == 2056 || id == 2559 || id == 2899 {
					want = hardRank + 2
				}
				if lv != want {
					t.Fatal(id, lv, want)
				}
				def, ok := s.skills.Get(10301, lv)
				if !ok {
					t.Fatal(lv)
				}
				p.MaxHP = 10000
				p.HP = 1
				s.applySkill(p, p, def, 0, false)
				// Original flat healing continues +30 per rank; the original percentage
				// retains +5 percentage points per rank up to100%, with no overheal shield.
				wantFlat := int32(100) + 30*(want-1)
				if def.Effect.HealFlat != wantFlat || def.Effect.HealPctOfMax != min(int32(100), 10+5*(want-1)) || def.HasDamageEffect() || def.ProgressionShieldFlat != 0 {
					t.Fatal("healing changed its original effect", def)
				}
				wantHeal := min(int32(9999), wantFlat+100*def.Effect.HealPctOfMax)
				if p.HP != 1+wantHeal {
					t.Fatal("equipped heal real result", p.HP, wantHeal)
				}
				p.HP = 1
				base, _ := s.skills.Get(10301, hardRank)
				s.applySkill(p, p, base, 0, false)
				if def.Effect.HealAmount(10000) <= base.Effect.HealAmount(10000) {
					t.Fatal("equipped rank did not increase original healing potency", def.Effect, base.Effect)
				}
				if hardRank == 4 && p.HP >= 1+wantHeal {
					t.Fatal("equipped rank did not increase available healing", p.HP, wantHeal)
				}
				if hardRank == 20 && (p.HP != p.MaxHP || wantHeal != 9999) {
					t.Fatal("capped healing overflowed missing health", p.HP, wantHeal)
				}
				st.Durability = 0
				p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
				lv, _ = s.skillLevelForUse(p, 10301)
				if lv != hardRank || ch.Skills[10301] != hardRank {
					t.Fatal("broken gear retained soft/hard changed", lv, ch.Skills)
				}
				p.Player.Worn.Set(domain.EquipSlot(u.Slot), domain.Stack{})
				lv, _ = s.skillLevelForUse(p, 10301)
				if lv != hardRank {
					t.Fatal("unequip retained soft")
				}
			})
		}
	}
}
func TestV5StatusCarrierConsumedAndPersistentDowngrade(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "药师", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race = domain.Healer
	ch.Level = 100
	ch.Skills = domain.Learned{10320: 20}
	s.skills, s.statuses = realV5SkillFixtures(t)
	def, _ := s.skills.Get(10320, 22)
	def.SnapshotRawRanks = domain.Learned{10320: 22}
	def.SnapshotHardRanks = ch.Skills.Clone()
	if !s.applySkill(p, p, def, 0, false) {
		t.Fatal("shield cast failed")
	}
	// applyStatus refresh immediately reconciles unsupported current22 to actual20.
	var before domain.Active
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusDamageShield {
			before = a
		}
	})
	if before.Def.DamageShield != 4500 {
		t.Fatal("soft source downgrade not applied", before)
	}
	damage, _ := p.Status.MitigateDamage(false, 50)
	if damage != 0 {
		t.Fatal(damage)
	}
	ch.Skills[10320] = 1
	s.refreshSkillPersistentEffects(p)
	var after domain.Active
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusDamageShield {
			after = a
		}
	})
	if after.Def.DamageShield != 700 || after.ShieldLeft != 650 || after.ExpireAt > before.ExpireAt {
		t.Fatal("shield refunded/kept high rank", after)
	}
	ch.Skills[10320] = 20
	s.refreshSkillPersistentEffects(p)
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusDamageShield && a.ShieldLeft != 650 {
			t.Fatal("later rank retroactively amplified old shield", a)
		}
	})
}
