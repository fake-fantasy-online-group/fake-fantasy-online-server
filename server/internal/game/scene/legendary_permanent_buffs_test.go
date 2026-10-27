package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

func TestLegendaryPermanentBuffLearnedLifecycle(t *testing.T) {
	for _, tc := range []struct {
		power  string
		skill  domain.SkillID
		status domain.StatusID
	}{
		{"SS-U03", 10418, 1026}, {"SS-D10", 10419, 1027}, {"SS-A30", 10420, 1028}, {"SS-U08", 10417, 1011},
		{"YS-U02", 10322, 1026}, {"YS-A29", 10328, 1027}, {"YS-A30", 10329, 1028},
	} {
		t.Run(tc.power, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.power)
			p := c.Owner
			p.Status = domain.NewStatusSet()
			delete(p.Player.Char.Skills, tc.skill)
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			if p.Status.Has(tc.status) {
				t.Fatal("unlearned buff granted")
			}
			p.Player.Char.Skills[tc.skill] = 1
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			var first domain.Active
			p.Status.Each(func(a domain.Active) {
				if a.Def.ID == tc.status {
					first = a
				}
			})
			if first.Def.LegendaryPermanentPower != tc.power || first.ExpireAt != 0 || first.Def.SkillSnapshotRank != 1 {
				t.Fatalf("missing learned permanent: %+v", first)
			}
			p.Status.ConsumeAttackModifiers(true, true, true, true)
			if !p.Status.Has(tc.status) {
				t.Fatal("consumed")
			}
			s.tick += domain.Ticks(5000)
			s.syncLegendaryPermanentBuffs(p)
			if !p.Status.Has(tc.status) {
				t.Fatal("expired")
			}
			if len(p.Status.SaveForLogout(s.tick, time.Now(), p.ID)) != 0 {
				t.Fatal("persisted")
			}
			p.Player.Char.Skills[tc.skill] = 2
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			p.Status.Each(func(a domain.Active) {
				if a.Def.ID == tc.status && a.Def.SkillSnapshotRank != 2 {
					t.Fatal("rank not rebuilt")
				}
			})
			p.HP = 0
			s.syncLegendaryPermanentBuffs(p)
			if p.Status.Has(tc.status) {
				t.Fatal("dead buff")
			}
			p.HP = 1
			s.syncLegendaryPermanentBuffs(p)
			if !p.Status.Has(tc.status) {
				t.Fatal("revive missing")
			}
			delete(p.Player.Char.Skills, tc.skill)
			s.invalidateCombatAttributes(p)
			s.refreshSkillPersistentEffects(p)
			if p.Status.Has(tc.status) {
				t.Fatal("respec retained")
			}
			p.Player.Char.Skills[tc.skill] = 1
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			slot := domain.SlotWeapon
			if tc.power[3] == 'D' {
				slot = domain.SlotBody
			}
			worn := p.Player.Worn.At(slot)
			worn.Durability = 0
			p.Player.Worn.Set(slot, worn)
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			if p.Status.Has(tc.status) {
				t.Fatal("broken equipment retained passive")
			}
			worn.Durability = worn.MaxDurability
			p.Player.Worn.Set(slot, worn)
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			if !p.Status.Has(tc.status) {
				t.Fatal("repaired equipment missing passive")
			}
			p.Player.Worn = domain.NewEquipSet()
			s.invalidateCombatAttributes(p)
			s.syncLegendaryPermanentBuffs(p)
			if p.Status.Has(tc.status) {
				t.Fatal("unequip retained")
			}
		})
	}
}

func TestLegendaryPermanentPoisonAndWrongClass(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Assassin, domain.Learned{10211: 3}, "CK-A10")
	s.syncLegendaryPermanentBuffs(p)
	var poison domain.Active
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoisonHit {
			poison = a
		}
	})
	if poison.Def.LegendaryPermanentPower != "CK-A10" || poison.Def.PoisonOnHitChance <= 0 || poison.Def.SkillSnapshotRank != 3 {
		t.Fatalf("bad permanent learned poison: %+v", poison)
	}
	before := poison.Def.PoisonOnHitChance
	for i := 0; i < 3; i++ {
		s.stepLegendaryPowers()
	}
	count := 0
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoisonHit {
			count++
			if a.Def.PoisonOnHitChance != before {
				t.Fatal("poison chance grew")
			}
		}
	})
	if count != 1 {
		t.Fatal("native poison stacked", count)
	}
	p.Player.Char.Race = domain.Warlock
	s.invalidateCombatAttributes(p)
	s.syncLegendaryPermanentBuffs(p)
	if p.Status.Has(domain.StatusPoisonHit) {
		t.Fatal("other-class buff survived")
	}
}

