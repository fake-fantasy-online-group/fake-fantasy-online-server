package scene

import (
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Independent integration review: preserve the exact source generation even
// when the same UID is removed and restored before the next scene tick.
func TestV6ReviewMendSourceGeneration(t *testing.T) {
	for _, change := range []string{"rewear", "break_repair", "content_version", "source_death", "source_removed"} {
		t.Run(change, func(t *testing.T) {
			s, p := behaviorAcceptancePlayer(t, 4)
			p.Player.EndgameSkillVariants = true
			p.Player.Char.Skills[10301] = 5
			var item domain.UniqueDefinition
			for _, d := range domain.UniqueCatalog() {
				if d.ID == 2056 {
					item = d
					break
				}
			}
			st := wearEquipmentV5(t, s, p, item.BaseID, 45454)
			def, _ := s.skills.Get(10301, 5)
			s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
			if def.EquipmentVariant == nil {
				t.Fatal("real catalog fixture did not resolve")
			}
			p.HP, p.MaxHP = 100, 1000
			s.startVariantMend(p, p, def, 500)
			slot := domain.EquipSlot(item.Slot)
			switch change {
			case "rewear":
				p.Player.Worn.Set(slot, domain.Stack{})
				p.Player.Worn.Set(slot, st)
			case "break_repair":
				broken := st
				broken.Durability = 0
				p.Player.Worn.Set(slot, broken)
				p.Player.Worn.Set(slot, st)
			case "content_version":
				st.Endgame.UniqueContentVersion = 5
				p.Player.Worn.Set(slot, st)
			case "source_death":
				p.HP = 0
			case "source_removed":
				delete(s.entities, p.ID)
			}
			before := p.HP
			s.stepVariantMends(s.now().Add(2 * time.Second))
			if p.HP != before || len(s.equipmentState().variantMends) != 0 {
				t.Fatalf("%s retained source-bound deposit hp=%d before=%d pending=%d", change, p.HP, before, len(s.equipmentState().variantMends))
			}
		})
	}
}

func TestV6ReviewVariantDuplicateWinnerAndIsolation(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 2)
	cacheVariant(t, s, p, "sword_gale_fan")
	p.Player.Char.Skills[10102] = 5
	cached := s.equipmentEffects(p)
	duplicate := cached.effects[len(cached.effects)-1]
	duplicate.SourceUID = 7
	cached.effects = append(cached.effects, duplicate)
	s.equipmentState().cache[p.ID] = cached
	def, _ := s.skills.Get(10102, 5)
	s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
	if def.EquipmentVariant == nil || def.EquipmentVariant.SourceUID != 7 || def.EquipmentVariant.MaxTargets != 4 {
		t.Fatalf("duplicate changed winner/budget: %+v", def.EquipmentVariant)
	}
	before := append([]domain.SkillEffect(nil), def.DirectEffects()...)
	secondary := variantTargetDef(def, 1)
	if reflect.DeepEqual(secondary.DirectEffects(), before) || !reflect.DeepEqual(def.DirectEffects(), before) {
		t.Fatal("secondary scaling missing or mutated primary snapshot")
	}
	for i := 0; i < 3; i++ {
		again := variantTargetDef(def, 1)
		if !reflect.DeepEqual(again.DirectEffects(), secondary.DirectEffects()) {
			t.Fatal("secondary target index compounded damage")
		}
	}
	delete(p.Player.Char.Skills, 10102)
	unlearned, _ := s.skills.Get(10102, 5)
	s.applyResolvedSkillModifiers(p, &unlearned, p.Player.Char.Skills, domain.Learned{10102: 5})
	if unlearned.EquipmentVariant != nil {
		t.Fatal("soft ranks unlocked variant")
	}
}

func TestV6ReviewFixedHealingProcIgnoresMendVariant(t *testing.T) {
	var baseline int32
	for _, variant := range []bool{false, true} {
		s, p := behaviorAcceptancePlayer(t, 4)
		p.Player.Char.Skills[10301] = 20
		if variant {
			cacheVariant(t, s, p, "healer_guarded_mend")
		}
		effect := domain.ActiveEquipmentEffect{SourceUID: 1234, Effect: domain.EquipmentEffect{Kind: "proc", Proc: &domain.EquipmentProc{ID: "review_fixed_heal", SkillID: 10301, FixedRank: 1, TargetRule: "self", Payload: "heal", ScaleBP: 10000}}}
		c := s.equipmentEffects(p)
		c.effects = append(c.effects, effect)
		s.equipmentState().cache[p.ID] = c
		p.HP, p.MaxHP = 1, 10000
		s.executeEquipmentProc(p, equipmentProcCandidate{active: effect})
		gain := p.HP - 1
		if gain <= 0 || len(s.equipmentState().variantMends) != 0 {
			t.Fatalf("fixed proc failed or stored variant heal: gain=%d", gain)
		}
		if !variant {
			baseline = gain
		} else if gain != baseline {
			t.Fatalf("fixed-rank heal borrowed variant/rank: baseline=%d variant=%d", baseline, gain)
		}
	}
}

func TestV6ReviewMendCapabilityRevocationAndReplacement(t *testing.T) {
	for _, change := range []string{"variant_cap", "growth_cap", "plain_heal"} {
		t.Run(change, func(t *testing.T) {
			s, p := behaviorAcceptancePlayer(t, 4)
			join(t, s, 2, 200, "队友", 120, 100)
			q := s.players[2]
			p.Player.Party, q.Player.Party = 1, 1
			p.Player.EndgameGrowthV5, q.Player.EndgameGrowthV5 = true, true
			s.requireGrowthV5 = true
			cacheVariant(t, s, p, "healer_guarded_mend")
			p.Player.Char.Skills[10301] = 1
			def, _ := s.skills.Get(10301, 1)
			base := def
			s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
			q.HP, q.MaxHP = 100, 1000
			s.startVariantMend(p, q, def, 500)
			if len(s.equipmentState().variantMends) != 1 {
				t.Fatal("missing fixture deposit")
			}
			switch change {
			case "variant_cap":
				p.Player.EndgameSkillVariants = false
			case "growth_cap":
				p.Player.EndgameGrowthV5 = false
			case "plain_heal":
				if amount := s.startVariantMend(p, q, base, 100); amount != 100 {
					t.Fatal("ordinary heal modified")
				}
			}
			s.stepVariantMends(s.now().Add(2 * time.Second))
			if q.HP != 100 || len(s.equipmentState().variantMends) != 0 {
				t.Fatalf("%s released stale deposit hp=%d pending=%d", change, q.HP, len(s.equipmentState().variantMends))
			}
		})
	}
}
