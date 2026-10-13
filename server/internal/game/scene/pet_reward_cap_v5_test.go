package scene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func v5OverOwnerPetFixture(t *testing.T) (*Scene, *riftTestBackend, *entity.Entity) {
	t.Helper()
	s, b, owner := v5PetFixture(t)
	owner.Player.Char.Level, owner.Player.Char.Exp = 60, 19
	s.levels = domain.NewLevelTable(map[int32]int64{50: 3200, 60: 10000})
	inst := &owner.Player.Char.Pets[0]
	inst.Level, inst.Exp, inst.Base = 65, 37, 海龟种.BaseAt(65)
	inst.HP, inst.MP = s.entities[8].HP, s.entities[8].MP
	s.entities[8].Level = inst.Level
	// The underlying fixture joined at level 1. Bring the entity and persisted
	// compatibility attributes in sync before comparing full retry snapshots.
	owner.Level = owner.Player.Char.Level
	s.refreshStats(owner)
	return s, b, owner
}

func assertV5OverOwnerPetCapped(t *testing.T, grant endgamePetGrant) {
	t.Helper()
	if !grant.Settled || grant.Outcome != "pet_capped" || grant.AppliedExperience != 0 || grant.FromLevel != 65 || grant.ToLevel != 65 {
		t.Fatalf("expected settled zero-XP capped pet grant: %+v", grant)
	}
}

func TestV5RiftClaimWithLegalOverOwnerPet(t *testing.T) {
	s, b, owner := v5OverOwnerPetFixture(t)
	before := owner.Player.Char.Clone()
	run, err := domain.NewRiftRun("v5-over-owner", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, 0, riftRulesFixture())
	if err != nil {
		t.Fatal(err)
	}
	run.Phase = domain.RiftCompleted
	run.Members[100].Eligible = true
	grant := endgameRiftGrant{Exp: 200, ProgressionVersion: domain.ProgressionVersion, Pet: s.freezeEndgamePetGrant(owner, 200, run.RunID, 100)}
	d := endgameRiftPersisted{Version: 2, Run: run, GrantsReady: true, Grants: map[domain.CharID]endgameRiftGrant{100: grant}}
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "claim-v5-over-owner", ExpectedRevision: "0"}
	s.claimEndgameRift(owner, req, d, 1, false)
	if b.commits != 1 {
		t.Fatal("legal over-owner pet blocked the character reward")
	}
	first := b.plan.Snapshot.Char.Clone()
	if first.Level != 60 || first.Exp != before.Exp+200 || !reflect.DeepEqual(first.Pets, before.Pets) || owner.Player.Char.Exp != before.Exp {
		t.Fatal("reward was not detached or changed the capped pet", first)
	}
	persisted, ok := decodeEndgameRift(b.plan.Run, riftRulesTableFixture())
	if !ok || !persisted.Run.Members[100].Claimed {
		t.Fatal("claim was not included in the reward transaction")
	}
	assertV5OverOwnerPetCapped(t, persisted.Grants[100].Pet)
	b.finish(false)
	s.finishEndgame(false)
	if owner.Player.Char.Exp != before.Exp || !reflect.DeepEqual(owner.Player.Char.Pets, before.Pets) || d.Run.Members[100].Claimed || d.Grants[100].Pet.Settled {
		t.Fatal("failed claim published state or consumed the entitlement")
	}
	s.claimEndgameRift(owner, req, d, 1, false)
	if b.commits != 2 || !reflect.DeepEqual(first, b.plan.Snapshot.Char) {
		t.Fatal("retry changed the detached reward", b.commits)
	}
	b.finish(true)
	s.finishEndgame(false)
	if owner.Player.Char.Exp != before.Exp+200 || !reflect.DeepEqual(owner.Player.Char.Pets, before.Pets) || s.entities[8].Pet.Inst != owner.Player.Char.FindPet(900001) || s.entities[8].Level != 65 {
		t.Fatal("committed character reward or pet projection incorrect")
	}
	s.claimEndgameRift(owner, domain.EndgameRequest{Op: "rift.claim", RequestID: "claim-v5-over-owner-again", ExpectedRevision: "1"}, persisted, 2, false)
	if b.commits != 2 || owner.Player.Char.Exp != before.Exp+200 {
		t.Fatal("duplicate claim awarded experience")
	}
}

