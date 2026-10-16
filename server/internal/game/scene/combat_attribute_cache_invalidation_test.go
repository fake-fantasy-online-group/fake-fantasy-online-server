package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"testing"
)

// Change authoritative inputs directly so caller refreshes cannot hide a missing dependency.
func assertCombatAttributeCacheFresh(t *testing.T, s *Scene, p *entity.Entity) {
	t.Helper()
	gotWorn := s.functionalWorn(p)
	gotBase, gotStats, gotGrowth := s.combatGrowthStats(p)
	gotPanel := s.attributeSnapshot(p)
	s.invalidateCombatAttributes(p)
	delete(s.skillRawCache, p.ID)
	wantWorn := s.functionalWornResolved(p, newEquipmentRequirementResolution())
	wantBase, wantStats, wantGrowth := s.computeSkillGrowthStats(p, wantWorn)
	wantPanel := s.attributeSnapshot(p)
	if !reflect.DeepEqual(requirementResolutionStacks(gotWorn), requirementResolutionStacks(wantWorn)) || gotBase != wantBase || gotStats != wantStats || gotGrowth != wantGrowth || !reflect.DeepEqual(gotPanel, wantPanel) {
		t.Fatalf("warmed resolution differs from clean recomputation: base=%+v/%+v stats=%+v/%+v growth=%+v/%+v", gotBase, wantBase, gotStats, wantStats, gotGrowth, wantGrowth)
	}
}

func TestCombatAttributeCacheIndependentInputMutations(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	p.Status = domain.NewStatusSet()
	requirementResolutionItem(s, p, domain.SlotHead, 30, 20)
	requirementResolutionItem(s, p, domain.SlotRing, 0, 20)
	assertCombatAttributeCacheFresh(t, s, p)
	changes := []struct {
		name   string
		change func()
	}{
		{"same_slot_replacement", func() {
			st := p.Player.Worn.At(domain.SlotRing)
			st.WashCount = 1
			st.WashAffixes[0] = domain.Affix{Attr: domain.AttrSTR, Value: -30}
			p.Player.Worn.Set(domain.SlotRing, st)
		}},
		{"status_enables_requirement", func() {
			p.Status.Apply(domain.StatusDef{ID: 990001, Level: 1, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 40}}}, s.tick, p.ID)
		}},
		{"status_removed", func() { p.Status.Remove(990001) }},
		{"base_allocation", func() { p.Player.Char.Base.STR += 40 }},
		{"hard_skill_in_place", func() { p.Player.Char.Skills[10401] = 10 }},
		{"hard_skill_replaced", func() { p.Player.Char.Skills = domain.Learned{10415: 20} }},
		{"level", func() { p.Player.Char.Level++ }},
		{"gender", func() { p.Player.Char.Appear.Gender = 1 }},
		{"profession", func() { p.Player.Char.Race = domain.Warrior }},
		{"employment", func() { p.Player.Char.Employed = false }},
		{"broken_item", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.Durability = 0
			st.MaxDurability = 100
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"repaired_item", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.Durability = 100
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"worn_replaced", func() { p.Player.Worn = p.Player.Worn.Clone() }},
		{"base_move_speed", func() { p.Player.Char.Attrs[9] = 123 }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) { tc.change(); assertCombatAttributeCacheFresh(t, s, p) })
	}
}

func TestCombatAttributeCacheIndependentPetDependencies(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	s.entities = map[domain.EntityID]*entity.Entity{p.ID: p}
	inst := sourcePetFixtureV5(s, p, 10000)
	s.petSkills[13001] = domain.PetSkillDef{ID: 13001, MaxLevel: 50, Effect: &domain.PetSkillEffect{Kind: "max_weight_from_vit", SourceStep: 1, Value: 2, ValuePerLevel: 1, RequireSelected: true}}
	inst.Base.VIT = 10
	inst.ActiveSkill = 13001
	assertCombatAttributeCacheFresh(t, s, p)
	changes := []struct {
		name   string
		change func()
	}{
		{"pet_base", func() { inst.Base.VIT = 20 }},
		{"pet_rank", func() { inst.Skills[0].Level = 2 }},
		{"pet_selection", func() { inst.ActiveSkill = 0 }},
		{"pet_selection_restore", func() { inst.ActiveSkill = 13001 }},
		{"pet_death", func() { s.entities[p.Player.Pet].HP = 0 }},
		{"pet_revive", func() { s.entities[p.Player.Pet].HP = 100 }},
		{"pet_removed", func() { delete(s.entities, p.Player.Pet) }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) { tc.change(); assertCombatAttributeCacheFresh(t, s, p) })
	}
}

