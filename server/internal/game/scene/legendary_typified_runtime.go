package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const typifiedReductionCapBP int32 = 7000

func typifiedReductionPower(id string) bool {
	switch id {
	case "TY-D05", "TY-D06", "TY-D07", "TY-D08", "TY-D10", "TY-D12", "TY-D17", "TY-D19", "TY-U17":
		return true
	}
	return false
}

func typifiedStatusID(powerID string) domain.StatusID {
	switch powerID {
	case "TY-U07":
		return 198007
	case "TY-U08":
		return 198008
	case "TY-U12":
		return 198012
	case "TY-U13":
		return 198013
	case "TY-U16":
		return 198016
	case "TY-U17":
		return 198017
	case "TY-U18":
		return 198018
	case "TY-U19":
		return 198019
	case "TY-U20":
		return 198020
	case "TY-D09":
		return 198109
	case "TY-D12":
		return 198112
	case "TY-D14":
		return 198114
	case "TY-D15":
		return 198115
	case "TY-D16":
		return 198116
	case "TY-D17":
		return 198117
	case "TY-D20":
		return 198120
	default:
		return 198000
	}
}

func typifiedResourcePath(id string) []string {
	return []string{"typified-resource", id}
}

func typifiedInResourcePath(c *legendaryContext) bool {
	for _, p := range c.CausalPath {
		if p == "typified-resource" {
			return true
		}
	}
	return false
}

func typifiedPctOf(n int32, pct int32) int32 {
	if n <= 0 || pct <= 0 {
		return 0
	}
	return domain.SaturateInt32(int64(n) * int64(pct) / 100)
}

func typifiedScaleStat(current int32, pct int32) int32 {
	if current <= 0 || pct <= 0 {
		return current
	}
	return domain.SaturateInt32(int64(current) + int64(current)*int64(pct)/100)
}

func typifiedRestoreMP(c *legendaryContext, target *entity.Entity, amount int32) int32 {
	if c == nil || c.Preview || target == nil || !target.Alive() || amount <= 0 || typifiedInResourcePath(c) {
		return 0
	}
	defer c.Scene.withCombatCausalPath(append(append([]string{}, c.CausalPath...), typifiedResourcePath(c.Power.ID)...))()
	before := target.MP
	target.MP = min(target.MaxMP, domain.SaturateInt32(int64(target.MP)+int64(amount)))
	gained := target.MP - before
	if gained > 0 {
		c.Scene.emitAttributesIfPlayer(target)
	}
	return gained
}

func typifiedHeal(c *legendaryContext, target *entity.Entity, amount int32) int32 {
	if c == nil || c.Preview || target == nil || !target.Alive() || amount <= 0 || typifiedInResourcePath(c) {
		return 0
	}
	defer c.Scene.withCombatCausalPath(append(append([]string{}, c.CausalPath...), typifiedResourcePath(c.Power.ID)...))()
	before := target.HP
	target.HP = min(target.MaxHP, domain.SaturateInt32(int64(target.HP)+int64(amount)))
	gained := target.HP - before
	if gained > 0 {
		c.Scene.emitAttributesIfPlayer(target)
	}
	return gained
}

func typifiedOnce(c *legendaryContext, phase string, icdMS int64) bool {
	if c == nil || c.Preview || c.RootID == 0 {
		return false
	}
	st := c.State(0, phase)
	if st.RootID == c.RootID {
		return false
	}
	if icdMS > 0 && !c.Ready(icdMS) {
		return false
	}
	st.RootID = c.RootID
	return true
}

func typifiedOwnedShieldCap(owner *entity.Entity, sourceUID int64, add int32) int32 {
	if owner == nil || owner.MaxHP <= 0 || add <= 0 {
		return 0
	}
	per := typifiedPctOf(owner.MaxHP, 50)
	total := typifiedPctOf(owner.MaxHP, 100)
	var same, typified int32
	if owner.Status != nil {
		owner.Status.EachEffective(func(a domain.Active) {
			if a.ShieldLeft <= 0 {
				return
			}
			if typifiedGeneratedShield(a.Def) {
				typified += a.ShieldLeft
				if a.Def.LegendarySourceUID == sourceUID {
					same += a.ShieldLeft
				}
			}
		})
	}
	roomSame := max(int32(0), per-same)
	roomTotal := max(int32(0), total-typified)
	return min(add, roomSame, roomTotal)
}

