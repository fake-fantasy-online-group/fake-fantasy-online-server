package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

// 上行解码的测试。
//
// 最重要的一条是 TestOnlyProvenOpcodesAreRegistered ——
// 它挡住"为了让功能跑起来随手编一个 opcode"这件事。编出来的号在真客户端上一定是错的，
// 而错的地方会在联调时表现为"服务端收到包但什么都没发生"，极难查。

func TestDecodeLogin(t *testing.T) {
	payload := NewW(0).
		Str("123").Str("321").Str("device-test").Str("1.6.0").
		U8(0).U8(0).Raw(make([]byte, 32)).Bytes()[2:]

	req, ok := Decode(0x1002, payload)
	if !ok {
		t.Fatal("真实抓包样本解不出来")
	}
	if req.Kind != ReqLogin {
		t.Fatalf("Kind = %v", req.Kind)
	}
	if req.S1 != "123" || req.S2 != "321" {
		t.Fatalf("账号/密码 = %q / %q", req.S1, req.S2)
	}
	if req.ClientVersion != "1.6.0" || len(req.ClientAuthTag) != 32 {
		t.Fatalf("版本/auth tag = %q / %d", req.ClientVersion, len(req.ClientAuthTag))
	}
}

// 历史 1.3.4 登录抓包必须被当前 1.5.8-only 服务端拒绝。
func TestLegacyLoginCaptureIsRejected(t *testing.T) {
	path := filepath.Join("..", "..", "..", "research", "capture", "0001-c2s.bin")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("没有抓包样本: %v", err)
	}
	if len(raw) < 2 {
		t.Fatal("样本太短")
	}
	// 前 2 字节是 opcode(小端), 其余是载荷
	op := Op(uint16(raw[0]) | uint16(raw[1])<<8)
	if op != 0x1002 {
		t.Fatalf("样本的 opcode 是 0x%04x, 该是 0x1002", op)
	}
	if _, ok := Decode(op, raw[2:]); ok {
		t.Fatal("1.3.4 登录载荷不应被 1.5.8-only 服务端接受")
	}
}

// 0x1017 是**真正的移动**(客户端 PlayerMove), 载荷就两个小端 i32。
//
// 实证: 角色站在 (9000,5101) 时抓到 `28 23 00 00 ed 13 00 00`。
//
// ⚠️ 以前 0x1017 被当成"场景加载"、0x100a 被当成移动 —— **反了**。
// 代价是每次移动上报都触发一次整图 NPC 重发(约 340 个包/秒)。
func TestPlayerMoveIsOx1017(t *testing.T) {
	req, ok := Decode(0x1017, []byte{0x28, 0x23, 0x00, 0x00, 0xed, 0x13, 0x00, 0x00})
	if !ok || req.Kind != ReqMove {
		t.Fatalf("移动包: ok=%v kind=%v", ok, req.Kind)
	}
	if req.X != 9000 || req.Y != 5101 {
		t.Fatalf("坐标 = (%d,%d), 该是实测的 (9000,5101)", req.X, req.Y)
	}
	// 截断的要失败, 不能把人瞬移到 (0,0)
	for n := 0; n < 8; n++ {
		if _, ok := Decode(0x1017, make([]byte, n)); ok {
			t.Fatalf("%d 字节的移动包不该解成功", n)
		}
	}
	// 0x100a 是 SavePos, 不是移动 —— 两者别再搞混
	if req, _ := Decode(0x100a, []byte{1, 0, 0, 0, 0x40, 0x1f, 0, 0, 0x88, 0x13, 0, 0, 0}); req.Kind != ReqSavePos {
		t.Fatalf("0x100a 的 Kind = %v, 该是 SavePos", req.Kind)
	}
}

func TestDecodeMove(t *testing.T) {
	// i32 模式 + i32 X + i32 Y + u8 标志
	payload := []byte{
		0x01, 0x00, 0x00, 0x00,
		0x40, 0x1f, 0x00, 0x00, // 8000
		0x88, 0x13, 0x00, 0x00, // 5000
		0x00,
	}
	req, ok := Decode(0x100a, payload)
	if !ok {
		t.Fatal("移动包解不出来")
	}
	if req.X != 8000 || req.Y != 5000 {
		t.Fatalf("坐标 = (%d,%d), 该是 (8000,5000)", req.X, req.Y)
	}
}

