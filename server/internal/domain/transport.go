package domain

// TransportDestination 是 NPC 传送菜单中的一个服务端权威目的地。
// Index 与正式客户端 TransportUI 的零基 sel 一致；客户端回传的地图和坐标
// 一律不参与结算，选择后只按这份数据库配置落地。
type TransportDestination struct {
	Index int32
	Name  string
	Pos   Pos
	Price int32
	// Dungeon 表示选择后进入队伍专属副本实例，而不是常驻地图。
	// 入口仍复用客户端已有 TransportUI/0x1071/0x1072；这个标志只来自
	// PostgreSQL，客户端回传的索引不能把普通航线升级成副本入口。
	Dungeon bool
	// RequireClear 表示当前副本楼层全部可战斗怪物死亡后才开放。
	// 客户端只提交列表与索引，不能自行绕过这条服务端条件。
	RequireClear bool
}

// TransportList 是一个 NPC TransList 对应的完整传送菜单。
type TransportList struct {
	ID           int32
	SourceMapID  int32
	Destinations []TransportDestination
}

// TransportTable 是全服 NPC 传送菜单，按客户端 TransList 编号索引。
type TransportTable map[int32]TransportList
