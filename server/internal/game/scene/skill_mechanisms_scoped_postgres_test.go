package scene

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func scopedSkillPostgresFixture(t *testing.T) (domain.SkillTable, domain.StatusTable, domain.StatusOverlay) {
	t.Helper()
	path := os.Getenv("FANTASY_SCOPED_SKILLS_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration66 PostgreSQL directory required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active/stale-postmaster cluster")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	skills, _, err := data.LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	statuses, _, err := data.LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	overlay, _, err := data.LoadStatusOverlay(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, statuses, overlay); err != nil {
		t.Fatal(err)
	}
	return skills, statuses, overlay
}

func scopedSkillTarget(t *testing.T, s *Scene, kind domain.EntityKind) *entity.Entity {
	t.Helper()
	if kind == domain.KindMonster {
		base := domain.Stats{Hit: 1000, Def: 100, MoveSpeed: 100}
		dst := putMonster(s, 801, 100000, base, 120, 100)
		dst.Status = domain.NewStatusSet()
		s.defs = spawn.MapDefs{dst.Monster.TypeID: {ID: dst.Monster.TypeID, Stats: base}}
		return dst
	}
	join(t, s, 2, 2, "target", 120, 100)
	dst := s.players[2]
	dst.Player.Char.Base = domain.Base{STR: 100, VIT: 10000, SPI: 1000, DEX: 1000, AGI: 100}
	s.refreshStats(dst)
	dst.HP = dst.MaxHP
	return dst
}

func scopedSkillApplication(t *testing.T, def domain.SkillDef, kind domain.EntityKind) domain.StatusApplication {
	t.Helper()
	var apps []domain.StatusApplication
	for _, app := range def.Statuses {
		if app.TargetType.Matches(kind) {
			apps = append(apps, app)
		}
	}
	if len(apps) != 1 {
		t.Fatalf("skill %d/%d kind%d matched %d status branches", def.ID, def.Level, kind, len(apps))
	}
	return apps[0]
}

