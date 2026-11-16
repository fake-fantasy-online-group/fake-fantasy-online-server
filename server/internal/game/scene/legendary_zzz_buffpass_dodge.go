package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Legendary dodge. The defender's equipped powers contribute independent
// chances for one incoming direct hit (physical, magic, critical and
// guaranteed hits alike). combat.CalculateStrike rolls the combined chance
// once, after the critical roll; a success is a Miss. Periodic damage is never
// dodged. Independent sources combine as 1-Π(1-p) and the total is capped.
const legendaryDodgeCapBP = 7500

// legendaryDodgeProviders: c.Owner is the wearer (defender), c.Target the attacker.
var legendaryDodgeProviders = map[string]func(c *legendaryContext) int32{}

// legendaryDodgeReactions run after a hit was dodged by the wearer.
var legendaryDodgeReactions = map[string]func(c *legendaryContext){}

func (s *Scene) legendaryDodgeChanceBP(src, dst *entity.Entity, blow combat.Blow) int32 {
	if src == nil || dst == nil || dst.Player == nil || src.ID == dst.ID || sameParty(src, dst) || blow.Origin == domain.DamageDOTOrigin || blow.Origin == domain.DamageAuraOrigin {
		// Periodic damage and aura/environment hazards are not dodgeable attacks.
		return 0
	}
	remain := int64(10000)
	for _, c := range s.legendaryIncomingContexts(src, dst, blow, event.DamageDealt{Src: src.ID, Dst: dst.ID, SkillID: blow.SkillID, Origin: blow.Origin}) {
		fn := legendaryDodgeProviders[c.Power.ID]
		if fn == nil {
			continue
		}
		bp := min(int64(10000), max(int64(0), int64(fn(&c))))
		remain = remain * (10000 - bp) / 10000
	}
	return int32(min(int64(legendaryDodgeCapBP), 10000-remain))
}

func (s *Scene) legendaryDodged(src, dst *entity.Entity, blow combat.Blow, ev event.DamageDealt) {
	for _, c := range s.legendaryIncomingContexts(src, dst, blow, ev) {
		if fn := legendaryDodgeReactions[c.Power.ID]; fn != nil {
			c.invoke(fn)
		}
	}
}

// Party guards let a wearer reduce damage dealt to its party members
// (victim != wearer). c.Owner is the wearer, c.Target the attacker.
var legendaryPartyGuards = map[string]func(c *legendaryContext, victim *entity.Entity, amount int32) int32{}

func (s *Scene) legendaryPartyGuardIncoming(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt, amount int32) int32 {
	if amount <= 0 || src == nil || dst == nil || dst.Player == nil || dst.Player.Party == 0 || sameParty(src, dst) || len(legendaryPartyGuards) == 0 {
		return amount
	}
	for _, m := range s.partyMembersInScene(dst) {
		if m == nil || m.ID == dst.ID || !m.Alive() {
			continue
		}
		for _, c := range s.legendaryContextsFor(m, src, blow, hit, true) {
			if fn := legendaryPartyGuards[c.Power.ID]; fn != nil {
				amount = max(int32(0), fn(&c, dst, amount))
			}
		}
	}
	return amount
}
