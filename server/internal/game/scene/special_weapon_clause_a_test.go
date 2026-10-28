package scene

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
)

// Removed native effects must disappear at settlement, not just from preview.
func TestSpecialWeaponClauseARemovedEffectSettlement(t *testing.T) {
	for _, tc := range []struct {
		item  domain.ItemID
		skill domain.SkillID
		kind  domain.EffectKind
	}{{1021, 10102, domain.EffectKnockback}, {1527, 10123, domain.EffectInstantDeath}} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/active=%v", tc.item, active), func(t *testing.T) {
				s, p, m := specialWeaponActors(t)
				s.rng = rand.New(meleeAuditZeroSource{})
				p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
				m.Pos.X += 20
				start := m.Pos
				d := domain.SkillDef{ID: tc.skill, Prof: 2, TargetEnemy: true, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}, {Kind: tc.kind, ChanceBP: 10000, ChanceExplicit: true, EffectHitBP: 10000, KnockbackDistance: 40}}}
				if active {
					applySpecialWeaponDefinition(tc.item, &d)
				}
				s.applySkill(p, m, d, 0, false)
				if tc.kind == domain.EffectKnockback {
					want := start.X + 40
					if active {
						want = start.X
					}
					if m.Pos.X != want {
						t.Fatalf("position=%v wantX%v", m.Pos, want)
					}
				} else {
					want := int32(0)
					if active {
						want = 9900
					}
					if m.HP != want {
						t.Fatalf("HP=%d want%d", m.HP, want)
					}
				}
			})
		}
	}
}

func TestSpecialWeaponClauseADefenseCapSettlement(t *testing.T) {
	for _, reduction := range []int32{60, 90} {
		for _, magic := range []bool{false, true} {
			t.Run(fmt.Sprintf("reduction%d/magic%v", reduction, magic), func(t *testing.T) {
				s, p, m := specialWeaponActors(t)
				p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk = 100, 100, 100
				d := domain.SkillDef{ID: 10107, Prof: 2}
				applySpecialWeaponDefinition(1026, &d)
				st := domain.StatusDef{ID: 180099900, Level: 1, DurationSec: 20, PhysicalDamageReductionPct: reduction, MagicDamageReductionPct: reduction}
				specialWeaponStatus(d, &st)
				s.applyStatusDef(m, st, p.ID)
				before := m.HP
				s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true, Magic: magic})
				want := int32(20)
				if reduction == 90 {
					want = 10
				}
				if before-m.HP != want {
					t.Fatalf("damage=%d want%d", before-m.HP, want)
				}
			})
		}
	}
}

func TestSpecialWeaponClauseAShieldsReplaceAndExpire(t *testing.T) {
	for _, item := range []domain.ItemID{1522, 1622} {
		t.Run(fmt.Sprint(item), func(t *testing.T) {
			s, p, m := specialWeaponActors(t)
			p.MaxMP = 1000
			p.Stats.MaxAtk = 200
			seconds := int32(5)
			if item == 1622 {
				seconds = 4
			}
			grant := func(amount int32) {
				p.Stats.MaxAtk = 200
				if item == 1522 {
					p.MP = p.MaxMP
					s.applySkill(p, p, domain.SkillDef{ID: 10119, Prof: 2, SpecialWeaponBaseID: item, TargetSelf: true, Effect: domain.SkillEffect{Kind: domain.EffectRestoreMP, RestoreMPFlat: amount * 2}}, 0, false)
				} else {
					m.MP = amount * 2
					s.applySkill(p, m, domain.SkillDef{ID: 10013, Prof: 1, SpecialWeaponBaseID: item, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamageMP, SourceAttackPct: 100}}, 0, false)
				}
			}
			shield := func() int32 {
				var n int32
				p.Status.EachEffective(func(a domain.Active) {
					if a.Def.ID == domain.StatusID(1800100000+item) {
						n += a.ShieldLeft
					}
				})
				return n
			}
			for _, amount := range []int32{50, 20, 80} {
				grant(amount)
				if shield() != amount {
					t.Fatalf("replacement=%d want%d", shield(), amount)
				}
			}
			s.tick += domain.Ticks(int(seconds)*1000) - 1
			s.stepStatus()
			if shield() != 80 {
				t.Fatal("shield expired early", shield())
			}
			s.tick++
			s.stepStatus()
			if shield() != 0 {
				t.Fatal("shield retained at expiry", shield())
			}
			p.HP = p.MaxHP
			before := p.HP
			m.Stats.MinAtk, m.Stats.MaxAtk = 100, 100
			p.Stats.Def = 0
			s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
			if before-p.HP != 100 {
				t.Fatalf("expired shield still absorbed: damage%d", before-p.HP)
			}
		})
	}
}

