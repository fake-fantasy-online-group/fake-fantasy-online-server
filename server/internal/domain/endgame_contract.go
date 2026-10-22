package domain

// EndgameV1 is explicitly negotiated; native positional packets remain unchanged.
const EndgameV1 = "endgame.v1"
const EndgameNativeStatsV1 = "endgame.native-stats.v1"
const EndgameGrowthV5 = "endgame.growth.v5"
const EndgameRiftPortalsV1 = "endgame.rift-portals.v1"
const EndgameRiftPagesV1 = "endgame.rift-pages.v1"
const EndgameRiftEntranceV1 = "endgame.rift-entrance.v1"
const EndgameCubeInputsV1 = "endgame.cube-inputs.v1"
const EndgameSchema = 1
const EndgameMaxJSONBytes = 60 * 1024

// EndgameItemState is server-owned metadata on the existing durable equipment UID.
// UniqueID is the base ItemID in a separate namespace. Zero means ordinary gear.
type EndgameItemState struct {
	Revision     int64
	UniqueID     int32
	UniqueRollBP int32
	// Zero selects the frozen V4 catalog; versions are never inferred on load.
	UniqueContentVersion    int32
	FormationContentVersion int32
	// FormationRollBP is generated once for new equipment. Legacy zero stays midpoint.
	FormationRollBP int32
}

type EndgameRequest struct {
	SkillID          int32       `json:"skillId,omitempty"`
	Op               string      `json:"op"`
	RequestID        string      `json:"requestId,omitempty"`
	ExpectedRevision string      `json:"expectedRevision,omitempty"`
	Capabilities     []string    `json:"capabilities,omitempty"`
	Catalog          string      `json:"catalog,omitempty"`
	Cursor           string      `json:"cursor,omitempty"`
	TargetUID        string      `json:"targetUid,omitempty"`
	RuneUID          string      `json:"runeUid,omitempty"`
	Socket           int32       `json:"socket,omitempty"`
	RecipeID         int32       `json:"recipeId,omitempty"`
	QuoteID          string      `json:"quoteId,omitempty"`
	RunID            string      `json:"runId,omitempty"`
	Tier             int32       `json:"tier,omitempty"`
	NPCEntityID      EntityID    `json:"npcEntityId,omitempty"`
	FloorIndex       int32       `json:"floorIndex,omitempty"`
	ItemUIDs         []string    `json:"itemUids,omitempty"`
	CubeInputs       []CubeInput `json:"cubeInputs,omitempty"`
}

func (r EndgameRequest) Mutates() bool {
	switch r.Op {
	case "skill.upgrade", "socket.insert", "socket.remove", "cube.execute", "cube.recover", "rift.start", "rift.claim", "rift.leave", "rift.advance":
		return true
	}
	return false
}

