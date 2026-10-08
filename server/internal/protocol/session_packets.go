package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// 登录、选服与进世界主链上的下行 opcode。
const (
	SCHeartbeatReply   = 0x8001
	SCLoginResult      = 0x8002
	SCCharacterList    = 0x8004
	SCCreateResult     = 0x8005
	SCRegisterResult   = 0x8009
	SCPartySnapshot    = 0x8033
	SCTradeOpen        = 0x8030
	SCTradeState       = 0x8031
	SCTradeEnd         = 0x8032
	SCSystemRequests   = 0x8035
	SCPrivateChat      = 0x8036
	SCFriendList       = 0x8037
	SCFamilySnapshot   = 0x8034
	SCFamilyBrowse     = 0x8040
	SCFamilyPositions  = 0x8041
	SCApprentice       = 0x805a
	SCDeposit          = 0x8042
	SCOnlineCount      = 0x805b
	SCResetResult      = 0x804f
	SCRenameNotice     = 0x8056
	SCRenameResult     = 0x8057
	SCAccountSecSeed   = 0x8065
	SCAccountSecResult = 0x8066
	SCHelpTopics       = 0x8055
	SCBlockList        = 0x8068
)

// LoginResult 编码 0x8002：I32 结果码 + Str 消息。
func LoginResult(code int32, message string) []byte {
	return NewW(SCLoginResult).I32(code).Str(message).Bytes()
}

type BlockView struct {
	CharID int64
	Name   string
	Scope  uint8
}

// BlockList 编码 1.5.8 的 0x8068。主段是 U16 计数后的
// (I64 charId, Str name) 列表；共用 ReadBlockedTail 再读一段
// U16 计数 + U8[]，按原顺序把 scope 补到 SocialClient.Blocks 的第三项。
func BlockList(blocks []BlockView) []byte {
	if len(blocks) > int(^uint16(0)) {
		return nil
	}
	for _, block := range blocks {
		if block.CharID <= 0 || block.Scope > 1 || !wireString(block.Name) {
			return nil
		}
	}
	w := NewW(SCBlockList).U16(uint16(len(blocks)))
	for _, block := range blocks {
		w.I64(block.CharID).Str(block.Name)
	}
	// 1.5.8 始终写出完整 scope 尾，不依赖客户端的缺省 false。
	w.U16(uint16(len(blocks)))
	for _, block := range blocks {
		w.U8(block.Scope)
	}
	return w.Bytes()
}

// RegisterResult 编码 0x8009：I32 结果码 + Str 消息。
// TitleFlow 在 code==0 时结束注册流程，非零时保留表单并显示消息。
func RegisterResult(code int32, message string) []byte {
	return NewW(SCRegisterResult).I32(code).Str(message).Bytes()
}

// CreateCharacterResult 编码 0x8005：I32 结果码 + Str 消息。
// 客户端只区分 code==0（成功并返回选角页）与非零（停留并显示消息）。
func CreateCharacterResult(code int32, message string) []byte {
	return NewW(SCCreateResult).I32(code).Str(message).Bytes()
}

// HeartbeatReply 编码 0x8001。真实流量的 28 个样本均为两个 I64：
// 客户端 tick 原样回显，以及服务端 Unix 毫秒时间。
func HeartbeatReply(clientTickMs, serverUnixMs int64) []byte {
	return NewW(SCHeartbeatReply).I64(clientTickMs).I64(serverUnixMs).Bytes()
}

// OnlineCount 编码 0x805b：当前在线人数 U16。
func OnlineCount(count uint16) []byte {
	return NewW(SCOnlineCount).U16(count).Bytes()
}

func ResetPasswordResult(code int32, message string) []byte {
	return NewW(SCResetResult).I32(code).Str(message).Bytes()
}

func RenameCharacterResult(code int32, message, newName string) []byte {
	return NewW(SCRenameResult).I32(code).Str(message).Str(newName).Bytes()
}

// AccountSecuritySeed 编码 0x8065：是否已有安全码 + 客户端免重复输入窗口毫秒数。
// 原服窗口常量尚未确认，因此生产明确下发 0：每次解锁都校验安全码，
// 不凭空引入一个未经证明的宽限期。
func AccountSecuritySeed(hasSecurity bool, windowMS int32) []byte {
	if windowMS < 0 {
		return nil
	}
	return NewW(SCAccountSecSeed).U8(b2u8(hasSecurity)).I32(windowMS).Bytes()
}

// AccountSecurityResult 编码 0x8066。kind 1/2/3/4 分别对应现有客户端分支中的
// 锁定、解锁、修改密码、修改安全码；code=0 成功，其余为失败。
func AccountSecurityResult(kind, code uint8, message string, tab uint8, slot int32, item domain.ItemID) []byte {
	if kind < 1 || kind > 4 || !wireString(message) {
		return nil
	}
	return NewW(SCAccountSecResult).U8(kind).U8(code).Str(message).
		U8(tab).I32(slot).I32(int32(item)).Bytes()
}

