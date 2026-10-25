package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// Frozen game_levels.exp inputs queried from real PostgreSQL through migration77.
// These are the production per-level rows restored by migration52, not exp_accum
// or a synthetic polynomial; all edges are included to verify actual settlement.
func riftExperienceAuditLevels() *domain.LevelTable {
	values := []int64{
		0, 30, 110, 270, 546, 875, 1350, 2016,
		2838, 3978, 5460, 7770, 11421, 16833, 24486, 34920,
		48735, 66591, 89208, 117366, 151905, 193725, 243786, 303108,
		372771, 453915, 547740, 655506, 778533, 918201, 1075950, 1253280,
		1451751, 1672983, 1918656, 2190510, 2490345, 2820021, 3181458, 3576636,
		4007595, 4476435, 4985316, 5536458, 6132141, 6774705, 7466550, 8210136,
		9007983, 9862671, 10776840, 11753190, 12794481, 13903533, 15083226, 16336500,
		17666355, 19075851, 20568108, 22146306, 23813685, 27027200, 28516992, 30071847,
		31694328, 33387093, 35504429, 38104444, 40860936, 43781925, 46875803, 50151347,
		53617734, 57284564, 61161868, 65260139, 69590342, 74163935, 78992894, 84089734,
		89467526, 95139929, 101121207, 107426261, 114070648, 121070615, 128443125, 136205888,
		144377387, 152976922, 162024625, 171541513, 181549514, 192071508, 203131365, 214753983,
		226965335, 239792513, 253263765, 267408556, 282257606,
	}
	curve := map[int32]int64{}
	for level := 1; level < len(values); level++ {
		curve[int32(level)] = values[level]
	}
	return domain.NewLevelTable(curve)
}

func riftExperienceAuditScene(t *testing.T, tier int32, currentRules bool, levels ...int32) (*Scene, *riftTestBackend, *int64, *entity.Entity) {
	t.Helper()
	s, backend, now := riftFixture(t)
	s.levels = riftExperienceAuditLevels()
	s.trial.MaxLevel = 100
	s.trial.FirstPct = 300
	s.trial.Multipliers = []int32{100, 110, 120, 130, 140, 200}
	rule := riftRulesFixture()
	if currentRules {
		rule.Version = 4
		rule.StatAnchorTier = 30
		rule.NormalBossEquipment = 10
		rule.CompletionLevelExperience = true
		var err error
		s.riftRules, err = domain.NewRiftRuleTable(4, []domain.RiftRules{*riftRulesFixture(), *rule}, lootRulesTableFixture(), balanceItemsFixture())
		if err != nil {
			t.Fatal(err)
		}
	}
	var roster []domain.TrialKeyMember
	names := map[domain.CharID]string{}
	profiles := map[domain.CharID]endgameRiftIdentity{}
	for i, level := range levels {
		id, char := domain.EntityID(i+1), domain.CharID((i+1)*100)
		if i != 0 {
			join(t, s, id, int64(char), fmt.Sprintf("member%d", i), 100, 100)
		}
		p := s.players[id]
		p.Level, p.Player.Char.Level = level, level
		p.Player.Char.Exp = 0
		p.Player.Char.Trial = domain.TrialProgress{Cycle: int32(i % 6)}
		s.endgamePlayer(p).loaded = true
		roster = append(roster, domain.TrialKeyMember{CharID: char, Level: level})
		names[char] = p.Name
		profiles[char] = endgameRiftIdentity{Race: p.Player.Char.Race, RewardLevel: level}
	}
	run, err := domain.NewRiftRun("xp-audit", tier, 100, roster, *now, rule)
	if err != nil {
		t.Fatal(err)
	}
	run.Activate(*now)
	run.Phase = domain.RiftBoss
	d := endgameRiftPersisted{Version: 2, Run: run, MapID: 7, Return: domain.Pos{MapID: 8}, Names: names, Profiles: profiles, Grants: map[domain.CharID]endgameRiftGrant{}}
	backend.runs[run.RunID] = d.record(1)
	if !s.initEndgameRift(d) {
		t.Fatal("rift initialization failed")
	}
	for _, p := range s.players {
		s.syncEndgameRift(p)
	}
	// Deliberately large native monster XP must never be awarded alongside the
	// per-character completion grant, even through the full onDeath path.
	def := domain.MonsterDef{ID: 999991, Name: "XP audit guardian", Kind: domain.MonsterBoss, Level: 63, HP: 1000, Exp: 9999999}
	s.defs = endgameRiftDefs{base: s.defs, trial: map[domain.MonsterID]domain.MonsterDef{def.ID: def}}
	boss := spawn.Encounter(s.alloc.Monster(), s.players[1].Pos, 0, def)
	s.entities[boss.ID], s.monsters[boss.ID] = boss, boss
	s.aoi.Enter(boss)
	run.RegisterMonster(boss.ID, 3)
	return s, backend, now, boss
}

