package domain

type ChatScope uint8

const (
	ChatScopeMap ChatScope = iota + 1
	ChatScopeParty
	ChatScopeFamily
	ChatScopeWorld
)

type ChatChannelRule struct {
	Channel    uint8
	Scope      ChatScope
	CooldownMS int32
	MaxShares  int32
}

type ChatChannelRules map[uint8]ChatChannelRule

type ChatShare struct {
	Kind                                 uint8
	Item                                 ItemID
	Quality                              uint8
	Name                                 string
	Desc                                 string
	Pet                                  *PetItemInfo
	PPAiUsed, PPAiCap, PPShUsed, PPShCap int32
}