func typifiedGeneratedShield(d domain.StatusDef) bool {
	if d.LegendaryPowerID == "" || d.DamageShield <= 0 {
		return false
	}
	switch d.LegendaryPowerID {
	case "TY-U13", "TY-U16", "TY-U18", "TY-D09", "TY-D16":
		return true
	}
	return d.ID >= 198013 && d.ID <= 198116
}

func typifiedCommitStatus(s *Scene, target *entity.Entity, def domain.StatusDef, src domain.EntityID) {
	if s == nil || target == nil || !target.Alive() {
		return
	}
	if target.Status == nil {
		target.Status = domain.NewStatusSet()
	}
	// Typified generated IDs are not in the native overlay matrix. Persist them
	// through the domain Apply path used by the existing typified shield-clear tests.
	target.Status.Apply(def, s.tick, src)
	// Do not call refreshEntityStats here: that path runs legendaryEquipmentChanged
	// and typified EquipmentChanged hooks, which clear typified generated shields.
	s.emitAttributesIfPlayer(target)
}

func typifiedApplyShield(c *legendaryContext, target *entity.Entity, pct int32, durationSec int32) {
	if c == nil || c.Preview || target == nil || !target.Alive() || pct <= 0 || durationSec <= 0 {
		return
	}
	amount := typifiedOwnedShieldCap(target, c.SourceUID, typifiedPctOf(target.MaxHP, pct))
	if amount <= 0 {
		return
	}
	// Refresh same source: keep the higher remaining absorb and refresh duration.
	if target.Status != nil {
		id := typifiedStatusID(c.Power.ID)
		target.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == id && a.Def.LegendarySourceUID == c.SourceUID && a.ShieldLeft > amount {
				amount = a.ShieldLeft
			}
		})
	}
	typifiedCommitStatus(c.Scene, target, domain.StatusDef{
		ID: typifiedStatusID(c.Power.ID), Level: 1, Name: c.Power.Name, Desc: c.Power.Effect,
		Icon: legendaryPresentationIcons[c.Power.ID], IconKnown: legendaryPresentationIcons[c.Power.ID] != 0,
		TypeKnown: true, Type: domain.StatusGood, DurationSec: durationSec,
		EffectHitBP:  100000000,
		DamageShield: amount, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
	}, c.Owner.ID)
}

func typifiedApplyTimedAffix(c *legendaryContext, target *entity.Entity, durationSec int32, affixes ...domain.Affix) {
	if c == nil || c.Preview || target == nil || !target.Alive() || durationSec <= 0 {
		return
	}
	typifiedCommitStatus(c.Scene, target, domain.StatusDef{
		ID: typifiedStatusID(c.Power.ID), Level: 1, Name: c.Power.Name, Desc: c.Power.Effect,
		Icon: legendaryPresentationIcons[c.Power.ID], IconKnown: legendaryPresentationIcons[c.Power.ID] != 0,
		TypeKnown: true, Type: domain.StatusGood, DurationSec: durationSec,
		EffectHitBP: 100000000,
		Affixes:     affixes, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID,
	}, c.Owner.ID)
}

func typifiedShredRemaining(c *legendaryContext, def domain.SkillDef, shredMS int32, floorBP int32) {
	if c == nil || c.Preview || c.Owner == nil || c.Owner.Player == nil || shredMS <= 0 || def.ID == 0 {
		return
	}
	floorMS := int32(1000)
	if def.CooldownMS > 0 && floorBP > 0 {
		floorMS = max(int32(1000), domain.SaturateInt32(int64(def.CooldownMS)*int64(floorBP)/10000))
	}
	c.Owner.Player.ReduceRemainingSkillCooldownWithFloor(def.ID, c.Scene.tick, domain.Ticks(int(shredMS)), domain.Ticks(int(floorMS)))
}