func riftExperienceAuditKill(t *testing.T, s *Scene, now *int64, boss *entity.Entity, elapsed int64) {
	t.Helper()
	*now += elapsed
	boss.HP = 0
	s.onDeath(boss, s.players[1].ID)
	if s.rift.data.Run.Phase != domain.RiftCompleted || !s.rift.data.GrantsReady {
		t.Fatal("guardian did not create completion grants")
	}
	s.stepEndgameRift()
	s.stepEndgameRift()
}

func riftExperienceAuditTotal(s *Scene, p *entity.Entity) int64 {
	total := p.Player.Char.Exp
	for level := int32(1); level < p.Player.Char.Level; level++ {
		total += s.levels.Need(level)
	}
	return total
}

func TestRiftExperienceRealDatabaseAnchorsAndExactlyOnceSettlement(t *testing.T) {
	anchors := []struct {
		level   int32
		base    int64
		rewards [3]int64
	}{
		{12, 52740, [3]int64{58014, 210960, 843840}},
		{50, 673553, [3]int64{740908, 2694212, 10776848}},
		{60, 836924, [3]int64{920616, 3347696, 13390784}},
		{80, 5591720, [3]int64{6150892, 22366880, 89467520}},
		{100, 17641100, [3]int64{19405210, 70564400, 282257600}},
	}
	for _, anchor := range anchors {
		for i, tier := range []int32{1, 30, 150} {
			t.Run(fmt.Sprintf("level%d/tier%d", anchor.level, tier), func(t *testing.T) {
				s, b, now, boss := riftExperienceAuditScene(t, tier, true, anchor.level)
				p := s.players[1]
				before := riftExperienceAuditTotal(s, p)
				p.Player.StartExperienceBoost(900, 0, s.tick, 10000)
				s.flush()
				p.Player.Sink.(*fakeSink).take()
				riftExperienceAuditKill(t, s, now, boss, 1000)
				grant := s.rift.data.Grants[100]
				if grant.ExperienceLevel != anchor.level || grant.ExperienceBase != anchor.base || grant.Exp != anchor.rewards[i] {
					t.Fatalf("wrong frozen XP: level=%d base=%d XP=%d", grant.ExperienceLevel, grant.ExperienceBase, grant.Exp)
				}
				if riftExperienceAuditTotal(s, p) != before || b.commits != 1 {
					t.Fatal("native or premature XP awarded")
				}
				finishRiftRewardCommit(t, s, b)
				want := anchor.rewards[i]
				if anchor.level == domain.LevelCap {
					want = 0
				}
				if got := riftExperienceAuditTotal(s, p) - before; got != want {
					t.Fatalf("applied XP %d != %d", got, want)
				}
				if !s.rift.data.Grants[100].ExperienceClaimed {
					t.Fatal("XP receipt not durable")
				}
				s.flush()
				count := 0
				for _, ev := range p.Player.Sink.(*fakeSink).take() {
					if xp, ok := ev.(event.ExpGained); ok {
						count++
						if xp.Delta != want {
							t.Fatal("bonus or native XP leaked", xp)
						}
					}
				}
				wantCount := 1
				if want == 0 {
					wantCount = 0
				}
				if count != wantCount {
					t.Fatalf("XP event count %d != %d", count, wantCount)
				}
				s.onDeath(boss, p.ID)
				s.generateEndgameRiftGrants()
				s.syncEndgameRiftCompletion()
				if b.commits != 1 || riftExperienceAuditTotal(s, p)-before != want || s.rift.data.Grants[100].Exp != grant.Exp {
					t.Fatal("replayed guardian/reroll paid twice")
				}
			})
		}
	}
}

func TestRiftExperiencePersonalLevelIgnoresPartyTimeAndTrialRound(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("late%v", late), func(t *testing.T) {
			s, b, now, boss := riftExperienceAuditScene(t, 150, true, 12, 50, 60, 80, 100)
			elapsed := int64(1000)
			if late {
				elapsed = s.rift.data.Run.Rules.TimedMS + 1
			}
			riftExperienceAuditKill(t, s, now, boss, elapsed)
			if s.rift.data.Run.Timely == late {
				t.Fatal("wrong completion time fixture")
			}
			expected := []int64{843840, 10776848, 13390784, 89467520, 282257600}
			for i, want := range expected {
				if got := s.rift.data.Grants[domain.CharID((i+1)*100)].Exp; got != want {
					t.Fatalf("member%d XP %d != %d", i, got, want)
				}
			}
			for range expected {
				finishRiftRewardCommit(t, s, b)
			}
			if b.commits != 5 {
				t.Fatalf("expected one XP settlement per member, got %d", b.commits)
			}
			for char, grant := range s.rift.data.Grants {
				if !grant.ExperienceClaimed {
					t.Fatal("unsettled member", char)
				}
			}
		})
	}
}

