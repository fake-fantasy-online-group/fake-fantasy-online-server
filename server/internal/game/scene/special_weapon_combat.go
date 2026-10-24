package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type specialWeaponState struct {
	owner          *entity.Entity
	base           domain.ItemID
	weaponRevision uint64
	cryUntil       domain.Tick
	cryDamage      int64
	breakUntil     domain.Tick
}
type specialWeaponRuntime struct {
	players map[domain.EntityID]*specialWeaponState
	smoke   map[domain.EntityID]*specialWeaponSmoke
}

func (s *Scene) specialWeaponState(p *entity.Entity) *specialWeaponState {
	if p == nil || p.Player == nil {
		return nil
	}
	base := s.specialWeapon(p)
	if s.specialWeapons == nil {
		s.specialWeapons = &specialWeaponRuntime{players: make(map[domain.EntityID]*specialWeaponState)}
	}
	st := s.specialWeapons.players[p.ID]
	if st == nil || st.owner != p || st.base != base || st.weaponRevision != p.Player.Worn.WeaponEffectsRevision() {
		st = &specialWeaponState{owner: p, base: base, weaponRevision: p.Player.Worn.WeaponEffectsRevision()}
		s.specialWeapons.players[p.ID] = st
	}
	return st
}
func (s *Scene) specialWeaponShield(target *entity.Entity, base domain.ItemID, amount, seconds int32, source domain.EntityID) {
	if amount <= 0 {
		return
	}
	id := domain.StatusID(1800100000 + base)
	if target.Status != nil {
		s.statusesRemoved(target, target.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == id }))
	}
	s.applyStatusDef(target, domain.StatusDef{ID: id, Level: 1, Name: "武器护盾", TypeKnown: true, Type: domain.StatusGood, DurationSec: seconds, DamageShield: amount}, source)
}
func (s *Scene) specialWeaponLostHP(p *entity.Entity, amount int32) {
	if amount <= 0 || p == nil || p.Player == nil || s.specialWeapons == nil {
		return
	}
	st := s.specialWeapons.players[p.ID]
	if st != nil && st.owner == p && st.weaponRevision == p.Player.Worn.WeaponEffectsRevision() && st.base == 1624 && st.cryUntil > s.tick && s.specialWeapon(p) == 1624 {
		st.cryDamage += int64(amount)
	}
}
func (s *Scene) specialWeaponConsumeCry(p *entity.Entity) {
	if s.specialWeapons == nil || p == nil {
		return
	}
	st := s.specialWeapons.players[p.ID]
	if st != nil && st.owner == p && st.weaponRevision == p.Player.Worn.WeaponEffectsRevision() && st.base == 1624 && s.specialWeapon(p) == 1624 && st.cryDamage > 0 {
		amount := min(domain.SaturateInt32(st.cryDamage/5), p.MaxHP/10)
		st.cryDamage = 0
		st.cryUntil = 0
		s.specialWeaponShield(p, 1624, amount, 5, p.ID)
	}
}
func (s *Scene) specialWeaponPrepareCast(p *entity.Entity, d *domain.SkillDef) {
	if p == nil {
		return
	}
	if d.HasDamageEffect() && d.RequiredWeaponType == 1 {
		s.specialWeaponConsumeCry(p)
	}
	if d.SpecialWeaponBaseID == 1626 && d.ID == 10009 && s.specialWeapons != nil {
		st := s.specialWeapons.players[p.ID]
		if st != nil && st.owner == p && st.weaponRevision == p.Player.Worn.WeaponEffectsRevision() && st.breakUntil > s.tick {
			specialDamage(d, 140)
			st.breakUntil = 0
		}
	}
	if p.Status != nil && len(d.Statuses) > 0 {
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.NextStatusDurationPct > 0 {
				d.SpecialWeaponStatusPowerPct = max(d.SpecialWeaponStatusPowerPct, a.Def.SpecialWeaponBuffBonusPct)
			}
		})
	}
}
func (s *Scene) specialWeaponCastReleased(p *entity.Entity, d domain.SkillDef) {
	if d.SpecialWeaponBaseID == 1624 && d.ID == 10028 {
		st := s.specialWeaponState(p)
		st.cryUntil = s.tick + domain.Ticks(3000)
		st.cryDamage = 0
	}
	if d.SpecialWeaponBaseID == 1825 && d.ID == 10224 {
		s.applyStatusDef(p, domain.StatusDef{ID: domain.StatusStealth, Level: 1, Name: "烟幕隐身", TypeKnown: true, Type: domain.StatusGood, DurationSec: 2, Invisible: true, BreakOnAttack: true}, p.ID)
	}
}
func (s *Scene) specialWeaponCastFinished(p *entity.Entity, d domain.SkillDef, hits []domain.EntityID) {
	if d.SpecialWeaponBaseID == 1825 && d.ID == 10224 {
		s.specialWeaponState(p)
		if s.specialWeapons.smoke == nil {
			s.specialWeapons.smoke = make(map[domain.EntityID]*specialWeaponSmoke)
		}
		s.specialWeapons.smoke[p.ID] = &specialWeaponSmoke{owner: p, at: p.Pos, def: d, expires: s.tick + domain.Ticks(6000), next: s.tick + domain.Ticks(1000), targets: append([]domain.EntityID(nil), hits...)}
	}
	if d.SpecialWeaponBaseID == 1526 && d.ID == 10127 && len(hits) >= 3 {
		s.applyStatusDef(p, domain.StatusDef{ID: 1800101526, Name: "挑衅疾行", Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 5, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: 20}}}, p.ID)
	}
}
func (s *Scene) specialWeaponBeforeHit(src, dst *entity.Entity, d *domain.SkillDef) {
	if d.SpecialWeaponBaseID == 1028 && int64(dst.HP)*2 < int64(dst.MaxHP) {
		specialDamage(d, 145)
	}
	if d.SpecialWeaponBaseID == 1527 && int64(dst.HP)*100 < int64(dst.MaxHP)*30 {
		specialDamage(d, 180)
	}
}
func (s *Scene) specialWeaponAfterHit(src, dst *entity.Entity, d domain.SkillDef, landed, killed bool) {
	if !landed {
		return
	}
	switch d.SpecialWeaponBaseID {
	case 1626:
		if d.ID == 10027 {
			s.specialWeaponState(src).breakUntil = s.tick + domain.Ticks(5000)
		}
	case 1824:
		if d.ID == 10209 && killed && src.Player != nil {
			src.Player.ScaleRemainingSkillCooldown(10208, s.tick, 50)
		}
	case 1027:
		s.specialWeaponDispelOne(src, dst)
	case 1525:
		s.applyStatusDef(dst, domain.StatusDef{ID: 1800101525, Name: "混沌破甲", Level: 1, DurationSec: 5, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -15}}}, src.ID)
	case 1925:
		s.specialWeaponSlow(src, dst, 30, 3, 1800101925)
	case 1928:
		s.specialWeaponSlow(src, dst, 70, 6, 1800101928)
	case 1320:
		if d.Prof == 5 {
			s.specialWeaponSlow(src, dst, 20, 2, 1800101320)
		}
	case 1322:
		if d.ID == 10401 {
			s.specialWeaponBurn(src, dst, 60, true)
		}
	case 1325:
		if d.ID == 10405 {
			s.specialWeaponBurn(src, dst, 100, false)
		}
	}
}
func (s *Scene) specialWeaponSlow(src, dst *entity.Entity, pct, seconds int32, id domain.StatusID) {
	if dst == nil || !dst.Alive() {
		return
	}
	if dst.Status != nil {
		stronger := false
		dst.Status.EachEffective(func(a domain.Active) {
			{
				for _, af := range a.Def.Affixes {
					if af.Attr == domain.AttrMoveSpeed && af.Mode == domain.ModePercent && af.Value < 0 && (af.Value <= -pct || a.ExpireAt >= s.tick+domain.Ticks(int(seconds)*1000)) {
						stronger = true
					}
				}
			}
		})
		if stronger {
			return
		}
	}
	s.applyStatusDef(dst, domain.StatusDef{ID: domain.StatusSlow, Name: "武器减速", Level: 1, DurationSec: seconds, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -pct}}}, src.ID)
}
func (s *Scene) specialWeaponBurn(src, dst *entity.Entity, pct int32, consume bool) {
	if dst.Status == nil || !dst.Alive() {
		return
	}
	var remaining int64
	dst.Status.EachEffective(func(a domain.Active) {
		if a.Source == src.ID && a.Def.ID == 1036 {
			remaining += classSetPoisonRemaining(a, s.tick, dst.MaxHP)
		}
	})
	if remaining <= 0 {
		return
	}
	if consume {
		s.specialWeaponRemoveStatuses(dst, src.ID, func(a domain.Active) bool { return a.Def.ID == 1036 })
	}
	// Same periodic-damage mitigation/death path, never a new skill cast or DOT.
	s.tickStatus(dst, domain.Active{Source: src.ID, Def: domain.StatusDef{ID: 1036, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -domain.SaturateInt32(remaining * int64(pct) / 100), Mode: domain.ModeAbsolute}}}})
}
func specialDefensiveStatus(id domain.StatusID) bool {
	return id == 1010 || id == 1011 || id == 1021 || id == 1037 || id == 1038
}
func (s *Scene) specialWeaponDispelOne(src, dst *entity.Entity) {
	if dst.Status == nil {
		return
	}
	var selected domain.StatusID
	dst.Status.EachEffective(func(a domain.Active) {
		if specialDefensiveStatus(a.Def.ID) && (selected == 0 || a.Def.ID < selected) {
			selected = a.Def.ID
		}
	})
	if selected != 0 {
		s.statusesRemoved(dst, dst.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == selected }))
	}
}
func (s *Scene) specialWeaponDispel(src, dst *entity.Entity, d domain.SkillDef, ids []domain.StatusID) []domain.Active {
	if d.SpecialWeaponBaseID != 1324 || d.ID != 10433 || dst.Status == nil {
		return nil
	}
	var stolen []domain.Active
	dst.Status.EachEffective(func(a domain.Active) {
		for _, id := range ids {
			if a.Def.ID == id {
				stolen = append(stolen, a)
				break
			}
		}
	})
	return stolen
}
func (s *Scene) specialWeaponAfterDispel(src, dst *entity.Entity, d domain.SkillDef, removed []domain.StatusID, stolen []domain.Active) {
	if d.SpecialWeaponBaseID == 1325 && d.ID == 10316 && len(removed) > 0 {
		amount := domain.SaturateInt32(int64(dst.MaxHP) * int64(min(3, len(removed))) * 3 / 100)
		s.specialWeaponHeal(src, dst, amount, d)
	}
	for _, a := range stolen {
		if a.ExpireAt <= s.tick && !a.Def.Permanent {
			continue
		}
		x := a.Def
		x.SkillSnapshotID = 0
		x.SkillSnapshotRank = 0
		x.SkillSnapshotRaw = nil
		x.SkillSnapshotHard = nil
		x.Affixes = append([]domain.Affix(nil), x.Affixes...)
		if a.Def.Permanent {
			x.DurationSec = 0
		} else {
			x.DurationSec = int32((a.ExpireAt - s.tick + domain.Ticks(1000) - 1) / domain.Ticks(1000))
		}
		x.DamageShield = a.ShieldLeft
		x.ChanceExplicit = true
		x.ChanceBP = 10000
		if s.applyStatusDef(src, x, src.ID) {
			src.Status.RestoreTransferredStatusBalance(src.ID, a)
		}
	}
}
func (s *Scene) specialWeaponConvertShield(src, dst *entity.Entity, d domain.SkillDef) bool {
	if d.SpecialWeaponBaseID != 1323 || d.ID != 10320 || dst.Status == nil {
		return false
	}
	var shield int32
	dst.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusDamageShield && a.Source == src.ID {
			shield += a.ShieldLeft
		}
	})
	if shield <= 0 {
		return false
	}
	s.specialWeaponRemoveStatuses(dst, src.ID, func(a domain.Active) bool { return a.Def.ID == domain.StatusDamageShield })
	amount := specialScale(shield, 60)
	var second *entity.Entity
	for _, p := range s.players {
		if p.ID == dst.ID || !p.Alive() || p.HP >= p.MaxHP || sqDist(dst.Pos, p.Pos) > 150*150 || !s.validTarget(src, p, d) {
			continue
		}
		if second == nil || int64(p.HP)*int64(second.MaxHP) < int64(second.HP)*int64(p.MaxHP) || int64(p.HP)*int64(second.MaxHP) == int64(second.HP)*int64(p.MaxHP) && p.ID < second.ID {
			second = p
		}
	}
	if second != nil {
		other := amount / 2
		amount -= other
		s.specialWeaponHeal(src, second, other, d)
	}
	s.specialWeaponHeal(src, dst, amount, d)
	return true
}
func (s *Scene) specialWeaponAim(src *entity.Entity, d *domain.SkillDef, target domain.EntityID) {
	if d.SpecialWeaponBaseID != 1323 || d.ID != 10410 {
		return
	}
	dst := s.entities[target]
	if dst == nil || dst.Status == nil {
		return
	}
	found := false
	dst.Status.EachEffective(func(a domain.Active) {
		if a.Source == src.ID && (a.Def.SkillSnapshotID == 10428 || a.Def.SourceSkillID == 10428) && a.Def.ID == domain.StatusFreeze {
			found = true
			d.SpecialWeaponFreezeExpiresAt = uint64(a.ExpireAt)
		}
	})
	if !found {
		return
	}
	d.Area.Center = domain.SkillAreaCenterTarget
	d.SpecialWeaponConsumeFreeze = target
	if ice, ok := s.skills.Get(10428, src.Player.Char.Skills[10428]); ok {
		d.Dist = ice.Dist
	}
	specialDamage(d, 80)
	out := d.Statuses[:0]
	for _, a := range d.Statuses {
		if a.ID != domain.StatusFreeze {
			out = append(out, a)
		}
	}
	d.Statuses = out
}
func (s *Scene) specialWeaponConsumeFreeze(src *entity.Entity, d domain.SkillDef) {
	if d.SpecialWeaponConsumeFreeze == 0 {
		return
	}
	dst := s.entities[d.SpecialWeaponConsumeFreeze]
	if dst == nil || dst.Status == nil {
		return
	}
	s.specialWeaponRemoveStatuses(dst, src.ID, func(a domain.Active) bool {
		x := a.Def
		return uint64(a.ExpireAt) == d.SpecialWeaponFreezeExpiresAt && x.ID == domain.StatusFreeze && (x.SkillSnapshotID == 10428 || x.SourceSkillID == 10428)
	})
}
func (s *Scene) specialWeaponCounterDamage(p *entity.Entity, damage int32) int32 {
	if s.specialWeapon(p) == 1821 {
		return specialScale(damage, 150)
	}
	return damage
}
func (s *Scene) specialWeaponBasicAttack(p *entity.Entity, blow *combat.Blow) {
	if p == nil || blow == nil || p.Player == nil {
		return
	}
	s.specialWeaponConsumeCry(p)
	if p.Status != nil {
		if bonus := p.Status.ConsumeSpecialWeaponBasicBonus(); bonus > 0 {
			pct := blow.SkillPct
			if pct == 0 {
				pct = 100
			}
			blow.SkillPct = specialScale(pct, 100+bonus)
		}
	}
	if s.specialWeapon(p) == 1821 && p.Player.Char.Skills[10203] > 0 {
		pct := blow.SkillPct
		if pct == 0 {
			pct = 100
		}
		blow.SkillPct = specialScale(pct, 90)
	}
}

