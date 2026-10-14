package scene

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func TestReviewExpiredSuppressedVulnerabilityCannotAffectEarlyFrameStrike(t *testing.T) {
	for _, duration := range []int32{5, 6} {
		s := newCombatScene(t)
		src := putMonster(s, 898001, 100000, domain.Stats{MinAtk: 1000, MaxAtk: 1000}, 100, 100)
		dst := putMonster(s, 898002, 100000, domain.Stats{}, 110, 100)
		bad := domain.StatusDef{ID: 898003, Level: 1, TypeKnown: true, Type: domain.StatusBad,
			DurationSec: duration, PhysicalDamageTakenPct: 100}
		if !s.applyStatusDef(dst, bad, src.ID) || !s.applyStatusDef(dst, explicitSceneMitigationBuff(), dst.ID) {
			t.Fatal("status setup failed")
		}
		s.tick += domain.Ticks(7000)
		ev, raw, _ := s.resolveStrikeStatuses(src, dst, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
		if raw <= 0 || ev.Amount != raw {
			t.Errorf("expired %ds vulnerability affected early-frame strike: raw=%d final=%d", duration, raw, ev.Amount)
		}
	}
}

func TestReviewWallsRotatedContinuousCrossingAndFlatEnds(t *testing.T) {
	for degrees := 0; degrees < 360; degrees += 5 {
		t.Run(fmt.Sprint(degrees), func(t *testing.T) {
			a := float64(degrees) * math.Pi / 180
			ux, uy := math.Cos(a), math.Sin(a)
			pos := func(x, y float64) domain.Pos { return temporaryTestPos(1000+x*ux-y*uy, 1000+x*uy+y*ux) }
			p, q := pos(-40, 0), pos(40, 0)
			w := temporaryTestWall("rotated", p.X, p.Y, q.X, q.Y)
			// Cross through the middle, just outside the expanded flat end, and
			// diagonally through the tiny expanded corner without sampling it.
			for _, c := range []struct {
				x1, y1, x2, y2 float64
				blocked        bool
			}{
				{0, -100000, 0, 100000, true}, {52.01, -100, 52.01, 100, false},
				{51.99, 21.99, 100, 100, true}, {-100, 22.01, 100, 22.01, false},
				{-52.01, 21.98, -51.98, 22.01, true},
			} {
				from, to := pos(c.x1, c.y1), pos(c.x2, c.y2)
				if got := w.pathBlocked(from, to, temporaryWallActorClearance); got != c.blocked {
					t.Fatalf("rotation=%d case=%+v collision=%v", degrees, c, got)
				}
				if got := w.pathBlocked(to, from, temporaryWallActorClearance); got != c.blocked {
					t.Fatalf("reversal changes rotation=%d case=%+v collision=%v", degrees, c, got)
				}
			}
		})
	}
}

func TestReviewWallExpiryBeforeSweepReleasesKnockback(t *testing.T) {
	s, now := temporaryWallFixture(t)
	w := temporaryTestWall("boundary", 100, 60, 100, 140)
	s.temporaryWalls = map[string]temporaryWall{w.ID: w}
	src := &entity.Entity{Pos: temporaryTestPos(0, 100)}
	target := putMonster(s, 898100, 100, domain.Stats{}, 50, 100)
	*now = w.ExpiresAtMS - 1
	if !s.knockbackMonster(src, target, 200) || target.Pos.X >= 78 {
		t.Fatal("pre-expiry knockback crossed padded wall", target.Pos)
	}
	s.aoi.Move(target, temporaryTestPos(50, 100))
	*now = w.ExpiresAtMS
	if !s.knockbackMonster(src, target, 200) || target.Pos.X != 250 {
		t.Fatal("exact-expiry wall blocked knockback before sweep", target.Pos)
	}
}

func TestReviewUnsafeWallRestoreIsAtomic(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	live := temporaryTestWall("existing", 100, 60, 100, 140)
	if !s.addTemporaryWalls([]temporaryWall{live}) {
		t.Fatal("initial safe wall failed")
	}
	invalid := temporaryTestWall("bad", 300, 60, 300, 140)
	invalid.Width = math.Inf(1)
	if s.restoreTemporaryWalls([]temporaryWall{invalid}) {
		t.Fatal("nonfinite snapshot accepted")
	}
	if len(s.temporaryWalls) != 1 || s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("failed restore replaced good collision state")
	}
}

func TestReviewCounteractingMixedStatusRefreshesRemainingEffectiveStats(t *testing.T) {
	s, mid, _ := statusScene(t)
	p := beefUp(s)
	baseline := p.Stats.MinAtk
	bad := domain.StatusDef{ID: 898201, Level: 1, TypeKnown: true, Type: domain.StatusBad, DurationSec: 20, Control: domain.ControlStun}
	good := domain.StatusDef{ID: bad.ID, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 20,
		Affixes: []domain.Affix{{Attr: domain.AttrAtk, Mode: domain.ModePercent, Value: 50}}}
	s.statusOverlay = domain.StatusOverlay{{Existing: bad.ID, Incoming: bad.ID}: domain.OverlayCoexist,
		{Existing: bad.ID, Incoming: 898202}: domain.OverlayCounteract}
	if !s.applyStatusDef(p, bad, mid) || !s.applyStatusDef(p, explicitSceneMitigationBuff(), p.ID) || !s.applyStatusDef(p, good, p.ID) {
		t.Fatal("mixed status setup failed")
	}
	if p.Stats.MinAtk <= baseline {
		t.Fatal("positive sibling did not increase attack")
	}
	if s.applyStatusDef(p, domain.StatusDef{ID: 898202, Level: 1, TypeKnown: true, Type: domain.StatusGood, DurationSec: 10}, p.ID) {
		t.Fatal("counteracted incoming effect was applied")
	}
	if !p.Status.Has(bad.ID) {
		t.Fatal("counteracting the positive sibling deleted the suppressed harmful status")
	}
	if p.Stats.MinAtk != baseline {
		t.Fatalf("partial counteract left stale effective attack: got=%d want=%d", p.Stats.MinAtk, baseline)
	}
	s.tick += domain.Ticks(5000)
	s.stepStatus()
	if canMove(p) {
		t.Fatal("remaining original harmful control failed to resume")
	}
}