func TestCombatAttributeCacheIndependentSuppressionAndLimits(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	p.Status = domain.NewStatusSet()
	bad := domain.StatusDef{ID: 990001, Level: 1, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: -10}}}
	p.Status.Apply(bad, s.tick, p.ID)
	assertCombatAttributeCacheFresh(t, s, p)
	p.Status.Apply(domain.StatusDef{ID: 990002, Level: 1, SuppressHarmful: true}, s.tick, p.ID)
	assertCombatAttributeCacheFresh(t, s, p)
	p.Status.Remove(990002)
	assertCombatAttributeCacheFresh(t, s, p)
	p.Status.Apply(domain.StatusDef{ID: 990003, Level: 1, AttackCapKnown: true, AttackCap: 0, MaxHPFloor: 100000}, s.tick, p.ID)
	assertCombatAttributeCacheFresh(t, s, p)
	p.Status.Remove(990003)
	assertCombatAttributeCacheFresh(t, s, p)
}

func TestCombatAttributeCacheIndependentXPAndResourceSnapshots(t *testing.T) {
	s, _ := rewardScene(t, 给经验的树妖)
	sink := join(t, s, 1, 100, "cache", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 3
	p.Level = 3
	s.refreshStats(p)
	s.flush()
	sink.take()
	state := s.combatAttributeState(p)
	p.HP = 17
	p.MP = 3
	s.grantExpExact(p, 1)
	if s.combatAttributeState(p) != state {
		t.Fatal("XP without level gain invalidated combat attributes")
	}
	s.flush()
	events := sink.take()
	var snapshots, experience int
	for _, ev := range events {
		switch v := ev.(type) {
		case event.StatsChanged:
			snapshots++
			if v.Current[1] != 1 || v.Current[3] != 17 || v.Current[5] != 3 {
				t.Fatalf("live XP/resources missing: %+v", v.Current)
			}
		case event.ExpGained:
			experience++
			if snapshots != 1 {
				t.Fatal("experience preceded authoritative attributes")
			}
		}
	}
	if snapshots != 1 || experience != 1 {
		t.Fatalf("XP event contract changed: snapshots=%d experience=%d", snapshots, experience)
	}
	s.grantExpExact(p, 269)
	if p.Player.Char.Level != 4 || s.combatAttributeState(p) == state {
		t.Fatal("level gain did not rebuild combat attributes")
	}
	assertCombatAttributeCacheFresh(t, s, p)
}

func TestCombatAttributeCacheIndependentPositiveWearAndOwnerReplacement(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	requirementResolutionItem(s, p, domain.SlotHead, 0, 20)
	s.players = map[domain.EntityID]*entity.Entity{p.ID: p}
	s.combatGrowthStats(p)
	state := s.combatAttributeState(p)
	st := p.Player.Worn.At(domain.SlotHead)
	st.Durability = 90
	st.DurabilityWearRaw = 123
	p.Player.Worn.Set(domain.SlotHead, st)
	if s.combatAttributeState(p) != state {
		t.Fatal("positive wear rebuilt unchanged attributes")
	}
	if got := s.functionalWorn(p).At(domain.SlotHead); got.Durability != 90 || got.DurabilityWearRaw != 123 {
		t.Fatal("cached eligibility returned stale durability", got)
	}
	preview := *p
	if s.combatAttributeState(&preview) != nil {
		t.Fatal("preview populated owner cache")
	}
	if s.combatAttributeState(p) != state {
		t.Fatal("preview evicted owner cache")
	}
	replacement := *p
	player := *p.Player
	ch := *p.Player.Char
	replacement.Player = &player
	player.Char = &ch
	s.players[p.ID] = &replacement
	if s.combatAttributeState(&replacement) == state {
		t.Fatal("same-ID replacement reused prior owner's state")
	}
	assertCombatAttributeCacheFresh(t, s, &replacement)
}

func TestCombatAttributeCacheIndependentEquivalentStatusInputsReuse(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	p.Status = domain.NewStatusSet()
	for i := int32(0); i < 8; i++ {
		p.Status.Apply(domain.StatusDef{ID: domain.StatusID(990100 + i), Level: 1, DurationSec: 30, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: i + 1}}}, s.tick, p.ID)
	}
	s.combatGrowthStats(p)
	state := s.combatAttributeState(p)
	for i := 0; i < 100; i++ {
		if s.combatAttributeState(p) != state {
			t.Fatal("status map iteration order invalidated unchanged attributes")
		}
	}
	p.Status.Apply(domain.StatusDef{ID: 990200, Level: 1, DurationSec: 30, ExperienceBonusPct: 100}, s.tick, p.ID)
	if s.combatAttributeState(p) != state {
		t.Fatal("XP-only status invalidated combat attributes")
	}
	p.Status.Remove(990200)
	p.Status.Rebase(s.tick, s.tick+100)
	if s.combatAttributeState(p) != state {
		t.Fatal("status lifetime change invalidated unchanged attributes")
	}
	p.HP = 0
	if s.combatAttributeState(p) != state {
		t.Fatal("health change invalidated static attributes")
	}
	p.HP = 100
	if s.combatAttributeState(p) != state {
		t.Fatal("revival without stat changes invalidated static attributes")
	}
}

