package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func cacheVariant(t *testing.T, s *Scene, p *entity.Entity, id string) {
	t.Helper()
	c := s.equipmentEffects(p)
	c.effects = append(c.effects, domain.ActiveEquipmentEffect{SourceUID: 42, Item: 123, Effect: domain.EquipmentEffect{Kind: "skill_variant", VariantID: id}})
	s.equipmentState().cache[p.ID] = c
	p.Player.EndgameSkillVariants = true
}
func TestEquipmentSkillVariantsAllRanksSnapshotAndOriginal(t *testing.T) {
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance", "assassin_bomb_fuse", "healer_guarded_mend", "warlock_flash_fire"} {
		v, _ := catalogfixture.EndgameCatalog().Variant(id)
		for _, rank := range []int32{1, 5, 20, 21, 60, 100} {
			t.Run(fmt.Sprintf("%s/rank%d", id, rank), func(t *testing.T) {
				s, p := behaviorAcceptancePlayer(t, v.Profession)
				cacheVariant(t, s, p, id)
				p.Player.Char.Skills[v.SkillID] = min(rank, 20)
				def, ok := s.skills.Get(v.SkillID, rank)
				if !ok {
					t.Fatal("rank", rank)
				}
				base := def
				s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, domain.Learned{v.SkillID: rank})
				if def.EquipmentVariant == nil || def.EquipmentVariant.ID != id {
					t.Fatal("missing variant", def)
				}
				view := s.skillBehaviorPayload(p, def)
				if view.Variant == nil || view.MPCost != def.MPCost || view.PrepareMS != def.PrepareMS {
					t.Fatal("view drift")
				}
				unchanged, _ := s.skills.Get(v.SkillID, rank)
				if !reflect.DeepEqual(base, unchanged) {
					t.Fatal("mutated source row")
				}
				p.Player.EndgameSkillVariants = false
				old := base
				s.applyResolvedSkillModifiers(p, &old, p.Player.Char.Skills, domain.Learned{v.SkillID: rank})
				wire, _ := json.Marshal(s.skillBehaviorPayload(p, old))
				if strings.Contains(string(wire), "variant") {
					t.Fatal("old protocol leaked", string(wire))
				}
				p.Player.EndgameSkillVariants = true
				delete(s.equipmentState().cache, p.ID)
				unequipped := base
				s.applyResolvedSkillModifiers(p, &unequipped, p.Player.Char.Skills, domain.Learned{v.SkillID: rank})
				if unequipped.EquipmentVariant != nil {
					t.Fatal("unequip retained variant")
				}
				if def.EquipmentVariant.ID != id {
					t.Fatal("cast snapshot changed")
				}
			})
		}
	}
}
func TestEquipmentSkillVariantsGeometryAndBudget(t *testing.T) {
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance"} {
		v, _ := catalogfixture.EndgameCatalog().Variant(id)
		s, p := behaviorAcceptancePlayer(t, v.Profession)
		cacheVariant(t, s, p, id)
		p.Player.Char.Skills[v.SkillID] = 1
		for n := 0; n < 8; n++ {
			putMonster(s, domain.EntityID(900+n), 10000, domain.Stats{}, float64(120+n*8), 100)
		}
		putMonster(s, 999, 10000, domain.Stats{}, 80, 100)
		d, _ := s.skills.Get(v.SkillID, 1)
		s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, p.Player.Char.Skills)
		ids, _ := s.resolveSkillTargets(p, d, 905, nil)
		ids = s.limitVariantTargets(p, d, ids, 905)
		if len(ids) != 8 || ids[0] != 905 {
			t.Fatal("bounded primary", id, ids)
		}
		for _, n := range ids {
			if n == 999 {
				t.Fatal("hit behind caster")
			}
		}
		if id == "sword_gale_fan" {
			edge := variantTargetDef(d, 1)
			if len(edge.Statuses) != 0 {
				t.Fatal("edge control")
			}
			if reflect.DeepEqual(edge.DirectEffects(), d.DirectEffects()) {
				t.Fatal("edge not scaled")
			}
		}
	}
}
func TestEquipmentSkillVariantsGuardedMendFinite(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	cacheVariant(t, s, p, "healer_guarded_mend")
	p.Player.Char.Skills[10301] = 1
	d, _ := s.skills.Get(10301, 1)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, p.Player.Char.Skills)
	p.HP, p.MaxHP = 500, 1000
	if got := s.startVariantMend(p, p, d, 1000); got != 350 {
		t.Fatal("initial budget", got)
	}
	p.HP = 850
	s.stepVariantMends(s.now().Add(2 * time.Second))
	if p.HP != 1000 || len(s.equipmentState().variantMends) != 0 {
		t.Fatal("delayed cap", p.HP)
	}
	if got := s.startVariantMend(p, p, d, 1000); got != 0 || len(s.equipmentState().variantMends) != 0 {
		t.Fatal("full health stored")
	}
	p.HP = 500
	s.startVariantMend(p, p, d, 1000)
	delete(s.equipmentState().cache, p.ID)
	s.stepVariantMends(s.now().Add(2 * time.Second))
	if p.HP != 500 {
		t.Fatal("unequipped paid stored heal")
	}
	cacheVariant(t, s, p, "healer_guarded_mend")
	s.startVariantMend(p, p, d, 1000)
	p.HP = 0
	s.stepVariantMends(s.now().Add(2 * time.Second))
	if p.HP != 0 {
		t.Fatal("resurrected")
	}
}
func TestEquipmentSkillVariantsBombFiniteOwnerPayload(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 3)
	s.alloc = domain.NewEntityAlloc()
	cacheVariant(t, s, p, "assassin_bomb_fuse")
	p.Player.Char.Skills[10223] = 1
	dst := putMonster(s, 900, 10000, domain.Stats{}, 150, 100)
	s.monsters[dst.ID] = dst
	d, _ := s.skills.Get(10223, 1)
	s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, p.Player.Char.Skills)
	// Synthetic bombs use a fully executable base payload, no learned-rank mutation.
	bomb := domain.SkillDef{ID: 10213, Kind: domain.SkillSingle, TargetEnemy: true, HurtType: domain.HurtFire, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
	s.placeTrap(p, bomb, domain.Pos{MapID: 14, X: 280, Y: 100}, 100, 100)
	s.placeTrap(p, bomb, domain.Pos{MapID: 14, X: 300, Y: 100}, 100, 100)
	if len(s.traps) != 2 {
		t.Fatal("trap fixture")
	}
	s.stepTraps()
	if len(s.traps) != 2 {
		t.Fatal("bomb should not auto-trigger outside original sensing radius")
	}
	s.detonateVariantBomb(p, dst, d)
	if len(s.traps) != 2 {
		t.Fatal("empty hit detonated")
	}
	finish := s.beginEquipmentRoot(p.ID, dst.ID, s.equipmentOffense(p))
	s.equipment.root.hits = append(s.equipment.root.hits, equipmentHit{dst: dst.ID, ev: event.DamageDealt{Amount: 1}})
	s.detonateVariantBomb(p, dst, d)
	finish()
	if len(s.traps) != 1 {
		t.Fatal("not exactly one bomb", len(s.traps))
	}
	if dst.HP >= 10000 {
		t.Fatal("early explosion failed to reach outside original sensing radius")
	}
}

