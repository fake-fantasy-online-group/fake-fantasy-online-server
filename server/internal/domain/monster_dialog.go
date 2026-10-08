package domain

// 场景号来自客户端 ResDef 的 RES_SCENE_* 常量。PICK=1 在 ov_dialog
// 没有台词；当前战斗系统也没有怪物主动逃跑动作，因此不会触发 RUNAWAY。
const (
	MonsterSceneNormal  uint8 = 0
	MonsterScenePick    uint8 = 1
	MonsterSceneEnemy   uint8 = 2
	MonsterSceneAttack  uint8 = 3
	MonsterSceneBeHit   uint8 = 4
	MonsterSceneDefeat  uint8 = 5
	MonsterSceneDie     uint8 = 6
	MonsterSceneRunaway uint8 = 7
)

// MonsterDialog 是 ov_dialog 的一条原始怪物台词。Scene 对应客户端
// RES_SCENE_*，Group 保留原表组号，不推断原作的选组概率。
type MonsterDialog struct {
	Monster MonsterID
	Scene   uint8
	Group   uint8
	Text    string
}

type monsterDialogKey struct {
	monster MonsterID
	scene   uint8
	group   uint8
}

type monsterDialogSceneKey struct {
	monster MonsterID
	scene   uint8
}

// MonsterDialogs 按怪物、场景编号、组号索引台词，并保持数据库 row_no 顺序。
// 构造后只读；Lines 返回副本，避免调用方改动场景间共享的配置。
type MonsterDialogs struct {
	lines    map[monsterDialogKey][]string
	byScene  map[monsterDialogSceneKey][]string
	rows     int
	monsters int
}

func NewMonsterDialogs(entries []MonsterDialog) *MonsterDialogs {
	result := &MonsterDialogs{
		lines:   make(map[monsterDialogKey][]string),
		byScene: make(map[monsterDialogSceneKey][]string),
	}
	seen := make(map[MonsterID]struct{})
	for _, entry := range entries {
		key := monsterDialogKey{entry.Monster, entry.Scene, entry.Group}
		result.lines[key] = append(result.lines[key], entry.Text)
		sceneKey := monsterDialogSceneKey{entry.Monster, entry.Scene}
		result.byScene[sceneKey] = append(result.byScene[sceneKey], entry.Text)
		result.rows++
		seen[entry.Monster] = struct{}{}
	}
	result.monsters = len(seen)
	return result
}

// SceneLines 返回某个怪物在原始场景编号下的所有台词。组号的选取规则尚无
// 原作证据；运行时统一从本场景所有原文中选取，避免丢掉组 2~4 的内容。
func (d *MonsterDialogs) SceneLines(monster MonsterID, scene uint8) []string {
	if d == nil {
		return nil
	}
	return append([]string(nil), d.byScene[monsterDialogSceneKey{monster, scene}]...)
}

func (d *MonsterDialogs) Lines(monster MonsterID, scene, group uint8) []string {
	if d == nil {
		return nil
	}
	lines := d.lines[monsterDialogKey{monster, scene, group}]
	return append([]string(nil), lines...)
}

func (d *MonsterDialogs) RowCount() int {
	if d == nil {
		return 0
	}
	return d.rows
}

func (d *MonsterDialogs) MonsterCount() int {
	if d == nil {
		return 0
	}
	return d.monsters
}
