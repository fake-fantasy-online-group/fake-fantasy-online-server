package domain

import (
	"encoding/json"
	"testing"
)

func TestColorFirstSocketWeightsExact(t *testing.T) {
	for m := 0; m <= 5; m++ {
		weights := ColorFirstSocketWeights(m)
		var sum int64
		for i, w := range weights {
			sum += w
			if i > 0 && weights[i-1] <= w {
				t.Fatal("not decreasing", m, weights)
			}
			if i > 1 && weights[i-2]-weights[i-1] != weights[i-1]-w {
				t.Fatal("not arithmetic", weights)
			}
		}
		if m == 0 {
			if weights[0] != 1 {
				t.Fatal(weights)
			}
			continue
		}
		if weights[m]*10 != sum {
			t.Fatal("full socket probability not 10%", m, weights)
		}
		if m == 4 {
			for i, p := range []int64{30, 25, 20, 15, 10} {
				if weights[i]*100 != p*sum {
					t.Fatal(weights)
				}
			}
		}
	}
}

func TestColorFirstBudgetMFNativeAndLowLevel(t *testing.T) {
	table := colorRollTable()
	rule := lootRulesFixture()
	def := colorRollDef()
	def.Equip.Slot = int32(SlotWeapon)
	def.Equip.LevelReq = 100
	for _, mf := range []int64{0, 100, 99999} {
		for fixed := 0; fixed <= 5; fixed++ {
			for count := 0; count <= 5; count++ {
				def.Equip.Affixes = make([]Affix, fixed)
				table.Counts[DefaultColorProfile] = []WeightedAffixCount{{Count: uint8(count), Weight: 100}}
				for _, last := range []bool{false, true} {
					draws := 0
					holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, mf, rule, func(n int64) int64 { draws++; return 0 }, func(n int64) int64 {
						if draws != 1 {
							t.Fatal("sockets before color")
						}
						if last {
							return n - 1
						}
						return 0
					}, func(int64) int64 { return 0 })
					want := max(0, count-fixed)
					if int(holes)+len(affixes) != want {
						t.Fatalf("mf%d fixed%d count%d: holes%d lines%d want%d", mf, fixed, count, holes, len(affixes), want)
					}
					if last && int(holes) != want {
						t.Fatal("full feasible socket outcome unreachable")
					}
				}
			}
		}
	}
	def.Equip.Affixes = nil
	def.Equip.LevelReq = 1
	table.Counts[DefaultColorProfile] = []WeightedAffixCount{{5, 100}}
	h, a := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, rule, func(int64) int64 { return 0 }, func(n int64) int64 { return n - 1 }, func(int64) int64 { return 0 })
	if int(h) != 5 || int(h)+len(a) != 5 {
		t.Fatal("legacy level cap changed color-first distribution", h, len(a))
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
	for _, last := range []bool{false, true} {
		holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, lootRulesFixture(), func(int64) int64 { return 0 }, func(n int64) int64 {
			if last {
				return n - 1
			}
			return 0
		}, func(int64) int64 { return 0 })
		if int(holes)+len(affixes) != 2 {
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

func TestColorFirstEverySocketBucketHasExactWeight(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	def.Equip.Slot = int32(SlotWeapon)
	def.Equip.LevelReq = 1
	for n := 1; n <= 5; n++ {
		table.Counts[DefaultColorProfile] = []WeightedAffixCount{{Count: uint8(n), Weight: 1}}
		weights := ColorFirstSocketWeights(n)
		var total int64
		for _, w := range weights {
			total += w
		}
		got := make([]int64, n+1)
		for draw := int64(0); draw < total; draw++ {
			holes, affixes := table.RollColorFirstEquipment(def, DefaultColorProfile, 0, nil, func(int64) int64 { return 0 }, func(bound int64) int64 {
				if bound != total {
					t.Fatal("wrong normalization", bound, total)
				}
				return draw
			}, func(int64) int64 { return 0 })
			got[holes]++
			if int(holes)+len(affixes) != n {
				t.Fatal("bucket changed color", draw, holes, len(affixes))
			}
		}
		for i, w := range weights {
			if got[i] != w {
				t.Fatalf("N%d bucket%d: got%d want%d", n, i, got[i], w)
			}
		}
	}
}