func specialWeaponBuffPower(d *domain.StatusDef, pct int32) {
	if pct <= 0 || d.Harmful() {
		return
	}
	d.Affixes = append([]domain.Affix(nil), d.Affixes...)
	for i := range d.Affixes {
		a := &d.Affixes[i]
		if a.Value > 0 && a.Attr != domain.AttrAtkSpeed || a.Value < 0 && a.Attr == domain.AttrAtkSpeed {
			a.Value = specialScale(a.Value, 100+pct)
		}
	}
	d.HPChanges = append([]domain.ResourceChange(nil), d.HPChanges...)
	for i := range d.HPChanges {
		if d.HPChanges[i].Value > 0 {
			d.HPChanges[i].Value = specialScale(d.HPChanges[i].Value, 100+pct)
		}
	}
	d.MPChanges = append([]domain.ResourceChange(nil), d.MPChanges...)
	for i := range d.MPChanges {
		if d.MPChanges[i].Value > 0 {
			d.MPChanges[i].Value = specialScale(d.MPChanges[i].Value, 100+pct)
		}
	}
	for _, v := range []*int32{&d.PhysicalDamageReductionPct, &d.MagicDamageReductionPct, &d.PhysicalReflectPct, &d.MagicReflectPct, &d.PhysicalReflectFlat, &d.MagicReflectFlat, &d.DamageShield, &d.StealthDamagePct, &d.NextBasicAttackPct, &d.NextSkillDamagePct, &d.NextCastTimeReductionPct, &d.LifeStealPct, &d.ManaStealPct, &d.OutgoingMagicDamageFlat, &d.MagicDamageReductionFlat, &d.ManaToHealthRate} {
		if *v > 0 {
			*v = specialScale(*v, 100+pct)
		}
	}
	// Boolean immunity/stealth remains binary; strengthening never extends its
	// duration a second time or creates unsupported 125%-true control semantics.
}
func (s *Scene) specialWeaponImmediateTrap(src *entity.Entity, d domain.SkillDef, at domain.Pos, radius int32, duration int32, source *domain.CombatSourceSnapshotV5) bool {
	if d.SpecialWeaponBaseID != 1822 || d.ID != 10213 {
		return false
	}
	positions := []domain.Pos{at}
	if legendaryAssassinHas(s, src, "CK-A13", source) {
		positions = append(legendaryAssassinSidePlacements(src.Pos, at, radius, d), at)
	}
	for _, pos := range positions {
		s.specialWeaponImmediateTrapAt(src, d, pos, radius, duration, source)
	}
	return true
}