func TestSkillMechanismsScopedPostgresRealSettlement(t *testing.T) {
	skills, statuses, overlay := scopedSkillPostgresFixture(t)
	for _, id := range []domain.SkillID{10317, 10326} {
		for rank := int32(1); rank <= 5; rank++ {
			def, ok := skills.Get(id, rank)
			if !ok {
				t.Fatal(id, rank)
			}
			if def.Description != skills[domain.SkillKey{ID: id, Level: rank}].Description {
				t.Fatal("authored description changed")
			}
			for _, kind := range []domain.EntityKind{domain.KindMonster, domain.KindPlayer} {
				app := scopedSkillApplication(t, def, kind)
				for _, mode := range []string{"hit", "immune", "fatal", "expire", "reapply"} {
					t.Run(fmt.Sprintf("skill%d/rank%d/kind%d/%s", id, rank, kind, mode), func(t *testing.T) {
						s := newCombatScene(t)
						s.skills, s.statuses, s.statusOverlay = skills, statuses, overlay
						src := putMonster(s, 800, 100000, domain.Stats{MAtk: 10, EffectHitBP: 1000000}, 100, 100)
						dst := scopedSkillTarget(t, s, kind)
						beforeHit, beforeDef := dst.Stats.Hit, dst.Stats.Def
						if mode == "immune" {
							dst.Status.Apply(domain.StatusDef{ID: 99999, DurationSec: 300, BlockHarmful: true}, s.tick, 0)
						}
						if mode == "fatal" {
							dst.HP = 1
							dst.Stats.MDef = 0
							dst.Stats.MagicResist = 0
						}
						hp := dst.HP
						if !s.applySkill(src, dst, def, 0, false) || dst.HP >= hp {
							t.Fatalf("native damage missing: HP %d -> %d", hp, dst.HP)
						}
						if mode == "fatal" || mode == "immune" {
							if dst.Status != nil && dst.Status.Has(app.ID) {
								t.Fatal("status applied despite fatal/immune gate")
							}
							return
						}
						var active domain.Active
						dst.Status.Each(func(a domain.Active) {
							if a.Def.ID == app.ID {
								active = a
							}
						})
						wantDuration := int32(60)
						wantIcon := int32(7074)
						if kind == domain.KindPlayer {
							wantDuration = 8
						}
						if id == 10326 {
							wantDuration = 5 + rank
							wantIcon = 7076
							if kind == domain.KindPlayer {
								wantDuration = 3
							}
						}
						if active.Def.ID != app.ID || active.Def.Level != app.Level || active.Def.DurationSec != wantDuration {
							t.Fatalf("wrong scoped state: %+v", active.Def)
						}
						if !active.Def.IconKnown || active.Def.Icon != wantIcon {
							t.Fatalf("status icon lost: %+v", active.Def)
						}
						if id == 10317 {
							pct := 5 + rank*5
							if kind == domain.KindPlayer {
								pct = 8 + rank*2
							}
							if want := beforeHit - int32(int64(beforeHit)*int64(pct)/100); dst.Stats.Hit != want {
								t.Fatalf("blind not consumed in stats bucket: hit %d -> %d want%d", beforeHit, dst.Stats.Hit, want)
							}
						} else {
							if canMove(dst) || !canAct(dst) || !canCast(dst) || dst.Stats.Def != beforeDef {
								t.Fatalf("root inherited stun/defense penalty: %+v", dst.Stats)
							}
							old := dst.Pos
							next := old
							next.X += 10
							s.moveEntity(dst, next)
							if dst.Pos != old {
								t.Fatal("root did not block actual movement")
							}
						}
						visible := s.visibleStatuses(dst)
						if len(visible) != 1 || visible[0].icon.Icon != wantIcon || visible[0].effect.Effect != active.Def.Name || visible[0].buff.SkillID != int32(app.ID) {
							t.Fatalf("native wire presentation lost: %+v", visible)
						}
						if mode == "reapply" {
							s.applySkill(src, dst, def, 0, false)
							if dst.Status.Count() != 1 {
								t.Fatal("reapply stacked mutually-exclusive target branches")
							}
							if id == 10317 && dst.Stats.Hit != beforeHit-int32(int64(beforeHit)*int64(5+rank*5)/100) && kind == domain.KindMonster {
								t.Fatal("reapply compounded blind")
							}
						}
						if mode == "expire" {
							s.tick = active.ExpireAt
							s.stepStatus()
							if dst.Status.Has(app.ID) || dst.Stats.Hit != beforeHit || dst.Stats.Def != beforeDef || !canMove(dst) {
								t.Fatal("status removal did not restore base attributes/movement")
							}
						}
						lines := s.skillOriginalEffectLines(def)
						if len(lines) < 2 || lines[0] != def.Description || !strings.Contains(lines[1], "非玩家") || !strings.Contains(lines[1], "玩家") {
							t.Fatalf("full original text/scoped detail lost: %v", lines)
						}
					})
				}
			}
		}
	}
}

func TestSkillMechanismsScopedPostgresSingleChanceAndAdmission(t *testing.T) {
	skills, statuses, overlay := scopedSkillPostgresFixture(t)
	for _, id := range []domain.SkillID{10317, 10326} {
		for _, kind := range []domain.EntityKind{domain.KindMonster, domain.KindPlayer} {
			s := newCombatScene(t)
			s.skills, s.statuses, s.statusOverlay = skills, statuses, overlay
			src := putMonster(s, 800, 100000, domain.Stats{}, 100, 100)
			dst := scopedSkillTarget(t, s, kind)
			def, _ := skills.Get(id, 1)
			app := scopedSkillApplication(t, def, kind)
			base := int32(10000)
			if id == 10326 {
				base = 3000
				if kind == domain.KindPlayer {
					base = 1000
				}
			}
			if app.ChanceBP != base || !app.ChanceExplicit {
				t.Fatal("authored base chance changed", app)
			}
			// Isolate probability draws from the preceding damage RNG. The real native
			// damage + state chain is exercised above; these are the loaded applications.
			def.Effect = domain.SkillEffect{}
			def.Effects = nil
			expected := rand.New(rand.NewSource(884))
			s.rng = rand.New(rand.NewSource(884))
			for i := 0; i < 100; i++ {
				dst.Stats.StatusResist = 100
				want := expected.Float64() < float64(base)/20000
				s.applySkill(src, dst, def, 0, false)
				got := dst.Status != nil && dst.Status.Has(app.ID)
				if got != want {
					t.Fatalf("skill%d kind%d draw%d doubled/mis-scoped chance got%v want%v", id, kind, i, got, want)
				}
				if got {
					dst.Status.Remove(app.ID)
					s.refreshEntityStats(dst)
				}
			}
			if s.rng.Float64() != expected.Float64() {
				t.Fatal("extra state probability draws")
			}
		}
	}
	s := newCombatScene(t)
	s.skills = skills
	join(t, s, 1, 1, "caster", 100, 100)
	join(t, s, 2, 2, "other", 120, 100)
	for _, id := range []domain.SkillID{10317, 10326} {
		def, _ := skills.Get(id, 1)
		if s.validTarget(s.players[1], s.players[2], def) {
			t.Fatal("status config enabled unapproved PvP")
		}
	}
}

