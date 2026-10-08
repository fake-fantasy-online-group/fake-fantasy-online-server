package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// AOI(Area of Interest) —— 服务端九宫格空间索引。
//
// 客户端实体已经改为同地图全量同步，这里不再决定谁能看见谁；它只用于怪物
// 索敌、范围技能、宠物找目标等距离相关查询，避免每次都扫描整张地图。
//
// 只在场景 goroutine 内访问, 无锁。换成四叉树/十字链表不影响外部 —— 这是可逆决策。

const defaultCellSize = 300 // 世界单位。与客户端视野量级匹配

type cell struct{ x, y int32 }

// AOI 管一张图上实体的空间分布。
type AOI struct {
	cellSize float64
	cells    map[cell]map[domain.EntityID]*entity.Entity
	at       map[domain.EntityID]cell // 反查: 实体现在在哪个格
}

func newAOI(cellSize float64) *AOI {
	if cellSize <= 0 {
		cellSize = defaultCellSize
	}
	return &AOI{
		cellSize: cellSize,
		cells:    map[cell]map[domain.EntityID]*entity.Entity{},
		at:       map[domain.EntityID]cell{},
	}
}

func (a *AOI) cellOf(p domain.Pos) cell {
	return cell{int32(p.X / a.cellSize), int32(p.Y / a.cellSize)}
}

// Enter 把实体放进它坐标所在的格。
func (a *AOI) Enter(e *entity.Entity) cell {
	c := a.cellOf(e.Pos)
	a.add(c, e)
	return c
}

// Leave 把实体从格里拿掉。
func (a *AOI) Leave(e *entity.Entity) {
	c, ok := a.at[e.ID]
	if !ok {
		return
	}
	a.remove(c, e.ID)
	delete(a.at, e.ID)
}

// Move 更新实体在服务端空间索引中的位置。
func (a *AOI) Move(e *entity.Entity, to domain.Pos) (crossed bool, from, dst cell) {
	from = a.at[e.ID]
	dst = a.cellOf(to)
	e.Pos = to
	if dst == from {
		return false, from, dst
	}
	a.remove(from, e.ID)
	a.add(dst, e)
	return true, from, dst
}

// Around 遍历以 c 为中心 3×3 格内的实体。用回调不返回切片, 是为了不在每帧
// 每次广播都分配一个 slice —— 这条路径每帧要走上千次。
func (a *AOI) Around(c cell, fn func(*entity.Entity)) {
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			for _, e := range a.cells[cell{c.x + dx, c.y + dy}] {
				fn(e)
			}
		}
	}
}

// AroundPos 遍历某坐标视野内的实体。
func (a *AOI) AroundPos(p domain.Pos, fn func(*entity.Entity)) { a.Around(a.cellOf(p), fn) }

// AroundRadius 遍历覆盖指定半径的全部格子。普通九宫格只保证约 300 像素范围；
// 多重投掷等客户端技能半径可到 500，继续用 AroundPos 会随施法者所在格位置
// 不同而随机漏掉第二圈格子。
func (a *AOI) AroundRadius(p domain.Pos, radius float64, fn func(*entity.Entity)) {
	if radius <= 0 {
		a.AroundPos(p, fn)
		return
	}
	min := a.cellOf(domain.Pos{X: p.X - radius, Y: p.Y - radius})
	max := a.cellOf(domain.Pos{X: p.X + radius, Y: p.Y + radius})
	for x := min.x; x <= max.x; x++ {
		for y := min.y; y <= max.y; y++ {
			for _, e := range a.cells[cell{x, y}] {
				fn(e)
			}
		}
	}
}

// CellOf 返回实体当前所在格。实体不在 AOI 里时第二个返回值为 false。
func (a *AOI) CellOf(id domain.EntityID) (cell, bool) {
	c, ok := a.at[id]
	return c, ok
}

func (a *AOI) add(c cell, e *entity.Entity) {
	m := a.cells[c]
	if m == nil {
		m = map[domain.EntityID]*entity.Entity{}
		a.cells[c] = m
	}
	m[e.ID] = e
	a.at[e.ID] = c
}

func (a *AOI) remove(c cell, id domain.EntityID) {
	m := a.cells[c]
	if m == nil {
		return
	}
	delete(m, id)
	if len(m) == 0 {
		delete(a.cells, c) // 空格子要删掉, 否则跑图跑久了 map 会一直涨
	}
}
