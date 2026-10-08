package scene

import (
	"container/heap"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 定时器 —— 按**逻辑帧**触发, 不是墙钟。
//
// 冷却、状态到期、怪物重生、副本限时、掉落物消失, 全部走这里。
// 用帧号而不是 time.Timer 有三个好处: 可复现(重放同样的帧数得到同样的结果)、
// 可测(不用 sleep)、不占 goroutine(几万个重生计时器只是堆里几万个元素)。

type timer struct {
	at domain.Tick
	fn func()
}

type timerHeap []timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].at < h[j].at }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)        { *h = append(*h, x.(timer)) }
func (h *timerHeap) Pop() any          { old := *h; n := len(old); v := old[n-1]; *h = old[:n-1]; return v }

// timers 是一个按触发帧排序的最小堆。只在场景 goroutine 内访问。
type timers struct{ h timerHeap }

// after 安排一个回调在 d 帧之后触发。d <= 0 视为下一帧。
func (t *timers) after(now, d domain.Tick, fn func()) {
	if d == 0 {
		d = 1
	}
	heap.Push(&t.h, timer{at: now + d, fn: fn})
}

// advance 触发所有到期的定时器。
//
// 回调里再排新定时器是安全的: 新的 at 必然 > now(after 至少 +1 帧),
// 所以不会在本帧被再次弹出, 不会死循环。
func (t *timers) advance(now domain.Tick) {
	for len(t.h) > 0 && t.h[0].at <= now {
		e := heap.Pop(&t.h).(timer)
		e.fn()
	}
}

func (t *timers) pending() int { return len(t.h) }
