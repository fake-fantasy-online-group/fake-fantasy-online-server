package domain

import (
	"reflect"
	"strconv"
	"testing"
)

func lifeCubeFixture(t *testing.T, operation string, materials []CubeMaterialDef) (CubeContext, EndgameRequest) {
	ctx := cubeFixture()
	st, def, table := lifeFixture(t)
	def.InventoryTab = 2
	def.InventoryTabKnown = true
	def.InstanceKind = ItemInstanceEquipment
	def.Name = "暗金"
	def.Equip.SocketLimit = 5
	st.InstanceKind = ItemInstanceEquipment
	st.UID = 42
	ctx.Items[def.ID] = def
	ctx.EquipmentRoll = table
	ctx.Bag.Set(200, st)
	for i := 0; i < 10; i++ {
		id := ItemID(4001 + 3*i)
		ctx.Items[id] = ItemDef{ID: id, Name: "原矿", Stackable: true, InventoryTabKnown: true, InventoryTab: 1}
	}
	req := EndgameRequest{Op: "cube.quote", CubeInputs: []CubeInput{{UID: "42"}}}
	for i, m := range materials {
		if m.ItemID >= FirstAstralRune && m.ItemID <= LastAstralRune {
			st := cubeRune(ctx, m.ItemID, int64(501+i))
			st.Count = m.Count + 4
			ctx.Bag.Set(110+i, st)
			req.CubeInputs = append(req.CubeInputs, CubeInput{UID: strconv.Itoa(501 + i)})
		} else {
			ctx.Items[m.ItemID] = ItemDef{ID: m.ItemID, Name: "材料", Stackable: true, InventoryTabKnown: true, InventoryTab: 1}
			ctx.Bag.Set(120+i, Stack{Item: m.ItemID, Count: m.Count + 4})
			req.CubeInputs = append(req.CubeInputs, CubeInput{ItemID: m.ItemID})
		}
	}
	var err error
	ctx.Rules, err = NewCubeRuleTable([]CubeRecipeDef{{ID: 13001, Name: "生活炼化", Description: "炼化", Operation: operation, TargetRequired: true, Materials: materials}}, ctx.Items)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, req
}
func TestLifeCubeAutomaticSelectedStackCostsAndNoPreviewRNG(t *testing.T) {
	for _, count := range []int32{-1, 0, 1, 999999} {
		ctx, req := lifeCubeFixture(t, "life.affix.value", []CubeMaterialDef{{FirstAstralRune, 3}})
		for i := range req.CubeInputs {
			req.CubeInputs[i].Count = count
		}
		before := ctx.Bag.Clone()
		calls := 0
		ctx.Intn = func(n int) int { calls++; return n - 1 }
		choices, plans, err := MatchCubeInputs(ctx, req)
		if err != nil || len(plans) != 1 {
			t.Fatalf("%v %d", err, len(plans))
		}
		if calls != 0 || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("preview mutated or rolled")
		}
		plan, err := PlanCube(ctx, choices[0], true)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Bag.At(110).Count != 4 || plan.Target.Legendary.Affixes[0].Affix.Value != plan.Target.Legendary.Bounds[0].Max || calls == 0 {
			t.Fatal("wrong debit or roll")
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("execution mutated caller before commit")
		}
	}
}
func TestLifeCubeCostsFailuresAndIdentity(t *testing.T) {
	for _, name := range []string{"short", "locked", "duplicate", "fixed", "wrong", "extra"} {
		t.Run(name, func(t *testing.T) {
			ctx, req := lifeCubeFixture(t, "life.affix.value", []CubeMaterialDef{{FirstAstralRune, 3}})
			switch name {
			case "short":
				s := ctx.Bag.At(110)
				s.Count = 2
				ctx.Bag.Set(110, s)
			case "locked":
				s := ctx.Bag.At(110)
				s.Locked = true
				ctx.Bag.Set(110, s)
			case "duplicate":
				req.CubeInputs = append(req.CubeInputs, req.CubeInputs[1])
			case "fixed":
				s := ctx.Bag.At(200)
				s.Legendary.Bounds[0].Min = s.Legendary.Bounds[0].Max
				s.Legendary.Affixes[0].Affix.Value = s.Legendary.Bounds[0].Max
				ctx.Bag.Set(200, s)
			case "wrong":
				s := ctx.Bag.At(110)
				s.Item = FirstAstralRune + 7
				ctx.Bag.Set(110, s)
			case "extra":
				req.CubeInputs = append(req.CubeInputs, CubeInput{ItemID: CubeStarMarrow})
			}
			before := ctx.Bag.Clone()
			ctx.Intn = func(int) int { t.Fatal("failure consumed RNG"); return 0 }
			_, _, err := MatchCubeInputs(ctx, req)
			if err == nil {
				t.Fatal("accepted invalid input")
			}
			if !reflect.DeepEqual(before, ctx.Bag) {
				t.Fatal("mutated failure")
			}
		})
	}
}
func TestLifeCubeTypeSevenIdentitiesAndTemperShuffle(t *testing.T) {
	mats := []CubeMaterialDef{}
	for i := 0; i < 7; i++ {
		mats = append(mats, CubeMaterialDef{FirstAstralRune + ItemID(i), 1})
	}
	for _, tc := range []struct {
		op string
		m  []CubeMaterialDef
	}{{"life.affix.type", mats}, {"life.affix.temper", []CubeMaterialDef{{4478, 10}}}, {"life.affix.shuffle", []CubeMaterialDef{{4501, 30}}}} {
		t.Run(tc.op, func(t *testing.T) {
			ctx, req := lifeCubeFixture(t, tc.op, tc.m)
			before := ctx.Bag.Clone()
			choices, _, err := MatchCubeInputs(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			p, err := PlanCube(ctx, choices[0], true)
			if err != nil {
				t.Fatal(err)
			}
			if p.Target.Legendary.PowerID != before.At(200).Legendary.PowerID || p.Target.Endgame.Revision != before.At(200).Endgame.Revision+1 {
				t.Fatal("power/revision changed")
			}
			for _, m := range tc.m {
				if before.CountOf(m.ItemID)-p.Bag.CountOf(m.ItemID) != m.Count {
					t.Fatal("wrong cost")
				}
			}
		})
	}
}
func TestLifeCubeSalvageRawOreAndReturnedSocket(t *testing.T) {
	ctx, req := lifeCubeFixture(t, "life.salvage.unique", nil)
	st := ctx.Bag.At(200)
	st.Legendary = LegendaryInstance{}
	st.Endgame.UniqueID = 1001
	st.Endgame.UniqueRollBP = 10000
	ctx.Bag.Set(200, st)
	cubeInstall(&ctx, FirstAstralRune)
	before := ctx.Bag.Clone()
	choices, _, err := MatchCubeInputs(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	p, err := PlanCube(ctx, choices[0], true)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Target.Empty() || !p.Bag.At(200).Empty() || p.Bag.CountOf(FirstAstralRune) != 1 {
		t.Fatal("salvage identities")
	}
	total := int32(0)
	for _, o := range p.Outputs {
		if o.Item >= 4001 && o.Item <= 4028 {
			total += o.Count
		}
	}
	if total != 48 || p.Bag.CountOf(CubeStarMarrow) != before.CountOf(CubeStarMarrow) {
		t.Fatal("wrong raw ore output")
	}
}

func TestLifeCubeSalvageCapacityFailsBeforeRandomDraw(t *testing.T) {
	ctx, req := lifeCubeFixture(t, "life.salvage.unique", nil)
	for i := 100; i < 200; i++ {
		ctx.Bag.Set(i, Stack{Item: CubeStarMarrow, Count: MaxStack})
	}
	before := ctx.Bag.Clone()
	ctx.Intn = func(int) int { t.Fatal("full bag must not sample salvage"); return 0 }
	_, _, err := MatchCubeInputs(ctx, req)
	if err == nil {
		t.Fatal("full bag accepted")
	}
	if !reflect.DeepEqual(before, ctx.Bag) {
		t.Fatal("failed salvage consumed gear")
	}
}
func TestLifeCubeCardConsumedExactlyOnceAndLockedRejected(t *testing.T) {
	mats := []CubeMaterialDef{}
	for i := 0; i < 7; i++ {
		mats = append(mats, CubeMaterialDef{FirstAstralRune + ItemID(i), 1})
	}
	for _, locked := range []bool{false, true} {
		ctx, req := lifeCubeFixture(t, "life.affix.type", mats)
		card := Stack{Item: 6001, UID: 700, Count: 1, InstanceKind: ItemInstanceSocketCard, Locked: locked, Card: CardInstanceState{Initialized: true, Count: 1}}
		card.Card.Effects[0] = CardEffect{Op: 1, Probability: 100, Attr: AttrSTR, Mode: ModeAbsolute, Value: 1}
		ctx.Items[6001] = ItemDef{ID: 6001, Name: "引灵卡", Level: 50, InventoryTabKnown: true, InventoryTab: 1, InstanceKind: ItemInstanceSocketCard}
		ctx.Bag.Set(130, card)
		req.CubeInputs = append(req.CubeInputs, CubeInput{UID: "700"})
		before := ctx.Bag.Clone()
		choices, _, err := MatchCubeInputs(ctx, req)
		if locked {
			if err == nil {
				t.Fatal("locked card accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		p, err := PlanCube(ctx, choices[0], true)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Bag.At(130).Empty() || !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("card consumption not isolated")
		}
	}
}

func TestLifeCubeDirectCardIdentityMustBeCanonical(t *testing.T) {
	mats := []CubeMaterialDef{}
	for i := 0; i < 7; i++ {
		mats = append(mats, CubeMaterialDef{FirstAstralRune + ItemID(i), 1})
	}
	for _, uid := range []string{"0", "-1", "broken", "0700"} {
		ctx, input := lifeCubeFixture(t, "life.affix.type", mats)
		reqs, _, err := MatchCubeInputs(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		req := reqs[0]
		req.ItemUIDs = []string{uid}
		before := ctx.Bag.Clone()
		if _, err = PlanCube(ctx, req, true); err == nil {
			t.Fatalf("accepted card %s", uid)
		}
		if !reflect.DeepEqual(before, ctx.Bag) {
			t.Fatal("bad card identity consumed materials")
		}
	}
}
