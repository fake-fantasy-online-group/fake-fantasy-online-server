package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const astralRuneRegenInterval = domain.Tick(2000 / domain.TickMS)

func astralRuneRegenFixture(t *testing.T) (*Scene, *entity.Entity, *fakeSink, domain.Stack) {
	t.Helper()
	s, sink, _ := astralSceneFixture(t)
	p := s.players[1]
	st := p.Player.Bag.At(200)
	st.SocketCount = 1
	st.Sockets[0], st.SocketUIDs[0] = domain.LastAstralRune, 91828
	st.SocketCards[0] = s.items[domain.LastAstralRune].CardDefaults
	p.Player.Bag.Set(200, st)
	s.onEquip(Equip{ID: p.ID, BagSlot: 200})
	st = p.Player.Worn.At(domain.SlotWeapon)
	if st.UID != 42 {
		t.Fatal("could not equip rune fixture")
	}
	p.HP, p.MaxHP, p.MP, p.MaxMP = 500, 1000, 250, 500
	astralTake(s, sink)
	return s, p, sink, st
}

func TestAstralRuneRegenStoredFlatAmountAndBoundedClock(t *testing.T) {
	s, p, sink, st := astralRuneRegenFixture(t)
	// The snapshot, not a later changed catalogue/default, owns the amount.
	st.SocketCards[0].Effects[0].Value = 3
	p.Player.Worn.Set(domain.SlotWeapon, st)
	def := s.items[domain.LastAstralRune]
	def.CardDefaults.Effects[0].Value = 999
	s.items[def.ID] = def
	beforeBag, beforeWorn := p.Player.Bag.Clone(), p.Player.Worn.Clone()
	beforeMoney := p.Player.Char.Money
	s.stepAstralRuneRegeneration()
	if p.HP != 500 {
		t.Fatal("equipping granted an immediate heal")
	}
	s.tick += astralRuneRegenInterval - 1
	s.stepAstralRuneRegeneration()
	if p.HP != 500 {
		t.Fatal("early pulse")
	}
	s.tick++
	s.stepAstralRuneRegeneration()
	if p.HP != 503 || p.MP != 250 {
		t.Fatalf("flat stored heal got HP=%d MP=%d", p.HP, p.MP)
	}
	if ev, ok := firstOf[event.HealDone](astralTake(s, sink)); !ok || ev.Amount != 3 || ev.DstHP != 503 {
		t.Fatal("missing actual healing event", ev)
	}
	s.stepAstralRuneRegeneration()
	if p.HP != 503 {
		t.Fatal("same-frame duplicate pulse")
	}
	s.tick += astralRuneRegenInterval * 1000
	s.stepAstralRuneRegeneration()
	if p.HP != 506 {
		t.Fatal("elapsed frames caused catch-up healing", p.HP)
	}
	if !reflect.DeepEqual(beforeBag, p.Player.Bag) || !reflect.DeepEqual(beforeWorn, p.Player.Worn) || beforeMoney != p.Player.Char.Money {
		t.Fatal("regeneration consumed or rewrote rune/equipment/resources")
	}
}

