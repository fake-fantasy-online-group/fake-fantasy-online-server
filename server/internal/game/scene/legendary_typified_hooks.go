package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func typifiedActiveSkill(c *legendaryContext) bool {
	if c == nil || c.Def.LegendaryBasic || c.IsBasic {
		return false
	}
	if c.Skill == 0 && legendaryNativeBasic(c.Blow) {
		return false
	}
	return true
}

func typifiedPaidHighMana(c *legendaryContext, pct int32) bool {
	if c == nil || c.Owner == nil || c.Owner.MaxMP <= 0 || c.PaidMP <= 0 || pct <= 0 {
		return false
	}
	return int64(c.PaidMP)*100 >= int64(c.Owner.MaxMP)*int64(pct)
}

func typifiedCastReleased(c *legendaryContext) {
	if c == nil || !typifiedActiveSkill(c) || !c.Def.Kind.Active() {
		return
	}
	switch c.Power.ID {
	case "TY-U01":
		if c.PaidMP > 0 && typifiedOnce(c, "shred", 2000) {
			typifiedShredRemaining(c, c.Def, c.RollValue("shred_ms", 1500), 2500)
		}
	case "TY-U13":
		if typifiedOnce(c, "shield", 4000) {
			typifiedApplyShield(c, c.Owner, c.RollValue("shield_pct", 25), 5)
		}
	case "TY-U20":
		typifiedApplyTimedAffix(c, c.Owner, 6, domain.Affix{Attr: domain.AttrMoveSpeed, Value: c.RollValue("benefit", 50), Mode: domain.ModePercent})
	}
}

func typifiedAfterHit(c *legendaryContext) {
	if c == nil || c.Owner == nil || !typifiedActiveSkill(c) {
		return
	}
	hitEnemy := c.Target != nil && c.Owner.ID != c.Target.ID && !sameParty(c.Owner, c.Target)
	healed := false
	switch c.Power.ID {
	case "TY-U02":
		if (hitEnemy || healed) && typifiedOnce(c, "restore", 1000) {
			typifiedRestoreMP(c, c.Owner, typifiedPctOf(c.Owner.MaxMP, c.RollValue("restore_pct", 6)))
		}
	case "TY-U03":
		if hitEnemy && typifiedOnce(c, "refund", 0) && c.PaidMP > 0 {
			c.RefundMP(typifiedPctOf(c.PaidMP, c.RollValue("refund_pct", 40)))
		}
	case "TY-U06":
		if hitEnemy && typifiedOnce(c, "restore", 0) {
			// 2.5% authored as 250 / 100
			typifiedRestoreMP(c, c.Owner, domain.SaturateInt32(int64(c.Owner.MaxMP)*int64(c.RollValue("restore_pct", 250))/10000))
		}
	case "TY-U12":
		if hitEnemy && typifiedOnce(c, "shred", 4000) && universalNativeStatusKind(c, &c.Def, true) {
			typifiedShredRemaining(c, c.Def, c.RollValue("shred_ms", 3000), 0)
			typifiedApplyTimedAffix(c, c.Owner, 5)
		}
	case "TY-U15":
		if hitEnemy && c.Owner.Player != nil && typifiedOnce(c, "shred-"+fmt.Sprintf("%d", c.Def.ID), 3000) {
			remainingMS := domain.SaturateInt32(c.Owner.Player.RemainingSkillCooldown(c.Def.ID, c.Scene.tick).Millis())
			shred := domain.SaturateInt32(int64(remainingMS) * int64(c.RollValue("shred_bp", 2500)) / 10000)
			typifiedShredRemaining(c, c.Def, shred, 2500)
		}
	case "TY-U19":
		if hitEnemy && c.Target != nil && (entityInvisible(c.Target) || typifiedHasWindow(c.Target, "TY-U19")) {
			c.Scene.applyStatusDef(c.Target, domain.StatusDef{
				ID: typifiedStatusID("TY-U19"), Level: 1, Name: c.Power.Name, TypeKnown: true, Type: domain.StatusGood,
				DurationSec: 6, PreventInvisible: true, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
			}, c.Owner.ID)
			if c.Target.Status != nil {
				removed := c.Target.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.Invisible })
				c.Scene.statusesRemoved(c.Target, removed)
			}
		}
	}
}

