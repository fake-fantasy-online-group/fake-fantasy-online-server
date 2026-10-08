package domain

import (
	"sync/atomic"
	"time"
)

// itemInstanceSeq 使用“毫秒时间 + 毫秒内序号”生成正 int64。该项目只运行
// 一个本地正式服进程；20 位序号允许同一毫秒创建 1,048,576 个实例，并让
// 重启后的时间高位自然前进。数据库仍以 UID 主键做最后一道冲突保护。
var itemInstanceSeq atomic.Uint64

// NewItemInstanceID 分配服务端物品实例号。客户端不认识也不需要携带它。
func NewItemInstanceID() int64 {
	for {
		now := uint64(time.Now().UnixMilli())
		old := itemInstanceSeq.Load()
		oldMillis := old >> 20
		var next uint64
		if now > oldMillis {
			next = now << 20
		} else {
			next = old + 1
		}
		// 保持在有符号正数范围内；当前 Unix 毫秒左移 20 远低于上限。
		if next == 0 || next > uint64(^uint64(0)>>1) {
			continue
		}
		if itemInstanceSeq.CompareAndSwap(old, next) {
			return int64(next)
		}
	}
}
