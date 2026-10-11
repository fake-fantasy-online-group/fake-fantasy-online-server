package data

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

type riftFloorTestDefs map[domain.MonsterID]domain.MonsterDef

func (d riftFloorTestDefs) Def(id domain.MonsterID) (domain.MonsterDef, bool) {
	v, ok := d[id]
	return v, ok
}
func riftGeometryFixture() (*CollisionMask, riftFloorTestDefs, []domain.SpawnPoint) {
	mask := &CollisionMask{width: 140, height: 100, cellWidth: 16, cellHeight: 16, cells: make([]byte, 140*100)}
	defs := riftFloorTestDefs{1: {ID: 1, Sprite: 20, HP: 100, Kind: domain.MonsterNormal}, 2: {ID: 2, Sprite: 21, HP: 200, Kind: domain.MonsterBoss}, 3: {ID: 3, Sprite: 22, HP: 10, Kind: domain.MonsterProp}, 4: {ID: 4, HP: 100, Kind: domain.MonsterNormal}, 5: {ID: 5, Sprite: 25, HP: 100, NoAttack: true}}
	var points []domain.SpawnPoint
	for i := 0; i < 50; i++ {
		points = append(points, domain.SpawnPoint{ID: int32(i + 1), Monster: 1, Pos: domain.Pos{MapID: 7, X: float64(8 + (i%5)*96), Y: float64(8 + (i/5)*144)}})
	}
	for i := 0; i < 20; i++ {
		points = append(points, domain.SpawnPoint{ID: int32(100 + i), Monster: 1, Pos: domain.Pos{MapID: 7, X: float64(1288 + (i%5)*96), Y: float64(8 + (i/5)*144)}})
	}
	return mask, defs, points
}
func TestRiftFloorTemplateConnectedNativeAnchors(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	for y := 0; y < int(mask.height); y++ {
		mask.cells[y*int(mask.width)+65] = maskBlockedBit
	}
	points = append(points, domain.SpawnPoint{ID: 200, Monster: 2, Pos: domain.Pos{MapID: 7, X: 500, Y: 600}}, domain.SpawnPoint{ID: 201, Monster: 3, Pos: domain.Pos{MapID: 7, X: 510, Y: 600}}, domain.SpawnPoint{ID: 202, Monster: 4, Pos: domain.Pos{MapID: 7, X: 520, Y: 600}}, domain.SpawnPoint{ID: 203, Monster: 5, Pos: domain.Pos{MapID: 7, X: 530, Y: 600}}, domain.SpawnPoint{ID: 204, Monster: 1, Pos: domain.Pos{MapID: 7, X: 1040, Y: 600}})
	layout, ok := BuildRiftFloorTemplate(7, "森林", points, defs, mask)
	if !ok || len(layout.Anchors) != 51 || len(layout.Monsters) != 1 || len(layout.Guardians) != 1 || layout.Guardians[0] != 2 {
		t.Fatalf("bad validated component: %+v", layout)
	}
	for _, p := range layout.Anchors {
		if p.Pos.X >= 1040 || !mask.Walkable(p.Pos) {
			t.Fatal("unreachable or blocked anchor")
		}
	}
	for floor := int32(1); floor <= 30; floor++ {
		p, err := domain.PlanRiftFloor("geometry", floor, []domain.RiftFloorTemplate{layout}, 0)
		if err != nil || !p.Valid() {
			t.Fatal("validated map failed generation", err)
		}
	}
}
func TestRiftFloorTemplateFollowsWindingCorridor(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	for y := 0; y < int(mask.height)-1; y++ {
		mask.cells[y*int(mask.width)+65] = maskBlockedBit
	}
	layout, ok := BuildRiftFloorTemplate(7, "洞穴", points, defs, mask)
	if !ok || len(layout.Anchors) != 70 {
		t.Fatal("lost reachable anchors around winding wall", len(layout.Anchors))
	}
}
func TestRiftFloorTemplateRejectsUnverifiedAssets(t *testing.T) {
	mask, defs, points := riftGeometryFixture()
	if _, ok := BuildRiftFloorTemplate(7, "", points, defs, nil); ok {
		t.Fatal("accepted absent mask")
	}
	if _, ok := BuildRiftFloorTemplate(7, "", points[:4], defs, mask); ok {
		t.Fatal("accepted sparse floor")
	}
	for i := range mask.cells {
		mask.cells[i] = maskBlockedBit
	}
	if _, ok := BuildRiftFloorTemplate(7, "", points, defs, mask); ok {
		t.Fatal("accepted blocked floor")
	}
	mask, defs, points = riftGeometryFixture()
	for i := range points {
		points[i].Monster = 4
	}
	if _, ok := BuildRiftFloorTemplate(7, "", points, defs, mask); ok {
		t.Fatal("accepted missing model")
	}
}