func TestSpecialWeaponClauseATauntSpeedThresholdAndExpiry(t *testing.T) {
	for _, n := range []int{2, 3} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s, p, _ := specialWeaponActors(t)
			base := p.Stats.MoveSpeed
			hits := []domain.EntityID{2, 3, 4}
			s.specialWeaponCastFinished(p, domain.SkillDef{ID: 10127, SpecialWeaponBaseID: 1526}, hits[:n])
			want := base
			if n == 3 {
				want = base * 120 / 100
			}
			if p.Stats.MoveSpeed != want {
				t.Fatalf("speed%d want%d", p.Stats.MoveSpeed, want)
			}
			s.tick += domain.Ticks(5000) - 1
			s.stepStatus()
			if p.Stats.MoveSpeed != want {
				t.Fatal("early expiration")
			}
			s.tick++
			s.stepStatus()
			if p.Stats.MoveSpeed != base {
				t.Fatal("speed not restored", p.Stats.MoveSpeed, base)
			}
		})
	}
}

func TestSpecialWeaponClauseAChaosNativeDenialAndDispelSelection(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, p, m, d := verifiedEffectScene(t, 10122, 5)
			if active {
				applySpecialWeaponDefinition(1027, &d)
			}
			// A non-defense buff is never selected; the native denial still applies.
			other := domain.StatusDef{ID: 180099901, Level: 1, DurationSec: 30, TypeKnown: true, Type: domain.StatusGood, Affixes: []domain.Affix{{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: 1}}}
			s.applyStatusDef(m, other, m.ID)
			s.applySkill(p, m, d, 0, false)
			if !m.Status.Has(other.ID) {
				t.Fatal("non-defense buff was removed")
			}
			blocked := domain.StatusDef{ID: 180099902, Level: 1, DurationSec: 30, TypeKnown: true, Type: domain.StatusGood, DamageShield: 100}
			if s.applyStatusDef(m, blocked, m.ID) || m.Status.Has(blocked.ID) {
				t.Fatal("native chaos admitted new beneficial status")
			}
		})
	}
}

func TestSpecialWeaponClauseABreakFollowupExpiry(t *testing.T) {
	for _, elapsed := range []int{4999, 5000} {
		t.Run(fmt.Sprint(elapsed), func(t *testing.T) {
			s, p, m := specialWeaponActors(t)
			p.Player.Char.Race = domain.Warrior
			p.Player.Char.Skills = domain.Learned{10027: 1, 10009: 1}
			specialWeaponEquip(t, s, p, 1626)
			p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
			s.specialWeaponAfterHit(p, m, domain.SkillDef{ID: 10027, Prof: 1, SpecialWeaponBaseID: 1626}, true, false)
			s.tick += domain.Ticks(5000)
			if elapsed == 4999 {
				s.tick--
			}
			d := domain.SkillDef{ID: 10009, Prof: 1, SpecialWeaponBaseID: 1626, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			s.specialWeaponPrepareCast(p, &d)
			before := m.HP
			s.applySkill(p, m, d, 0, false)
			want := int32(140)
			if elapsed == 5000 {
				want = 100
			}
			if before-m.HP != want {
				t.Fatalf("actual damage%d want%d", before-m.HP, want)
			}
		})
	}
}

func TestSpecialWeaponClauseAWarcryRefreshAndGunOnly(t *testing.T) {
	s, p, _ := specialWeaponActors(t)
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Skills = domain.Learned{10028: 1}
	specialWeaponEquip(t, s, p, 1624)
	p.MaxHP = 10000
	cry := domain.SkillDef{ID: 10028, SpecialWeaponBaseID: 1624}
	s.specialWeaponCastReleased(p, cry)
	s.specialWeaponLostHP(p, 500)
	s.tick += domain.Ticks(1000)
	s.specialWeaponCastReleased(p, cry)
	s.specialWeaponLostHP(p, 100)
	other := domain.SkillDef{RequiredWeaponType: 2, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.specialWeaponPrepareCast(p, &other)
	if p.Status.Has(1800101624) {
		t.Fatal("non-gun attack consumed charge")
	}
	gun := other
	gun.RequiredWeaponType = 1
	s.specialWeaponPrepareCast(p, &gun)
	shield := func() int32 {
		var n int32
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == 1800101624 {
				n += a.ShieldLeft
			}
		})
		return n
	}
	if shield() != 20 {
		t.Fatalf("refresh must replace accumulated loss; shield%d want20", shield())
	}
	s.tick += domain.Ticks(5000) - 1
	s.stepStatus()
	if shield() != 20 {
		t.Fatal("early shield expiry")
	}
	s.tick++
	s.stepStatus()
	if shield() != 0 {
		t.Fatal("late shield expiry")
	}
}