func TestSkillMechanismsScopedPostgresContinuationAndOriginalDamage(t *testing.T) {
	skills, statuses, _ := scopedSkillPostgresFixture(t)
	raw, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var originals []domain.SkillDef
	if err := json.Unmarshal(raw, &originals); err != nil {
		t.Fatal(err)
	}
	for _, native := range originals {
		if native.ID != 10317 && native.ID != 10326 {
			continue
		}
		actual, ok := skills.Get(native.ID, native.Level)
		if !ok {
			t.Fatal("missing original rank")
		}
		if actual.Description != native.Description || !reflect.DeepEqual(actual.DirectEffects(), native.DirectEffects()) || actual.HurtType != native.HurtType || actual.Dist != native.Dist || actual.MPCost != native.MPCost || actual.PrepareMS != native.PrepareMS || actual.CooldownMS != native.CooldownMS {
			t.Fatalf("original authored damage/text/cost/timing changed %d/%d", native.ID, native.Level)
		}
	}
	for _, id := range []domain.SkillID{10317, 10326} {
		for _, rank := range []int32{6, 10, 11, 20, 100, 1000} {
			def, ok := skills.Get(id, rank)
			if !ok {
				t.Fatal("extended definition missing", id, rank)
			}
			for _, kind := range []domain.EntityKind{domain.KindMonster, domain.KindPlayer} {
				app := scopedSkillApplication(t, def, kind)
				resolved, ok := statuses.ResolveSkillApplication(app)
				if !ok {
					t.Fatal("extended status missing", app)
				}
				wantChance := int32(10000)
				if id == 10326 {
					wantChance = 3000
					if kind == domain.KindPlayer {
						wantChance = 1000
					}
				}
				if app.ChanceBP != wantChance {
					t.Fatal("continuation raised base chance", app)
				}
				if id == 10317 && resolved.DurationSec != map[domain.EntityKind]int32{domain.KindMonster: 60, domain.KindPlayer: 8}[kind] {
					t.Fatal("blind duration changed", rank, resolved)
				}
				if id == 10326 {
					want := min(int32(15), 5+rank)
					if kind == domain.KindPlayer {
						want = 3
					}
					if resolved.DurationSec != want || !resolved.BlockMove || resolved.Control != domain.ControlNone || len(resolved.Affixes) != 0 {
						t.Fatalf("root continuation changed semantics: %+v", resolved)
					}
				}
				if rank == 100 && resolved.EffectHitBP <= 0 {
					t.Fatal("capped duration lost effect-hit continuation")
				}
			}
			def.Statuses[0].TargetType = domain.SkillStatusTargetPlayer
			again, _ := skills.Get(id, rank)
			if again.Statuses[0].TargetType != domain.SkillStatusTargetNonPlayer {
				t.Fatal("resolved clone mutated source target scope")
			}
		}
	}
}