// 截断的包必须解失败，而不是解出一个零值请求。
// 解出零值的话玩家会被瞬移到 (0,0)。
func TestTruncatedPayloadFailsInsteadOfZeroing(t *testing.T) {
	for n := 0; n < 12; n++ {
		payload := make([]byte, n)
		if _, ok := Decode(0x100a, payload); ok {
			t.Fatalf("%d 字节的移动包不该解成功 —— 会把人瞬移到 (0,0)", n)
		}
	}
	// 字符串长度前缀撒谎: 说有 100 字节, 实际没那么多
	bad := []byte{0x64, 0x00, 'a', 'b'}
	if _, ok := Decode(0x1002, bad); ok {
		t.Fatal("长度前缀越界的登录包不该解成功")
	}
}

// 这条防止服务端注册一个协议资产里不存在的 opcode。
func TestOnlyProvenOpcodesAreRegistered(t *testing.T) {
	// 这些号的写入顺序均来自 docs/protocol/c2s_map.json；0x1013 是
	// UseSkill 与 UseSkillAt 共享的已验证五字段包；0x1015 是 UseItem 的普通物品
	// 分支；0x1006 是 ChatUI 斜杠命令写出的单 Str；0x100b 是 11 个客户端方法
	// 共享的宠物子命令包，首字节是子命令号；0x1034 放弃任务、0x1009 加点、0x1012 升技能、
	// 0x1018 复活、0x1076 上报已展示的新手提示编号、0x1077 帮助、0x1011 坐下；
	// 0x1019..0x101b 是已从正式客户端写入闭包闭合并实机贯通的 NPC 开店/买入/卖出；
	// 0x100d/0x1078 是 ItemMove/SortBag 写入闭包闭合的背包位置链路；
	// 0x101d/0x101e 是 Repair/RepairConfirm 写入闭包与 RepairDlg 参数闭合的修理链路；
	// 0x1040..0x1044 是源码镜像已闭合写入顺序的完整玩家交易握手；
	// 0x1080/0x1088/0x1089 是物品锁、修改密码与修改安全码的精确闭包字段序列；
	// 0x102c..0x102e/0x1030/0x1031/0x1087 是用户确认规则后的个人仓库链路。
	const provenCount = 102

	if got := len(decoders); got != provenCount {
		t.Fatalf("注册表里有 %d 个 opcode, 该是 %d。\n"+
			"往里加号之前先回答: 这个号是抓包抓到的, 还是猜的?\n"+
			"猜的号在真客户端上一定错, 而错的表现是'收到包但什么都没发生', 极难查。",
			got, provenCount)
	}
	for op, e := range decoders {
		if !e.Proven {
			t.Fatalf("0x%04x 标着未实证却进了注册表", uint16(op))
		}
	}
	for _, op := range []Op{0x1001, 0x1002, 0x1003, 0x1004, 0x1006, 0x100a, 0x100d, 0x100f,
		0x1015, 0x1016, 0x1017,
		0x1019, 0x101a, 0x101b, 0x101d, 0x101e,
		0x1040, 0x1041, 0x1042, 0x1043, 0x1044, 0x1049, 0x105d, 0x1071, 0x1072,
		0x1022, 0x1023, 0x1024, 0x1025, 0x1026, 0x1029, 0x102a,
		0x102c, 0x102d, 0x102e, 0x1030, 0x1031,
		0x1053, 0x1054, 0x1055, 0x1056, 0x1057, 0x105b, 0x105c, 0x105e, 0x1073, 0x1075, 0x1076, 0x1078, 0x1079, 0x107c, 0x107d,
		0x1060, 0x1061, 0x1062, 0x1063, 0x1064, 0x1065, 0x1066,
		0x1080, 0x1087, 0x1088, 0x1089, 0x1027, 0x1028} {
		if !Registered(op) {
			t.Fatalf("0x%04x 是实证过的, 不该缺", uint16(op))
		}
	}
}

// LeaveWorld 是客户端从游戏内回选角页的正式入口。客户端发送闭包已证明它是
// 0x1049 空载荷；若漏接，TCP 仍连着时旧场景实体永远收不到 Leave。
func TestLeaveWorldIsEmptyOx1049(t *testing.T) {
	req, ok := Decode(0x1049, nil)
	if !ok || req.Kind != ReqLeaveWorld {
		t.Fatalf("LeaveWorld: ok=%v kind=%v", ok, req.Kind)
	}
	if _, ok := Decode(0x1049, []byte{0}); ok {
		t.Fatal("LeaveWorld 多一个字节仍解成功，空包边界失效")
	}
}