func TestAstralRuneRegenEligibilityAndSnapshotValidation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Scene, *entity.Entity, *domain.Stack)
	}{
		{"underlevel", func(_ *Scene, p *entity.Entity, _ *domain.Stack) { p.Player.Char.Level = 59 }},
		{"wrong_class", func(s *Scene, _ *entity.Entity, st *domain.Stack) {
			s.items[st.Item].Equip.Need.Professions = 1 << uint8(domain.Healer)
		}},
		{"wrong_sex", func(s *Scene, p *entity.Entity, st *domain.Stack) {
			p.Player.Char.Appear.Gender = 1
			s.items[st.Item].Equip.Need.Sex = 2
		}},
		{"stat_requirement", func(s *Scene, _ *entity.Entity, st *domain.Stack) {
			s.items[st.Item].Equip.Need.Base.STR = 1000000
		}},
		{"broken", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.Durability = 0 }},
		{"wrong_slot", func(s *Scene, _ *entity.Entity, st *domain.Stack) {
			s.items[st.Item].Equip.Slot = int32(domain.SlotHead)
		}},
		{"missing_equipment_uid", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.UID = 0 }},
		{"missing_card_uid", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketUIDs[0] = 0 }},
		{"uid_collision", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketUIDs[0] = st.UID }},
		{"duplicate_card_uid", func(_ *Scene, _ *entity.Entity, st *domain.Stack) {
			st.SocketCount = 2
			st.Sockets[1], st.SocketUIDs[1], st.SocketCards[1] = st.Sockets[0], st.SocketUIDs[0], st.SocketCards[0]
		}},
		{"uninitialized_card", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Initialized = false }},
		{"empty_card", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Count = 0 }},
		{"oversize_card", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Count = 9 }},
		{"oversize_sockets", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCount = 6 }},
		{"attribute_capacity", func(s *Scene, _ *entity.Entity, st *domain.Stack) {
			s.items[st.Item].Equip.Affixes = make([]domain.Affix, domain.MaxEquipmentAttributes)
		}},
		{"wrong_instance_kind", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.InstanceKind = domain.ItemInstanceSocketCard }},
		{"wrong_count", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.Count = 2 }},
		{"wrong_rune", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.Sockets[0] = domain.FirstAstralRune }},
		{"percent_mode", func(_ *Scene, _ *entity.Entity, st *domain.Stack) {
			st.SocketCards[0].Effects[0].Mode = domain.ModePercent
		}},
		{"conditional_effect", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Effects[0].Probability = 50 }},
		{"wrong_operator", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Effects[0].Op = 2 }},
		{"negative_amount", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Effects[0].Value = -3 }},
		{"zero_amount", func(_ *Scene, _ *entity.Entity, st *domain.Stack) { st.SocketCards[0].Effects[0].Value = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, p, _, st := astralRuneRegenFixture(t)
			tc.edit(s, p, &st)
			// This fixture edits immutable item templates as well as live instances.
			s.invalidateCombatAttributes(p)
			p.Player.Worn.Set(domain.SlotWeapon, st)
			if got := s.astralRuneRegenAmount(p); got != 0 {
				t.Fatal("invalid source produced recovery", got)
			}
			s.stepAstralRuneRegeneration()
			s.tick += astralRuneRegenInterval
			s.stepAstralRuneRegeneration()
			if p.HP != 500 {
				t.Fatal("invalid source healed", p.HP)
			}
		})
	}
}

func TestAstralRuneRegenUnequipDeadAndRecoveryBounds(t *testing.T) {
	s, p, sink, st := astralRuneRegenFixture(t)
	s.stepAstralRuneRegeneration()
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if p.HP != 500 || len(s.astralRuneRegen) != 0 {
		t.Fatal("unequipped rune remained active")
	}
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.stepAstralRuneRegeneration()
	if p.HP != 500 {
		t.Fatal("re-equipping skipped the wait")
	}
	p.HP = 0
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if p.HP != 0 || len(s.astralRuneRegen) != 0 {
		t.Fatal("rune resurrected player")
	}
	p.HP = 999
	s.stepAstralRuneRegeneration()
	st.SocketCount = 2
	st.Sockets[1], st.SocketUIDs[1], st.SocketCards[1] = domain.LastAstralRune, 91829, st.SocketCards[0]
	st.SocketCards[0].Effects[0].Value = math.MaxInt32
	st.SocketCards[1].Effects[0].Value = math.MaxInt32
	p.Player.Worn.Set(domain.SlotWeapon, st)
	if got := s.astralRuneRegenAmount(p); got != math.MaxInt32 {
		t.Fatal("amount overflowed instead of saturating", got)
	}
	s.stepAstralRuneRegeneration() // Changed rune sources start a fresh interval.
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if p.HP != p.MaxHP {
		t.Fatal("recovery did not cap to missing HP", p.HP)
	}
	if ev, ok := firstOf[event.HealDone](astralTake(s, sink)); !ok || ev.Amount != 1 {
		t.Fatal("overheal amount was reported", ev)
	}
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if n := countOf[event.HealDone](astralTake(s, sink)); n != 0 {
		t.Fatal("full HP emitted zero-value healing", n)
	}
	p.HP--
	s.stepAstralRuneRegeneration()
	if p.HP != 999 {
		t.Fatal("full-health pulse failed to maintain cadence")
	}
}

