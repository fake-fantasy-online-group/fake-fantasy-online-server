package data

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

const (
	maskHeaderSize = 32
	maskBlockedBit = 0x80
	maxMaskRawSize = 64 << 20
)

var (
	ErrCollisionMaskMissing       = errors.New("data: 原始资源确认缺少碰撞图")
	ErrCollisionMaskNotConfigured = errors.New("data: 碰撞图配置行不存在")
)

var maskMagic = []byte{'Q', 'Q', 'F', 0x1a, 'M', 'A', 'S', 'K'}

// CollisionMask 是客户端 QQF MASK 的只读碰撞网格。bit7 置位表示阻挡，
// 包括水面、地图外缘和不可穿越地形。
type CollisionMask struct {
	width, height         uint32
	cellWidth, cellHeight uint32
	cells                 []byte
}

// loadCollisionMask 验证 QQF MASK。格式为 32 字节头，之后每格一个字节。
func loadCollisionMask(resource string, b []byte) (*CollisionMask, error) {
	if len(b) < maskHeaderSize || !bytes.Equal(b[:8], maskMagic) {
		return nil, fmt.Errorf("data: %s 不是 QQF MASK", resource)
	}
	m := &CollisionMask{
		width:      binary.LittleEndian.Uint32(b[16:20]),
		height:     binary.LittleEndian.Uint32(b[20:24]),
		cellWidth:  binary.LittleEndian.Uint32(b[24:28]),
		cellHeight: binary.LittleEndian.Uint32(b[28:32]),
	}
	cellCount := uint64(m.width) * uint64(m.height)
	if m.width == 0 || m.height == 0 || m.cellWidth == 0 || m.cellHeight == 0 ||
		cellCount != uint64(len(b)-maskHeaderSize) {
		return nil, fmt.Errorf("data: 碰撞图 %s 尺寸无效: %dx%d cell=%dx%d bytes=%d",
			resource, m.width, m.height, m.cellWidth, m.cellHeight, len(b))
	}
	m.cells = b[maskHeaderSize:]
	return m, nil
}

// CollisionOfMap 按地图 id 从数据库惰性读取碰撞图。
func (c *MapCatalog) CollisionOfMap(ctx context.Context, q Querier, id int32) (*CollisionMask, error) {
	d, ok := c.ByID(id)
	if !ok {
		return nil, fmt.Errorf("data: 无地图 id=%d", id)
	}
	rows, err := q.Query(ctx, `SELECT mask_gzip,raw_size FROM game_map_collision WHERE resource_name=$1`, d.File)
	if err != nil {
		return nil, fmt.Errorf("data: 查地图碰撞图 %s: %w", d.File, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("data: 读地图碰撞图 %s: %w", d.File, err)
		}
		return nil, fmt.Errorf("%w: %s", ErrCollisionMaskNotConfigured, d.File)
	}
	var compressed []byte
	var rawSize int32
	if err := rows.Scan(&compressed, &rawSize); err != nil {
		return nil, fmt.Errorf("data: 读地图碰撞图 %s: %w", d.File, err)
	}
	if rows.Next() {
		return nil, fmt.Errorf("data: 地图碰撞图 %s 存在重复行", d.File)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: 遍历地图碰撞图 %s: %w", d.File, err)
	}
	if rawSize == 0 && len(compressed) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrCollisionMaskMissing, d.File)
	}
	if rawSize <= 0 || rawSize > maxMaskRawSize {
		return nil, fmt.Errorf("data: 地图碰撞图 %s 原始尺寸越界: %d", d.File, rawSize)
	}
	if len(compressed) == 0 {
		return nil, fmt.Errorf("data: 地图碰撞图 %s 压缩内容为空", d.File)
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("data: 解压地图碰撞图 %s: %w", d.File, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(zr, int64(rawSize)+1))
	closeErr := zr.Close()
	if readErr != nil {
		return nil, fmt.Errorf("data: 解压地图碰撞图 %s: %w", d.File, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("data: 关闭地图碰撞图 %s: %w", d.File, closeErr)
	}
	if len(raw) != int(rawSize) {
		return nil, fmt.Errorf("data: 地图碰撞图 %s 原始尺寸不符: expected=%d actual=%d", d.File, rawSize, len(raw))
	}
	return loadCollisionMask(d.File, raw)
}

// Walkable 判断世界坐标所在格是否可走。无穷、NaN、负数和地图外坐标都不可走。
func (m *CollisionMask) Walkable(pos domain.Pos) bool {
	if m == nil || math.IsNaN(pos.X) || math.IsNaN(pos.Y) ||
		math.IsInf(pos.X, 0) || math.IsInf(pos.Y, 0) || pos.X < 0 || pos.Y < 0 {
		return false
	}
	gx := uint32(pos.X) / m.cellWidth
	gy := uint32(pos.Y) / m.cellHeight
	if gx >= m.width || gy >= m.height {
		return false
	}
	return m.cells[uint64(gy)*uint64(m.width)+uint64(gx)]&maskBlockedBit == 0
}
