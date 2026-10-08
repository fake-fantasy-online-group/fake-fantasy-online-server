package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 与 domain/levelpenalty_test.go 同源：定案参数，直接打到场景字段上，
// 验证接入点（awardKill 经验、dropMultBP 掉率）真的把这些参数用上。
func 定案惩罚() domain.LevelPenalty {
	return domain.LevelPenalty{
		ExpFullAbove:  9,
		ExpStepBP:     800,
		ExpBelowStart: 1,
		DropFull:      5,
		DropStepBP:    800,
		DropFloorBP:   5000,
	}
}

// 越级打低级怪：22 级打 14 级树妖，级差 −8 → 经验 58 × 36% = 20。
func TestKillExpPenalizedBelow(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	s.penalty = 定案惩罚()
	sink := join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()
	sink.take()
	s.entities[1].Player.Char.Level = 22

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	ev, ok := firstOf[event.ExpGained](sink.take())
	if !ok {
		t.Fatal("打死怪应给经验")
	}
	if ev.Delta != 20 {
		t.Fatalf("22级打14级树妖应得 58×36%%=20, 实际 %d", ev.Delta)
	}
	if ch := s.entities[1].Player.Char; ch.Exp != 20 {
		t.Errorf("角色身上经验应是 20, 实际 %d", ch.Exp)
	}
}

// 同等级打怪不受惩罚：14 级打 14 级树妖，满经验 58。
func TestKillExpFullWhenSameLevel(t *testing.T) {
	s, mid := rewardScene(t, 给经验的树妖)
	s.penalty = 定案惩罚()
	sink := join(t, s, 1, 100, "甲", 100, 100)
	oneShot(s)
	s.step()
	sink.take()
	s.entities[1].Player.Char.Level = 14

	s.exec(Attack{ID: 1, Target: mid})
	s.step()

	ev, ok := firstOf[event.ExpGained](sink.take())
	if !ok {
		t.Fatal("打死怪应给经验")
	}
	if ev.Delta != 58 {
		t.Fatalf("14级打14级树妖应得满经验 58, 实际 %d", ev.Delta)
	}
}

// 跨级惩罚掉率：30 级打 14 级树妖，级差 −16 → 掉率倍率 50% 封底。
func TestDropMultPenaltyFloor(t *testing.T) {
	s, mid := lootScene(t, nil)
	s.penalty = 定案惩罚()
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	s.entities[1].Player.Char.Level = 30

	if got := s.dropMultBP(1, s.entities[mid].Monster.TypeID); got != 5000 {
		t.Fatalf("30级打14级树妖掉率倍率应封底 5000, 实际 %d", got)
	}
}

// 同等级掉率不惩罚。
func TestDropMultFullWhenSameLevel(t *testing.T) {
	s, mid := lootScene(t, nil)
	s.penalty = 定案惩罚()
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	s.entities[1].Player.Char.Level = 14

	if got := s.dropMultBP(1, s.entities[mid].Monster.TypeID); got != 10000 {
		t.Fatalf("14级打14级树妖掉率倍率应满 10000, 实际 %d", got)
	}
}

// 惩罚未启用（零值配置）时一律满倍率。
func TestDropMultDisabledWhenZeroConfig(t *testing.T) {
	s, mid := lootScene(t, nil)
	// 不设 s.penalty —— 保持零值
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	s.entities[1].Player.Char.Level = 100

	if got := s.dropMultBP(1, s.entities[mid].Monster.TypeID); got != 10000 {
		t.Fatalf("未启用惩罚时掉率倍率应满 10000, 实际 %d", got)
	}
}