func TestAstralRuneRegenRelogPreservesSnapshotsWithoutCatchup(t *testing.T) {
	s, p, sink, st := astralRuneRegenFixture(t)
	st.SocketCards[0].Effects[0].Value = 4
	st.InstanceKind = domain.ItemInstanceNone // Persisted legacy equipment remains usable.
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.stepAstralRuneRegeneration()
	s.tick += astralRuneRegenInterval - 1
	snap := s.snapshotOf(p)
	s.onLeave(Leave{ID: p.ID})
	s.tick += astralRuneRegenInterval * 100
	s.exec(Enter{ID: p.ID, Char: snap.Char, Bag: snap.Bag, Worn: snap.Worn, Sink: sink})
	loaded := s.players[p.ID]
	loaded.HP, loaded.MaxHP = 500, 1000
	if loaded.Player.Worn.At(domain.SlotWeapon) != st {
		t.Fatal("relogin changed stored rune/equipment")
	}
	s.stepAstralRuneRegeneration()
	if loaded.HP != 500 {
		t.Fatal("relogin inherited old clock or offline healing")
	}
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if loaded.HP != 504 || loaded.Player.Worn.At(domain.SlotWeapon) != st {
		t.Fatal("relogin did not preserve stored flat amount and identity", loaded.HP)
	}
	loaded.Player.Char.Level = 59
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if loaded.HP != 504 {
		t.Fatal("loaded underlevel rune remained active")
	}
}

func TestAstralRuneRegenSceneStepAndOrdinaryAttr38Isolation(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	// A large ordinary stat is deliberately not a second source of healing.
	s.items[st.Item].Equip.Affixes = []domain.Affix{{Attr: domain.AttrHPRegen, Value: 10000, Mode: domain.ModeAbsolute}}
	p.Stats.HPRegen = 20000
	s.step()
	for n := domain.Tick(0); n < astralRuneRegenInterval; n++ {
		s.step()
	}
	if p.HP != 501 {
		t.Fatal("scene step did not apply exactly the rune's flat amount", p.HP)
	}
	st.Sockets[0] = 998888 // An ordinary card with the same attribute is not the rune.
	p.Player.Worn.Set(domain.SlotWeapon, st)
	for n := domain.Tick(0); n <= astralRuneRegenInterval; n++ {
		s.step()
	}
	if p.HP != 501 {
		t.Fatal("ordinary equipment/card attr38 was globally enabled", p.HP)
	}
}

func TestAstralRuneRegenTooltipUsesSnapshotAndCadence(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	st.SocketCards[0].Effects[0].Value = 7
	const want = "有效穿戴时每2秒恢复7点生命"
	if tip := s.ownedItemTooltip(s.items[st.Item], st, p); !strings.Contains(tip, want) {
		t.Fatal("equipped tooltip lost stored amount or timing", tip)
	}
	card := domain.Stack{UID: st.SocketUIDs[0], Item: domain.LastAstralRune, Count: 1, InstanceKind: domain.ItemInstanceSocketCard, Card: st.SocketCards[0]}
	if tip := s.ownedItemTooltip(s.items[card.Item], card, p); !strings.Contains(tip, want) {
		t.Fatal("rune tooltip used template instead of instance", tip)
	}
	a := domain.Affix{Attr: domain.AttrHPRegen, Value: 7}
	if s.astralRuneAffixText(999, a) != affixText(a) {
		t.Fatal("ordinary card tooltip changed")
	}
}

func TestAstralRuneRegenRemainsActiveWhenFormationSuppressed(t *testing.T) {
	s, p, _, _ := astralRuneRegenFixture(t)
	var recipe domain.AstralRecipe
	for _, r := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
		for _, id := range r.Runes {
			if id == domain.LastAstralRune && len(r.Slots) >= 2 {
				recipe = r
				break
			}
		}
		if recipe.ID != 0 {
			break
		}
	}
	if recipe.ID == 0 {
		t.Fatal("fixture needs a rune28 formation with two valid slots")
	}
	if recipe.Professions != 0 {
		for race := domain.Warrior; race <= domain.Warlock; race++ {
			if recipe.Professions&(1<<uint8(race)) != 0 {
				p.Player.Char.Race = race
				break
			}
		}
	}
	p.Player.Worn = domain.NewEquipSet()
	for n, slot := range recipe.Slots[:2] {
		def := s.items[999001]
		equip := *def.Equip
		def.Equip, equip.Slot = &equip, int32(slot)
		def.ID += domain.ItemID(n + 1)
		s.items[def.ID] = def
		st := domain.NewStack(def, 1)
		st.UID = int64(200000 + n)
		st.SocketCount = uint8(len(recipe.Runes))
		for i, id := range recipe.Runes {
			st.Sockets[i], st.SocketUIDs[i] = id, int64(300000+n*10+i)
			st.SocketCards[i] = s.items[id].CardDefaults
		}
		p.Player.Worn.Set(slot, st)
	}
	if got := len(domain.ActiveAstralRecipes(s.functionalWorn(p), s.itemDef, p.Player.Char)); got != 1 {
		t.Fatal("fixture failed to suppress one formation", got)
	}
	if got := s.astralRuneRegenAmount(p); got != 2 {
		t.Fatal("suppressed formation removed standalone rune effect", got)
	}
}