type EndgameStatView struct {
	Attr  int32 `json:"attr"`
	Value int32 `json:"value"`
	Mode  int32 `json:"mode"`
}
type EndgameSocketView struct {
	Name   string `json:"name,omitempty"`
	Index  int32  `json:"index"`
	ItemID int32  `json:"itemId"`
	UID    string `json:"uid,omitempty"`
}
type EndgameItemView struct {
	Tab            int32               `json:"tab"`
	Level          int32               `json:"level,omitempty"`
	Profession     int32               `json:"profession,omitempty"`
	UID            string              `json:"uid"`
	ItemID         int32               `json:"itemId"`
	Location       string              `json:"location"`
	Slot           int32               `json:"slot"`
	Revision       string              `json:"revision"`
	UniqueID       int32               `json:"uniqueId,omitempty"`
	UniqueRollBP   int32               `json:"uniqueRollBp,omitempty"`
	Name           string              `json:"name,omitempty"`
	Description    string              `json:"description,omitempty"`
	Trait          string              `json:"trait,omitempty"`
	SocketCount    int32               `json:"socketCount"`
	Sockets        []EndgameSocketView `json:"sockets,omitempty"`
	Stats          []EndgameStatView   `json:"stats,omitempty"`
	FormationID    int32               `json:"formationId,omitempty"`
	Active         bool                `json:"active,omitempty"`
	InactiveReason string              `json:"inactiveReason,omitempty"`
	Bound          bool                `json:"bound"`
	Locked         bool                `json:"locked"`
}
type EndgameRuneView struct {
	MinLevel    int32             `json:"minLevel"`
	Mansion     string            `json:"mansion,omitempty"`
	ItemID      int32             `json:"itemId"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Quadrant    string            `json:"quadrant,omitempty"`
	Stats       []EndgameStatView `json:"stats,omitempty"`
}
type EndgameFormationView struct {
	ID             int32             `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Channel        string            `json:"channel"`
	Runes          []int32           `json:"runes"`
	Stats          []EndgameStatView `json:"stats,omitempty"`
	MinLevel       int32             `json:"minLevel,omitempty"`
	Professions    int32             `json:"professions,omitempty"`
	Slots          []int32           `json:"slots,omitempty"`
	Active         bool              `json:"active,omitempty"`
	InactiveReason string            `json:"inactiveReason,omitempty"`
}
type EndgameMaterialView struct {
	Owned  int32  `json:"owned,omitempty"`
	ItemID int32  `json:"itemId"`
	Count  int32  `json:"count"`
	Name   string `json:"name,omitempty"`
}
type EndgameRecipeView struct {
	ID             int32                 `json:"id"`
	Name           string                `json:"name"`
	Description    string                `json:"description,omitempty"`
	Operation      string                `json:"operation"`
	Money          string                `json:"money"`
	Materials      []EndgameMaterialView `json:"materials,omitempty"`
	OutputItemID   int32                 `json:"outputItemId,omitempty"`
	TargetRequired bool                  `json:"targetRequired,omitempty"`
}
type EndgameQuoteView struct {
	QuoteID     string                `json:"quoteId"`
	RecipeID    int32                 `json:"recipeId"`
	TargetUID   string                `json:"targetUid,omitempty"`
	Revision    string                `json:"revision"`
	Money       string                `json:"money"`
	Materials   []EndgameMaterialView `json:"materials,omitempty"`
	ExpiresAt   string                `json:"expiresAt,omitempty"`
	Description string                `json:"description,omitempty"`
}
type EndgameRiftMemberView struct {
	Name     string `json:"name,omitempty"`
	CharID   string `json:"charId"`
	Level    int32  `json:"level"`
	Eligible bool   `json:"eligible"`
	Claimed  bool   `json:"claimed"`
}
type EndgameRiftAuraView struct {
	GoldenUntilMS int64    `json:"goldenUntilMS,omitempty"`
	CloneOf       string   `json:"cloneOf,omitempty"`
	AbilityIDs    []int32  `json:"abilityIds,omitempty"`
	OriginalName  string   `json:"originalName,omitempty"`
	EntityID      string   `json:"entityId"`
	Class         int32    `json:"class"`
	Affixes       []string `json:"affixes,omitempty"`
}
type EndgameRiftGlobeView struct {
	EntityID string `json:"entityId"`
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	Value    int32  `json:"value"`
}
type EndgameRiftTelegraphView struct {
	ID          string `json:"id"`
	EntityID    string `json:"entityId"`
	Ability     string `json:"ability"`
	Shape       string `json:"shape"`
	X           int32  `json:"x"`
	Y           int32  `json:"y"`
	EndX        int32  `json:"endX"`
	EndY        int32  `json:"endY"`
	Radius      int32  `json:"radius"`
	InnerRadius int32  `json:"innerRadius"`
	RemainingMS int32  `json:"remainingMS"`
}
type EndgameRiftGuardianArrivalView struct {
	ID           string `json:"id"`
	Seed         string `json:"seed"`
	AnchorCharID string `json:"anchorCharId"`
	X            int32  `json:"x"`
	Y            int32  `json:"y"`
	StartedAtMS  int64  `json:"startedAtMS"`
	DeadlineMS   int64  `json:"deadlineMS"`
	DurationMS   int32  `json:"durationMS"`
	RemainingMS  int32  `json:"remainingMS"`
}