// 心跳载荷里的 I64 必须完整解出来并原样回显，客户端拿它算延迟。
func TestPingCarriesClientTimestamp(t *testing.T) {
	// NetClient.Ping(Int64 t) 的发送闭包只写一个 I64。
	req, ok := Decode(0x1001, []byte{0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0})
	if !ok || req.Kind != ReqPing {
		t.Fatalf("心跳没解出来: ok=%v kind=%v", ok, req.Kind)
	}
	if req.TickMS != 0x12345678 {
		t.Fatalf("时间戳 = %#x, 该是 0x12345678", req.TickMS)
	}
	for _, short := range [][]byte{nil, {1}, {1, 2, 3}} {
		if _, ok := Decode(0x1001, short); ok {
			t.Fatalf("%d 字节不是完整 I64，不该伪造一个回显值", len(short))
		}
	}
}

// 任务的号与载荷是从客户端**闭包**里读出来的(见 docs/protocol/上行包定义.md):
// TaskAccept/TaskComplete 各是一个 I32 任务号, 与它们的 C# 签名 (int) 一致。
func TestQuestOpcodes(t *testing.T) {
	for _, c := range []struct {
		op   Op
		kind ReqKind
		name string
	}{
		{0x1033, ReqAcceptQuest, "TaskAccept"},
		{0x1035, ReqSubmitQuest, "TaskComplete"},
	} {
		req, ok := Decode(c.op, []byte{0x39, 0x05, 0x00, 0x00}) // 任务 1337
		if !ok || req.Kind != c.kind {
			t.Fatalf("%s: ok=%v kind=%v", c.name, ok, req.Kind)
		}
		if req.ID32 != 1337 {
			t.Fatalf("%s 任务号 = %d, 该是 1337", c.name, req.ID32)
		}
		// 截断的要失败, 不能解成 0 号任务
		if _, ok := Decode(c.op, []byte{1, 2}); ok {
			t.Fatalf("%s 载荷只有 2 字节不该解成功", c.name)
		}
	}
}

// 0x1032 TaskNpcList 载荷是一个字符串(NPC 名)。
//
// 实证: 站在「主持人王小丫」旁边时客户端发的是 `12 00` + 那 6 个汉字的 UTF-8。
func TestNpcTaskListOpcode(t *testing.T) {
	name := "主持人王小丫"
	pay := append([]byte{byte(len(name)), byte(len(name) >> 8)}, name...)
	req, ok := Decode(0x1032, pay)
	if !ok || req.Kind != ReqNpcTasks {
		t.Fatalf("ok=%v kind=%v", ok, req.Kind)
	}
	if req.S1 != name {
		t.Fatalf("NPC 名 = %q, 该是 %q", req.S1, name)
	}
	// 长度前缀撒谎的要失败, 不能解出半截名字
	if _, ok := Decode(0x1032, []byte{0x64, 0x00, 'a'}); ok {
		t.Fatal("长度越界的包不该解成功")
	}
}

// 服务端已经实现、但还等着 opcode 的那些请求，Kind 必须都定义好 ——
// 定义好了，抓到号的那天就只是加一行表。
func TestPendingKindsAreDefined(t *testing.T) {
	pending := []ReqKind{
		ReqEnterDungeon,
	}
	for _, k := range pending {
		if k.String() == "" || k == ReqUnknown {
			t.Fatalf("请求种类 %d 没定义名字", uint8(k))
		}
		// 还没有 opcode: 注册表里不该出现
		for op, e := range decoders {
			if e.Kind == k {
				t.Fatalf("%v 居然已经注册在 0x%04x 上 —— 那个号是哪来的?", k, uint16(op))
			}
		}
	}
}

func TestUnknownOpcodeIsRecordedNotDropped(t *testing.T) {
	u := NewUnknownLog()

	if !u.Note(0x1234, []byte{1, 2, 3, 4}) {
		t.Fatal("第一次见到该返回 true")
	}
	if u.Note(0x1234, []byte{1, 2, 3, 4, 5, 6}) {
		t.Fatal("第二次见到该返回 false —— 否则日志会被刷爆")
	}
	u.Note(0x1234, []byte{1})
	u.Note(0x0099, nil)

	if u.Len() != 2 {
		t.Fatalf("见了 %d 种未知 opcode, 该是 2", u.Len())
	}
	snap := u.Snapshot()
	// 按 opcode 升序
	if snap[0].Op != 0x0099 || snap[1].Op != 0x1234 {
		t.Fatalf("没按 opcode 排序: %v", snap)
	}
	st := snap[1]
	if st.Count != 3 {
		t.Fatalf("0x1234 计了 %d 次, 该是 3", st.Count)
	}
	// 长度区间是判断包结构的主要线索, 必须准
	if st.MinLen != 1 || st.MaxLen != 6 {
		t.Fatalf("长度区间 %d~%d, 该是 1~6", st.MinLen, st.MaxLen)
	}
	if len(st.Sample) != 4 {
		t.Fatalf("样本留了 %d 字节, 该是第一次见到的那 4 个", len(st.Sample))
	}
}

