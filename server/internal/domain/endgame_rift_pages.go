package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// These are generic parser/transport memory budgets, not actor counts. Match
// the client's existing bounded JSON reader; each native frame stays <=60KiB.
const EndgameRiftSnapshotBytes = 1024 * 1024
const EndgameRiftPageBytes = 36 * 1024
const EndgameRiftMaxPages = (EndgameRiftSnapshotBytes + EndgameRiftPageBytes - 1) / EndgameRiftPageBytes

// PageEndgameRift preserves the complete response, including receipt identity.
// No partial business response is exposed: clients atomically apply the decoded
// original after receiving all pages. Call only after explicit capability opt-in.
func PageEndgameRift(v EndgameResponse, snapshotID string) ([]EndgameResponse, error) {
	if v.Rift == nil || v.Kind == "rift.page" || snapshotID == "" || len(snapshotID) > 64 {
		return nil, fmt.Errorf("invalid rift snapshot")
	}
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > EndgameRiftSnapshotBytes {
		return nil, fmt.Errorf("rift snapshot exceeds JSON resource budget: %d", len(raw))
	}
	if len(raw) <= EndgameMaxJSONBytes {
		return []EndgameResponse{v}, nil
	}
	n := (len(raw) + EndgameRiftPageBytes - 1) / EndgameRiftPageBytes
	pages := make([]EndgameResponse, 0, n)
	for start := 0; start < len(raw); start += EndgameRiftPageBytes {
		end := min(start+EndgameRiftPageBytes, len(raw))
		page := EndgameResponse{Kind: "rift.page", OK: true, SnapshotID: snapshotID,
			Chunk: int32(len(pages)), Chunks: int32(n), TotalBytes: int32(len(raw)),
			Data: base64.StdEncoding.EncodeToString(raw[start:end])}
		encoded, err := json.Marshal(page)
		if err != nil || len(encoded) > EndgameMaxJSONBytes {
			return nil, fmt.Errorf("rift page exceeds native frame budget")
		}
		pages = append(pages, page)
	}
	return pages, nil
}