func typifiedHealResolved(c *legendaryContext, applied, requested, overflow int32) {
	if c == nil || c.Preview || typifiedInResourcePath(c) || applied <= 0 {
		return
	}
	switch c.Power.ID {
	case "TY-U02":
		if typifiedOnce(c, "restore", 1000) {
			typifiedRestoreMP(c, c.Owner, typifiedPctOf(c.Owner.MaxMP, c.RollValue("restore_pct", 6)))
		}
	case "TY-U03":
		if typifiedOnce(c, "refund", 0) && c.PaidMP > 0 {
			c.RefundMP(typifiedPctOf(c.PaidMP, c.RollValue("refund_pct", 40)))
		}
	case "TY-U04":
		if c.Target != nil && typifiedOnce(c, "heal-"+fmt.Sprintf("%d", c.Target.ID), 0) {
			typifiedHeal(c, c.Target, typifiedPctOf(c.Target.MaxHP, c.RollValue("heal_pct", 5)))
		}
		if typifiedOnce(c, "restore", 0) {
			typifiedRestoreMP(c, c.Owner, typifiedPctOf(c.Owner.MaxMP, c.RollValue("restore_pct", 3)))
		}
	case "TY-U05":
		typifiedRefreshOwnedHots(c)
	case "TY-U06":
		if typifiedOnce(c, "restore", 0) {
			typifiedRestoreMP(c, c.Owner, domain.SaturateInt32(int64(c.Owner.MaxMP)*int64(c.RollValue("restore_pct", 250))/10000))
		}
	case "TY-U10":
		if c.Target != nil && c.Primary != 0 && c.Target.ID == c.Primary {
			typifiedHeal(c, c.Target, typifiedPctOf(applied, c.RollValue("primary_bp", 4000)/100))
		}
	case "TY-U15":
		if c.Owner.Player != nil && typifiedOnce(c, "shred-"+fmt.Sprintf("%d", c.Def.ID), 3000) {
			remainingMS := domain.SaturateInt32(c.Owner.Player.RemainingSkillCooldown(c.Def.ID, c.Scene.tick).Millis())
			shred := domain.SaturateInt32(int64(remainingMS) * int64(c.RollValue("shred_bp", 2500)) / 10000)
			typifiedShredRemaining(c, c.Def, shred, 2500)
		}
	case "TY-U14":
		if typifiedPaidHighMana(c, 8) && typifiedOnce(c, "heal-power", 0) && c.Target != nil {
			typifiedHeal(c, c.Target, typifiedPctOf(applied, c.RollValue("power_bp", 6000)/100))
		}
	case "TY-U16":
		if overflow > 0 && c.Target != nil {
			convert := typifiedPctOf(overflow, c.RollValue("convert_pct", 50))
			cap := typifiedPctOf(c.Target.MaxHP, 25)
			typifiedApplyShieldAmount(c, c.Target, min(convert, cap), 4)
		}
	}
}

func typifiedApplyShieldAmount(c *legendaryContext, target *entity.Entity, amount, durationSec int32) {
	if c == nil || target == nil || amount <= 0 {
		return
	}
	amount = typifiedOwnedShieldCap(target, c.SourceUID, amount)
	if amount <= 0 {
		return
	}
	typifiedCommitStatus(c.Scene, target, domain.StatusDef{
		ID: typifiedStatusID(c.Power.ID), Level: 1, Name: c.Power.Name, Desc: c.Power.Effect,
		Icon: legendaryPresentationIcons[c.Power.ID], IconKnown: legendaryPresentationIcons[c.Power.ID] != 0,
		TypeKnown: true, Type: domain.StatusGood, DurationSec: durationSec,
		EffectHitBP:  100000000,
		DamageShield: amount, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
	}, c.Owner.ID)
}

func typifiedRefreshOwnedHots(c *legendaryContext) {
	if c.Target == nil || c.Target.Status == nil || c.Owner == nil {
		return
	}
	ids := map[domain.StatusID]struct{}{}
	c.Target.Status.Each(func(a domain.Active) {
		if a.Source == c.Owner.ID {
			ids[a.Def.ID] = struct{}{}
		}
	})
	fn := func(current domain.Active) domain.Active {
		if current.Def.LegendaryPowerID == "" && current.Def.IntervalSec > 0 && universalStatusHeals(current.Def) && current.ExpireAt > c.Scene.tick {
			base := current.Def.LegendaryDurationMS
			if base <= 0 {
				base = domain.SaturateInt32(int64(current.Def.DurationSec) * 1000)
			}
			current.ExpireAt = c.Scene.tick + domain.Ticks(int(base))
		}
		return current
	}
	for id := range ids {
		c.Target.Status.TransformSourceStatus(id, c.Owner.ID, fn)
	}
}