func typifiedClearSourceShields(c *legendaryContext, owner *entity.Entity) {
	if owner == nil || owner.Status == nil || c == nil {
		return
	}
	removed := owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		return typifiedGeneratedShield(d) && d.LegendarySourceUID == c.SourceUID
	})
	c.Scene.statusesRemoved(owner, removed)
}

func typifiedHasWindow(owner *entity.Entity, powerID string) bool {
	found := false
	if owner != nil && owner.Status != nil {
		owner.Status.EachEffective(func(a domain.Active) {
			if a.Def.LegendaryPowerID == powerID && a.Def.ID == typifiedStatusID(powerID) {
				found = true
			}
		})
	}
	return found
}

func typifiedMarkWindow(c *legendaryContext, durationSec int32, icdMS int64) {
	if c == nil || c.Owner == nil || !c.Ready(icdMS) {
		return
	}
	typifiedApplyTimedAffix(c, c.Owner, durationSec)
}

func typifiedControlLike(d *domain.StatusDef) bool {
	return d != nil && (d.Control != domain.ControlNone || d.BlockMove)
}

func typifiedMovementImpairment(d *domain.StatusDef) bool {
	if d == nil {
		return false
	}
	hard := d.Control == domain.ControlStun || d.ID == domain.StatusStun || d.ID == domain.StatusFreeze || d.ID == domain.StatusPetrify
	if d.BlockMove && !hard {
		return true
	}
	if d.Immobile != 0 && !hard {
		return true
	}
	for _, a := range d.Affixes {
		if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
			return true
		}
	}
	return false
}

func typifiedMoveLimit(d *domain.StatusDef) bool {
	if d == nil {
		return false
	}
	if d.BlockMove || d.Control == domain.ControlStun || d.Immobile != 0 {
		return true
	}
	for _, a := range d.Affixes {
		if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
			return true
		}
	}
	return false
}

func typifiedPoisonOrBurn(d *domain.StatusDef) bool {
	return d != nil && (d.ID == domain.StatusPoison || d.ID == 1036)
}

func typifiedDefenseLoss(d *domain.StatusDef) bool {
	if d == nil {
		return false
	}
	for _, a := range d.Affixes {
		if a.Value < 0 && (a.Attr == domain.AttrDef || a.Attr == domain.AttrMDef) {
			return true
		}
	}
	return false
}

func typifiedAttackSlow(d *domain.StatusDef) bool {
	if d == nil {
		return false
	}
	for _, a := range d.Affixes {
		if a.Attr == domain.AttrAtkSpeed && a.Value > 0 {
			return true
		}
	}
	return false
}

func typifiedImmuneBlock(d *domain.StatusDef, kind string) bool {
	if d == nil {
		return false
	}
	switch kind {
	case "move":
		return typifiedMoveLimit(d)
	case "defense":
		return typifiedDefenseLoss(d)
	case "haste":
		return typifiedAttackSlow(d)
	case "dot":
		return typifiedPoisonOrBurn(d)
	}
	return false
}

func typifiedSingleDirect(c *legendaryContext) bool {
	return c.Blow.Origin != domain.DamageDOTOrigin && !c.Blow.Area
}

func typifiedAreaDirect(c *legendaryContext) bool {
	return c.Blow.Origin != domain.DamageDOTOrigin && c.Blow.Area
}

func typifiedPhysicalDirect(c *legendaryContext) bool {
	return c.Blow.Origin != domain.DamageDOTOrigin && !c.Blow.Magic && !c.Hit.Flag.Has(event.DamageMagic)
}

func typifiedMagic(c *legendaryContext) bool {
	return c.Blow.Magic || c.Hit.Flag.Has(event.DamageMagic)
}