// Optional JSON-only v6 extension. No native binary packet layout changes.
type EndgameRiftHazardView struct {
	ID            string                `json:"id"`
	SourceID      string                `json:"sourceId"`
	AbilityID     int32                 `json:"abilityId"`
	Mechanism     string                `json:"mechanism"`
	X             int32                 `json:"x"`
	Y             int32                 `json:"y"`
	Radius        int32                 `json:"radius"`
	Width         int32                 `json:"width,omitempty"`
	BornMS        int64                 `json:"bornMS"`
	ExpiresMS     int64                 `json:"expiresMS"`
	RemainingMS   int32                 `json:"remainingMS"`
	FirstPulseMS  int64                 `json:"firstPulseMS,omitempty"`
	PulseMS       int64                 `json:"pulseMS,omitempty"`
	PropagationMS int64                 `json:"propagationMS,omitempty"`
	Walls         []EndgameRiftWallView `json:"walls,omitempty"`
}
type EndgameRiftWallView struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	X2    float64 `json:"x2"`
	Y2    float64 `json:"y2"`
	Width float64 `json:"width"`
}
type EndgameRiftImpactView struct {
	ID        string `json:"id"`
	Mechanism string `json:"mechanism"`
	X         int32  `json:"x"`
	Y         int32  `json:"y"`
	Radius    int32  `json:"radius"`
	AtMS      int64  `json:"atMS"`
	ExpiresMS int64  `json:"expiresMS"`
}
type EndgameRiftView struct {
	Impacts []EndgameRiftImpactView `json:"impacts,omitempty"`

	MapID       int32                   `json:"mapId,omitempty"`
	ServerNowMS int64                   `json:"serverNowMS,omitempty"`
	Hazards     []EndgameRiftHazardView `json:"hazards,omitempty"`

	GuardianArrival  *EndgameRiftGuardianArrivalView `json:"guardianArrival,omitempty"`
	PendingLoot      int32                           `json:"pendingLoot,omitempty"`
	Seed             string                          `json:"seed,omitempty"`
	GeneratorVersion int32                           `json:"generatorVersion,omitempty"`
	FloorIndex       int32                           `json:"floorIndex,omitempty"`
	MapName          string                          `json:"mapName,omitempty"`
	CanAdvance       bool                            `json:"canAdvance,omitempty"`
	Transitioning    bool                            `json:"transitioning,omitempty"`
	ExitX            int32                           `json:"exitX,omitempty"`
	ExitY            int32                           `json:"exitY,omitempty"`
	MaxTier          int32                           `json:"maxTier,omitempty"`
	UnlockedTier     int32                           `json:"unlockedTier,omitempty"`
	Telegraphs       []EndgameRiftTelegraphView      `json:"telegraphs,omitempty"`

	Name             string                  `json:"name,omitempty"`
	KeyCount         int32                   `json:"keyCount"`
	KeyItemID        int32                   `json:"keyItemId"`
	Auras            []EndgameRiftAuraView   `json:"auras,omitempty"`
	Globes           []EndgameRiftGlobeView  `json:"globes,omitempty"`
	RunID            string                  `json:"runId"`
	Revision         string                  `json:"revision"`
	Phase            string                  `json:"phase"`
	Tier             int32                   `json:"tier"`
	Progress         int32                   `json:"progress"`
	ProgressMax      int32                   `json:"progressMax"`
	RemainingSeconds int32                   `json:"remainingSeconds"`
	TotalSeconds     int32                   `json:"totalSeconds,omitempty"`
	BossName         string                  `json:"bossName,omitempty"`
	Members          []EndgameRiftMemberView `json:"members,omitempty"`
	Claimable        bool                    `json:"claimable"`
}

// EndgameRiftEntranceView identifies a configured native NPC, not a renamed clone.
// NPCID is map_npcs.npc_id; the client submits the clicked live entity ID separately.
type EndgameRiftEntranceView struct {
	MapID  int32  `json:"mapId"`
	NPCID  int32  `json:"npcId"`
	NPC    string `json:"npc"`
	Script string `json:"script"`
}

type EndgameResponse struct {
	RiftEntrances  []EndgameRiftEntranceView `json:"riftEntrances,omitempty"`
	CombatStats    *EndgameCombatStatsV5     `json:"combatStats,omitempty"`
	SkillRanks     []SkillRankView           `json:"skillRanks,omitempty"`
	Kind           string                    `json:"kind"`
	RequestID      string                    `json:"requestId,omitempty"`
	OK             bool                      `json:"ok"`
	Error          string                    `json:"error,omitempty"`
	Message        string                    `json:"message,omitempty"`
	Revision       string                    `json:"revision,omitempty"`
	CatalogVersion string                    `json:"catalogVersion,omitempty"`
	Capabilities   []string                  `json:"capabilities,omitempty"`
	Cursor         string                    `json:"cursor,omitempty"`
	NextCursor     string                    `json:"nextCursor,omitempty"`
	SnapshotID     string                    `json:"snapshotId,omitempty"`
	Chunk          int32                     `json:"chunk,omitempty"`
	Chunks         int32                     `json:"chunks,omitempty"`
	TotalBytes     int32                     `json:"totalBytes,omitempty"`
	Data           string                    `json:"data,omitempty"`
	Items          []EndgameItemView         `json:"items,omitempty"`
	Runes          []EndgameRuneView         `json:"runes,omitempty"`
	Formations     []EndgameFormationView    `json:"formations,omitempty"`
	Recipes        []EndgameRecipeView       `json:"recipes,omitempty"`
	Materials      []EndgameMaterialView     `json:"materials,omitempty"`
	Quote          *EndgameQuoteView         `json:"quote,omitempty"`
	Rift           *EndgameRiftView          `json:"rift,omitempty"`
	Result         map[string]string         `json:"result,omitempty"`
}