func riftExperienceAuditUseLevelItem(t *testing.T, s *Scene) {
	t.Helper()
	p := s.players[1]
	item := domain.ItemDef{ID: 999992, Name: "XP audit item", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, PlayerExperience: map[int32]int64{60: s.levels.Need(60)}}
	s.items[item.ID] = item
	if p.Player.Bag.Add(item, 1) != 0 {
		t.Fatal("could not add XP item")
	}
	s.onUseItem(UseItem{ID: p.ID, Item: item.ID, Tab: 0xff, BagSlot: -1})
	if p.Player.Char.Level != 61 || p.Player.Char.Exp != 0 || p.Player.Bag.CountOf(item.ID) != 0 || s.rift.data.Run.Members[100].Level != 60 {
		t.Fatal("usable XP item did not establish opening/current divergence")
	}
}

func TestRiftExperienceCompletionLevelItemAndLegacySnapshot(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(fmt.Sprintf("current%v", current), func(t *testing.T) {
			s, b, now, boss := riftExperienceAuditScene(t, 150, current, 60)
			riftExperienceAuditUseLevelItem(t, s)
			before := riftExperienceAuditTotal(s, s.players[1])
			riftExperienceAuditKill(t, s, now, boss, 1000)
			// Historical grants retain their original serialized shape: no new evidence fields.
			want, base, level := int64(13390784), int64(0), int32(0)
			if current {
				want, base, level = 27027200, 1689200, 61
			}
			grant := s.rift.data.Grants[100]
			if grant.Exp != want || grant.ExperienceBase != base || grant.ExperienceLevel != level {
				t.Fatalf("wrong version-specific XP: got exp=%d base=%d level=%d, want %d/%d/%d", grant.Exp, grant.ExperienceBase, grant.ExperienceLevel, want, base, level)
			}
			finishRiftRewardCommit(t, s, b)
			if riftExperienceAuditTotal(s, s.players[1])-before != want {
				t.Fatal("wrong actual character settlement")
			}
			s.players[1].Player.Char.Level = 80
			s.generateEndgameRiftGrants()
			if s.rift.data.Grants[100].Exp != want || b.commits != 1 {
				t.Fatal("post-completion level change rerolled XP")
			}
		})
	}
}

func TestRiftExperienceQueuedGuardianFreezesActualDeathLevel(t *testing.T) {
	s, b, now, boss := riftExperienceAuditScene(t, 150, true, 60)
	riftExperienceAuditUseLevelItem(t, s)
	s.players[1].Player.Char.Race = domain.Assassin
	s.rift.economicPending = true
	*now += 1000
	boss.HP = 0
	s.onDeath(boss, 1)
	if len(s.rift.delayedDeaths) != 1 || s.rift.data.GrantsReady {
		t.Fatal("guardian was not queued")
	}
	s.players[1].Player.Char.Race = domain.Warrior
	s.players[1].Player.Char.Level = 80 // A later economic update must not alter death-time evidence.
	s.rift.economicPending = false
	s.stepEndgameRift()
	s.stepEndgameRift()
	grant := s.rift.data.Grants[100]
	if s.rift.data.RewardClasses[100] != 4 {
		t.Fatal("queued guardian borrowed later recipient class", s.rift.data.RewardClasses)
	}
	if grant.ExperienceLevel != 61 || grant.ExperienceBase != 1689200 || grant.Exp != 27027200 {
		t.Fatalf("queued death used later current level: level=%d base=%d XP=%d", grant.ExperienceLevel, grant.ExperienceBase, grant.Exp)
	}
	finishRiftRewardCommit(t, s, b)
	if s.players[1].Player.Char.Exp != 27027200 {
		t.Fatal("queued reward not settled exactly")
	}
}