func typifiedReplaceHandler(id string, h legendaryHandler) {
	h.revision = "synergy-v1"
	legendaryHandlers[id] = bindLegendaryEquipmentHandlers(id, h)
}

func (s *Scene) typifiedPoolReduction(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt) int32 {
	if src == nil || dst == nil || src.ID == dst.ID || sameParty(src, dst) {
		return 0
	}
	var bp int64
	for _, c := range s.legendaryIncomingContexts(src, dst, blow, hit) {
		if !typifiedReductionPower(c.Power.ID) {
			continue
		}
		if c.Power.ID == "TY-D07" && blow.Origin == domain.DamageDOTOrigin {
			continue
		}
		c.Blow = blow
		c.Hit = hit
		if h := legendaryHandlers[c.Power.ID].ModifyIncoming; h != nil {
			bp += 10000 - int64(h(&c, 10000))
		}
	}
	if dst.Status != nil {
		dst.Status.EachEffective(func(a domain.Active) {
			if a.Def.LegendaryPowerID == "TY-U17" && a.Def.ID == typifiedStatusID("TY-U17") && a.Def.Level > 0 {
				bp += int64(a.Def.Level) * 100
			}
		})
	}
	return int32(min(int64(typifiedReductionCapBP), max(int64(0), bp)))
}

func typifiedClearHeldGeneratedShields(s *Scene, owner *entity.Entity) {
	if s == nil || owner == nil || owner.Status == nil {
		return
	}
	removed := owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		return typifiedGeneratedShield(d)
	})
	s.statusesRemoved(owner, removed)
}

func typifiedAfterStatusMutation(s *Scene, owner *entity.Entity) {
	if s == nil || owner == nil || !owner.Alive() {
		return
	}
	for _, c := range s.legendaryContextsFor(owner, owner, combat.Blow{}, event.DamageDealt{}, true) {
		if c.Power.ID != "TY-D17" || c.Preview {
			continue
		}
		typifiedD17AfterStatusChange(&c)
	}
}

func typifiedD17AfterStatusChange(c *legendaryContext) {
	if c == nil || c.Owner == nil {
		return
	}
	st := c.State(0, "control-armed")
	if universalNativeControlled(c.Owner) {
		st.Stacks = 1
		return
	}
	if st.Stacks == 0 {
		return
	}
	// Consume the armed bit before stripping slows so nested statusesRemoved cannot retrigger.
	st.Stacks = 0
	typifiedD17ControlEnded(c)
}

func typifiedD17ControlEnded(c *legendaryContext) {
	if c == nil || c.Preview || c.Owner == nil || !c.Owner.Alive() || c.Scene == nil {
		return
	}
	if c.Owner.Status != nil {
		changed, removed := c.Owner.Status.StripMovementImpairments()
		if len(removed) > 0 {
			c.Scene.statusesRemoved(c.Owner, removed)
		} else if changed {
			c.Scene.refreshEntityStats(c.Owner)
			c.Scene.emitAttributesIfPlayer(c.Owner)
			c.Scene.pushStatusSnapshots(c.Owner)
		}
	}
	typifiedApplyTimedAffix(c, c.Owner, 2, domain.Affix{Attr: domain.AttrMoveSpeed, Value: c.RollValue("move_pct", 40), Mode: domain.ModePercent})
	c.Scene.refreshEntityStats(c.Owner)
	c.Scene.emitAttributesIfPlayer(c.Owner)
	c.Scene.pushStatusSnapshots(c.Owner)
}

func typifiedStatusPostCommit(s *Scene, owner, source *entity.Entity, def *domain.StatusDef) {
	if s == nil || owner == nil || def == nil || !typifiedControlLike(def) {
		return
	}
	for _, c := range s.legendaryContextsFor(owner, source, combat.Blow{}, event.DamageDealt{}, true) {
		if c.Power.ID != "TY-D09" || c.Preview {
			continue
		}
		if !c.Ready(8000) {
			continue
		}
		typifiedApplyShield(&c, c.Owner, c.RollValue("shield_pct", 35), 4)
	}
}
