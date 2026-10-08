package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// NpcChatterLine 是客户端环境气泡的一条“NPC 名 → 文本”映射。
type NpcChatterLine struct {
	NPC  string
	Role domain.NPCRole
	Text string
}

// NpcChatter 是当前地图完整的 NPC 环境闲聊配置，只发给刚进图的玩家。
type NpcChatter struct {
	Who     domain.EntityID
	Enabled bool
	MinGap  int32
	MaxGap  int32
	ShowSec int32
	Lines   []NpcChatterLine
}

func (e NpcChatter) Subject() domain.EntityID { return e.Who }

// NpcDialogText 是玩家已打开 NPC 对话框后的非任务正文。
type NpcDialogText struct {
	Who  domain.EntityID
	NPC  string
	Text string
}

func (e NpcDialogText) Subject() domain.EntityID { return e.Who }