// One resolution may visit the same fixed-slot subsets in many orders. Compare
// every result with the non-memoized path, including real soft ranks and errors.
func TestCombatAttributeCacheIndependentRawSubsetEquivalence(t *testing.T) {
	for _, mode := range []string{"learned_soft_rank", "treasure_grant", "nil_treasure_grant", "legacy", "invalid_hard_rank"} {
		t.Run(mode, func(t *testing.T) {
			s, p := requirementResolutionFixture(t)
			for _, slot := range []domain.EquipSlot{domain.SlotHead, domain.SlotBody, domain.SlotRing, domain.SlotTreasure} {
				requirementResolutionItem(s, p, slot, 0, 1)
			}
			d := domain.ItemDef{EndgameCatalog: s.endgameCatalog, ID: 2018, Equip: &domain.EquipDef{Slot: int32(domain.SlotHead), Durable: 100}}
			s.items[d.ID] = d
			st := domain.NewStack(d, 1)
			st.UID = 2018
			st.Endgame = domain.EndgameItemState{UniqueID: 2018, UniqueRollBP: 8000, UniqueContentVersion: domain.UniqueContentVersion}
			p.Player.Worn.Set(domain.SlotHead, st)
			switch mode {
			case "treasure_grant", "nil_treasure_grant":
				p.Player.Char.Skills = domain.Learned{}
				if mode == "nil_treasure_grant" {
					p.Player.Char.Skills = nil
				}
				id := p.Player.Worn.At(domain.SlotTreasure).Item
				d := s.items[id]
				d.Equip.Skill = &domain.EquipmentSkill{ID: 10401}
				s.items[id] = d
			case "legacy":
				p.Player.Char.Skills = domain.Learned{999999: 3}
			case "invalid_hard_rank":
				p.Player.Char.Skills = domain.Learned{10401: -1}
			}
			original := p.Player.Worn
			ctx := newEquipmentRequirementResolution()
			slots := []domain.EquipSlot{domain.SlotHead, domain.SlotBody, domain.SlotRing, domain.SlotTreasure}
			for _, mask := range []int{15, 0, 5, 10, 1, 14, 2, 13, 3, 12, 4, 11, 6, 9, 7, 8, 15, 0} {
				preview := *p
				player := *p.Player
				preview.Player = &player
				player.Worn = domain.NewEquipSet()
				for i, slot := range slots {
					if mask&(1<<i) != 0 {
						player.Worn.Set(slot, original.At(slot))
					}
				}
				s.skillRawCache = nil
				want := s.rawSkillLevelsResolved(&preview, nil)
				s.skillRawCache = nil
				got := s.rawSkillLevelsResolved(&preview, ctx)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("subset %04b got%v want%v", mask, got, want)
				}
				// Neither caller mutation nor eviction of the unrelated legacy memo may
				// change the resolution's retained ranks.
				if got != nil {
					got[10401] = 999
					delete(got, 999999)
				}
				s.skillRawCache = nil
				again := s.rawSkillLevelsResolved(&preview, ctx)
				if !reflect.DeepEqual(again, want) {
					t.Fatalf("subset %04b memo aliases caller output: got%v want%v", mask, again, want)
				}
			}
			if ctx.rawHits < 18 || len(ctx.raw) > 16 {
				t.Fatalf("raw memo hit/subset bound: hits=%d subsets=%d", ctx.rawHits, len(ctx.raw))
			}
		})
	}
}

