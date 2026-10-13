package scene

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/jackc/pgx/v5"
)

// Replay the original loader fallback without changing any database rows.
// Only migration69's two explicit effects are hidden in this read-only view.
type gangqiBeforeQuerier struct{ data.Querier }

func (q gangqiBeforeQuerier) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	query = strings.ReplaceAll(query, "FROM game_skill_effects\n", "FROM (SELECT * FROM game_skill_effects WHERE skill_id<>10102) AS game_skill_effects\n")
	return q.Querier.Query(ctx, query, args...)
}

func TestGangqiKnockbackPostgres(t *testing.T) {
	path := os.Getenv("FANTASY_GANGQI_KNOCKBACK_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration69 PostgreSQL data directory required")
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
	before, _, err := data.LoadSkills(context.Background(), gangqiBeforeQuerier{q})
	if err != nil {
		t.Fatal(err)
	}

	ranks := []int32{1, 2, 3, 4, 5, 6, 10, 11, 20, 100, 1000}
	t.Run("only-knockback-payload-added", func(t *testing.T) {
		for _, id := range skills.ProgressionCatalog().IDs() {
			for _, rank := range ranks {
				got, ok := skills.Get(id, rank)
				old, oldOK := before.Get(id, rank)
				if !ok || !oldOK {
					t.Fatal("missing skill rank", id, rank)
				}
				if id == 10102 {
					effects, originals := got.DirectEffects(), old.DirectEffects()
					if len(effects) != 2 || len(originals) != 1 || effects[0].Kind != domain.EffectDamage || effects[1].Kind != domain.EffectKnockback {
						t.Fatalf("effect order %d: %+v", rank, effects)
					}
					damage := effects[0]
					damage.ChanceExplicit, damage.ChanceBP = originals[0].ChanceExplicit, originals[0].ChanceBP
					if !reflect.DeepEqual(damage, originals[0]) {
						t.Fatalf("original damage changed rank%d: %+v / %+v", rank, damage, originals[0])
					}
					donor, ok := skills.Get(10223, rank)
					if !ok || len(donor.DirectEffects()) != 2 || !reflect.DeepEqual(effects[1], donor.DirectEffects()[1]) {
						t.Fatalf("donor knockback mismatch rank%d", rank)
					}
					if effects[1].KnockbackDistance != 100 || effects[1].ChanceBP != 10000 || !effects[1].RequiresPreviousLanded {
						t.Fatalf("unexpected isolated fixture: %+v", effects[1])
					}
					got.Effect, got.Effects = old.Effect, old.Effects
				}
				if !reflect.DeepEqual(got, old) {
					t.Fatalf("unrelated payload changed skill%d/rank%d", id, rank)
				}
			}
		}
	})
	t.Run("full-original-description-and-detail-preserved", func(t *testing.T) {
		s, oldScene := newCombatScene(t), newCombatScene(t)
		s.skills, oldScene.skills = skills, before
		for _, rank := range ranks {
			got, _ := skills.Get(10102, rank)
			old, _ := before.Get(10102, rank)
			if !reflect.DeepEqual(s.skillOriginalEffectLines(got), oldScene.skillOriginalEffectLines(old)) {
				t.Fatalf("display changed rank%d", rank)
			}
			if rank <= 5 && got.Description != fmt.Sprintf("造成%d%%伤害，震退敌人。", 127+rank*3) {
				t.Fatal("native description shortened", rank, got.Description)
			}
		}
	})
	t.Run("backstab-duration-remains-uninvented", func(t *testing.T) {
		for _, rank := range ranks {
			def, _ := skills.Get(10201, rank)
			if len(def.PassiveTriggers) != 0 {
				t.Fatal("unconfirmed backstab stun activated", rank, def.PassiveTriggers)
			}
		}
	})
	for _, rank := range ranks {
		def, _ := skills.Get(10102, rank)
		for _, mode := range []string{"clear", "wall", "thin-wall", "adjacent-wall", "diagonal", "coincident", "miss", "fatal", "dead", "immune"} {
			t.Run(fmt.Sprintf("rank%d/%s", rank, mode), func(t *testing.T) {
				s := newCombatScene(t)
				s.skills = skills
				sink := join(t, s, 1, 1, "observer", 100, 100)
				src := putMonster(s, 800, 100000, sureHit, 100, 100)
				dst := putMonster(s, 801, 100000, domain.Stats{}, 120, 100)
				dst.Monster.CombatPathActive, dst.Monster.Roaming = true, true
				switch mode {
				case "diagonal":
					s.aoi.Move(dst, domain.Pos{MapID: 7, X: 120, Y: 120})
				case "coincident":
					s.aoi.Move(dst, src.Pos)
				case "miss":
					src.Stats.Hit = 0
					dst.Stats.Def = 1 << 20
				case "fatal":
					dst.HP, dst.MaxHP = 1, 1
				case "dead":
					dst.HP = 0
				case "immune":
					s.defs = spawn.MapDefs{dst.Monster.TypeID: {ID: dst.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{10102: {}}}}
				}
				from, hp := dst.Pos, dst.HP
				switch mode {
				case "wall":
					s.walkable = func(p domain.Pos) bool { return p.X <= from.X+30 }
				case "thin-wall":
					s.walkable = func(p domain.Pos) bool { return p.X < from.X+35 || p.X > from.X+40 }
				case "adjacent-wall":
					s.walkable = func(p domain.Pos) bool { return p.X <= from.X }
				}
				sink.take()
				landed := s.applySkill(src, dst, def, 0, false)
				s.flush()
				want := 100.0
				switch mode {
				case "wall", "thin-wall":
					want = 30
				case "adjacent-wall", "coincident", "miss", "fatal", "dead", "immune":
					want = 0
				}
				actual := math.Hypot(dst.Pos.X-from.X, dst.Pos.Y-from.Y)
				if math.Abs(actual-want) > 1e-9 || dst.Pos.MapID != from.MapID {
					t.Fatalf("displacement %.9f want %.9f; HP%d -> %d", actual, want, hp, dst.HP)
				}
				switch mode {
				case "miss", "dead", "immune":
					if landed || dst.HP != hp {
						t.Fatalf("blocked damage: landed=%v HP%d -> %d", landed, hp, dst.HP)
					}
				default:
					if !landed || dst.HP >= hp {
						t.Fatalf("damage missing: landed=%v HP%d -> %d", landed, hp, dst.HP)
					}
				}
				moved := 0
				for _, ev := range sink.take() {
					if v, ok := ev.(event.EntityMoved); ok && v.ID == dst.ID {
						moved++
						if !v.Snap || v.To != dst.Pos {
							t.Fatal("wrong existing move event", v)
						}
					}
				}
				if want > 0 {
					if moved != 1 || dst.Monster.CombatPathActive || dst.Monster.Roaming {
						t.Fatalf("knockback event/path reset: moved%d path=%v roaming=%v", moved, dst.Monster.CombatPathActive, dst.Monster.Roaming)
					}
				} else if moved != 0 {
					t.Fatal("unexpected movement event", moved)
				}
			})
		}
	}
	t.Run("fan-secondary-target-does-not-inherit-knockback", func(t *testing.T) {
		def, _ := skills.Get(10102, 5)
		s := newCombatScene(t)
		variant, ok := s.endgameCatalog.Variant("sword_gale_fan")
		if !ok {
			t.Fatal("missing native fan variant")
		}
		def.EquipmentVariant = &variant
		primary, secondary := variantTargetDef(def, 0), variantTargetDef(def, 1)
		for i, d := range []domain.SkillDef{primary, secondary} {
			scene := newCombatScene(t)
			src := putMonster(scene, 800, 100000, sureHit, 100, 100)
			dst := putMonster(scene, 801, 100000, domain.Stats{}, 120, 100)
			scene.applySkill(src, dst, d, 0, false)
			want := 120.0
			if i == 0 {
				want += 100
			}
			if dst.Pos.X != want || dst.HP >= dst.MaxHP {
				t.Fatal("fan target settlement", i, dst.Pos, dst.HP)
			}
		}
		if len(def.DirectEffects()) != 2 {
			t.Fatal("secondary variant mutated cached original")
		}
	})
	t.Run("does-not-enable-player-target-or-displacement", func(t *testing.T) {
		s := newCombatScene(t)
		join(t, s, 1, 1, "source", 100, 100)
		join(t, s, 2, 2, "target", 120, 100)
		src, dst := s.players[1], s.players[2]
		def, _ := skills.Get(10102, 5)
		if s.validTarget(src, dst, def) {
			t.Fatal("PvP unexpectedly enabled")
		}
		from := dst.Pos
		if s.knockbackMonster(src, dst, 100) || dst.Pos != from {
			t.Fatal("player displacement enabled")
		}
	})
}