func TestEquipmentSkillVariantsNegotiationDependencies(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 5)
	s.endgamePlayer(p).loaded = true
	for _, caps := range [][]string{{domain.EquipmentSkillVariantsV1}, {domain.EndgameV1, domain.EndgameGrowthV5, domain.EquipmentSkillVariantsV1}, {domain.EndgameV1, domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1, domain.EquipmentSkillVariantsV1}, {domain.EndgameV1, domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}} {
		s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: caps})
		if p.Player.EndgameSkillVariants != (len(caps) == 4) {
			t.Fatal("cap dependency/revocation", caps)
		}
	}
}
func TestEquipmentSkillVariantsFlashFirePaidSixtySeconds(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 5)
		p.Player.Char.Skills[10401] = 5
		p.Stats = domain.Stats{MAtk: 1000, Hit: 1 << 20}
		p.MP, p.MaxMP = 3000, 3000
		if enabled {
			cacheVariant(t, s, p, "warlock_flash_fire")
		}
		dst := putMonster(s, 900, 100000000, domain.Stats{}, 150, 100)
		now := time.Unix(1700000000, 0)
		s.now = func() time.Time { return now }
		paid, casts := int32(0), 0
		for i := 0; i < 600; i++ {
			before := p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: dst.ID})
			if p.MP < before {
				casts++
				paid += before - p.MP
				if enabled && s.casts[len(s.casts)-1].def.PrepareMS != 0 {
					t.Fatal("not instant")
				}
			}
			s.stepCastsAt(now, false)
			now = now.Add(100 * time.Millisecond)
			s.tick++
		}
		if casts == 0 || paid != 3000-p.MP || p.MP < 0 {
			t.Fatal("no finite paid casts", enabled, casts, paid, p.MP)
		}
		if enabled && casts > 50 {
			t.Fatal("1200ms debt bypass", casts)
		}
		t.Logf("variant=%v casts=%d paid=%d remainingMP=%d damage=%d", enabled, casts, paid, p.MP, dst.MaxHP-dst.HP)
	}
}

