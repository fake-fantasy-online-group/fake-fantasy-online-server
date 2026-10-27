package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"sort"
	"testing"
)

func (s *Scene) auditLegacyAuraRecipients(source equipmentAuraSource) []*entity.Entity {
	owner := s.players[source.owner]
	if owner == nil {
		return nil
	}
	a := source.effect
	r2 := float64(a.RadiusPX) * float64(a.RadiusPX)
	candidates := []*entity.Entity{owner}
	s.aoi.AroundRadius(owner.Pos, float64(a.RadiusPX), func(p *entity.Entity) {
		if p.ID != owner.ID && p.Player != nil && p.Pos.MapID == owner.Pos.MapID && p.Alive() && !s.entityMapLoading(p) && s.equipmentCombatReady(p) && sameParty(owner, p) && sqDist(owner.Pos, p.Pos) <= r2 {
			candidates = append(candidates, p)
		}
	})
	sort.Slice(candidates[1:], func(i, j int) bool {
		x, y := candidates[i+1], candidates[j+1]
		dx, dy := sqDist(owner.Pos, x.Pos), sqDist(owner.Pos, y.Pos)
		if dx != dy {
			return dx < dy
		}
		return x.ID < y.ID
	})
	out := append([]*entity.Entity(nil), candidates...)
	for _, p := range candidates {
		if a.IncludeActivePet && p.Player.Pet != 0 {
			pet := s.pets[p.Player.Pet]
			if pet != nil && pet.Pos.MapID == owner.Pos.MapID && pet.Alive() && pet.Pet != nil && pet.Pet.Owner == p.ID && sqDist(owner.Pos, pet.Pos) <= r2 {
				out = append(out, pet)
			}
		}
	}
	return out
}
func (s *Scene) auditLegacyAuraStatuses(e *entity.Entity) []statusPresentation {
	if e == nil || !e.Alive() {
		return nil
	}
	grants := s.equipmentState().auras[e.ID]
	var ordered []equipmentAuraGrant
	for _, g := range grants {
		if g.expires > s.tick && s.equipmentAuraGrantValid(e, g) {
			ordered = append(ordered, g)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].effect.Priority != ordered[j].effect.Priority {
			return ordered[i].effect.Priority > ordered[j].effect.Priority
		}
		return ordered[i].effect.ID < ordered[j].effect.ID
	})
	var out []statusPresentation
	for i, g := range ordered {
		a := g.effect
		id := equipmentAuraStatusID(a.ID)
		iconID := equipmentAuraIconID(a.ID)
		position := uint8(domain.StatusPositionOrbit)
		if i == 0 && g.owner == e.ID {
			position = 0
		}
		label := "物理减伤"
		if a.Modifier == "element_resist_bp" {
			label = map[string]string{"fire": "火抗", "ice": "冰抗", "holy": "圣/光抗", "dark": "暗抗"}[a.Element]
		}
		sourceName := "自身"
		if source := s.players[g.owner]; source != nil && g.owner != e.ID {
			sourceName = source.Name
		}
		pvpCondition := "受到玩家攻击时"
		if a.Modifier == "element_damage_bp" {
			label = equipmentElementName(a.Element) + "伤害"
			pvpCondition = "对玩家造成伤害时"
		}
		desc := fmt.Sprintf("%d级光环：%s +%s；%s +%s。\n来源：%s；范围%d内的同队成员均可生效；同类光环只取最高，不叠加。", a.FixedRank, label, equipmentPercent(a.Value), pvpCondition, equipmentPercent(int32(int64(a.Value)*int64(a.PVPScaleBP)/10000)), sourceName, a.RadiusPX)
		if a.IncludeActivePet {
			desc += "惠及出战宠物。"
		}
		if g.owner == e.ID {
			desc += "\n自身装备有效时持续生效，脱装、损坏、死亡或离线时移除。"
		} else {
			desc += "\n来源装备有效且处于同队范围内时持续生效，来源脱装、损坏、死亡、离线或自身离队、离开范围时移除。"
		}
		remaining := nonnegativeI32((g.expires - s.tick).Millis())
		out = append(out, statusPresentation{name: a.VisualKey, desc: desc, icon: event.StatusIconView{Status: id, Icon: iconID, Beneficial: true, RemainMS: -1}, effect: event.EntityStatusEffect{Who: e.ID, Effect: a.VisualKey, PositionMode: position, DurationMS: remaining}, buff: event.BuffView{SkillID: id, Icon: iconID, Name: a.VisualKey, Desc: desc, Beneficial: true, RemainSec: equipmentAuraSourceBoundSeconds, PositionMode: position}})
	}
	return out
}
func TestAuditAuraAllocationParity(t *testing.T) {
	s := newCombatScene(t)
	for i := 1; i <= 20; i++ {
		id := domain.EntityID(i)
		join(t, s, id, int64(i), fmt.Sprintf("p%d", i), 100+float64(i%4)*10, 100+float64(i/4)*10)
		p := s.players[id]
		p.Player.Party = 1
		if i%7 == 0 {
			p.Player.Party = 2
		}
		cacheTestAura(s, p, int64(900+i), int32(100+i))
		pet := &entity.Entity{ID: domain.EntityID(100 + i), Kind: domain.KindPet, HP: 100, MaxHP: 100, Pos: p.Pos, Pet: &entity.Pet{Owner: id}}
		p.Player.Pet = pet.ID
		s.pets[pet.ID] = pet
		s.entities[pet.ID] = pet
	}
	s.stepEquipmentAuras()
	for _, source := range s.equipmentState().auraSources {
		for _, cap := range []int32{0, 1, 2, 6, 20, 30} {
			for _, pets := range []bool{false, true} {
				source.effect.MaxPlayers = cap
				source.effect.IncludeActivePet = pets
				got, want := s.equipmentAuraRecipients(source), s.auditLegacyAuraRecipients(source)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("recipients source=%d cap=%d pets=%v", source.owner, cap, pets)
				}
			}
		}
	}
	for _, e := range s.entities {
		if got, want := s.equipmentAuraStatuses(e), s.auditLegacyAuraStatuses(e); !reflect.DeepEqual(got, want) {
			t.Fatalf("presentation entity=%d", e.ID)
		}
	}
	s.tick += domain.Ticks(1000)
	for _, e := range s.entities {
		if got, want := s.equipmentAuraStatuses(e), s.auditLegacyAuraStatuses(e); !reflect.DeepEqual(got, want) {
			t.Fatalf("expired presentation entity=%d", e.ID)
		}
	}
}