func TestCombatAttributeCacheIndependentRawKeyDependencies(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	requirementResolutionItem(s, p, domain.SlotHead, 0, 1)
	p.Player.Char.Skills = domain.Learned{10401: 20, 10415: 3, 10301: 1}
	before := skillRawFingerprint(p)
	p.Player.Char.Skills = domain.Learned{10301: 1, 10415: 3, 10401: 20}
	if skillRawFingerprint(p) != before {
		t.Fatal("hard rank insertion order changes key")
	}
	st := p.Player.Worn.At(domain.SlotHead)
	st.Durability = 90
	st.DurabilityWearRaw = 77
	p.Player.Worn.Set(domain.SlotHead, st)
	if skillRawFingerprint(p) != before {
		t.Fatal("positive wear changes raw-rank key")
	}
	changes := []struct {
		name   string
		change func()
	}{
		{"hard_rank", func() { p.Player.Char.Skills[10401]++ }},
		{"hard_rank_deleted", func() { delete(p.Player.Char.Skills, 10301) }},
		{"character_id", func() { p.Player.Char.ID++ }},
		{"level", func() { p.Player.Char.Level++ }},
		{"base", func() { p.Player.Char.Base.STR++ }},
		{"race", func() { p.Player.Char.Race = domain.Warrior }},
		{"employment", func() { p.Player.Char.Employed = !p.Player.Char.Employed }},
		{"employment_known", func() { p.Player.Char.EmploymentKnown = !p.Player.Char.EmploymentKnown }},
		{"gender", func() { p.Player.Char.Appear.Gender++ }},
		{"broken", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.Durability = 0
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"repair", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.Durability = 100
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"rolled_affix", func() {
			st := p.Player.Worn.At(domain.SlotHead)
			st.WashCount = 1
			st.WashAffixes[0] = domain.Affix{Attr: domain.AttrSTR, Value: 5}
			p.Player.Worn.Set(domain.SlotHead, st)
		}},
		{"worn_identity", func() { p.Player.Worn = p.Player.Worn.Clone() }},
		{"character_identity", func() { ch := *p.Player.Char; p.Player.Char = &ch }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			before := skillRawFingerprint(p)
			tc.change()
			if skillRawFingerprint(p) == before {
				t.Fatal("changed dependency reused key")
			}
		})
	}
}

// Admission must complete the derived bundle before any combat consumer runs.
// Do not call a warming accessor before inspecting the admission result.
func TestCombatAttributeCacheIndependentAdmissionAndMutationReady(t *testing.T) {
	rs, rp := requirementResolutionFixture(t)
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(rs, rp, slot, 0, 1)
	}
	for id, d := range rs.items {
		d.InventoryTab, d.InventoryTabKnown = 2, true
		rs.items[id] = d
	}
	rp.Player.Char.ID = 100
	rp.Player.Char.Name = "live attributes"
	rp.Player.Char.Pos = domain.Pos{MapID: 7, X: 100, Y: 100}
	rp.Player.Char.FreePoints = 5
	s := New(Config{EndgameCatalog: rs.endgameCatalog, ID: domain.SceneID{MapID: 7}, Items: rs.items, Skills: rs.skills, Levels: rewardLevels(), SaveEvery: 10000})
	s.exec(Enter{ID: 1, Char: rp.Player.Char, Worn: rp.Player.Worn, Sink: &fakeSink{}})
	p := s.players[1]
	if p == nil {
		t.Fatal("player admission failed")
	}
	checkReady := func(label string) *combatAttributeDependencies {
		t.Helper()
		state := s.combatAttributes[p.ID]
		if state == nil || state.values == nil || !state.values.Ready || state.values.FunctionalWorn == nil {
			t.Fatalf("%s left derived attributes uninitialized", label)
		}
		if state.values.Stats != p.Stats {
			t.Fatalf("%s left live stats out of sync", label)
		}
		return state
	}
	admitted := checkReady("login")
	owned := p.Player.CombatAttributes
	revision := owned.Revision
	if owned == nil || owned != admitted.values {
		t.Fatal("derived values are not player-owned")
	}
	m := putMonster(s, 900, 1000000, domain.Stats{}, 110, 100)
	s.exec(Attack{ID: p.ID, Target: m.ID})
	s.step()
	if checkReady("first attack") != admitted || p.Player.CombatAttributes != owned || owned.Revision != revision {
		t.Fatal("first combat performed hidden cold rebuild")
	}
	s.exec(AllocPoints{ID: p.ID, STR: 1})
	allocated := checkReady("attribute allocation")
	if allocated == admitted || p.Player.CombatAttributes != owned || owned.Revision <= revision {
		t.Fatal("attribute allocation did not update derived state")
	}
	s.exec(Unequip{ID: p.ID, Slot: domain.SlotHead})
	unequipped := checkReady("unequip")
	if unequipped == allocated || !unequipped.values.FunctionalWorn.At(domain.SlotHead).Empty() {
		t.Fatal("unequip did not update derived state")
	}
	buff := domain.StatusDef{ID: 991900, Level: 1, DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 5}}}
	if !s.applyStatusDef(p, buff, p.ID) {
		t.Fatal("status rejected")
	}
	buffed := checkReady("status application")
	if buffed == unequipped {
		t.Fatal("status application did not update derived state")
	}
	p.Status.Remove(buff.ID)
	s.statusesRemoved(p, []domain.StatusID{buff.ID})
	if checkReady("status removal") == buffed {
		t.Fatal("status removal did not update derived state")
	}
	s.exec(Leave{ID: p.ID})
	if s.combatAttributes[p.ID] != nil {
		t.Fatal("departure retained derived entity")
	}
}

