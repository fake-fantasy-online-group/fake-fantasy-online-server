package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// An independent deterministic reward stream cannot perturb gear rolls. Once
// generated, the stack joins the existing durable ground/pickup entitlement.
func (s *Scene) riftSpiritEssenceStacks(char domain.CharID) ([]domain.Stack, error) {
	r := s.rift.data.Run
	if r.Rules.Version < domain.RiftSpiritRewardVersion {
		return nil, nil
	}
	if r.Phase != domain.RiftCompleted && r.Phase != domain.RiftClosed {
		return nil, nil
	}
	member := r.Members[char]
	if member == nil || !member.Eligible {
		return nil, nil
	}
	lo, hi := domain.SpiritRiftQuantityBounds(r.Tier)
	if lo == 0 {
		return nil, fmt.Errorf("invalid spirit reward tier")
	}
	defs := domain.SpiritDefinitions()
	// Validate the complete pool even if this particular roll selects another ID.
	for _, spirit := range defs {
		def, ok := s.itemDef(spirit.EssenceID)
		if !ok || !def.Stackable || def.InstanceKind != domain.ItemInstanceNone {
			return nil, fmt.Errorf("missing spirit essence %d", spirit.EssenceID)
		}
	}
	rng := s.riftCumulativeRandom(char, 0, "spirit-essence-v19")
	spirit := defs[rng.Intn(len(defs))]
	quantity := lo + int32(rng.Intn(int(hi-lo+1)))
	return []domain.Stack{{Item: spirit.EssenceID, Count: quantity}}, nil
}

// Existing native shop packets are reused. Only the nine new shops need this
// additional exact-NPC proximity check; unrelated historical shops are unchanged.
func (s *Scene) canUseSpiritShop(p *entity.Entity, shop int32) bool {
	def, spiritShop := domain.SpiritDefinitionForShop(shop)
	if !spiritShop {
		return true
	}
	if p == nil || p.Player == nil || !p.Player.EndgameSpirit {
		return false
	}
	for _, npc := range s.entities {
		if npc == nil || npc.NPC == nil || npc.NPC.Sell != shop {
			continue
		}
		actual, ok := s.spiritNPC(p, npc.ID)
		if ok && actual.Kind == def.Kind {
			return true
		}
	}
	return false
}
