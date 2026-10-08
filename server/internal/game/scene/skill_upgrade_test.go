package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// testSkills 是测试用的最小技能表。
func testSkills() domain.SkillTable {
	return domain.SkillTable{
		{ID: 1, Level: 1}:   {ID: 1, Level: 1, Name: "普攻1", Prof: 1, LevelNeed: 1}, // 战士(Race=0, Prof=1)
		{ID: 1, Level: 2}:   {ID: 1, Level: 2, Name: "普攻2", Prof: 1, LevelNeed: 5},
		{ID: 1, Level: 3}:   {ID: 1, Level: 3, Name: "普攻3", Prof: 1, LevelNeed: 10},
		{ID: 1, Level: 4}:   {ID: 1, Level: 4, Name: "普攻4", Prof: 1, LevelNeed: 20},
		{ID: 1, Level: 5}:   {ID: 1, Level: 5, Name: "普攻5", Prof: 1, LevelNeed: 30},
		{ID: 5, Level: 1}:   {ID: 5, Level: 1, Name: "技能5", Prof: 1, LevelNeed: 10},
		{ID: 101, Level: 1}: {ID: 101, Level: 1, Name: "法师技", Prof: 5, LevelNeed: 1}, // 术士(Race=4, Prof=5)
	}
}

func TestUpgradeSkill(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Skills: testSkills(),
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 10, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take() // 清掉进场事件

	// 战士学「普通攻击」1 级 (技能 1, LevelNeed=1)
	s.exec(UpgradeSkill{ID: 1, Skill: 1})
	s.step()

	if ch.Skills.LevelOf(1) != 1 {
		t.Errorf("技能没学会: %v", ch.Skills)
	}

	// 应该有技能快照下发
	evs := sink.take()
	snap, ok := firstOf[event.SkillSnapshot](evs)
	if !ok {
		t.Fatal("学技能后没下发技能快照")
	}
	if len(snap.Skills) != 1 || snap.Skills[0].ID != 1 || snap.Skills[0].Level != 1 {
		t.Errorf("技能快照不对: %+v", snap.Skills)
	}
}

// 升级已学技能。
func TestUpgradeSkillToLevel2(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Skills: testSkills(),
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 20, Race: domain.Warrior,
		Pos:    domain.Pos{MapID: 7, X: 100, Y: 100},
		Skills: domain.Learned{1: 1}, // 已经学了 1 级
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 升到 2 级
	s.exec(UpgradeSkill{ID: 1, Skill: 1})
	s.step()

	if ch.Skills.LevelOf(1) != 2 {
		t.Errorf("技能没升级: %v", ch.Skills)
	}
}

// 等级不够, 拒掉。
func TestUpgradeSkillLevelTooLow(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Skills: testSkills(),
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 1, Race: domain.Warrior,
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 试图学一个需要 10 级的技能
	s.exec(UpgradeSkill{ID: 1, Skill: 5})
	s.step()

	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok {
		t.Fatal("等级不够应该被拒")
	}
	if rej.Reason != event.RejectLevelTooLow {
		t.Errorf("拒绝理由不对: 期望 LevelTooLow, 实际 %v", rej.Reason)
	}
}

// 职业不符, 拒掉。
func TestUpgradeSkillWrongProf(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Skills: testSkills(),
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 50, Race: domain.Warrior, // 战士(Race=1, Prof=2)
		Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 试图学法师技能 (Prof=3)
	s.exec(UpgradeSkill{ID: 1, Skill: 101})
	s.step()

	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok {
		t.Fatal("职业不符应该被拒")
	}
	if rej.Reason != event.RejectWrongProf {
		t.Errorf("拒绝理由不对: 期望 WrongProf, 实际 %v", rej.Reason)
	}
}

// 已满级(5 级), 拒掉。
func TestUpgradeSkillMaxLevel(t *testing.T) {
	s := New(Config{
		ID:     domain.SceneID{MapID: 7},
		Skills: testSkills(),
	})
	ch := &domain.Character{
		ID: 100, Name: "测试", Level: 50, Race: domain.Warrior,
		Pos:    domain.Pos{MapID: 7, X: 100, Y: 100},
		Skills: domain.Learned{1: 5}, // 已经 5 级
	}
	sink := &fakeSink{}
	s.exec(Enter{ID: 1, Char: ch, Sink: sink})
	s.step()
	sink.take()

	// 试图升到 6 级
	s.exec(UpgradeSkill{ID: 1, Skill: 1})
	s.step()

	evs := sink.take()
	rej, ok := firstOf[event.Rejected](evs)
	if !ok {
		t.Fatal("满级后升级应该被拒")
	}
	if rej.Reason != event.RejectMaxLevel {
		t.Errorf("拒绝理由不对: 期望 MaxLevel, 实际 %v", rej.Reason)
	}
	if ch.Skills.LevelOf(1) != 5 {
		t.Fatal("满级后技能等级被改了")
	}
}
