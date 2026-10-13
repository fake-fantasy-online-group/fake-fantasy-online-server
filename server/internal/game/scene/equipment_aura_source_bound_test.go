package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strings"
	"testing"
)

// Cache injection isolates the aura lifecycle from catalog balance. The source
// still uses the real worn-item revision/eligibility gate, never a fake StatusDef.
func cacheTestAura(s *Scene, p *entity.Entity, uid int64, value int32) {
	c := s.equipmentEffects(p)
	c.stats = domain.EquipmentCombatStats{}
	c.effects = []domain.ActiveEquipmentEffect{{SourceUID: uid, Effect: domain.EquipmentEffect{Kind: "aura", Aura: &domain.EquipmentAura{
		ID: "white_tiger_guard", Family: "white_tiger_guard", FixedRank: 2,
		Modifier: "physical_mitigation_bp", Value: value, RadiusPX: 240,
		RefreshMS: 500, LeaseMS: 1000, MaxPlayers: 6, IncludeActivePet: true,
		VisualKey: "白虎卫身", PVPScaleBP: 5000,
	}}}}
	s.equipmentState().cache[p.ID] = c
}

func TestEquipmentAuraSourceBoundBuffAndSkillCoexist(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "来源", 100, 100)
	join(t, s, 2, 200, "队友", 120, 100)
	p, q := s.players[1], s.players[2]
	p.Player.Party, q.Player.Party = 1, 1
	cacheTestAura(s, p, 991, 1200)
	skill := domain.StatusDef{ID: 1178, Level: 1, Name: "白虎卫身", Desc: "技能原有效果", SourceSkillID: 10001, DurationSec: 30}
	q.Status.Apply(skill, s.tick, q.ID)
	s.stepEquipmentAuras()
	buffs := s.buffSnapshot(q).Buffs
	if len(buffs) != 2 {
		t.Fatalf("skill and aura should coexist: %+v", buffs)
	}
	var auraFound, skillFound bool
	for _, b := range buffs {
		if b.SkillID == equipmentAuraStatusID("white_tiger_guard") {
			auraFound = true
			if b.RemainSec != -2 {
				t.Fatalf("source-bound aura must not count down: %+v", b)
			}
			for _, part := range []string{"2级", "12%", "6%", "来源：来源", "范围240", "最多6人", "只取最高", "离线"} {
				if !strings.Contains(b.Desc, part) {
					t.Errorf("missing %q in %q", part, b.Desc)
				}
			}
		}
		if b.SkillID == 10001 {
			skillFound = b.Desc == "技能原有效果" && b.RemainSec > 0
		}
	}
	if !auraFound || !skillFound {
		t.Fatalf("source identity collision: %+v", buffs)
	}
	for _, st := range s.equipmentAuraStatuses(q) {
		if st.icon.RemainMS != -1 || st.effect.DurationMS <= 0 || st.effect.DurationMS > 1000 {
			t.Fatalf("presentation must hide internal visual lease: %+v", st)
		}
	}
	if got := s.equipmentDefense(q, false).PhysicalMitigationBP; got != 1200 {
		t.Fatalf("aura applied twice: %d", got)
	}
	delete(s.equipmentState().cache, p.ID)
	s.stepEquipmentAuras()
	if got := s.buffSnapshot(q).Buffs; len(got) != 1 || got[0].SkillID != 10001 {
		t.Fatalf("removal erased skill or retained aura: %+v", got)
	}
}