func typifiedAfterIncoming(c *legendaryContext) {
	if c == nil || c.Owner == nil || !c.Owner.Alive() {
		return
	}
	switch c.Power.ID {
	case "TY-U20":
		typifiedApplyTimedAffix(c, c.Owner, 6, domain.Affix{Attr: domain.AttrMoveSpeed, Value: c.RollValue("benefit", 50), Mode: domain.ModePercent})
	case "TY-D16":
		if c.Owner.MaxHP > 0 && int64(c.Owner.HP)*100 <= int64(c.Owner.MaxHP)*60 && c.Ready(12000) {
			typifiedApplyShield(c, c.Owner, c.RollValue("shield_pct", 45), 6)
		}
	}
}

func typifiedTeleported(c *legendaryContext, _ domain.Pos) {
	if c == nil || c.Owner == nil {
		return
	}
	if c.Power.ID == "TY-U07" && c.Ready(3000) {
		typifiedApplyTimedAffix(c, c.Owner, 6)
	}
}

func typifiedResurrected(c *legendaryContext) {
	if c == nil || c.Target == nil {
		return
	}
	if c.Power.ID == "TY-U17" {
		floor := typifiedPctOf(c.Target.MaxHP, c.RollValue("life_pct", 100))
		if c.Target.HP < floor {
			c.Target.HP = floor
		}
		typifiedRestoreMP(c, c.Target, typifiedPctOf(c.Target.MaxMP, 50))
		c.Scene.applyStatusDef(c.Target, domain.StatusDef{
			ID: typifiedStatusID("TY-U17"), Level: c.RollValue("reduction_pct", 30), Name: c.Power.Name,
			TypeKnown: true, Type: domain.StatusGood, DurationSec: 8,
			LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
		}, c.Owner.ID)
	}
}

func typifiedDispelled(c *legendaryContext, removed []domain.Active) {
	if c == nil || c.Target == nil || len(removed) == 0 || c.Power.ID != "TY-U18" {
		return
	}
	if c.Ready(3000) {
		typifiedApplyShield(c, c.Target, c.RollValue("shield_pct", 15), 5)
	}
}

func typifiedTick(c *legendaryContext) {
	if c == nil || c.Preview || c.Owner == nil || !c.Owner.Alive() || c.Scene == nil {
		return
	}
	if c.Power.ID != "TY-U19" {
		return
	}
	radius := c.RollValue("aura", 400)
	c.Scene.detectTraps(c.Owner, radius, 10000)
	radiusSq := float64(radius) * float64(radius)
	for _, target := range c.Scene.entities {
		if target == nil || target.ID == c.Owner.ID || !target.Alive() || sameParty(c.Owner, target) {
			continue
		}
		if sqDist(c.Owner.Pos, target.Pos) > radiusSq {
			continue
		}
		if entityInvisible(target) {
			removed := target.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.Invisible })
			c.Scene.statusesRemoved(target, removed)
			typifiedCommitStatus(c.Scene, target, domain.StatusDef{
				ID: typifiedStatusID("TY-U19"), Level: 1, Name: c.Power.Name, TypeKnown: true, Type: domain.StatusGood,
				DurationSec: 2, PreventInvisible: true, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
			}, c.Owner.ID)
		}
	}
}

func typifiedIncomingHeal(c *legendaryContext, n int32) int32 {
	if typifiedInResourcePath(c) {
		return n
	}
	if c.Power.ID == "TY-D12" {
		n = universalScaleBP(n, c.RollValue("benefit", 8000))
		if n > 0 {
			typifiedMarkWindow(c, 4, 6000)
		}
	}
	return n
}

