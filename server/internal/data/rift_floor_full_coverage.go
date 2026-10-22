package data

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// buildRiftFullCombatRegions keeps the entire native component selected by the
// existing flood fill. Unlike the frozen v4/v5 catalog, neither a population
// budget nor artificial BFS boundaries can remove playable rooms or tiles.
func buildRiftFullCombatRegions(mapID int32, mask *CollisionMask, labels []int32, component int32) []domain.RiftCombatRegion {
	if mask == nil || mask.cellWidth == 0 || mask.cellHeight == 0 || component <= 0 || len(labels) != len(mask.cells) {
		return nil
	}
	var region domain.RiftCombatRegion
	width := int(mask.width)
	spacing := domain.RiftFloorPopulationSpacing
	clearance := spacing / 2
	maxX, maxY := int(mask.width)*int(mask.cellWidth), int(mask.height)*int(mask.cellHeight)
	for y := clearance; y < maxY; y += spacing {
		for x := clearance; x < maxX; x += spacing {
			// Check the whole 64px tile, not only its center. The actual native
			// component may connect through narrow corridors, but those corridors
			// never become fighting positions or inflate combat area.
			safe := true
			for cy := (y - clearance) / int(mask.cellHeight); cy <= (y+clearance-1)/int(mask.cellHeight) && safe; cy++ {
				for cx := (x - clearance) / int(mask.cellWidth); cx <= (x+clearance-1)/int(mask.cellWidth); cx++ {
					if cx < 0 || cy < 0 || cx >= width || cy >= int(mask.height) || labels[cy*width+cx] != component || mask.cells[cy*width+cx]&maskBlockedBit != 0 {
						safe = false
						break
					}
				}
			}
			if safe {
				region.Population = append(region.Population, domain.SpawnPoint{ID: -int32(len(region.Population) + 1), Pos: domain.Pos{MapID: mapID, X: float64(x), Y: float64(y)}})
			}
		}
	}
	if len(region.Population) < 8 {
		return nil
	}
	region.CombatArea = float64(len(region.Population)) * float64(spacing*spacing)
	return []domain.RiftCombatRegion{region}
}
