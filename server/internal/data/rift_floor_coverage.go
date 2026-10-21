package data

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Partition only the already-proven native component. A bounded BFS prefix is
// connected by construction, including winding corridors. Regions need not be
// rectangles and never join islands through diagonal corner cutting. Startup
// work is linear in MASK cells and candidate count; no per-floor flood fill.
func buildRiftCombatRegions(mapID int32, mask *CollisionMask, labels []int32, component int32) []domain.RiftCombatRegion {
	cellArea := float64(mask.cellWidth) * float64(mask.cellHeight)
	maxCells := int(float64(domain.RiftCoverageMaxArea) / cellArea)
	if maxCells < 1 {
		return nil
	}
	regions := make([]int32, len(labels))
	width := int(mask.width)
	regionCount := 0
	var queue []int
	for start, label := range labels {
		if label != component || regions[start] != 0 {
			continue
		}
		id := int32(regionCount + 1)
		queue = append(queue[:0], start)
		regions[start] = id
		for head := 0; head < len(queue); head++ {
			at := queue[head]
			visit := func(next int) {
				if len(queue) < maxCells && labels[next] == component && regions[next] == 0 {
					regions[next] = id
					queue = append(queue, next)
				}
			}
			if at%width > 0 {
				visit(at - 1)
			}
			if at%width < width-1 {
				visit(at + 1)
			}
			if at >= width {
				visit(at - width)
			}
			if at+width < len(labels) {
				visit(at + width)
			}
		}
		regionCount++
	}
	// One reusable queue bounds transient memory even on fragmented maps.
	queue = nil
	out := make([]domain.RiftCombatRegion, regionCount)
	spacing := domain.RiftFloorPopulationSpacing
	clearance := spacing / 2
	maxX, maxY := int(mask.width*mask.cellWidth), int(mask.height*mask.cellHeight)
	for y := spacing / 2; y < maxY; y += spacing {
		for x := spacing / 2; x < maxX; x += spacing {
			cell := int(uint32(y)/mask.cellHeight)*width + int(uint32(x)/mask.cellWidth)
			id := regions[cell]
			if id == 0 {
				continue
			}
			// The entire tile, not just its center, must be walkable. Narrow corridors
			// connect the region but do not inflate combat area or become monster packs.
			safe := true
			for cy := (y - clearance) / int(mask.cellHeight); cy <= (y+clearance-1)/int(mask.cellHeight) && safe; cy++ {
				for cx := (x - clearance) / int(mask.cellWidth); cx <= (x+clearance-1)/int(mask.cellWidth); cx++ {
					if cx < 0 || cy < 0 || cx >= width || cy >= int(mask.height) || regions[cy*width+cx] != id || mask.cells[cy*width+cx]&maskBlockedBit != 0 {
						safe = false
						break
					}
				}
			}
			if !safe {
				continue
			}
			r := &out[id-1]
			r.Population = append(r.Population, domain.SpawnPoint{ID: -int32(len(r.Population) + 1), Pos: domain.Pos{MapID: mapID, X: float64(x), Y: float64(y)}})
			r.CombatArea += float64(spacing * spacing)
		}
	}
	largest := 0.0
	for _, r := range out {
		if r.CombatArea > largest {
			largest = r.CombatArea
		}
	}
	valid := out[:0]
	for _, r := range out {
		// Tile accounting may straddle a partition boundary; keep its density area
		// inside the same hard bound even in unusual large-cell custom masks.
		if r.CombatArea > domain.RiftCoverageMaxArea {
			r.CombatArea = domain.RiftCoverageMaxArea
		}
		// Avoid tiny BFS boundary fragments on otherwise large maps. Small
		// native maps retain their own full region, without forcing extra mobs.
		if len(r.Population) >= 8 && r.CombatArea >= largest/2 {
			valid = append(valid, r)
		}
	}
	return valid
}
