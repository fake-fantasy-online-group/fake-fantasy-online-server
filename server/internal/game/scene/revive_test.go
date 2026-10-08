package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestReviveRespawn(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Revive: ReviveAt{Scene: domain.SceneID{MapID: 1}, Pos: domain.Pos{MapID: 1, X: 50, Y: 50}},
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 杀死玩家
	p := s.entities[1]
	p.HP = 0
	s.onDeath(p, 0)
	s.step()
	sink.take() // 清掉死亡事件

	if p.Alive() {
		t.Fatal("玩家应该已经死了")
	}

	// 主动回城复活
	s.exec(Revive{ID: 1, ReviveType: ReviveTypeRespawn})
	s.step()

	// 玩家应该满血并被传送(实际传送需要路由器支持, 这里只检查满血)
	if !p.Alive() || p.HP != p.MaxHP {
		t.Fatalf("复活后应满血, 实际 %d/%d", p.HP, p.MaxHP)
	}
	if !p.Player.Protected(s.Tick()) {
		t.Fatal("复活后应有保护")
	}

	// 应该有 PlayerRevived 事件
	evs := sink.take()
	if _, ok := firstOf[event.PlayerRevived](evs); !ok {
		t.Fatal("没有 PlayerRevived 事件")
	}
}

func TestReviveInPlaceNotImplemented(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 杀死玩家
	p := s.entities[1]
	p.HP = 0
	s.onDeath(p, 0)
	s.step()
	sink.take()

	// 试图原地复活(需要复活卷轴)
	s.exec(Revive{ID: 1, ReviveType: ReviveTypeInPlace})
	s.step()

	// 应该被拒(当前未实现)
	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok {
		t.Fatal("原地复活应该被拒(未实现)")
	}
	if rej.Reason != event.RejectNotEnough {
		t.Errorf("拒绝理由不对: 期望 NotEnough, 实际 %v", rej.Reason)
	}
	if p.Alive() {
		t.Fatal("被拒后不应该复活")
	}
}

func TestReviveAlreadyAlive(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 活着时试图复活
	s.exec(Revive{ID: 1, ReviveType: ReviveTypeRespawn})
	s.step()

	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok {
		t.Fatal("活着时复活应该被拒")
	}
	if rej.Reason != event.RejectAlreadyAlive {
		t.Errorf("拒绝理由不对: 期望 AlreadyAlive, 实际 %v", rej.Reason)
	}
}

func TestDeathWaitsForClientRevive(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 杀死玩家。
	p := s.entities[1]
	p.HP = 0
	s.onDeath(p, 0) // 模拟死亡处理
	s.step()
	sink.take()

	// 没有客户端复活指令时，推进再多帧也必须保持死亡态。
	for i := 0; i < 200; i++ {
		s.step()
	}

	if p.Alive() || p.HP != 0 {
		t.Fatalf("未收到客户端指令前应保持死亡, 实际 %d/%d", p.HP, p.MaxHP)
	}
}

func TestClientReviveOnlyHappensOnce(t *testing.T) {
	s := New(Config{ID: domain.SceneID{MapID: 7}})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 杀死玩家
	p := s.entities[1]
	p.HP = 0
	s.onDeath(p, 0)
	s.step()
	sink.take()

	// 正式客户端选择回城复活。
	s.exec(Revive{ID: 1, ReviveType: ReviveTypeRespawn})
	s.step()
	sink.take()

	if !p.Alive() {
		t.Fatal("主动复活应该立即生效")
	}

	// 继续推进不得凭空产生第二次复活。
	for i := 0; i < 200; i++ {
		s.step()
	}

	// 不应该有第二次复活事件
	evs := sink.take()
	for _, ev := range evs {
		if _, ok := ev.(event.PlayerRevived); ok {
			t.Fatal("客户端只发了一次指令，不应该有第二次复活")
		}
	}
}
