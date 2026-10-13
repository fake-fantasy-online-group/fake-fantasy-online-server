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
	"github.com/jackc/pgx/v5"
)

// Reconstruct the exact pre73 loader view without mutating the test database.
type backstabBeforeQuerier struct{ data.Querier }

func (q backstabBeforeQuerier) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	query = strings.ReplaceAll(query, "FROM game_skill_passive_trigger_statuses p", "FROM (SELECT * FROM game_skill_passive_trigger_statuses WHERE passive_skill_id<>10201) p")
	query = strings.ReplaceAll(query, "COALESCE(sd.desc_,'')", "COALESCE(CASE WHEN sd.skill_id=10201 THEN format('背刺时20%%的几率不会中止隐身状态，%s%%几率击晕对手。',sd.skill_level*5) ELSE sd.desc_ END,'')")
	return q.Querier.Query(ctx, query, args...)
}

func backstabLoadedCaster(t *testing.T, s *Scene, rank int32) *entity.Entity {
	t.Helper()
	join(t, s, 1, 1, "backstab", 100, 100)
	p := s.players[1]
	p.Player.Char.Race, p.Player.Char.Level = domain.Assassin, 100
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Skills = domain.Learned{10209: 1}
	if rank > 0 {
		p.Player.Char.Skills[10201] = min(rank, 20)
	}
	p.Player.Char.Base = domain.Base{STR: 100, VIT: 100, SPI: 100, DEX: 100, AGI: 100}
	s.refreshStats(p)
	p.HP, p.MP = p.MaxHP, p.MaxMP
	p.Stats = sureHit
	return p
}

func backstabLoadedActive(t *testing.T, s *Scene, p *entity.Entity, rank int32) domain.SkillDef {
	t.Helper()
	def, ok := s.skills.Get(10209, 1)
	if !ok {
		t.Fatal("missing original Backstab")
	}
	levels := domain.Learned{10209: 1}
	if rank > 0 {
		levels[10201] = rank
	}
	s.applyPassiveSkillModifiersWithLevels(p, &def, levels)
	return def
}

func backstabActive(target *entity.Entity) (out domain.Active, found bool) {
	if target.Status != nil {
		target.Status.Each(func(a domain.Active) {
			if a.Def.ID == domain.StatusStun {
				out, found = a, true
			}
		})
	}
	return
}

func TestBackstabStunPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_BACKSTAB_STUN_PG_DATA")
	if path == "" {
		t.Skip("explicit owned idle migration73 PostgreSQL data directory required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active/stale-postmaster cluster")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	ctx := context.Background()
	skills, _, err := data.LoadSkills(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := data.LoadSkills(ctx, backstabBeforeQuerier{q})
	if err != nil {
		t.Fatal(err)
	}
	states, _, err := data.LoadStatuses(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	overlay, _, err := data.LoadStatusOverlay(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, states, overlay); err != nil {
		t.Fatal(err)
	}
	scene := func() *Scene {
		s := newCombatScene(t)
		s.skills, s.statuses, s.statusOverlay = skills, states, overlay
		return s
	}
	ranks := []int32{1, 2, 3, 4, 5, 6, 10, 11, 20, 100, 1000}

	t.Run("only-approved-passive-and-description-delta", func(t *testing.T) {
		for _, id := range skills.ProgressionCatalog().IDs() {
			for _, rank := range ranks {
				got, ok := skills.Get(id, rank)
				old, oldOK := before.Get(id, rank)
				if !ok || !oldOK {
					t.Fatal(id, rank)
				}
				if id == 10201 {
					if len(got.PassiveTriggers) != 1 {
						t.Fatal("stun trigger missing", rank)
					}
					x := got.PassiveTriggers[0]
					if x.TargetSkill != 10209 || x.Status != 1002 || x.StatusLevel != 27 || x.ChanceBP != 10000 || !x.ChanceExplicit || x.DurationSec != 3 {
						t.Fatal("wrong approved configuration", x)
					}
					got.PassiveTriggers, got.Description = old.PassiveTriggers, old.Description
				}
				if !reflect.DeepEqual(got, old) {
					t.Fatalf("unapproved skill delta %d/%d", id, rank)
				}
			}
		}
	})
	t.Run("native-pure-stun-and-runtime-description", func(t *testing.T) {
		state, ok := states.Get(1002, 27)
		if !ok || state.DurationSec != 3 || state.Control != domain.ControlStun || state.Immobile != 1 || state.IntervalSec != 0 || len(state.Affixes) != 0 || len(state.HPChanges) != 0 || len(state.MPChanges) != 0 || state.Icon != 7055 || !state.IconKnown {
			t.Fatal("wrong native stun", state)
		}
		s := scene()
		for _, rank := range ranks {
			def, _ := skills.Get(10201, rank)
			want := "背刺时20%的几率不会中止隐身状态，100%几率击晕对手3秒。"
			lines := s.skillOriginalEffectLines(def)
			if len(lines) == 0 || lines[0] != want {
				t.Fatal("approved description mismatch", rank, lines)
			}
			resolved, ok := s.skillDescriptionTrigger(def, 0)
			if !ok || resolved.DurationSec != 3 || resolved.ChanceBP != 10000 || resolved.EffectHitBP != max(int32(0), rank-5)*500 {
				t.Fatal("fixed duration/local accuracy", rank, resolved)
			}
			if rank > 5 && !strings.Contains(strings.Join(lines, " "), "效果命中") {
				t.Fatal("high rank accuracy absent", rank, lines)
			}
		}
		odd, _ := skills.Get(10201, 6)
		odd.Description = "管理文案只有17。"
		if s.skillDescriptionForDisplay(odd, odd) != odd.Description {
			t.Fatal("unknown template rewritten")
		}
	})
	for _, rank := range ranks {
		for _, mode := range []string{"hit", "fatal", "dead", "immune", "skill-immune", "framework-miss", "expire", "reapply", "overlay-block"} {
			t.Run(fmt.Sprintf("rank%d/%s", rank, mode), func(t *testing.T) {
				s := scene()
				p := backstabLoadedCaster(t, s, rank)
				def := backstabLoadedActive(t, s, p, rank)
				if !def.DirectEffects()[0].GuaranteedHit {
					t.Fatal("native Backstab lost guaranteed damage hit")
				}
				dst := putMonster(s, 801, 100000, domain.Stats{Def: 100, MoveSpeed: 100}, 120, 100)
				dst.Status = domain.NewStatusSet()
				s.defs = spawn.MapDefs{338: {ID: 338, Kind: domain.MonsterNormal, Stats: dst.Stats}}
				switch mode {
				case "fatal":
					dst.HP = 1
				case "dead":
					dst.HP = 0
				case "immune":
					dst.Status.Apply(domain.StatusDef{ID: 1022, DurationSec: 30, BlockHarmful: true}, s.tick, 0)
				case "skill-immune":
					s.defs = spawn.MapDefs{338: {ID: 338, Kind: domain.MonsterNormal, ImmuneSkills: map[domain.SkillID]struct{}{10209: {}}}}
				case "framework-miss":
					// Native Backstab guarantees its damage hit. Only this cloned negative
					// control disables that carrier to test the generic preceding-hit gate.
					def.Effects = append([]domain.SkillEffect(nil), def.DirectEffects()...)
					def.Effects[0].GuaranteedHit = false
					def.Effect = def.Effects[0]
					p.Stats.Hit = 0
					dst.Stats.Def = 1 << 20
				case "overlay-block":
					dst.Status.Apply(domain.StatusDef{ID: 99991, DurationSec: 30}, s.tick, 0)
					s.statusOverlay = domain.StatusOverlay{{Existing: 99991, Incoming: 1002}: domain.OverlayBlock}
				}
				hp := dst.HP
				s.applySkill(p, dst, def, 0, false)
				active, found := backstabActive(dst)
				blocked := mode == "fatal" || mode == "dead" || mode == "immune" || mode == "skill-immune" || mode == "framework-miss" || mode == "overlay-block"
				if found == blocked {
					t.Fatal("stun gate", mode, found)
				}
				if mode == "dead" || mode == "framework-miss" || mode == "skill-immune" {
					if dst.HP != hp {
						t.Fatal("blocked damage changed HP")
					}
				} else if dst.HP >= hp {
					t.Fatal("native damage missing")
				}
				if blocked {
					return
				}
				if active.Def.DurationSec != 3 || active.Def.EffectHitBP != max(int32(0), rank-5)*500 || canAct(dst) || canCast(dst) || canMove(dst) || dst.Stats.Def != 100 {
					t.Fatal("stun payload changed", active.Def, dst.Stats)
				}
				visible := s.visibleStatuses(dst)
				if len(visible) != 1 || visible[0].icon.Icon != 7055 || visible[0].buff.SkillID != 1002 || visible[0].effect.Effect != stateName(states, 1002, 27) {
					t.Fatal("native stun presentation", visible)
				}
				if mode == "reapply" {
					s.applySkill(p, dst, def, 0, false)
					if dst.Status.Count() != 1 {
						t.Fatal("stun stacked")
					}
				}
				if mode == "expire" {
					s.tick = active.ExpireAt
					s.stepStatus()
					if dst.Status.Has(1002) || !canAct(dst) || !canCast(dst) || !canMove(dst) {
						t.Fatal("stun did not expire")
					}
				}
			})
		}
	}
	t.Run("preserve-stealth-and-stun-independent", func(t *testing.T) {
		kept, broken := 0, 0
		for seed := int64(0); seed < 100; seed++ {
			s := scene()
			p := backstabLoadedCaster(t, s, 1)
			def := backstabLoadedActive(t, s, p, 1)
			stealth, _ := states.Get(domain.StatusStealth, 1)
			p.Status = domain.NewStatusSet()
			p.Status.Apply(stealth, s.tick, p.ID)
			s.rng = rand.New(rand.NewSource(seed))
			oracle := rand.New(rand.NewSource(seed))
			wantKeep := oracle.Intn(10000) < 2000
			s.prepareSkillStatuses(p, &def)
			if entityInvisible(p) != wantKeep {
				t.Fatal("20% preservation changed", seed)
			}
			if wantKeep {
				kept++
			} else {
				broken++
			}
			p.Stats = sureHit
			dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
			s.applySkill(p, dst, def, 0, false)
			if _, ok := backstabActive(dst); !ok {
				t.Fatal("stun wrongly requires reveal", seed, wantKeep)
			}
		}
		if kept == 0 || broken == 0 {
			t.Fatal("both preservation outcomes not exercised")
		}
	})
	t.Run("one-effect-contest-and-local-accuracy-growth", func(t *testing.T) {
		for _, rank := range []int32{1, 6, 20} {
			s := scene()
			p := backstabLoadedCaster(t, s, rank)
			def := backstabLoadedActive(t, s, p, rank)
			// Damage is tested above. Isolate its loaded state to account for every
			// RNG draw in the common base-probability/effect-hit/resistance contest.
			def.Effect = domain.SkillEffect{}
			def.Effects = nil
			dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
			oracle := rand.New(rand.NewSource(20261013))
			s.rng = rand.New(rand.NewSource(20261013))
			probability := domain.EffectSuccessProbability(10000, int64(max(int32(0), rank-5))*500, 10000)
			for n := 0; n < 200; n++ {
				dst.Status = domain.NewStatusSet()
				dst.Stats.StatusResist = 100
				want := oracle.Float64() < probability
				s.applySkill(p, dst, def, 0, false)
				_, got := backstabActive(dst)
				if got != want {
					t.Fatal("extra/missing effect roll", rank, n, got, want)
				}
			}
			if oracle.Float64() != s.rng.Float64() {
				t.Fatal("state contest consumed extra RNG", rank)
			}
		}
	})
	t.Run("unlearned-soft-ranks-and-other-skill-scope", func(t *testing.T) {
		s := scene()
		p := backstabLoadedCaster(t, s, 0)
		_, gear := growthAllSkillGear(s)
		p.Player.Worn.Set(domain.SlotGlove, gear)
		s.refreshStats(p)
		if s.rawSkillLevels(p)[10201] != 0 {
			t.Fatal("equipment learned passive")
		}
		def, _ := skills.Get(10209, 1)
		s.applyPassiveSkillModifiers(p, &def)
		if len(def.Statuses) != 0 {
			t.Fatal("unlearned passive attached stun")
		}
		p.Player.Char.Skills[10201] = 5
		s.refreshStats(p)
		if s.rawSkillLevels(p)[10201] != 6 {
			t.Fatal("equipped raw level did not rise")
		}
		def, _ = skills.Get(10209, 1)
		s.applyPassiveSkillModifiers(p, &def)
		resolved, ok := s.resolveSkillStatusDef(def, def.Statuses[0], 0)
		if !ok || resolved.DurationSec != 3 || resolved.EffectHitBP != 500 || p.Player.Char.Skills[10201] != 5 {
			t.Fatal("soft level altered base duration/hard points", resolved)
		}
		p.Player.Worn.Set(domain.SlotGlove, domain.Stack{})
		s.refreshStats(p)
		def, _ = skills.Get(10209, 1)
		s.applyPassiveSkillModifiers(p, &def)
		resolved, _ = s.resolveSkillStatusDef(def, def.Statuses[0], 0)
		if resolved.EffectHitBP != 0 || resolved.DurationSec != 3 {
			t.Fatal("unequip retained added accuracy")
		}
		other, _ := skills.Get(10205, 1)
		old := other
		s.applyPassiveSkillModifiers(p, &other)
		if !reflect.DeepEqual(other.Statuses, old.Statuses) {
			t.Fatal("passive leaked to other attack")
		}
	})
	t.Run("foreign-source-stun-cannot-resurrect-after-relogin", func(t *testing.T) {
		s := scene()
		p := backstabLoadedCaster(t, s, 20)
		def := backstabLoadedActive(t, s, p, 20)
		dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
		s.applySkill(p, dst, def, 0, false)
		wall := time.Unix(1800000000, 0)
		saved := dst.Status.SaveForLogout(s.tick, wall, dst.ID)
		if len(saved) != 0 {
			t.Fatal("external skill snapshot unexpectedly persisted", saved)
		}
		active, ok := backstabActive(dst)
		if !ok || active.Def.DurationSec != 3 || active.Def.EffectHitBP != 7500 {
			t.Fatal("missing actual pre-logout stun", active)
		}
		// The existing persistence contract rejects foreign-source skill
		// snapshots on both write and restore; a stale serialized row cannot
		// restore one after its source entity/snapshot has disappeared.
		blob, err := json.Marshal([]domain.SavedStatus{{Def: active.Def, RemainingTicks: int64(domain.Ticks(3000)), ExpiresAtUnixMS: wall.Add(3 * time.Second).UnixMilli()}})
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip []domain.SavedStatus
		if err = json.Unmarshal(blob, &roundtrip); err != nil {
			t.Fatal(err)
		}
		for _, offline := range []time.Duration{0, time.Second, 3 * time.Second} {
			if domain.RestoreSavedStatuses(roundtrip, s.tick, wall.Add(offline), dst.ID).Has(1002) {
				t.Fatal("foreign-source saved stun revived", offline)
			}
		}
	})
	t.Run("native-use-skill-reaches-cast-settlement", func(t *testing.T) {
		s := scene()
		p := backstabLoadedCaster(t, s, 5)
		stealth, _ := states.Get(domain.StatusStealth, 1)
		p.Status = domain.NewStatusSet()
		p.Status.Apply(stealth, s.tick, p.ID)
		dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
		dst.Monster.Kind = domain.MonsterNormal
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10209, Target: dst.ID})
		if len(s.casts) != 1 {
			t.Fatal("native Backstab not accepted", len(s.casts), p.MP)
		}
		// Retain the accepted definition and targets; only advance the test clock
		// past the existing visual impact delay rather than sleeping in a unit test.
		now := time.Now().Add(time.Hour)
		s.now = func() time.Time { return now }
		s.stepCasts()
		now = now.Add(time.Hour)
		s.stepCasts()
		if _, ok := backstabActive(dst); !ok || dst.HP >= dst.MaxHP {
			t.Fatal("native queued cast omitted damage/stun")
		}
	})
}

func stateName(states domain.StatusTable, id domain.StatusID, rank int32) string {
	s, _ := states.Get(id, rank)
	return s.Name
}