func TestEquipmentSkillVariantsRealCatalogWearBreakRemove(t *testing.T) {
	for _, itemID := range []int32{1009, 1610, 1908, 2056, 1306} {
		var item domain.UniqueDefinition
		for _, d := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
			if d.ID == itemID {
				item = d
				break
			}
		}
		var v domain.EquipmentSkillVariant
		for _, e := range item.TypedEffects {
			if e.Kind == "skill_variant" {
				v, _ = catalogfixture.EndgameCatalog().Variant(e.VariantID)
			}
		}
		if v.ID == "" {
			t.Fatal("missing catalogue binding", itemID)
		}
		s, p := behaviorAcceptancePlayer(t, v.Profession)
		p.Player.EndgameSkillVariants = true
		p.Player.Char.Skills[v.SkillID] = 5
		st := wearEquipmentV5(t, s, p, item.BaseID, 4242)
		d, _ := s.skills.Get(v.SkillID, 5)
		s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, p.Player.Char.Skills)
		if d.EquipmentVariant == nil || d.EquipmentVariant.SourceUID != st.UID {
			t.Fatal("real item not active", itemID)
		}
		slot := domain.EquipSlot(item.Slot)
		st.Durability = 0
		p.Player.Worn.Set(slot, st)
		broken, _ := s.skills.Get(v.SkillID, 5)
		s.applyResolvedSkillModifiers(p, &broken, p.Player.Char.Skills, p.Player.Char.Skills)
		if broken.EquipmentVariant != nil {
			t.Fatal("broken variant active", itemID)
		}
		st.Durability = 100
		p.Player.Worn.Set(slot, st)
		repaired, _ := s.skills.Get(v.SkillID, 5)
		s.applyResolvedSkillModifiers(p, &repaired, p.Player.Char.Skills, p.Player.Char.Skills)
		if repaired.EquipmentVariant == nil {
			t.Fatal("repair not restored", itemID)
		}
	}
}

