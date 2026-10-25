package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Visibility is intentionally independent of AOI.cellSize and its legacy
// truncation semantics: combat/AI queries are not changed by local replication.
const riftVisibilityBlockSize = 300
const riftVisibilityHalfX = 7
const riftVisibilityHalfY = 5

type riftBlock struct{ x, y int64 }
type riftBlockWindow struct {
	min, max riftBlock
	global   bool
}

func (w riftBlockWindow) contains(b riftBlock) bool {
	return w.global || b.x >= w.min.x && b.x <= w.max.x && b.y >= w.min.y && b.y <= w.max.y
}
func visibilityBlock(p domain.Pos) riftBlock {
	// Native coordinates are int32. Clamp malformed internal input before an
	// integer conversion, also keeping loop arithmetic safe at either extreme.
	coordinate := func(v float64) int64 {
		if math.IsNaN(v) {
			return 0
		}
		v = math.Max(math.Min(v, math.MaxInt32), math.MinInt32)
		return int64(math.Floor(v / riftVisibilityBlockSize))
	}
	return riftBlock{coordinate(p.X), coordinate(p.Y)}
}

type riftIndexedEntity struct {
	block  riftBlock
	entity *entity.Entity
}

// riftBlockIndex is scene-owned. Membership changes touch entering/leaving
// blocks only; an unchanged player or monster block costs a map lookup.
type riftBlockIndex struct {
	width, height float64
	global        map[domain.EntityID]struct{}
	windows       map[domain.EntityID]riftBlockWindow
	subscribers   map[riftBlock]map[domain.EntityID]struct{}
	blocks        map[riftBlock]map[domain.EntityID]*entity.Entity
	entities      map[domain.EntityID]riftIndexedEntity
	changed       func(domain.EntityID, *entity.Entity, bool)
}

