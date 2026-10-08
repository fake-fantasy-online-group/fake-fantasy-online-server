package data

import (
	"encoding/binary"
	"fmt"
	"os"
)

// 读取 ov_*.bin 定长表容器:
//
//	[u32 magic=0x5566][u32 ver][u32 recordSize][u32 rowCount][定长记录 × rowCount]
//
// 记录内部紧凑排布, 字符串是定长 GBK 数组、尾部补 \0。
// 精确解出的列由各自的 XxxRow 函数取; 其余整条 raw 保留, 不丢信息。

const ovMagic = 0x5566

// OvTable 是一张已加载的表。
type OvTable struct {
	Name       string
	Version    uint32
	RecordSize int
	Rows       [][]byte
}

// LoadOvTable 读一张 ov 表并校验容器格式。
func LoadOvTable(path, name string) (*OvTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data: 读 %s: %w", path, err)
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("data: %s 太短", name)
	}
	magic := binary.LittleEndian.Uint32(b[0:])
	ver := binary.LittleEndian.Uint32(b[4:])
	rs := int(binary.LittleEndian.Uint32(b[8:]))
	rc := int(binary.LittleEndian.Uint32(b[12:]))
	if magic != ovMagic {
		return nil, fmt.Errorf("data: %s magic=%#x 非 ov 表", name, magic)
	}
	if rs <= 0 || rc < 0 || 16+rs*rc != len(b) {
		return nil, fmt.Errorf("data: %s 尺寸不符: 16+%d*%d != %d", name, rs, rc, len(b))
	}
	rows := make([][]byte, rc)
	for i := 0; i < rc; i++ {
		rows[i] = b[16+i*rs : 16+(i+1)*rs]
	}
	return &OvTable{Name: name, Version: ver, RecordSize: rs, Rows: rows}, nil
}

// i32 取记录内偏移处的 int32。
func i32(rec []byte, off int) int32 {
	if off+4 > len(rec) {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(rec[off:]))
}

// u8/u32 取记录内偏移处的无符号整数。
func u8(rec []byte, off int) uint8 {
	if off >= len(rec) {
		return 0
	}
	return rec[off]
}

func u32(rec []byte, off int) uint32 {
	if off+4 > len(rec) {
		return 0
	}
	return binary.LittleEndian.Uint32(rec[off:])
}

// gbkField 取记录内定长 GBK 字符串字段(到 \0 截断)。
func gbkField(rec []byte, off, size int) string {
	if off >= len(rec) {
		return ""
	}
	if off+size > len(rec) {
		size = len(rec) - off
	}
	raw := rec[off : off+size]
	if i := indexZero(raw); i >= 0 {
		raw = raw[:i]
	}
	return gbk(string(raw))
}

func indexZero(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

// ── 已解出结构的表 ──────────────────────────────────────────────────────────

// ItemRow 物品: id(+0) + 名字(+4, 24B) + 价格(+82 u32)。ov_item.bin, 12541 行 × 233B。
// 价格经交叉验证: 肉片30/牛肉面105/豆奶50/果酒125, 12253/12541 非零。
type ItemRow struct {
	ID    int32
	Name  string
	Price int64
	Level int32 // 等级/品阶(+76), 官方对照 39/39 = 100%
	Raw   []byte
}

func ItemsOf(t *OvTable) []ItemRow {
	out := make([]ItemRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, ItemRow{ID: i32(r, 0), Name: gbkField(r, 4, 24),
			Price: int64(u32(r, 82)), Level: u16At(r, 76), Raw: r})
	}
	return out
}

// slot 读取 ov_arm 的属性槽。**关键结构**: 装备属性是从 +79 开始的 u16 数组,
// 每个槽位固定对应一种属性。
// 早期误用 u8 读会截断 >255 的值(如涅磐神兵攻击 456 被读成 200)。
func slot(rec []byte, i int) int32 {
	off := 79 + 2*i
	if off+2 > len(rec) {
		return 0
	}
	return int32(binary.LittleEndian.Uint16(rec[off:]))
}