func TestEquipmentAuraRangeInvalidatesBeforeNextScan(t *testing.T) {
	for _, transition := range []string{"range", "map", "party", "dead", "disconnect"} {
		t.Run(transition, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "来源", 100, 100)
			sink := join(t, s, 2, 200, "队友", 120, 100)
			p, q := s.players[1], s.players[2]
			p.Player.Party, q.Player.Party = 1, 1
			cacheTestAura(s, p, 991, 1200)
			s.stepEquipmentAuras()
			s.flush()
			sink.take()
			switch transition {
			case "range":
				s.aoi.Move(q, domain.Pos{MapID: 7, X: 900, Y: 100})
			case "map":
				q.Pos.MapID++
			case "party":
				q.Player.Party = 0
			case "dead":
				p.HP = 0
			case "disconnect":
				delete(s.players, p.ID)
				delete(s.entities, p.ID)
			}
			if got := s.equipmentDefense(q, false).PhysicalMitigationBP; got != 0 {
				t.Fatalf("stale numerical aura before scan: %d", got)
			}
			if got := s.equipmentAuraStatuses(q); len(got) != 0 {
				t.Fatalf("stale displayed aura before scan: %+v", got)
			}
			s.stepEquipmentAuras() // same tick, no 500 ms delay
			if len(s.equipmentState().auras[q.ID]) != 0 {
				t.Fatal("source-bound aura survived transition")
			}
			s.flush()
			removed := false
			for _, ev := range sink.take() {
				if snapshot, ok := ev.(event.BuffSnapshot); ok && snapshot.Who == q.ID && len(snapshot.Buffs) == 0 {
					removed = true
				}
			}
			if !removed {
				t.Fatal("client did not receive source-bound buff removal snapshot")
			}

		})
	}
}

func TestEquipmentAuraStrongestWinsAndFallsBack(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "弱来源", 100, 100)
	join(t, s, 2, 200, "强来源", 120, 100)
	join(t, s, 3, 300, "队友", 125, 100)
	p, q, r := s.players[1], s.players[2], s.players[3]
	p.Player.Party, q.Player.Party, r.Player.Party = 1, 1, 1
	cacheTestAura(s, p, 991, 800)
	cacheTestAura(s, q, 992, 1600)
	s.stepEquipmentAuras()
	if got := s.equipmentDefense(r, false).PhysicalMitigationBP; got != 1600 {
		t.Fatal("same aura stacked", got)
	}
	if got := s.buffSnapshot(r).Buffs; len(got) != 1 || !strings.Contains(got[0].Desc, "强来源") {
		t.Fatalf("wrong winning source: %+v", got)
	}
	q.HP = 0
	s.stepEquipmentAuras()
	if got := s.equipmentDefense(r, false).PhysicalMitigationBP; got != 800 {
		t.Fatal("weaker source did not resume", got)
	}
}

func wearFormationAuraTest(t *testing.T, s *Scene, p *entity.Entity, recipeID int32, uid int64) domain.Stack {
	t.Helper()
	r, ok := domain.AstralRecipeByID(recipeID, catalogfixture.EndgameCatalog())
	if !ok {
		t.Fatal("missing formation", recipeID)
	}
	item := domain.ItemID(990000 + recipeID)
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: item, Name: "普通宿阵基底", InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(r.Slots[0]), Durable: 100}}
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	s.items[item] = def
	st := domain.NewStack(def, 1)
	st.UID = uid
	st.Durability, st.MaxDurability = 100, 100
	st.Endgame.FormationContentVersion = domain.UniqueContentVersion
	st.Endgame.FormationRollBP = 10000
	st.SocketCount = uint8(len(r.Runes))
	runeDefs := map[domain.ItemID]domain.ItemDef{}
	for _, rd := range astralRuneItemsFixture(7) {
		runeDefs[rd.ID] = rd
	}
	for i, runeID := range r.Runes {
		card := domain.NewStack(runeDefs[runeID], 1)
		st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = runeID, card.UID, card.Card
	}
	p.Player.Char.Level = 100
	p.Player.Char.Base = domain.Base{STR: 1000, VIT: 1000, INT: 1000, SPI: 1000, AGI: 1000, DEX: 1000}
	p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
	for race := domain.Warrior; race <= domain.Warlock; race++ {
		if r.Professions == 0 || r.Professions&(1<<uint8(race)) != 0 {
			p.Player.Char.Race = race
			break
		}
	}
	p.Player.Worn.Set(r.Slots[0], st)
	if _, ok := domain.MatchAstralRecipe(def, st); !ok {
		t.Fatal("invalid formation fixture", recipeID)
	}
	return st
}

