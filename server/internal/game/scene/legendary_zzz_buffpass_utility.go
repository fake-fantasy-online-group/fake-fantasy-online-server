package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Buff pass (2026-11-16) utility-row additions. Every added number here is a
// fixed mechanic of the row and is spelled out in the power description.
const (
	bpWarcryAllyRadius   = 250
	bpWarcryAllyMovePct  = 40
	bpWarcryAllyAtkPct   = 15
	bpWarcryAllySeconds  = 12
	bpWarcryShieldPct    = 15
	bpWarcryShieldSecs   = 12
	bpQuakePullStop      = 60
	bpTauntPullStop      = 60
	bpSoulRingAllyRadius = 200
	bpSoulRingAllyPct    = 30
	bpSmokeGuardPct      = 20
	bpSmokeGuardSeconds  = 8
	bpTrapSlowPct        = 40
	bpTrapSlowSeconds    = 3
	bpManaShareRadius    = 250
	bpManaShareBP        = 3000
)

func init() {
	// ZS-U02: lance of mana-break leaves +20% magic vulnerability for 10s.
	legendaryAugment("ZS-U02", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10009 && bpHostile(c, c.Target) {
			bpVulnerable(c, c.Target, 0, 20, 10)
		}
	}})
	// ZS-U04: war cry also hastes and empowers nearby party members.
	legendaryAugment("ZS-U04", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill != 10028 || !warriorFunctionalLearned(c, 10028) {
			return
		}
		for _, ally := range bpAllies(c, c.Owner.Pos, bpWarcryAllyRadius, false) {
			bpTimed(c, ally, "march-ally", bpWarcryAllySeconds, false, func(d *domain.StatusDef) {
				d.Affixes = []domain.Affix{
					{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: bpWarcryAllyMovePct},
					{Attr: domain.AttrAtk, Mode: domain.ModePercent, Value: bpWarcryAllyAtkPct},
					{Attr: domain.AttrMAtk, Mode: domain.ModePercent, Value: bpWarcryAllyAtkPct},
				}
			})
		}
	}})
	// ZS-U06: spear skills leave +10% physical vulnerability for 5s.
	legendaryAugment("ZS-U06", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if legendaryMeleeSpearActive(c.Def) && !c.Def.LegendaryBasic && bpHostile(c, c.Target) {
			bpVulnerable(c, c.Target, 10, 0, 5)
		}
	}})
	// ZS-U10: war cry shields the wearer and nearby party members.
	legendaryAugment("ZS-U10", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill != 10028 || !warriorFunctionalLearned(c, 10028) {
			return
		}
		for _, ally := range bpAllies(c, c.Owner.Pos, bpWarcryAllyRadius, true) {
			if ally.ID == c.Owner.ID {
				// The native war-cry cast cue already marks the wearer.
				bpShieldQuiet(c, ally, "warcry-shield", bpWarcryShieldPct, bpWarcryShieldSecs)
				continue
			}
			bpShield(c, ally, "warcry-shield", bpWarcryShieldPct, bpWarcryShieldSecs)
		}
	}})
	// ZS-U05: ground quake drags hit enemies to 60 in front of the wearer.
	legendaryAugment("ZS-U05", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10006 && bpHostile(c, c.Target) {
			bpPullToward(c, c.Target, c.Owner.Pos, bpQuakePullStop)
		}
	}})
	// JK-U06: bound enemies take +20% damage while the bind lasts.
	legendaryAugment("JK-U06", legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill == 10111 && bpHostile(c, c.Target) {
			d.PhysicalDamageTakenPct += 20
			d.MagicDamageTakenPct += 20
		}
	}})
	// JK-U10: sealed enemies take +25% magic damage for 12s.
	legendaryAugment("JK-U10", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10113 && bpHostile(c, c.Target) {
			bpVulnerable(c, c.Target, 0, 25, 12)
		}
	}})
	// JK-U03: party members within 200 regain 30% of the paid mana.
	legendaryAugment("JK-U03", legendaryHandler{RootFinished: func(c *legendaryContext) {
		if c.Preview || c.Skill != 10119 || c.PaidMP <= 0 || swordLandedCount(c.Owner.ID, c.Skill, c.Hits) == 0 {
			return
		}
		for _, ally := range bpAllies(c, c.Owner.Pos, bpSoulRingAllyRadius, false) {
			legendaryCasterRestoreMP(c, ally, specialScale(c.PaidMP, bpSoulRingAllyPct))
			c.Scene.emitAttributesIfPlayer(ally)
		}
	}})
	// JK-U04: taunt gathers enemies in its radius to the wearer.
	legendaryAugment("JK-U04", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill == 10127 && bpHostile(c, c.Target) {
			bpPullToward(c, c.Target, c.Owner.Pos, bpTauntPullStop)
		}
	}})
	// CK-U02: party members inside the smoke take 20% less damage for 8s.
	legendaryAugment("CK-U02", legendaryHandler{CastReleased: func(c *legendaryContext) {
		if c.Skill != 10224 {
			return
		}
		r := float64(max(c.Def.Radius, c.Def.Area.Radius))
		if r <= 0 {
			r = 130
		}
		for _, ally := range bpAllies(c, c.Center, r, true) {
			bpTimed(c, ally, "smoke-guard", bpSmokeGuardSeconds, false, func(d *domain.StatusDef) {
				d.PhysicalDamageReductionPct = bpSmokeGuardPct
				d.MagicDamageReductionPct = bpSmokeGuardPct
			})
		}
	}})
	// CK-U07: the five traps slow hit enemies by 40% for 3s.
	legendaryAugment("CK-U07", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if legendaryAssassinTrapSkill(c.Skill) && bpHostile(c, c.Target) {
			bpTimed(c, c.Target, "trap-slow", bpTrapSlowSeconds, true, func(d *domain.StatusDef) {
				d.Affixes = []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -bpTrapSlowPct}}
			})
		}
	}})
	// CK-U05: continuous and multiple throws recover 40% faster.
	legendaryAugment("CK-U05", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[10222] > 0 && (d.ID == 10205 || d.ID == 10206) {
			d.LegendaryModifiers.CooldownReductionBP += 4000
		}
	}})
	// SS-U02: enemies stripped by the dispel take +15% damage for 8s.
	legendaryAugment("SS-U02", legendaryHandler{Dispelled: func(c *legendaryContext, removed []domain.Active) {
		if c.Skill == 10433 && len(removed) > 0 && bpHostile(c, c.Target) {
			bpVulnerable(c, c.Target, 15, 15, 8)
		}
	}})
}

