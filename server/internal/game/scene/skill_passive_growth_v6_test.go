package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"strings"
	"testing"
)

func TestPassiveOverflowRealRuntimeV6(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	join(t, s, 1, 100, "overflow", 100, 100)
	p := s.players[1]
	// Supply the effective-rank snapshot directly. Hard learned ranks remain
	// capped at 20; equipment resolution is covered independently.
	setRanks := func(id domain.SkillID, rank int32) {
		p.Player.Char.Skills = domain.Learned{id: min(rank, 20)}
		s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{id: rank}}}
	}
	for _, rank := range []int32{20, 21, 30, 50, 100} {
		p.Player.Char.Race = domain.Race(0)
		setRanks(10008, rank)
		p.MP = 0
		p.MaxMP = 1000
		s.restorePassiveResistMP(p, 1002)
		if p.MP != rank-10 {
			t.Fatalf("rank%d resisted MP=%d want%d", rank, p.MP, rank-10)
		}
		p.MP = 0
		s.restorePassiveResistMP(p, 1009)
		if p.MP != 0 {
			t.Fatal("resist scope expanded")
		}
		p.Player.Char.Race = domain.Race(2)
		setRanks(10230, rank)
		damage := s.passiveHalfDamage(p, domain.PassiveIncomingMagicHalfChance, 1000000)
		setRanks(10230, rank+1)
		if n := s.passiveHalfDamage(p, domain.PassiveIncomingMagicHalfChance, 1000000); n >= damage || n <= 0 {
			t.Fatalf("rank%d magic mitigation did not improve: %d ->%d", rank, damage, n)
		}
		if n := s.passiveHalfDamage(p, domain.PassiveTrapHalfDamageChance, 1000000); n != 500000 {
			t.Fatal("magic mitigation leaked to traps", n)
		}
		p.Player.Char.Race = domain.Race(0)
		target, _ := skills.Get(10003, 5)
		other, _ := skills.Get(10002, 5)
		levels := domain.Learned{10024: rank}
		if n := s.passiveManaCostReductionRanks(p, target, levels); n != rank-5 {
			t.Fatalf("rank%d durability efficiency=%d", rank, n)
		}
		if n := s.passiveManaCostReductionRanks(p, other, levels); n != 0 {
			t.Fatal("durability efficiency target expanded", n)
		}
	}
}

func TestAttackDurabilityOverflowKeepsWeaponScopeV6(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	join(t, s, 1, 100, "overflow", 100, 100)
	p := s.players[1]
	p.Player.Char.Race, p.Player.Char.Level = domain.Assassin, 100
	p.Player.Char.Skills = domain.Learned{10222: 20}
	item := domain.ItemDef{ID: 1905, Equip: &domain.EquipDef{Slot: 13, Type: 32, Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{1905: item}
	stack := domain.NewStack(item, 1)
	stack.Durability, stack.MaxDurability = 100, 100
	for _, rank := range []int32{20, 21, 30, 50, 100} {
		p.Player.Worn.Set(domain.SlotTwoHand, stack)
		s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{10222: rank}}}
		blow := combat.Blow{SkillPct: 100}
		s.prepareBasicAttackStatuses(p, &blow)
		if blow.SkillPct != 100+rank-10 {
			t.Fatalf("rank%d basic damage multiplier=%d", rank, blow.SkillPct)
		}
		item.Equip.Type = 8
		s.items[item.ID] = item
		blow = combat.Blow{SkillPct: 100}
		s.prepareBasicAttackStatuses(p, &blow)
		if blow.SkillPct != 100 {
			t.Fatal("durability benefit leaked to other weapon", blow.SkillPct)
		}
		item.Equip.Type = 32
		s.items[item.ID] = item
	}
}

func TestCastEfficiencyOverflowKeepsChantScopeV6(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	join(t, s, 1, 100, "overflow", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Warlock
	for _, rank := range []int32{20, 21, 30, 50, 100} {
		chant, _ := skills.Get(10401, rank)
		instant, _ := skills.Get(10409, rank)
		if chant.PrepareMS <= 0 || instant.PrepareMS != 0 {
			t.Fatal("fixture does not distinguish chant/instant")
		}
		chantBefore, instantBefore := chant.ManaCostReductionRanks, instant.ManaCostReductionRanks
		levels := domain.Learned{10415: rank}
		s.applyPassiveSkillModifiersWithLevels(p, &chant, levels)
		s.applyPassiveSkillModifiersWithLevels(p, &instant, levels)
		if chant.ManaCostReductionRanks != chantBefore+rank-17 {
			t.Fatalf("rank%d chant overflow=%d", rank, chant.ManaCostReductionRanks-chantBefore)
		}
		if instant.ManaCostReductionRanks != instantBefore {
			t.Fatal("chant efficiency leaked to instant skill")
		}
	}
}

func TestPassiveOverflowDescriptionV6(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, statuses
	for id, want := range map[domain.SkillID]string{10008: "法力", 10203: "反击伤害", 10222: "普通攻击", 10229: "陷阱", 10230: "法术"} {
		d, _ := skills.Get(id, 20)
		lines := s.passiveOverflowEffectLines(d)
		if !strings.Contains(strings.Join(lines, ""), want) {
			t.Fatalf("skill%d missing overflow description %q: %v", id, want, lines)
		}
		d, _ = skills.Get(id, 5)
		if len(s.passiveOverflowEffectLines(d)) != 0 {
			t.Fatal("legacy prose added")
		}
	}
	d, _ := skills.Get(10024, 20)
	if len(s.passiveOverflowEffectLines(d)) != 0 {
		t.Fatal("source gains outgoing target list")
	}
}

func TestCastEfficiencyMembershipSurvivesRoundedTimingV6(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 5)
	levels := domain.Learned{10415: 100, 10416: 100}
	for _, rank := range []int32{249955, 249956, 1000000} {
		d, ok := s.skills.Get(10413, rank)
		if !ok {
			t.Fatal(rank)
		}
		before := d.ManaCostReductionRanks
		s.applyPassiveSkillModifiersWithLevels(p, &d, levels)
		if d.ManaCostReductionRanks-before != (100-17)+(100-5) {
			t.Fatalf("rank%d lost original chant membership: %d", rank, d.ManaCostReductionRanks-before)
		}
	}
}
