package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"testing"
)

func TestRedesignIndependentBurnPassiveBoostsLowRankRealTicks(t *testing.T) {
	for _, targetRank := range []int32{1, 5, 20} {
		damage := func(passiveRank int32) int32 {
			s, p := behaviorAcceptancePlayer(t, 5)
			p.Player.Char.Skills = domain.Learned{10421: min(targetRank, 20), 10422: min(passiveRank, 20)}
			d, ok := s.skills.Get(10421, targetRank)
			if !ok {
				t.Fatal(targetRank)
			}
			s.applyPassiveSkillModifiersWithLevels(p, &d, domain.Learned{10422: passiveRank})
			dst := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
			s.entities[dst.ID] = dst
			if !s.applySkill(p, dst, d, 0, false) || dst.Status == nil {
				t.Fatal("burn did not apply", targetRank, passiveRank)
			}
			var burn domain.Active
			dst.Status.Each(func(a domain.Active) {
				if a.Def.IntervalSec > 0 {
					burn = a
				}
			})
			if burn.Def.IntervalSec <= 0 {
				t.Fatal("periodic payload missing", targetRank, passiveRank)
			}
			before := dst.HP
			s.tickStatus(dst, burn)
			return before - dst.HP
		}
		before, next, later := damage(15), damage(16), damage(25)
		if next < before || later <= before {
			t.Fatalf("burn rank%d actual tick ignored passive growth: %d -> %d -> %d", targetRank, before, next, later)
		}
	}
}

func TestRedesignIndependentGrowthTransferAndLoginInitialization(t *testing.T) {
	for _, tc := range []struct {
		prof int32
		id   domain.SkillID
	}{{1, 10024}, {4, 10306}, {5, 10416}} {
		s, p := growthAttributePlayer(t, tc.prof)
		p.Player.Char.Skills = domain.Learned{tc.id: 20}
		s.refreshStats(p)
		p.HP, p.MP = 17, 9
		originalBase, originalStats := p.Player.Char.Base, p.Stats
		snap := s.snapshotOf(p)
		runtime := p.CapturePlayerRuntime(s.tick)
		s.onLeave(Leave{ID: p.ID})
		s.exec(Enter{ID: 2, Char: snap.Char, Bag: snap.Bag, Worn: snap.Worn, Runtime: &runtime, Sink: &fakeSink{}})
		loaded := s.players[2]
		if loaded == nil {
			t.Fatal("relog rejected")
		}
		for i := 0; i < 3; i++ {
			s.refreshStats(loaded)
			if loaded.HP != 17 || loaded.MP != 9 || loaded.Stats != originalStats || loaded.Player.Char.Base != originalBase {
				t.Fatalf("skill %d relog/refresh%d changed resource or primary stats: hp%d mp%d", tc.id, i, loaded.HP, loaded.MP)
			}
		}
		// A new connection intentionally refills the old base maximum. It must
		// not grant the new growth contribution again on the next refresh.
		_, beforeGrowth := domain.Compute(domain.StatSource{Base: originalBase, Level: loaded.Player.Char.Level, Innate: characterInnateStats(loaded.Player.Char), Worn: loaded.Player.Worn, Defs: s.itemDef})
		freshSnap := s.snapshotOf(loaded)
		s.onLeave(Leave{ID: loaded.ID})
		s.exec(Enter{ID: 3, Char: freshSnap.Char, Bag: freshSnap.Bag, Worn: freshSnap.Worn, Sink: &fakeSink{}})
		fresh := s.players[3]
		if fresh == nil {
			t.Fatal("fresh login rejected")
		}
		for i := 0; i < 3; i++ {
			s.refreshStats(fresh)
			if fresh.HP != beforeGrowth.MaxHP || fresh.MP != beforeGrowth.MaxMP || fresh.Stats != originalStats || fresh.Player.Char.Base != originalBase {
				t.Fatalf("skill %d fresh-login growth refilled resources: HP%d MP%d expected old maximum %d/%d", tc.id, fresh.HP, fresh.MP, beforeGrowth.MaxHP, beforeGrowth.MaxMP)
			}
		}
	}
}

func TestRedesignIndependentGrowthAndOrdinaryEquipmentResourceDeltas(t *testing.T) {
	s, p := growthAttributePlayer(t, 1)
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 990771, Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), Durable: 100, Bonus: domain.Stats{MaxHP: 100, MaxMP: 40}}}
	s.items = map[domain.ItemID]domain.ItemDef{item.ID: item}
	p.Player.Char.Skills = domain.Learned{10024: 4}
	s.refreshStats(p)
	p.HP, p.MP = 100, 50
	for i := 0; i < 3; i++ {
		p.Player.Worn.Set(domain.SlotBody, domain.NewStack(item, 1))
		p.Player.Char.Skills[10024] = 20
		s.refreshStats(p)
		if p.HP != 200 || p.MP != 90 {
			t.Fatalf("equipment's existing resource increase changed or growth leaked: hp%d mp%d", p.HP, p.MP)
		}
		p.Player.Worn.Set(domain.SlotBody, domain.Stack{})
		p.Player.Char.Skills[10024] = 4
		s.refreshStats(p)
		if p.HP != 100 || p.MP != 50 {
			t.Fatalf("equipment/growth cycle restores resources: hp%d mp%d", p.HP, p.MP)
		}
	}
}