func TestSkillMechanismsScopedPostgresOverlayAndRankLoss(t *testing.T) {
	skills, statuses, overlay := scopedSkillPostgresFixture(t)
	s := newCombatScene(t)
	s.skills, s.statuses, s.statusOverlay = skills, statuses, overlay
	nativeStrong, _ := statuses.Get(1008, 6)
	nativeWeak, _ := statuses.Get(1008, 3)
	customStrong, _ := statuses.Get(1008, 25)
	equalNative, _ := statuses.Get(1008, 2)
	customEqual, _ := statuses.Get(1008, 21)
	for _, tc := range []struct {
		old, next domain.StatusDef
		blocked   bool
	}{{nativeStrong, customStrong, true}, {nativeWeak, customStrong, false}, {equalNative, customEqual, false}, {customStrong, nativeStrong, false}} {
		if tc.next.Icon <= 0 || !tc.next.IconKnown {
			t.Fatal("loaded status icon missing")
		}
		set := domain.NewStatusSet()
		set.ApplyWithOverlay(tc.old, 0, 1, overlay)
		got := set.ApplyWithOverlay(tc.next, 1, 2, overlay)
		if got.Blocked != tc.blocked {
			t.Fatalf("actual SQL priority mismatch old=%+v next=%+v result=%+v", tc.old, tc.next, got)
		}
	}
	for _, id := range []domain.SkillID{10317, 10326} {
		for _, kind := range []domain.EntityKind{domain.KindMonster, domain.KindPlayer} {
			s := newCombatScene(t)
			s.skills, s.statuses, s.statusOverlay = skills, statuses, overlay
			join(t, s, 1, 1, "source", 100, 100)
			src := s.players[1]
			src.Player.Char.Race, src.Player.Char.Level = domain.Healer, 100
			src.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
			src.Player.Char.Skills = domain.Learned{id: 5}
			s.refreshStats(src)
			src.Stats.EffectHitBP = 1000000
			dst := scopedSkillTarget(t, s, kind)
			def, _ := skills.Get(id, 5)
			def.SnapshotRawRanks = s.rawSkillLevels(src)
			def.SnapshotHardRanks = src.Player.Char.Skills.Clone()
			app := scopedSkillApplication(t, def, kind)
			state, ok := s.resolveSkillStatusDef(def, app, 0)
			if !ok || !s.applyStatusDef(dst, state, src.ID) {
				t.Fatal("cast scoped state failed")
			}
			var old domain.Active
			dst.Status.Each(func(a domain.Active) {
				if a.Def.ID == app.ID {
					old = a
				}
			})
			src.Player.Char.Skills[id] = 1
			s.refreshStats(src)
			var next domain.Active
			dst.Status.Each(func(a domain.Active) {
				if a.Def.ID == app.ID {
					next = a
				}
			})
			want, _ := skills.Get(id, 1)
			wantApp := scopedSkillApplication(t, want, kind)
			if next.Def.ID != app.ID || next.Def.Level != wantApp.Level || next.Def.SkillSnapshotRank != 1 {
				t.Fatalf("rank loss changed target branch: kind%d skill%d %+v", kind, id, next.Def)
			}
			if next.ExpireAt > old.ExpireAt {
				t.Fatal("rank loss extended status")
			}
			if kind == domain.KindPlayer && ((id == 10317 && next.Def.DurationSec != 8) || (id == 10326 && next.Def.DurationSec != 3)) {
				t.Fatal("player state became non-player duration")
			}
			wire, err := json.Marshal(s.buffSnapshot(dst))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(wire), "OverlayStrength") || strings.Contains(string(wire), "TargetType") || strings.Contains(string(wire), "LegacyOverlayLevel") {
				t.Fatal("internal configuration escaped into existing protocol")
			}
			src.Player.Char.Skills[id] = 5
			s.refreshStats(src)
			dst.Status.Each(func(a domain.Active) {
				if a.Def.ID == app.ID && a.Def.SkillSnapshotRank != 1 {
					t.Fatal("re-equipping upgraded old scoped state")
				}
			})
		}
	}
}

