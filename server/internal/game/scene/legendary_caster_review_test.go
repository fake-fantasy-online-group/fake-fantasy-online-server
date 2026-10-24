package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
	"testing"
)

// A04 is one extra explosion at the original fireball's location, not a new
// primary-target cast for each secondary victim. Use native ranks and legal gear.
func TestCasterReviewFireballEchoRetainsOriginalPrimary(t *testing.T) {
	for _, primaryPower := range []string{"SS-A05", "TY-A34", "TY-A36"} {
		t.Run(primaryPower, func(t *testing.T) {
			run := func(echo bool) (int32, int32) {
				powers := []string{primaryPower}
				if echo {
					powers = append(powers, "SS-A04")
				}
				s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1}, powers...)
				side := putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
				putMonster(s, 902, 100000, domain.Stats{}, 150, 100)
				nativeRepairCast(t, s, p, 10402, m.ID)
				return 100000 - m.HP, 100000 - side.HP
			}
			primary, side := run(false)
			primaryEcho, sideEcho := run(true)
			if d := primaryEcho - primary; d < primary*70/100-1 || d > primary*70/100+1 {
				t.Fatalf("primary echo %d must be 70%% of original %d", d, primary)
			}
			if d := sideEcho - side; d < side*70/100-1 || d > side*70/100+1 {
				t.Fatalf("secondary echo %d acquired original-primary bonus; original side damage %d", d, side)
			}
		})
	}
}

func TestCasterReviewAcceptedSourceSurvivesRealEquip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		race    domain.Race
		skill   domain.SkillID
		learned domain.Learned
		power   string
	}{
		{"fireball_echo", domain.Warlock, 10402, domain.Learned{10402: 1}, "SS-A04"},
		{"heal_granted_bolt", domain.Healer, 10301, domain.Learned{10301: 1, 10319: 1}, "YS-A23"},
		{"freeze_granted_arrow", domain.Warlock, 10428, domain.Learned{10428: 1, 10409: 1}, "SS-A24"},
		{"teleport_departure_burst", domain.Warlock, 10432, domain.Learned{10418: 1, 10432: 5, 10401: 1}, "SS-A27"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(change string) (int32, int32) {
				basePower := "TY-A26"
				if tc.skill == 10301 {
					basePower = "TY-A38"
				}
				s, p, m := nativeRepairFixture(t, tc.race, tc.learned, tc.power, basePower)
				for id, item := range s.items {
					item.InventoryTab, item.InventoryTabKnown = 2, true
					s.items[id] = item
				}
				weapon := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
				weapon.Equip.Bonus.MAtk = 100
				s.items[weapon.ID] = weapon
				s.refreshStats(p)
				p.HP = p.MaxHP
				p.MP = p.MaxMP
				target := m.ID
				if tc.skill == 10301 {
					target = p.ID
				}
				cmd := UseSkill{ID: p.ID, Skill: tc.skill, Target: target}
				if tc.skill == 10432 {
					to := domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 300, Y: p.Pos.Y}
					cmd.Target = 0
					cmd.At = &to
				}
				s.onUseSkill(cmd)
				if len(s.casts) != 1 {
					t.Fatal("native parent not admitted", s.outbox)
				}
				if change == "upgrade" || change == "new_power" {
					slot := domain.SlotRing
					bonus := domain.Stats{MAtk: 1000, MaxMP: 1000}
					power := ""
					if change == "new_power" {
						slot = domain.SlotHead
						bonus = domain.Stats{}
						power = "TY-A08"
					}
					gear := domain.ItemDef{ID: 991700, Name: "review equipment", InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Bonus: bonus}}
					s.items[gear.ID] = gear
					p.Player.Bag.Set(0, domain.Stack{UID: 991700, Item: gear.ID, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: power}})
					s.onEquip(Equip{ID: p.ID, BagSlot: 0})
					if p.Player.Worn.At(slot).Item != gear.ID || len(s.casts) != 1 {
						t.Fatal("real mid-windup Equip unavailable", s.outbox)
					}
					if power != "" && len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)) != 3 {
						t.Fatal("new power not eligible")
					}
				} else if change == "remove_producer" {
					s.onUnequip(Unequip{ID: p.ID, Slot: domain.SlotWeapon})
					if p.Player.Worn.At(domain.SlotWeapon).Item != 0 || len(s.casts) != 1 {
						t.Fatal("real mid-windup Unequip unavailable")
					}
				}
				s.outbox = nil
				sharedTargetFinishCasts(t, s)
				var direct, child int32
				for _, out := range s.outbox {
					if e, ok := out.ev.(event.DamageDealt); ok && e.Dst == m.ID {
						if e.Origin == domain.DamageProcOrigin {
							child += e.Amount
						} else {
							direct += e.Amount
						}
					}
				}
				return direct, child
			}
			direct, child := run("")
			if child <= 0 {
				t.Fatal("native child did not execute")
			}
			for _, change := range []string{"upgrade", "new_power"} {
				a, b := run(change)
				if a != direct || b != child {
					t.Fatalf("accepted parent/child changed after %s: %d/%d -> %d/%d", change, direct, child, a, b)
				}
			}
			_, removed := run("remove_producer")
			if removed != 0 {
				t.Fatal("unequipped producer still generated child", removed)
			}
		})
	}
}