func TestSpecialWeaponClauseANativeControlRelease(t *testing.T) {
	for _, tc := range []struct {
		item            domain.ItemID
		skill           domain.SkillID
		seconds         int
		move, act, cast bool
	}{{1024, 10111, 5, false, true, true}, {1025, 10113, 3, true, true, false}, {1521, 10111, 5, false, true, true}, {1626, 10027, 5, false, false, false}, {1820, 10219, 1, false, false, false}} {
		t.Run(fmt.Sprint(tc.item), func(t *testing.T) {
			s, p, m, d := verifiedEffectScene(t, tc.skill, 5)
			applySpecialWeaponDefinition(tc.item, &d)
			// Native statuses retain their normal control flags and transformed lifetime.
			s.applySkill(p, m, d, 0, false)
			if canMove(m) != tc.move || canAct(m) != tc.act || canCast(m) != tc.cast {
				t.Fatalf("initial permission move/act/cast %v/%v/%v", canMove(m), canAct(m), canCast(m))
			}
			s.tick += domain.Ticks(tc.seconds*1000) - 1
			s.stepStatus()
			if canMove(m) != tc.move || canAct(m) != tc.act || canCast(m) != tc.cast {
				t.Fatalf("early release move/act/cast %v/%v/%v", canMove(m), canAct(m), canCast(m))
			}
			s.tick++
			s.stepStatus()
			if !canMove(m) || !canAct(m) || !canCast(m) {
				t.Fatal("control not released at expiry")
			}
		})
	}
}

func TestSpecialWeaponClauseADefenseDebuffActualStatsAndExpiry(t *testing.T) {
	s, p, m, _ := verifiedEffectScene(t, 10122, 5)
	base := m.Stats.Def
	s.specialWeaponAfterHit(p, m, domain.SkillDef{ID: 10122, Prof: 2, SpecialWeaponBaseID: 1525}, true, false)
	if m.Stats.Def != base*85/100 {
		t.Fatalf("defense%d want%d", m.Stats.Def, base*85/100)
	}
	debuffChance := combat.HitChance(100, m.Stats.Def)
	s.tick += domain.Ticks(5000) - 1
	s.stepStatus()
	if m.Stats.Def != base*85/100 {
		t.Fatal("early expiry")
	}
	s.tick++
	s.stepStatus()
	if m.Stats.Def != base {
		t.Fatal("defense not restored")
	}
	if debuffChance <= combat.HitChance(100, m.Stats.Def) {
		t.Fatal("defense reduction did not improve physical hit chance")
	}
}

// Cooldown transformations all feed this same native acceptance clock.
func TestSpecialWeaponClauseANativeCooldownBoundary(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, p := behaviorAcceptancePlayer(t, 3)
			p.Player.Char.Skills = domain.Learned{10225: 5}
			if active {
				specialWeaponEquip(t, s, p, 1929)
			}
			p.MaxMP, p.MP = 100000, 100000
			native, _ := s.skills.Get(10225, 5)
			ms := native.CooldownMS
			if active {
				ms = ms * 75 / 100
			}
			at := domain.Pos{X: 120, Y: 100}
			s.exec(UseSkill{ID: p.ID, Skill: 10225, Target: p.ID, At: &at})
			if len(s.casts) != 1 {
				t.Fatal("native cast rejected")
			}
			expiry := s.tick + domain.Ticks(int(ms))
			s.stepCastsAt(s.now().Add(time.Minute), false)
			s.stepCastsAt(s.now().Add(2*time.Minute), false)
			if len(s.casts) != 0 {
				t.Fatal("first cast not finished")
			}
			s.tick = expiry - 1
			s.exec(UseSkill{ID: p.ID, Skill: 10225, Target: p.ID, At: &at})
			if len(s.casts) != 0 {
				t.Fatal("recast admitted before cooldown expiry")
			}
			s.tick++
			if p.Player.SkillReadyAt(10225, expiry-1) || !p.Player.SkillReadyAt(10225, expiry) {
				t.Fatalf("native cooldown boundary failed ms%d", ms)
			}
			s.exec(UseSkill{ID: p.ID, Skill: 10225, Target: p.ID, At: &at})
			if len(s.casts) != 1 {
				t.Fatal("recast rejected at cooldown expiry")
			}
		})
	}
}

func TestSpecialWeaponClauseAChaosPreservesUndispellableDefense(t *testing.T) {
	s, p, m := specialWeaponActors(t)
	m.Status.Apply(domain.StatusDef{ID: 1010, Level: 1, TypeKnown: true, Type: domain.StatusGood, Permanent: true}, s.tick, m.ID)
	m.Status.Apply(domain.StatusDef{ID: 1011, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 30}, s.tick, m.ID)
	m.Status.Apply(domain.StatusDef{ID: 1021, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 30}, s.tick, m.ID)
	s.specialWeaponAfterHit(p, m, domain.SkillDef{ID: 10122, Prof: 2, SpecialWeaponBaseID: 1027}, true, false)
	if !m.Status.Has(1010) {
		t.Fatal("undispellable permanent defense removed")
	}
	if m.Status.Has(1011) || !m.Status.Has(1021) {
		t.Fatal("must remove exactly one eligible defense")
	}
}

