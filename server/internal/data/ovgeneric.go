package data

// 通用 ov 表读取由列计划驱动。字段名取自客户端数据表定义；其余字段依据记录布局辨认，
// 列类型根据数据格式确定。

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// OvColumn 一列在定长记录里的位置与类型。
type OvColumn struct {
	// Signed: 1 字节列按有符号 int8 读取；其他列按无符号数值读取。
	Signed bool   `json:"signed"`
	Name   string `json:"name"`
	Off    int    `json:"off"`
	Size   int    `json:"size"`
	Type   string `json:"type"` // smallint / integer / bigint / text / bytea
}

// OvTablePlan 一张表: SQL 表名 -> (源文件名, 列定义)。
// 表名与文件名分开是因为 SQL 表名必须小写(PG 折叠未加引号标识符), 而文件名保持原样。
type OvTablePlan struct {
	File string     `json:"file"`
	Cols []OvColumn `json:"cols"`
}

// OvPlan SQL 表名 -> 表计划。
type OvPlan map[string]OvTablePlan

// LoadOvPlan 读取列计划。
func LoadOvPlan(path string) (OvPlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p OvPlan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("列计划 %s: %w", path, err)
	}
	return p, nil
}

// ovRaw 读出一张定长记录表，格式为:
// [u32 magic=0x5566][u32 ver][u32 recordSize][u32 rowCount][定长记录 × rowCount]
func ovRaw(path string) (rows [][]byte, recSize int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(b) < 16 {
		return nil, 0, fmt.Errorf("%s: 文件过短", path)
	}
	magic := binary.LittleEndian.Uint32(b[0:])
	rs := int(binary.LittleEndian.Uint32(b[8:]))
	rc := int(binary.LittleEndian.Uint32(b[12:]))
	if magic != 0x5566 || rs <= 0 || 16+rs*rc != len(b) {
		return nil, 0, fmt.Errorf("%s: 非 0x5566 定长表(magic=%#x rs=%d rc=%d len=%d)",
			filepath.Base(path), magic, rs, rc, len(b))
	}
	rows = make([][]byte, rc)
	for i := 0; i < rc; i++ {
		rows[i] = b[16+i*rs : 16+(i+1)*rs]
	}
	return rows, rs, nil
}

// cutGBK 定长字符串字段: 读到第一个 \0 截断, GBK 转 UTF-8。
func cutGBK(b []byte) string {
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	if len(b) == 0 {
		return ""
	}
	s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	if err != nil {
		return string(b) // 转码失败按原样保留, 不丢内容
	}
	return string(s)
}

// OvRepeat 单行 × N 个重复子记录的表(如 ov_cycletrunk 的 500 条循环任务)。
// 摊成宽表会超过 PostgreSQL 单表 1600 列的上限, 所以拆成子表: 每个子记录一行。
type OvRepeat struct {
	// Src 指定数据来自哪张父表。为空时取配置的 key —— 老配置都是这样,
	// 保持向后兼容。一张父表有**多个**重复块时(如 ov_starattr 的 4 组
	// ArmType 数组), key 必须是各自的子表名, 由 Src 指回同一个 .bin。
	Src    string `json:"src"`
	Header int    `json:"header"`
	Block  int    `json:"block"`
	Count  int    `json:"count"`
	// Inner 描述**块内还有一层重复**(ov_exceptdetail: 40 级 x 16 个效果 op)。
	// 为空表示只有一层。有 Inner 时子表多一列 idx2, Cols 的偏移相对**内层块**。
	Inner *OvInner `json:"inner"`
	Cols  [][]any  `json:"cols"` // [名字, 块内偏移, 长度, pg类型]
}

// OvInner 外层块内的第二层重复。
type OvInner struct {
	Off   int `json:"off"`   // 内层数组在外层块中的起始偏移
	Block int `json:"block"` // 每个内层块字节数
	Count int `json:"count"` // 内层块个数
}

// LoadOvRepeat 读取重复块配置。
func LoadOvRepeat(path string) (map[string]OvRepeat, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]OvRepeat
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("重复块配置 %s: %w", path, err)
	}
	return m, nil
}