func newRiftBlockIndex(width, height float64, changed func(domain.EntityID, *entity.Entity, bool)) *riftBlockIndex {
	if width <= 0 || math.IsNaN(width) || math.IsInf(width, 0) {
		width = 0
	}
	if height <= 0 || math.IsNaN(height) || math.IsInf(height, 0) {
		height = 0
	}
	return &riftBlockIndex{width: width, height: height, changed: changed,
		global: make(map[domain.EntityID]struct{}), windows: make(map[domain.EntityID]riftBlockWindow), subscribers: make(map[riftBlock]map[domain.EntityID]struct{}),
		blocks: make(map[riftBlock]map[domain.EntityID]*entity.Entity), entities: make(map[domain.EntityID]riftIndexedEntity)}
}
func (i *riftBlockIndex) window(pos domain.Pos, extraBuffer float64) riftBlockWindow {
	// The native rectangular camera clamp uses map dimensions, not population
	// bounds. With a smaller map, the camera is fixed at the map midpoint.
	clamp := func(v, size, half float64) float64 {
		if size <= 0 {
			return v
		}
		if size <= 2*half {
			return size / 2
		}
		return math.Max(half, math.Min(size-half, v))
	}
	pos.X = clamp(pos.X, i.width, 1280)
	pos.Y = clamp(pos.Y, i.height, 720)
	c := visibilityBlock(pos)
	// Baseline is 15 x 11 blocks for a known clamped camera. Unknown native
	// GSM dimensions cannot be substituted with collision MASK dimensions.
	// A rectangular clamp may shift the camera by up to half a viewport, so
	// full viewport dimensions + 600 cover every possible camera position.
	// Wider client viewports are not reported and are not guaranteed here.
	extraBuffer = math.Max(0, math.Min(extraBuffer, math.MaxInt32))
	if math.IsNaN(extraBuffer) {
		extraBuffer = 0
	}
	hx, hy := int64(riftVisibilityHalfX), int64(riftVisibilityHalfY)
	radiusX, radiusY := 1280.0+600+extraBuffer, 720.0+600+extraBuffer
	if i.width <= 0 {
		radiusX += 1280
	}
	if i.height <= 0 {
		radiusY += 720
	}
	hx = max(hx, int64(math.Ceil(radiusX/riftVisibilityBlockSize)))
	hy = max(hy, int64(math.Ceil(radiusY/riftVisibilityBlockSize)))
	// A pathological but authoritative speed must not allocate millions of
	// subscription cells or silently omit visible monsters. Fall back to full
	// indexed visibility; transport staging still bounds bytes and delivery work.
	if (2*hx+1)*(2*hy+1) > 4096 {
		return riftBlockWindow{global: true}
	}
	return riftBlockWindow{min: riftBlock{c.x - hx, c.y - hy}, max: riftBlock{c.x + hx, c.y + hy}}
}
func eachRiftBlock(w riftBlockWindow, f func(riftBlock)) {
	if w.global {
		return
	}
	for x := w.min.x; x <= w.max.x; x++ {
		for y := w.min.y; y <= w.max.y; y++ {
			f(riftBlock{x, y})
		}
	}
}
func (i *riftBlockIndex) subscribe(id domain.EntityID, pos domain.Pos, extraBuffer ...float64) {
	margin := 0.0
	if len(extraBuffer) > 0 {
		margin = extraBuffer[0]
	}
	next := i.window(pos, margin)
	old, ok := i.windows[id]
	if ok && old == next {
		return
	}
	i.windows[id] = next
	if (ok && old.global) || next.global {
		// Only mode transitions scan the index. Ordinary full-mode movement has an
		// identical canonical window and returns above without scanning entities.
		if ok && !old.global {
			eachRiftBlock(old, func(b riftBlock) {
				delete(i.subscribers[b], id)
				if len(i.subscribers[b]) == 0 {
					delete(i.subscribers, b)
				}
			})
		}
		delete(i.global, id)
		if next.global {
			i.global[id] = struct{}{}
		} else {
			eachRiftBlock(next, func(b riftBlock) {
				if i.subscribers[b] == nil {
					i.subscribers[b] = make(map[domain.EntityID]struct{})
				}
				i.subscribers[b][id] = struct{}{}
			})
		}
		if i.changed != nil {
			for _, indexed := range i.entities {
				was := ok && old.contains(indexed.block)
				now := next.contains(indexed.block)
				if was != now {
					i.changed(id, indexed.entity, now)
				}
			}
		}
		return
	}
	if ok {
		eachRiftBlock(old, func(b riftBlock) {
			if !next.contains(b) {
				i.removeSubscriber(id, b)
			}
		})
	}
	eachRiftBlock(next, func(b riftBlock) {
		if ok && old.contains(b) {
			return
		}
		if i.subscribers[b] == nil {
			i.subscribers[b] = make(map[domain.EntityID]struct{})
		}
		i.subscribers[b][id] = struct{}{}
		if i.changed != nil {
			for _, e := range i.blocks[b] {
				i.changed(id, e, true)
			}
		}
	})
}
func (i *riftBlockIndex) removeSubscriber(id domain.EntityID, b riftBlock) {
	delete(i.subscribers[b], id)
	if len(i.subscribers[b]) == 0 {
		delete(i.subscribers, b)
	}
	if i.changed != nil {
		for _, e := range i.blocks[b] {
			i.changed(id, e, false)
		}
	}
}
func (i *riftBlockIndex) unsubscribe(id domain.EntityID) {
	old, ok := i.windows[id]
	if !ok {
		return
	}
	delete(i.windows, id)
	if old.global {
		delete(i.global, id)
		if i.changed != nil {
			for _, indexed := range i.entities {
				i.changed(id, indexed.entity, false)
			}
		}
		return
	}
	eachRiftBlock(old, func(b riftBlock) { i.removeSubscriber(id, b) })
}
func (i *riftBlockIndex) updateEntity(e *entity.Entity, present bool) {
	if e == nil {
		return
	}
	old, exists := i.entities[e.ID]
	if !present && exists && old.entity != e {
		return
	} // Stale lifetime removal.
	next := visibilityBlock(e.Pos)
	if i.changed != nil {
		if exists && (!present || old.entity != e) {
			for id := range i.global {
				i.changed(id, old.entity, false)
			}
		}
		if present && (!exists || old.entity != e) {
			for id := range i.global {
				i.changed(id, e, true)
			}
		}
	}
	if present && exists && old.entity == e && old.block == next {
		return
	}
	if exists {
		delete(i.blocks[old.block], e.ID)
		if len(i.blocks[old.block]) == 0 {
			delete(i.blocks, old.block)
		}
		delete(i.entities, e.ID)
		if i.changed != nil {
			for id := range i.subscribers[old.block] {
				_, still := i.subscribers[next][id]
				if !present || old.entity != e || !still {
					i.changed(id, old.entity, false)
				}
			}
		}
	}
	if !present {
		return
	}
	if i.blocks[next] == nil {
		i.blocks[next] = make(map[domain.EntityID]*entity.Entity)
	}
	i.blocks[next][e.ID] = e
	i.entities[e.ID] = riftIndexedEntity{next, e}
	if i.changed != nil {
		for id := range i.subscribers[next] {
			_, was := i.subscribers[old.block][id]
			if !exists || old.entity != e || !was {
				i.changed(id, e, true)
			}
		}
	}
}
func (i *riftBlockIndex) visible(id domain.EntityID, e *entity.Entity) bool {
	if e == nil {
		return false
	}
	w, ok := i.windows[id]
	return ok && w.contains(visibilityBlock(e.Pos))
}
func (i *riftBlockIndex) eachVisible(id domain.EntityID, fn func(*entity.Entity)) {
	w, ok := i.windows[id]
	if !ok {
		return
	}
	if w.global {
		for _, indexed := range i.entities {
			fn(indexed.entity)
		}
		return
	}
	eachRiftBlock(w, func(b riftBlock) {
		for _, e := range i.blocks[b] {
			fn(e)
		}
	})
}