// Each native blast gets its own target set and one target-modifier pass. Side
// blasts cannot generate more placements or pay the accepted cast cost again.
func (s *Scene) specialWeaponImmediateTrapAt(src *entity.Entity, d domain.SkillDef, at domain.Pos, radius int32, duration int32, source *domain.CombatSourceSnapshotV5) {
	payload := trapPayload(d)
	// Placement spells carry self/point targeting, not the damage payload's
	// enemy flag. Reuse native trap victim selection, including faction rules.
	ids := s.trapTargets(&entity.Entity{Pos: at, Trap: &entity.Trap{Owner: src.ID, Radius: radius}})
	offense := s.equipmentOffense(src)
	if source != nil {
		offense = source.Equipment
	}
	finish := s.beginEquipmentRoot(src.ID, 0, offense)
	defer finish()
	s.equipment.root.def = payload
	s.equipment.root.targets = ids
	s.equipment.root.center = at
	if source != nil {
		s.equipment.root.source = source
		s.equipment.root.attackStats = source.Stats
		s.equipment.root.attackSnapshot = true
	}
	for _, id := range ids {
		target := s.entities[id]
		local := s.legendaryTargetDefinition(src, payload, target, 0, ids)
		s.applySkill(src, target, local, duration, true)
	}
}

type specialWeaponSmoke struct {
	owner         *entity.Entity
	at            domain.Pos
	def           domain.SkillDef
	expires, next domain.Tick
	targets       []domain.EntityID
}

