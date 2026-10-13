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
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/jackc/pgx/v5"
)

type beforeRandomPunishmentQuery struct{ data.Querier }

func (q beforeRandomPunishmentQuery) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	sql = strings.ReplaceAll(sql, "FROM game_skill_statuses s\n", "FROM game_skill_statuses s WHERE s.skill_id<>10313\n")
	return q.Querier.Query(ctx, sql, args...)
}

func TestRandomPunishmentPostgresRealSettlement(t *testing.T) {
	path := os.Getenv("FANTASY_RANDOM_PUNISHMENT_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration72 PostgreSQL directory required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active/stale-postmaster fixture")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	skills, _, err := data.LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := data.LoadSkills(context.Background(), beforeRandomPunishmentQuery{q})
	if err != nil {
		t.Fatal(err)
	}
	states, _, err := data.LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	overlay, _, err := data.LoadStatusOverlay(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateSkillStatusRuntimeV5(skills, states, overlay); err != nil {
		t.Fatal(err)
	}
	base := domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000, MDef: 50, Hit: 1000, Def: 100, MoveSpeed: 100}
	makeScene := func(seed int64, hit, resist int32) (*Scene, *entity.Entity, *entity.Entity) {
		s := newCombatScene(t)
		s.skills, s.statuses, s.statusOverlay = skills, states, overlay
		s.rng = rand.New(rand.NewSource(seed))
		src := putMonster(s, 800, 10000000, domain.Stats{MAtk: 1000, EffectHitBP: hit}, 100, 100)
		stats := base
		stats.StatusResist = resist
		dst := putMonster(s, 801, 10000000, stats, 120, 100)
		dst.Status = domain.NewStatusSet()
		s.defs = spawn.MapDefs{dst.Monster.TypeID: {ID: dst.Monster.TypeID, Stats: stats}}
		return s, src, dst
	}
	active := func(dst *entity.Entity) domain.Active {
		var got domain.Active
		dst.Status.Each(func(a domain.Active) {
			if a.Def.SkillSnapshotID == 10313 {
				got = a
			}
		})
		return got
	}
	t.Run("only-pool-changes-native-and-high-ranks", func(t *testing.T) {
		for _, id := range skills.ProgressionCatalog().IDs() {
			for _, rank := range []int32{1, 2, 3, 4, 5, 6, 10, 11, 20, 100, 1000} {
				got, ok := skills.Get(id, rank)
				if !ok {
					t.Fatal(id, rank)
				}
				old, ok := before.Get(id, rank)
				if !ok {
					t.Fatal("before missing", id, rank)
				}
				if id == 10313 {
					if got.Radius != 150 || got.Area.Radius != 150 || got.Area.Center != domain.SkillAreaCenterTarget || len(got.Statuses) != 4 {
						t.Fatal("pool/geometry", rank, got)
					}
					for _, app := range got.Statuses {
						if app.RandomGroup != 1 || app.RandomWeight != 1 || app.ChanceBP != 4000 || !app.ChanceExplicit {
							t.Fatal("pool changed with rank", app)
						}
						st, ok := states.ResolveSkillApplication(app)
						if !ok {
							t.Fatal(app)
						}
						if rank >= 100 {
							cap := int32(60)
							if app.ID == 1034 {
								cap = 15
							}
							if st.DurationSec != cap || st.EffectHitBP <= 0 {
								t.Fatal("high rank lost bounded-duration/effect-hit growth", rank, st)
							}
						}
					}
					got.Statuses = old.Statuses
				}
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(old)
				if string(a) != string(b) {
					t.Fatalf("non-pool gameplay payload changed %d/%d", id, rank)
				}
			}
		}
	})
	seeds := map[int32]map[domain.StatusID]int64{}
	for rank := int32(1); rank <= 5; rank++ {
		def, _ := skills.Get(10313, rank)
		old, _ := before.Get(10313, rank)
		seeds[rank] = map[domain.StatusID]int64{}
		for seed := int64(1); seed < 1000 && len(seeds[rank]) < 4; seed++ {
			s, src, dst := makeScene(seed, 1000000, 0)
			bs, bsrc, bdst := makeScene(seed, 1000000, 0)
			if !s.applySkill(src, dst, def, 0, false) || !bs.applySkill(bsrc, bdst, old, 0, false) {
				t.Fatal("native damage failed")
			}
			if dst.HP != bdst.HP {
				t.Fatalf("pool changed native damage at rank%d", rank)
			}
			if dst.Status.Count() != 1 {
				t.Fatal("one hit stacked or failed certain selected state", dst.Status.Count())
			}
			st := active(dst)
			seeds[rank][st.Def.ID] = seed
		}
		if len(seeds[rank]) != 4 {
			t.Fatal("not all pool candidates reachable", rank, seeds[rank])
		}
		for id, seed := range seeds[rank] {
			t.Run(fmt.Sprintf("rank%d/state%d", rank, id), func(t *testing.T) {
				s, src, dst := makeScene(seed, 1000000, 0)
				if !s.applySkill(src, dst, def, 0, false) {
					t.Fatal("cast failed")
				}
				st := active(dst)
				wantDuration := rank + 5
				if id == 1034 {
					wantDuration = rank + 2
				}
				if st.Def.DurationSec != wantDuration || st.Def.Icon <= 0 || !st.Def.IconKnown {
					t.Fatal("duration/icon", st.Def)
				}
				switch id {
				case 1006:
					if dst.Stats.MinAtk != 1000-rank*100 || dst.Stats.MaxAtk != 1000-rank*100 {
						t.Fatal("weaken not applied", dst.Stats)
					}
				case 1007:
					if dst.Stats.MAtk != 1000-rank*50 || dst.Stats.MDef != 50-int32(int64(50)*int64(rank*5)/100) {
						t.Fatal("dampen not applied", dst.Stats)
					}
				case 1008:
					if dst.Stats.Hit != 1000-(rank+1)*50 {
						t.Fatal("blind not applied", dst.Stats)
					}
				case 1034:
					if canMove(dst) || !canAct(dst) || !canCast(dst) || dst.Stats.Def != base.Def {
						t.Fatal("root has unwanted stun/defense effect", st.Def, dst.Stats)
					}
				}
				s.tick = st.ExpireAt
				s.stepStatus()
				if dst.Status.Count() != 0 || dst.Stats != base {
					t.Fatal("state/attribute not restored on expiry", dst.Stats)
				}
			})
		}
	}
	t.Run("single-contest-reproducible-seeds", func(t *testing.T) {
		def, _ := skills.Get(10313, 1)
		old, _ := before.Get(10313, 1)
		for _, resist := range []int32{0, 100, 1000000} {
			landed := 0
			for seed := int64(1); seed <= 400; seed++ {
				s, src, dst := makeScene(seed, 0, resist)
				ref, rsrc, rdst := makeScene(seed, 0, resist)
				s.applySkill(src, dst, def, 0, false)
				ref.applySkill(rsrc, rdst, old, 0, false)
				selected := int(ref.rng.Int63n(4))
				chance := domain.EffectSuccessProbability(4000, 0, int64(resist)*100)
				want := ref.rng.Float64() < chance
				got := dst.Status.Count() == 1
				if got != want || got && active(dst).Def.ID != def.Statuses[selected].ID {
					t.Fatalf("extra/base/resist draw seed%d resist%d got%v want%v", seed, resist, got, want)
				}
				if s.rng.Int63() != ref.rng.Int63() {
					t.Fatal("selected state rolled more than once")
				}
				if got {
					landed++
				}
			}
			t.Logf("400 seeded actual settlements at resist%d: %d states", resist, landed)
		}
	})
	t.Run("death-miss-immunity-and-no-reroll", func(t *testing.T) {
		def, _ := skills.Get(10313, 1)
		for _, mode := range []string{"dead", "fatal", "skill-immune", "harmful-immune", "overlay-block", "miss"} {
			s, src, dst := makeScene(seeds[1][1008], 1000000, 0)
			sink := join(t, s, 1, 1, "observer", 100, 100)
			switch mode {
			case "dead", "fatal":
				dst.HP = 0
				if mode == "fatal" {
					dst.HP = 1
				}
			case "skill-immune":
				s.defs = spawn.MapDefs{dst.Monster.TypeID: {ID: dst.Monster.TypeID, Stats: base, ImmuneSkills: map[domain.SkillID]struct{}{10313: {}}}}
			case "harmful-immune":
				dst.Status.Apply(domain.StatusDef{ID: 99999, DurationSec: 30, BlockHarmful: true}, s.tick, 0)
			case "overlay-block":
				strong, _ := states.Get(1008, 6)
				dst.Status.ApplyWithOverlay(strong, s.tick, 0, overlay)
			case "miss":
				def.HurtType = domain.HurtPhysical
				src.Stats.Hit = 0
				dst.Stats.Def = 1 << 20
			}
			count := dst.Status.Count()
			s.applySkill(src, dst, def, 0, false)
			if active(dst).Def.ID != 0 || dst.Status.Count() != count {
				t.Fatal("blocked/dead state rerolled or applied", mode)
			}
			s.flush()
			for _, ev := range sink.take() {
				if applied, ok := ev.(event.StatusApplied); ok && applied.Who == dst.ID {
					t.Fatal("unexpected status packet", mode, applied)
				}
			}
		}
	})
	t.Run("actual-area-each-target-picks-one", func(t *testing.T) {
		def, _ := skills.Get(10313, 5)
		s := newCombatScene(t)
		s.skills, s.statuses, s.statusOverlay = skills, states, overlay
		s.rng = rand.New(rand.NewSource(72001))
		join(t, s, 1, 1, "caster", 1000, 1000)
		src := s.players[1]
		src.Stats.MAtk, src.Stats.EffectHitBP = 1000, 1000000
		center := putMonster(s, 800, 10000000, base, 1300, 1000)
		for i := 0; i < 20; i++ {
			putMonster(s, domain.EntityID(900+i), 10000000, base, 1300+float64(i), 1000)
		}
		putMonster(s, 950, 10000000, base, 1449, 1000)
		putMonster(s, 951, 10000000, base, 1450, 1000)
		outside := putMonster(s, 952, 10000000, base, 1451, 1000)
		putMonster(s, 953, 0, base, 1300, 1000)
		targets := s.resolveTargets(src, def, center.ID, nil)
		if len(targets) != 23 {
			t.Fatal("150 radius/dead admission", targets)
		}
		seen := map[domain.StatusID]bool{}
		for _, id := range targets {
			dst := s.entities[id]
			s.applySkill(src, dst, def, 0, false)
			if dst.Status.Count() != 1 {
				t.Fatal("multi-target stacked", id)
			}
			seen[active(dst).Def.ID] = true
		}
		if len(seen) < 2 || outside.HP != 10000000 || outside.Status != nil && outside.Status.Count() != 0 {
			t.Fatal("shared per-cast selection or outside hit", seen)
		}
	})
	t.Run("overlay-lifecycle-save-rank-loss-source-departure", func(t *testing.T) {
		for _, id := range []domain.StatusID{1006, 1007, 1008, 1034} {
			s, _, dst := makeScene(seeds[5][id], 1000000, 0)
			join(t, s, 1, 1, "source", 100, 100)
			src := s.players[1]
			src.Player.Char.Race, src.Player.Char.Level = domain.Healer, 100
			src.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
			src.Player.Char.Skills = domain.Learned{10313: 5}
			s.refreshStats(src)
			src.Stats.MAtk, src.Stats.EffectHitBP = 1000, 1000000
			def, _ := skills.Get(10313, 5)
			def.SnapshotRawRanks = s.rawSkillLevels(src)
			def.SnapshotHardRanks = src.Player.Char.Skills.Clone()
			// Guaranteed selected identity, still through grouped application and
			// real damage; seed reachability of all four full pools was checked above.
			for _, app := range def.Statuses {
				if app.ID == id {
					def.Statuses = []domain.StatusApplication{app}
					break
				}
			}
			if !s.applySkill(src, dst, def, 0, false) || active(dst).Def.ID != id {
				t.Fatal("persistent cast", id)
			}
			old := active(dst)
			weak, _ := skills.Get(10313, 1)
			for _, app := range weak.Statuses {
				if app.ID == id {
					state, _ := s.resolveSkillStatusDef(weak, app, 0)
					if s.applyStatusDef(dst, state, src.ID) {
						t.Fatal("weaker state overwrote stronger", id)
					}
				}
			}
			if got := dst.Status.SaveForLogout(s.tick, time.Unix(1700000000, 0), dst.ID); len(got) != 0 {
				t.Fatal("foreign skill source persisted across logout", id)
			}
			// Legacy/self source snapshots retain selected identity and wall expiry,
			// never random configuration or a rerolled candidate.
			self := domain.NewStatusSet()
			self.Apply(old.Def, s.tick, dst.ID)
			wall := time.Unix(1700000000, 0)
			saved := self.SaveForLogout(s.tick, wall, dst.ID)
			blob, _ := json.Marshal(saved)
			if strings.Contains(string(blob), "RandomGroup") || strings.Contains(string(blob), "RandomWeight") {
				t.Fatal("pool leaked into persistent/wire state")
			}
			var roundTrip []domain.SavedStatus
			if err := json.Unmarshal(blob, &roundTrip); err != nil {
				t.Fatal(err)
			}
			restored := domain.RestoreSavedStatuses(roundTrip, 100, wall.Add(time.Second), dst.ID)
			if !restored.Has(id) || restored.Count() != 1 {
				t.Fatal("save rerolled selected state", id)
			}
			restored.Each(func(a domain.Active) {
				if a.Def.SkillSnapshotRank != 5 || a.ExpireAt != 100+domain.Ticks(int(old.Def.DurationSec-1)*1000) {
					t.Fatal("serialized selected state lost snapshot or offline expiry", id, a)
				}
			})
			src.Player.Char.Skills[10313] = 1
			s.refreshStats(src)
			down := active(dst)
			if down.Def.ID != id || down.Def.SkillSnapshotRank != 1 || down.ExpireAt > old.ExpireAt {
				t.Fatal("rank loss rerolled/upgraded state", id, down)
			}
			for _, app := range weak.Statuses {
				if app.ID == id && down.Def.Level != app.Level {
					t.Fatal("rank loss retained strong payload", id, down)
				}
			}
			src.Player.Char.Skills[10313] = 5
			s.refreshStats(src)
			if active(dst).Def.SkillSnapshotRank != 1 {
				t.Fatal("re-equip upgraded old state", id)
			}
			s.tick += domain.Ticks(1000)
			src.Stats.MAtk, src.Stats.EffectHitBP = 1000, 1000000 // refreshStats above removed the test combat overrides.
			if !s.applySkill(src, dst, def, 0, false) || dst.Status.Count() != 1 {
				t.Fatal("actual recast failed native damage or stacked states", id, active(dst))
			}
			if id == 1034 {
				if rule, ok := overlay.Rule(id, id); !ok || rule != domain.OverlayBlock || active(dst).Def.SkillSnapshotRank != 1 || active(dst).ExpireAt != down.ExpireAt {
					t.Fatal("native root self-block rule was bypassed/refreshed", active(dst))
				}
			} else if active(dst).Def.SkillSnapshotRank != 5 || active(dst).ExpireAt <= down.ExpireAt {
				t.Fatal("actual stronger recast failed to replace/refresh one state", id, active(dst))
			}
			s.revokeDepartingSkillSource(src)
			if dst.Status.Count() != 0 {
				t.Fatal("source departure kept stale state", id)
			}
		}
	})
	t.Run("full-original-tooltip-and-no-wire-fields", func(t *testing.T) {
		s, _, dst := makeScene(1, 1000000, 0)
		def, _ := skills.Get(10313, 1)
		lines := s.skillOriginalEffectLines(def)
		if len(lines) < 5 || lines[0] != def.Description || !strings.Contains(strings.Join(lines, "\n"), "有40%几率随机施加衰弱、降幅、致盲、定身之一。") {
			t.Fatal("original/pool tooltip", lines)
		}
		blob, _ := json.Marshal(s.buffSnapshot(dst))
		if strings.Contains(string(blob), "Random") {
			t.Fatal("wire changed")
		}
		if !reflect.DeepEqual(def.DirectEffects(), skills[domain.SkillKey{ID: 10313, Level: 1}].DirectEffects()) {
			t.Fatal("display changed damage")
		}
	})
}
