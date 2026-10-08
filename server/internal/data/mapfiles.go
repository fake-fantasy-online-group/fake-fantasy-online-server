package data

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// 解析从客户端解包出的地图导入文件(均为 gbk 明文):
//   .lst 怪物刷新点   .link 传送门
// 游戏运行时读取 PostgreSQL，不调用这些文件导入解析器。

// Spawn 是一个怪物刷新点(来自 <map>.lst)。
//
//	id 111 / index 31072 / init_pos 4535 7446 / init_dir 67
type Spawn struct {
	ID      int32 // 图内刷新点编号
	Monster int32 // index, 对应怪物表 id
	X, Y    int32
	Dir     int32
}

// ParseSpawnFile 解析 .lst。格式是「键 值」以空格分隔, 空行分隔记录。
func ParseSpawnFile(path string) ([]Spawn, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data: 读 %s: %w", path, err)
	}
	var out []Spawn
	var cur Spawn
	var has bool
	flush := func() {
		if has {
			out = append(out, cur)
			cur, has = Spawn{}, false
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "id":
			flush() // 新记录以 id 开头
			cur.ID = atoi32(f[1])
			has = true
		case "index":
			cur.Monster = atoi32(f[1])
		case "init_pos":
			if len(f) >= 3 {
				cur.X, cur.Y = atoi32(f[1]), atoi32(f[2])
			}
		case "init_dir":
			cur.Dir = atoi32(f[1])
		}
	}
	flush()
	return out, sc.Err()
}

// Teleport 是一个传送门(来自 <map>.link)。
//
//	proc_id 1 / map_to 84 / map_to_pos 11420 4923 / map_to_dir 112 / map_back_pos 8239 7553
type Teleport struct {
	ProcID       int32 // 对应 .prc 里的区域编号(踩到触发)
	MapTo        int32 // 目标地图 id
	Script       string
	ToX, ToY     int32
	ToDir        int32
	BackX, BackY int32
	BackDir      int32
}

// ParseTeleportFile 解析 .link。
func ParseTeleportFile(path string) ([]Teleport, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data: 读 %s: %w", path, err)
	}
	var out []Teleport
	var cur Teleport
	var has bool
	flush := func() {
		if has {
			out = append(out, cur)
			cur, has = Teleport{}, false
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "proc_id":
			flush()
			cur.ProcID = atoi32(f[1])
			has = true
		case "map_to":
			cur.MapTo = atoi32(f[1])
		case "map_script":
			cur.Script = f[1]
		case "map_to_pos":
			if len(f) >= 3 {
				cur.ToX, cur.ToY = atoi32(f[1]), atoi32(f[2])
			}
		case "map_to_dir":
			cur.ToDir = atoi32(f[1])
		case "map_back_pos":
			if len(f) >= 3 {
				cur.BackX, cur.BackY = atoi32(f[1]), atoi32(f[2])
			}
		case "map_back_dir":
			cur.BackDir = atoi32(f[1])
		}
	}
	flush()
	return out, sc.Err()
}
