package scene

import (
	"fmt"
	"slices"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// The rune's stored attr38 is a flat HP amount per pulse, never a percentage.
// Use the existing two-second recovery cadence without inheriting resting's
// percentage/flat bonus or requiring the player to sit. This clock is transient:
// equipping, reviving, or entering a new scene starts a full interval, and there
// are no offline or missed-frame catch-up heals.
const astralRuneRegenInterval = domain.Tick(2000 / domain.TickMS)

type astralRuneRegenClock struct {
	owner   *entity.Entity
	next    domain.Tick
	sources []astralRuneRegenSource
}

// Compare exact eligible source identities instead of just their total amount:
// replacing one +1 rune with another must still start a new interval.
type astralRuneRegenSource struct {
	slot         domain.EquipSlot
	socket       uint8
	equipment    domain.ItemID
	equipmentUID int64
	runeUID      int64
	amount       int64
}

func astralRuneAffixText(item domain.ItemID, a domain.Affix) string {
	if item == domain.LastAstralRune && a.Attr == domain.AttrHPRegen && a.Mode == domain.ModeAbsolute && a.Value > 0 {
		return fmt.Sprintf("有效穿戴时每%d秒恢复%d点生命", astralRuneRegenInterval*domain.TickMS/1000, a.Value)
	}
	if item >= domain.FirstAstralRune && item <= domain.LastAstralRune && a.Attr == domain.AttrAtkSpeed && a.Mode == domain.ModePercent {
		// Use the same player-speed sign convention as ordinary equipment;
		// the durable rune snapshot remains the original negative interval %.
		line, _ := equipmentAffixLine(a)
		return line
	}
	return affixText(a)
}

func hasEquippedAstralRegenRune(worn *domain.EquipSet) bool {
	found := false
	worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			found = found || st.Sockets[i] == domain.LastAstralRune
		}
	})
	return found
}

// astralRuneRegenAmount deliberately reads only the durable 轸 rune snapshots.
// Ordinary gear/card attr38 remains untouched. Rune effects still work when a
// formation is absent or suppressed, just like the other standalone rune stats.
func (s *Scene) astralRuneRegenAmount(p *entity.Entity) int32 {
	amount, _ := s.astralRuneRegenState(p, nil)
	return amount
}

func (s *Scene) astralRuneRegenState(p *entity.Entity, active *domain.EquipSet) (int32, []astralRuneRegenSource) {
	if p == nil || p.Kind != domain.KindPlayer || p.Player == nil || p.Player.Char == nil || p.Player.Worn == nil {
		return 0, nil
	}
	// Validate identity across the complete worn set, including inactive gear.
	// A duplicated card UID, or a card sharing an equipment UID, contributes zero.
	uids := make(map[int64]int)
	hasRune := false
	p.Player.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		if st.UID > 0 {
			uids[st.UID]++
		}
		for i, uid := range st.SocketUIDs {
			if uid > 0 {
				uids[uid]++
			}
			if i < int(st.SocketCount) && st.Sockets[i] == domain.LastAstralRune {
				hasRune = true
			}
		}
	})
	if !hasRune {
		return 0, nil
	}
	if active == nil {
		active = s.functionalWorn(p)
	}
	var total int64
	var sources []astralRuneRegenSource
	active.Each(func(slot domain.EquipSlot, st domain.Stack) {
		def, ok := s.itemDef(st.Item)
		if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot || st.Count != 1 || st.UID <= 0 || uids[st.UID] != 1 ||
			(st.InstanceKind != domain.ItemInstanceEquipment && st.InstanceKind != domain.ItemInstanceNone) ||
			int(st.SocketCount) > len(st.Sockets) || int(st.SocketCount) > domain.EquipmentSocketCapacity(def, st) {
			return
		}
		for i := 0; i < int(st.SocketCount); i++ {
			card := st.SocketCards[i]
			if st.Sockets[i] != domain.LastAstralRune || st.SocketUIDs[i] <= 0 || uids[st.SocketUIDs[i]] != 1 ||
				!card.Initialized || card.Count == 0 || int(card.Count) > len(card.Effects) {
				continue
			}
			var amount int64
			for j := 0; j < int(card.Count); j++ {
				e := card.Effects[j]
				if e.Op == 1 && e.Probability == 100 && e.Attr == domain.AttrHPRegen && e.Mode == domain.ModeAbsolute && e.Value > 0 {
					amount += int64(e.Value)
				}
			}
			if amount > 0 {
				total += amount
				sources = append(sources, astralRuneRegenSource{
					slot: slot, socket: uint8(i), equipment: st.Item,
					equipmentUID: st.UID, runeUID: st.SocketUIDs[i], amount: amount,
				})
			}
		}
	})
	return domain.SaturateInt32(total), sources
}

// refreshStats calls this at each authoritative equipment/eligibility mutation.
// That boundary is necessary even with per-frame checks: unequip/re-equip of the
// same UID, or losing/regaining a requirement, can happen between world frames.
// Unrelated stat refreshes do not postpone a still-valid rune's next pulse.
func (s *Scene) invalidateAstralRuneRegen(p *entity.Entity, active *domain.EquipSet) {
	clock, scheduled := s.astralRuneRegen[p.ID]
	if !scheduled {
		return
	}
	if !p.Alive() || clock.owner != p {
		delete(s.astralRuneRegen, p.ID)
		return
	}
	amount, sources := s.astralRuneRegenState(p, active)
	if amount <= 0 || !slices.Equal(clock.sources, sources) {
		delete(s.astralRuneRegen, p.ID)
	}
}

func (s *Scene) stepAstralRuneRegeneration() {
	for id, clock := range s.astralRuneRegen {
		if s.players[id] != clock.owner {
			delete(s.astralRuneRegen, id)
		}
	}
	for id, p := range s.players {
		if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil || p.MaxHP <= 0 || (s.rift != nil && !s.riftCombatReady()) {
			delete(s.astralRuneRegen, id)
			continue
		}
		if !hasEquippedAstralRegenRune(p.Player.Worn) {
			delete(s.astralRuneRegen, id)
			continue
		}
		clock, scheduled := s.astralRuneRegen[id]
		// Eligibility must remain stable throughout the interval, not only at
		// its endpoints. The fixed equipment/socket bounds keep this bounded.
		amount, sources := s.astralRuneRegenState(p, nil)
		if amount <= 0 {
			delete(s.astralRuneRegen, id)
			continue
		}
		if !scheduled || clock.owner != p || !slices.Equal(clock.sources, sources) {
			if s.astralRuneRegen == nil {
				s.astralRuneRegen = make(map[domain.EntityID]astralRuneRegenClock)
			}
			s.astralRuneRegen[id] = astralRuneRegenClock{owner: p, next: s.tick + astralRuneRegenInterval, sources: sources}
			continue
		}
		if s.tick < clock.next {
			continue
		}
		// One pulse at most. Long pauses cannot generate a burst or a loop.
		s.astralRuneRegen[id] = astralRuneRegenClock{owner: p, next: s.tick + astralRuneRegenInterval, sources: sources}
		if p.HP >= p.MaxHP {
			continue
		}
		ev := combat.Restore(p, p, amount, 0)
		if ev.Amount > 0 {
			s.emit(ev)
			s.emitAttributesIfPlayer(p)
		}
	}
}
