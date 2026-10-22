package data

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type riftMonsterDefs interface {
	Def(domain.MonsterID) (domain.MonsterDef, bool)
}

// LoadRiftFloorTemplates builds the immutable catalog before scene actors start.
// Only real, non-city maps with native hostile spawns and a verified MASK qualify.
// Missing assets are excluded; a database/decoding failure is reported instead of
// silently treating an unknown map as an empty, walkable plane.
func LoadRiftFloorTemplates(ctx context.Context, q Querier, catalog *MapCatalog, spawns map[int32][]domain.SpawnPoint, defs riftMonsterDefs) ([]domain.RiftFloorTemplate, error) {
	if catalog == nil || q == nil || defs == nil {
		return nil, nil
	}
	rows, err := q.Query(ctx, `SELECT id FROM map_defs WHERE NOT is_city ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var ids []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var out []domain.RiftFloorTemplate
	for _, id := range ids {
		// Exclude special islands and user-rejected scenery before loading masks.
		// Generators apply the same policy to older/injected template catalogs.
		if domain.RiftMapExcluded(id) || len(spawns[id]) < domain.RiftFloorMinAnchors {
			continue
		}
		m, ok := catalog.ByID(id)
		if !ok {
			continue
		}
		combat := 0
		for _, p := range spawns[id] {
			if d, ok := defs.Def(p.Monster); ok && riftCombatTemplate(d) {
				combat++
			}
		}
		if combat < domain.RiftFloorMinAnchors {
			continue
		}
		mask, err := catalog.CollisionOfMap(ctx, q, id)
		if errors.Is(err, ErrCollisionMaskMissing) || errors.Is(err, ErrCollisionMaskNotConfigured) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("rift layout map %d: %w", id, err)
		}
		if t, ok := BuildRiftFloorTemplate(id, m.Name, spawns[id], defs, mask); ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func riftCombatTemplate(d domain.MonsterDef) bool {
	return d.ID > 0 && d.Sprite > 0 && d.Kind.Hostile() && !d.NoAttack && d.HP > 0
}

// BuildRiftFloorTemplate keeps one connected component, proven by 4-neighbor
// flood fill on the actual native collision cells. No diagonal corner cutting,
// guessed pathfinding stride, coordinate jitter, or unavailable sprite is used.
func BuildRiftFloorTemplate(mapID int32, name string, spawns []domain.SpawnPoint, defs riftMonsterDefs, mask *CollisionMask) (domain.RiftFloorTemplate, bool) {
	out := domain.RiftFloorTemplate{MapID: mapID, MapName: name}
	if mapID <= 0 || mask == nil || defs == nil || mask.width == 0 || mask.height == 0 {
		return out, false
	}
	points := append([]domain.SpawnPoint(nil), spawns...)
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	valid := points[:0]
	seenPos := map[domain.Pos]bool{}
	for _, p := range points {
		p.Pos.MapID = mapID
		d, ok := defs.Def(p.Monster)
		if !ok || !riftCombatTemplate(d) || !mask.Walkable(p.Pos) || seenPos[p.Pos] {
			continue
		}
		seenPos[p.Pos] = true
		valid = append(valid, p)
	}
	points = valid
	if len(points) < domain.RiftFloorMinAnchors {
		return out, false
	}
	labels := make([]int32, len(mask.cells))
	var label int32
	var queue []int
	groups := map[int32][]domain.SpawnPoint{}
	width := int(mask.width)
	for _, p := range points {
		cell := int(uint32(p.Pos.Y)/mask.cellHeight)*width + int(uint32(p.Pos.X)/mask.cellWidth)
		if labels[cell] == 0 {
			label++
			labels[cell] = label
			queue = append(queue[:0], cell)
			for head := 0; head < len(queue); head++ {
				i := queue[head]
				visit := func(j int) {
					if labels[j] == 0 && mask.cells[j]&maskBlockedBit == 0 {
						labels[j] = label
						queue = append(queue, j)
					}
				}
				if i%width > 0 {
					visit(i - 1)
				}
				if i%width < width-1 {
					visit(i + 1)
				}
				if i >= width {
					visit(i - width)
				}
				if i+width < len(labels) {
					visit(i + width)
				}
			}
		}
		groups[labels[cell]] = append(groups[labels[cell]], p)
	}
	// Component labels follow sorted native anchor IDs, making ties stable.
	var selectedLabel int32
	for i := int32(1); i <= label; i++ {
		if len(groups[i]) > len(out.Anchors) {
			out.Anchors = groups[i]
			selectedLabel = i
		}
	}
	if len(out.Anchors) < domain.RiftFloorMinAnchors {
		return out, false
	}
	// A 1024px diameter guarantees a >=512px exit for every possible entry.
	var diameter float64
	for i, p := range out.Anchors {
		for _, q := range out.Anchors[i+1:] {
			dx, dy := p.Pos.X-q.Pos.X, p.Pos.Y-q.Pos.Y
			if d := dx*dx + dy*dy; d > diameter {
				diameter = d
			}
		}
	}
	if diameter < 1024*1024 {
		return out, false
	}
	for _, component := range labels {
		if component == selectedLabel {
			out.WalkableArea += float64(mask.cellWidth) * float64(mask.cellHeight)
		}
	}
	// A globally aligned lattice gives distinct, spaced positions around native
	// hostile regions. Every point must be in this exact flood-filled component;
	// no unchecked jitter, blocked cell or disconnected island is eligible.
	out.Population = riftPopulationCandidates(mapID, out.Anchors, mask, labels, selectedLabel, domain.RiftFloorPopulationSpacing, domain.RiftFloorPopulationSpacing/2)
	// Only compact maps need a supplementary 32px lattice. It is aligned with
	// the original lattice, retaining at least 32px separation and the exact v2
	// population/seed. 64 spare positions conservatively cover both safe circles.
	if len(out.Population) < domain.RiftFloorMaxConfiguredPopulation+64 {
		seen := make(map[domain.Pos]bool, len(out.Population))
		for _, p := range out.Population {
			seen[p.Pos] = true
		}
		for _, p := range riftPopulationCandidates(mapID, out.Anchors, mask, labels, selectedLabel, domain.RiftFloorPopulationSpacing/2, domain.RiftFloorPopulationSpacing/2) {
			if !seen[p.Pos] {
				p.ID = -int32(len(out.Population) + len(out.ExtraPopulation) + 1)
				out.ExtraPopulation = append(out.ExtraPopulation, p)
			}
		}
	}
	monsters, bosses := map[domain.MonsterID]bool{}, map[domain.MonsterID]bool{}
	for _, p := range out.Anchors {
		d, _ := defs.Def(p.Monster)
		if d.Kind == domain.MonsterBoss {
			bosses[p.Monster] = true
		} else {
			monsters[p.Monster] = true
		}
	}
	if len(monsters) == 0 {
		return out, false
	}
	for id := range monsters {
		out.Monsters = append(out.Monsters, id)
	}
	for id := range bosses {
		out.Guardians = append(out.Guardians, id)
	}
	if len(out.Guardians) == 0 {
		out.Guardians = append(out.Guardians, out.Monsters...)
	}
	sort.Slice(out.Monsters, func(i, j int) bool { return out.Monsters[i] < out.Monsters[j] })
	sort.Slice(out.Guardians, func(i, j int) bool { return out.Guardians[i] < out.Guardians[j] })
	out.CombatRegions = buildRiftCombatRegions(mapID, mask, labels, selectedLabel)
	out.FullCombatRegions = buildRiftFullCombatRegions(mapID, mask, labels, selectedLabel)
	if !out.Valid() {
		return out, false
	}
	_, err := domain.PlanRiftFloor("catalog-validation", 1, []domain.RiftFloorTemplate{out}, 0)
	return out, err == nil
}

// riftPopulationCandidates returns only distinct points on the verified component.
func riftPopulationCandidates(mapID int32, anchors []domain.SpawnPoint, mask *CollisionMask, labels []int32, component int32, spacing, offset int) []domain.SpawnPoint {
	radius := domain.RiftFloorPopulationRadius
	width := int(mask.width)
	var population []domain.SpawnPoint
	seenCells := map[[2]int]bool{}
	for _, anchor := range anchors {
		for y := (int(anchor.Pos.Y) - radius) / spacing; y <= (int(anchor.Pos.Y)+radius)/spacing; y++ {
			for x := (int(anchor.Pos.X) - radius) / spacing; x <= (int(anchor.Pos.X)+radius)/spacing; x++ {
				key := [2]int{x, y}
				if seenCells[key] {
					continue
				}
				pos := domain.Pos{MapID: mapID, X: float64(x*spacing + offset), Y: float64(y*spacing + offset)}
				dx, dy := pos.X-anchor.Pos.X, pos.Y-anchor.Pos.Y
				if dx*dx+dy*dy > float64(radius*radius) || !mask.Walkable(pos) {
					continue
				}
				cell := int(uint32(pos.Y)/mask.cellHeight)*width + int(uint32(pos.X)/mask.cellWidth)
				if labels[cell] != component {
					continue
				}
				seenCells[key] = true
				population = append(population, domain.SpawnPoint{Pos: pos, Dir: anchor.Dir})
			}
		}
	}
	sort.Slice(population, func(i, j int) bool {
		if population[i].Pos.Y != population[j].Pos.Y {
			return population[i].Pos.Y < population[j].Pos.Y
		}
		return population[i].Pos.X < population[j].Pos.X
	})
	for i := range population {
		// Negative IDs distinguish catalog-only positions from native spawns.
		// Encounter entity IDs are allocated independently by the scene.
		population[i].ID = -int32(i + 1)
	}
	return population
}
