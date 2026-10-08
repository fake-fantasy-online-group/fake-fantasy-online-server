package data

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// LoadDungeons 从 PostgreSQL 读取正式服可用的副本定义。pworld.def 解析器继续
// 保留为离线取证工具；生产运行时不再把客户端目录当业务配置源。
func LoadDungeons(ctx context.Context, q Querier) (domain.DungeonTable, error) {
	rows, err := q.Query(ctx, `
		SELECT dungeon_id, name, map_id, enter_x, enter_y,
		       exit_map_id, exit_x, exit_y, timeout_sec, max_instances, solo
		  FROM game_dungeons
	 ORDER BY dungeon_id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_dungeons: %w", err)
	}
	defer rows.Close()

	out := domain.DungeonTable{}
	for rows.Next() {
		var id, mapID, enterX, enterY, exitMapID, exitX, exitY, timeout, maxInstances int32
		var name string
		var solo bool
		if err := rows.Scan(&id, &name, &mapID, &enterX, &enterY,
			&exitMapID, &exitX, &exitY, &timeout, &maxInstances, &solo); err != nil {
			return nil, fmt.Errorf("data: 读 game_dungeons: %w", err)
		}
		if id <= 0 || name == "" || mapID <= 0 || exitMapID <= 0 ||
			enterX < 0 || enterY < 0 || exitX < 0 || exitY < 0 ||
			timeout < 0 || maxInstances < 0 {
			return nil, fmt.Errorf("data: 副本定义非法 id=%d map=%d name=%q", id, mapID, name)
		}
		if _, exists := out[mapID]; exists {
			return nil, fmt.Errorf("data: 副本地图 %d 重复", mapID)
		}
		out[mapID] = domain.DungeonDef{
			ID: domain.DungeonID(id), Name: name,
			Enter:      domain.Pos{MapID: mapID, X: float64(enterX), Y: float64(enterY)},
			Exit:       domain.Pos{MapID: exitMapID, X: float64(exitX), Y: float64(exitY)},
			TimeoutSec: timeout, MaxInstances: maxInstances, Solo: solo,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_dungeons: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: game_dungeons 为空")
	}
	groups := make(map[domain.DungeonID]domain.DungeonDef)
	for _, def := range out {
		if first, exists := groups[def.ID]; exists {
			if first.Exit != def.Exit || first.TimeoutSec != def.TimeoutSec ||
				first.MaxInstances != def.MaxInstances || first.Solo != def.Solo {
				return nil, fmt.Errorf("data: 多层副本 %d 的出口、限时或实例上限不一致", def.ID)
			}
			continue
		}
		groups[def.ID] = def
	}
	return out, nil
}

// LoadDungeonEncounters 读取数据库驱动的副本首领阶段链。
func LoadDungeonEncounters(ctx context.Context, q Querier, dungeons domain.DungeonTable,
	monsters MonsterDefs) (domain.DungeonEncounterTable, error) {
	rows, err := q.Query(ctx, `
		SELECT map_id, trigger_monster_id, next_monster_id
		  FROM game_dungeon_encounter_steps
	 ORDER BY map_id, step_no`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 game_dungeon_encounter_steps: %w", err)
	}
	defer rows.Close()

	out := domain.DungeonEncounterTable{}
	for rows.Next() {
		var mapID int32
		var trigger, next domain.MonsterID
		if err := rows.Scan(&mapID, &trigger, &next); err != nil {
			return nil, fmt.Errorf("data: 读 game_dungeon_encounter_steps: %w", err)
		}
		if _, ok := dungeons[mapID]; !ok {
			return nil, fmt.Errorf("data: 副本阶段链引用非副本地图 %d", mapID)
		}
		if _, ok := monsters[trigger]; !ok {
			return nil, fmt.Errorf("data: 副本阶段链触发怪 %d 不存在", trigger)
		}
		if _, ok := monsters[next]; !ok {
			return nil, fmt.Errorf("data: 副本阶段链下一怪 %d 不存在", next)
		}
		key := domain.DungeonEncounterKey{MapID: mapID, Trigger: trigger}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("data: 副本阶段链重复 map=%d trigger=%d", mapID, trigger)
		}
		out[key] = next
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 game_dungeon_encounter_steps: %w", err)
	}
	return out, nil
}

// 副本定义的离线证据来源。
//
//	pworld.def              INI 风格, GBK 编码, 69 个 [Pworld] 块
//	dungeon_entrances.json  入口: NPC → 能送进哪些副本
//
// ⚠️ 两个文件对不上的地方: pworld.def 里 66/69 没有 MaxInst, 7 个 Timeout=0。
// 那些缺口由 domain 里的默认值兜, 不猜。正式服务启动只读 PostgreSQL；
// 下方文件解析器用于生成、复核或研究，不进入生产运行路径。

// ParsePworldDef 解析 pworld.def。
//
// 文件是 **GBK** 的 —— 副本名里有中文, 按 UTF-8 读会全是乱码,
// 而乱码的名字会一路带到日志和客户端。
func ParsePworldDef(path string) (domain.DungeonTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data: 读 %s: %w", path, err)
	}
	out := domain.DungeonTable{}
	var cur map[string]string

	flush := func() {
		if cur == nil {
			return
		}
		d, ok := buildDungeon(cur)
		if ok {
			out[d.Enter.MapID] = d
		}
		cur = nil
	}

	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			flush()
			if strings.EqualFold(line, "[Pworld]") {
				cur = map[string]string{}
			}
			continue
		}
		if cur == nil {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		cur[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("data: 扫 %s: %w", path, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: %s 里一个副本都没解出来", path)
	}
	return out, nil
}

func buildDungeon(kv map[string]string) (domain.DungeonDef, bool) {
	initMap := iniInt(kv["InitMapID"])
	if initMap == 0 {
		return domain.DungeonDef{}, false
	}
	d := domain.DungeonDef{
		ID:           domain.DungeonID(iniInt(kv["PworldID"])),
		Name:         gbkToUTF8(kv["Name"]),
		TimeoutSec:   iniInt(kv["Timeout"]),
		MaxInstances: iniInt(kv["MaxInst"]),
	}
	ix, iy := parsePos(kv["InitPos"])
	d.Enter = domain.Pos{MapID: initMap, X: ix, Y: iy}

	exitMap := iniInt(kv["ExpMapID"])
	ex, ey := parsePos(kv["ExpPos"])
	if exitMap == 0 {
		exitMap = initMap // 没写出口就原地出来 —— 总比把人扔到 0 号图强
	}
	d.Exit = domain.Pos{MapID: exitMap, X: ex, Y: ey}
	return d, true
}

// iniInt 读一个整数键, 缺失或非法都当 0 —— 缺省值由 domain 那边兜。
func iniInt(s string) int32 {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return int32(n)
}

func parsePos(s string) (float64, float64) {
	x, y, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0
	}
	fx, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
	fy, _ := strconv.ParseFloat(strings.TrimSpace(y), 64)
	return fx, fy
}

// gbkToUTF8 把 GBK 字节转成 UTF-8。
//
// 用 x/text 的现成解码器, 不自己写 GBK 表 —— 手写的表迟早在某个生僻字上出错,
// 而那个错会一路带到日志和客户端。
// 转不出来的原样返回, 不报错: 一个名字乱码不该让整张副本表加载失败。
func gbkToUTF8(s string) string {
	out, err := simplifiedchinese.GBK.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return out
}

// LoadDungeonEntrances 读 dungeon_entrances.json。
//
// 结构是 NPC → 一串副本。查询方向就是这个: 玩家点了一个 NPC,
// 要知道他能送去哪些副本。
func LoadDungeonEntrances(path string) (domain.EntranceTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data: 读 %s: %w", path, err)
	}
	// 文件外面还套了一层, 取里面那个数组
	var wrapper map[string][]struct {
		NPC      string `json:"npc"`
		Dungeons []struct {
			Name string  `json:"name"`
			Map  int32   `json:"map"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
		} `json:"dungeons"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("data: 解析 %s: %w", path, err)
	}
	out := domain.EntranceTable{}
	for _, list := range wrapper {
		for _, n := range list {
			for _, d := range d2(n.Dungeons) {
				out[n.NPC] = append(out[n.NPC], domain.DungeonEntrance{
					NPC: n.NPC, Name: d.name, MapID: d.mapID,
					Enter: domain.Pos{MapID: d.mapID, X: d.x, Y: d.y},
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("data: %s 里一个入口都没有", path)
	}
	return out, nil
}

type entryTmp struct {
	name  string
	mapID int32
	x, y  float64
}

func d2(in []struct {
	Name string  `json:"name"`
	Map  int32   `json:"map"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}) []entryTmp {
	out := make([]entryTmp, 0, len(in))
	for _, d := range in {
		if d.Map == 0 {
			continue
		}
		out = append(out, entryTmp{d.Name, d.Map, d.X, d.Y})
	}
	return out
}

// DungeonPaths 返回两个文件的默认位置。
func DungeonPaths(clientDir string) (pworld, entrances string) {
	return filepath.Join(clientDir, "pkg", "map", "pworld.def"),
		filepath.Join(clientDir, "dungeon_entrances.json")
}
