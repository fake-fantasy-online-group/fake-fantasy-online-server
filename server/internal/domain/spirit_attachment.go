package domain

import "fmt"

// SpiritAttachment retains the same inventory instance while it is on equipment.
// It is deliberately comparable and contains no mutable reference types.
type SpiritAttachment struct {
	UID    int64
	Item   ItemID
	Bound  bool
	Locked bool
	State  SpiritState
}

func (a SpiritAttachment) Empty() bool {
	return a.UID == 0 && a.Item == 0 && a.State == (SpiritState{}) && !a.Bound && !a.Locked
}
func (a SpiritAttachment) Stack() Stack {
	return Stack{UID: a.UID, Item: a.Item, Count: 1, InstanceKind: ItemInstanceSpirit, Bound: a.Bound, Locked: a.Locked, Spirit: a.State}
}
func SpiritAttachmentOf(st Stack) SpiritAttachment {
	return SpiritAttachment{UID: st.UID, Item: st.Item, Bound: st.Bound, Locked: st.Locked, State: st.Spirit}
}
func ValidateSpiritStack(st Stack) error {
	if st.Empty() {
		if st.Spirit != (SpiritState{}) || !st.FusedSpirit.Empty() || st.SpiritAppearance != 0 {
			return fmt.Errorf("spirit: metadata on empty stack")
		}
		return nil
	}
	if st.InstanceKind == ItemInstanceSpirit || st.Spirit != (SpiritState{}) {
		d, ok := SpiritDefinitionForItem(st.Item)
		if !ok || st.InstanceKind != ItemInstanceSpirit || st.UID <= 0 || st.Count != 1 || st.Spirit.Kind != d.Kind {
			return fmt.Errorf("spirit: invalid inventory identity uid=%d", st.UID)
		}
		if err := st.Spirit.Validate(); err != nil {
			return err
		}
		if st.Endgame != (EndgameItemState{}) || st.Legendary != (LegendaryInstance{}) || st.WingPowers != (WingPowers{}) || st.FusedSoul != 0 || st.FusedAppearance != 0 || st.FusedDragon != 0 || !st.FusedSpirit.Empty() || st.SpiritAppearance != 0 || st.SocketCount != 0 || st.Card != (CardInstanceState{}) {
			return fmt.Errorf("spirit: mixed instance state")
		}
	}
	if !st.FusedSpirit.Empty() {
		if st.UID <= 0 || st.Count != 1 || (st.InstanceKind != ItemInstanceEquipment && st.InstanceKind != ItemInstanceNone) || st.FusedSpirit.UID == st.UID || st.FusedSoul != st.FusedSpirit.Item {
			return fmt.Errorf("spirit: invalid fused identity")
		}
		if err := ValidateSpiritStack(st.FusedSpirit.Stack()); err != nil {
			return err
		}
		if st.FusedSpirit.State.AttrCount == 0 && !st.FusedSpirit.State.LegacyFixed {
			return fmt.Errorf("spirit: sealed fused spirit")
		}
	}
	if st.SpiritAppearance != 0 {
		if st.UID <= 0 || st.Count != 1 || (st.InstanceKind != ItemInstanceEquipment && st.InstanceKind != ItemInstanceNone) {
			return fmt.Errorf("spirit: invalid appearance owner")
		}
		if _, ok := SpiritAppearanceForItem(st.SpiritAppearance); !ok {
			return fmt.Errorf("spirit: unknown appearance")
		}
	}
	return nil
}