func HelpTopics(topics []domain.HelpTopic) []byte {
	if len(topics) > 255 {
		return nil
	}
	for _, topic := range topics {
		if !wireString(topic.Title) || !wireString(topic.Content) {
			return nil
		}
	}
	w := NewW(SCHelpTopics).U8(uint8(len(topics)))
	for _, topic := range topics {
		w.Str(topic.Title).Str(topic.Content)
	}
	return w.Bytes()
}

// ApView 是 0x8003 与 0x8004 共用的外观及攻击表现段。
type ApView struct {
	Body        uint16
	Cap         uint16
	Backpack    uint16
	WeaponR     uint16
	WeaponL     uint16
	Face        uint16
	AtkVariant  uint8
	WeaponCType uint8
	AtkDist     uint16
}

func (a ApView) appendTo(w *W) {
	w.U16(a.Body).U16(a.Cap).U16(a.Backpack).
		U16(a.WeaponR).U16(a.WeaponL).U16(a.Face).
		U8(a.AtkVariant).U8(a.WeaponCType).U16(a.AtkDist)
}

// ApViewOf 从领域角色提取客户端使用的六个外观槽。
func ApViewOf(c *domain.Character) ApView {
	v := c.Appear.EquipView
	return ApView{
		Body: v[0], Cap: v[1], Backpack: v[2],
		WeaponR: v[3], WeaponL: v[4], Face: v[5],
		AtkVariant: c.Appear.AtkVariant, WeaponCType: c.Appear.WeaponCType,
		AtkDist: c.Appear.AtkDist,
	}
}

// CharBrief 是 0x8004 的单个角色记录。
type CharBrief struct {
	Name      string
	Race      uint8
	Gender    uint8
	Head      uint8
	Hair      uint8
	Level     int32
	MapID     int32
	X, Y      float32
	LastLogin string
	Appear    ApView
	Attrs     []int32
	Honor     int64
}

func (c *CharBrief) appendTo(w *W) {
	w.Str(c.Name).
		U8(c.Race).U8(c.Gender).U8(c.Head).U8(c.Hair).
		I32(c.Level).I32(c.MapID).F32(c.X).F32(c.Y).Str(c.LastLogin)
	c.Appear.appendTo(w)
	w.U16(uint16(len(c.Attrs)))
	for _, value := range c.Attrs {
		w.I32(value)
	}
	w.I64(c.Honor)
}

// CharacterList 编码 0x8004。
func CharacterList(chars []CharBrief) []byte {
	if len(chars) > 1<<8-1 {
		return nil
	}
	for i := range chars {
		if len(chars[i].Attrs) > 1<<16-1 {
			return nil
		}
	}
	w := NewW(SCCharacterList).U8(uint8(len(chars)))
	for i := range chars {
		chars[i].appendTo(w)
	}
	return w.Bytes()
}

// EmptyPartySnapshot 编码 0x8033 的空队伍快照。这个包不是“进场确认”；
// 真实 7 字节载荷恰好是 leaderId=0、partyName=""、memberCount=0。
func EmptyPartySnapshot() []byte {
	return NewW(SCPartySnapshot).I32(0).Str("").U8(0).Bytes()
}

// TradeOpen encodes 0x8030: I32 otherEntityId + Str otherName. The client resets
// SocialClient's trade state before opening its native DealDlg.
func TradeOpen(other domain.EntityID, otherName string) []byte {
	return NewW(SCTradeOpen).I32(int32(other)).Str(otherName).Bytes()
}

type TradeItemView struct {
	Pet     *domain.PetItemInfo
	ID      domain.ItemID
	Count   int32
	Name    string
	Info    string
	Quality uint8
}

type TradeStateView struct {
	MyLocked, OtherLocked       bool
	MyConfirmed, OtherConfirmed bool
	MyMoney, OtherMoney         int64
	MyItems, OtherItems         []TradeItemView
}

