package scene

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func rebalanceRiftRulesFixture(t *testing.T) *domain.RiftRuleTable {
	t.Helper()
	loot := weightedSceneLootFixture(t)
	a, b, c, d := *riftRulesFixture(), *riftRulesFixture(), *riftRulesFixture(), *riftRulesFixture()
	b.Version, b.FloorPopulationMultiplier = 2, 2
	c.Version, c.FloorPopulationMultiplier, c.LootVersion = 3, 2, 2
	d.Version, d.FloorPopulationMultiplier, d.LootVersion, d.StatAnchorTier, d.NormalBossEquipment = 4, 2, 2, 30, 10
	d.CompletionLevelExperience = true
	d.ExitName, d.ExitSprite, d.ExitPortrait, d.NPCModel = "洞天残魂", "npc362", "", 362
	d.ExitGreeting = "好久没见到别的人了……这里的风，总算静下来了。来吧，我这就送你回去。"
	rules, err := domain.NewRiftRuleTable(4, []domain.RiftRules{a, b, c, d}, loot, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestRiftRebalanceActualSpawnAnchorUsesRosterBase(t *testing.T) {
	for _, boss := range []bool{false, true} {
		for _, party := range []int{1, 5} {
			before, _ := seededRiftLayoutFixture(t, 1)
			after, _ := seededRiftLayoutFixture(t, 1)
			for _, s := range []*Scene{before, after} {
				for n := 2; n <= party; n++ {
					s.rift.data.Run.Members[domain.CharID(n*100)] = &domain.RiftMember{CharID: domain.CharID(n * 100), Level: 60}
				}
				if boss {
					s.rift.data.Run.Phase = domain.RiftBoss
				}
			}
			after.rift.data.Run.Tier = 30
			after.rift.data.Run.Rules = rebalanceRiftRulesFixture(t).Active()
			after.rift.data.Run.RulesVersion = 4
			before.populateEndgameRift(boss)
			after.populateEndgameRift(boss)
			if len(before.monsters) != len(after.monsters) || len(before.monsters) == 0 {
				t.Fatal("population changed")
			}
			for id, old := range before.monsters {
				now := after.monsters[id]
				if now == nil || old.MaxHP != now.MaxHP || old.Stats != now.Stats || old.Level != now.Level || old.Level != 60 {
					t.Fatalf("boss%v party%d actual anchor mismatch: old%+v new%+v", boss, party, old, now)
				}
			}
		}
	}
}

func normalizeRiftStacks(items []domain.Stack) []domain.Stack {
	out := append([]domain.Stack(nil), items...)
	for i := range out {
		out[i].UID = 0
		out[i].Endgame.FormationRollBP = 0
	}
	return out
}

func TestRiftTenNormalBossItemsAddToUnchangedSpecialRuneDust(t *testing.T) {
	for _, tier := range []int32{1, 10, 30, 150} {
		for _, timely := range []bool{false, true} {
			s, b, now := riftFixture(t)
			activateTestRift(t, s, b, *now)
			s.lootRules = weightedSceneLootFixture(t)
			s.riftRules = rebalanceRiftRulesFixture(t)
			d := &s.rift.data
			d.Version = 2
			d.Run.Tier = tier
			d.Run.Timely = timely
			d.Run.Phase = domain.RiftCompleted
			d.Run.Members[100].Eligible = true
			d.Run.RulesVersion = 3
			d.Run.Rules, _ = s.riftRules.Version(3)
			s.generateEndgameRiftGrants()
			original := d.Grants[100]
			d.GrantsReady = false
			d.Grants = map[domain.CharID]endgameRiftGrant{}
			d.Run.RulesVersion = 4
			d.Run.Rules = s.riftRules.Active()
			s.generateEndgameRiftGrants()
			revised := d.Grants[100]
			if len(revised.Items) != len(original.Items)+10 {
				t.Fatalf("tier%d timely%v: count %d vs old%d", tier, timely, len(revised.Items), len(original.Items))
			}
			if !reflect.DeepEqual(normalizeRiftStacks(original.Items), normalizeRiftStacks(revised.Items[:len(original.Items)])) {
				t.Fatal("old special/rune/dust stream changed")
			}
			equipment := 0
			for _, st := range revised.Items {
				if st.InstanceKind == domain.ItemInstanceEquipment {
					equipment++
				}
			}
			want := 11
			if timely {
				want = 12
			}
			if equipment != want {
				t.Fatal("wrong total equipment", equipment, want)
			}
			for _, st := range revised.Items[len(original.Items):] {
				if st.InstanceKind != domain.ItemInstanceEquipment || st.UID == 0 || !st.Bound {
					t.Fatal("invalid normal boss instance")
				}
			}
			// An accidental repeated generation call must preserve exact UIDs/rolls.
			s.generateEndgameRiftGrants()
			if !reflect.DeepEqual(revised, d.Grants[100]) {
				t.Fatal("rerolled frozen grant")
			}
			record := d.record(99)
			restored, ok := decodeEndgameRift(&record, s.riftRules)
			if !ok || !reflect.DeepEqual(restored.Grants[100], revised) {
				t.Fatal("persisted grant changed")
			}
		}
	}
}

func TestRiftNormalBossMFIsPersonalFrozenAndQualityOnly(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.riftRules = rebalanceRiftRulesFixture(t)
	s.rift.data.Run.Rules = s.riftRules.Active()
	s.rift.data.Run.RulesVersion = 4
	s.rift.data.Version = 2
	p := s.players[1]
	slot, gear, mf := magicFindGearV5(t, s, p)
	s.equipmentRoll = magicFindRollTableV5()
	join(t, s, 2, 200, "other", 100, 100)
	s.players[2].Player.Char.Level = 60
	s.rift.data.Run.Members[200] = &domain.RiftMember{CharID: 200, Level: 60, Eligible: true}
	s.rift.data.Run.Members[300] = &domain.RiftMember{CharID: 300, Level: 60, Eligible: false}
	s.rift.data.Run.Members[100].Eligible = true
	s.rift.data.Run.Phase = domain.RiftCompleted
	s.generateEndgameRiftGrants()
	if s.rift.data.Grants[100].MagicFind != mf || s.rift.data.Grants[200].MagicFind != 0 {
		t.Fatal("recipient borrowed other player's MF")
	}
	if _, ok := s.rift.data.Grants[300]; ok {
		t.Fatal("ineligible player got gear")
	}
	frozen := s.rift.data.clone()
	p.Player.Worn.Set(slot, domain.Stack{})
	s.players[2].Player.Worn.Set(slot, gear)
	s.generateEndgameRiftGrants()
	if !reflect.DeepEqual(frozen.Grants, s.rift.data.Grants) {
		t.Fatal("gear swap rerolled grants")
	}
	// Deliberately choose one legal native base; MF changes natural quality only,
	// while boss-grade sockets and item selection are on independent streams.
	var base domain.ItemID
	for id, def := range s.items {
		if def.Equip != nil && domain.NativeEquipmentDropEligible(def) {
			base = id
			break
		}
	}
	if base == 0 {
		t.Fatal("missing pool")
	}
	beforeAffixes, afterAffixes, beforeUnique, afterUnique := 0, 0, 0, 0
	for i := int32(0); i < 2000; i++ {
		a := s.rollEndgameRiftNormalBossEquipment(100, i, []domain.ItemID{base}, 0)
		z := s.rollEndgameRiftNormalBossEquipment(100, i, []domain.ItemID{base}, 800)
		if a.Item != z.Item || a.SocketCount != z.SocketCount {
			t.Fatal("MF changed quantity/base/socket stream")
		}
		beforeAffixes += int(a.RolledAffixCount)
		afterAffixes += int(z.RolledAffixCount)
		if a.Endgame.UniqueID != 0 {
			beforeUnique++
		}
		if z.Endgame.UniqueID != 0 {
			afterUnique++
		}
	}
	if afterAffixes <= beforeAffixes || afterUnique <= beforeUnique {
		t.Fatalf("MF did not improve normal quality: affixes %d→%d uniques%d→%d", beforeAffixes, afterAffixes, beforeUnique, afterUnique)
	}
	t.Logf("2000 deterministic normal-boss slots: affixes %d→%d, unique %d→%d", beforeAffixes, afterAffixes, beforeUnique, afterUnique)
}

func TestRiftDisconnectedRecipientKeepsLastPersonalContext(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.riftRules = rebalanceRiftRulesFixture(t)
	s.rift.data.Run.Rules = s.riftRules.Active()
	s.rift.data.Run.RulesVersion = 4
	p := s.players[1]
	_, _, mf := magicFindGearV5(t, s, p)
	p.Player.Char.Level = 61
	s.disconnectEndgameRift(p)
	delete(s.players, p.ID)
	context := s.freezeEndgameRiftRewardContexts()[100]
	if context.Level != 61 || context.MagicFind != mf {
		t.Fatal("offline context lost", context)
	}
}

func TestRiftAddedGearPersistsAcrossLostAckAndReconnect(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.riftRules = rebalanceRiftRulesFixture(t)
	d := &s.rift.data
	d.Version = 2
	d.Run.RulesVersion = 4
	d.Run.Rules = s.riftRules.Active()
	d.Run.Phase = domain.RiftCompleted
	d.Run.Timely = true
	d.Run.Members[100].Eligible = true
	s.generateEndgameRiftGrants()
	d.Run.Revision++
	s.stepEndgameRift()
	s.stepEndgameRift()
	finishRiftRewardCommit(t, s, b)
	grant := s.rift.data.Grants[100]
	if len(grant.Items) != 15 {
		t.Fatal("expected12equipment+2runes+dust", len(grant.Items))
	}
	p := s.players[1]
	var selected domain.EntityID
	for id, ref := range s.rift.rewardDrops {
		if ref.Char == 100 && ref.Index == len(grant.Items)-1 {
			selected = id
		}
	}
	if selected == 0 {
		t.Fatal("normal item not projected")
	}
	drop := s.ground[selected]
	p.Pos = drop.Pos
	s.onPickUp(PickUp{ID: p.ID, Target: selected})
	b.runs[b.plan.Run.RunID] = *b.plan.Run
	snapshot := b.plan.Snapshot.Clone()
	b.commit <- domain.EndgameCommitResult{Authoritative: &snapshot, State: b.plan.State, Receipt: b.plan.Receipt, Replayed: true}
	s.finishEndgame(false)
	s.pollEndgameRiftReload(s.riftMS())
	s.syncEndgameRiftCompletion()
	if s.ground[selected] != nil {
		t.Fatal("lost ACK left collected extra gear")
	}
	before := b.commits
	s.onPickUp(PickUp{ID: p.ID, Target: selected})
	if b.commits != before {
		t.Fatal("duplicate normal pickup")
	}
	snapshot = s.snapshotOf(p).Clone()
	s.exec(Leave{ID: p.ID, Reason: "reconnect extra reward"})
	p = reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, &fakeSink{})
	s.syncEndgameRiftCompletion()
	if !reflect.DeepEqual(grant.Items, s.rift.data.Grants[100].Items) {
		t.Fatal("reconnect rerolled normal items")
	}
	count := 0
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.UID == drop.Drop.Stack.UID {
			count++
		}
	})
	if count != 1 {
		t.Fatal(fmt.Sprintf("extra reward identity count%d", count))
	}
}

