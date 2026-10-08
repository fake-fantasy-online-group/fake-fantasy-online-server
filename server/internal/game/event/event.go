// Package event 定义**游戏语义事件** —— 游戏层对外说"发生了什么"的唯一词汇。
//
// 这个包存在的全部理由, 是让依赖方向变成:
//
//	game/combat ──emit──> event.DamageDealt ──read──> protocol/encode ──> 0x8011 字节
//
// 也就是 protocol 认识 event, **event 与整个 game 层都不认识 protocol**。
// 反过来做(游戏逻辑里直接拼字节)会导致三件事, 现有 world 包三样全中:
// 改协议要动业务、业务没法脱网单测、字节偏移写错报在业务代码里。
//
// 因此本包的铁律: **只 import domain, 不 import 任何别的内部包。**
// 事件字段用游戏词汇("造成了 42 点伤害"), 不用协议词汇("第 9 字节是 flag")。
package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Event 是一个已发生的事实。命名一律用过去式 —— 事件描述**已经发生**的事,
// 与命令(祈使式, 描述"请求做")对称, 看名字就知道方向。
type Event interface {
	// Subject 返回事件的主体实体。场景按它的位置决定广播给谁(AOI 过滤)。
	// 返回 0 表示无主体, 属于全场景事件。
	Subject() domain.EntityID
}

// Sink 是"把事件送给一个观察者"的出口, 由会话层实现(它知道怎么编码成字节)。
//
// 实现必须**立即返回**: 内部缓冲 + 背压, 满了就丢并计数。
// 场景 goroutine 绝不能在这里被卡住 —— 一个慢客户端不能拖住整张图。
type Sink interface {
	Emit(Event)
	Close()
}

// nopSubject 供无主体的全场景事件内嵌。
type nopSubject struct{}

func (nopSubject) Subject() domain.EntityID { return 0 }
