package domain

import (
	"encoding/json"
	"testing"
)

func TestColorFirstBudgetMFNativeAndLowLevel(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	def.Equip.Slot = int32(SlotWeapon)
	for _, level := range []int32{1, 100} {
		def.Equip.LevelReq = level
		for _, mf := range []int64{0, 100, 99999} {
			for fixed := 0; fixed <= 5; fixed++ {
				for count := 0; count <= 5; count++ {
					def.Equip.Affixes = make([]Affix, fixed)
					table.Counts[DefaultColorProfile] = []WeightedAffixCount{{Count: uint8(count), Weight: 100}}
					draws := 0
					holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, mf, lootRulesFixture(),
						func(int64) int64 { draws++; return 0 }, nil, func(int64) int64 { return 0 })
					want := max(0, count-fixed)
					if holes != 0 || len(affixes) != want || draws != 1 {
						t.Fatalf("level%d mf%d fixed%d count%d: holes%d lines%d draws%d want%d", level, mf, fixed, count, holes, len(affixes), draws, want)
					}
				}
			}
		}
	}
}

func TestColorFirstSparsePoolAndDurableRoundTrip(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	def.Equip.Slot = int32(SlotWeapon)
	def.Equip.LevelReq = 100
	table.Counts[DefaultColorProfile] = []WeightedAffixCount{{5, 100}}
	for key, options := range table.Options {
		table.Options[key] = options[:2]
	}
	holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, lootRulesFixture(), func(int64) int64 { return 0 }, nil, func(int64) int64 { return 0 })
	if holes != 0 || len(affixes) != 2 {
		t.Fatal("sparse feasible color changed", holes, affixes)
	}
	st := NewStack(def, 1)
	st.SocketCount = holes
	st.RolledAffixCount = uint8(len(affixes))
	copy(st.RolledAffixes[:], affixes)
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var got Stack
	if err = json.Unmarshal(data, &got); err != nil || got != st {
		t.Fatal("durable fields changed", err)
	}
}

func TestUniquePromotionSocketlessAndNonDestructive(t *testing.T) {
	d := UniqueCatalog(testEndgameCatalog())[0]
	def := uniqueFixture(d)
	st := NewStack(def, 1)
	st.SocketCount = 5
	if !ApplyUnique(&st, def, func(int) int { return 0 }) || st.SocketCount != 0 {
		t.Fatal("new unique has holes", st)
	}
	for _, mutate := range []func(*Stack){
		func(s *Stack) { s.Sockets[4] = 123 }, func(s *Stack) { s.SocketUIDs[4] = 987 }, func(s *Stack) { s.SocketCards[4].Initialized = true },
	} {
		st = NewStack(def, 1)
		mutate(&st)
		before := st
		if ApplyUnique(&st, def, func(int) int { t.Fatal("unsafe promotion consumed RNG"); return 0 }) || st != before {
			t.Fatal("owned socket data changed")
		}
	}
	ctx, req := cubeInputs(CubeOpenSocket)
	st = ctx.Bag.At(200)
	st.Endgame.UniqueID = int32(st.Item)
	ctx.Bag.Set(200, st)
	if _, err := PlanCube(ctx, req, true); CubeErrorCode(err) != "requirements" {
		t.Fatal("unique drilling allowed", err)
	}
}

func TestNaturalEquipmentDoesNotUseSocketRNG(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	def.Equip.Slot = int32(SlotWeapon)
	table.Counts[DefaultColorProfile] = []WeightedAffixCount{{Count: 5, Weight: 1}}
	holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, nil,
		func(int64) int64 { return 0 },
		func(int64) int64 { t.Fatal("natural drop consumed socket RNG"); return 0 },
		func(int64) int64 { return 0 })
	if holes != 0 || len(affixes) != 5 {
		t.Fatal("natural drop lost affixes or gained holes", holes, affixes)
	}
}
