package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLegendaryFinalReviewGroupStealSpecialWeapon(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U02")
	other := putMonster(s, 9991, 10000, domain.Stats{}, 140, 100)
	other.Status = domain.NewStatusSet()
	for _, target := range []domain.EntityID{c.Target.ID, other.ID} {
		e := s.entities[target]
		e.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, DurationSec: 20, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}, s.tick, e.ID)
		e.Status.AbsorbDamage(90)
	}
	d := domain.SkillDef{ID: 10433, SpecialWeaponBaseID: 1324, TargetEnemy: true, Effects: []domain.SkillEffect{{RemoveStatusIDs: []domain.StatusID{1021}, DispelChanceBP: 10000}}}
	finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, s.equipmentOffense(c.Owner))
	s.equipment.root.def = d
	s.equipment.root.targets = []domain.EntityID{c.Target.ID, other.ID}
	s.applySkill(c.Owner, c.Target, d, 100, false)
	s.applySkill(c.Owner, other, d, 100, false)
	finish()
	shields := 0
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1021 {
			shields++
			if a.ShieldLeft != 100 {
				t.Errorf("group steal plus weapon1324 kept residual shield %d, want100", a.ShieldLeft)
			}
		}
	})
	if shields != 1 {
		t.Fatal("shield count", shields)
	}
}

func TestLegendaryFinalReviewWeaponStealOnlyRemoved(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-U02")
	// Disable the legendary instance: the native weapon path remains independent.
	c.Owner.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		st.Legendary = domain.LegendaryInstance{}
		c.Owner.Player.Worn.Set(slot, st)
	})
	d := domain.SkillDef{ID: 10433, SpecialWeaponBaseID: 1324}
	permanent := domain.StatusDef{ID: 1021, Permanent: true, DamageShield: 100}
	c.Target.Status.Apply(permanent, s.tick, c.Target.ID)
	if got := s.specialWeaponDispel(c.Owner, c.Target, d, []domain.StatusID{1021}); len(got) != 0 {
		t.Fatal("permanent state stealable")
	}
	c.Target.Status.Remove(1021)
	temporary := domain.StatusDef{ID: 1021, DurationSec: 20, DamageShield: 100}
	c.Target.Status.Apply(temporary, s.tick, c.Target.ID)
	stolen := s.specialWeaponDispel(c.Owner, c.Target, d, []domain.StatusID{1021})
	if len(stolen) != 1 {
		t.Fatal("native capture missing")
	}
	s.specialWeaponAfterDispel(c.Owner, c.Target, d, nil, stolen)
	if c.Owner.Status.Has(1021) {
		t.Fatal("copied without successful dispel")
	}
	s.specialWeaponAfterDispel(c.Owner, c.Target, d, []domain.StatusID{1021}, stolen)
	if !c.Owner.Status.Has(1021) {
		t.Fatal("removed native state not transferred")
	}
}

func TestLegendaryFinalReviewFailedEscapeKeepsControl(t *testing.T) {
	s, p, _ := casterIndependentFixture(t, "SS-U06", domain.Learned{10432: 1}, true)
	d, _ := s.skills.Get(10432, 1)
	d.Effects = append([]domain.SkillEffect(nil), d.DirectEffects()...)
	for i := range d.Effects {
		d.Effects[i].ChanceBP = 0
	}
	d.Effect = d.Effects[0]
	s.skills[domain.SkillKey{ID: 10432, Level: 1}] = d
	s.refreshStats(p)
	p.MP = p.MaxMP
	p.Status.Apply(domain.StatusDef{ID: domain.StatusStun, Level: 1, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun, DurationSec: 10}, s.tick, 900)
	from := p.Pos
	to := from
	to.X += 40
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10432, At: &to})
	sharedTargetFinishCasts(t, s)
	if p.Pos != from || !p.Status.Has(domain.StatusStun) {
		t.Fatal("failed teleport cleared control")
	}
}