// 装备属性偏移 —— **用官方资料站的 71 件武器逐件对照确认**
// (fo.qq.com/web200604/information/item03.htm, 存档于 research/official/)。
// 校验命中率: 攻击min/命中/力量/敏捷/智慧/防御/魔法攻击/魔法防御 = 100%,
//
//	灵巧 95%, 重量 86%, 攻击max 85%(巨剑类官方页数值略低, 差 2~5)。
//
// 例: 铁剑 攻击1~9 命中5 重量25 力量4 敏捷4 —— 与 +79/+81/+91/+101/+117/+121 完全吻合。
const (
	offAtkMin    = 79  // 最小攻击
	offAtkMax    = 81  // 最大攻击
	offDefense   = 85  // 防御
	offMagicAtk  = 87  // 魔法攻击
	offMagicDef  = 89  // 魔法防御
	offHit       = 91  // 命中
	offWeight    = 101 // 重量(此前误标为"耐久")
	offLevelReq  = 115 // 等级需求
	offStrength  = 117 // 力量要求(17173标注'力量要求', 9/9)
	offVitality  = 119 // 体力要求(24/24)
	offAgility   = 121 // 敏捷要求(9/9)
	offWisdom    = 123 // 智慧要求
	offDexterity = 125 // 灵巧要求
	offSpirit    = 213 // 精神**要求**(17173标注'精神要求', 16/16)
	offMaxLoad   = 205 // 最大负重(2007版官网对照)
	offSlot      = 68  // 装备部位(1面/2头/3项链/4武器/5盾/7戒指/8衣/9靴/10包/13武器类)
	offTier      = 72  // 装备等级(17173 表头'装备等级'列, 49票确认)
)

// u16At 读记录内偏移处的 u16(装备属性一律 u16, 用 u8 读会截断 >255 的值)。
func u16At(rec []byte, off int) int32 {
	if off+2 > len(rec) {
		return 0
	}
	return int32(binary.LittleEndian.Uint16(rec[off:]))
}

// EquipRow 装备: id(+0) + 名字(+4) + 价格(+107 u32) + 等级需求(+115 u8)。
// ov_arm.bin, 9089 行 × 338B。均经交叉验证:
//
//	价格   铁剑500/钢剑4730/辉石剑14840/妖泣101700 (9048/9089 非零)
//	等级需求 铁剑1/钢剑6/辉石剑12/妖泣30 (8692/9089 非零, 96% 落在 1..150)
//
// 攻击/防御尚未解出(非简单标量, 疑似属性对列表), 仍在 Raw 里。
type EquipRow struct {
	ID        int32
	Name      string
	Price     int64
	LevelReq  int32
	Slot      int32 // 装备部位
	Tier      int32 // 装备等级
	AtkMin    int32 // 最小攻击
	AtkMax    int32 // 最大攻击
	Defense   int32 // 防御
	MagicAtk  int32 // 魔法攻击
	MagicDef  int32 // 魔法防御
	Hit       int32 // 命中
	Weight    int32 // 重量
	Strength  int32 // 力量
	Vitality  int32 // 体质
	Agility   int32 // 敏捷
	Wisdom    int32 // 智慧
	Dexterity int32 // 灵巧
	Spirit    int32 // 精神
	MaxLoad   int32 // 最大负重
	Raw       []byte
}

func EquipsOf(t *OvTable) []EquipRow {
	out := make([]EquipRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, EquipRow{
			ID: i32(r, 0), Name: gbkField(r, 4, 24), Price: int64(u32(r, 107)),
			LevelReq: u16At(r, offLevelReq), Slot: int32(u8(r, offSlot)), Tier: int32(u8(r, offTier)),
			AtkMin: u16At(r, offAtkMin), AtkMax: u16At(r, offAtkMax),
			Defense: u16At(r, offDefense), MagicAtk: u16At(r, offMagicAtk),
			MagicDef: u16At(r, offMagicDef), Hit: u16At(r, offHit),
			Weight: u16At(r, offWeight), Strength: u16At(r, offStrength),
			Vitality: u16At(r, offVitality), Agility: u16At(r, offAgility),
			Wisdom: u16At(r, offWisdom), Dexterity: u16At(r, offDexterity),
			Spirit: u16At(r, offSpirit), MaxLoad: u16At(r, offMaxLoad), Raw: r})
	}
	return out
}

// GBKField / I32 导出版本, 供导入器按 namemap 的偏移通用取字段。
func GBKField(rec []byte, off, size int) string { return gbkField(rec, off, size) }
func I32(rec []byte, off int) int32             { return i32(rec, off) }