func TestRiftOfflineContextSurvivesAnotherMembersPendingLeave(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, killBeforeTick := range []bool{false, true} {
			t.Run(fmt.Sprintf("replayed_%v_kill_before_tick_%v", replay, killBeforeTick), func(t *testing.T) {
				s, b, now, boss := riftExperienceAuditScene(t, 150, true, 60, 60, 60)
				*now += 1000
				s.rift.data.Run.ObserveClock(*now, map[domain.CharID]bool{100: true, 200: true, 300: true})
				s.leaveEndgameRift(s.players[1], domain.EndgameRequest{Op: "rift.leave", RequestID: "context-leave-0001", ExpectedRevision: "0"})
				s.finishEndgame(false)
				if !s.rift.economicPending || b.commit == nil {
					t.Fatal("leave did not start pending aggregate")
				}
				recipient := s.players[2]
				recipient.Player.Char.Level = 61
				_, _, mf := magicFindGearV5(t, s, recipient)
				s.exec(Leave{ID: recipient.ID, Reason: "disconnect during teammate leave"})
				if s.players[2] != nil || s.rift.data.Profiles[200].RewardLevel != 61 {
					t.Fatal("offline context not cached")
				}
				if replay {
					b.runs[b.plan.Run.RunID] = *b.plan.Run
					snapshot := b.plan.Snapshot.Clone()
					b.commit <- domain.EndgameCommitResult{Authoritative: &snapshot, State: b.plan.State, Receipt: b.plan.Receipt, Replayed: true}
				} else {
					b.finish(true)
				}
				s.finishEndgame(false)
				if replay {
					s.pollEndgameRiftReload(s.riftMS())
				}
				if !killBeforeTick {
					s.stepEndgameRift()
				}
				context := s.freezeEndgameRiftRewardContexts()[200]
				if context.Level != 61 || context.MagicFind != mf {
					t.Fatal("pending aggregate discarded offline observation", context, mf)
				}
				*now += 1000
				boss.HP = 0
				s.onDeath(boss, s.players[3].ID)
				grant, ok := s.rift.data.Grants[200]
				if !ok || !s.rift.data.Run.Members[200].Eligible || grant.ExperienceLevel != 61 || grant.Exp != 25338000 || grant.MagicFind != mf {
					t.Fatalf("wrong offline grant: %+v", grant)
				}
				if s.rift.data.Run.PenaltyMS != 0 {
					t.Fatal("context event incorrectly counted as player death")
				}
			})
		}
	}
}