func TestSkillMechanismsScopedPostgresFinalStrengthCrossover(t *testing.T) {
	skills, statuses, overlay := scopedSkillPostgresFixture(t)
	resolve := func(rank int32, kind domain.EntityKind) domain.StatusDef {
		def, ok := skills.Get(10317, rank)
		if !ok {
			t.Fatal(rank)
		}
		state, ok := statuses.ResolveSkillApplication(scopedSkillApplication(t, def, kind))
		if !ok {
			t.Fatal("resolve", rank)
		}
		return state
	}
	for _, tc := range []struct {
		name               string
		oldRank, newRank   int32
		oldKind, newKind   domain.EntityKind
		oldValue, newValue int64
		blocked            bool
	}{
		{"pvp12 beats native5", 5, 12, domain.KindMonster, domain.KindPlayer, 30, 32, false},
		{"native5 cannot replace pvp12", 12, 5, domain.KindPlayer, domain.KindMonster, 32, 30, true},
		{"pve12 beats pvp20", 20, 12, domain.KindPlayer, domain.KindMonster, 48, 65, false},
		{"pvp20 cannot replace pve12", 12, 20, domain.KindMonster, domain.KindPlayer, 65, 48, true},
		{"pve cap equals pvp cap", 19, 46, domain.KindMonster, domain.KindPlayer, 100, 100, false},
		{"pvp cap equals pve cap", 46, 19, domain.KindPlayer, domain.KindMonster, 100, 100, false},
		{"high pvp cap remains equal", 1000, 1000, domain.KindMonster, domain.KindPlayer, 100, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, next := resolve(tc.oldRank, tc.oldKind), resolve(tc.newRank, tc.newKind)
			a, va := old.OverlayPriority()
			b, vb := next.OverlayPriority()
			if !va || !vb || a != tc.oldValue || b != tc.newValue {
				t.Fatalf("final strength %d/%d want%d/%d", a, b, tc.oldValue, tc.newValue)
			}
			set := domain.NewStatusSet()
			set.ApplyWithOverlay(old, 0, 1, overlay)
			result := set.ApplyWithOverlay(next, 1, 2, overlay)
			if result.Blocked != tc.blocked || result.Applied == tc.blocked {
				t.Fatalf("cross rank outcome %+v", result)
			}
			var active domain.Active
			set.Each(func(v domain.Active) { active = v })
			if (tc.blocked && active.Source != 1) || (!tc.blocked && active.Source != 2) {
				t.Fatal("strong/equal replacement failed")
			}
		})
	}
	// Final same-ID strength does not leak into unrelated matrix level rules.
	for _, id := range []domain.StatusID{1000, 1013} {
		if rule, ok := overlay.Rule(1008, id); !ok || rule != domain.OverlayReplace {
			t.Fatal("actual cross-ID fixture changed", id, rule)
		}
		for level := int32(1); level <= 7; level++ {
			other, ok := statuses.Get(id, level)
			if !ok {
				if id != 1000 {
					t.Fatal("native cross-ID state missing", id, level)
				}
				// 1000 is a matrix-reserved coordinate with no native status definitions.
				other = domain.StatusDef{ID: id, Level: level, DurationSec: 10}
			}
			for _, rank := range []int32{1, 2, 3, 4, 5, 100} {
				custom := resolve(rank, domain.KindPlayer)
				wantLogical := int64(20+4*(min(rank, 5)-1)) * 1000
				if custom.LegacyOverlayLevelBP != wantLogical {
					t.Fatal("logical alias changed with rank", rank)
				}
				set := domain.NewStatusSet()
				set.ApplyWithOverlay(custom, 0, 1, overlay)
				wantBlock := wantLogical > int64(level)*10000
				if got := set.ApplyWithOverlay(other, 1, 2, overlay); got.Blocked != wantBlock {
					t.Fatalf("cross-ID %d/%d vs rank%d %+v", id, level, rank, got)
				}
			}
			// Equal -10% native and custom blindness must give the same result in
			// both actual matrix directions, including block/coexist/mustreplace.
			native, _ := statuses.Get(1008, 2)
			custom, _ := statuses.Get(1008, 21)
			for _, reverse := range []bool{false, true} {
				results := []domain.StatusApplyResult{}
				for _, blind := range []domain.StatusDef{native, custom} {
					old, next := blind, other
					if reverse {
						old, next = other, blind
					}
					set := domain.NewStatusSet()
					set.ApplyWithOverlay(old, 0, 1, overlay)
					results = append(results, set.ApplyWithOverlay(next, 1, 2, overlay))
				}
				if !reflect.DeepEqual(results[0], results[1]) {
					t.Fatal("storage identity changed actual cross-ID rule", id, level, reverse, results)
				}
			}
		}
	}
	for key, rule := range overlay {
		if (key.Existing == 1034 || key.Incoming == 1034) && rule == domain.OverlayReplace {
			t.Fatal("root needs reviewed logical-level mapping", key)
		}
	}
}