// MonsterRow 怪物: id(+0) + sprite(+4) + 名字(+8, 20B)。ov_cmon.bin, 2262 行 × 76B。
type MonsterRow struct {
	ID     int32
	Sprite int32
	Name   string
	Raw    []byte
}

func MonstersOf(t *OvTable) []MonsterRow {
	out := make([]MonsterRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, MonsterRow{ID: i32(r, 0), Sprite: i32(r, 4), Name: gbkField(r, 8, 20), Raw: r})
	}
	return out
}

// DescRow 物品/装备描述(ov_desc.bin, 21630 行 × 717B)。
// 结构: id(+0) / 名字(+4,30B) / 资源名(+34,26B ASCII) / 描述(+84, 余下)。
// **描述文本里含真实数值效果**, 是物品效果的权威来源:
//
//	"(小)生命药水" -> "5秒内恢复生命725";  "月饼" -> "攻击力提升到300%, 五分钟"
type DescRow struct {
	ID   int32
	Name string
	Res  string // 资源名(weaponr001)
	Text string // 描述(含数值效果)
}

// descText 取描述文本。字段名义上从 +84 起, 但**前面可能有若干零字节**
// (如"(小)生命药水"的文本实际从 +86 开始) —— 直接按 \0 截断会读成空串。
// 故先跳过前导零, 再读到下一个 \0。
func descText(rec []byte) string {
	i := 84
	for i < len(rec) && rec[i] == 0 {
		i++
	}
	if i >= len(rec) {
		return ""
	}
	return gbkField(rec, i, len(rec)-i)
}

func DescsOf(t *OvTable) []DescRow {
	out := make([]DescRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, DescRow{
			ID: i32(r, 0), Name: gbkField(r, 4, 30),
			Res: gbkField(r, 34, 26), Text: descText(r),
		})
	}
	return out
}

// TaskRow 任务(ov_task.bin, 366 行 × 868B)。
// 结构: id(+0) / 任务名(+12,128B) / 描述(+140,256B) / 发布NPC(+396,32B) / 地图(+428,32B) / 奖励(+460,120B)
type TaskRow struct {
	ID            int32
	Name          string
	Desc          string
	NPC           string
	Map           string
	Reward        string
	LevelMin      int32    // 等级下限(+724)
	LevelMax      int32    // 推荐等级上界(+728)
	Exp           int64    // 经验奖励(+736)
	Prerequisites [5]int32 // 前置任务(+740..+759), 0 = 空槽
	GoalItem      int32    // 目标物品id(+788)
	GoalQty       int32    // 目标数量(+792)
}

func TasksOf(t *OvTable) []TaskRow {
	out := make([]TaskRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, TaskRow{
			ID: i32(r, 0), Name: gbkField(r, 12, 128), Desc: gbkField(r, 140, 256),
			NPC: gbkField(r, 396, 32), Map: gbkField(r, 428, 32), Reward: gbkField(r, 460, 120),
			LevelMin: i32(r, 724), LevelMax: i32(r, 728), Exp: int64(i32(r, 736)),
			Prerequisites: [5]int32{
				i32(r, 740), i32(r, 744), i32(r, 748), i32(r, 752), i32(r, 756),
			},
			GoalItem: i32(r, 788), GoalQty: i32(r, 792),
		})
	}
	return out
}

// RefineRow 精炼配方(ov_refine.bin, 3424 行 × 75B)。
// 结构: 等级(+0) / 费用(+4 u32) / 成功率(+24, %) / 材料1 id(+30)×数量(+32) / 材料2 id(+34)×数量(+36)
// 材料 ID 经验证均命中真实物品(4001=铁矿石, 4160=蝴蝶翅膀)。
type RefineRow struct {
	Level                            int32
	Cost                             int64
	Rate                             int32
	Mat1ID, Mat1Qty, Mat2ID, Mat2Qty int32
}

func RefinesOf(t *OvTable) []RefineRow {
	out := make([]RefineRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, RefineRow{
			Level: int32(u8(r, 0)), Cost: int64(u32(r, 4)), Rate: u16At(r, 24),
			Mat1ID: u16At(r, 30), Mat1Qty: u16At(r, 32),
			Mat2ID: u16At(r, 34), Mat2Qty: u16At(r, 36),
		})
	}
	return out
}