func formationAuraTest(t *testing.T, recipeID int32) domain.EquipmentAura {
	t.Helper()
	r, ok := domain.AstralRecipeByID(recipeID, catalogfixture.EndgameCatalog())
	if !ok {
		t.Fatal("missing recipe", recipeID)
	}
	for _, e := range r.TypedEffects {
		if e.Kind == "aura" && e.Aura != nil {
			return *e.Aura
		}
	}
	t.Fatal("formation lacks aura", recipeID)
	return domain.EquipmentAura{}
}

func TestFormationAuraFiveKindsSourceBound(t *testing.T) {
	ids := map[int32]bool{}
	for _, recipeID := range []int32{16, 18, 24, 27, 35} {
		a := formationAuraTest(t, recipeID)
		id := equipmentAuraStatusID(a.ID)
		if ids[id] || id < 1901000 {
			t.Fatal("aura status identity collision", id)
		}
		ids[id] = true
		t.Run(a.ID, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "来源", 100, 100)
			join(t, s, 2, 200, "队友", 120, 100)
			p, q := s.players[1], s.players[2]
			p.Player.Party, q.Player.Party = 1, 1
			st := wearFormationAuraTest(t, s, p, recipeID, 9001)
			s.stepEquipmentAuras()
			for _, target := range []*entity.Entity{p, q} {
				got := s.buffSnapshot(target).Buffs
				if len(got) != 1 || got[0].SkillID != id || got[0].RemainSec != -2 || got[0].Name != a.VisualKey {
					t.Fatalf("invalid formation buff: %+v", got)
				}
				base := s.equipmentEffects(target).stats
				pve, pvp := s.equipmentDefense(target, false), s.equipmentDefense(target, true)
				if a.Modifier == "physical_mitigation_bp" {
					if pve.PhysicalMitigationBP != base.PhysicalMitigationBP+a.Value || pvp.PhysicalMitigationBP != base.PhysicalMitigationBP+a.Value/2 {
						t.Fatal("physical aura not applied exactly once")
					}
				} else {
					element := domain.ElementIndex(a.Element)
					if pve.Elements[element].ResistBP != base.Elements[element].ResistBP+a.Value || pvp.Elements[element].ResistBP != base.Elements[element].ResistBP+a.Value/2 {
						t.Fatal("element aura not applied exactly once")
					}
				}
			}
			p.Player.Worn.Set(domain.EquipSlot(s.items[st.Item].Equip.Slot), domain.Stack{})
			s.stepEquipmentAuras()
			if len(s.buffSnapshot(p).Buffs) != 0 || len(s.buffSnapshot(q).Buffs) != 0 {
				t.Fatal("unequipped formation retained source-bound buff")
			}
		})
	}
}

