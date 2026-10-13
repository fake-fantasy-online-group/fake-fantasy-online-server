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
)

func TestMonsterTheftPostgresLoadedSkillPoolAndItem(t *testing.T) {
	path := os.Getenv("FANTASY_THEFT_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migrated PostgreSQL directory required")
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
	items, err := data.LoadItems(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	drops, stats, err := data.LoadDrops(ctx, q, items)
	if err != nil {
		t.Fatal(err)
	}
	rolls, err := data.LoadEquipmentRollTable(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := data.LoadLootRules(ctx, q, items)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Loaded == 0 || stats.KindLoaded == 0 {
		t.Fatal("real drop tables missing fixed or type entries")
	}

	// A normal monster and two real entries from its loaded pool exercise both
	// explicit templates and category/level-range resolution without a new pool.
	var monster domain.MonsterID
	var material, equipment domain.DropEntry
	rows, err := q.Query(ctx, `SELECT id FROM game_monsters WHERE kind='普通' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id domain.MonsterID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		var a, b domain.DropEntry
		for _, entry := range drops[id] {
			if len(entry.Choices) == 0 && items[entry.Item].Equip == nil {
				a = entry
			}
			if len(entry.Choices) > 0 && items[entry.Choices[0]].Equip != nil {
				b = entry
			}
		}
		if a.Item != 0 && b.Item != 0 {
			monster, material, equipment = id, a, b
			break
		}
	}
	rows.Close()
	if monster == 0 {
		t.Fatal("no real normal monster has both item/template and equipment-category pool")
	}

	makeScene := func(t *testing.T, rank int32) (*Scene, *entity.Entity, *entity.Entity) {
		s, src, dst, _ := theftTestScene(t, 10000)
		localItems := make(data.ItemDefs, len(items)+1)
		for id, item := range items {
			localItems[id] = item
		}
		if rank > 20 {
			// Use the real equipment/loadout resolver for synthetic high-rank
			// boundary coverage, not illegal hard-point allocations above 20.
			s, src, _ = roundingPlayer(t, 10212, rank)
			for id, item := range s.items {
				localItems[id] = item
			}
			s.alloc = domain.NewEntityAlloc()
			dst = putMonster(s, 900, 100000, domain.Stats{}, 120, 100)
		}
		s.skills, s.items, s.lootTable, s.equipmentRoll, s.lootRules = skills, localItems, drops, rolls, rules
		dst.Monster.TypeID, dst.Monster.ColorProfile = monster, 2
		s.defs = spawn.MapDefs{monster: {ID: monster, Kind: domain.MonsterNormal, Level: 1}}
		src.Player.Char.Skills = domain.Learned{10212: min(rank, 20)}
		src.Player.EndgameGrowthV5, src.Player.EndgameSkillBehavior = true, true
		// A zero drop multiplier must not suppress an approved weighted theft.
		s.penalty = domain.LevelPenalty{ExpBelowStart: 1, DropStepBP: 10000, DropFloorBP: 0}
		var seed int64
		for rand.New(rand.NewSource(seed)).Intn(10000) >= 5000 {
			seed++
		}
		s.rng = rand.New(rand.NewSource(seed))
		return s, src, dst
	}
	settle := func(t *testing.T, s *Scene, src, dst *entity.Entity) *entity.Entity {
		s.onUseSkill(UseSkill{ID: src.ID, Target: dst.ID, Skill: 10212})
		if len(s.casts) != 1 {
			t.Fatal("real onUseSkill rejected loaded theft")
		}
		at := time.Now().Add(time.Hour)
		s.stepCastsAt(at, false)
		s.stepCastsAt(at.Add(time.Hour), false)
		s.flush()
		return onlyTheftDrop(t, s)
	}
	var behaviorPackets []string
	for _, rank := range []int32{1, 2, 3, 4, 5, 6, 9, 10, 11, 20, 100, 1000, 8810, 10000, 65536} {
		t.Run(fmt.Sprintf("rank%d-real-entry-and-display", rank), func(t *testing.T) {
			s, src, dst := makeScene(t, rank)
			def, ok := skills.Get(10212, rank)
			if !ok {
				t.Fatal("unresolved rank")
			}
			s.applyResolvedSkillModifiers(src, &def, src.Player.Char.Skills, s.rawSkillLevels(src))
			policy, _ := domain.LookupSkillGrowthPolicy(def.ID, skills.ProgressionCatalog())
			if policy.EfficiencyCapRank != 10 {
				t.Fatal("missing approved post-cap continuation", policy)
			}
			t.Logf("actual resolved rank%d: MP%d cooldown%dms efficiencyCap%d", rank, def.MPCost, def.CooldownMS, policy.EfficiencyCapRank)
			effect, ok := def.TheftEffect()
			chance := min(int32(10000), 5000+(rank-1)*600)
			if !ok || effect.ChanceBP != chance || effect.TheftMaxSuccesses != 1 || def.HasDamageEffect() || len(def.DirectEffects()) != 1 {
				t.Fatalf("incorrect loaded theft: %+v", def.DirectEffects())
			}
			if rank <= 5 && (def.MPCost != 69+(rank-1)*12 || def.CooldownMS != 5000 || def.PrepareMS != 0 || def.Dist != 75) {
				t.Fatalf("original resource/targeting fields changed: %+v", def)
			}
			lines := s.skillOriginalEffectLines(def)
			if len(lines) < 4 || !strings.Contains(lines[0], skillBasisPoints(chance)+"%") || strings.Contains(lines[0], "伤害") {
				t.Fatal("tooltip did not follow theft", lines)
			}
			identity := s.skillBehaviorIdentity(10212)
			payload := s.skillBehaviorPayload(src, def)
			if identity == nil || identity.Name != "偷窃术" || !payload.Usable || payload.InputMode != "entity" || !payload.TargetEnemy || payload.TargetSelf || payload.TargetTeam {
				t.Fatal("typed contract still holds/targets incorrectly", identity, payload)
			}
			view := domain.SkillRankView{ID: 10212, Contract: identity}
			wire, err := json.Marshal(view)
			if err != nil || strings.Contains(string(wire), `"contract":null`) {
				t.Fatal("typed theft was suppressed", string(wire), err)
			}
			if rank == 1 || rank == 10 || rank == 11 || rank == 20 || rank == 100 {
				ranks, err := s.skillRanksForDisplay(src)
				if err != nil {
					t.Fatal(err)
				}
				view := ranks[10212]
				if view.Contract == nil || view.Contract.Current == nil || view.ContractUnavailable {
					t.Fatal("actual rank packet omits usable theft contract", view)
				}
				response := domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: []domain.SkillRankView{view}, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "formulaVersion": "5", "unspentPoints": "1"}}
				packet, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				behaviorPackets = append(behaviorPackets, string(packet))
			}
			hp, mana := dst.HP, src.MP
			drop := settle(t, s, src, dst)
			if mana-src.MP != def.MPCost {
				t.Fatal("actual payment differs from displayed cost", mana-src.MP, def.MPCost)
			}
			if rank > 10 {
				den := int64(rank) + 40
				wantMP := max(int64(1), (8850+den-1)/den)
				wantCD := max(int64(100), ((250000+den*100-1)/(den*100))*100)
				if int64(def.MPCost) != wantMP || int64(def.CooldownMS) != wantCD || !def.RoundingPreviewReady {
					t.Fatal("post-cap resource continuation wrong", def.MPCost, def.CooldownMS, wantMP, wantCD)
				}
				if err := domain.ValidateSkillRoundingCredits(src.Player.Char.SkillRoundingCredits); err != nil {
					t.Fatal("new theft rounding is not persistable", err)
				}
			}
			member := false
			for _, entry := range drops[monster] {
				if len(entry.Choices) == 0 && drop.Drop.Item == entry.Item {
					member = true
				}
				for _, id := range entry.Choices {
					if drop.Drop.Item == id {
						member = true
					}
				}
			}
			if !member || drop.Drop.Count != 1 || dst.HP != hp || dst.Monster.TheftSuccesses != 1 {
				t.Fatal("loaded skill result not one pool member with zero damage", drop.Drop)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		entry domain.DropEntry
	}{{"material", material}, {"equipment-type", equipment}} {
		t.Run("real-instance-"+tc.name, func(t *testing.T) {
			s, src, dst := makeScene(t, 1)
			// Isolate one already-loaded row to deterministically cover both kinds;
			// neither its candidates, weights nor generation flags are rewritten.
			s.lootTable = map[domain.MonsterID][]domain.DropEntry{monster: {tc.entry}}
			drop := settle(t, s, src, dst)
			st := drop.Drop.Stack
			if tc.name == "equipment-type" && (st.UID == 0 || st.InstanceKind != domain.ItemInstanceEquipment || st.MaxDurability <= 0 || st.Durability != st.MaxDurability) {
				t.Fatal("equipment generation/identity missing", st)
			}
			src.Player.Bag = domain.NewBag(1)
			s.onPickUp(PickUp{ID: src.ID, Target: drop.ID})
			if src.Player.Bag.At(0) != st || s.ground[drop.ID] != nil {
				t.Fatal("ground->inventory rerolled/lost generated stack")
			}
		})
	}
	for _, rank := range []int32{11, 20, 100, 10000} {
		t.Run(fmt.Sprintf("rank%d-51-casts-rounding-and-rejoin", rank), func(t *testing.T) {
			s, src, _ := makeScene(t, rank)
			s.lootTable = map[domain.MonsterID][]domain.DropEntry{monster: {material}}
			var manaPaid, ticksPaid int64
			for i := 0; i < 51; i++ {
				// Replenish test mana: re-entry correctly clamps runtime MP to the
				// rebuilt character maximum; this test measures paid costs, not regen.
				src.MP, src.MaxMP = 1000000, 1000000
				dst := putMonster(s, domain.EntityID(1000+i), 100000, domain.Stats{}, 120, 100)
				dst.Monster.TypeID = monster
				before := src.MP
				s.onUseSkill(UseSkill{ID: src.ID, Target: dst.ID, Skill: 10212})
				if len(s.casts) != 1 {
					t.Fatal("repeated legitimate cast rejected", i)
				}
				cost, cooldown := before-src.MP, domain.Ticks(int(s.casts[0].cooldownMS))
				if cost < 1 || cooldown < 1 {
					t.Fatal("resource floor bypassed", cost, cooldown)
				}
				credit, remaining := src.Player.Char.SkillRoundingCredits[10212], src.MP
				s.onUseSkill(UseSkill{ID: src.ID, Target: dst.ID, Skill: 10212})
				if len(s.casts) != 1 || src.MP != remaining || src.Player.Char.SkillRoundingCredits[10212] != credit {
					t.Fatal("duplicate packet consumed rounding/payment")
				}
				at := time.Now().Add(time.Hour)
				s.stepCastsAt(at, false)
				s.stepCastsAt(at.Add(time.Hour), false)
				drop := onlyTheftDrop(t, s)
				s.removeLoot(drop.ID, event.DespawnPickedUp)
				manaPaid += int64(cost)
				ticksPaid += int64(cooldown)
				s.tick += cooldown
				if i == 24 {
					snap := s.snapshotOf(src)
					encoded, err := json.Marshal(snap.Char)
					if err != nil {
						t.Fatal(err)
					}
					var restored domain.Character
					if err := json.Unmarshal(encoded, &restored); err != nil {
						t.Fatal(err)
					}
					if err := domain.ValidateSkillRoundingCredits(restored.SkillRoundingCredits); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(restored.SkillRoundingCredits, src.Player.Char.SkillRoundingCredits) {
						t.Fatal("rounding credit changed in saved character")
					}
					runtime := src.CapturePlayerRuntime(s.tick)
					id := src.ID
					s.onLeave(Leave{ID: id})
					s.onEnter(Enter{ID: id, Char: &restored, Runtime: &runtime, Bag: snap.Bag, Worn: snap.Worn, Sink: &fakeSink{}})
					src = s.players[id]
					if src == nil || !reflect.DeepEqual(src.Player.Char.SkillRoundingCredits, restored.SkillRoundingCredits) {
						t.Fatal("rejoin lost rounding credit")
					}
				}
			}
			den := int64(rank) + 40
			wantMana, wantTicks := max(int64(51), (51*8850+den-1)/den), max(int64(51), (51*2500+den-1)/den)
			if manaPaid != wantMana || ticksPaid != wantTicks {
				t.Fatalf("51 real casts paid mana%d/ticks%d, want %d/%d", manaPaid, ticksPaid, wantMana, wantTicks)
			}
			t.Logf("51 accepted casts, duplicate packets, mid-run save/rejoin: mana%d ticks%d", manaPaid, ticksPaid)
		})
	}
	if path := os.Getenv("FANTASY_THEFT_BEHAVIOR_JSONL"); path != "" {
		if err := os.WriteFile(path, []byte(strings.Join(behaviorPackets, "\n")+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("loaded %d skills, %d items, %d explicit + %d type drop rows; selected real monster %d", len(skills), len(items), stats.Loaded, stats.KindLoaded, monster)
}
