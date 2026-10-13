package scene

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/jackc/pgx/v5"
)

type beforeFamilyQuery struct{ data.Querier }

func (q beforeFamilyQuery) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	sql = strings.ReplaceAll(sql, "FROM game_skill_target_families ORDER BY", "FROM game_skill_target_families WHERE false ORDER BY")
	return q.Querier.Query(ctx, sql, args...)
}

func monsterFamilyFixture(t *testing.T) (domain.SkillTable, domain.SkillTable, data.MonsterDefs) {
	t.Helper()
	path := os.Getenv("FANTASY_MONSTER_FAMILIES_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration71 database required")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active cluster")
	}
	q := &skillMechanismsSingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	a, _, err := data.LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := data.LoadSkills(context.Background(), beforeFamilyQuery{q})
	if err != nil {
		t.Fatal(err)
	}
	m, err := data.LoadMonsters(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return a, b, m
}
func familyTarget(s *Scene, id domain.EntityID, d domain.MonsterDef, x, y float64) *entity.Entity {
	m := putMonster(s, id, 100000000, domain.Stats{}, x, y)
	m.Name, m.Level = d.Name, d.Level
	m.Monster.TypeID, m.Monster.Kind = d.ID, d.Kind
	return m
}

func TestExorcismMonsterFamilyPostgres(t *testing.T) {
	skills, before, defs := monsterFamilyFixture(t)
	ranks := []int32{1, 2, 3, 4, 5, 6, 10, 11, 20, 100, 1000}
	for _, rank := range ranks {
		d, ok := skills.Get(10312, rank)
		if !ok {
			t.Fatal(rank)
		}
		old, _ := before.Get(10312, rank)
		if !reflect.DeepEqual(d.TargetFamilies, []domain.MonsterFamily{domain.MonsterFamilyDemon, domain.MonsterFamilyUndead}) {
			t.Fatal("family condition lost", rank, d.TargetFamilies)
		}
		without := d
		without.TargetFamilies = nil
		x, _ := json.Marshal(without)
		y, _ := json.Marshal(old)
		if string(x) != string(y) {
			t.Fatalf("non-family payload changed at rank%d", rank)
		}
		if rank <= 5 && (d.Radius != 150 || d.Area.Radius != 150 || d.HurtType != 12 || d.Description != fmt.Sprintf("对半径150范围内的不死系和恶魔系敌人造成%d%%的伤害。", 270+10*rank)) {
			t.Fatalf("native original fields changed: %+v", d)
		}
		for _, id := range []domain.MonsterID{1025, 4100, 5013, 1209, 4158, 5089, 1003, 4062, 3124, 3125, 6016, 6114, 6214, 1207, 6080, 6067, 31009, 31010, 1001, 31736, 4131, 6002, 3086, 3173, 3202, 3210} {
			t.Run(fmt.Sprintf("rank%d/monster%d", rank, id), func(t *testing.T) {
				md, ok := defs[id]
				if !ok {
					t.Fatal("missing actual template", id)
				}
				s := newCombatScene(t)
				s.skills = skills
				s.defs = spawn.MapDefs(defs)
				src := putMonster(s, 800000, 100000000, domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 1000000}, 100, 100)
				dst := familyTarget(s, 800001, md, 120, 100)
				want := md.Kind.Hostile() && (md.Family == domain.MonsterFamilyUndead || md.Family == domain.MonsterFamilyDemon)
				if s.validTarget(src, dst, d) != want {
					t.Fatal("admission mismatch")
				}
				hp := dst.HP
				if got := s.applySkill(src, dst, d, 0, false); got != want || (dst.HP < hp) != want {
					t.Fatalf("settlement want%v landed%v HP%d->%d", want, got, hp, dst.HP)
				}
				if want {
					oldScene := newCombatScene(t)
					oldScene.defs = spawn.MapDefs(defs)
					oldSrc := putMonster(oldScene, 800000, 100000000, src.Stats, 100, 100)
					oldDst := familyTarget(oldScene, 800001, md, 120, 100)
					oldScene.applySkill(oldSrc, oldDst, old, 0, false)
					if dst.HP != oldDst.HP {
						t.Fatal("family check changed original damage/crit/RNG")
					}
				}
			})
		}
		// Obtaining or mutating evaluated ranks must not modify original anchors.
		d.TargetFamilies[0] = domain.MonsterFamilyOther
		again, _ := skills.Get(10312, rank)
		if again.TargetFamilies[0] != domain.MonsterFamilyDemon {
			t.Fatal("shared allow-list mutation")
		}
	}
	t.Run("all-other-skills-preserved", func(t *testing.T) {
		for _, id := range skills.ProgressionCatalog().IDs() {
			if id == 10312 {
				continue
			}
			for _, rank := range ranks {
				a, _ := skills.Get(id, rank)
				b, _ := before.Get(id, rank)
				x, _ := json.Marshal(a)
				y, _ := json.Marshal(b)
				if string(x) != string(y) {
					t.Fatal("unrelated skill changed", id, rank)
				}
			}
		}
	})
	t.Run("area-boundaries-and-independent-targets", func(t *testing.T) {
		d, _ := skills.Get(10312, 1)
		s := newCombatScene(t)
		s.defs = spawn.MapDefs(defs)
		src := putMonster(s, 800000, 1000000, domain.Stats{MAtk: 100, Hit: 1000000}, 100, 100)
		for i, dx := range []float64{149, 150, 151} {
			familyTarget(s, domain.EntityID(810000+i), defs[1025], 100+dx, 100)
		}
		familyTarget(s, 820000, defs[1003], 100, 101)
		familyTarget(s, 820001, defs[1001], 100, 102)
		familyTarget(s, 820002, defs[6080], 100, 103)
		familyTarget(s, 820003, defs[3173], 100, 104)
		at := src.Pos
		got := s.resolveTargets(src, d, 0, &at)
		want := []domain.EntityID{810000, 810001, 820000}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("radius/classification targets got%v want%v", got, want)
		}
	})
	t.Run("unknown-same-name-spoofs-players-and-template-changes", func(t *testing.T) {
		d, _ := skills.Get(10312, 5)
		s := newCombatScene(t)
		s.defs = spawn.MapDefs(defs)
		src := putMonster(s, 800000, 1000000, domain.Stats{MAtk: 100, Hit: 1000000}, 100, 100)
		missing := defs[1025]
		missing.ID = 999999
		dst := familyTarget(s, 800001, missing, 120, 100)
		if s.validTarget(src, dst, d) || s.applySkill(src, dst, d, 0, false) {
			t.Fatal("same name with no mapped template admitted")
		}
		dst.Monster.TypeID = 1025
		if !s.validTarget(src, dst, d) {
			t.Fatal("expected mapped undead")
		}
		dst.Monster.TypeID = 1001 // Type changes between selection and settlement.
		if s.applySkill(src, dst, d, 0, false) {
			t.Fatal("stale selection bypassed family recheck")
		}
		for _, kind := range []domain.MonsterKind{domain.MonsterProp, domain.MonsterGather} {
			dst.Monster.TypeID = 1025
			dst.Monster.Kind = kind
			if s.applySkill(src, dst, d, 0, false) {
				t.Fatal("noncombat spoof inherited family")
			}
		}
		dst.Monster.Kind = domain.MonsterNormal
		dst.Monster.TypeID = 3173
		if s.applySkill(src, dst, d, 0, false) {
			t.Fatal("spoofed instance rank overrode noncombat template")
		}
		join(t, s, 2, 2, "player", 120, 100)
		if s.validTarget(src, s.players[2], d) || s.applySkill(src, s.players[2], d, 0, false) {
			t.Fatal("family rule opened player damage")
		}
		for _, kind := range []domain.EntityKind{domain.KindPet, domain.KindTrap} {
			dst.Kind = kind
			dst.Monster.TypeID = 1025
			dst.Monster.Kind = domain.MonsterNormal
			if s.applySkill(src, dst, d, 0, false) {
				t.Fatal("non-monster type inherited family")
			}
		}
		dst.Kind = domain.KindMonster
		dst.Monster = nil
		if s.applySkill(src, dst, d, 0, false) {
			t.Fatal("missing monster state admitted")
		}
	})
	t.Run("summoned-template-family-no-reward", func(t *testing.T) {
		d, _ := skills.Get(10312, 1)
		s := newCombatScene(t)
		s.defs = spawn.MapDefs(defs)
		join(t, s, 1, 1, "caster", 100, 100)
		src := s.players[1]
		src.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 1000000}
		m := spawn.Summon(800001, domain.Pos{MapID: 7, X: 120, Y: 100}, defs[1025], 999)
		m.HP = 1
		s.entities[m.ID] = m
		s.monsters[m.ID] = m
		s.aoi.Enter(m)
		rewardSnapshot := func() []byte {
			c := src.Player.Char
			stacks := map[int]domain.Stack{}
			src.Player.Bag.Each(func(slot int, stack domain.Stack) { stacks[slot] = stack })
			b, _ := json.Marshal([]any{c.Level, c.Exp, c.Money, c.Honor, c.Quests, c.Skills, c.Trial, stacks, len(s.ground)})
			return b
		}
		saved := rewardSnapshot()
		if !s.validTarget(src, m, d) || !s.applySkill(src, m, d, 0, false) || m.Alive() {
			t.Fatal("summon should retain combat template family")
		}
		after := rewardSnapshot()
		if string(saved) != string(after) {
			t.Fatal("summon kill changed player rewards/progression")
		}
		if m.Monster.Summoner != 999 || m.Monster.SpawnID != 0 {
			t.Fatal("summon lifecycle identity changed")
		}
		pet := spawn.Summon(800002, domain.Pos{MapID: 7, X: 120, Y: 100}, defs[1001], 999)
		if s.validTarget(src, pet, d) || s.applySkill(src, pet, d, 0, false) {
			t.Fatal("summon without family wrongly admitted")
		}
	})
}