func (s *Scene) stepSpecialWeaponSmoke() {
	if s.specialWeapons == nil {
		return
	}
	for id, st := range s.specialWeapons.players {
		if s.players[id] != st.owner || !st.owner.Alive() {
			delete(s.specialWeapons.players, id)
		}
	}
	for id, area := range s.specialWeapons.smoke {
		remove := func(target domain.EntityID) {
			if p := s.entities[target]; p != nil && p.Status != nil {
				s.specialWeaponRemoveStatuses(p, id, func(a domain.Active) bool { return a.Def.SpecialWeaponSourceBase == 1825 })
			}
		}
		if s.players[id] != area.owner || !area.owner.Alive() || area.expires <= s.tick {
			for _, target := range area.targets {
				remove(target)
			}
			delete(s.specialWeapons.smoke, id)
			continue
		}
		if area.next > s.tick {
			continue
		}
		area.next = s.tick + domain.Ticks(1000)
		var ids []domain.EntityID
		for target, p := range s.entities {
			if p.Alive() && sqDist(area.at, p.Pos) <= 120*120 && s.validTarget(area.owner, p, area.def) {
				ids = append(ids, target)
			}
		}
		center := &entity.Entity{Pos: area.at}
		ids = s.limitSpecialWeaponTargets(center, area.def, ids, 0)
		chosen := make(map[domain.EntityID]bool, len(ids))
		for _, target := range ids {
			chosen[target] = true
		}
		for _, target := range area.targets {
			if !chosen[target] {
				remove(target)
			}
		}
		for _, target := range ids {
			dst := s.entities[target]
			for _, app := range s.selectSkillStatuses(area.def.Statuses, dst.Kind) {
				state, ok := s.resolveSkillStatusDef(area.def, app, 0)
				if ok {
					state.DurationSec = max(int32(1), int32((area.expires-s.tick+domain.Ticks(1000)-1)/domain.Ticks(1000)))
					s.applyStatusDef(dst, state, id)
				}
			}
		}
		area.targets = ids
	}
}

