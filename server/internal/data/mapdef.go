package data

import (
	"context"
	"fmt"
)

// MapDef 是一张地图的定义。
type MapDef struct {
	ID   int32
	File string
	Name string
}

// MapCatalog 是全部地图的目录(id 索引)。
type MapCatalog struct {
	byID   map[int32]MapDef
	byFile map[string]MapDef
}

// LoadMapCatalog 从 PostgreSQL 加载地图目录。运行时不读取客户端目录。
func LoadMapCatalog(ctx context.Context, q Querier) (*MapCatalog, error) {
	rows, err := q.Query(ctx, `SELECT id,file,name FROM map_defs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("data: 查 map_defs: %w", err)
	}
	defer rows.Close()

	c := &MapCatalog{byID: make(map[int32]MapDef), byFile: make(map[string]MapDef)}
	for rows.Next() {
		var d MapDef
		if err := rows.Scan(&d.ID, &d.File, &d.Name); err != nil {
			return nil, fmt.Errorf("data: 读 map_defs 行: %w", err)
		}
		c.byID[d.ID] = d
		c.byFile[d.File] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历 map_defs: %w", err)
	}
	return c, nil
}

func (c *MapCatalog) ByID(id int32) (MapDef, bool)   { d, ok := c.byID[id]; return d, ok }
func (c *MapCatalog) ByFile(f string) (MapDef, bool) { d, ok := c.byFile[f]; return d, ok }
func (c *MapCatalog) Len() int                       { return len(c.byID) }

// All 返回全部地图定义(顺序不保证)。
func (c *MapCatalog) All() []MapDef {
	out := make([]MapDef, 0, len(c.byID))
	for _, d := range c.byID {
		out = append(out, d)
	}
	return out
}
