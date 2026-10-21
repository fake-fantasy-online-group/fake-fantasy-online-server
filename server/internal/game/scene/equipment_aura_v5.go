package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"slices"
	"sort"
)

// Source-bound buffs have no countdown; the short visual lease is internal only.
const equipmentAuraSourceBoundSeconds int32 = -2

type equipmentAuraSource struct {
	owner  domain.EntityID
	uid    int64
	party  domain.PartyID
	effect domain.EquipmentAura
}
type equipmentAuraGrant struct {
	owner   domain.EntityID
	uid     int64
	effect  domain.EquipmentAura
	expires domain.Tick
}

// One spatial membership refresh clock per complete source rule. Source UID,
// party and definition changes cannot inherit a previous source's lease/debt.
type equipmentAuraSchedule struct {
	next       domain.Tick
	recipients map[domain.EntityID]equipmentAuraGrant
}

// EquipmentAuraView is shared by authoritative observations and the status
// presentation path. Every receiver names its winning source explicitly.
type EquipmentAuraView = domain.EquipmentAuraView

func (s *Scene) equipmentAuraViews(p *entity.Entity) []EquipmentAuraView {
	if p == nil {
		return nil
	}
	var out []EquipmentAuraView
	for _, g := range s.equipmentState().auras[p.ID] {
		if !s.equipmentAuraGrantValid(p, g) {
			continue
		}
		a := g.effect
		out = append(out, EquipmentAuraView{AuraID: a.ID, Family: a.Family, SourceEntityID: g.owner, SourceUID: g.uid, Rank: a.FixedRank, AssetKey: a.VisualKey, Modifier: a.Modifier, Element: a.Element, Value: a.Value, LeaseMS: a.LeaseMS})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AuraID < out[j].AuraID })
	return out
}
func (s *Scene) stepEquipmentAuras() {
	rt := s.equipmentState()
	var sources []equipmentAuraSource
	for _, id := range sortedEquipmentEntities(s.players) {
		p := s.players[id]
		if p == nil || !p.Alive() || s.entityMapLoading(p) || !s.equipmentCombatReady(p) {
			continue
		}
		sources = append(sources, s.classSetAuraSources(p)...)
		for _, e := range s.equipmentEffects(p).effects {
			if a := e.Effect.Aura; a != nil && e.Effect.Kind == "aura" {
				// Size a fresh snapshot only after finding a source; no sources must stay nil.
				if sources == nil && len(rt.auraSources) > 0 {
					sources = make([]equipmentAuraSource, 0, len(rt.auraSources))
				}
				sources = append(sources, equipmentAuraSource{id, e.SourceUID, p.Player.Party, *a})
			}
		}
	}
	if len(sources) == 0 && len(rt.auras) == 0 && len(rt.auraSchedules) == 0 {
		return
	}
	changed := (sources == nil) != (rt.auraSources == nil) || !slices.Equal(sources, rt.auraSources)
	if !changed && s.tick < rt.nextAuraScan {
		for id, grants := range rt.auras {
			for _, g := range grants {
				target, owner := s.entities[id], s.entities[g.owner]
				if owner == nil || !s.equipmentAuraGrantValid(target, g) {
					changed = true
					break
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			return
		}
	}
	defer func() {
		for _, id := range sortedEquipmentEntities(s.players) {
			s.publishEquipmentCombatStats(s.players[id])
		}
	}()
	rt.auraSources = sources
	forceRefresh := rt.nextAuraScan == 0
	rt.nextAuraScan = 0
	schedules := make(map[equipmentAuraSource]equipmentAuraSchedule, len(sources))
	next := map[domain.EntityID]map[string]equipmentAuraGrant{}
	add := func(target *entity.Entity, source equipmentAuraSource, g equipmentAuraGrant) {
		a := source.effect
		m := next[target.ID]
		if m == nil {
			m = map[string]equipmentAuraGrant{}
			next[target.ID] = m
		}
		var replace []string
		for key, old := range m {
			// Offensive auras of the same element share one winner even when a
			// future catalog uses a different family name. Defense is independent.
			sameElement := a.Modifier == "element_damage_bp" && old.effect.Modifier == a.Modifier && old.effect.Element == a.Element
			if key != a.Family && !sameElement {
				continue
			}
			stronger := a.Value > old.effect.Value || a.Value == old.effect.Value && (a.Priority > old.effect.Priority || a.Priority == old.effect.Priority && (source.owner < old.owner || source.owner == old.owner && source.uid < old.uid))
			if !stronger {
				return
			}
			replace = append(replace, key)
		}
		for _, key := range replace {
			delete(m, key)
		}
		m[a.Family] = g
	}
	for _, source := range sources {
		state, exists := rt.auraSchedules[source]
		due := forceRefresh || !exists || s.tick >= state.next
		if !due {
			for id, grant := range state.recipients {
				if !s.equipmentAuraGrantValid(s.entities[id], grant) {
					due = true
					break
				}
			}
		}
		if due {
			state = equipmentAuraSchedule{next: s.tick + domain.Ticks(int(source.effect.RefreshMS)), recipients: map[domain.EntityID]equipmentAuraGrant{}}
			for _, target := range s.equipmentAuraRecipients(source) {
				state.recipients[target.ID] = equipmentAuraGrant{source.owner, source.uid, source.effect, s.tick + domain.Ticks(int(source.effect.LeaseMS))}
			}
		}
		schedules[source] = state
		if rt.nextAuraScan == 0 || state.next < rt.nextAuraScan {
			rt.nextAuraScan = state.next
		}
		for _, id := range sortedEquipmentEntities(state.recipients) {
			grant := state.recipients[id]
			if target := s.entities[id]; s.equipmentAuraGrantValid(target, grant) {
				add(target, source, grant)
			}
		}
	}
	// Omitted/dead/disconnected sources drop their own timers and grants now.
	rt.auraSchedules = schedules
	old := rt.auras
	rt.auras = next
	affected := map[domain.EntityID]bool{}
	for id := range old {
		affected[id] = true
	}
	for id := range next {
		affected[id] = true
	}
	for _, id := range sortedEquipmentEntities(affected) {
		e := s.entities[id]
		if e == nil {
			continue
		}
		for family, g := range old[id] {
			n, ok := next[id][family]
			if !ok || n.effect.VisualKey != g.effect.VisualKey {
				s.emitEquipmentAuraRemoval(e, g)
			}
		}
		if classSetAuraStatKey(old[id]) != classSetAuraStatKey(next[id]) && e.Player != nil {
			s.invalidateCombatAttributes(e)
			s.refreshStats(e)
			s.emitAttributesIfPlayer(e)
		}
		// Lease renewal extends the existing keyed effect, never creates another
		// emitter. Numeric membership stays server-authoritative if visuals are hidden.
		s.pushStatusSnapshots(e)
	}
}
func (s *Scene) equipmentAuraRecipients(source equipmentAuraSource) []*entity.Entity {
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
	if a.MaxPlayers > 0 && len(candidates) > int(a.MaxPlayers) {
		candidates = candidates[:a.MaxPlayers]
	}
	// candidates is private to this call. Pet appends cannot overwrite its player prefix.
	out := candidates
	if len(out) == 0 {
		out = nil
	}
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

func (s *Scene) auraRecipient(owner, target *entity.Entity) bool {
	if owner == nil || target == nil {
		return false
	}
	if target.Pet != nil {
		p := s.players[target.Pet.Owner]
		return p != nil && p.Player.Pet == target.ID && (owner.ID == p.ID || sameParty(owner, p))
	}
	return target.Player != nil && (target.ID == owner.ID || sameParty(owner, target))
}
func (s *Scene) emitEquipmentAuraRemoval(e *entity.Entity, g equipmentAuraGrant) {
	s.emit(event.EntityStatusEffect{Who: e.ID, Effect: g.effect.VisualKey, DurationMS: 0})
}
func (s *Scene) equipmentAuraStatuses(e *entity.Entity) []statusPresentation {
	if e == nil || !e.Alive() {
		return nil
	}
	grants := s.equipmentState().auras[e.ID]
	var ordered []equipmentAuraGrant
	for _, g := range grants {
		if g.expires > s.tick && s.equipmentAuraGrantValid(e, g) {
			if ordered == nil {
				ordered = make([]equipmentAuraGrant, 0, len(grants))
			}
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
	if len(ordered) > 0 {
		out = make([]statusPresentation, 0, len(ordered))
	}
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
		desc := fmt.Sprintf("%d级光环：%s +%s；%s +%s。\n来源：%s；范围%d，含来源最多%d人；同类光环只取最高，不叠加。", a.FixedRank, label, equipmentPercent(a.Value), pvpCondition, equipmentPercent(int32(int64(a.Value)*int64(a.PVPScaleBP)/10000)), sourceName, a.RadiusPX, a.MaxPlayers)
		if a.IncludeActivePet {
			desc += "惠及出战宠物。"
		}
		if g.owner == e.ID {
			desc += "\n自身装备有效时持续生效，脱装、损坏、死亡或离线时移除。"
		} else {
			desc += "\n来源装备有效且处于同队范围内时持续生效，来源脱装、损坏、死亡、离线或自身离队、离开范围时移除。"
		}
		if classSetAuraID(a) != 0 {
			position = 0
			desc = classSetAuraDescription(a, sourceName)
		}
		remaining := nonnegativeI32((g.expires - s.tick).Millis())
		out = append(out, statusPresentation{name: a.VisualKey, desc: desc, icon: event.StatusIconView{Status: id, Icon: iconID, Beneficial: true, RemainMS: -1}, effect: event.EntityStatusEffect{Who: e.ID, Effect: a.VisualKey, PositionMode: position, DurationMS: remaining}, buff: event.BuffView{SkillID: id, Icon: iconID, Name: a.VisualKey, Desc: desc, Beneficial: true, RemainSec: equipmentAuraSourceBoundSeconds, PositionMode: position}})
	}
	return out
}
func equipmentAuraStatusID(id string) int32 {
	switch id {
	case "white_tiger_guard":
		return 1901001
	case "ember_shelter":
		return 1901002
	case "holy_feather":
		return 1901003
	case "empty_talisman_ward":
		return 1901004
	case "north_gate_ice":
		return 1901005
	case "backward_flame":
		return 1901006
	case "paper_donkey_frost":
		return 1901007
	}
	h := uint32(2166136261)
	for _, c := range []byte(id) {
		h = (h ^ uint32(c)) * 16777619
	}
	return 1902000 + int32(h%100000)
}
func (s *Scene) removeEquipmentEntity(id domain.EntityID) {
	s.clearClassSetMelee(id)
	if s.equipment == nil {
		return
	}
	delete(s.equipment.cache, id)
	delete(s.equipment.observations, id)
	delete(s.equipment.classSetCasters, id)
	delete(s.equipment.absorption, id)
	s.equipment.nextAuraScan = 0
	s.stepEquipmentAuras()
}

// Eligibility can change between the periodic spatial scans. Never retain a
// numerical or observed benefit after its source became invalid in this frame.
func (s *Scene) equipmentAuraGrantValid(target *entity.Entity, g equipmentAuraGrant) bool {
	source := s.players[g.owner]
	if source == nil || !source.Alive() || target == nil || !target.Alive() || g.expires <= s.tick || !s.equipmentCombatReady(source) || !s.equipmentCombatReady(target) || s.entityMapLoading(source) || s.entityMapLoading(target) || source.Pos.MapID != target.Pos.MapID || sqDist(source.Pos, target.Pos) > float64(g.effect.RadiusPX)*float64(g.effect.RadiusPX) || !s.auraRecipient(source, target) {
		return false
	}
	if target.Pet != nil {
		player := s.players[target.Pet.Owner]
		if player == nil || !player.Alive() || s.entityMapLoading(player) || !s.equipmentCombatReady(player) || player.Pos.MapID != source.Pos.MapID || sqDist(source.Pos, player.Pos) > float64(g.effect.RadiusPX)*float64(g.effect.RadiusPX) {
			return false
		}
	}
	if suit := classSetAuraID(g.effect); suit != 0 {
		return g.uid == -int64(930000+suit) && s.classSetPieces(source, suit) >= 6
	}
	for _, a := range s.equipmentEffects(source).effects {
		if a.SourceUID == g.uid && a.Effect.Aura != nil && *a.Effect.Aura == g.effect {
			return true
		}
	}
	return false
}

// Offensive auras contribute to the existing additive bucket after their own
// configured PvP scale. Keep fractional BP until all matching contributions are
// summed; the default 50% case remains identical even for odd-BP equipment.
func (s *Scene) equipmentAuraDamageNumerator(src *entity.Entity, element int, pvp bool) int64 {
	if src == nil || element < 0 {
		return 0
	}
	var numerator int64
	for _, grant := range s.equipmentState().auras[src.ID] {
		a := grant.effect
		if a.Modifier != "element_damage_bp" || domain.ElementIndex(a.Element) != element || !s.equipmentAuraGrantValid(src, grant) {
			continue
		}
		scale := int32(10000)
		if pvp {
			scale = a.PVPScaleBP
		}
		numerator += int64(a.Value) * int64(scale)
	}
	return numerator
}

// Reuse installed skill artwork for new equipment auras, not their synthetic
// status identities. IDs verified against skills.json and item/item/*.gso index.
func equipmentAuraIconID(id string) int32 {
	switch id {
	case "white_tiger_guard":
		return 5017 // 铁城神盾
	case "ember_shelter":
		return 7084 // 火盾术
	case "holy_feather":
		return 7006 // 光盾术
	case "empty_talisman_ward":
		return 5062 // 魔镜术
	case "north_gate_ice":
		return 7093 // 冰盾术
	case "backward_flame":
		return 7085 // 火系掌握
	case "paper_donkey_frost":
		return 7086 // 冰系掌握
	default:
		return 0
	}
}