func TestRedesignIndependentApprovedSupportRangeReachesActualNewTargets(t *testing.T) {
	for _, tc := range []struct {
		id        domain.SkillID
		prof, cap int32
		enemy     bool
	}{{10127, 2, 5, true}, {10028, 1, 5, true}, {10311, 4, 17, false}} {
		s, p := behaviorAcceptancePlayer(t, tc.prof)
		before, _ := s.skills.Get(tc.id, tc.cap)
		after, _ := s.skills.Get(tc.id, tc.cap+1)
		x := p.Pos.X + float64(before.Area.Radius+after.Area.Radius)/2
		outsideX := p.Pos.X + float64(after.Area.Radius) + 1
		var target, outside *entity.Entity
		if tc.enemy {
			target = putMonster(s, 800, 1000, domain.Stats{}, x, p.Pos.Y)
			outside = putMonster(s, 801, 1000, domain.Stats{}, outsideX, p.Pos.Y)
		} else {
			p.Player.Party = 123
			join(t, s, 2, 200, "newly in support range", x, p.Pos.Y)
			join(t, s, 3, 300, "still outside support range", outsideX, p.Pos.Y)
			target, outside = s.players[2], s.players[3]
			target.Player.Party, outside.Player.Party = 123, 123
		}
		contains := func(ids []domain.EntityID, id domain.EntityID) bool {
			for _, v := range ids {
				if v == id {
					return true
				}
			}
			return false
		}
		oldTargets, newTargets := s.resolveTargets(p, before, 0, nil), s.resolveTargets(p, after, 0, nil)
		if contains(oldTargets, target.ID) || !contains(newTargets, target.ID) || contains(newTargets, outside.ID) {
			t.Fatalf("skill%d range failed actual AOI target selection: old%v new%v radii%d/%d", tc.id, oldTargets, newTargets, before.Area.Radius, after.Area.Radius)
		}
	}
}

func TestRedesignIndependentSixSavingsNeverReverseAtCap(t *testing.T) {
	for id, cap := range redesignUtilityCaps {
		ident, _ := domain.V5SkillIdentity(id, catalogfixture.SkillProgressionCatalog())
		s, p := behaviorAcceptancePlayer(t, ident.Profession)
		var previous domain.SkillDef
		for _, rank := range []int32{cap, cap + 1, cap + 2, cap + 20, cap + 100} {
			hard := domain.Learned{id: min(rank, 20)}
			raw := domain.Learned{id: rank}
			p.Player.Char.Skills = hard
			s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: raw}}
			d, ok := s.skills.Get(id, rank)
			if !ok {
				t.Fatal(id, rank)
			}
			s.applyResolvedSkillModifiers(p, &d, hard, raw)
			if previous.ID != 0 {
				if d.MPCost > previous.MPCost || d.CooldownMS > previous.CooldownMS {
					t.Errorf("skill%d cap%d rank%d efficiency reversal: MP%d -> %d CD%d -> %d", id, cap, rank, previous.MPCost, d.MPCost, previous.CooldownMS, d.CooldownMS)
				}
				if previous.MPCost == 0 && d.MPCost != 0 || previous.CooldownMS == 0 && d.CooldownMS != 0 {
					t.Errorf("skill%d invented absent cost/cooldown", id)
				}
			}
			previous = d
		}
	}
}

func TestRedesignIndependentPoisonBasicHitConsumesGrowingTickPayload(t *testing.T) {
	var previous int32
	var interval, duration int32
	for _, rank := range []int32{5, 6, 20} {
		s, p := behaviorAcceptancePlayer(t, 3)
		p.Player.Char.Skills = domain.Learned{10211: rank}
		d, ok := s.skills.Get(10211, rank)
		if !ok {
			t.Fatal(rank)
		}
		if !s.applySkill(p, p, d, 0, false) {
			t.Fatal("poison coating failed", rank)
		}
		var coating domain.Active
		p.Status.Each(func(a domain.Active) {
			if a.Def.PoisonOnHitChance > 0 {
				coating = a
			}
		})
		if coating.Def.PoisonOnHitChance != 50 {
			t.Fatal("poison base chance changed", rank, coating.Def.PoisonOnHitChance)
		}
		dst := putMonster(s, 990, 1000000, domain.Stats{}, 120, 100)
		// Spell damage must never trigger an ordinary-attack-only coating.
		s.afterStrikeStatuses(p, dst, event.DamageDealt{Src: p.ID, Dst: dst.ID, SkillID: 10209, Amount: 1}, 1, 0)
		if dst.Status != nil && dst.Status.Has(domain.StatusPoison) {
			t.Fatal("poison leaked onto skill hit", rank)
		}
		// A deterministic successful 50% base draw exercises the real
		// after-strike hook, carrier resolution, harmful status and periodic tick.
		s.rng = rand.New(rand.NewSource(2))
		expected := rand.New(rand.NewSource(2)).Float64() < .5
		if !expected {
			t.Fatal("chosen seed is not a proc")
		}
		s.afterStrikeStatuses(p, dst, event.DamageDealt{Src: p.ID, Dst: dst.ID, Amount: 1}, 1, 0)
		if dst.Status == nil || !dst.Status.Has(domain.StatusPoison) {
			t.Fatal("basic hit failed to apply poison", rank)
		}
		var poison domain.Active
		dst.Status.Each(func(a domain.Active) {
			if a.Def.ID == domain.StatusPoison {
				poison = a
			}
		})
		if rank == 5 {
			interval, duration = poison.Def.IntervalSec, poison.Def.DurationSec
		} else if poison.Def.IntervalSec != interval || poison.Def.DurationSec != duration {
			t.Fatal("poison timing changed", rank, poison.Def)
		}
		before := dst.HP
		s.tickStatus(dst, poison)
		damage := before - dst.HP
		if damage <= previous {
			t.Fatalf("poison rank%d real tick stopped growing: %d -> %d", rank, previous, damage)
		}
		previous = damage
	}
}
