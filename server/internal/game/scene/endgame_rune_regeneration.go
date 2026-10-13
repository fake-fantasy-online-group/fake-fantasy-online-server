package scene

import (
	"fmt"
	"slices"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// The rune's stored attr38 is a flat HP amount per pulse, never a percentage.
// Use the source's configured recovery cadence without inheriting resting's
// percentage/flat bonus or requiring the player to sit. This clock is transient:
// equipping, reviving, or entering a new scene starts a full interval, and there
// are no offline or missed-frame catch-up heals.
type astralRuneRegenClock struct {
	owner   *entity.Entity
	next    map[int32]domain.Tick
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
	intervalMS   int32
	rune         domain.ItemID
}

// periodicRecoveryAffix admits a numerical affix into the online recovery
// bucket only when the source's typed rule gives it that meaning. Ordinary
// attr38 remains a native stat and is never silently promoted to a pulse.
func periodicRecoveryAffix(r domain.AstralRuneDef, a domain.Affix) bool {
	return domain.ValidRecoveryIntervalMS(r.RegenerationIntervalMS) &&
		a.Attr == domain.AttrHPRegen && a.Mode == domain.ModeAbsolute && a.Value > 0
}
func (s *Scene) astralRuneAffixText(item domain.ItemID, a domain.Affix) string {
	rune, known := s.endgameCatalog.Rune(item)
	if known && periodicRecoveryAffix(rune, a) {
		return periodicRecoveryText(rune.RegenerationIntervalMS, int64(a.Value))
	}
	if known && a.Attr == domain.AttrAtkSpeed && a.Mode == domain.ModePercent {
		// Use the same player-speed sign convention as ordinary equipment;
		// the durable rune snapshot remains the original negative interval %.
		line, _ := equipmentAffixLine(a)
		return line
	}
	return affixText(a)
}

func (s *Scene) hasEquippedAstralRegenRune(worn *domain.EquipSet) bool {
	found := false
	worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
			rune, ok := s.endgameCatalog.Rune(st.Sockets[i])
			found = found || ok && domain.ValidRecoveryIntervalMS(rune.RegenerationIntervalMS)
		}
	})
	return found
}

// astralRuneRegenAmount aggregates durable, explicitly periodic rune affixes.
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
			if i < int(st.SocketCount) {
				rune, ok := s.endgameCatalog.Rune(st.Sockets[i])
				hasRune = hasRune || ok && domain.ValidRecoveryIntervalMS(rune.RegenerationIntervalMS)
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
			rune, known := s.endgameCatalog.Rune(st.Sockets[i])
			if !known || !domain.ValidRecoveryIntervalMS(rune.RegenerationIntervalMS) || st.SocketUIDs[i] <= 0 || uids[st.SocketUIDs[i]] != 1 ||
				!card.Initialized || card.Count == 0 || int(card.Count) > len(card.Effects) {
				continue
			}
			var amount int64
			for j := 0; j < int(card.Count); j++ {
				e := card.Effects[j]
				if e.Op == 1 && e.Probability == 100 && periodicRecoveryAffix(rune, domain.Affix{Attr: e.Attr, Mode: e.Mode, Value: e.Value}) {
					amount += int64(e.Value)
				}
			}
			if amount > 0 {
				total += amount
				sources = append(sources, astralRuneRegenSource{
					slot: slot, socket: uint8(i), equipment: st.Item,
					equipmentUID: st.UID, runeUID: st.SocketUIDs[i], amount: amount, intervalMS: rune.RegenerationIntervalMS, rune: st.Sockets[i],
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
		if !s.hasEquippedAstralRegenRune(p.Player.Worn) {
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
			next := make(map[int32]domain.Tick)
			for interval := range recoveryBuckets(sources) {
				next[interval] = s.tick + domain.Tick(interval/int32(domain.TickMS))
			}
			s.astralRuneRegen[id] = astralRuneRegenClock{owner: p, next: next, sources: sources}
			continue
		}
		var due int64
		for interval, flat := range recoveryBuckets(sources) {
			if s.tick >= clock.next[interval] {
				// At most one pulse per cadence; no offline/missed-frame catch-up.
				due += flat
				clock.next[interval] = s.tick + domain.Tick(interval/int32(domain.TickMS))
			}
		}
		s.astralRuneRegen[id] = clock
		if due <= 0 {
			continue
		}
		amount = domain.SaturateInt32(due)
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

func recoveryBuckets(sources []astralRuneRegenSource) domain.PeriodicRecoveryBuckets {
	buckets := make(domain.PeriodicRecoveryBuckets)
	for _, source := range sources {
		buckets.AddFlatHP(source.intervalMS, source.amount)
	}
	return buckets
}
func periodicRecoveryText(intervalMS int32, amount int64) string {
	return fmt.Sprintf("有效穿戴时每%g秒恢复%d点生命", float64(intervalMS)/1000, domain.SaturateInt32(amount))
}