func TestSpecialWeaponClauseABlindMagnitudeAndHalfExpiry(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, p, m, d := verifiedEffectScene(t, 10103, 5)
			if active {
				applySpecialWeaponDefinition(1022, &d)
			}
			s.applySkill(p, m, d, 0, false)
			if m.Stats.Hit != 50 {
				t.Fatalf("native hit reduction changed: hit%d", m.Stats.Hit)
			}
			seconds := 15
			if active {
				seconds = 7
			}
			s.tick += domain.Ticks(seconds*1000) - 1
			s.stepStatus()
			if m.Stats.Hit != 50 {
				t.Fatal("early blind expiration")
			}
			s.tick++
			s.stepStatus()
			if m.Stats.Hit != 100 {
				t.Fatal("hit stat not restored")
			}
		})
	}
}

func TestSpecialWeaponClauseAStealthHalfExpiry(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, p, _, d := verifiedEffectScene(t, 10208, 5)
			if active {
				applySpecialWeaponDefinition(1823, &d)
			}
			s.applySkill(p, p, d, 0, false)
			if !entityInvisible(p) {
				t.Fatal("native stealth not active")
			}
			var expires domain.Tick
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.Invisible {
					expires = a.ExpireAt
				}
			})
			nativeStatus, _ := s.statuses.Get(d.Statuses[0].ID, d.Statuses[0].Level)
			seconds := nativeStatus.DurationSec
			if active {
				seconds = seconds / 2
			}
			if expires-s.tick != domain.Ticks(int(seconds)*1000) {
				t.Fatalf("wrong native stealth duration%d expires%d", seconds, expires)
			}
			s.tick = expires - 1
			s.stepStatus()
			if !entityInvisible(p) {
				t.Fatal("stealth released early")
			}
			s.tick++
			s.stepStatus()
			if entityInvisible(p) {
				t.Fatal("stealth did not expire")
			}
		})
	}
}

func TestSpecialWeaponClauseAUnchangedConeAndBasicTradeoffHP(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, p, m := specialWeaponActors(t)
			p.Player.Char.Race = domain.Assassin
			p.Player.Char.Skills = domain.Learned{10203: 1}
			if active {
				specialWeaponEquip(t, s, p, 1821)
			}
			p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
			blow := combat.Blow{SkillPct: 100, GuaranteedHit: true}
			s.specialWeaponBasicAttack(p, &blow)
			before := m.HP
			s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, blow)
			want := int32(100)
			if active {
				want = 90
			}
			if before-m.HP != want {
				t.Fatalf("basic damage%d want%d", before-m.HP, want)
			}
			d := domain.SkillDef{ID: 10006, Prof: 1, TargetEnemy: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			if active {
				applySpecialWeaponDefinition(1623, &d)
			}
			before = m.HP
			s.applySkill(p, m, d, 0, false)
			if before-m.HP != 100 {
				t.Fatal("cone damage changed")
			}
		})
	}
}

func TestSpecialWeaponClauseACircleActualBoundaryRecipients(t *testing.T) {
	for _, tc := range []struct {
		item   domain.ItemID
		skill  domain.SkillID
		prof   int32
		radius int32
		target bool
	}{{1022, 10103, 2, 100, false}, {1521, 10111, 2, 100, true}, {1524, 10121, 2, 135, false}, {1526, 10127, 2, 160, false}, {1627, 10022, 1, 80, false}} {
		t.Run(fmt.Sprint(tc.item), func(t *testing.T) {
			for _, outside := range []bool{false, true} {
				s, p, m := specialWeaponActors(t)
				m.Pos.X += 50
				s.aoi.Enter(m)
				d := domain.SkillDef{ID: tc.skill, Prof: tc.prof, Kind: domain.SkillArea, TargetEnemy: true, Dist: 500, Radius: 100, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}}
				applySpecialWeaponDefinition(tc.item, &d)
				probe := *m
				probe.ID = 998
				probe.Pos = p.Pos
				if tc.target {
					probe.Pos = m.Pos
				}
				probe.Pos.Y += float64(tc.radius)
				if outside {
					probe.Pos.Y++
				}
				s.entities[probe.ID] = &probe
				s.monsters[probe.ID] = &probe
				s.aoi.Enter(&probe)
				ids := s.resolveTargets(p, d, m.ID, nil)
				found := false
				for _, id := range ids {
					if id == probe.ID {
						found = true
					}
				}
				if found == outside {
					t.Fatalf("outside=%v recipients%v", outside, ids)
				}
			}
		})
	}
}