func TestEquipmentSkillVariantsWireFixtures(t *testing.T) {
	out := os.Getenv("SKILL_VARIANTS_FIXTURE_DIR")
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance", "assassin_bomb_fuse", "healer_guarded_mend", "warlock_flash_fire"} {
		v, _ := catalogfixture.EndgameCatalog().Variant(id)
		s, p := behaviorAcceptancePlayer(t, v.Profession)
		p.Player.Char.Skills[v.SkillID] = 5
		cacheVariant(t, s, p, id)
		rows := behaviorAcceptanceRows(t, s, p)
		var selected domain.SkillRankView
		for _, r := range rows {
			if r.ID == v.SkillID {
				selected = r
				break
			}
		}
		b, err := json.Marshal(domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: []domain.SkillRankView{selected}, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "unspentPoints": "94", "formulaVersion": "5"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"variant":{"id":`) {
			t.Fatal("missing variant wire", string(b))
		}
		if out != "" {
			if err = os.MkdirAll(out, 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(out, id+".json"), b, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestEquipmentSkillVariantsAcceptedCastAndUnequipFlight(t *testing.T) {
	for _, id := range []string{"sword_gale_fan", "warrior_piercing_lance"} {
		v, _ := catalogfixture.EndgameCatalog().Variant(id)
		s, p := behaviorAcceptancePlayer(t, v.Profession)
		cacheVariant(t, s, p, id)
		p.Player.Char.Skills[v.SkillID] = 5
		p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 1 << 20}
		primary := putMonster(s, 900, 100000, domain.Stats{}, 150, 100)
		side := putMonster(s, 901, 100000, domain.Stats{}, 155, 105)
		back := putMonster(s, 902, 100000, domain.Stats{}, 50, 100)
		now := time.Unix(1700000000, 0)
		s.now = func() time.Time { return now }
		s.onUseSkill(UseSkill{ID: p.ID, Skill: v.SkillID, Target: primary.ID})
		if len(s.casts) != 1 || len(s.casts[0].targets) != 2 {
			t.Fatalf("accepted shape %s: %+v", id, s.casts)
		}
		delete(s.equipmentState().cache, p.ID)
		now = now.Add(3 * time.Second)
		s.stepCastsAt(now, false)
		now = now.Add(3 * time.Second)
		s.stepCastsAt(now, false)
		if primary.HP >= 100000 || side.HP >= 100000 || back.HP != 100000 {
			t.Fatal("finite in-flight geometry changed after unequip", id, primary.HP, side.HP, back.HP)
		}
	}
}
func TestEquipmentSkillVariantsFirstPointPreview(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 2)
	cacheVariant(t, s, p, "sword_gale_fan")
	rows, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	r := rows[10102]
	if r.Contract.Current != nil || r.Contract.Next == nil || r.Contract.Next.Variant == nil || p.Player.Char.Skills[10102] != 0 {
		t.Fatal("first investment preview mismatch", r)
	}
}

func TestEquipmentVariantBehaviorUsesConfiguredIdentityAndParameters(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 5)
	input := catalogfixture.EndgameCatalogData()
	const renamed = "same_behavior_different_content_identity"
	for i := range input.Variants {
		if input.Variants[i].ID == "warlock_flash_fire" {
			input.Variants[i].ID = renamed
			input.Variants[i].Params.MinCooldownMS = 2300
		}
	}
	for i := range input.Uniques {
		for j := range input.Uniques[i].Definition.TypedEffects {
			effect := &input.Uniques[i].Definition.TypedEffects[j]
			if effect.VariantID == "warlock_flash_fire" {
				effect.VariantID = renamed
			}
		}
	}
	catalog, err := domain.NewEndgameCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	s.endgameCatalog = catalog
	domain.AttachEndgameCatalog(s.items, catalog)
	cacheVariant(t, s, p, renamed)
	p.Player.Char.Skills[10401] = 1
	def, _ := s.skills.Get(10401, 1)
	before := def
	s.applyEquipmentSkillVariant(p, &def, p.Player.Char.Skills)
	if def.EquipmentVariant == nil || def.EquipmentVariant.ID != renamed || def.PrepareMS != 0 || def.CooldownMS != max(before.CooldownMS, int32(2300)) || def.MPCost != before.MPCost {
		t.Fatal("behavior depended on preset ID or hardcoded cooldown", def)
	}
	raw, err := json.Marshal(def.EquipmentVariant)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Behavior") || strings.Contains(string(raw), "Params") || strings.Contains(string(raw), "instant_cast") {
		t.Fatal("server behavior config leaked into strict client DTO", string(raw))
	}
}
