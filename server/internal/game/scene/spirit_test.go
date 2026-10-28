package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"strconv"
	"testing"
)

func spiritFixture(t *testing.T) (*Scene, *fakeSink, *cubeTestBackend, domain.SpiritDefinition) {
	s, sink, b := cubeSceneFixture(t)
	p := s.players[1]
	p.Player.EndgameSpirit = true
	s.id.MapID = 7
	p.Pos.MapID = 7
	d, _ := domain.SpiritDefinitionForKind(9)
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: d.NPCName, Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: d.NPCID, Script: d.NPCScript}}
	for _, id := range []domain.ItemID{d.ItemID, d.EssenceID, d.SeparatorID} {
		s.items[id] = domain.ItemDef{ID: id, Name: "灵材料", InventoryTabKnown: true, Stackable: true}
	}
	sd := s.items[d.ItemID]
	sd.Stackable = false
	sd.InstanceKind = domain.ItemInstanceSpirit
	s.items[d.ItemID] = sd
	st := domain.NewStack(sd, 1)
	st.UID = 700
	p.Player.Bag.Set(5, st)
	p.Player.Bag.Set(6, domain.Stack{Item: d.EssenceID, Count: 500})
	p.Player.Bag.Set(7, domain.Stack{Item: d.SeparatorID, Count: 2})
	return s, sink, b, d
}
func spiritRequest(d domain.SpiritDefinition, op string) domain.EndgameRequest {
	return domain.EndgameRequest{Op: op, RequestID: "spirit-request-0001", ExpectedRevision: "0", NPCEntityID: 77, SpiritItem: &domain.SpiritSelection{Tab: 0, Slot: 5, ItemID: d.ItemID, UID: "700"}, SpiritMaterial: &domain.SpiritSelection{Tab: 0, Slot: 6, ItemID: d.EssenceID}}
}
func TestSpiritAtomicUnsealAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			s, sink, b, d := spiritFixture(t)
			p := s.players[1]
			before := p.Player.Bag.Clone()
			req := spiritRequest(d, "spirit.unseal")
			s.onEndgameSpirit(p, req)
			if b.commits != 1 {
				t.Fatal("no commit", cubeTake(s, sink))
			}
			if !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("published before commit")
			}
			planned := b.plan.Snapshot.Bag.At(5)
			if planned.Spirit.AttrCount < 2 || b.plan.Snapshot.Bag.At(6).Count != 498 {
				t.Fatal("invalid plan")
			}
			if fail {
				b.result <- domain.EndgameCommitResult{Err: errors.New("storage failure")}
			} else {
				b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			}
			s.finishEndgame(false)
			if fail && !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("failure consumed")
			}
			if !fail && p.Player.Bag.At(5) != planned {
				t.Fatal("not applied")
			}
		})
	}
}
func TestSpiritNPCAndSelectionGuards(t *testing.T) {
	for _, mode := range []string{"remote", "wrongnpc", "material", "uid", "oldclient"} {
		t.Run(mode, func(t *testing.T) {
			s, _, b, d := spiritFixture(t)
			p := s.players[1]
			r := spiritRequest(d, "spirit.unseal")
			switch mode {
			case "remote":
				p.Pos.X += 10000
			case "wrongnpc":
				s.entities[77].NPC.ConfigID++
			case "material":
				r.SpiritMaterial.ItemID++
			case "uid":
				r.SpiritItem.UID = "0700"
			case "oldclient":
				p.Player.EndgameSpirit = false
			}
			s.onEndgameSpirit(p, r)
			if b.commits != 0 {
				t.Fatal("invalid request committed")
			}
		})
	}
}
func TestSpiritFusionSplitRoundTrip(t *testing.T) {
	s, sink, b, d := spiritFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(5)
	var err error
	st.Spirit, err = domain.UnsealSpirit(d.Kind, d.Weights(), false, func(n int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	st.Spirit, err = st.Spirit.Process(func(n int) int { return n - 1 })
	if err != nil {
		t.Fatal(err)
	}
	p.Player.Bag.Set(5, st)
	gear := p.Player.Bag.At(200)
	r := spiritRequest(d, "spirit.fuse")
	r.TargetUID = "42"
	s.onEndgameSpirit(p, r)
	if b.commits != 1 {
		t.Fatal("fuse", cubeTake(s, sink))
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	fused := p.Player.Bag.At(200)
	if fused.FusedSpirit.Stack() != st || !p.Player.Bag.At(5).Empty() {
		t.Fatal("lost state on fuse")
	}
	r.Op = "spirit.separate"
	r.RequestID = "spirit-request-0002"
	r.ExpectedRevision = "1"
	r.SpiritItem = &domain.SpiritSelection{Tab: 0, Slot: 7, ItemID: d.SeparatorID}
	s.onEndgameSpirit(p, r)
	if b.commits != 2 {
		t.Fatal("split", cubeTake(s, sink))
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	actual := p.Player.Bag.At(200)
	if actual != gear {
		t.Fatalf("gear changed %#v %#v", actual, gear)
	}
	found := false
	p.Player.Bag.Each(func(_ int, x domain.Stack) {
		if x.UID == st.UID {
			found = x == st
		}
	})
	if !found {
		t.Fatal("spirit not returned intact")
	}
}

func TestSpiritLegacyFixedSplitPreservesTemplate(t *testing.T) {
	s, sink, b, d := spiritFixture(t)
	p := s.players[1]
	sd := s.items[d.ItemID]
	sd.EquipmentSoul = &domain.EquipmentSoulDef{TargetSlot: domain.SlotWeapon, Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: 123}}}
	s.items[d.ItemID] = sd
	gear := p.Player.Bag.At(200)
	gear.FusedSoul = d.ItemID
	p.Player.Bag.Set(200, gear)
	r := spiritRequest(d, "spirit.separate")
	r.TargetUID = "42"
	r.SpiritItem = &domain.SpiritSelection{Tab: 0, Slot: 7, ItemID: d.SeparatorID}
	s.onEndgameSpirit(p, r)
	if b.commits != 1 {
		t.Fatal("legacy split", cubeTake(s, sink))
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	var old domain.Stack
	var oldSlot int
	p.Player.Bag.Each(func(slot int, x domain.Stack) {
		if x.Spirit.LegacyFixed {
			old = x
			oldSlot = slot
		}
	})
	if old.UID <= 0 || old.Item != d.ItemID || old.Spirit.CanUnseal() {
		t.Fatal("legacy state lost", old)
	}
	r.Op = "spirit.fuse"
	r.RequestID = "spirit-legacy-refuse"
	r.ExpectedRevision = "1"
	r.SpiritItem = &domain.SpiritSelection{Tab: 0, Slot: int32(oldSlot), ItemID: old.Item, UID: strconv.FormatInt(old.UID, 10)}
	s.onEndgameSpirit(p, r)
	if b.commits != 2 {
		t.Fatal("legacy refuse", cubeTake(s, sink))
	}
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	if !p.Player.Bag.At(200).FusedSpirit.State.LegacyFixed {
		t.Fatal("legacy identity lost")
	}
}

func TestSpiritAppearanceUsesAttachedStateAndPreservesLayers(t *testing.T) {
	for _, enough := range []bool{false, true} {
		t.Run(strconv.FormatBool(enough), func(t *testing.T) {
			s, sink, b, d := spiritFixture(t)
			p := s.players[1]
			state := domain.SpiritState{Kind: 9, AttrCount: 4, Attrs: [4]int32{domain.AttrSTR, domain.AttrAGI, domain.AttrINT, domain.AttrVIT}}
			for row := 0; row < 4; row++ {
				for step := 0; step < 10; step++ {
					state.Rolls[row][step] = 20
				}
			}
			if !enough {
				state.Rolls[0][0] = 19
			}
			st := p.Player.Bag.At(5)
			st.Spirit = state
			gear := p.Player.Bag.At(200)
			gear.FusedSoul = d.ItemID
			gear.FusedSpirit = domain.SpiritAttachmentOf(st)
			gear.FusedAppearance = 555
			p.Player.Bag.Set(200, gear)
			p.Player.Bag.Set(5, domain.Stack{})
			ap := domain.ItemID(13698)
			s.items[ap] = domain.ItemDef{ID: ap, InventoryTabKnown: true, Stackable: true}
			p.Player.Bag.Set(8, domain.Stack{Item: ap, Count: 1})
			r := spiritRequest(d, "spirit.appearance")
			r.TargetUID = "42"
			r.SpiritItem = &domain.SpiritSelection{Tab: 0, Slot: 8, ItemID: ap}
			s.onEndgameSpirit(p, r)
			if !enough {
				if b.commits != 0 || p.Player.Bag.At(8).Empty() {
					t.Fatal("below inclusive threshold consumed")
				}
				return
			}
			if b.commits != 1 {
				t.Fatal("appearance", cubeTake(s, sink))
			}
			next := b.plan.Snapshot.Bag.At(200)
			next.SpiritAppearance = 0
			if next != gear {
				t.Fatal("appearance changed other layers")
			}
			if b.plan.Snapshot.Bag.At(200).SpiritAppearance != ap {
				t.Fatal("missing appearance")
			}
		})
	}
}

func TestSpiritPreFeatureBaseFusionKeepsFixedMeaning(t *testing.T) {
	s, sink, b, d := spiritFixture(t)
	p := s.players[1]
	old := p.Player.Bag.At(5)
	old.Spirit = domain.SpiritState{}
	old.InstanceKind = domain.ItemInstanceEquipment
	old.Bound = true
	p.Player.Bag.Set(5, old)
	r := spiritRequest(d, "spirit.fuse")
	r.TargetUID = "42"
	s.onEndgameSpirit(p, r)
	if b.commits != 1 {
		t.Fatal("old fixed base fusion", cubeTake(s, sink))
	}
	next := b.plan.Snapshot.Bag.At(200)
	if !next.FusedSpirit.State.LegacyFixed || !next.FusedSpirit.Bound || !next.Bound || next.FusedSpirit.UID <= 0 || next.FusedSpirit.UID == old.UID || next.UID != 42 {
		t.Fatal("fixed meaning or identity compatibility lost", next)
	}
	if !b.plan.Snapshot.Bag.At(5).Empty() {
		t.Fatal("old base not consumed")
	}
}

func TestSpiritPreFeatureCultivationNotDiscarded(t *testing.T) {
	s, _, b, d := spiritFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(5)
	st.Spirit = domain.SpiritState{}
	st.InstanceKind = domain.ItemInstanceEquipment
	st.RefineLevel = 1
	p.Player.Bag.Set(5, st)
	r := spiritRequest(d, "spirit.fuse")
	r.TargetUID = "42"
	s.onEndgameSpirit(p, r)
	if b.commits != 0 || p.Player.Bag.At(5) != st {
		t.Fatal("legacy cultivation silently consumed")
	}
}
