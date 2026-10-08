package domain

// PetItemInfo is an immutable description of the pet behind an inventory UID.
// It carries actual instance values, never a second owning pet instance.
type PetItemSkill struct {
	Name  string
	Level int32
}
type PetItemInfo struct {
	Model, Slot               int32
	Name, Prename, Species    string
	Gender, Habit             uint8
	Opened, Bound             bool
	Level                     int32
	Base                      Base
	Starve, Trust, FreePoints int32
	UsedPoints                int32
	PPAiUsed, PPAiCap         int32
	Skills                    []PetItemSkill
}

func (c *Character) PetByItem(uid int64) *PetInstance {
	if c == nil || uid <= 0 {
		return nil
	}
	for i := range c.Pets {
		if c.Pets[i].ItemUID == uid {
			return &c.Pets[i]
		}
	}
	return nil
}