func TestEquipmentAuraAllocationOwnsRecipientAndSourceSlices(t *testing.T) {
	s := newCombatScene(t)
	for i := 1; i <= 8; i++ {
		id := domain.EntityID(i)
		join(t, s, id, int64(i), fmt.Sprintf("p%d", i), 100+float64(i), 100)
		p := s.players[id]
		p.Player.Party = 1
		cacheTestAura(s, p, int64(900+i), 100)
		pet := &entity.Entity{ID: domain.EntityID(100 + i), Kind: domain.KindPet, HP: 100, MaxHP: 100, Pos: p.Pos, Pet: &entity.Pet{Owner: id}}
		p.Player.Pet = pet.ID
		s.pets[pet.ID], s.entities[pet.ID] = pet, pet
	}
	s.stepEquipmentAuras()
	rt := s.equipmentState()
	previousSources := rt.auraSources
	previousValues := append([]equipmentAuraSource(nil), previousSources...)
	source := previousSources[0]
	source.effect.MaxPlayers = 2 // Legacy headcount must not truncate the player prefix.
	first := s.equipmentAuraRecipients(source)
	if len(first) != 16 || first[0].ID != 1 || first[1].ID != 2 || first[8].ID != 101 || first[9].ID != 102 {
		t.Fatalf("pet append changed selected player prefix: %v", first)
	}
	first[0] = nil
	second := s.equipmentAuraRecipients(source)
	if second[0] != s.players[1] || second[1] != s.players[2] {
		t.Fatal("recipient slice storage leaked across calls or into scene state")
	}
	second[1] = nil
	if first[1] != s.players[2] {
		t.Fatal("recipient calls share their backing storage")
	}
	aura := rt.cache[1].effects[0].Effect.Aura
	aura.Value++
	s.stepEquipmentAuras()
	if !reflect.DeepEqual(previousSources, previousValues) {
		t.Fatal("source rebuild overwrote previously published source values")
	}
	if rt.auraSources[0].effect.Value != aura.Value {
		t.Fatal("source rebuild missed changed definition")
	}
	for id := range s.players {
		delete(rt.cache, id)
	}
	s.stepEquipmentAuras()
	if rt.auraSources != nil || len(rt.auraSchedules) != 0 || len(rt.auras) != 0 || rt.nextAuraScan != 0 {
		t.Fatal("preallocated source rebuild retained final-source state")
	}
}

func TestSortedEquipmentEntitiesPreservesOrderAndEmptyShape(t *testing.T) {
	if got := sortedEquipmentEntities(map[domain.EntityID]bool(nil)); got == nil || len(got) != 0 {
		t.Fatalf("empty IDs must retain nonnil empty shape: %#v", got)
	}
	values := map[domain.EntityID]bool{90: true, 1: true, 30: false, 0: true}
	want := []domain.EntityID{0, 1, 30, 90}
	got := sortedEquipmentEntities(values)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %v, want %v", got, want)
	}
	got[0] = 999
	if next := sortedEquipmentEntities(values); !reflect.DeepEqual(next, want) {
		t.Fatalf("returned IDs altered later sort: %v", next)
	}
}