func TestCasterReviewChildDonorRankStaysAccepted(t *testing.T) {
	for _, tc := range []struct {
		name         string
		skill, donor domain.SkillID
		learned      domain.Learned
		power        string
	}{
		{"freeze_arrow", 10428, 10409, domain.Learned{10428: 1, 10409: 1}, "SS-A24"},
		{"dispel_fire_basis", 10433, 10401, domain.Learned{10418: 1, 10432: 1, 10433: 5, 10401: 1}, "SS-A28"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(upgrade bool) int32 {
				s, p, m := nativeRepairFixture(t, domain.Warlock, tc.learned, tc.power)
				for id, item := range s.items {
					item.InventoryTab, item.InventoryTabKnown = 2, true
					s.items[id] = item
				}
				s.refreshStats(p)
				p.HP = p.MaxHP
				p.MP = p.MaxMP
				s.onUseSkill(UseSkill{ID: p.ID, Skill: tc.skill, Target: m.ID})
				if len(s.casts) != 1 {
					t.Fatal("native donor parent rejected")
				}
				before := s.rawSkillLevels(p).LevelOf(tc.donor)
				if upgrade {
					u, ok := domain.UniqueForBase(2059, s.endgameCatalog)
					if !ok {
						t.Fatal("native class-rank crown missing")
					}
					d := domain.ItemDef{EndgameCatalog: s.endgameCatalog, ID: u.BaseID, Name: u.BaseName, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: u.Slot, Durable: 100, LevelReq: u.LevelRequirement, Need: domain.Requirement{Level: u.LevelRequirement, Sex: u.Sex, Professions: u.Professions, ProfessionRestricted: !u.Unrestricted, BeginnerAllowed: u.BeginnerAllowed}}}
					s.items[d.ID] = d
					st := domain.NewStack(d, 1)
					st.UID = 991705
					st.Endgame = domain.EndgameItemState{UniqueID: u.ID, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
					p.Player.Bag.Set(0, st)
					s.onEquip(Equip{ID: p.ID, BagSlot: 0})
					if p.Player.Worn.At(domain.EquipSlot(u.Slot)).UID != st.UID || s.rawSkillLevels(p).LevelOf(tc.donor) <= before {
						t.Fatal("native rank item not active", s.outbox)
					}
				}
				s.outbox = nil
				sharedTargetFinishCasts(t, s)
				var amount int32
				for _, out := range s.outbox {
					if e, ok := out.ev.(event.DamageDealt); ok && e.Dst == m.ID && e.Origin == domain.DamageProcOrigin {
						amount += e.Amount
					}
				}
				return amount
			}
			a, b := run(false), run(true)
			if a <= 0 || a != b {
				t.Fatal("accepted donor rank changed after real equip", a, b)
			}

		})
	}
}

func TestCasterReviewUnselectedExplosionDoesNotInventPrimary(t *testing.T) {
	for _, power := range []string{"SS-A05", "TY-A34", "TY-A36"} {
		t.Run(power, func(t *testing.T) {
			run := func(echo bool) int32 {
				powers := []string{power}
				if echo {
					powers = append(powers, "SS-A04")
				}
				s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1}, powers...)
				putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
				putMonster(s, 902, 100000, domain.Stats{}, 150, 100)
				pos := m.Pos
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10402, At: &pos})
				if len(s.casts) != 1 || s.casts[0].primary != 0 {
					t.Fatal("point cast invented a primary")
				}
				sharedTargetFinishCasts(t, s)
				return 100000 - m.HP
			}
			n, with := run(false), run(true)
			if delta := with - n; delta < n*70/100-1 || delta > n*70/100+1 {
				t.Fatal("unselected echo borrowed a primary", n, with)
			}
		})
	}
}

