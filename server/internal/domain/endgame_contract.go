package domain

// EndgameV1 is explicitly negotiated; native positional packets remain unchanged.
const EndgameV1 = "endgame.v1"
const EndgameSchema = 1
const EndgameMaxJSONBytes = 60 * 1024

// EndgameItemState is server-owned metadata on the existing durable equipment UID.
// UniqueID is the base ItemID in a separate namespace. Zero means ordinary gear.
type EndgameItemState struct {
 Revision int64
 UniqueID int32
 UniqueRollBP int32
}

type EndgameRequest struct {
 Op string `json:"op"`
 RequestID string `json:"requestId,omitempty"`
 ExpectedRevision string `json:"expectedRevision,omitempty"`
 Capabilities []string `json:"capabilities,omitempty"`
 Catalog string `json:"catalog,omitempty"`
 Cursor string `json:"cursor,omitempty"`
 TargetUID string `json:"targetUid,omitempty"`
 RuneUID string `json:"runeUid,omitempty"`
 Socket int32 `json:"socket,omitempty"`
 RecipeID int32 `json:"recipeId,omitempty"`
 QuoteID string `json:"quoteId,omitempty"`
 RunID string `json:"runId,omitempty"`
 Tier int32 `json:"tier,omitempty"`
 ItemUIDs []string `json:"itemUids,omitempty"`
}
func (r EndgameRequest) Mutates() bool {
 switch r.Op { case "socket.insert", "socket.remove", "cube.execute", "rift.start", "rift.claim", "rift.leave": return true }; return false
}

type EndgameStatView struct { Attr int32 `json:"attr"`; Value int32 `json:"value"`; Mode int32 `json:"mode"` }
type EndgameSocketView struct { Name string `json:"name,omitempty"`; Index int32 `json:"index"`; ItemID int32 `json:"itemId"`; UID string `json:"uid,omitempty"` }
type EndgameItemView struct {
 Tab int32 `json:"tab"`; Level int32 `json:"level,omitempty"`; Profession int32 `json:"profession,omitempty"`; UID string `json:"uid"`; ItemID int32 `json:"itemId"`; Location string `json:"location"`; Slot int32 `json:"slot"`
 Revision string `json:"revision"`; UniqueID int32 `json:"uniqueId,omitempty"`; UniqueRollBP int32 `json:"uniqueRollBp,omitempty"`
 Name string `json:"name,omitempty"`; Description string `json:"description,omitempty"`; Trait string `json:"trait,omitempty"`
 SocketCount int32 `json:"socketCount"`; Sockets []EndgameSocketView `json:"sockets,omitempty"`
 Stats []EndgameStatView `json:"stats,omitempty"`; FormationID int32 `json:"formationId,omitempty"`; Active bool `json:"active,omitempty"`; InactiveReason string `json:"inactiveReason,omitempty"`
 Bound bool `json:"bound"`; Locked bool `json:"locked"`
}
type EndgameRuneView struct { Mansion string `json:"mansion,omitempty"`; ItemID int32 `json:"itemId"`; Name string `json:"name"`; Description string `json:"description,omitempty"`; Quadrant string `json:"quadrant,omitempty"`; Stats []EndgameStatView `json:"stats,omitempty"` }
type EndgameFormationView struct {
 ID int32 `json:"id"`; Name string `json:"name"`; Description string `json:"description,omitempty"`; Channel string `json:"channel"`; Runes []int32 `json:"runes"`
 Stats []EndgameStatView `json:"stats,omitempty"`; MinLevel int32 `json:"minLevel,omitempty"`; Professions int32 `json:"professions,omitempty"`; Slots []int32 `json:"slots,omitempty"`; Active bool `json:"active,omitempty"`; InactiveReason string `json:"inactiveReason,omitempty"`
}
type EndgameMaterialView struct { Owned int32 `json:"owned,omitempty"`; ItemID int32 `json:"itemId"`; Count int32 `json:"count"`; Name string `json:"name,omitempty"` }
type EndgameRecipeView struct { ID int32 `json:"id"`; Name string `json:"name"`; Description string `json:"description,omitempty"`; Operation string `json:"operation"`; Money string `json:"money"`; Materials []EndgameMaterialView `json:"materials,omitempty"`; OutputItemID int32 `json:"outputItemId,omitempty"`; TargetRequired bool `json:"targetRequired,omitempty"` }
type EndgameQuoteView struct { QuoteID string `json:"quoteId"`; RecipeID int32 `json:"recipeId"`; TargetUID string `json:"targetUid,omitempty"`; Revision string `json:"revision"`; Money string `json:"money"`; Materials []EndgameMaterialView `json:"materials,omitempty"`; ExpiresAt string `json:"expiresAt,omitempty"`; Description string `json:"description,omitempty"` }
type EndgameRiftMemberView struct { Name string `json:"name,omitempty"`; CharID string `json:"charId"`; Level int32 `json:"level"`; Eligible bool `json:"eligible"`; Claimed bool `json:"claimed"` }
type EndgameRiftAuraView struct { EntityID string `json:"entityId"`; Class int32 `json:"class"` }
type EndgameRiftGlobeView struct { EntityID string `json:"entityId"`; X int32 `json:"x"`; Y int32 `json:"y"`; Value int32 `json:"value"` }
type EndgameRiftView struct {
 Name string `json:"name,omitempty"`; KeyCount int32 `json:"keyCount"`; KeyItemID int32 `json:"keyItemId"`; Auras []EndgameRiftAuraView `json:"auras,omitempty"`; Globes []EndgameRiftGlobeView `json:"globes,omitempty"`;
 RunID string `json:"runId"`; Revision string `json:"revision"`; Phase string `json:"phase"`; Tier int32 `json:"tier"`; Progress int32 `json:"progress"`; ProgressMax int32 `json:"progressMax"`; RemainingSeconds int32 `json:"remainingSeconds"`; BossName string `json:"bossName,omitempty"`; Members []EndgameRiftMemberView `json:"members,omitempty"`; Claimable bool `json:"claimable"`
}
type EndgameResponse struct {
 Kind string `json:"kind"`; RequestID string `json:"requestId,omitempty"`; OK bool `json:"ok"`; Error string `json:"error,omitempty"`; Message string `json:"message,omitempty"`
 Revision string `json:"revision,omitempty"`; CatalogVersion string `json:"catalogVersion,omitempty"`; Capabilities []string `json:"capabilities,omitempty"`
 Cursor string `json:"cursor,omitempty"`; NextCursor string `json:"nextCursor,omitempty"`; SnapshotID string `json:"snapshotId,omitempty"`; Chunk int32 `json:"chunk,omitempty"`; Chunks int32 `json:"chunks,omitempty"`
 Items []EndgameItemView `json:"items,omitempty"`; Runes []EndgameRuneView `json:"runes,omitempty"`; Formations []EndgameFormationView `json:"formations,omitempty"`; Recipes []EndgameRecipeView `json:"recipes,omitempty"`; Materials []EndgameMaterialView `json:"materials,omitempty"`
 Quote *EndgameQuoteView `json:"quote,omitempty"`; Rift *EndgameRiftView `json:"rift,omitempty"`; Result map[string]string `json:"result,omitempty"`
}
