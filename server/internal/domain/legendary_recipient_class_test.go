package domain

import "testing"

func TestLegendaryRecipientCandidatesIntersectBaseSlotAndSource(t *testing.T) {
	def, table := sourceLevelFixture(t)
	def.Equip.LevelReq = 60
	for race := Warrior; race <= Warlock; race++ {
		recipient := LegendaryClassForCharacter(&Character{Race: race, Employed: true, EmploymentKnown: true})
		for _, slot := range []EquipSlot{SlotWeapon, SlotTwoHand, SlotBody, SlotHead, SlotRing, SlotNeck, SlotGlove, SlotShoe, SlotFace, SlotBag, SlotShield} {
			def.Equip.Slot, def.Equip.Need.Professions = int32(slot), 31
			powers := LegendaryCandidatesForRecipientAtLevel(def, 43, recipient)
			var universal, own bool
			for index, power := range powers {
				if !LegendaryPowerEligibleAtLevel(power, def, 43) || !recipient.allows(power) {
					t.Fatalf("recipient %d slot %d received %s", race, slot, power.ID)
				}
				universal = universal || power.Class == "全职业"
				own = own || power.Class != "全职业"
				st := NewStack(def, 1)
				first := true
				if !table.ApplyLegendaryForRecipientAtLevel(&st, def, 43, recipient, func(int) int {
					if first {
						first = false
						return index
					}
					return 0
				}) || st.Legendary.PowerID != power.ID || st.Legendary.Validate() != nil || st.Endgame.SourceLevel != 43 || st.Legendary.Tier != LegendaryTierSecret || st.RolledAffixCount != 0 {
					t.Fatal("recipient roll lost power, six affixes, provenance or variant", st)
				}
			}
			wantOwn := slot == SlotWeapon || slot == SlotTwoHand || slot == SlotBody || slot == SlotHead
			if !universal || own != wantOwn {
				t.Fatal("wrong slot-specific class support", race, slot, universal, own)
			}
		}
	}
	def.Equip.Slot, def.Equip.Need.Professions = int32(SlotWeapon), 1 // warrior base, assassin recipient
	for _, recipient := range []LegendaryRecipientClass{0, 4, 3, 32} {
		powers := LegendaryCandidatesForRecipientAtLevel(def, 100, recipient)
		if len(powers) == 0 {
			t.Fatal("legal universal cross-class base was filtered away")
		}
		for _, power := range powers {
			if power.Class != "全职业" {
				t.Fatal("other or unknown recipient got class power", recipient, power.ID)
			}
		}
	}
	def.Equip.Need.Professions = 31
	for _, power := range LegendaryCandidatesForRecipientAtLevel(def, 1, 1) {
		if power.Class != "全职业" {
			t.Fatal("recipient bypassed monster source-level requirement", power.ID)
		}
	}
	if len(LegendaryCandidatesForRecipientAtLevel(def, 1, 1)) == 0 {
		t.Fatal("universal minimum-level gate revived")
	}
}

func TestLegendaryRecipientUnknownAndOwnedInstances(t *testing.T) {
	for _, ch := range []*Character{nil, {Race: Race(255), Employed: true}, {Race: Warrior, EmploymentKnown: true}} {
		if LegendaryClassForCharacter(ch) != 0 {
			t.Fatal("invented recipient profession", ch)
		}
	}
	def, table := sourceLevelFixture(t)
	def.Equip.Slot, def.Equip.Need.Professions = int32(SlotWeapon), 31
	st := NewStack(def, 1)
	if !table.ApplyLegendaryForRecipientAtLevel(&st, def, 100, 4, func(int) int { return 0 }) {
		t.Fatal("roll failed")
	}
	before := st
	if table.ApplyLegendaryForRecipientAtLevel(&st, def, 100, 1, func(int) int { t.Fatal("existing item rerolled on recipient change"); return 0 }) || st != before {
		t.Fatal("owned/traded item changed")
	}
}

func TestLegendaryRecipientCubeAwakeningUsesActor(t *testing.T) {
	def, table := sourceLevelFixture(t)
	def.Equip.Slot, def.Equip.LevelReq, def.Equip.Need.Professions = int32(SlotWeapon), 60, 31
	for _, recipient := range []LegendaryRecipientClass{0, 1, 4, 16} {
		ctx, req := cubeInputs(CubeAwakenUnique)
		old := ctx.Items[def.ID]
		def.EndgameCatalog, def.InventoryTab, def.InventoryTabKnown = old.EndgameCatalog, old.InventoryTab, true
		ctx.Items[def.ID], ctx.EquipmentRoll, ctx.Level, ctx.RecipientClass = def, table, 100, recipient
		st := ctx.Bag.At(200)
		st.Endgame.SourceLevel = 43
		ctx.Bag.Set(200, st)
		ctx.Intn = func(int) int { return 0 }
		if _, err := PlanCube(ctx, req, false); err != nil {
			t.Fatal(err)
		}
		plan, err := PlanCube(ctx, req, true)
		if err != nil {
			t.Fatal(err)
		}
		power, ok := LegendaryPower(plan.Target.Legendary.PowerID)
		if !ok || !recipient.allows(power) || plan.Target.Endgame.SourceLevel != 43 {
			t.Fatal("cube borrowed base/player level or other class", power.ID)
		}
	}
}