// 样本要截断，不能把一个大包整个抄进内存。
func TestUnknownSampleIsTruncated(t *testing.T) {
	u := NewUnknownLog()
	big := make([]byte, 4096)
	u.Note(0x4321, big)
	if got := len(u.Snapshot()[0].Sample); got != sampleBytes {
		t.Fatalf("样本留了 %d 字节, 该截到 %d", got, sampleBytes)
	}
}

// 样本必须是拷贝。留原切片的话，网关复用读缓冲时样本会被后来的包覆盖。
func TestUnknownSampleIsACopy(t *testing.T) {
	u := NewUnknownLog()
	buf := []byte{0xaa, 0xbb, 0xcc}
	u.Note(0x5555, buf)
	buf[0], buf[1], buf[2] = 0, 0, 0 // 网关把缓冲拿去装下一个包了

	if got := u.Snapshot()[0].Sample; got[0] != 0xaa {
		t.Fatalf("样本被覆盖成了 % x —— 存的是引用不是拷贝", got)
	}
}

// 打工那两个号是 2026-08-13 用 tools/rig 驱动客户端采到的，
// 两轮不同顺序复跑都跟着方法走（见 docs/架构/30-客户端联调.md）。
func TestWorkOpcodesDecode(t *testing.T) {
	// BeginWork(0x11223344) 实测发出 44 33 22 11 —— 小端 i32
	req, ok := Decode(0x1027, []byte{0x44, 0x33, 0x22, 0x11})
	if !ok {
		t.Fatal("开工包解不出来")
	}
	if req.Kind != ReqStartWork {
		t.Fatalf("Kind = %v", req.Kind)
	}
	if req.ID32 != 0x11223344 {
		t.Fatalf("工种号 = %#x, 该是 0x11223344(小端读)", req.ID32)
	}
	// 截断的要失败, 不能解成 0 号工种
	if _, ok := Decode(0x1027, []byte{1, 2}); ok {
		t.Fatal("2 字节的开工包不该解成功 —— 会变成开 0 号工种")
	}

	if req, ok := Decode(0x1028, nil); !ok || req.Kind != ReqStopWork {
		t.Fatalf("收工包: ok=%v kind=%v", ok, req.Kind)
	}
}

// 已实证但还没接的那些号，不该混进 decoders —— 但也不该丢掉。
func TestProvenUnwiredIsRecordedButNotRegistered(t *testing.T) {
	// 0x100b 已在 2026-08-14 接线(放出/收回), 从这张表里搬走了。
	if len(ProvenUnwired) != 2 {
		t.Fatalf("已实证未接的号有 %d 个, 该是 2", len(ProvenUnwired))
	}
	for op, info := range ProvenUnwired {
		if Registered(op) {
			t.Fatalf("0x%04x(%s) 既在 ProvenUnwired 又在 decoders 里, 两边重了",
				uint16(op), info.Method)
		}
		if info.Method == "" {
			t.Fatalf("0x%04x 没记是哪个方法发的 —— 那这条记录就没用了", uint16(op))
		}
	}
}

