package scene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func v5PetFixture(t *testing.T) (*Scene, *riftTestBackend, *entity.Entity) {
	s, b, _ := riftFixture(t)
	owner := s.players[1]
	owner.Player.Char.Level = 100
	s.petDefs = domain.PetTable{海龟种.ID: 海龟种}
	s.petLvls = domain.NewPetLevelTable(map[int32]int64{78: 100, 79: 100, 80: 100})
	s.petSkills = domain.PetSkillTable{3001: {ID: 3001, MaxLevel: 10, LearnLevel: 1}, 3002: {ID: 3002, MaxLevel: 10, LearnLevel: 1}}
	def := s.petDefs[海龟种.ID]
	def.Learnable = []domain.SkillID{3001, 3002}
	s.petDefs[def.ID] = def
	owner.Player.Char.Pets = []domain.PetInstance{{ID: 900001, Def: def.ID, Level: 79, Prefix: domain.NoPetPrefix, Base: def.BaseAt(79), Deployed: true}, {ID: 900002, Slot: 1, Def: def.ID, Level: 79, Prefix: domain.NoPetPrefix, Base: def.BaseAt(79)}}
	pet := &entity.Entity{ID: 8, Kind: domain.KindPet, Level: 79, HP: 1, MaxHP: 100, Pet: &entity.Pet{Owner: owner.ID, Inst: &owner.Player.Char.Pets[0], Def: def}}
	owner.Player.Pet = 8
	s.entities[8] = pet
	s.pets[8] = pet
	return s, b, owner
}
func TestV5FrozenPetRole100AndSwap(t *testing.T) {
	s, _, owner := v5PetFixture(t)
	owner.Player.StartExperienceBoost(900, 100, s.tick, 100)
	grant := s.freezeEndgamePetGrant(owner, 101, "completion", 100)
	if grant.Experience != 100 || grant.BaseExperience != 101 || grant.BonusPct != 100 || grant.PetInstanceID != 900001 {
		t.Fatal(grant)
	}
	// Swap to same-species second pet and change both bonuses before claiming.
	s.entities[8].Pet.Inst = &owner.Player.Char.Pets[1]
	owner.Player.StartExperienceBoost(1000, 900, s.tick, 100)
	candidate := owner.Player.Char.Clone()
	if err := s.settleEndgamePetGrant(candidate, &grant); err != nil {
		t.Fatal(err)
	}
	if candidate.Pets[0].Level != 80 || candidate.Pets[1].Level != 79 || owner.Player.Char.Pets[0].Level != 79 || grant.AppliedExperience != 100 {
		t.Fatal(candidate.Pets, grant)
	}
	s.applyEndgameSnapshot(owner, domain.Snapshot{Char: candidate, Bag: owner.Player.Bag})
	if s.entities[8].Pet.Inst != owner.Player.Char.FindPet(900002) || s.entities[8].Level != 79 {
		t.Fatal("wrong current pet projection")
	}
	before := candidate.Clone()
	if err := s.settleEndgamePetGrant(candidate, &grant); err != nil || !reflect.DeepEqual(before, candidate) {
		t.Fatal("grant replay mutated", err)
	}
}
func TestV5PetIneligibleAndTransferredConsume(t *testing.T) {
	s, _, owner := v5PetFixture(t)
	owner.Player.Pet = 0
	grant := s.freezeEndgamePetGrant(owner, 1000, "empty", 100)
	owner.Player.Pet = 8
	if err := s.settleEndgamePetGrant(owner.Player.Char, &grant); err != nil || grant.Outcome != "no_deployed_pet" || owner.Player.Char.Pets[0].Level != 79 {
		t.Fatal(grant, err)
	}
	grant = s.freezeEndgamePetGrant(owner, 1000, "transfer", 100)
	owner.Player.Char.Pets = owner.Player.Char.Pets[1:]
	if err := s.settleEndgamePetGrant(owner.Player.Char, &grant); err != nil || grant.Outcome != "pet_no_longer_owned" || !grant.Settled || owner.Player.Char.Pets[0].Level != 79 {
		t.Fatal(grant, err)
	}
}
func TestV5PetLearningRetryDeterministic(t *testing.T) {
	s, _, owner := v5PetFixture(t)
	s.petRule.NaturalLearnChanceBP = 10000
	owner.Player.Char.Pets[0].Level = 78
	grant := s.freezeEndgamePetGrant(owner, 400, "learning", 100)
	a, b := owner.Player.Char.Clone(), owner.Player.Char.Clone()
	ga, gb := grant, grant
	if err := s.settleEndgamePetGrant(a, &ga); err != nil {
		t.Fatal(err)
	}
	// Unrelated scene RNG consumption must not reroll frozen learning.
	for i := 0; i < 100; i++ {
		s.rng.Intn(100)
	}
	if err := s.settleEndgamePetGrant(b, &gb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(ga, gb) || len(a.Pets[0].Skills) == 0 || a.Pets[0].Level != 80 {
		t.Fatal(a.Pets, b.Pets, ga, gb)
	}
}
func TestV5PetEligibilityMatchesOrdinaryEvenIfNotAlive(t *testing.T) {
	s, _, owner := v5PetFixture(t)
	s.entities[8].HP = 0
	if s.experienceEligiblePet(owner) == nil || s.freezeEndgamePetGrant(owner, 200, "zero-hp", 100).Eligibility != "deployed_pet" {
		t.Fatal("added unauthorized alive requirement")
	}
	delete(s.entities, 8)
	if s.freezeEndgamePetGrant(owner, 200, "removed", 100).Eligibility != "no_deployed_pet" {
		t.Fatal("removed pet qualified")
	}
}
func TestV5RiftClaimPersistsPetWithReceipt(t *testing.T) {
	s, b, owner := v5PetFixture(t)
	run, _ := domain.NewRiftRun("v5-claim", 150, 100, []domain.TrialKeyMember{{CharID: 100, Level: 100}}, 0, riftRulesFixture())
	run.Phase = domain.RiftCompleted
	run.Members[100].Eligible = true
	grant := endgameRiftGrant{Exp: 200, ProgressionVersion: domain.ProgressionVersion, Pet: s.freezeEndgamePetGrant(owner, 200, run.RunID, 100)}
	d := endgameRiftPersisted{Version: 2, Run: run, GrantsReady: true, Grants: map[domain.CharID]endgameRiftGrant{100: grant}}
	s.claimEndgameRift(owner, domain.EndgameRequest{Op: "rift.claim", RequestID: "claim-v5", ExpectedRevision: "0"}, d, 1, false)
	if b.commits != 1 || b.plan.Snapshot.Char.Pets[0].Level != 80 || owner.Player.Char.Pets[0].Level != 79 {
		t.Fatal("not detached/atomic", b.commits)
	}
	frozen, ok := decodeEndgameRift(b.plan.Run, riftRulesTableFixture())
	if !ok || !frozen.Grants[100].Pet.Settled || frozen.Grants[100].Pet.PetInstanceID != 900001 {
		t.Fatal("no grant in run transaction", frozen)
	}
	b.finish(true)
	s.finishEndgame(false)
	if owner.Player.Char.Pets[0].Level != 80 || s.entities[8].Pet.Inst != owner.Player.Char.FindPet(900001) {
		t.Fatal("published pet pointer stale")
	}
}
func TestV5LegacyRiftNoRetroactivePet(t *testing.T) {
	s, _, owner := v5PetFixture(t)
	run, _ := domain.NewRiftRun("legacy", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, 0, riftRulesFixture())
	d := endgameRiftPersisted{Version: 1, Run: run, Grants: map[domain.CharID]endgameRiftGrant{100: {Exp: 10000}}, GrantsReady: true}
	raw, _ := json.Marshal(d)
	record := &domain.EndgameRunRecord{RunID: run.RunID, Payload: string(raw)}
	decoded, ok := decodeEndgameRift(record, riftRulesTableFixture())
	if !ok {
		t.Fatal("v1 rejected")
	}
	grant := decoded.Grants[100]
	if err := s.settleEndgamePetGrant(owner.Player.Char, &grant.Pet); err != nil || owner.Player.Char.Pets[0].Level != 79 || grant.Pet.Settled {
		t.Fatal("backfilled legacy pet", err)
	}
}

func TestV5TrialCompletionFreezesExperienceAndPet(t *testing.T) {
	s, b, owner := v5PetFixture(t)
	s.trialLevel = 100
	s.trial.MaxLevel = 100
	s.trial.FirstPct = 100
	s.levels = domain.NewLevelTable(map[int32]int64{100: 3200})
	s.trial.Entrances = []domain.TrialEntrance{{ClaimMap: 7, ClaimNPC: "试炼小仙"}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "试炼小仙"}
	owner.Player.QuestNPC = "试炼小仙"
	s.finishEndgameTrialKeys()
	s.stepEndgameTrial()
	if s.trialRun == nil || !s.trialRun.Saved {
		t.Fatal("no durable completion")
	}
	c := s.trialRun.Completion
	if c.Version != 2 || c.Grants[100].Reward.Exp != 200 || c.Grants[100].Pet.Experience != 100 {
		t.Fatal(c)
	}
	s.assignEndgameTrial(owner, c)
	s.entities[8].Pet.Inst = &owner.Player.Char.Pets[1]
	owner.Player.StartExperienceBoost(1000, 900, s.tick, 100)
	// Claim-time curve/rule changes must not reprice a completed trial.
	s.trial.FirstPct = 900
	s.levels = domain.NewLevelTable(map[int32]int64{100: 6400})
	s.completeTrial(owner)
	first := b.plan.Snapshot.Char.Clone()
	if b.commits != 1 || first.Pets[0].Level != 80 || first.Pets[1].Level != 79 || first.Level != 100 {
		t.Fatal("bad candidate", first, b.commits)
	}
	var persisted endgameTrialCompletion
	if err := json.Unmarshal([]byte(b.plan.Run.Payload), &persisted); err != nil || !persisted.Grants[100].Claimed || !persisted.Grants[100].Pet.Settled {
		t.Fatal("not in same transaction", persisted, err)
	}
	b.finish(false)
	s.finishEndgame(false)
	if owner.Player.Char.Pets[0].Level != 79 {
		t.Fatal("failed commit published pet")
	}
	completeTrialReadyForTest(s, owner)
	if b.commits != 2 || !reflect.DeepEqual(first.Pets, b.plan.Snapshot.Char.Pets) {
		t.Fatal("retry rerolled", b.commits, first.Pets, b.plan.Snapshot.Char.Pets)
	}
	b.finish(true)
	s.finishEndgame(false)
	if owner.Player.Char.Pets[0].Level != 80 || owner.Player.Char.Pets[1].Level != 79 {
		t.Fatal("bad publication")
	}
	completeTrialReadyForTest(s, owner)
	if b.commits != 2 {
		t.Fatal("duplicate trial claim")
	}
}

func TestV5LostAckRebindsPetAuthoritativeAggregate(t *testing.T) {
	s, b, owner := v5PetFixture(t)
	candidate := s.snapshotOf(owner).Clone()
	candidate.Char.Pets[0].Level = 80
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "lost-ack", ExpectedRevision: "0"}
	if !s.endgameCommit(owner, req, candidate, domain.EndgameResponse{OK: true}, nil, 0, nil) {
		t.Fatal("not submitted")
	}
	b.commit <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt, Replayed: true, Authoritative: &candidate}
	s.finishEndgame(false)
	if owner.Player.Char.Pets[0].Level != 80 || s.entities[8].Pet.Inst != owner.Player.Char.FindPet(900001) || s.entities[8].Level != 80 {
		t.Fatal("replayed authoritative pet stale")
	}
}