func TestV5TrialClaimWithLegalOverOwnerPet(t *testing.T) {
	s, b, owner := v5OverOwnerPetFixture(t)
	s.trialLevel, s.trial.FirstPct = 60, 100
	s.trial.Entrances = []domain.TrialEntrance{{ClaimMap: 7, ClaimNPC: "试炼小仙"}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "试炼小仙"}
	owner.Player.QuestNPC = "试炼小仙"
	s.finishEndgameTrialKeys()
	s.stepEndgameTrial()
	if s.trialRun == nil || !s.trialRun.Saved {
		t.Fatal("completion was not saved")
	}
	completion := s.trialRun.Completion
	frozen := completion.Grants[100]
	if frozen.Reward.Exp <= 0 || frozen.Pet.Experience <= 0 {
		t.Fatal("fixture must freeze positive character and pet experience")
	}
	s.assignEndgameTrial(owner, completion)
	before := owner.Player.Char.Clone()
	s.completeTrial(owner)
	if b.commits != 1 {
		t.Fatal("legal over-owner pet blocked the character reward")
	}
	first := b.plan.Snapshot.Char.Clone()
	if first.Level != 60 || first.Exp != before.Exp+frozen.Reward.Exp || !reflect.DeepEqual(first.Pets, before.Pets) || owner.Player.Char.Exp != before.Exp || b.plan.Snapshot.Bag.CountOf(domain.RiftKeyItemID) == 0 {
		t.Fatal("reward was not detached or changed the capped pet", first)
	}
	var persisted endgameTrialCompletion
	if err := json.Unmarshal([]byte(b.plan.Run.Payload), &persisted); err != nil || !persisted.Grants[100].Claimed {
		t.Fatal("claim was not included in the reward transaction", err)
	}
	assertV5OverOwnerPetCapped(t, persisted.Grants[100].Pet)
	b.finish(false)
	s.finishEndgame(false)
	if owner.Player.Char.Exp != before.Exp || !reflect.DeepEqual(owner.Player.Char.Pets, before.Pets) || owner.Player.Bag.CountOf(domain.RiftKeyItemID) != 0 {
		t.Fatal("failed claim published state")
	}
	completeTrialReadyForTest(s, owner)
	if b.commits != 2 || !reflect.DeepEqual(first, b.plan.Snapshot.Char) {
		t.Fatal("retry changed the detached reward", b.commits)
	}
	b.finish(true)
	s.finishEndgame(false)
	if owner.Player.Char.Exp != before.Exp+frozen.Reward.Exp || !reflect.DeepEqual(owner.Player.Char.Pets, before.Pets) || owner.Player.Bag.CountOf(domain.RiftKeyItemID) == 0 || s.entities[8].Pet.Inst != owner.Player.Char.FindPet(900001) || s.entities[8].Level != 65 {
		t.Fatal("committed character reward or pet projection incorrect")
	}
	completeTrialReadyForTest(s, owner)
	if b.commits != 2 || owner.Player.Char.Exp != before.Exp+frozen.Reward.Exp {
		t.Fatal("duplicate claim awarded experience")
	}
}

func TestV5EndgamePetGrantRejectsHardCapViolations(t *testing.T) {
	for _, tc := range []struct {
		name       string
		level, max int32
	}{
		{name: "absolute cap", level: 81, max: 100},
		{name: "species cap", level: 65, max: 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, owner := v5OverOwnerPetFixture(t)
			inst := &owner.Player.Char.Pets[0]
			inst.Level = tc.level
			def := s.petDefs[inst.Def]
			def.MaxLevel = tc.max
			s.petDefs[inst.Def] = def
			before := owner.Player.Char.Clone()
			grant := s.freezeEndgamePetGrant(owner, 200, "invalid-save", 100)
			if err := s.settleEndgamePetGrant(owner.Player.Char, &grant); err == nil || grant.Outcome == "pet_capped" || !reflect.DeepEqual(before, owner.Player.Char) {
				t.Fatal("invalid pet save was treated as a legal capped reward", err, grant)
			}
		})
	}
}
