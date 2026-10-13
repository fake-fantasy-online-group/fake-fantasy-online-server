package scene

import (
	"encoding/json"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Configuration text is not a fixed-size record. Keep catalog cursors as row
// offsets while shrinking a page to the actual encoded transport budget.
func fitEndgameCatalogPage(r *domain.EndgameResponse, start, total int) bool {
	for {
		count := len(r.Items) + len(r.Runes) + len(r.Formations) + len(r.Recipes)
		r.NextCursor = ""
		if start+count < total {
			r.NextCursor = strconv.Itoa(start + count)
		}
		// endgameReply inserts an int64 revision when it is omitted. Reserve the
		// maximum positive width without depending on protocol-layer encoding.
		probe := *r
		if probe.Revision == "" {
			probe.Revision = "9223372036854775807"
		}
		encoded, err := json.Marshal(probe)
		if err == nil && len(encoded) <= domain.EndgameMaxJSONBytes {
			return count > 0 || start == total
		}
		if count <= 1 {
			return false
		}
		switch {
		case len(r.Items) > 0:
			r.Items = r.Items[:len(r.Items)-1]
		case len(r.Runes) > 0:
			r.Runes = r.Runes[:len(r.Runes)-1]
		case len(r.Formations) > 0:
			r.Formations = r.Formations[:len(r.Formations)-1]
		case len(r.Recipes) > 0:
			r.Recipes = r.Recipes[:len(r.Recipes)-1]
		}
	}
}

// Prepare every chunk before publishing any of them. Header data belongs to
// chunk zero; otherwise one long but valid description is repeated in every
// packet. The client publishes only after receiving the complete bounded set.
func splitEndgameSnapshot(base domain.EndgameResponse, items []domain.EndgameItemView) ([]domain.EndgameResponse, bool) {
	var out []domain.EndgameResponse
	start := 0
	for start < len(items) || len(out) == 0 {
		if len(out) >= 128 {
			return nil, false
		}
		r := base
		r.Chunk, r.Chunks = int32(len(out)), 128 // pessimistic envelope width
		if len(out) > 0 {
			r.Materials, r.Formations, r.CombatStats = nil, nil, nil
		}
		end := start + 32
		if end > len(items) {
			end = len(items)
		}
		for {
			r.Items = items[start:end]
			probe := r
			if probe.Revision == "" {
				probe.Revision = "9223372036854775807"
			}
			b, err := json.Marshal(probe)
			if err == nil && len(b) <= domain.EndgameMaxJSONBytes {
				break
			}
			if end == start {
				return nil, false
			}
			end--
		}
		if end == start && len(out) > 0 {
			return nil, false
		}
		out = append(out, r)
		start = end
	}
	for i := range out {
		out[i].Chunks = int32(len(out))
	}
	return out, true
}
