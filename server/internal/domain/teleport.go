package domain

// 传送门。来自客户端 `<map>.link`(目标与落点) + `.prc`(触发区域多边形)。
//
// 全服 995 个: 851 个带触发多边形(4~12 个顶点), 631 个的目标地图在 map_defs 里有定义。

// Teleport 是一个传送门。
type Teleport struct {
	// ProcID 是图内编号, 对应 .prc 里的区域号。
	ProcID int32

	// Area 是踩上去会触发的区域。空表示没有区域数据 ——
	// 那 144 个只能靠别的方式触发(点 NPC、用道具), 位置判定用不上。
	Area Polygon

	// To 是目标地图 id 与落点。
	To Pos
	// Dungeon 表示这扇门进入队伍专属副本实例。当前唯一闭合来源是
	// 通天塔 qw0507.link 的 map_script=Trap20502。
	Dungeon bool
	// ToDir 落地朝向。单位与刷怪点的 init_dir 相同, 同样没搞清, 原样透传。
	ToDir int32

	// Back 是从目标地图走回来时的落点。
	//
	// 存在的理由: 传送门是**单向**记录的, 出去和回来是两条不同的门。
	// 这一组是给目标图那扇门用的落点, 现在还没接 —— 记着免得将来重新去客户端翻。
	Back    Pos
	BackDir int32
}

// Polygon 是一圈顶点围成的区域(客户端世界坐标)。
//
// 顶点数实测 4~12: 510 个是四边形(菱形), 其余是更复杂的形状。
type Polygon []Pos

// Contains 判断点是否落在多边形内(射线法)。
//
// 边界上的点算不算**不做保证** —— 传送门有几十个单位宽, 差一个像素的判定
// 在游戏里没有任何可观测的区别, 为此写一套精确的边界处理不值得。
func (p Polygon) Contains(pt Pos) bool {
	if len(p) < 3 {
		return false
	}
	in := false
	j := len(p) - 1
	for i := 0; i < len(p); i++ {
		// 射线向 +X 打出去, 数穿过了几条边
		if (p[i].Y > pt.Y) != (p[j].Y > pt.Y) {
			x := (p[j].X-p[i].X)*(pt.Y-p[i].Y)/(p[j].Y-p[i].Y) + p[i].X
			if pt.X < x {
				in = !in
			}
		}
		j = i
	}
	return in
}

// Bounds 返回外接矩形。用来在射线法之前做一次便宜的排除 ——
// 一张图十几个传送门, 每次移动都跑射线法是白烧 CPU。
func (p Polygon) Bounds() (minX, minY, maxX, maxY float64) {
	if len(p) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY = p[0].X, p[0].Y
	maxX, maxY = minX, minY
	for _, v := range p[1:] {
		if v.X < minX {
			minX = v.X
		}
		if v.X > maxX {
			maxX = v.X
		}
		if v.Y < minY {
			minY = v.Y
		}
		if v.Y > maxY {
			maxY = v.Y
		}
	}
	return
}

// Portal 是一个传送门加上它的外接矩形, 供场景做快速命中判断。
type Portal struct {
	Teleport
	minX, minY, maxX, maxY float64
}

// NewPortal 预算好外接矩形。
func NewPortal(t Teleport) Portal {
	p := Portal{Teleport: t}
	p.minX, p.minY, p.maxX, p.maxY = t.Area.Bounds()
	return p
}

// Hit 报告某个坐标是否踩中了这扇门。
func (p *Portal) Hit(pt Pos) bool {
	if len(p.Area) < 3 {
		return false // 没有区域数据的门不能靠踩触发
	}
	// 先过外接矩形: 绝大多数移动离任何一扇门都很远, 这一步就筛掉了
	if pt.X < p.minX || pt.X > p.maxX || pt.Y < p.minY || pt.Y > p.maxY {
		return false
	}
	return p.Area.Contains(pt)
}