func assertAstralRuneWaitsFullInterval(t *testing.T, s *Scene, p *entity.Entity) {
	t.Helper()
	before := p.HP
	s.stepAstralRuneRegeneration()
	if p.HP != before {
		t.Fatal("changed source inherited an immediately due pulse", before, p.HP)
	}
	s.tick += astralRuneRegenInterval - 1
	s.stepAstralRuneRegeneration()
	if p.HP != before {
		t.Fatal("changed source healed before a full stable interval", before, p.HP)
	}
	s.tick++
	s.stepAstralRuneRegeneration()
	if p.HP != before+1 {
		t.Fatal("stable eligible rune did not heal at its new deadline", before, p.HP)
	}
}

func TestAstralRuneRegenActualEquipLifecycleRestartsClock(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		name := "same_uid_unequip_reequip"
		if replacement {
			name = "replacement_uid_same_amount"
		}
		t.Run(name, func(t *testing.T) {
			s, p, _, st := astralRuneRegenFixture(t)
			s.stepAstralRuneRegeneration()
			s.tick += astralRuneRegenInterval - 1
			bagSlot := 200
			if replacement {
				st.UID, st.SocketUIDs[0] = 43, 91829
				p.Player.Bag.Set(bagSlot, st)
			} else {
				// Both real commands execute between world frames. A source
				// fingerprint sampled only during step cannot see this gap.
				s.onUnequip(Unequip{ID: p.ID, Slot: domain.SlotWeapon})
				bagSlot = -1
				p.Player.Bag.Each(func(slot int, got domain.Stack) {
					if got.UID == st.UID {
						bagSlot = slot
					}
				})
				if bagSlot < 0 {
					t.Fatal("actual unequip failed")
				}
			}
			s.onEquip(Equip{ID: p.ID, BagSlot: bagSlot})
			if p.Player.Worn.At(domain.SlotWeapon).UID != st.UID {
				t.Fatal("actual equip failed")
			}
			p.HP = p.MaxHP - 10
			s.tick++ // The original clock would heal after only 100 ms worn.
			assertAstralRuneWaitsFullInterval(t, s, p)
		})
	}
}

func TestAstralRuneRegenEligibilityTransitionsRestartClock(t *testing.T) {
	for _, betweenFrames := range []bool{false, true} {
		name := "observed_ineligible_frame"
		if betweenFrames {
			name = "loss_and_recovery_between_frames"
		}
		t.Run(name, func(t *testing.T) {
			s, p, _, _ := astralRuneRegenFixture(t)
			s.stepAstralRuneRegeneration()
			s.tick += astralRuneRegenInterval - 2
			p.Player.Char.Level = 59
			if betweenFrames {
				s.refreshStats(p)
			} else {
				s.stepAstralRuneRegeneration()
			}
			if _, ok := s.astralRuneRegen[p.ID]; ok {
				t.Fatal("ineligible source retained its old deadline")
			}
			p.Player.Char.Level = 60
			if betweenFrames {
				s.refreshStats(p)
			}
			p.HP = p.MaxHP - 10
			s.tick++
			assertAstralRuneWaitsFullInterval(t, s, p)
		})
	}
}

func TestAstralRuneRegenActualQuickSwapRestartsClock(t *testing.T) {
	s, p, _, original := astralRuneRegenFixture(t)
	spare := original
	spare.UID, spare.SocketUIDs[0] = 43, 91829
	p.Player.ChangeSet = domain.NewChangeSet()
	p.Player.ChangeSet.Set(domain.SlotWeapon, spare)
	s.stepAstralRuneRegeneration()
	s.tick += astralRuneRegenInterval - 1
	s.onChangeSetSwap(ChangeSetSwap{ID: p.ID})
	if p.Player.Worn.At(domain.SlotWeapon).UID != spare.UID {
		t.Fatal("first actual quick swap failed")
	}
	s.onChangeSetSwap(ChangeSetSwap{ID: p.ID})
	if p.Player.Worn.At(domain.SlotWeapon).UID != original.UID {
		t.Fatal("second actual quick swap failed")
	}
	p.HP = p.MaxHP - 10
	s.tick++
	assertAstralRuneWaitsFullInterval(t, s, p)
}