// Native all-rank gloves are legal beside every caster producer. Exercise the
// remaining non-ChildSkill source consumers through admission and real equip.
func TestCasterReviewNonChildDerivedSourceStaysAccepted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		race    domain.Race
		skill   domain.SkillID
		learned domain.Learned
		power   string
	}{
		{"holy_field", domain.Healer, 10317, domain.Learned{10317: 1}, "YS-A07"},
		{"blessing_granted_buffs", domain.Healer, 10307, domain.Learned{10307: 1, 10321: 1, 10311: 1}, "YS-U06"},
		{"burn_shared_passive", domain.Warlock, 10421, domain.Learned{10421: 1, 10431: 1}, "SS-A25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(upgrade bool) string {
				s, p, m := nativeRepairFixture(t, tc.race, tc.learned, tc.power)
				for id, item := range s.items {
					item.InventoryTab, item.InventoryTabKnown = 2, true
					s.items[id] = item
				}
				s.refreshStats(p)
				p.HP, p.MP = p.MaxHP, p.MaxMP
				target := m.ID
				if tc.skill == 10307 {
					target = p.ID
				}
				s.onUseSkill(UseSkill{ID: p.ID, Skill: tc.skill, Target: target})
				if len(s.casts) != 1 {
					t.Fatal("native parent not admitted", s.outbox)
				}
				before := s.rawSkillLevels(p).LevelOf(tc.skill)
				if upgrade {
					u, ok := domain.UniqueForBase(2409, s.endgameCatalog)
					if !ok {
						t.Fatal("native all-rank gloves missing")
					}
					d := domain.ItemDef{EndgameCatalog: s.endgameCatalog, ID: u.BaseID, Name: u.BaseName, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: u.Slot, Durable: 100, LevelReq: u.LevelRequirement, Need: domain.Requirement{Level: u.LevelRequirement, Sex: u.Sex, Professions: u.Professions, ProfessionRestricted: !u.Unrestricted, BeginnerAllowed: u.BeginnerAllowed}}}
					s.items[d.ID] = d
					st := domain.NewStack(d, 1)
					st.UID = 991709
					st.Endgame = domain.EndgameItemState{UniqueID: u.ID, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
					p.Player.Bag.Set(0, st)
					s.onEquip(Equip{ID: p.ID, BagSlot: 0})
					if p.Player.Worn.At(domain.EquipSlot(u.Slot)).UID != st.UID || s.rawSkillLevels(p).LevelOf(tc.skill) <= before {
						t.Fatal("native all-rank upgrade unavailable", s.outbox)
					}
				}
				sharedTargetFinishCasts(t, s)
				if tc.skill == 10317 {
					beforeHP := m.HP
					s.tick += domain.Tick(1000)
					s.stepLegendaryPowers()
					if m.HP >= beforeHP {
						t.Fatal("native holy field did not tick")
					}
					return fmt.Sprint(beforeHP - m.HP)
				}
				recipient := m
				if tc.skill == 10307 {
					recipient = p
				}
				values := []string{}
				recipient.Status.EachEffective(func(a domain.Active) {
					if tc.skill == 10421 && a.Def.ID != 1036 {
						return
					}
					if tc.skill == 10307 && a.Def.SourceSkillID != 10321 && a.Def.SourceSkillID != 10311 {
						return
					}
					values = append(values, fmt.Sprintf("%d/%d/%d/%d/%d/%v", a.Def.ID, a.Def.Level, a.Def.PhysicalDamageReductionPct, a.Def.MagicDamageReductionPct, a.Def.LegendaryModifiers.DamageTickBP, a.Def.Affixes))
				})
				sort.Strings(values)
				if len(values) == 0 {
					t.Fatal("native derived status missing", recipient.Status)
				}
				return fmt.Sprint(values)
			}
			a, b := run(false), run(true)
			if a != b {
				t.Fatalf("accepted derived source changed after real equip: %s -> %s", a, b)
			}
		})
	}
}
