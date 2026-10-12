package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Frozen at the authoritative completion event; never reconstructed from the
// claim-time deployed pet, bonus, character experience, or a species identity.
type endgamePetGrant struct {
	Version            string
	PetInstanceID      domain.PetInstID
	BaseExperience     int64
	BonusPct           int32
	Experience         int64
	Eligibility        string
	LearningSeed       int64
	Settled            bool
	Outcome            string
	AppliedExperience  int64
	FromLevel, ToLevel int32
}

// Shared predicate intentionally has no Alive/last-hit requirement. Death and
// recall remove the deployment; same-tick ordering is the scene event order.
func (s *Scene) experienceEligiblePet(owner *entity.Entity) *entity.Entity {
	if owner == nil || owner.Player == nil || owner.Player.Pet == 0 {
		return nil
	}
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return nil
	}
	return pet
}

func (s *Scene) endgameRewardPlayer(char domain.CharID) *entity.Entity {
	for _, p := range s.players {
		if p.Player != nil && domain.CharID(p.Player.Char.ID) == char {
			return p
		}
	}
	return nil
}

func (s *Scene) freezeEndgamePetGrant(owner *entity.Entity, base int64, completion string, char domain.CharID) endgamePetGrant {
	g := endgamePetGrant{Version: domain.ProgressionVersion, BaseExperience: base, Eligibility: "no_deployed_pet"}
	if owner == nil {
		g.Eligibility = "owner_not_present"
		return g
	}
	pet := s.experienceEligiblePet(owner)
	if pet == nil {
		return g
	}
	g.PetInstanceID = pet.Pet.Inst.ID
	g.BonusPct = owner.Player.PetExperienceBonusPct(s.tick)
	var err error
	g.Experience, err = domain.PetRewardExperience(base, g.BonusPct)
	if err != nil {
		g.Eligibility = "invalid_experience"
		return g
	}
	g.Eligibility = "deployed_pet"
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/pet-learning/%d/%d/%s", completion, char, g.PetInstanceID, g.Version)))
	g.LearningSeed = int64(binary.BigEndian.Uint64(hash[:8]))
	return g
}

// Runs only against the detached candidate. The caller commits this candidate,
// the mutated grant/run and receipt together. Retries use the same learning RNG.
func (s *Scene) settleEndgamePetGrant(ch *domain.Character, g *endgamePetGrant) error {
	if g.Version == "" {
		return nil
	} // V1 has no completion-time pet evidence.
	if g.Version != domain.ProgressionVersion {
		return fmt.Errorf("unknown pet progression version %q", g.Version)
	}
	if g.Settled {
		return nil
	}
	g.Settled = true
	if g.Eligibility != "deployed_pet" {
		g.Outcome = g.Eligibility
		return nil
	}
	inst := ch.FindPet(g.PetInstanceID)
	if inst == nil {
		g.Outcome = "pet_no_longer_owned"
		return nil
	}
	def, ok := s.petDefs[inst.Def]
	if !ok || s.petLvls == nil {
		return fmt.Errorf("missing frozen pet progression data")
	}
	prefix, _ := s.petPrefixes.Find(inst.Prefix)
	res := inst.AddExpWithPrefix(def, prefix, s.petLvls, g.Experience, domain.PetCapFor(def, ch.Level))
	if res.Reason != "" {
		return fmt.Errorf("pet progression: %s", res.Reason)
	}
	g.FromLevel, g.ToLevel = res.FromLevel, res.ToLevel
	g.AppliedExperience = res.AppliedExp
	g.Outcome = "granted"
	if res.AppliedExp == 0 {
		g.Outcome = "pet_capped"
	}
	rng := rand.New(rand.NewSource(g.LearningSeed))
	for lv := res.FromLevel + 1; lv <= res.ToLevel; lv++ {
		s.resolvePetSkillLevelUpWithRNG(inst, def, lv, rng.Intn)
	}
	if res.Levels > 0 {
		inst.HP, inst.MP = inst.MaxHP(def), inst.MaxMP(def)
	}
	return nil
}

// Rebind the entity projection after ANY aggregate replacement (including
// lost-ACK recovery), so subsequent combat/snapshots cannot overwrite new pets.
func (s *Scene) syncEndgamePetProjection(owner *entity.Entity) {
	e := s.experienceEligiblePet(owner)
	if e == nil {
		return
	}
	inst := owner.Player.Char.FindPet(e.Pet.Inst.ID)
	if inst == nil {
		s.recallPet(owner, event.PetRecalledByOwnerLeft)
		return
	}
	e.Pet.Inst = inst
	e.Level = inst.Level
	e.Stats = domain.DeriveFromBase(inst.Base)
	e.MaxHP, e.MaxMP = inst.MaxHP(e.Pet.Def), inst.MaxMP(e.Pet.Def)
	e.Stats.MaxHP, e.Stats.MaxMP = e.MaxHP, e.MaxMP
	e.HP, e.MP = inst.HP, inst.MP
}

func (s *Scene) publishEndgamePetGrant(owner *entity.Entity, g endgamePetGrant) {
	if !g.Settled || g.Version == "" {
		return
	}
	if g.PetInstanceID != 0 && owner.Player.Char.FindPet(g.PetInstanceID) != nil {
		s.emitTo(owner.ID, event.PetExpGained{Who: owner.ID, Inst: g.PetInstanceID, Exp: g.AppliedExperience, Level: g.ToLevel, Ups: g.ToLevel - g.FromLevel})
	}
	s.pushPetSnapshot(owner)
}