func TestRiftExperienceCharacterAndPetBonusesAreSeparate(t *testing.T) {
	s, b, now, boss := riftExperienceAuditScene(t, 1, true, 60)
	owner := s.players[1]
	def := 海龟种
	s.petDefs = domain.PetTable{def.ID: def}
	s.petLvls = domain.NewPetLevelTable(map[int32]int64{1: 100000000})
	owner.Player.Char.Pets = []domain.PetInstance{{ID: 900001, Def: def.ID, Level: 1, Prefix: domain.NoPetPrefix, Base: def.BaseAt(1), Deployed: true}}
	pet := &entity.Entity{ID: 8, Kind: domain.KindPet, Level: 1, HP: 1, MaxHP: 100, Pet: &entity.Pet{Owner: owner.ID, Inst: &owner.Player.Char.Pets[0], Def: def}}
	owner.Player.Pet = pet.ID
	s.entities[pet.ID], s.pets[pet.ID] = pet, pet
	owner.Player.StartExperienceBoost(900, 100, s.tick, 10000)
	riftExperienceAuditKill(t, s, now, boss, 1000)
	grant := s.rift.data.Grants[100]
	// Character gets the fixed 920,616; pet gets floor(920,616/2) plus 100%.
	if grant.Exp != 920616 || grant.Pet.Experience != 920616 || grant.Pet.BonusPct != 100 {
		t.Fatalf("mixed character/pet multipliers: characterXP=%d petXP=%d petBonus=%d", grant.Exp, grant.Pet.Experience, grant.Pet.BonusPct)
	}
	owner.Player.StartExperienceBoost(1000, 900, s.tick, 10000)
	finishRiftRewardCommit(t, s, b)
	if owner.Player.Char.Exp != 920616 || owner.Player.Char.Pets[0].Exp != 920616 {
		t.Fatal("pet share deducted, rerolled or character bonus applied")
	}
}

func TestRiftExperienceOfflineUsesLastObservedLevelAndFrozenClaim(t *testing.T) {
	s, b, now, boss := riftExperienceAuditScene(t, 150, true, 60, 60)
	other := s.players[2]
	other.Player.Char.Level = 61
	*now += 1000
	s.stepEndgameRift() // Both members receive eligible presence before disconnection.
	s.disconnectEndgameRift(other)
	if s.rift.data.Profiles[200].RewardLevel != 61 {
		t.Fatal("disconnect did not cache current personal level")
	}
	delete(s.players, other.ID)
	riftExperienceAuditKill(t, s, now, boss, 1000)
	grant := s.rift.data.Grants[200]
	if !s.rift.data.Run.Members[200].Eligible || grant.ExperienceLevel != 61 || grant.ExperienceBase != 1689200 || grant.Exp != 27027200 {
		t.Fatalf("offline level evidence: eligible=%v level=%d base=%d XP=%d", s.rift.data.Run.Members[200].Eligible, grant.ExperienceLevel, grant.ExperienceBase, grant.Exp)
	}
	finishRiftRewardCommit(t, s, b) // Settle the online member first.
	other.Player.Char.Level = 80
	s.players[other.ID] = other // Reconnected for recovery, without live-run readiness.
	s.claimEndgameRift(other, domain.EndgameRequest{Op: "rift.claim", RequestID: "offline-xp-audit", ExpectedRevision: "0"}, s.rift.data, s.rift.revision, false)
	if b.commits != 2 || b.plan.Snapshot.Char.Exp != 27027200 {
		t.Fatal("offline claim rerolled at claim level or failed to settle")
	}
	finishRiftRewardCommit(t, s, b)
	if other.Player.Char.Exp != 27027200 {
		t.Fatal("offline frozen XP did not persist")
	}
}

func TestRiftExperienceAlreadyGeneratedLegacyGrantIsNotRecalculated(t *testing.T) {
	s, b, _, _ := riftExperienceAuditScene(t, 150, true, 60)
	d := s.rift.data.clone()
	d.Run.Phase = domain.RiftCompleted
	d.Run.Members[100].Eligible = true
	d.GrantsReady = true
	// A pre-change level-60, tier-150 completion already owns this exact grant.
	d.Grants[100] = endgameRiftGrant{Exp: 12553860, ExperienceLevel: 60, ExperienceBase: 836924}
	record := d.record(2)
	restored, ok := decodeEndgameRift(&record, s.riftRules)
	if !ok {
		t.Fatal("legacy frozen grant failed to reload")
	}
	s.rift.data = restored
	s.generateEndgameRiftGrants()
	if s.rift.data.Grants[100].Exp != 12553860 {
		t.Fatal("formula update rewrote an already generated reward")
	}
	p := s.players[1]
	p.Player.Char.Level = 80
	if !s.settleEndgameRiftExperience(p) || b.plan.Snapshot.Char.Exp != 12553860 {
		t.Fatal("legacy reward changed at settlement")
	}
	finishRiftRewardCommit(t, s, b)
	if p.Player.Char.Exp != 12553860 || !s.rift.data.Grants[100].ExperienceClaimed {
		t.Fatal("legacy frozen reward was not settled exactly")
	}
}