// TradeState encodes the complete byte- and semantics-exact 0x8031 payload,
// including the two item-info tails used by the native tooltip and quality color.
func TradeState(v TradeStateView) []byte {
	if len(v.MyItems) > 255 || len(v.OtherItems) > 255 {
		return nil
	}
	w := NewW(SCTradeState).
		U8(b2u8(v.MyLocked)).U8(b2u8(v.OtherLocked)).
		U8(b2u8(v.MyConfirmed)).U8(b2u8(v.OtherConfirmed)).
		I64(v.MyMoney).I64(v.OtherMoney).U8(uint8(len(v.MyItems)))
	for _, item := range v.MyItems {
		w.I32(int32(item.ID)).I32(item.Count).Str(item.Name)
	}
	w.U8(uint8(len(v.OtherItems)))
	for _, item := range v.OtherItems {
		w.I32(int32(item.ID)).I32(item.Count).Str(item.Name)
	}
	w.U8(uint8(len(v.MyItems)))
	for _, item := range v.MyItems {
		w.Str(item.Info).U8(item.Quality)
	}
	w.U8(uint8(len(v.OtherItems)))
	for _, item := range v.OtherItems {
		w.Str(item.Info).U8(item.Quality)
	}
	if !appendTradePets(w, v.MyItems) || !appendTradePets(w, v.OtherItems) {
		return nil
	}
	appendTradePetPP(w, v.MyItems)
	appendTradePetPP(w, v.OtherItems)
	return w.Bytes()
}

// TradeEnd encodes 0x8032. The client resets trade state, hides DealDlg, and
// appends text to ChatUI using color as the channel/style selector.
func TradeEnd(color uint8, text string) []byte {
	return NewW(SCTradeEnd).U8(color).Str(text).Bytes()
}

type PartyView struct {
	Leader  domain.EntityID
	Name    string
	Members []PartyMemberView
}

type PartyMemberView struct {
	Entity    domain.EntityID
	Char      domain.CharID
	Name      string
	Level     int32
	HP        int32
	MaxHP     int32
	Gender    uint8
	HairID    int32
	HairColor int32
	Online    bool
}

// PartySnapshot 编码 0x8033。两个兼容尾都是按原 memberCount 再遍历一遍，
// 不能夹在成员行内，否则客户端会把 online 字节当成下一位成员的 entityId。
func PartySnapshot(p PartyView) []byte {
	if len(p.Members) > 255 {
		return nil
	}
	w := NewW(SCPartySnapshot).I32(int32(p.Leader)).Str(p.Name).U8(uint8(len(p.Members)))
	for _, m := range p.Members {
		w.I32(int32(m.Entity)).Str(m.Name).I32(m.Level).I32(m.HP).I32(m.MaxHP).
			U8(m.Gender).I32(m.HairID).I32(m.HairColor)
	}
	for _, m := range p.Members {
		w.U8(b2u8(m.Online))
	}
	for _, m := range p.Members {
		w.I64(int64(m.Char))
	}
	return w.Bytes()
}

// SystemRequestView 与客户端 SystemInfoBar.Req 的线字段逐项对应。
type SystemRequestView struct {
	ID       int32
	Kind     uint8
	From     domain.EntityID
	FromName string
	Text     string
}

// SystemRequests 编码 0x8035：U8 count + count ×
// (I32 requestId, U8 kind, I32 fromEntityId, Str fromName, Str text)。
func SystemRequests(requests []SystemRequestView) []byte {
	if len(requests) > 255 {
		return nil
	}
	w := NewW(SCSystemRequests).U8(uint8(len(requests)))
	for _, req := range requests {
		w.I32(req.ID).U8(req.Kind).I32(int32(req.From)).Str(req.FromName).Str(req.Text)
	}
	return w.Bytes()
}

type FriendView struct {
	Name   string
	Online bool
}

// FriendList 编码 0x8037：U8 count + count × (Str name, U8 online)。
func FriendList(friends []FriendView) []byte {
	if len(friends) > 255 {
		return nil
	}
	w := NewW(SCFriendList).U8(uint8(len(friends)))
	for _, friend := range friends {
		w.Str(friend.Name).U8(b2u8(friend.Online))
	}
	return w.Bytes()
}

type FamilyMemberView struct {
	Name, Position, Contribution string
	Level                        int32
	Online                       bool
}

type FamilyView struct {
	Name                         string
	Level                        uint8
	MemberCap                    uint16
	Proclaim                     string
	Resist                       bool
	MyPosition                   uint8
	CanInvite, CanMail, OpenSelf bool
	Members                      []FamilyMemberView
}