// bpHostile reports a living enemy of the wearer.
func bpHostile(c *legendaryContext, t *entity.Entity) bool {
	return c != nil && t != nil && t.Alive() && c.Owner != nil && t.ID != c.Owner.ID && !sameParty(c.Owner, t)
}

// CK-U03 (replacement): seal and frost traps grow 50% and leave +15%
// physical vulnerability for 8s on every enemy they catch.
func bpTrapBreakerHandler() legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10215 || d.ID == 10216 {
			d.LegendaryModifiers.TrapRadiusBP += c.RollValue("radius_bp", 5000)
		}
	}, SkillResolved: func(c *legendaryContext) {
		if (c.Skill == 10215 || c.Skill == 10216) && bpHostile(c, c.Target) {
			bpVulnerable(c, c.Target, 15, 0, 8)
		}
	}}
}

// CK-U04 (replacement): shadow jump cooldown -50%, 3s of +60% speed after the
// jump, and the next backstab is a guaranteed critical hit.
func bpShadowStepHandler() legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10227 {
			d.LegendaryModifiers.CooldownReductionBP += 5000
		}
		if d.ID == 10209 && c.State(0, "shadow-crit").Stacks > 0 {
			d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
			for i := range d.Effects {
				if d.Effects[i].Kind == domain.EffectDamage {
					d.Effects[i].GuaranteedCrit = true
				}
			}
			if len(d.Effects) > 0 {
				d.Effect = d.Effects[0]
			}
		}
	}, CastReleased: func(c *legendaryContext) {
		switch c.Skill {
		case 10227:
			if !legendaryAssassinLearned(c, 10227) {
				return
			}
			bpTimed(c, c.Owner, "shadow-step", 3, false, func(d *domain.StatusDef) {
				d.Affixes = []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: c.RollValue("move_pct", 60)}}
			})
			c.State(0, "shadow-crit").Stacks = 1
		case 10209:
			c.State(0, "shadow-crit").Stacks = 0
		}
	}}
}

// SS-U05 (replacement): enemies carrying your burn take +12% damage from all
// sources while the burn lasts.
func bpEmberMarkHandler() legendaryHandler {
	return legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if d.ID == 1036 && bpHostile(c, c.Target) {
			d.PhysicalDamageTakenPct += c.RollValue("vulnerable_pct", 12)
			d.MagicDamageTakenPct += c.RollValue("vulnerable_pct", 12)
		}
	}}
}

// bpComposeShared applies auras that come from another party member's
// equipment to the caster's prepared skill. SS-U07: while a party member
// within 250 has an active mana reserve window, every skill costs 30% less.
func (s *Scene) bpComposeShared(owner *entity.Entity, def *domain.SkillDef, preview bool) {
	if preview || owner == nil || def == nil || owner.Player == nil {
		return
	}
	for _, m := range s.partyMembersInScene(owner) {
		if m == nil || m.ID == owner.ID || !m.Alive() || m.Pos.MapID != owner.Pos.MapID || sqDist(m.Pos, owner.Pos) > bpManaShareRadius*bpManaShareRadius {
			continue
		}
		for _, c := range s.legendaryContextsFor(m, nil, combat.Blow{}, event.DamageDealt{}, true) {
			if c.Power.ID == "SS-U07" && legendarySorcererWindow(&c, "mana-reserve", 0) {
				def.LegendaryModifiers.ManaReductionBP += bpManaShareBP
				return
			}
		}
	}
}