func TestReviewTemporaryWallsDoNotBlockHazardLineOfSight(t *testing.T) {
	s, m, p, now := riftEliteFixture(t, 2)
	origin := domain.Pos{MapID: 7, X: 940, Y: 1000}
	p.Pos = domain.Pos{MapID: 7, X: 1100, Y: 1000}
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: m.Stats, Pos: origin, Ability: s.currentRiftRules().Abilities[2]}
	before := p.HP
	s.hitRiftHazard(h, p, 100)
	baseline := before - p.HP
	if baseline <= 0 {
		t.Fatal("clear static line of sight did not deliver reference damage")
	}
	w := temporaryWall{ID: "review/los", Owner: "review/cast", MapID: 7, X: 1020, Y: 960, X2: 1020, Y2: 1040, Width: 20, ExpiresAtMS: *now + 15000}
	s.temporaryWalls = map[string]temporaryWall{w.ID: w}
	if s.temporaryPathWalkable(origin, p.Pos) {
		t.Fatal("wall did not block movement across the reference segment")
	}
	before = p.HP
	s.hitRiftHazard(h, p, 100)
	if before-p.HP != baseline {
		t.Fatalf("temporary movement wall changed hazard LOS/damage: got=%d want=%d", before-p.HP, baseline)
	}
}

func TestReviewIceOrbUsesResistanceAndCapturedEffectHit(t *testing.T) {
	s, m, p, _ := riftEliteFixture(t, 4)
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: m.Stats, Pos: p.Pos, Ability: s.currentRiftRules().Abilities[4]}
	// The detached hazard must use the captured value even if its still-living
	// source now has much more effect accuracy.
	m.Stats.EffectHitBP = 1000000
	for _, hit := range []int32{0, 10000} {
		h.Attack.EffectHitBP = hit
		landed := 0
		for i := 0; i < 128; i++ {
			s.rng = rand.New(rand.NewSource(int64(i + 1)))
			p.Status = domain.NewStatusSet()
			p.HP, p.MaxHP = 10000, 10000
			p.Stats.StatusResist = 100
			s.hitRiftHazard(h, p, 100)
			if p.Status.Has(domain.StatusFreeze) {
				landed++
			}
		}
		if hit == 0 && (landed == 0 || landed == 128) {
			t.Fatalf("100%% status resistance ignored or source snapshot bypassed: landed=%d/128", landed)
		}
		if hit == 10000 && landed != 128 {
			t.Fatalf("captured 100%% effect hit failed to offset 100%% resistance: landed=%d/128", landed)
		}
	}
}

func TestReviewTemporaryWallsDoNotBlockRangedRiftCastAdmission(t *testing.T) {
	for _, elite := range []bool{false, true} {
		t.Run(fmt.Sprint(elite), func(t *testing.T) {
			var s *Scene
			var m, p *entity.Entity
			var now *int64
			if elite {
				s, m, p, now = riftEliteFixture(t, 3)
			} else {
				s, m, p, now = riftAbilityFixture(t, riftEruption)
			}
			m.Pos = temporaryTestPos(1000, 1000)
			p.Pos = temporaryTestPos(1300, 1000)
			w := temporaryWall{ID: "review/admission", Owner: "review/cast", MapID: 7, X: 1150, Y: 960, X2: 1150, Y2: 1040, Width: 20, ExpiresAtMS: *now + 15000}
			s.temporaryWalls = map[string]temporaryWall{w.ID: w}
			s.stepEndgameRiftAbilities(*now)
			if elite {
				if len(s.eliteSnapshot().Effects) != 1 {
					t.Fatal("temporary movement wall prevented new ranged elite cast")
				}
			} else if s.rift.abilities[m.ID].pending == nil {
				t.Fatal("temporary movement wall prevented legacy ranged rift cast")
			}
		})
	}
}

func TestReviewHazardLineOfSightStillHonorsStaticTerrain(t *testing.T) {
	s, m, p, _ := riftEliteFixture(t, 2)
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: m.Stats, Pos: temporaryTestPos(940, 1000), Ability: s.currentRiftRules().Abilities[2]}
	p.Pos = temporaryTestPos(1100, 1000)
	s.walkable = func(at domain.Pos) bool { return at.X < 1000 || at.X > 1050 }
	before := p.HP
	s.hitRiftHazard(h, p, 100)
	if p.HP != before {
		t.Fatal("static terrain stopped blocking hazard line of sight")
	}
}
