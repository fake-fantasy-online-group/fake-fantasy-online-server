package domain

import "testing"

// 龙城通往南郊那扇门的真实多边形(map_teleports.poly):
//
//	{5442,6906, 5577,6986, 5442,7066, 5307,6986}
//
// 是个菱形: 中心 (5442,6986), 半宽 135, 半高 80。
var 南郊门区域 = Polygon{
	{X: 5442, Y: 6906}, {X: 5577, Y: 6986}, {X: 5442, Y: 7066}, {X: 5307, Y: 6986},
}

func TestPolygonContains(t *testing.T) {
	cases := []struct {
		name string
		pt   Pos
		want bool
	}{
		{"正中心", Pos{X: 5442, Y: 6986}, true},
		{"中心偏左", Pos{X: 5400, Y: 6986}, true},
		{"中心偏上", Pos{X: 5442, Y: 6950}, true},
		{"外接矩形内但在菱形四角之外", Pos{X: 5320, Y: 6920}, false},
		{"完全在外面(北)", Pos{X: 5442, Y: 6800}, false},
		{"完全在外面(东)", Pos{X: 6000, Y: 6986}, false},
		{"隔着半张图", Pos{X: 100, Y: 100}, false},
	}
	for _, c := range cases {
		if got := 南郊门区域.Contains(c.pt); got != c.want {
			t.Errorf("%s (%.0f,%.0f): 期望 %v 实际 %v", c.name, c.pt.X, c.pt.Y, c.want, got)
		}
	}
}

// 顶点不足三个围不成区域, 不能误判成"处处命中"。
func TestDegeneratePolygonContainsNothing(t *testing.T) {
	for _, p := range []Polygon{nil, {}, {{X: 1, Y: 1}}, {{X: 1, Y: 1}, {X: 2, Y: 2}}} {
		if p.Contains(Pos{X: 1, Y: 1}) {
			t.Fatalf("%d 个顶点的多边形不该包含任何点", len(p))
		}
	}
}

// 凹多边形也要判对 —— 995 个门里有 485 个不是四边形。
func TestConcavePolygon(t *testing.T) {
	// 一个 L 形
	l := Polygon{
		{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 40},
		{X: 40, Y: 40}, {X: 40, Y: 100}, {X: 0, Y: 100},
	}
	if !l.Contains(Pos{X: 20, Y: 20}) {
		t.Error("L 形的拐角内侧应算在内")
	}
	if !l.Contains(Pos{X: 80, Y: 20}) {
		t.Error("L 形的横臂应算在内")
	}
	if l.Contains(Pos{X: 80, Y: 80}) {
		t.Error("L 形的缺口应算在外 —— 凸包算法会在这里判错")
	}
}

func TestPolygonBounds(t *testing.T) {
	minX, minY, maxX, maxY := 南郊门区域.Bounds()
	if minX != 5307 || maxX != 5577 || minY != 6906 || maxY != 7066 {
		t.Fatalf("外接矩形不对: x[%.0f,%.0f] y[%.0f,%.0f]", minX, maxX, minY, maxY)
	}
	if _, _, mx, _ := (Polygon{}).Bounds(); mx != 0 {
		t.Error("空多边形应给零值而不是崩")
	}
}

// Portal.Hit 先过外接矩形再跑射线法, 两条路径的结论必须一致。
func TestPortalHitMatchesContains(t *testing.T) {
	p := NewPortal(Teleport{Area: 南郊门区域, To: Pos{MapID: 14}})
	for x := 5250.0; x <= 5650; x += 7 {
		for y := 6850.0; y <= 7120; y += 7 {
			pt := Pos{X: x, Y: y}
			if p.Hit(pt) != 南郊门区域.Contains(pt) {
				t.Fatalf("(%.0f,%.0f) 两条路径结论不一致 —— 外接矩形筛错了", x, y)
			}
		}
	}
}

// 没有触发区域的门(全服 144 个)踩不出来, 但也不能崩。
func TestPortalWithoutAreaNeverHits(t *testing.T) {
	p := NewPortal(Teleport{To: Pos{MapID: 14}})
	if p.Hit(Pos{X: 0, Y: 0}) || p.Hit(Pos{X: 5442, Y: 6986}) {
		t.Fatal("没有区域数据的门不该被踩中")
	}
}
