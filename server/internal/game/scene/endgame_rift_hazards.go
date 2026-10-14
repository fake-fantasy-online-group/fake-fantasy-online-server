package scene

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type endgameRiftHazard struct {
	ViewerGeneration              map[domain.EntityID]uint64
	ID                            string
	Source                        domain.EntityID
	SourceLevel                   int32
	Attack                        domain.Stats
	Pos                           domain.Pos
	Ability                       domain.RiftAbilityRule
	BornMS, ExpiresMS, LastStepMS int64
	Viewers                       map[domain.EntityID]int64
	Occupants                     map[domain.EntityID]endgameRiftOccupant
	PulseIndex                    int64
	PulseHits                     map[domain.EntityID]bool
	Walls                         []temporaryWall
}
type endgameRiftOccupant struct{ EnteredMS, NextMS int64 }

func (s *Scene) createRiftHazard(m *entity.Entity, pos domain.Pos, a domain.RiftAbilityRule, now int64) bool {
	if !s.eliteRift() || pos.MapID != s.id.MapID || math.IsNaN(pos.X) || math.IsNaN(pos.Y) || math.IsInf(pos.X, 0) || math.IsInf(pos.Y, 0) {
		return false
	}
	x := s.eliteSnapshot()
	seq := x.Sequence + 1
	floor := int32(0)
	if s.rift.data.Floor != nil {
		floor = s.rift.data.Floor.FloorIndex
	}
	id := fmt.Sprintf("%s/%d/e%d", s.rift.data.Run.RunID, floor, seq)
	h := &endgameRiftHazard{ID: id, Source: m.ID, SourceLevel: m.Level, Attack: m.Stats, Pos: pos, Ability: a, BornMS: now, ExpiresMS: now + a.DurationMS, LastStepMS: now, Viewers: map[domain.EntityID]int64{}, Occupants: map[domain.EntityID]endgameRiftOccupant{}, PulseIndex: -1, PulseHits: map[domain.EntityID]bool{}}
	if a.Mechanism == "walls" {
		h.Walls = s.planRiftWalls(id, pos, a, now)
		if len(h.Walls) != int(a.Count) {
			return false
		}
		x.Walls = s.temporaryWallSnapshot()
	}
	x.Sequence = seq
	x.Effects[id] = h
	s.eliteChanged()
	return true
}
func (s *Scene) planRiftWalls(owner string, center domain.Pos, a domain.RiftAbilityRule, now int64) []temporaryWall {
	// Three disconnected short segments on an open arc, with a full escape side.
	// Geometry safety (actors, anchors, corridors and existing walls) is atomic.
	for rotation := 0; rotation < 16; rotation++ {
		theta := float64(rotation) * math.Pi / 8
		for _, radius := range []float64{180, 220, 280, 340} {
			walls := make([]temporaryWall, 0, 3)
			for i := 0; i < 3; i++ {
				angle := theta + float64(i)*math.Pi/2
				x, y := center.X+radius*math.Cos(angle), center.Y+radius*math.Sin(angle)
				dx, dy := -math.Sin(angle)*float64(a.Length)/2, math.Cos(angle)*float64(a.Length)/2
				walls = append(walls, temporaryWall{ID: fmt.Sprintf("%s/w%d", owner, i), Owner: owner, MapID: center.MapID, X: x - dx, Y: y - dy, X2: x + dx, Y2: y + dy, Width: float64(a.Width), ExpiresAtMS: now + a.DurationMS})
			}
			// Segment endpoints must also honor the configured minimum opening.
			gapOK := true
			for i := 0; i < len(walls); i++ {
				for j := i + 1; j < len(walls); j++ {
					wi, wj := walls[i], walls[j]
					for _, p := range [][2]float64{{wi.X, wi.Y}, {wi.X2, wi.Y2}} {
						for _, q := range [][2]float64{{wj.X, wj.Y}, {wj.X2, wj.Y2}} {
							if math.Hypot(p[0]-q[0], p[1]-q[1])-float64(a.Width) < float64(a.Gap) {
								gapOK = false
							}
						}
					}
				}
			}
			if gapOK && s.addTemporaryWalls(walls) {
				return walls
			}
		}
	}
	return nil
}
func (s *Scene) armRiftHazards(p *entity.Entity, now int64) {
	if !s.eliteRift() || !s.riftAbilityPlayerReady(p) {
		return
	}
	for _, h := range s.eliteSnapshot().Effects {
		if now >= h.ExpiresMS {
			continue
		} // Not included in the just-published snapshot.
		if h.ViewerGeneration == nil {
			h.ViewerGeneration = map[domain.EntityID]uint64{}
		}
		if h.Viewers == nil {
			h.Viewers = map[domain.EntityID]int64{}
		}
		if _, seen := h.Viewers[p.ID]; !seen || h.ViewerGeneration[p.ID] != s.rift.hazardViewerGeneration[p.ID] {
			// A full current snapshot was just sent. No historical pulses are replayed.
			h.Viewers[p.ID] = now + 1000
			h.ViewerGeneration[p.ID] = s.rift.hazardViewerGeneration[p.ID]
			if h.Ability.Mechanism == "poison_pool" && riftDistance2(h.Pos, p.Pos) <= float64(h.Ability.Radius)*float64(h.Ability.Radius) {
				h.Occupants[p.ID] = endgameRiftOccupant{EnteredMS: now, NextMS: now + h.Ability.PulseMS}
			}
		}
	}
}
func (s *Scene) forgetRiftHazardPlayer(id domain.EntityID) {
	if s.rift == nil || s.rift.data.Elite == nil {
		return
	}
	if s.rift.hazardViewerGeneration == nil {
		s.rift.hazardViewerGeneration = map[domain.EntityID]uint64{}
	}
	s.rift.hazardViewerGeneration[id]++
	delete(s.rift.data.Elite.PoisonNext, id)
	for _, h := range s.rift.data.Elite.Effects {
		delete(h.Viewers, id)
		delete(h.Occupants, id)
		delete(h.PulseHits, id)
	}
}
func (s *Scene) riftHazardPlayerReady(h *endgameRiftHazard, p *entity.Entity, now int64) bool {
	if !s.riftAbilityPlayerReady(p) {
		return false
	}
	after, seen := h.Viewers[p.ID]
	return seen && now >= after && h.ViewerGeneration[p.ID] == s.rift.hazardViewerGeneration[p.ID]
}
func (s *Scene) stepRiftHazards(now int64) {
	x := s.eliteSnapshot()
	ids := make([]string, 0, len(x.Effects))
	for id := range x.Effects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	type poisonHit struct {
		h     *endgameRiftHazard
		p     *entity.Entity
		pct   int32
		power int64
	}
	poison := map[domain.EntityID]poisonHit{}
	poisonDue := map[domain.EntityID]bool{}
	for _, id := range ids {
		h := x.Effects[id]
		if h == nil {
			continue
		}
		a := h.Ability
		switch a.Mechanism {
		case "death_lava", "ice_orb":
			if now >= h.ExpiresMS {
				// Consume before callbacks: same-frame death/clear/reload cannot replay it.
				delete(x.Effects, id)
				s.eliteChanged()
				s.recordRiftImpact(h, now)
				for _, pid := range s.riftHazardPlayerIDs(h) {
					p := s.players[pid]
					if s.riftHazardPlayerReady(h, p, now) && riftDistance2(h.Pos, p.Pos) <= float64(a.Radius)*float64(a.Radius) {
						s.hitRiftHazard(h, p, a.DamagePct)
					}
				}
			}
		case "lightning_orb":
			elapsed := now - h.BornMS - a.FirstPulseMS
			if elapsed >= 0 {
				pulse := elapsed / a.PulseMS
				age := elapsed % a.PulseMS
				if pulse != h.PulseIndex {
					h.PulseIndex = pulse
					h.PulseHits = map[domain.EntityID]bool{}
				}
				if age <= a.PropagationMS && now <= h.ExpiresMS {
					start := h.BornMS + a.FirstPulseMS + pulse*a.PulseMS
					previous := max(start, h.LastStepMS)
					outer := float64(a.Radius) * float64(age) / float64(a.PropagationMS)
					inner := math.Max(0, float64(a.Radius)*float64(previous-start)/float64(a.PropagationMS)-float64(a.Width))
					for _, pid := range s.riftHazardPlayerIDs(h) {
						p := s.players[pid]
						if h.PulseHits[pid] || !s.riftHazardPlayerReady(h, p, now) {
							continue
						}
						d := math.Sqrt(riftDistance2(h.Pos, p.Pos))
						if d >= inner && d <= outer {
							h.PulseHits[pid] = true
							s.hitRiftHazard(h, p, a.DamagePct)
						}
					}
				}
			}
		case "poison_pool":
			for _, pid := range s.riftHazardPlayerIDs(h) {
				p := s.players[pid]
				if !s.riftAbilityPlayerReady(p) || h.ViewerGeneration[pid] != s.rift.hazardViewerGeneration[pid] || riftDistance2(h.Pos, p.Pos) > float64(a.Radius)*float64(a.Radius) {
					delete(h.Occupants, pid)
					continue
				}
				o, inside := h.Occupants[pid]
				if !inside {
					h.Occupants[pid] = endgameRiftOccupant{EnteredMS: now, NextMS: now + a.PulseMS}
					continue
				}
				if !s.riftHazardPlayerReady(h, p, now) || now-o.EnteredMS < a.PulseMS || now > h.ExpiresMS {
					continue
				}
				// At most one tick per scene observation; advance over missed intervals.
				seconds := (now - o.EnteredMS) / a.PulseMS
				pct := min(a.MaxDamagePct, a.DamagePct+int32(max(int64(0), seconds-1))*a.StepDamagePct)
				if now >= o.NextMS {
					poisonDue[pid] = true
					o.NextMS = now + a.PulseMS
					h.Occupants[pid] = o
				}
				power := int64(s.riftPoisonNominalDamage(h, p, pct))
				old, ok := poison[pid]
				if !ok || power > old.power {
					poison[pid] = poisonHit{h, p, pct, power}
				}
			}
		}
		h.LastStepMS = now
		if now >= h.ExpiresMS && x.Effects[id] != nil {
			delete(x.Effects, id)
			if a.Mechanism == "walls" {
				s.removeTemporaryWalls(id)
			}
			s.eliteChanged()
		}
	}
	pids := make([]domain.EntityID, 0, len(poison))
	for id := range poison {
		pids = append(pids, id)
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	for id := range x.PoisonNext {
		if _, inside := poison[id]; !inside {
			delete(x.PoisonNext, id)
		}
	}
	for _, id := range pids {
		hit := poison[id]
		if poisonDue[id] && now >= x.PoisonNext[id] && s.riftHazardPlayerReady(hit.h, hit.p, now) {
			x.PoisonNext[id] = now + hit.h.Ability.PulseMS
			s.hitRiftHazard(hit.h, hit.p, hit.pct)
		}
	}
}
func (s *Scene) riftHazardPlayerIDs(h *endgameRiftHazard) []domain.EntityID {
	ids := make([]domain.EntityID, 0, len(h.Viewers))
	for id := range h.Viewers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func (s *Scene) hitRiftHazard(h *endgameRiftHazard, p *entity.Entity, pct int32) {
	if !s.riftAbilityPlayerReady(p) || !s.staticPathWalkable(h.Pos, p.Pos) {
		return
	}
	// A detached, dead source preserves the accepted attack snapshot. It cannot
	// be targeted, controlled, killed or reflected into a second death/reward.
	source := &entity.Entity{ID: h.Source, Kind: domain.KindMonster, Level: h.SourceLevel, Pos: h.Pos, Stats: h.Attack, Status: domain.NewStatusSet(), Monster: &entity.Monster{}}
	magic := h.Attack.MAtk > h.Attack.MaxAtk
	hurt := int32(domain.HurtPhysical)
	if magic {
		hurt = domain.HurtFire
		if h.Ability.Mechanism == "ice_orb" {
			hurt = domain.HurtIce
		}
	}
	ev, raw, reflected := s.resolveStrikeStatuses(source, p, &combat.HitTracker{}, combat.Blow{Origin: domain.DamageAuraOrigin, Area: true, Magic: magic, HurtType: hurt, SkillPct: pct, GuaranteedHit: true, AttackStatsSnapshot: true, AttackStats: h.Attack})
	s.emit(ev)
	s.afterStrikeStatuses(source, p, ev, raw, reflected)
	s.consumeHitDurability(p, ev)
	s.emitAttributesIfPlayer(p)
	if ev.Flag.Has(event.DamageFatal) {
		s.onDeath(p, h.Source)
		s.retainFatalRiftImpact(h, p)
		return
	}
	if h.Ability.Mechanism == "ice_orb" && !ev.Flag.Has(event.DamageMiss) {
		def, ok := s.statuses.Get(domain.StatusFreeze, 1)
		if !ok {
			def = domain.StatusDef{ID: domain.StatusFreeze, Level: 1, Name: "冰冻", TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun}
		}
		def.DurationSec = int32(h.Ability.FreezeMS / 1000)
		def.ChanceBP = 10000
		def.ChanceExplicit = true
		def.SourceEffectHitKnown = true
		def.SourceEffectHitBP = h.Attack.EffectHitBP
		s.applyStatusDef(p, def, h.Source)
	}
}
func (s *Scene) riftHazardViews(now int64) []domain.EndgameRiftHazardView {
	if !s.eliteRift() || s.rift.data.Elite == nil {
		return nil
	}
	var out []domain.EndgameRiftHazardView
	for _, h := range s.rift.data.Elite.Effects {
		if now >= h.ExpiresMS {
			continue
		}
		a := h.Ability
		v := domain.EndgameRiftHazardView{ID: h.ID, SourceID: strconv.FormatUint(uint64(h.Source), 10), AbilityID: a.ID, Mechanism: a.Mechanism, X: int32(h.Pos.X), Y: int32(h.Pos.Y), Radius: a.Radius, Width: a.Width, BornMS: h.BornMS, ExpiresMS: h.ExpiresMS, RemainingMS: int32(h.ExpiresMS - now), FirstPulseMS: a.FirstPulseMS, PulseMS: a.PulseMS, PropagationMS: a.PropagationMS}
		for _, w := range h.Walls {
			v.Walls = append(v.Walls, domain.EndgameRiftWallView{ID: w.ID, X: w.X, Y: w.Y, X2: w.X2, Y2: w.Y2, Width: w.Width})
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Impact receipts exist only for a short presentation window and only for the
// connection generations which saw that hazard. Cleanup never fabricates one.
type endgameRiftImpact struct {
	FatalViewers map[domain.EntityID]bool
	View         domain.EndgameRiftImpactView
	Viewers      map[domain.EntityID]uint64
}

func (s *Scene) recordRiftImpact(h *endgameRiftHazard, now int64) {
	receipt := endgameRiftImpact{View: domain.EndgameRiftImpactView{ID: h.ID, Mechanism: h.Ability.Mechanism, X: int32(h.Pos.X), Y: int32(h.Pos.Y), Radius: h.Ability.Radius, AtMS: now, ExpiresMS: now + 700}, Viewers: map[domain.EntityID]uint64{}}
	for id := range h.Viewers {
		if s.riftHazardPlayerReady(h, s.players[id], now) {
			receipt.Viewers[id] = s.rift.hazardViewerGeneration[id]
		}
	}
	live := s.rift.hazardImpacts[:0]
	for _, old := range s.rift.hazardImpacts {
		if old.View.ExpiresMS > now {
			live = append(live, old)
		}
	}
	s.rift.hazardImpacts = append(live, receipt)
}
func (s *Scene) riftImpactViews(p *entity.Entity, now int64) []domain.EndgameRiftImpactView {
	if p == nil || !s.eliteRift() || p.Pos.MapID != s.id.MapID {
		return nil
	}
	var out []domain.EndgameRiftImpactView
	for _, i := range s.rift.hazardImpacts {
		if generation, ok := i.Viewers[p.ID]; ok && generation == s.rift.hazardViewerGeneration[p.ID] && (!i.FatalViewers[p.ID] || !p.Alive()) && i.View.AtMS <= now && now < i.View.ExpiresMS {
			out = append(out, i.View)
		}
	}
	return out
}

// Rank overlapping pools by target-aware nominal damage, not raw source attack.
// This uses the native defense formula and common mitigation buckets on a cloned
// shield set. It neither rolls procs/crit nor consumes HP, shields or combat RNG;
// only the selected pool performs the ordinary authoritative random strike.
func (s *Scene) riftPoisonNominalDamage(h *endgameRiftHazard, p *entity.Entity, pct int32) int32 {
	if p == nil || gmGodEnabled(p) || p.Status.Invulnerable() || !s.staticPathWalkable(h.Pos, p.Pos) {
		return 0
	}
	magic := h.Attack.MAtk > h.Attack.MaxAtk
	midpoint := domain.SaturateInt32((int64(h.Attack.MinAtk) + int64(h.Attack.MaxAtk)) / 2)
	amount := combat.RollPhysicalSkill(midpoint, midpoint, p.Stats.PhysResist, pct, nil)
	defensive := s.equipmentDefense(p, false)
	elemental := domain.ElementCombatStats{}
	if magic {
		amount = combat.RollMagicFixedDefense(h.Attack.MAtk, pct, p.Stats.MDef, p.Stats.MagicResist)
		elemental = defensive.Elements[domain.HurtElementIndex(domain.HurtFire)]
	}
	guard := domain.EvaluateUniqueTraits(s.uniqueTraits(p), domain.UniqueContext{HPPercent: uniquePercent(p.HP, p.MaxHP), Magic: magic})
	mitigation := int64(elemental.MitigationBP) + int64(guard.MitigationBP)
	if !magic {
		mitigation += int64(defensive.PhysicalMitigationBP)
	}
	half := 0
	if magic && s.incomingMagicHalfChance(p) >= 10000 {
		half = 1
	}
	buckets := domain.CombatDamageBuckets{ResistanceBP: elemental.ResistBP, MitigationBP: mitigation, DamageTakenIncreasePct: p.Status.DamageTakenIncreasePct(magic), DamageReductionPct: p.Status.DamageReductionPct(magic), DamageReductionCapPct: p.Status.DamageReductionCapPct(), HalfDamageLayers: half, FlatReduction: p.Status.DamageReductionFlat(magic) + int64(s.petFlatReduction(p, magic))}
	amount = buckets.IncomingDamage(buckets.OutgoingDamage(amount))
	if magic && p.Status.MagicFullReflectChance() >= 100 {
		return 0
	}
	if status := p.Status.Clone(); status != nil {
		amount, _ = status.AbsorbDamage(amount)
	}
	if magic {
		amount = domain.ResolveElementAbsorb(p.HP, p.MaxHP, amount, elemental.AbsorbBP, 0, false).Damage
	}
	return amount
}

// Death invalidates damage/occupancy admission immediately. The victim may still
// see this exact already-warned blast on its dead body in the same connection.
// This receipt cannot arm a hazard or survive revival/reconnect/map generation.
func (s *Scene) retainFatalRiftImpact(h *endgameRiftHazard, p *entity.Entity) {
	if p == nil || p.Alive() || s.rift == nil {
		return
	}
	current := s.rift.hazardViewerGeneration[p.ID]
	for index := range s.rift.hazardImpacts {
		i := &s.rift.hazardImpacts[index]
		previous, seen := i.Viewers[p.ID]
		if i.View.ID != h.ID || !seen || previous != h.ViewerGeneration[p.ID] || current != previous+1 {
			continue
		}
		i.Viewers[p.ID] = current
		if i.FatalViewers == nil {
			i.FatalViewers = map[domain.EntityID]bool{}
		}
		i.FatalViewers[p.ID] = true
	}
}