func TestAstralRuneRegenObservedSourceIdentityChangeRestartsClock(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	s.stepAstralRuneRegeneration()
	s.tick += astralRuneRegenInterval - 1
	st.SocketUIDs[0]++ // Equal healing amount, different durable source.
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.tick++
	assertAstralRuneWaitsFullInterval(t, s, p)
}

func TestAstralRuneRegenStableRefreshDoesNotPostponeClock(t *testing.T) {
	s, p, _, _ := astralRuneRegenFixture(t)
	s.stepAstralRuneRegeneration()
	s.tick += astralRuneRegenInterval - 1
	s.refreshStats(p)
	p.HP = p.MaxHP - 10
	before := p.HP
	s.tick++
	s.stepAstralRuneRegeneration()
	if p.HP != before+1 {
		t.Fatal("unrelated stat refresh postponed a stable eligible source")
	}
}

func TestAstralRuneRegenActualDeathAndReviveRestartClock(t *testing.T) {
	for _, skillRevive := range []bool{false, true} {
		name := "ordinary_revive"
		if skillRevive {
			name = "skill_revive"
		}
		t.Run(name, func(t *testing.T) {
			s, p, _, _ := astralRuneRegenFixture(t)
			s.stepAstralRuneRegeneration()
			s.tick += astralRuneRegenInterval - 1
			p.HP = 0
			s.onDeath(p, 0)
			if _, ok := s.astralRuneRegen[p.ID]; ok {
				t.Fatal("actual death retained rune clock")
			}
			if skillRevive {
				if !s.reviveBySkill(p, p, domain.SkillDef{}, domain.SkillEffect{HealPctOfMax: 50}) {
					t.Fatal("skill revive failed")
				}
			} else {
				s.onRevive(Revive{ID: p.ID, ReviveType: ReviveTypeRespawn})
			}
			if !p.Alive() {
				t.Fatal("actual revive failed")
			}
			p.HP = p.MaxHP - 10
			s.tick++
			assertAstralRuneWaitsFullInterval(t, s, p)
		})
	}
}

func TestAstralRuneSpeedTooltipUsesPositivePlayerSpeedAndStoredValue(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	rune := domain.FirstAstralRune + 19
	st.Sockets[0], st.SocketCards[0] = rune, s.items[rune].CardDefaults
	st.SocketCards[0].Effects[0].Value = -7 // Snapshot overrides catalogue -2.
	before := st.SocketCards[0]
	const want = "攻击速度 +7%"
	if tip := s.ownedItemTooltip(s.items[st.Item], st, p); !strings.Contains(tip, want) || strings.Contains(tip, "攻击速度 -7%") {
		t.Fatal("socket tooltip reversed the stored interval/player-speed sign", tip)
	}
	card := domain.Stack{UID: st.SocketUIDs[0], Item: rune, Count: 1, InstanceKind: domain.ItemInstanceSocketCard, Card: before}
	if tip := s.ownedItemTooltip(s.items[rune], card, p); !strings.Contains(tip, want) {
		t.Fatal("rune tooltip lost the snapshot speed sign/value", tip)
	}
	if st.SocketCards[0] != before || card.Card != before {
		t.Fatal("tooltip formatting rewrote the durable snapshot")
	}
}

func configureRecoveryTestCatalog(t *testing.T, s *Scene, edit func(*domain.EndgameCatalogData)) {
	t.Helper()
	input := catalogfixture.EndgameCatalogData()
	edit(&input)
	catalog, err := domain.NewEndgameCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	s.endgameCatalog = catalog
	domain.AttachEndgameCatalog(s.items, catalog)
}

