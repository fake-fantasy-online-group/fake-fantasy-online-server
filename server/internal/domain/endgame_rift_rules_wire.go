package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Validate the actual client wire contract, including the encoded snapshot size.
// These are transport bounds, not balance defaults. The conservative envelope
// bounds legacy/v2/v3 floors by 24 blue, 8 gold and 320 ordinary/elite actors,
// plus one guardian. Counts deliberately over-approximate simultaneous state.
func (r *RiftRules) validateWire() error {
	return r.validateWirePopulation(RiftFloorMaxConfiguredPopulation)
}

// ValidateFloorWire checks the chosen complete floor before any key charge or
// transition. The same conservative elite/effect envelope is retained; only
// the ordinary population varies with actual legal map area.
func (r *RiftRules) ValidateFloorWire(plan RiftFloorPlan) error {
	if plan.GeneratorVersion != RiftFloorFullCoverageVersion {
		return nil
	}
	return r.validateWirePopulation(len(plan.Spawns))
}

func (r *RiftRules) validateWirePopulation(population int) error {
	population = max(population, 33) // conservative elite/guardian envelope, not spawn count
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
	// A monster carries at most three selected affixes, never the entire catalog.
	if r.Elite != nil {
		sort.Slice(affixes, func(i, j int) bool {
			a, _ := json.Marshal(affixes[i])
			b, _ := json.Marshal(affixes[j])
			return len(a) > len(b)
		})
		if len(affixes) > 3 {
			affixes = affixes[:3]
		}
	}
	id := strings.Repeat("9", 20)
	v := &EndgameRiftView{Name: r.Name, BossName: r.BossName, RunID: strings.Repeat("f", 32), Revision: id, Seed: id, GeneratorVersion: math.MaxInt32, FloorIndex: math.MaxInt32, MapName: strings.Repeat("<", 256), ExitX: math.MinInt32, ExitY: math.MinInt32, MaxTier: r.MaxTier, UnlockedTier: r.MaxTier, KeyCount: math.MaxInt32, KeyItemID: math.MaxInt32, Phase: "reward_pending", Tier: r.MaxTier, Progress: r.ProgressMax, ProgressMax: r.ProgressMax, RemainingSeconds: math.MaxInt32, TotalSeconds: math.MaxInt32, CanAdvance: true, Transitioning: true}
	v.PendingLoot = 69 // Maximum configured personal reward slots, conservatively bounded.
	v.GuardianArrival = &EndgameRiftGuardianArrivalView{ID: strings.Repeat("f", 64) + "/2147483647/guardian", Seed: id, AnchorCharID: id, X: math.MinInt32, Y: math.MinInt32, StartedAtMS: math.MaxInt64 - 5000, DeadlineMS: math.MaxInt64, DurationMS: 5000, RemainingMS: 5000}
	for i := 0; i < 5; i++ {
		v.Members = append(v.Members, EndgameRiftMemberView{CharID: id, Name: strings.Repeat("<", maxNameBytes), Level: math.MaxInt32})
	}
	for i := 0; i < population+1; i++ {
		aura := EndgameRiftAuraView{EntityID: "4294967295", Class: 3}
		if i < 33 {
			aura.Affixes = affixes
			aura.OriginalName = strings.Repeat("<", 128)
		}
		v.Auras = append(v.Auras, aura)
	}
	maxGlobes := int(24*r.BlueOrbs + 8*r.GoldOrbs)
	if maxGlobes > 1024 {
		return fmt.Errorf("rift globe count exceeds client limit")
	}
	for i := 0; i < maxGlobes; i++ {
		v.Globes = append(v.Globes, EndgameRiftGlobeView{EntityID: "4294967295", X: math.MinInt32, Y: math.MinInt32, Value: r.GlobeProgress})
	}
	for i := int32(0); r.Elite == nil && i < r.AbilityMaxConcurrent; i++ {
		v.Telegraphs = append(v.Telegraphs, EndgameRiftTelegraphView{ID: id + ":" + id, EntityID: id, Ability: longestAbility, Shape: "circle", X: math.MinInt32, Y: math.MinInt32, EndX: math.MinInt32, EndY: math.MinInt32, Radius: 10000, InnerRadius: 10000, RemainingMS: 60000})
	}
	if r.Elite != nil {
		v.MapID = math.MaxInt32
		v.ServerNowMS = math.MaxInt64
		for i := 0; i < 33; i++ {
			v.Auras[i].AbilityIDs = []int32{0, 1, 2}
			v.Auras[i].GoldenUntilMS = math.MaxInt64
		}
		// Every elite may own three clones; one guardian cannot coexist with
		// the cleared trash wave. IDs are native uint32 rather than 64-bit.
		for i := 0; i < 96; i++ {
			v.Auras = append(v.Auras, EndgameRiftAuraView{EntityID: "4294967295", Class: 0, CloneOf: "4294967295"})
		}
		// Worst live envelope: active budget may all be walls. Death lava replaces
		// an elite aura; its bounded payload is smaller than the removed maximum
		// original-name/three-affix aura, so 32 simultaneous deaths cannot grow it.
		for i := int32(0); i < r.Elite.HazardMaxConcurrent; i++ {
			h := EndgameRiftHazardView{ID: strings.Repeat("f", 32) + "/2147483647/e" + id, SourceID: "4294967295", AbilityID: 7, Mechanism: "lightning_orb", X: math.MinInt32, Y: math.MinInt32, Radius: 10000, Width: 200, BornMS: math.MaxInt64, ExpiresMS: math.MaxInt64, RemainingMS: 60000, FirstPulseMS: 60000, PulseMS: 60000, PropagationMS: 60000}
			for j := 0; j < 3; j++ {
				h.Walls = append(h.Walls, EndgameRiftWallView{ID: h.ID + "/w2", X: -9999999.999999998, Y: -9999999.999999998, X2: -9999999.999999998, Y2: -9999999.999999998, Width: 200})
			}
			v.Hazards = append(v.Hazards, h)
			v.Impacts = append(v.Impacts, EndgameRiftImpactView{ID: h.ID, Mechanism: "death_lava", X: math.MinInt32, Y: math.MinInt32, Radius: 10000, AtMS: math.MaxInt64, ExpiresMS: math.MaxInt64})
		}
	}
	raw, err := json.Marshal(EndgameResponse{Kind: "rift.advance", RequestID: strings.Repeat("x", 64), OK: true, Message: "进入下一层，进度与计时保留", Rift: v})
	budget := EndgameMaxJSONBytes
	if r.FloorLayoutVersion == RiftFloorFullCoverageVersion {
		budget = EndgameRiftSnapshotBytes
	}
	if err != nil || len(raw) > budget {
		return fmt.Errorf("rift snapshot exceeds client byte budget: %d", len(raw))
	}
	if r.FloorLayoutVersion == RiftFloorFullCoverageVersion {
		var value any
		if json.Unmarshal(raw, &value) != nil || riftJSONNodes(value) > 65536 {
			return fmt.Errorf("rift snapshot exceeds bounded JSON node budget")
		}
	}
	return nil
}

func riftJSONNodes(value any) int {
	n := 1
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			n += riftJSONNodes(child)
		}
	case []any:
		for _, child := range v {
			n += riftJSONNodes(child)
		}
	}
	return n
}