// CombineRow 合成配方(ov_combine.bin, 4782 行 × 75B), 与精炼表同族。
// 产物(+2) / 费用(+4 u32) / 成功率(+24) / 材料1 id(+30)×数量(+32) / 材料2 id(+34)×数量(+36)
// 语义验证: 泳池派对*白(男)+灵魂之证+100000银 -> 真*泳池派对*白(男), 成功率100%
type CombineRow struct {
	Product                          int32
	Cost                             int64
	Rate                             int32
	Mat1ID, Mat1Qty, Mat2ID, Mat2Qty int32
}

func CombinesOf(t *OvTable) []CombineRow {
	out := make([]CombineRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, CombineRow{
			Product: u16At(r, 2), Cost: int64(u32(r, 4)), Rate: u16At(r, 24),
			Mat1ID: u16At(r, 30), Mat1Qty: u16At(r, 32),
			Mat2ID: u16At(r, 34), Mat2Qty: u16At(r, 36),
		})
	}
	return out
}

// PetRow 宠物(ov_petgrow.bin, 504 行 × 260B)。宠物 id 就是怪物 id。
// 结构: id(+0) / 名字(+4,20B) / 捕捉道具id(+38) / 捕捉率(+40,%) / 星级(+42)
// 语义验证: 捕捉道具全命中真实道具(3382捕兽夹/3383玉净瓶/3384捆仙索/3388老君壶)
type PetRow struct {
	ID        int32
	Name      string
	CatchItem int32
	CatchRate int32
	Star      int32
	Raw       []byte
}

func PetsOf(t *OvTable) []PetRow {
	out := make([]PetRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, PetRow{
			ID: i32(r, 0), Name: gbkField(r, 4, 20), CatchItem: u16At(r, 38),
			CatchRate: u16At(r, 40), Star: u16At(r, 42), Raw: r,
		})
	}
	return out
}

// EnchaseRow 镶嵌(ov_enchase.bin, 91 行 × 75B), 与精炼/合成同族。
type EnchaseRow struct {
	Target, Rate                     int32
	Cost                             int64
	CoreItem                         int32
	Mat1ID, Mat1Qty, Mat2ID, Mat2Qty int32
}

func EnchasesOf(t *OvTable) []EnchaseRow {
	out := make([]EnchaseRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, EnchaseRow{
			Target: u16At(r, 0), Cost: int64(u32(r, 4)), CoreItem: u16At(r, 14),
			Rate: u16At(r, 24), Mat1ID: u16At(r, 30), Mat1Qty: u16At(r, 32),
			Mat2ID: u16At(r, 34), Mat2Qty: u16At(r, 36),
		})
	}
	return out
}

// LifeSkillRow 生活技能(ov_lifeskill.bin, 54 行): 采集物id / 技能名 / 动作词。
type LifeSkillRow struct {
	NodeID int32
	Skill  string
	Action string
}

func LifeSkillsOf(t *OvTable) []LifeSkillRow {
	out := make([]LifeSkillRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, LifeSkillRow{ID3(r), gbkField(r, 16, 16), gbkField(r, 32, 16)})
	}
	return out
}

func ID3(r []byte) int32 { return i32(r, 0) }

// TitleRow 称号(ov_title.bin, 40 行 × 516B): 每行 14 个称号, 按职业索引分组。
type TitleRow struct {
	JobIdx int32
	Seq    int32
	Title  string
}

func TitlesOf(t *OvTable) []TitleRow {
	var out []TitleRow
	for _, r := range t.Rows {
		job := i32(r, 0)
		seq := int32(0)
		for off := 4; off+32 <= len(r); off += 32 {
			s := gbkField(r, off, 32)
			if s == "" {
				continue
			}
			out = append(out, TitleRow{JobIdx: job, Seq: seq, Title: s})
			seq++
		}
	}
	return out
}

// LevelRow 等级经验: 12×int32/行, 前三列 [等级, 经验, 累计]。ov_level.bin, 150 行 × 48B。
type LevelRow struct {
	Level    int32
	Exp      int64
	ExpAccum int64
	Raw      []byte
}

func LevelsOf(t *OvTable) []LevelRow {
	out := make([]LevelRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		out = append(out, LevelRow{
			Level: i32(r, 0), Exp: int64(i32(r, 4)), ExpAccum: int64(i32(r, 8)), Raw: r,
		})
	}
	return out
}