func TestCombatAttributeCacheIndependentTransferAdmissionReady(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})
	ch := &domain.Character{ID: 100, Name: "attribute transfer", Level: 10, Base: domain.Base{STR: 20, AGI: 20, VIT: 20, INT: 20, SPI: 20, DEX: 20}, Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}
	worn := domain.NewEquipSet()
	worn.Set(domain.SlotWeapon, domain.NewStack(场景钢剑, 1))
	src.Post(Enter{ID: 1, Char: ch, Worn: worn, Sink: &fakeSink{}})
	var old *combatAttributeDependencies
	inspect(t, src, rig, func(s *Scene) {
		old = s.combatAttributes[1]
		if old == nil || old.values == nil || !old.values.Ready {
			t.Error("source admission not ready")
		}
	})
	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14}, At: domain.Pos{MapID: 14, X: 100, Y: 100}})
	inspect(t, src, rig, func(s *Scene) {
		if s.combatAttributes[1] != nil {
			t.Error("transfer retained source derived state")
		}
	})
	inspect(t, dst, rig, func(s *Scene) {
		p := s.players[1]
		state := s.combatAttributes[1]
		if p == nil || state == nil || state.values == nil || !state.values.Ready {
			t.Error("destination admission not ready before gameplay")
			return
		}
		if state == old || state.values == old.values || state.values != p.Player.CombatAttributes || state.owner != p || state.values.Stats != p.Stats {
			t.Error("destination reused source derived owner or stale stats")
		}
		s.rawSkillLevels(p)
		s.attributeSnapshot(p)
		if s.combatAttributes[1] != state {
			t.Error("first destination consumer caused hidden rebuild")
		}
	})
}

func TestCombatAttributeCacheIndependentRejectedAdmissionCleanup(t *testing.T) {
	rs, rp := requirementResolutionFixture(t)
	// Missing inventory-tab metadata is deliberately rejected after attributes
	// were constructed, but before the player is admitted to the scene.
	requirementResolutionItem(rs, rp, domain.SlotHead, 0, 1)
	rp.Player.Char.ID = 100
	rp.Player.Char.Pos = domain.Pos{MapID: 7, X: 100, Y: 100}
	s := New(Config{EndgameCatalog: rs.endgameCatalog, ID: domain.SceneID{MapID: 7}, Items: rs.items, Skills: rs.skills})
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: rp.Player.Char, Worn: rp.Player.Worn, Sink: sink})
	if s.players[1] != nil || !sink.isClosed() {
		t.Fatal("fixture did not reject admission")
	}
	if s.combatAttributes[1] != nil {
		t.Fatal("rejected admission retained derived dependencies")
	}
	if _, ok := s.skillRawCache[1]; ok {
		t.Fatal("rejected admission retained raw ranks")
	}
}
