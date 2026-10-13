package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Validate the actual client wire contract, including the encoded snapshot size.
// These are transport bounds, not balance defaults. The conservative envelope
// bounds legacy/v2/v3 floors by 24 blue, 8 gold and 320 ordinary/elite actors,
// plus one guardian. Counts deliberately over-approximate simultaneous state.
func (r *RiftRules) validateWire() error {
	fits := func(s string, max int) bool { return utf8.ValidString(s) && len(utf16.Encode([]rune(s))) <= max }
	for _, s := range []string{r.Name, r.BossName, r.ExitName, r.ExitGreeting} {
		if !fits(s, 8192) {
			return fmt.Errorf("rift text exceeds client limit")
		}
	}
	for _, s := range []string{r.ExitSprite, r.ExitPortrait} {
		if !fits(s, 256) {
			return fmt.Errorf("rift resource name exceeds client limit")
		}
	}
	var affixes []string
	longestAbility := ""
	for _, a := range r.Abilities {
		if !fits(a.Name, 128) || !fits(a.Affix, 64) || !fits(a.Warning, 8192) {
			return fmt.Errorf("rift ability text exceeds client limit")
		}
		affixes = append(affixes, a.Affix)
		raw, _ := json.Marshal(a.Name)
		old, _ := json.Marshal(longestAbility)
		if len(raw) > len(old) {
			longestAbility = a.Name
		}
	}
	id := strings.Repeat("9", 20)
	v := &EndgameRiftView{Name: r.Name, BossName: r.BossName, RunID: strings.Repeat("f", 32), Revision: id, Seed: id, GeneratorVersion: math.MaxInt32, FloorIndex: math.MaxInt32, MapName: strings.Repeat("<", 256), ExitX: math.MinInt32, ExitY: math.MinInt32, MaxTier: r.MaxTier, UnlockedTier: r.MaxTier, KeyCount: math.MaxInt32, KeyItemID: math.MaxInt32, Phase: "reward_pending", Tier: r.MaxTier, Progress: r.ProgressMax, ProgressMax: r.ProgressMax, RemainingSeconds: math.MaxInt32, TotalSeconds: math.MaxInt32, CanAdvance: true, Transitioning: true}
	for i := 0; i < 5; i++ {
		v.Members = append(v.Members, EndgameRiftMemberView{CharID: id, Name: strings.Repeat("<", maxNameBytes), Level: math.MaxInt32})
	}
	for i := 0; i < RiftFloorMaxConfiguredPopulation+1; i++ {
		aura := EndgameRiftAuraView{EntityID: id, Class: 3}
		if i < 33 {
			aura.Affixes = affixes
		}
		v.Auras = append(v.Auras, aura)
	}
	maxGlobes := int(24*r.BlueOrbs + 8*r.GoldOrbs)
	if maxGlobes > 1024 {
		return fmt.Errorf("rift globe count exceeds client limit")
	}
	for i := 0; i < maxGlobes; i++ {
		v.Globes = append(v.Globes, EndgameRiftGlobeView{EntityID: id, X: math.MinInt32, Y: math.MinInt32, Value: r.GlobeProgress})
	}
	for i := int32(0); i < r.AbilityMaxConcurrent; i++ {
		v.Telegraphs = append(v.Telegraphs, EndgameRiftTelegraphView{ID: id + ":" + id, EntityID: id, Ability: longestAbility, Shape: "circle", X: math.MinInt32, Y: math.MinInt32, EndX: math.MinInt32, EndY: math.MinInt32, Radius: 10000, InnerRadius: 10000, RemainingMS: 60000})
	}
	raw, err := json.Marshal(EndgameResponse{Kind: "rift.advance", RequestID: strings.Repeat("x", 64), OK: true, Message: "进入下一层，进度与计时保留", Rift: v})
	if err != nil || len(raw) > EndgameMaxJSONBytes {
		return fmt.Errorf("rift snapshot exceeds client byte budget: %d", len(raw))
	}
	return nil
}