// 这批号的载荷布局是**自证**的：调方法时传一个好认的值，
// 那个值原样出现在样本字节里。换顺序换值复跑过一次，结论一致。
func TestGameplayOpcodesDecodePayloads(t *testing.T) {
	// Attack(0x22334455) 实测发出 55 44 33 22
	for _, c := range []struct {
		op   Op
		kind ReqKind
		name string
	}{
		{0x100c, ReqAttack, "Attack"},
		{0x1081, ReqQueryTargetStatus, "QueryTargetStatus"},
		{0x1010, ReqPickUp, "PickItem"},
		{0x1007, ReqEquip, "EquipItem"},
		{0x1008, ReqUnequip, "UnequipItem"},
	} {
		req, ok := Decode(c.op, []byte{0x55, 0x44, 0x33, 0x22})
		if !ok {
			t.Fatalf("%s 包解不出来", c.name)
		}
		if req.Kind != c.kind {
			t.Fatalf("%s 的 Kind = %v", c.name, req.Kind)
		}
		if req.TargetID != 0x22334455 || req.ID32 != 0x22334455 {
			t.Fatalf("%s 解出 %#x/%#x, 该是 0x22334455(小端读)",
				c.name, req.TargetID, req.ID32)
		}
		// 截断的要失败 —— 解成 0 的话就是"攻击 0 号实体"
		if _, ok := Decode(c.op, []byte{1, 2}); ok {
			t.Fatalf("%s: 2 字节不该解成功", c.name)
		}
	}

	// Capture(0x0A0B0C0D, 0x0E0F1011) 实测发出 0d 0c 0b 0a 11 10 0f 0e
	req, ok := Decode(0x102b, []byte{0x0d, 0x0c, 0x0b, 0x0a, 0x11, 0x10, 0x0f, 0x0e})
	if !ok {
		t.Fatal("捕捉包解不出来")
	}
	if req.Kind != ReqCapturePet {
		t.Fatalf("Kind = %v", req.Kind)
	}
	if req.TargetID != 0x0A0B0C0D {
		t.Fatalf("目标 = %#x, 该是 0x0A0B0C0D", req.TargetID)
	}
	if req.ID32 != 0x0E0F1011 {
		t.Fatalf("道具 = %#x, 该是 0x0E0F1011", req.ID32)
	}
	// 只有 4 字节时不能把道具解成 0
	if _, ok := Decode(0x102b, []byte{1, 2, 3, 4}); ok {
		t.Fatal("4 字节的捕捉包不该解成功 —— 道具会变成 0")
	}
}

func TestQueryTargetStatusStrictI32(t *testing.T) {
	req, ok := Decode(0x1081, []byte{0x49, 0x00, 0x00, 0x08})
	if !ok || req.Kind != ReqQueryTargetStatus || req.TargetID != 0x08000049 {
		t.Fatalf("0x1081 解码不符: ok=%v req=%+v", ok, req)
	}
	for _, bad := range [][]byte{{1, 2, 3}, {1, 2, 3, 4, 5}} {
		if _, ok := Decode(0x1081, bad); ok {
			t.Fatalf("0x1081 必须严格 4 字节, %d 字节不该成功", len(bad))
		}
	}
}

// 0x100b 是**子命令包**: 一个 opcode 底下挂着 11 个客户端方法, 首字节才是动作。
// 子命令号来自最终 C2S 资产的 delegatingMethods(sub=2..12), 不是猜的。
func TestPetCmdSubCommands(t *testing.T) {
	pkt := func(sub byte, arg int32, text string) []byte {
		p := []byte{sub,
			byte(arg), byte(arg >> 8), byte(arg >> 16), byte(arg >> 24),
			byte(len(text)), byte(len(text) >> 8)}
		return append(p, text...)
	}
	for _, c := range []struct {
		name string
		sub  byte
		arg  int32
		want ReqKind
	}{
		{"放出第 2 只", petSubDeploy, 2, ReqSummonPet},
		{"收回", petSubRecall, 0, ReqRecallPet},
		{"关掉显示 = 收回", petSubShow, 0, ReqRecallPet},
		// 打开显示没接: 服务端没有"当前出战槽"这个状态, 放出必须由 PetDeploy 指定
		{"打开显示", petSubShow, 1, ReqPetOther},
		{"加点书(MVP 不做)", petSubAddPoint, 3, ReqPetOther},
		{"觉醒(MVP 不做)", petSubAwaken, 0, ReqPetOther},
	} {
		req, ok := Decode(0x100b, pkt(c.sub, c.arg, ""))
		if !ok {
			t.Fatalf("%s: 解码失败", c.name)
		}
		if req.Kind != c.want {
			t.Fatalf("%s: kind = %v, 该是 %v", c.name, req.Kind, c.want)
		}
		if req.Slot != c.arg || req.Flag != c.sub {
			t.Fatalf("%s: 子命令/参数丢了: sub=%d arg=%d", c.name, req.Flag, req.Slot)
		}
	}
	// 带文本的(改名)也要解得出来, 哪怕现在没接
	if req, ok := Decode(0x100b, pkt(petSubRename, 0, "夹子")); !ok || req.S1 != "夹子" {
		t.Fatalf("改名: ok=%v text=%q", ok, req.S1)
	}
	// 截断的包不能解成"放出 0 号槽"
	if _, ok := Decode(0x100b, []byte{petSubDeploy, 1, 0}); ok {
		t.Fatal("截断的宠物子命令包不该解成功")
	}
}