func typifiedModifyIncoming(c *legendaryContext, n int32) int32 {
	if n <= 0 || c.Owner == nil || c.Target == nil || c.Owner.ID == c.Target.ID || sameParty(c.Owner, c.Target) {
		return n
	}
	var bp int32
	switch c.Power.ID {
	case "TY-D05":
		if typifiedAreaDirect(c) {
			bp = c.RollValue("benefit", 6000)
		}
	case "TY-D06":
		if typifiedSingleDirect(c) {
			bp = c.RollValue("benefit", 5500)
		}
	case "TY-D07":
		if typifiedPhysicalDirect(c) {
			bp = c.RollValue("benefit", 5500)
		}
	case "TY-D08":
		if typifiedMagic(c) {
			bp = c.RollValue("benefit", 5500)
		}
	case "TY-D10":
		if c.Hit.Flag.Has(event.DamageCrit) {
			bp += c.RollValue("crit_bp", 5000)
		}
		if c.Owner.MaxHP > 0 && (int64(c.Hit.Amount) >= int64(typifiedPctOf(c.Owner.MaxHP, 15)) || int64(n) == 10000 && c.Hit.Amount == 0) {
			// When amount is known, apply the heavy-hit clause. 10000-scale calls without
			// Amount leave this to typifiedPoolReduction which copies the strike amount.
		}
		if c.Owner.MaxHP > 0 && int64(c.Hit.Amount) >= int64(typifiedPctOf(c.Owner.MaxHP, 15)) {
			bp += c.RollValue("heavy_bp", 3000)
		}
	case "TY-D12":
		if typifiedHasWindow(c.Owner, "TY-D12") {
			bp = c.RollValue("reduction_pct", 2500)
		}
	case "TY-D17":
		if universalNativeControlled(c.Owner) {
			bp = c.RollValue("benefit", 4500)
		}
	case "TY-D19":
		if c.Blow.Origin == domain.DamageDOTOrigin {
			bp = c.RollValue("benefit", 6500)
		}
	}
	if bp <= 0 {
		return n
	}
	return universalScaleBP(n, -bp)
}

func typifiedBeforeDamage(c *legendaryContext, n int32) int32 {
	if n <= 0 || c == nil || !typifiedActiveSkill(c) {
		return n
	}
	switch c.Power.ID {
	case "TY-U12":
		if typifiedHasWindow(c.Owner, "TY-U12") && c.Def.PrepareMS <= 0 {
			n = universalScaleBP(n, c.RollValue("power_bp", 2500))
		}
	case "TY-U14":
		if typifiedPaidHighMana(c, 8) {
			n = universalScaleBP(n, c.RollValue("power_bp", 6000))
		}
	case "TY-U19":
		if c.Target != nil && typifiedHasWindow(c.Target, "TY-U19") {
			n = universalScaleBP(n, c.RollValue("damage_bp", 2500))
		}
	}
	return n
}

func typifiedEquipmentChanged(c *legendaryContext) {
	if c == nil || c.Scene == nil || c.Owner == nil || c.Owner.Status == nil || c.Owner.Player == nil || c.Owner.Player.Char == nil {
		return
	}
	active := map[int64]struct{}{}
	for _, power := range domain.ActiveLegendaryPowers(c.Scene.functionalWorn(c.Owner), c.Scene.itemDef, c.Owner.Player.Char) {
		active[power.SourceUID] = struct{}{}
	}
	removed := c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		_, ok := active[d.LegendarySourceUID]
		if typifiedGeneratedShield(d) && !ok {
			return true
		}
		return d.LegendaryPowerID == "TY-D17" && d.ID == typifiedStatusID("TY-D17") && !ok
	})
	c.Scene.statusesRemoved(c.Owner, removed)
}

func typifiedOnShieldsResolved(s *Scene, owner *entity.Entity, before []domain.Active) {
	if s == nil || owner == nil || owner.Status == nil || len(before) == 0 {
		return
	}
	var broken int32
	for _, a := range before {
		if a.ShieldLeft <= 0 {
			continue
		}
		remaining := int32(0)
		found := false
		owner.Status.Each(func(cur domain.Active) {
			if cur.Def.ID == a.Def.ID && cur.Source == a.Source && cur.Def.LegendarySourceUID == a.Def.LegendarySourceUID {
				found = true
				remaining = cur.ShieldLeft
			}
		})
		if !found || remaining <= 0 {
			broken += a.ShieldLeft
		}
	}
	if broken <= 0 {
		return
	}
	for _, c := range s.legendaryContextsFor(owner, owner, combat.Blow{}, event.DamageDealt{Dst: owner.ID}, true) {
		if c.Power.ID != "TY-D18" || typifiedInResourcePath(&c) {
			continue
		}
		if !c.Ready(6000) {
			continue
		}
		typifiedHeal(&c, owner, typifiedPctOf(broken, c.RollValue("heal_pct", 30)))
	}
}