func TestRiftOfflineContextPersistsBeforeFloorPublication(t *testing.T) {
	for _, initialLostACK := range []bool{false, true} {
		for _, followup := range []string{"success", "lost_ack", "failed_then_retry"} {
			t.Run(fmt.Sprintf("initial_lost_ack_%v_followup_%s", initialLostACK, followup), func(t *testing.T) {
				s, b, now := seededRiftFloorFixture(t, domain.TrialKeyMember{CharID: 100, Level: 60}, domain.TrialKeyMember{CharID: 200, Level: 60})
				for i := range s.riftLayouts {
					layout := &s.riftLayouts[i]
					layout.WalkableArea = 1000 * domain.RiftFloorAreaPerMonster
					for n := 0; n < 400; n++ {
						layout.Population = append(layout.Population, domain.SpawnPoint{ID: -int32(n + 1), Pos: domain.Pos{MapID: layout.MapID, X: float64(n%20)*80 + 32, Y: float64(n/20)*80 + 32}})
					}
				}
				s.riftRules = rebalanceRiftRulesFixture(t)
				s.rift.data.Run.RulesVersion = 4
				s.rift.data.Run.Rules = s.riftRules.Active()
				s.rift.data.Version = 2
				s.rift.data.Profiles = map[domain.CharID]endgameRiftIdentity{100: {RewardLevel: 60}, 200: {RewardLevel: 60}}
				join(t, s, 2, 200, "offline member", 100, 100)
				recipient := s.players[2]
				recipient.Player.Char.Level = 60
				s.rift.ready[200] = true
				target := prepareRiftFloorTarget(t, s)
				s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
				if len(b.writes) != 1 {
					t.Fatal("missing initial floor write")
				}
				// A change after the immutable next-floor write, before its ACK.
				recipient.Player.Char.Level = 61
				_, _, mf := magicFindGearV5(t, s, recipient)
				s.exec(Leave{ID: recipient.ID, Reason: "disconnect during floor save"})
				b.finishFloorWrite(t, 0, true, initialLostACK)
				s.stepEndgameRift()
				if s.rift.retired || len(b.writes) != 2 || b.cas[1] != 6 || b.writes[1].Revision != 7 {
					t.Fatal("late context not persisted before ownership publication", len(b.writes), b.cas)
				}
				b.finishFloorWrite(t, 1, followup != "failed_then_retry", followup == "lost_ack")
				s.stepEndgameRift()
				if followup == "failed_then_retry" {
					if len(b.writes) != 3 || b.cas[2] != 6 || b.writes[2].Revision != 7 || s.rift.retired {
						t.Fatal("context-only CAS retry incorrect", len(b.writes), b.cas)
					}
					b.finishFloorWrite(t, 2, true, false)
					s.stepEndgameRift()
				}
				if !s.rift.retired {
					t.Fatal("durable context did not release floor")
				}
				admission, ok := s.router.riftAdmission(target.id)
				if !ok || admission.Profiles[200].RewardLevel != 61 || admission.Profiles[200].MagicFind != mf {
					t.Fatal("target received stale offline context", admission.Profiles)
				}
				record := b.runs[s.rift.data.Run.RunID]
				restored, ok := decodeEndgameRift(&record, s.riftRules)
				if !ok || restored.Profiles[200].RewardLevel != 61 || restored.Profiles[200].MagicFind != mf {
					t.Fatal("reconnect durable context stale", restored.Profiles)
				}
				// The target can complete with the recipient still offline; its grant uses
				// the persisted personal observation instead of the older admission level.
				target.levels = riftExperienceAuditLevels()
				target.trial.MaxLevel = 100
				if target.rift == nil {
					if !target.initEndgameRift(admission) {
						t.Fatal("target init")
					}
				}
				target.rift.data.Grants = map[domain.CharID]endgameRiftGrant{}
				target.rift.data.Run.Members[200].Eligible = true
				target.rift.data.Run.Tier = 150
				target.rift.data.Run.Phase = domain.RiftCompleted
				target.generateEndgameRiftGrants()
				g := target.rift.data.Grants[200]
				if g.ExperienceLevel != 61 || g.Exp != 25338000 || g.MagicFind != mf {
					t.Fatal("target offline reward used stale snapshot", g.ExperienceLevel, g.Exp, g.MagicFind)
				}
				*now += 1000
			})
		}
	}
}
