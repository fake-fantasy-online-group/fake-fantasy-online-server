package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"testing"
)

type astralTestBackend struct {
	result  chan domain.EndgameCommitResult
	plan    domain.EndgameCommitPlan
	receipt *domain.EndgameReceipt
	commits int
}

func (b *astralTestBackend) Load(int64) <-chan domain.EndgameLoadResult {
	ch := make(chan domain.EndgameLoadResult, 1)
	ch <- domain.EndgameLoadResult{}
	return ch
}
func (b *astralTestBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	ch := make(chan domain.EndgameReceiptLoadResult, 1)
	ch <- domain.EndgameReceiptLoadResult{Receipt: b.receipt}
	return ch
}
func (b *astralTestBackend) LoadRun(string) <-chan domain.EndgameRunLoadResult {
	ch := make(chan domain.EndgameRunLoadResult, 1)
	ch <- domain.EndgameRunLoadResult{}
	return ch
}
func (b *astralTestBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.plan = p
	b.commits++
	b.result = make(chan domain.EndgameCommitResult, 1)
	return b.result
}
func astralTake(s *Scene, sink *fakeSink) []event.Event { s.flush(); return sink.take() }
func astralSceneFixture(t *testing.T) (*Scene, *fakeSink, *astralTestBackend) {
	t.Helper()
	s := newTestScene(t, nil)
	s.items = map[domain.ItemID]domain.ItemDef{}
	for _, d := range astralRuneItemsFixture(7) {
		s.items[d.ID] = d
	}
	for _, d := range cubeMaterialDefinitionsFixture() {
		s.items[d.ID] = d
	}
	s.items[999001] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999001, Name: "星阵测试剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), LevelReq: 50, Durable: 100}}
	sink := join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 60
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Money = moneyFromInventory(100000)
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	target := domain.NewStack(s.items[999001], 1)
	target.UID = 42
	target.SocketCount = 2
	p.Player.Bag.Set(200, target)
	r := domain.NewStack(s.items[domain.FirstAstralRune], 1)
	r.UID = 51
	p.Player.Bag.Set(100, r)
	p.Player.Bag.Set(102, domain.Stack{Item: domain.CubeStarMarrow, Count: 1})
	p.Player.Bag.Set(103, domain.Stack{Item: domain.CubePurifiedDust, Count: 1})
	b := &astralTestBackend{}
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	astralTake(s, sink)
	return s, sink, b
}
func TestAstralSocketCommitReplayAndStorageFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit_replay", true: "rollback"}[fail], func(t *testing.T) {
			s, sink, b := astralSceneFixture(t)
			p := s.players[1]
			req := domain.EndgameRequest{Op: "socket.insert", RequestID: "astral-insert-00001", ExpectedRevision: "0", TargetUID: "42", RuneUID: "51", Socket: 0}
			before := p.Player.Bag.Clone()
			s.onEndgameSocket(p, req)
			if b.commits != 1 || !p.Player.TradeBusy || !reflect.DeepEqual(before, p.Player.Bag) {
				t.Fatal("candidate leaked before durable success")
			}
			if b.plan.Snapshot.Bag.At(200).SocketUIDs[0] != 51 || !b.plan.Snapshot.Bag.At(100).Empty() {
				t.Fatal("candidate wrong")
			}
			if fail {
				b.result <- domain.EndgameCommitResult{Err: errors.New("injected storage failure")}
				s.finishEndgame(false)
				if !reflect.DeepEqual(before, p.Player.Bag) || p.Player.TradeBusy {
					t.Fatal("failure lost card")
				}
				return
			}
			b.receipt = &b.plan.Receipt
			b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			s.finishEndgame(false)
			astralTake(s, sink)
			if p.Player.Bag.At(200).SocketUIDs[0] != 51 || p.Player.TradeBusy {
				t.Fatal("commit unpublished")
			}
			s.onEndgame(EndgameRequest{ID: p.ID, Request: req})
			s.finishEndgame(false)
			if b.commits != 1 {
				t.Fatal("replay duplicated mutation")
			}
			found := false
			for _, e := range astralTake(s, sink) {
				if m, ok := e.(event.EndgameMessage); ok && m.Message.OK && m.Message.RequestID == req.RequestID {
					found = true
				}
			}
			if !found {
				t.Fatal("missing receipt replay")
			}
		})
	}
}
func TestAstralStandaloneRemovalIsUnavailable(t *testing.T) {
	s, sink, b := astralSceneFixture(t)
	p := s.players[1]
	plan, err := domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, 42, 51, 0, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	p.Player.Bag = plan.Bag
	before := s.snapshotOf(p).Clone()
	s.onEndgameSocket(p, domain.EndgameRequest{Op: "socket.remove", RequestID: "astral-remove-00001", ExpectedRevision: "0", TargetUID: "42", Socket: 0})
	if b.commits != 0 || !reflect.DeepEqual(before.Bag, p.Player.Bag) || before.Char.Money != p.Player.Char.Money {
		t.Fatal("removed feature changed items or charged materials")
	}
	m, ok := firstOf[event.EndgameMessage](astralTake(s, sink))
	if !ok || m.Message.Error != "unsupported_operation" {
		t.Fatal("removed extraction endpoint is still enabled")
	}
}
func TestAstralFormationChangesRealCombatStats(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	p.Player.Char.Base = domain.Base{STR: 200, VIT: 100, INT: 100, SPI: 100, AGI: 100, DEX: 100}
	plan, err := domain.PlanAstralInsert(p.Player.Bag, p.Player.Worn, s.itemDef, 42, 51, 0, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	tail := domain.NewStack(s.items[domain.FirstAstralRune+5], 1)
	tail.UID = 52
	plan.Bag.Set(101, tail)
	plan, err = domain.PlanAstralInsert(plan.Bag, plan.Worn, s.itemDef, 42, 52, 1, p.Player.Char)
	if err != nil {
		t.Fatal(err)
	}
	p.Player.Bag = plan.Bag
	p.Player.Bag.Set(200, domain.Stack{})
	p.Player.Worn.Set(domain.SlotWeapon, plan.Target)
	recipe, ok := domain.AstralRecipeByID(1, catalogfixture.EndgameCatalog())
	if !ok || len(recipe.Runes) != int(plan.Target.SocketCount) {
		t.Fatal("formation fixture missing")
	}
	var runeBonuses []domain.Affix
	for i, runeID := range recipe.Runes {
		if plan.Target.Sockets[i] != runeID {
			t.Fatal("formation fixture no longer matches authored ingredient order")
		}
		runeBonuses = append(runeBonuses, plan.Target.SocketCards[i].StaticAffixes()...)
	}
	// Independently assemble the expected sources, without MatchAstralRecipe,
	// AstralBonuses, functionalWorn or the scene's passive-affix selection. This
	// verifies every new recipe line and leaves rune snapshots separate.
	source := domain.StatSource{Base: p.Player.Char.EffectiveBase(), Level: p.Player.Char.Level,
		Innate: characterInnateStats(p.Player.Char), Passive: runeBonuses}
	_, wantReversed := domain.Compute(source)
	source.Passive = append(append([]domain.Affix{}, runeBonuses...), domain.ResolveVariableAffixes(recipe.Affixes, recipe.VariableAffixes, plan.Target.Endgame.FormationRollBP)...)
	source.Passive = append(source.Passive, domain.ResolveItemLocalEnhancements(s.items[plan.Target.Item].Equip, recipe.LocalEnhancements, plan.Target.Endgame.FormationRollBP)...)
	_, wantComplete := domain.Compute(source)
	s.refreshStats(p)
	complete := p.Stats
	if complete != wantComplete || p.Player.Worn.At(domain.SlotWeapon) != plan.Target {
		t.Fatalf("active recipe lost extras or changed rune snapshots: got %+v want %+v", complete, wantComplete)
	}
	st := plan.Target
	st.Sockets[0], st.Sockets[1] = st.Sockets[1], st.Sockets[0]
	st.SocketUIDs[0], st.SocketUIDs[1] = st.SocketUIDs[1], st.SocketUIDs[0]
	st.SocketCards[0], st.SocketCards[1] = st.SocketCards[1], st.SocketCards[0]
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.refreshStats(p)
	if p.Stats != wantReversed || complete.MinAtk <= p.Stats.MinAtk || p.Player.Worn.At(domain.SlotWeapon) != st {
		t.Fatalf("reversed recipe must retain only rune effects: active %+v; reversed %+v; want %+v", complete, p.Stats, wantReversed)
	}
	p.Player.Worn.Set(domain.SlotWeapon, plan.Target)
	s.refreshStats(p)
	if p.Stats != wantComplete || p.Player.Worn.At(domain.SlotWeapon) != plan.Target {
		t.Fatal("restoring order failed to reactivate all formation extras without rerolling")
	}
}
func TestAstralNaturalDropsKeepFullAffixBudgetWithoutSockets(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 999002, RatePct: 100, RollAffixes: true}})
	s.items[999002] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999002, Name: "空白星剑", InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), LevelReq: 50, Durable: 100}}
	s.equipmentRoll = domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{1: {{Count: 5, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{{}: {{Affix: domain.Affix{Attr: domain.AttrSTR, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrINT, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrVIT, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrAGI, Value: 2}, Weight: 1}, {Affix: domain.Affix{Attr: domain.AttrDEX, Value: 2}, Weight: 1}}}}
	s.entities[mid].Monster.Kind = domain.MonsterBoss
	for i := 0; i < 100; i++ {
		s.tick = domain.Tick(i) // Distinct death events have independent purpose streams.
		s.rollLoot(s.entities[mid], 0)
	}
	seen := map[int64]bool{}
	for _, e := range s.ground {
		st := e.Drop.Stack
		if st.Item != 999002 {
			continue
		}
		if st.UID <= 0 || seen[st.UID] || st.SocketCount != 0 || int(st.RolledAffixCount) != domain.MaxEquipmentAttributes {
			t.Fatal("bad generated stack", st)
		}
		seen[st.UID] = true
		attributes := map[int32]bool{}
		for _, affix := range st.RolledAffixes[:st.RolledAffixCount] {
			if attributes[affix.Affix.Attr] || affix.Affix.Value != 2 {
				t.Fatal("missing distinct full-value random affixes", st)
			}
			attributes[affix.Affix.Attr] = true
		}
	}
	if len(seen) != 100 {
		t.Fatalf("generated %d equipment instances, want 100", len(seen))
	}
}