func TestPeriodicRecoveryUsesConfiguredSourceAndCadence(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	replacement := domain.LastAstralRune - 1
	configureRecoveryTestCatalog(t, s, func(input *domain.EndgameCatalogData) {
		var oldIndex, replacementIndex int
		for i := range input.Runes {
			if input.Runes[i].Item == domain.LastAstralRune {
				oldIndex = i
			}
			if input.Runes[i].Item == replacement {
				replacementIndex = i
			}
		}
		input.Runes[oldIndex].Affixes, input.Runes[replacementIndex].Affixes = input.Runes[replacementIndex].Affixes, input.Runes[oldIndex].Affixes
		input.Runes[oldIndex].RegenerationIntervalMS = 0
		input.Runes[replacementIndex].RegenerationIntervalMS = 1500
	})
	// Identical durable numbers on the old ID no longer grant the behavior.
	if got := s.astralRuneRegenAmount(p); got != 0 {
		t.Fatal("runtime still dispatches original rune ID", got)
	}
	st.Sockets[0] = replacement
	st.SocketCards[0].Effects[0].Value = 3
	p.Player.Worn.Set(domain.SlotWeapon, st)
	if got := s.astralRuneRegenAmount(p); got != 3 {
		t.Fatal("configured new source ignored", got)
	}
	if got := s.astralRuneAffixText(replacement, domain.Affix{Attr: domain.AttrHPRegen, Value: 3}); got != "有效穿戴时每1.5秒恢复3点生命" {
		t.Fatal(got)
	}
	s.stepAstralRuneRegeneration()
	s.tick += domain.Tick(1400 / domain.TickMS)
	s.stepAstralRuneRegeneration()
	if p.HP != 500 {
		t.Fatal("configured cadence fired early", p.HP)
	}
	s.tick++
	s.stepAstralRuneRegeneration()
	if p.HP != 503 {
		t.Fatal("configured cadence not used", p.HP)
	}
}

func TestPeriodicRecoveryMixedCadencesDoNotCrossMultiply(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	other := domain.LastAstralRune - 1
	configureRecoveryTestCatalog(t, s, func(input *domain.EndgameCatalogData) {
		for i := range input.Runes {
			if input.Runes[i].Item == other {
				input.Runes[i].RegenerationIntervalMS = 1000
			}
		}
	})
	st.SocketCount = 2
	st.SocketCards[0].Effects[0].Value = 3
	st.Sockets[1], st.SocketUIDs[1], st.SocketCards[1] = other, 91829, st.SocketCards[0]
	st.SocketCards[1].Effects[0].Value = 7
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.stepAstralRuneRegeneration()
	s.tick += domain.Tick(1000 / domain.TickMS)
	s.stepAstralRuneRegeneration()
	if p.HP != 507 {
		t.Fatal("1 second bucket wrong", p.HP)
	}
	s.tick += domain.Tick(1000 / domain.TickMS)
	s.stepAstralRuneRegeneration()
	if p.HP != 517 {
		t.Fatal("simultaneous buckets counted more than once", p.HP)
	}
	s.tick += domain.Tick(100000 / domain.TickMS)
	s.stepAstralRuneRegeneration()
	if p.HP != 527 {
		t.Fatal("mixed cadence caught up missed pulses", p.HP)
	}
}

func TestNativeTooltipSeparatesOrdinaryAndPeriodicRecoveryBuckets(t *testing.T) {
	s, p, _, st := astralRuneRegenFixture(t)
	// Make an actual unique item so the native combined-attribute renderer runs.
	// The extra ordinary card deliberately uses the same numeric attribute.
	def := s.items[st.Item]
	def.ID = 1001
	def.EndgameCatalog = s.endgameCatalog
	s.items[def.ID] = def
	st.Item = def.ID
	st.Endgame = domain.EndgameItemState{UniqueID: 1001, UniqueContentVersion: domain.UniqueContentVersion, UniqueRollBP: 10000}
	st.SocketCount = 2
	st.SocketCards[0].Effects[0].Value = 3
	st.Sockets[1], st.SocketUIDs[1], st.SocketCards[1] = 998888, 91829, st.SocketCards[0]
	st.SocketCards[1].Effects[0].Value = 7
	p.Player.Worn.Set(domain.SlotWeapon, st)
	p.Player.EndgameNativeStats = true
	tip := s.ownedItemTooltip(def, st, p)
	if !strings.Contains(tip, "有效穿戴时每2秒恢复3点生命") || !strings.Contains(tip, affixText(domain.Affix{Attr: domain.AttrHPRegen, Value: 7})) || strings.Contains(tip, "每2秒恢复10点生命") {
		t.Fatal("different recovery scopes merged", tip)
	}
	s.stepAstralRuneRegeneration()
	s.tick += astralRuneRegenInterval
	s.stepAstralRuneRegeneration()
	if p.HP != 503 {
		t.Fatal("ordinary attr38 entered online recovery", p.HP)
	}
}
