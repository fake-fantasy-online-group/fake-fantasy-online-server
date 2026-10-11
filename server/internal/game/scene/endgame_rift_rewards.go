package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"sort"
)

type endgameRiftIdentity struct {
	Race     domain.Race
	Gender   uint8
	Employed bool
}

// Completion constructs exact instance identities once; retries never reroll.
func (s *Scene) generateEndgameRiftGrants() {
	d := &s.rift.data
	r := d.Run
	plan := domain.PlanRiftRewards(r.Tier, r.Timely)
	for char, m := range r.Members {
		if !m.Eligible {
			continue
		}
		grant := endgameRiftGrant{}
		var equipment []domain.ItemID
		best := int32(0)
		identity := d.Profiles[char]
		character := &domain.Character{Race: identity.Race, Employed: identity.Employed, EmploymentKnown: true}
		for id, def := range s.items {
			if def.Equip == nil || def.Equip.LevelReq > m.Level {
				continue
			}
			if unique, ok := domain.UniqueForBase(id); !ok || !unique.NativeDropEligible {
				continue
			}
			need := def.Equip.Need
			if !need.AllowsCharacter(character) {
				continue
			}
			sex := uint8(2)
			if identity.Gender == 1 {
				sex = 1
			}
			if need.Sex != 0 && need.Sex != sex {
				continue
			}
			if def.Equip.LevelReq > best {
				best = def.Equip.LevelReq
				equipment = nil
			}
			if def.Equip.LevelReq == best {
				equipment = append(equipment, id)
			}
		}
		sort.Slice(equipment, func(i, j int) bool { return equipment[i] < equipment[j] })
		for i := int32(0); i < plan.Equipment && len(equipment) > 0; i++ {
			def := s.items[equipment[s.rng.Intn(len(equipment))]]
			stack := domain.NewStack(def, 1)
			stack.Bound = true
			stack.SocketCount = domain.RollDroppedSocketCount(def, domain.AstralDropRealm, s.rng.Int63n)
			affixes := s.equipmentRoll.RollWithReservedSockets(def, domain.DefaultColorProfile, stack.SocketCount, s.rng.Int63n)
			stack.RolledAffixCount = uint8(len(affixes))
			copy(stack.RolledAffixes[:], affixes)
			if (r.Tier >= 10 && i == 0) || s.rng.Intn(100) < int(plan.UniqueChance) {
				domain.ApplyUnique(&stack, def, s.rng.Intn)
			}
			grant.Items = append(grant.Items, stack)
		}
		for i := int32(0); i < plan.Rune; i++ {
			id := domain.ItemID(1900028001 + s.rng.Intn(28))
			if def, ok := s.itemDef(id); ok {
				stack := domain.NewStack(def, 1)
				stack.Bound = true
				grant.Items = append(grant.Items, stack)
			}
		}
		if def, ok := s.itemDef(1900028202); ok {
			stack := domain.NewStack(def, plan.Dust)
			stack.Bound = true
			grant.Items = append(grant.Items, stack)
		}
		grant.Exp = s.trial.BaseExperience(m.Level, s.levels) / 2
		d.Grants[char] = grant
	}
	d.GrantsReady = true
}

func (s *Scene) publishEndgameRewardExperience(p *entity.Entity, previousLevel int32, exp int64) {
	if exp <= 0 {
		return
	}
	s.emitTo(p.ID, s.attributeSnapshot(p))
	s.emitTo(p.ID, event.ExpGained{Who: p.ID, Delta: exp, Total: p.Player.Char.Exp})
	if p.Player.Char.Level > previousLevel {
		s.emit(event.LevelUp{Who: p.ID, Level: p.Player.Char.Level})
		s.emitTo(p.ID, s.skillSnapshot(p))
		s.refreshChangeSetTooltips(p)
	}
}
