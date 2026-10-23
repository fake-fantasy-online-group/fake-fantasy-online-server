package domain

// LegendaryRuntimeTransfer is a typed server-to-server handoff, never client
// input. Owner-bound state is rebased; foreign-target marks stay in their map.
type LegendaryRuntimeTransfer struct {
	States    []LegendaryTransferredState
	Cooldowns []LegendaryTransferredCooldown
}
type LegendaryTransferredState struct {
	VisualStage       string
	ExpirySentinel    int64
	ValueIsTimestamp  bool
	ValueAgeMS        int64
	SourceUID         int64
	PowerID, Phase    string
	SelfTarget        bool
	Stacks            int32
	Value             int64
	NextInMS          int64
	RemainingMS       int64
	Permanent         bool
	Name, Description string
	Unit              string
	IconID            int32
	Good              bool
}
type LegendaryTransferredCooldown struct {
	SourceUID   int64
	PowerID     string
	RemainingMS int64
}

// LegendarySourceRef is the accepted equipment identity for a persistent skill.
// Reconciliation may retain or remove these sources, never acquire new gear.
type LegendarySourceRef struct {
	PowerID   string
	SourceUID int64
}