func TestLegendaryPermanentThreePreparationsCoexist(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{10418: 2, 10419: 3, 10420: 4}, "SS-U03", "SS-D10", "SS-A30")
	s.syncLegendaryPermanentBuffs(p)
	for _, id := range []domain.StatusID{1026, 1027, 1028} {
		if !p.Status.HasEffective(id) {
			t.Fatal("preparations conflict", id)
		}
	}
	for i := 0; i < 3; i++ {
		damage, duration, cast, removed := p.Status.ConsumeAttackModifiers(true, true, true, true)
		if damage <= 0 || duration <= 0 || cast <= 0 || len(removed) > 0 {
			t.Fatal("combined preparation consumption", damage, duration, cast, removed)
		}
	}
	saved := p.Status.SaveForLogout(s.tick, time.Now(), p.ID)
	p.Status = domain.RestoreSavedStatuses(saved, s.tick, time.Now(), p.ID)
	if p.Status.Has(1026) {
		t.Fatal("saved equipment buff")
	}
	s.syncLegendaryPermanentBuffs(p)
	for _, id := range []domain.StatusID{1026, 1027, 1028} {
		if !p.Status.HasEffective(id) {
			t.Fatal("relog recreation", id)
		}
	}
}

func TestLegendaryPermanentManualRecastPreservesResources(t *testing.T) {
	for _, tc := range []struct {
		power       string
		prep, spell domain.SkillID
		race        domain.Race
	}{
		{"SS-U03", 10418, 10428, domain.Warlock}, {"SS-D10", 10419, 10401, domain.Warlock}, {"SS-A30", 10420, 10401, domain.Warlock},
		{"YS-U02", 10322, 10301, domain.Healer}, {"YS-A29", 10328, 10319, domain.Healer}, {"YS-A30", 10329, 10319, domain.Healer},
	} {
		t.Run(tc.power, func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, tc.race, domain.Learned{tc.prep: 1, tc.spell: 1}, tc.power)
			s.refreshStats(p)
			p.HP = p.MaxHP
			p.MP = p.MaxMP
			hp, mp, maxHP, maxMP := p.HP, p.MP, p.MaxHP, p.MaxMP
			preparationStrength := func() int32 {
				var strength int32
				p.Status.Each(func(a domain.Active) {
					if a.Def.LegendaryPermanentPower == tc.power {
						strength = a.Def.NextSkillDamagePct + a.Def.NextStatusDurationPct + a.Def.NextCastTimeReductionPct
					}
				})
				return strength
			}
			strength := preparationStrength()
			nativeRepairCast(t, s, p, tc.prep, p.ID)
			if p.HP != hp || p.MaxHP != maxHP || p.MaxMP != maxMP || p.MP <= 0 || mp-p.MP > maxMP/2 {
				t.Fatalf("recast changed resources unexpectedly: hp %d/%d max %d/%d mp %d/%d max %d/%d", p.HP, hp, p.MaxHP, maxHP, p.MP, mp, p.MaxMP, maxMP)
			}
			if preparationStrength() != strength {
				t.Fatal("preparation amplified itself on recast", strength, preparationStrength())
			}
			target := m.ID
			if tc.spell == 10301 {
				target = p.ID
			}
			nativeRepairCast(t, s, p, tc.spell, target)
			if p.HP != hp || p.MaxHP != maxHP || p.MaxMP != maxMP || p.MP <= 0 {
				t.Fatal("second cast corrupted resources", p.HP, p.MP, p.MaxHP, p.MaxMP)
			}
		})
	}
}

func TestLegendaryPermanentPreparationTracksSpecialWeaponSwap(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10322: 1}, "YS-U02")
	// Keep the power source and skill ranks stable while changing only the
	// independent named weapon mechanism that modifies native concentration.
	power := p.Player.Worn.At(domain.SlotWeapon)
	powerDef := s.items[power.Item]
	powerDef.Equip.Slot = int32(domain.SlotRing)
	s.items[power.Item] = powerDef
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
	p.Player.Worn.Set(domain.SlotRing, power)
	weapon := domain.ItemID(1800001326)
	s.items[weapon] = domain.ItemDef{ID: weapon, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 100}}
	assertBonus := func(want int32) {
		t.Helper()
		s.invalidateCombatAttributes(p)
		s.syncLegendaryPermanentBuffs(p)
		found := false
		p.Status.Each(func(a domain.Active) {
			if a.Def.LegendaryPermanentPower == "YS-U02" {
				found = true
				if a.Def.SpecialWeaponBuffBonusPct != want {
					t.Errorf("concentration weapon bonus = %d, want %d", a.Def.SpecialWeaponBuffBonusPct, want)
				}
			}
		})
		if !found {
			t.Fatal("missing permanent concentration")
		}
	}
	assertBonus(0)
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{UID: 1326, Item: weapon, Count: 1, Durability: 100, MaxDurability: 100})
	if s.specialWeapon(p) != 1326 {
		t.Fatal("named weapon fixture is not active")
	}
	assertBonus(25)
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
	assertBonus(0)
}