// OvRepeatRows 展开重复子记录; 首两列固定为 (row_no, idx)。全零的块跳过。
func OvRepeatRows(zipbinDir, table string, r OvRepeat) ([][]any, []string, error) {
	src := r.Src
	if src == "" {
		src = table
	}
	rows, rs, err := ovRaw(filepath.Join(zipbinDir, src+".bin"))
	if err != nil {
		return nil, nil, err
	}
	names := []string{"row_no", "idx"}
	if r.Inner != nil {
		names = append(names, "idx2")
	}
	for _, c := range r.Cols {
		names = append(names, c[0].(string))
	}
	allZero := func(b []byte) bool {
		for _, x := range b {
			if x != 0 {
				return false
			}
		}
		return true
	}
	var out [][]any
	for ri, row := range rows {
		for k := 0; k < r.Count; k++ {
			base := r.Header + r.Block*k
			if base+r.Block > rs {
				break
			}
			outer := row[base : base+r.Block]
			if allZero(outer) {
				continue
			}
			// 两层: 外层块内再按 Inner 切一遍, 每个非零内层块一行
			inner := []struct {
				j   int
				blk []byte
			}{{0, outer}}
			if r.Inner != nil {
				inner = inner[:0]
				for j := 0; j < r.Inner.Count; j++ {
					o := r.Inner.Off + r.Inner.Block*j
					if o+r.Inner.Block > len(outer) {
						break
					}
					b := outer[o : o+r.Inner.Block]
					if !allZero(b) {
						inner = append(inner, struct {
							j   int
							blk []byte
						}{j, b})
					}
				}
			}
			for _, it := range inner {
				blk := it.blk
				rec := []any{ri, k}
				if r.Inner != nil {
					rec = append(rec, it.j)
				}
				for _, c := range r.Cols {
					off, size := int(c[1].(float64)), int(c[2].(float64))
					seg := blk[off : off+size]
					// 类型分支必须覆盖 1/2/4 字节整数 —— 只写 bigint 会在 2 字节列上
					// 报 "unable to encode []byte{...} into binary format for int4",
					// 因为 default 分支把原始字节当 bytea 塞给了 int4 列(实测被
					// ov_exceptdetail 的 interval(2B) 打中)。
					switch c[3].(string) {
					case "text":
						rec = append(rec, cutGBK(seg))
					case "bigint":
						rec = append(rec, int64(int32(binary.LittleEndian.Uint32(seg))))
					case "integer":
						// 默认**无符号**扩展: 多数 2 字节列(持续时间/间隔/数量)是非负量,
						// 按 int16 读会让 >32767 的值翻成负数(实测 ov_exceptdetail.LastTime
						// 出现 -29536, 实为 36000 = 10 小时)。
						rec = append(rec, int32(binary.LittleEndian.Uint16(seg)))
					case "integer_s":
						// 确实有符号的 2 字节列走这个类型。实测 ov_NewPVPGrow 的
						// RandomAttrLowLimit 取值 65526/65528/65530/65532, 即 int16 的
						// -10/-8/-6/-4 —— 洗练属性区间的下限本来就是负数。
						rec = append(rec, int32(int16(binary.LittleEndian.Uint16(seg))))
					case "smallint":
						rec = append(rec, int16(seg[0]))
					default:
						cp := make([]byte, len(seg))
						copy(cp, seg)
						rec = append(rec, cp)
					}
				}
				out = append(out, rec)
			}
		}
	}
	return out, names, nil
}

// OvRows 按列计划把一张表读成 [][]any, 可直接喂 pgx.CopyFromRows。
// 首列固定是 row_no(行号), 保证每张表都有主键 —— 原始数据里不少表的 id 列有重复。
func OvRows(zipbinDir, table string, cols []OvColumn) ([][]any, error) {
	rows, rs, err := ovRaw(filepath.Join(zipbinDir, table+".bin"))
	if err != nil {
		return nil, err
	}
	out := make([][]any, 0, len(rows))
	for i, r := range rows {
		rec := make([]any, 0, len(cols)+1)
		rec = append(rec, i)
		for _, c := range cols {
			if c.Off+c.Size > rs {
				rec = append(rec, nil)
				continue
			}
			seg := r[c.Off : c.Off+c.Size]
			switch c.Type {
			case "text":
				rec = append(rec, cutGBK(seg))
			case "smallint":
				// 默认无符号; 只有列计划里明确标了 signed 的才按 int8 读。
				// (ov_petnickgrow 的成长列: 取值 {0,1,255}, 按 int8 读后每行和恒为 0)
				if c.Signed && c.Size == 1 {
					rec = append(rec, int16(int8(seg[0])))
				} else {
					rec = append(rec, int16(seg[0]))
				}
			case "integer":
				// **默认无符号** —— 2 字节列多数是 id, 而 id 空间远超 32767
				// (ov_item 到 43226), 按 int16 读会变负、JOIN 全失效。
				// 真有符号的走 gen_sql 的 SIGNED16 白名单标 signed。
				if c.Signed {
					rec = append(rec, int32(int16(binary.LittleEndian.Uint16(seg))))
				} else {
					rec = append(rec, int32(binary.LittleEndian.Uint16(seg)))
				}
			case "bigint":
				rec = append(rec, int64(int32(binary.LittleEndian.Uint32(seg))))
			default: // bytea: 未解出结构的定长块原样保留, 一个字节不丢
				cp := make([]byte, len(seg))
				copy(cp, seg)
				rec = append(rec, cp)
			}
		}
		out = append(out, rec)
	}
	return out, nil
}
