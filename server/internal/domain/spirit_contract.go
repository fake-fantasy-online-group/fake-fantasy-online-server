package domain

import "strconv"

const EndgameSpiritV1 = "endgame.spirit.v1"

// SpiritSelection reserves an existing bag slot, never a client-created item.
// Slot is page-relative, UID is mandatory once an instance exists.
type SpiritSelection struct {
	Tab    int32  `json:"tab"`
	Slot   int32  `json:"slot"`
	ItemID ItemID `json:"itemId"`
	UID    string `json:"uid,omitempty"`
}
type SpiritDefinitionView struct {
	Kind         int32  `json:"kind"`
	Name         string `json:"name"`
	ItemID       ItemID `json:"itemId"`
	EssenceID    ItemID `json:"essenceId"`
	SeparatorID  ItemID `json:"separatorId"`
	NPCID        int32  `json:"npcId"`
	NPCName      string `json:"npcName"`
	NPCScript    string `json:"npcScript"`
	MapID        int32  `json:"mapId"`
	Slot         int32  `json:"slot"`
	UIKey        string `json:"uiKey"`
	ProcessMoney string `json:"processMoney"`
}
type SpiritAttributeView struct {
	Attr         int32 `json:"attr"`
	Value        int32 `json:"value"`
	Permanent    int32 `json:"permanent"`
	Last         int32 `json:"last"`
	Level        int32 `json:"level"`
	PerfectLevel int32 `json:"perfectLevel"`
}
type SpiritView struct {
	Kind            int32                 `json:"kind"`
	ItemID          ItemID                `json:"itemId"`
	Sealed          bool                  `json:"sealed"`
	Level           int32                 `json:"level"`
	Attrs           []SpiritAttributeView `json:"attrs"`
	CanUnseal       bool                  `json:"canUnseal"`
	CanProcess      bool                  `json:"canProcess"`
	CanRollback     bool                  `json:"canRollback"`
	CanReset        bool                  `json:"canReset"`
	UnsealEssence   int32                 `json:"unsealEssence"`
	ProcessEssence  int32                 `json:"processEssence"`
	RollbackEssence int32                 `json:"rollbackEssence"`
	ResetEssence    int32                 `json:"resetEssence"`
	ProcessMoney    string                `json:"processMoney"`
}

func SpiritProcessEssence(level int32) int32 {
	if level < 5 {
		return 2
	}
	if level < 10 {
		return 4
	}
	return 5
}
func NewSpiritView(item ItemID, state SpiritState) *SpiritView {
	def, ok := SpiritDefinitionForItem(item)
	if !ok {
		return nil
	}
	v := &SpiritView{Kind: int32(def.Kind), ItemID: item, Sealed: state.AttrCount == 0 && !state.LegacyFixed, Level: state.Level(), Attrs: []SpiritAttributeView{}, CanUnseal: state.CanUnseal(), CanProcess: state.CanProcess(), CanRollback: state.CanRollback(), CanReset: state.CanReset(), UnsealEssence: 2, ProcessEssence: SpiritProcessEssence(state.Level()), RollbackEssence: 1, ResetEssence: 200, ProcessMoney: strconv.FormatInt(def.ProcessMoney, 10)}
	values, permanent, last := state.Values(), state.PermanentValues(), state.LastValues()
	levels, perfect := state.Levels(), state.PerfectLevels()
	for i := 0; i < int(state.AttrCount); i++ {
		v.Attrs = append(v.Attrs, SpiritAttributeView{Attr: state.Attrs[i], Value: values[i], Permanent: permanent[i], Last: last[i], Level: levels[i], PerfectLevel: perfect[i]})
	}
	return v
}
func SpiritDefinitionViews() []SpiritDefinitionView {
	out := []SpiritDefinitionView{}
	for _, d := range SpiritDefinitions() {
		out = append(out, SpiritDefinitionView{Kind: int32(d.Kind), Name: d.Name, ItemID: d.ItemID, EssenceID: d.EssenceID, SeparatorID: d.SeparatorID, NPCID: d.NPCID, NPCName: d.NPCName, NPCScript: d.NPCScript, MapID: d.MapID, Slot: int32(d.Slot), UIKey: d.UIKey, ProcessMoney: strconv.FormatInt(d.ProcessMoney, 10)})
	}
	return out
}