func TestSkillMechanismsScopedPostgresOldJSONRestoreStrength(t *testing.T) {
	skills, statuses, overlay := scopedSkillPostgresFixture(t)
	blind, _ := statuses.Get(1008, 6)
	// A persisted grown state must keep its own strength rather than the base row.
	blind.Affixes = append([]domain.Affix(nil), blind.Affixes...)
	for i := range blind.Affixes {
		if blind.Affixes[i].Attr == domain.AttrHit {
			blind.Affixes[i].Value = -47
		}
	}
	burn, _ := statuses.Get(1036, 1)
	wall := time.Now()
	saved := []domain.SavedStatus{
		{Def: blind, RemainingTicks: 600, ExpiresAtUnixMS: wall.Add(time.Minute).UnixMilli()},
		{Def: burn, RemainingTicks: 600, ExpiresAtUnixMS: wall.Add(time.Minute).UnixMilli(), NextTickIn: 10, PeriodicDamageRemainder100: 37},
	}
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "OverlayStrength") || strings.Contains(string(raw), "LegacyOverlayLevel") {
		t.Fatal("rule persisted into status snapshot")
	}
	var oldJSON []domain.SavedStatus
	if err = json.Unmarshal(raw, &oldJSON); err != nil {
		t.Fatal(err)
	}
	if oldJSON[0].Def.OverlayStrength != nil {
		t.Fatal("old JSON already has rule")
	}
	s := newCombatScene(t)
	s.skills, s.statuses, s.statusOverlay = skills, statuses, overlay
	s.exec(Enter{ID: 1, Char: &domain.Character{ID: 1, Name: "saved", Level: 50, Pos: domain.Pos{MapID: 7, X: 100, Y: 100}, SavedStatuses: oldJSON}, Sink: &fakeSink{}})
	p := s.players[1]
	if p == nil {
		t.Fatal("restored entry failed")
	}
	var actual domain.Active
	foundBurn := false
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == 1008 {
			actual = a
		}
		if a.Def.ID == 1036 {
			foundBurn = true
			if a.PeriodicDamageRemainder100 != 37 || !reflect.DeepEqual(a.Def.Affixes, burn.Affixes) {
				t.Fatal("metadata update changed DOT remainder/damage")
			}
		}
	})
	if !foundBurn || actual.Def.OverlayStrength == nil || !reflect.DeepEqual(actual.Def.Affixes, blind.Affixes) || actual.ExpireAt > s.tick+600 {
		t.Fatal("old snapshot not faithfully enriched", actual)
	}
	stripped := actual.Def
	stripped.OverlayStrength = nil
	stripped.LegacyOverlayLevelBP = 0
	if !reflect.DeepEqual(stripped, oldJSON[0].Def) {
		t.Fatal("metadata enrichment changed saved combat definition")
	}
	for _, rank := range []int32{12, 20} {
		def, _ := skills.Get(10317, rank)
		next, ok := statuses.ResolveSkillApplication(scopedSkillApplication(t, def, domain.KindPlayer))
		if !ok {
			t.Fatal(rank)
		}
		if p.Status.BlockedByOverlay(next, overlay) != (rank == 12) {
			t.Fatal("old saved strength compared on wrong axis", rank)
		}
	}
	def, _ := skills.Get(10317, 20)
	next, _ := statuses.ResolveSkillApplication(scopedSkillApplication(t, def, domain.KindPlayer))
	if got := p.Status.ApplyWithOverlay(next, s.tick, 2, overlay); !got.Applied {
		t.Fatal("stronger new state could not replace old JSON snapshot", got)
	}
}