// FamilySnapshot 编码 0x8034。成员行 wire 类型序列为
// (Str,I32,Str,Str,U8)，对应客户端 ValueTuple<String,Int32,String,String,Boolean>。
// 证据报告（s2c_direct_candidates_20260814.md）明确告诫 Item1..5 保持位置语义，
// 不得无证据赋予姓名/等级/职位等业务名；以下字段名仅为服务端领域模型内部标识，
// 不代表客户端消费语义已确认。
func FamilySnapshot(f FamilyView) []byte {
	if len(f.Members) > 255+65535 {
		return nil
	}
	firstCount := len(f.Members)
	if firstCount > 255 {
		firstCount = 255
	}
	legacyCap := f.MemberCap
	if legacyCap > 255 {
		legacyCap = 255
	}
	w := NewW(SCFamilySnapshot).Str(f.Name).U8(f.Level).U8(uint8(legacyCap)).Str(f.Proclaim).
		U8(b2u8(f.Resist)).U8(f.MyPosition).U8(b2u8(f.CanInvite)).U8(b2u8(f.CanMail)).
		U8(b2u8(f.OpenSelf)).U8(uint8(firstCount))
	writeMember := func(member FamilyMemberView) {
		w.Str(member.Name).I32(member.Level).Str(member.Position).Str(member.Contribution).
			U8(b2u8(member.Online))
	}
	for _, member := range f.Members[:firstCount] {
		writeMember(member)
	}
	w.U16(f.MemberCap)
	if firstCount < len(f.Members) {
		w.U16(uint16(len(f.Members) - firstCount))
		for _, member := range f.Members[firstCount:] {
			writeMember(member)
		}
	}
	return w.Bytes()
}

type FamilyBrowseView struct {
	Name, Text        string
	Level             uint8
	Members, Capacity uint8
}

func FamilyBrowse(entries []FamilyBrowseView) []byte {
	if len(entries) > 255 {
		return nil
	}
	w := NewW(SCFamilyBrowse).U8(uint8(len(entries)))
	for _, entry := range entries {
		w.Str(entry.Name).U8(entry.Level).U8(entry.Members).U8(entry.Capacity).Str(entry.Text)
	}
	return w.Bytes()
}

func FamilyPositions(positions []domain.FamilyPosition) []byte {
	if len(positions) > 255 {
		return nil
	}
	w := NewW(SCFamilyPositions).U8(uint8(len(positions)))
	for _, position := range positions {
		w.U8(uint8(position.ID)).Str(position.Name).Str(position.Description)
	}
	return w.Bytes()
}

type SocialPersonView struct {
	Name   string
	Level  int32
	Online bool
}

type ApprenticeView struct {
	Master      SocialPersonView
	Peers       []SocialPersonView
	Apprentices []SocialPersonView
}

func ApprenticeSnapshot(v ApprenticeView) []byte {
	if len(v.Peers) > 255 || len(v.Apprentices) > 255 {
		return nil
	}
	w := NewW(SCApprentice).Str(v.Master.Name).I32(v.Master.Level).U8(b2u8(v.Master.Online)).
		U8(uint8(len(v.Peers)))
	for _, peer := range v.Peers {
		w.Str(peer.Name).I32(peer.Level).U8(b2u8(peer.Online))
	}
	w.U8(uint8(len(v.Apprentices)))
	for _, apprentice := range v.Apprentices {
		w.Str(apprentice.Name).I32(apprentice.Level).U8(b2u8(apprentice.Online))
	}
	return w.Bytes()
}

func DepositResult(ok bool, chain, address string, amount int32, caiyu int64, warning, qrB64 string) []byte {
	return NewW(SCDeposit).U8(b2u8(ok)).Str(chain).Str(address).I32(amount).I64(caiyu).
		Str(warning).Str(qrB64).Bytes()
}

// PrivateChat 编码无分享附件的 0x8036。分享段是可选尾；省略它与客户端读取器
// 的 remaining>=1 判断一致，不伪造一个不存在的物品分享。
func PrivateChat(from, text string, shares ...ChatShareView) []byte {
	w := NewW(SCPrivateChat).Str(from).Str(text)
	if len(shares) > 0 && !appendShares(w, shares) {
		return nil
	}
	return w.Bytes()
}

// EnterWorld 编码 0x8003。所有基础字段及五段兼容尾均按客户端定义写出。
func EnterWorld(entityID domain.EntityID, c *domain.Character) []byte {
	if c == nil || len(c.Attrs) > 1<<16-1 {
		return nil
	}
	w := NewW(SCSelfData).
		I32(int32(entityID)).
		U8(1). // ignoredCompatByte；真实样本为 1，客户端读取后丢弃。
		Str(c.Name).
		I32(c.Pos.MapID).
		F32(float32(c.Pos.X)).F32(float32(c.Pos.Y)).
		U8(c.ClientRace()).U8(c.Appear.Gender).U8(c.Appear.Head).U8(c.Appear.Hair).
		U16(uint16(len(c.Attrs)))
	for _, value := range c.Attrs {
		w.I32(value)
	}
	w.Str(c.Appear.Title)
	ApViewOf(c).appendTo(w)
	w.I32(0).Str("").I32(0) // 当前没有已部署宠物时的标准空宠物段。

	// 兼容尾按已验证顺序全部发出。第一项是持久化角色 ID，不是 Honor。
	return w.I64(c.ID).
		U8(glowModeOf(c.Appear)).
		U8(b2u8(c.SmartCast)).
		U8(b2u8(c.Appear.GlowUnlocked)).
		I32(c.PetViewMask).
		Bytes()
}