func TestFormationOffensiveAuraRealStrikesAndWithdrawal(t *testing.T) {
	for _, recipeID := range []int32{14, 34} {
		aura := formationAuraTest(t, recipeID)
		element := domain.ElementIndex(aura.Element)
		hurt := int32(domain.HurtFire)
		if aura.Element == "ice" {
			hurt = domain.HurtIce
		}
		t.Run(aura.ID, func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "来源", 100, 100)
			join(t, s, 2, 200, "队友", 120, 100)
			join(t, s, 3, 300, "敌方玩家", 130, 100)
			p, q, enemy := s.players[1], s.players[2], s.players[3]
			p.Player.Party, q.Player.Party = 1, 1
			st := wearFormationAuraTest(t, s, p, recipeID, 9001)
			pet := &entity.Entity{ID: 800, Kind: domain.KindPet, HP: 1000, MaxHP: 1000, Pos: domain.Pos{MapID: 7, X: 125, Y: 100}, Pet: &entity.Pet{Owner: q.ID}}
			q.Player.Pet = pet.ID
			s.entities[pet.ID] = pet
			s.pets[pet.ID] = pet
			s.aoi.Enter(pet)
			monster := putMonster(s, 900, 100000, domain.Stats{}, 150, 100)
			enemy.Stats = domain.Stats{}
			s.stepEquipmentAuras()
			for _, src := range []*entity.Entity{p, q, pet} {
				src.Stats = domain.Stats{MAtk: 1000}
				base := s.equipmentOffense(src)
				if got := base.Elements[element].DamageBP + int32(s.equipmentAuraDamageNumerator(src, element, false)/10000); got != base.Elements[element].DamageBP+aura.Value {
					t.Fatalf("aura not granted once to %d: %d", src.ID, got)
				}
				for _, dst := range []*entity.Entity{monster, enemy} {
					dst.HP, dst.MaxHP = 100000, 100000
					_, raw, _ := s.resolveStrikeStatuses(src, dst, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: hurt, EquipmentSnapshot: true, EquipmentOffense: base})
					want := domain.ElementOutgoingDamage(1000, 0, base.Elements[element].DamageBP+aura.Value, s.equipmentPVP(src, dst))
					if raw != want {
						t.Fatalf("source %d vs %d raw=%d want=%d", src.ID, dst.ID, raw, want)
					}
				}
			}
			// Accepted payloads store only personal gear, never a stale aura.
			accepted := combat.Blow{Magic: true, HurtType: hurt, EquipmentSnapshot: true, EquipmentOffense: s.equipmentOffense(q)}
			q.Stats = domain.Stats{MAtk: 1000}
			var capped domain.EquipmentCombatStats
			capped.Elements[element].DamageBP = 9900
			monster.HP = 100000
			_, raw, _ := s.resolveStrikeStatuses(q, monster, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: hurt, EquipmentSnapshot: true, EquipmentOffense: capped})
			if raw != 2000 {
				t.Fatal("offensive aura bypassed shared +100% damage cap", raw)
			}
			p.Player.Worn.Set(domain.EquipSlot(s.items[st.Item].Equip.Slot), domain.Stack{})
			// No periodic scan or lease expiry is needed for numerical removal.
			monster.HP = 100000
			_, raw, _ = s.resolveStrikeStatuses(q, monster, &combat.HitTracker{}, accepted)
			if raw != 1000 {
				t.Fatal("accepted cast retained unequipped source aura", raw)
			}
			s.stepEquipmentAuras()
			if len(s.buffSnapshot(q).Buffs) != 0 || len(s.equipmentAuraStatuses(pet)) != 0 {
				t.Fatal("offensive source-bound presentation survived unequip")
			}
		})
	}
}

func TestOffensiveAuraSameElementStrongestAndDefenseCoexist(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "弱来源", 100, 100)
	join(t, s, 2, 200, "强来源", 110, 100)
	join(t, s, 3, 300, "队友", 120, 100)
	join(t, s, 4, 400, "防护来源", 125, 100)
	p, q, r, d := s.players[1], s.players[2], s.players[3], s.players[4]
	p.Player.Party, q.Player.Party, r.Player.Party, d.Player.Party = 1, 1, 1, 1
	wearFormationAuraTest(t, s, p, 14, 9001)
	wearFormationAuraTest(t, s, q, 14, 9002)
	wearFormationAuraTest(t, s, d, 24, 9003)
	c := s.equipmentEffects(q)
	for i := range c.effects {
		if a := c.effects[i].Effect.Aura; a != nil {
			copy := *a
			copy.ID = "alternate_fire"
			copy.Family = "different_fire_family"
			copy.Value = 2000
			c.effects[i].Effect.Aura = &copy
		}
	}
	s.equipmentState().cache[q.ID] = c
	s.stepEquipmentAuras()
	if got := int32(s.equipmentAuraDamageNumerator(r, 0, false) / 10000); got != 2000 {
		t.Fatal("same-element damage auras stacked across families", got)
	}
	if got := s.equipmentDefense(r, false).Elements[0].ResistBP; got != formationAuraTest(t, 24).Value {
		t.Fatal("offense suppressed same-element defensive aura", got)
	}
	if len(s.buffSnapshot(r).Buffs) != 2 {
		t.Fatal("offense/defense should have independent buff rows")
	}
	q.HP = 0
	s.stepEquipmentAuras()
	if got := int32(s.equipmentAuraDamageNumerator(r, 0, false) / 10000); got != formationAuraTest(t, 14).Value {
		t.Fatal("weaker source did not resume", got)
	}
}
