package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestAllocPoints(t *testing.T) {
	s := newTestScene(t, nil)
	charID := int64(100)
	ch := &domain.Character{
		ID: charID, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take() // 清掉进场事件

	// 给 5 个自由点
	ch.FreePoints = 5
	old := ch.Base

	// 加 3 点力量、2 点体质(增量)
	s.exec(AllocPoints{
		ID:  1,
		STR: 3,
		VIT: 2,
	})
	s.step()

	if ch.Base.STR != old.STR+3 || ch.Base.VIT != old.VIT+2 {
		t.Errorf("加点没生效: 旧=%+v 新=%+v", old, ch.Base)
	}
	if ch.FreePoints != 0 {
		t.Errorf("自由点没扣: %d", ch.FreePoints)
	}

	// 应该有属性快照下发
	evs := sink.take()
	if _, ok := firstOf[event.StatsChanged](evs); !ok {
		t.Fatal("加点后没下发属性快照")
	}
}

// 不能减点(不能洗点): 任一维增量为负都拒。
func TestAllocPointsNoWashout(t *testing.T) {
	s := newTestScene(t, nil)
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	ch.FreePoints = 10
	old := ch.Base

	// 试图把力量减 2、体质加 2(增量含负数)
	s.exec(AllocPoints{
		ID:  1,
		STR: -2,
		VIT: 2,
	})
	s.step()

	evs := sink.take()
	if _, ok := firstOf[event.Rejected](evs); !ok {
		t.Fatal("减点应该被拒")
	}
	if ch.Base.STR != old.STR {
		t.Fatal("减点被接受了")
	}
}

// wire 未证明槽(d7[5]) 带非零值时仍应正常加点(客户端实测确实会写)。
func TestAllocPointsUnknownSlotAccepted(t *testing.T) {
	s := newTestScene(t, nil)
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	ch.FreePoints = 10
	old := ch.Base

	// 合法的 3 点力量, 未知槽带非零值(实测 93)
	s.exec(AllocPoints{
		ID:    1,
		STR:   3,
		Slot5: 93,
	})
	s.step()

	if ch.Base.STR != old.STR+3 {
		t.Fatal("带未知槽的加点应该被接受")
	}
	if ch.FreePoints != 7 {
		t.Fatalf("自由点应扣 3: got %d", ch.FreePoints)
	}
}

// 自由点不够, 拒掉。
func TestAllocPointsNotEnoughFreePoints(t *testing.T) {
	s := newTestScene(t, nil)
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	ch.FreePoints = 2
	old := ch.Base

	// 想加 5 点, 但只有 2 点
	s.exec(AllocPoints{
		ID:  1,
		STR: 5,
	})
	s.step()

	evs := sink.take()
	if _, ok := firstOf[event.Rejected](evs); !ok {
		t.Fatal("自由点不够应该被拒")
	}
	if ch.Base.STR != old.STR {
		t.Fatal("超额加点被接受了")
	}
}