// These are real healing events and keep the ordinary healer equipment chain.
func (s *Scene) specialWeaponHeal(src, dst *entity.Entity, amount int32, d domain.SkillDef) {
	if amount <= 0 {
		return
	}
	amount = s.uniqueHealing(src, dst, amount, d)
	amount = s.classSetCardHealing(src, amount)
	s.casterSetCopyHeal(src, dst, amount, d.ID)
	s.emit(combat.Restore(src, dst, amount, int32(d.ID)))
	s.emitAttributesIfPlayer(dst)
	rt := s.equipmentState()
	if rt.classSetTriggered == 0 && rt.root != nil && rt.root.src == src.ID {
		rt.root.classSetHealingStatus = true
	}
}
func (s *Scene) specialWeaponRemoveStatuses(dst *entity.Entity, source domain.EntityID, match func(domain.Active) bool) {
	changed := false
	dst.Status.Each(func(a domain.Active) {
		if a.Source == source && match(a) {
			changed = true
		}
	})
	ids := dst.Status.RemoveSourceActiveWhere(source, match)
	s.statusesRemoved(dst, ids)
	if changed && len(ids) == 0 {
		s.refreshEntityStats(dst)
		s.emitAttributesIfPlayer(dst)
		s.pushStatusSnapshots(dst)
	}
}
