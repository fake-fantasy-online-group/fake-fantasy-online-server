package protocol

import (
	"sort"
	"strconv"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// ── 下行 opcode。数字全部来自 NetClient.Dispatch 跳转表 + 真实流量对拍 ──
const (
	SCEntitySpawn    = 0x800f // 怪物/宠物出场
	SCGroundSpawn    = 0x8013 // 地面掉落物出场
	SCEntityMove     = 0x8018 // 实体移动
	SCEntityDespawn  = 0x8010 // 玩家/怪物/宠物离开视野
	SCSelfRiding     = 0x8012 // 本人骑乘状态
	SCGroundRemove   = 0x8014 // 地面掉落物移除
	SCRemotePlayer   = 0x8023 // 远端玩家出场
	SCNPCRemove      = 0x8047 // NPC 移除
	SCCombat         = 0x8011 // 战斗结算
	SCServerText     = 0x8008 // 服务端文本动作
	SCCombatNumber   = 0x800c // 精确参数飘字
	SCSkillCast      = 0x800e // 技能吟唱/释放表现
	SCSkillHitFX     = 0x8045 // 技能命中目标的一次性实体特效
	SCInventory      = 0x8006 // 自身背包、穿戴与货币完整快照
	SCInventoryDelta = 0x8069 // 1.5.8 背包、穿戴与货币增量补丁
	SCShopItems      = 0x801c // NPC 商店完整库存
	SCRackCatalog    = 0x803b // 神奇货架目录
	SCFittingCatalog = 0x8059 // 试衣目录；1.5.8 支持精简分页尾
	SCFittingDetails = 0x806e // 1.5.8 试衣物品详情批次
	SCRackRefund     = 0x806c // 1.5.8 货架退货规则与可退购买流水
	SCFamilyStash    = 0x806b // 1.5.8 家族仓库快照与取出权限
	SCChangeSet      = 0x8073 // 1.5.8 快速换装备用套装快照
	SCRepairQuote    = 0x801d // 修理报价
	SCAttributes     = 0x8007 // 自身属性双数组快照
	SCAppearance     = 0x800a // 玩家基础外观变化
	SCSkillSnapshot  = 0x8015 // 已学技能与技能点完整快照
	SCLifeSkills     = 0x8022 // 生活技能列表
	SCCraftRecipes   = 0x8020 // 合成配方列表
	SCRefineInfo     = 0x8021 // 装备精炼信息
	SCSocketInfo     = 0x8024 // 打孔/镶嵌信息
	SCAttrWashInfo   = 0x804e // 洗练信息
	SCHotbar         = 0x8016 // 固定 20 格快捷栏快照
	SCBuffSnapshot   = 0x8017 // buff/debuff 完整快照
	SCPlayerLife     = 0x801a // 玩家死亡/复活状态
	SCStatusEffect   = 0x801b // 世界实体状态特效
	SCTeleport       = 0x803a // 跨场景落点
	SCTransportList  = 0x8049 // NPC 传送目的地列表
	SCTargetStatus   = 0x805e // 目标栏状态快照
	SCEntityStatus   = 0x8064 // 已在场实体的状态图标快照
	SCNpcChatter     = 0x804b // NPC 环境闲聊完整配置
	SCTitleSnapshot  = 0x804c // 当前称号与已解锁称号完整快照
	SCNewbieTipSeed  = 0x8054 // 已读 NewbieTipBox 编号种子
	SCSelfPKState    = 0x8052 // 本人 PK 模式与安全状态
	SCRemotePKState  = 0x8053 // 远端玩家 PK 状态
	SCRemoteRiding   = 0x8058 // 远端玩家骑乘状态
	SCRemoteInvis    = 0x805d // 远端玩家隐身状态
	SCPlayerEmote    = 0x8025 // 玩家表情
	SCPlayerInspect  = 0x8050 // 查看玩家装备
	SCItemIconMap    = 0x804d // 特殊物品图标映射
	SCWarehouse      = 0x8026 // 个人仓库快照
	SCWarehouseDelta = 0x806a // 1.5.8 仓库页头与槽位增量补丁
	SCWardrobe       = 0x8038 // 衣柜容量、收藏与穿戴状态
	SCStallContents  = 0x803c // 摊位内容与成交记录
	SCStallOwner     = 0x803d // 世界摊位招牌状态
	SCStallClosed    = 0x803e // 摊位关闭
	SCHairChanged    = 0x803f // 发型/发色更新
	SCMailList       = 0x801e // 邮件列表
	SCMailNew        = 0x801f // 新邮件标志
	SCSelfData       = 0x8003 // 自身数据
	SCPetSnapshot    = 0x800b // 宠物栏完整快照
	SCPetVitals      = 0x806f // 1.5.8 宠物生命/经验增量
)

type MailAttachmentView struct {
	ItemID int32
	Count  int32
	Name   string
	Desc   string
}

// PetVitals 编码 1.5.8 的固定 10×I32 宠物生命数据。字段顺序由
// Dispatch 对 CharData.petLevel/petExp/petExpUp/petCurHp/petMaxHp/
// petCurSp/petMaxSp/petStarve/petTrust/petPoints 的直接落点闭合。
func PetVitals(p event.PetView) []byte {
	return NewW(SCPetVitals).
		I32(p.Level).I32(p.Exp).I32(p.ExpToNext).
		I32(p.HP).I32(p.MaxHP).I32(p.MP).I32(p.MaxMP).
		I32(p.Starve).I32(p.Trust).I32(p.FreePoints).
		Bytes()
}

type MailView struct {
	ID          int64
	From        string
	Title       string
	Body        string
	Money       int64
	Caiyu       int64
	Type        uint8
	Read        bool
	CanClaim    bool
	CanReturn   bool
	Time        int64
	Attachments []MailAttachmentView
}

// MailList 按 MailDlg.SetMailList 的精确字段顺序生成 0x801e。
func MailList(total, capacity, newCount int32, mails []MailView) []byte {
	if len(mails) > 1<<16-1 {
		return nil
	}
	w := NewW(SCMailList).I32(total).I32(capacity).I32(newCount).U16(uint16(len(mails)))
	for _, mail := range mails {
		if len(mail.Attachments) > 255 || !wireString(mail.From) || !wireString(mail.Title) || !wireString(mail.Body) {
			return nil
		}
		w.I64(mail.ID).Str(mail.From).Str(mail.Title).Str(mail.Body).
			I64(mail.Money).I64(mail.Caiyu).U8(mail.Type).
			U8(b2u8(mail.Read)).U8(b2u8(mail.CanClaim)).U8(b2u8(mail.CanReturn)).
			I64(mail.Time).U8(uint8(len(mail.Attachments)))
		for _, item := range mail.Attachments {
			if !wireString(item.Name) || !wireString(item.Desc) {
				return nil
			}
			w.I32(item.ItemID).I32(item.Count).Str(item.Name).Str(item.Desc)
		}
	}
	return w.Bytes()
}

func MailNew(hasNew bool) []byte { return NewW(SCMailNew).U8(b2u8(hasNew)).Bytes() }

// noActivePetWire 是"当前没有宠在外面"的线上表示。
//
// 依据：唯一一份真实抓包里 `petActive = 255`，而同一份包带着 10 行宠物槽 ——
// 也就是"有 10 只宠但没有出战的那只"。客户端把这个 U8 存进
// `CharData.petActive:System.Int32`（不是 bool，bool 是另一个 petShow 字段），
// 所以它是个槽位号，255 是"没有"的哨兵。
//
// ⚠️ 这一条是**从一个样本推的**，不是逐指令证出来的：真机上放出宠之后要核
// 客户端自己的 `CharData.petActive`，对不上就以客户端为准改这里。
const noActivePetWire = 255

// 0x800f 的 kind 码。**这是客户端的编号, 不是我们的** ——
// game 层用 domain.EntityKind(从 0 开始), 到这里才翻成客户端认识的 3/4。
// 这一行翻译就是分层的价值: 客户端改了编号, 只改这里。
const (
	wireKindMonster = 3
	wireKindPet     = 4
	wireKindTrap    = 5 // 0x800f 的专用 SpawnTrap 分支；NPC 实际走 0x8046
	wireKindDrop    = 6 // 掉落物走 0x8013, 同上
)

// 0x8011 的表现位。同样是客户端编号, 与 event.DamageFlag 不是一回事。
// OnCombat 的实际分支证明 bit0=暴击、bit1=未命中、bit2=致死；此前把 bit0
// 误命名成“首击”，导致每轮交战第一刀都播放暴击表现。
const (
	wireHitNormal = 0
	wireHitCrit   = 1
	wireHitMiss   = 2
	wireHitFatal  = 4
)

// 0x800c CombatNumber.SpawnOn 的目标锚点与表现类型。target 不是实体号：
// 0=本机玩家，1=本机出战宠物；type=5 是客户端内置的治疗数字。
const (
	wireNumberPlayer = 0
	wireNumberPet    = 1
	wireNumberHeal   = 5
)

// 0x800f 的 elite 字节低 7 位是精英类别，最高位会落到客户端
// MonsterEntity.eliteFresh，并触发已有的刷新表现。
const (
	wireElite      = 0x01
	wireSpawnFresh = 0x80
)

// Encode 把一条游戏事件翻成零到多个下行包。
//
// 返回空切片表示当前没有经过验证的下行映射 —— 那是正常的, 不是错误。
// 一个领域事实可能需要多个客户端动作才能完整表达，例如经验同时需要数值飘字
// 与聊天提示；调用方必须按返回顺序逐包发送，不能只取第一包。
func Encode(observer domain.EntityID, ev event.Event) [][]byte {
	switch e := ev.(type) {
	case event.MonsterSpoke:
		return packets(EntityBubble(int32(e.Who), e.Text))
	case event.EntityOwnerChanged:
		return packets(EntityOwner(e.Who, e.Owner))
	case event.EntityEffectRequested:
		return packets(EntityEffect(e.Target, e.Effect))
	case event.HornDialogOpened:
		return packets(OpenHorn(e.Item, e.Tier, e.Skin))
	case event.EntitySpawned:
		// NPC 走**自己的落位包** 0x8046 —— 0x800f 里 NPC 那个 kind 是留空的,
		// 而且 0x800f 带不了对话脚本/立绘这些 NPC 专有字段。
		if e.NPC != nil {
			return packets(encodeNPCSpawn(e))
		}
		if e.Trap != nil {
			return packets(encodeTrapSpawn(e))
		}
		if e.Kind == domain.KindPlayer {
			result := packets(encodeRemotePlayer(e))
			if e.Stall != nil {
				result = append(result, encodeStallOwner(*e.Stall))
			} else {
				result = append(result, NewW(SCStallClosed).I32(int32(e.ID)).Bytes())
			}
			return result
		}
		if e.Kind == domain.KindDrop {
			if e.Ground == nil {
				return nil // 掉落专有字段不全时不能退化成错误的 0x800f
			}
			return packets(encodeGroundSpawn(observer, e))
		}
		if pkt := encodeSpawnFor(observer, e); pkt != nil {
			result := packets(pkt)
			if e.Owner != 0 {
				result = append(result, EntityOwner(e.ID, e.Owner))
			}
			return result
		}
		return nil
	case event.EntityMoved:
		return packets(encodeMove(e))
	case event.RidingChanged:
		return packets(encodeRiding(observer, e))
	case event.PlayerVisibilityChanged:
		if observer != e.Who {
			return packets(encodeRemoteInvisibility(e))
		}
		return nil
	case event.PKStateSnapshot:
		if observer == e.Who {
			return packets(encodeSelfPKState(e))
		}
		return nil
	case event.PlayerPKStateChanged:
		if observer != e.Who {
			return packets(encodeRemotePKState(e))
		}
		return nil
	case event.PlayerRenamed:
		if observer != e.Who {
			return packets(encodePlayerRenamed(e))
		}
		return nil
	case event.PlayerEmoted:
		if e.Face < 0 || e.Face >= 96 {
			return nil
		}
		return packets(NewW(SCPlayerEmote).I32(int32(e.Who)).I32(e.Face).Bytes())
	case event.PlayerEquipmentInspected:
		if pkt := encodePlayerEquipmentInspected(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.EntityDespawned:
		return packets(encodeDespawn(e))
	case event.NpcTasks:
		return packets(encodeNpcTasks(e))
	case event.NpcChatter:
		if pkt := encodeNpcChatter(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.NpcDialogText:
		return packets(encodeNpcDialogText(e))
	case event.ActiveQuests:
		return packets(encodeActiveQuests(e))
	case event.TitleSnapshot:
		if pkt := encodeTitleSnapshot(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.QuestAccepted:
		return packets(encodeQuestAccepted(e))
	case event.QuestItemObtained:
		return packets(encodeQuestItemObtained(e))
	case event.QuestItemBlocked:
		return packets(encodeQuestItemBlocked(e))
	case event.QuestCompleted:
		return packets(encodeQuestCompleted(e))
	case event.DamageDealt:
		return packets(encodeDamage(e))
	case event.HealDone:
		if pkt := encodeHealNumber(observer, e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.SkillCastChanged:
		return packets(encodeSkillCast(observer, e))
	case event.SkillHitEffect:
		if e.Target == 0 || !wireString(e.Effect) {
			return nil
		}
		return packets(NewW(SCSkillHitFX).I32(int32(e.Target)).Str(e.Effect).Bytes())
	case event.ExpGained:
		return encodeExpGained(e)
	case event.InventorySnapshot:
		if pkt := encodeInventory(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.RackCatalogSnapshot:
		if pkt := encodeRackCatalog(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.FittingCatalogSnapshot:
		if pkt := encodeFittingCatalog(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.FittingDetailsSnapshot:
		if pkt := encodeFittingDetails(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.RackRefundSnapshot:
		if pkt := encodeRackRefunds(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.FamilyStashSnapshot:
		if pkt := encodeFamilyStash(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.ChangeSetSnapshot:
		if pkt := encodeChangeSet(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.ItemIconMap:
		if len(e.Pairs) > 1<<16-1 {
			return nil
		}
		w := NewW(SCItemIconMap).U16(uint16(len(e.Pairs)))
		for _, pair := range e.Pairs {
			if pair.Item <= 0 || pair.Icon <= 0 {
				return nil
			}
			w.I32(int32(pair.Item)).I32(pair.Icon)
		}
		return packets(w.Bytes())
	case event.WarehouseSnapshot:
		if pkt := encodeWarehouse(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.WardrobeSnapshot:
		if pkt := encodeWardrobe(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.StallContents:
		if pkt := encodeStallContents(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.StallOwnerChanged:
		if pkt := encodeStallOwner(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.StallClosed:
		if e.Owner != 0 {
			return packets(NewW(SCStallClosed).I32(int32(e.Owner)).Bytes())
		}
		return nil
	case event.ItemLockChanged:
		kind, code := uint8(2), uint8(1)
		if e.On {
			kind = 1
		}
		if e.OK {
			code = 0
		}
		if pkt := AccountSecurityResult(kind, code, e.Message, e.Tab, domain.ClientBagPosition(e.Slot), e.Item); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.ShopItems:
		if pkt := encodeShopItems(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.RepairQuoted:
		if pkt := encodeRepairQuote(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.StatsChanged:
		return packets(encodeAttributes(e))
	case event.NewbieTipSeed:
		return packets(encodeNewbieTipSeed(e))
	case event.NewbieTipRequested:
		if pkt := encodeNewbieTip(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.SkillSnapshot:
		if pkt := encodeSkillSnapshot(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.LifeSkillSnapshot:
		if pkt := encodeLifeSkills(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.CraftRecipeSnapshot:
		if pkt := encodeCraftRecipes(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.RefineInfo:
		if pkt := encodeRefineInfo(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.SocketInfo:
		if pkt := encodeSocketInfo(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.AttrWashInfo:
		if pkt := encodeAttrWashInfo(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.NewbieTextNotice:
		if e.Who == observer {
			return packets(encodeServerText(e.Text, serverTextNewbieTip))
		}
		return nil
	case event.ServerNotice:
		if e.Who == observer {
			return packets(encodeServerText(e.Text, serverTextChat))
		}
		return nil
	case event.HotbarSnapshot:
		return packets(encodeHotbarSnapshot(e))
	case event.BuffSnapshot:
		if pkt := encodeBuffSnapshot(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.EntityStatusSnapshot:
		if pkt := encodeEntityStatus(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.EntityStatusEffect:
		if pkt := encodeStatusEffect(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.PetSnapshot:
		if pkt := encodePetSnapshot(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.Teleported:
		return packets(encodeTeleport(e))
	case event.TransportDestinations:
		if pkt := encodeTransportDestinations(e); pkt != nil {
			return packets(pkt)
		}
		return nil
	case event.AppearanceChanged:
		return packets(encodeAppearance(e))
	case event.HairChanged:
		return packets(NewW(SCHairChanged).I32(int32(e.Who)).U8(e.Appearance.Head).
			U8(e.Appearance.Hair).Bytes())
	case event.EntityDied:
		if e.Kind == domain.KindPlayer {
			return packets(encodePlayerDied(e))
		}
	case event.PlayerRevived:
		return packets(encodePlayerLife(e.Who, true, 0))
	case event.TargetStatusSnapshot:
		if pkt := encodeTargetStatus(e); pkt != nil {
			return packets(pkt)
		}
	case event.Rejected:
		if e.Who == observer {
			if e.Cmd == "StartWork" {
				return packets(encodeWorkRejected(e.Reason))
			}
			return packets(encodeRejected(e))
		}
	case event.WorkStopped:
		if e.Who == observer {
			return packets(encodeWorkStopped(e.Reason))
		}
	}
	return nil
}

// encodeNewbieTipSeed 生成 0x8054：U16 count + count × I32 tipId。
// 排序与去重让同一份角色状态总是产生相同字节；非法编号不进线。
func encodeNewbieTipSeed(e event.NewbieTipSeed) []byte {
	ids := append([]int32(nil), e.TipIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	clean := ids[:0]
	for _, id := range ids {
		if id <= 0 || (len(clean) > 0 && clean[len(clean)-1] == id) {
			continue
		}
		if len(clean) == 1<<16-1 {
			return nil
		}
		clean = append(clean, id)
	}
	w := NewW(SCNewbieTipSeed).U16(uint16(len(clean)))
	for _, id := range clean {
		w.I32(id)
	}
	return w.Bytes()
}

const serverTextNewbieTip = 5

// encodeNewbieTip 使用客户端已经验证的 0x8008 mode=5 →
// NewbieTipBox.PushText(text) 路径。这个入口只接收文本而不接收 tipId，因此
// ID→文案必须与正式客户端内置字典逐字一致；一次性判定由会话层负责。
func encodeNewbieTip(e event.NewbieTipRequested) []byte {
	text := newbieTipText[e.ID]
	if text == "" {
		return nil
	}
	return encodeServerText(text, serverTextNewbieTip)
}

// 第一批只收录服务端能由权威玩法结果精确判定的提示。其余文案虽然也存在于
// 客户端，但 F11、打开面板等纯本地动作没有服务端入口，不能随便挂到近似事件上。
var newbieTipText = map[int32]string{
	1:  "挺厉害的嘛！不过也不要过分轻敌哦！最好找适合自己的怪物练级，如果想挑战更强的敌人，建议与你的朋友们一起组队，否则会白白送掉性命的哦！看见怪物的掉落物了吗？快去捡起来吧！",
	2:  "做得对！将鼠标移到物品上，当其形状变成人手时，单击左键就可以捡起来啦！注意，当物品的名字是红色时，那表明它属于别人，你是无法捡拾的。",
	3:  "恭喜你升级啦！每次升级时，你都会获得自由分配的 2 个属性点，单击“C”键打开角色面板，使用“＋”按钮将属性点添加到你需要的属性上吧，添加完后别忘了点击“确认”，也可以选择由系统“自动分配”。",
	4:  "对了！装备就是这样使用的！既可以从物品栏拖动到角色栏的装备区，也可以在直接左键双击该装备。同样地，如果要脱下装备，既可以将装备拖回到物品栏，也可以通过左键双击脱下。",
	5:  "没错，道具就是这样使用的，左键双击就可以啦！也可以把道具拖到快捷栏中，单击对应的快捷键（F1～F10）就可以使用啦！是不是很方便呢？",
	6:  "左键单击某只怪物，在屏幕左上方可以查看到该怪物的名称、等级和血量，怪物名字的颜色不同，代表着怪物对你的危险程序不同：绿色和灰色的怪物比较安全，红色和紫色的怪物则是危险的，而黄色则是适合你挑战的怪物。",
	8:  "做得不错！要想战胜强大的敌人，技能是必不可少的！如果你想升级技能，就要努力积累技能点和提升自己的等级哦！",
	9:  "你的技能升级啦！不错不错！技能升级以后，会消耗更多的内力，但是威力会变得更强喔！某些技能升级后还可激活其它新技能的学习条件。",
	10: "恭喜你交到第一个好朋友！你可以右键点击某个玩家将其加为好友，也可以在好友面板中直接输入该玩家的名字。如果你想在幻想世界里玩得更开心，就快去交几个好朋友吧！",
	13: "交易的操作很简单吧？你可以直接拖动，也可以双击要交易的物品，对于可以叠加的物品，还需输入交易的数量。交易过程中，你可以在清单面板里查看交易的物品数量和金钱数额。",
	14: "做得好！为了确保交易的安全，双方在交易之前必须先确定自己是否已经就绪，并且可以随时取消交易。虽然麻烦一点，但是安全和公平最重要，对吗？",
	15: "没错！鼠标左键单击一下怪物就可以对其进行攻击啦，按住Ctrl键后可自动进行连续攻击。鼠标左键单击地面，就可以结束攻击。",
	17: "恭喜你成功创建了自己的队伍！成为队长的感觉是不是很棒哩？你可以点击队伍面板里的“组队”按钮创建队伍，也可以右键点击某个自由玩家发出“组队邀请”创建队伍。组队后队长可以邀请更多的玩家加入，队伍最多容纳10人。",
	18: "恭喜你成功与人组队！你可以接收邀请加入队伍，也可以右键点击某队长申请“加入队伍”。组队后你在小地图上可以看到队友们的位置。点击队伍面板里的“离开”按钮即可离开队伍。如果右键点击队长选择“跟随”，就可进入自动跟随的状态。",
	19: "恭喜你顺利完成第一个任务！随着等级的提升，后面的任务会越来越有挑战性哦！通过做任务，你可以就职、可以获得丰富的奖励和称号，还可以积累经验值，做任务将使你的升级过程变得更加有趣哦！",
	26: "点击F12键，你可以打开帮助面板，通过查询你可以获得关于游戏各方面的详尽介绍，你一定会觉得很方便的！再次点击F12即可关闭帮助文件。",
	28: "很好！行走时，只需将鼠标指向目的点，然后单击左键即可；也可以按下鼠标左键停留一秒钟，你就会跟随鼠标的指向而移动，鼠标移动时会出现一组跟随的蓝色光圈，指出你将要移动的方向；按住Ctrl键再单击鼠标左键，也可以实现跟随行走的功能。",
	29: "哈哈，没错，单击“S”键就坐下啦，再次单击此键你就可以重新站起来。坐下的时候，你的生命和法力的回复速度会比站立时更快一些。",
	30: "我们为您准备了一套初行者的套装和一把防身的小剑，单击“I”键打开物品栏，就放在装备栏里，看见了吗？左键双击就可以穿上套装、拿起小剑啦！",
	33: "恭喜你升到10级！现在你可以就职啦！在龙城找钟离门人裴谌(254,106) 可就职战士，找吕仙门人许栖岩(293,138)可就职剑客，找仙姑门人萼绿华(242,141)可就职刺客，找铁拐门人蓟子训(219,188)可就职药师，找果老门人寒山子(142,122)可就职术士。",
	34: "看见了吧？只要在对象栏里输入玩家名字，就可以与他或她进行密聊啦！也可以在好友面板中右键单击某个好友选择“发送信息”，或者右键单击某个游戏中的玩家选择“与他交谈”。左键单击聊天窗口里显示的玩家名字，也可以自动将其列为密语对象哦！",
	35: "开始打工了吗？打工的过程中你可以和朋友聊聊天，如果在野外，还可以顺便欣赏一下四周美丽的风景，是不是很悠闲惬意哩？如果想结束打工，只需要左键点击自己或头顶的文字，确认后就可以啦。",
	42: "恭喜你升到11级！想拥有一只自己的宠物吗？只需要买一个捕兽夹，然后去野外捕捉特定的怪物就可以啦！现在你可以捕捉的宠物有：白兔、海龟、绿蛙兵。",
	43: "想拥有一只更强大的宠物吗？去买一个玉净瓶，然后去野外捕捉红鱼怪和QQ糖吧，祝你好运哦！",
	44: "想捕捉火鸟、大闸蟹和毒蜘蛛作为自己的骑宠吗？带上捆仙索吧！不过要小心受到攻击哦！",
	45: "运气不错哦！终于拥有自己的第一只宠物啦！点击屏幕左上方的宠物头像打开宠物面板，在面板下方是专为宠物设定的宠物栏，你刚刚捕捉到的小家伙就在里面，双击它以后就可以将其召唤到身边啦！如果想让小家伙回到栏中休息，再次双击它就可以啦。",
	47: "你受伤啦，快吃些东西补充一下体能吧，食品和药品在城市和村庄的道具商人那里可以买到，你也可以通过按“S”键坐下来恢复体力。对于新手而言，桃源村和长乐村的治疗师们可以为你们免费治疗哦！",
	50: "你的宠物升级啦！小家伙这么努力，你是不是应该好好奖励奖励它呢？记住，只有召唤在身边的宠物才能分取你战斗时的经验值，所以如果你希望宠物快些成长，就要经常与它并肩作战哦！",
	64: "想拥有一只更强大的宠物吗？去买一个老君壶，去野外捕捉大蜗牛怪和山狼吧，祝你好运哦！",
	65: "想拥有一只更强大的宠物吗？去买一个捆仙索，去野外和迷宫捕捉独角兽和苍狼皇吧，祝你好运哦！",
}

// encodeTargetStatus 生成 0x805e。23 组真实流量闭合了空列表；非空行的字段顺序
// 字段对应 GameHud.TgSt 的构造参数。超过 U8 容量时拒绝编码，不能截断计数后
// 继续追加数据把后续帧边界写坏。
func encodeTargetStatus(e event.TargetStatusSnapshot) []byte {
	if len(e.Statuses) > 255 {
		return nil
	}
	w := NewW(SCTargetStatus).I32(int32(e.Target)).U8(uint8(len(e.Statuses)))
	for _, st := range e.Statuses {
		w.I32(st.Icon).U8(b2u8(st.Beneficial)).I32(st.RemainMS).Str(st.Name).Str(st.Desc)
	}
	return w.Bytes()
}

// encodeSkillCast 生成 0x800e。根字段和三个兼容尾的顺序来自客户端 Dispatch；
// phase=1 会进入 CombatWorld.PlayerCast 的释放分支并调用 SkillCaster.Cast/PlayFire。
// 客户端会无条件把 cooldown 写入自己的技能快捷栏，所以只有施法者本人能收到
// 非零值；旁观者仍收到同一条表现包，但不能被别人的同名技能锁住快捷栏。
func encodeSkillCast(observer domain.EntityID, e event.SkillCastChanged) []byte {
	cooldownMS := int32(0)
	if observer == e.Caster {
		cooldownMS = e.CooldownMS
	}
	w := NewW(SCSkillCast).
		I32(int32(e.Skill)).I32(e.ChantHoldMS).U8(uint8(e.Phase)).I32(cooldownMS).
		I32(int32(e.Caster)).I32(int32(e.Target))
	if e.Aim != nil {
		w.U8(1).I32(int32(e.Aim.X)).I32(int32(e.Aim.Y))
	}
	// 明确写 100%，走客户端已验证的 NextScale=1 分支；不依赖静态默认值。
	return w.U8(100).Bytes()
}

func packets(pkt []byte) [][]byte {
	if len(pkt) < 2 {
		return nil
	}
	return [][]byte{pkt}
}

// SCNPCSpawn 是 NPC 落位包。
//
// **NPC 不走 0x800f。** 0x800f 的 kind 码里 NPC 那位是留空的,
// 而且它带不了 sprite 资源名/对话脚本/立绘 —— 那几样是 NPC 系统专有的。
const SCNPCSpawn = 0x8046

// npcStandingDir 是这个客户端 NPC 落位包使用的固定站立朝向。
//
// 真服留存的 58 个 0x8046 样本末字段全部为 3。客户端随后会在
// PlayerController.OctToStored 中只取该值的低 3 位来选择 NPC 朝向帧。
// `.npc` 的 Init_Dir 还包含旧客户端的高位方向编码，不能原样下发：例如
// 15/63 会被当前客户端当成方向 7，41 会被当成方向 1，导致部分 NPC 选中
// 裁切边界不同的错误朝向帧，模型便会相对影子和头顶标记产生小幅偏移。
const npcStandingDir int32 = 3

// encodeNPCSpawn 生成 0x8046。
//
// 字段顺序取自 58 个真实样本并与 NpcEntry 字段数据流交叉验证:
//
//	i32 实体id | i32 x | i32 y | str sprite | str 名字 | str 名牌 | str 立绘 | str 脚本
//	| i32 商店表id | i32 传送表id | i32 方向
//
// ⚠️ **实体 id 必须是场景分配的那个。**
// 以前这个包由 session/scenegen 单独生成, 用的是自己编的 2000000+ 序号,
// 于是客户端手上的 NPC id 与场景里的对不上 —— 客户端说"攻击 2000000",
// 场景里根本没有这个实体, 命令投进去查不到目标, **静静地什么都不发生**。
func encodeNPCSpawn(e event.EntitySpawned) []byte {
	n := e.NPC
	// 字段序列:
	//
	//	+0x10 I32  +0x20 I32  +0x30 I32  +0x40 Str  +0x50 Str  +0x60 Str
	//	+0x70 Str  +0x80 Str  +0x90 I32  +0xa0 I32  +0xb0 I32
	//	        ↓ 这里跳了 264 字节 —— 后面的读取在**条件分支**里, 不是定长尾部
	//	+0x1b8 I32  +0x234 U8  +0x244 Str …
	//
	// 后续字段位于条件分支中，不属于必写字段。
	return NewW(SCNPCSpawn).
		I32(int32(e.ID)).
		I32(int32(e.Pos.X)).I32(int32(e.Pos.Y)).
		Str(n.Sprite).
		Str(e.Name).
		Str(n.Label).
		Str(n.Portrait).
		Str(n.Script).
		I32(n.Sell).I32(n.Trans).I32(npcStandingDir).
		Bytes()
}

// npcSpawnFields 是客户端**无条件连续读**的字段数。
//
// 前 11 个字段为连续读取的必填字段；后续字段位于条件分支中。
const npcSpawnFields = 11

// encodeSpawn 生成 0x800f。布局经 297 个真实样本闭合校验:
//
//	i32 实体id | u8 kind | i32 模型号 | u16 名字长 | utf8 名字
//	| i32 等级,x,y,当前HP,最大HP,ownerId | u8 eliteAndFresh
//	| [i32 mapId] [u8 friendly] [u8 statusFxCount] [u8 statusIconCount]
//
// 真实样本均带最后四段兼容尾且两个状态计数为 0；发出 mapId 能让客户端在
// 切图竞态里拒绝旧场景实体。事件模型已携带 owner 与 elite；
// friendly/status 尚未进入领域模型，因而发送明确的零值。
func encodeSpawn(e event.EntitySpawned) []byte {
	return encodeSpawnFor(0, e)
}

func encodeSpawnFor(observer domain.EntityID, e event.EntitySpawned) []byte {
	if len(e.StatusIcons) > 255 {
		return nil
	}
	friendly := e.Owner != 0 && e.Owner == observer
	w := NewW(SCEntitySpawn).
		I32(int32(e.ID)).
		U8(wireKind(e.Kind)).
		I32(e.Look.ModelID).
		Str(e.Name).
		I32(e.Level).
		I32(int32(e.Pos.X)).
		I32(int32(e.Pos.Y)).
		I32(e.HP).
		I32(e.MaxHP).
		I32(int32(e.Owner)).
		U8(b2u8(e.Elite)*wireElite | b2u8(e.Fresh)*wireSpawnFresh).
		I32(e.Pos.MapID).
		U8(b2u8(friendly)).
		U8(0). // statusFxCount：当前没有独立于 BuffSnapshot 的世界持续特效来源
		U8(uint8(len(e.StatusIcons)))
	for _, st := range e.StatusIcons {
		w.I32(st.Status).I32(st.Icon).U8(b2u8(st.Beneficial)).I32(st.RemainMS)
	}
	return w.Bytes()
}

// encodeTrapSpawn 仍写完整的 0x800f 根字段；客户端读完根字段后在 kind=5
// 分支只把 id/skill/x/y/radius 交给 CombatWorld.SpawnTrap。
func encodeTrapSpawn(e event.EntitySpawned) []byte {
	t := e.Trap
	if t == nil || t.Skill <= 0 || t.Radius <= 0 {
		return nil
	}
	e.Kind = domain.KindTrap
	e.Look.ModelID = int32(t.Skill)
	e.HP = t.Radius
	e.MaxHP = 0
	e.StatusIcons = nil
	return encodeSpawnFor(0, e)
}

// encodeGroundSpawn 生成 0x8013：
//
//	i32 实体id | i32 物品id | i32 数量 | i32 x | i32 y | str 名字
//	| u8 owned | i32 剩余归属保护毫秒 | u8 kind | [u8 quality]
//
// owned 是**相对于接收者**的字段，因此 Encode 必须接收 observer；同一事件发给
// 击杀者和路人时字节不同。kind 的真实样本均为 0；quality 来自已经创建好的装备
// 实例品质，与背包、穿戴、仓库使用同一 0..4 枚举，普通物品自然为 0。
func encodeGroundSpawn(observer domain.EntityID, e event.EntitySpawned) []byte {
	g := e.Ground
	lockMS := g.ProtectionMS
	if lockMS < 0 {
		lockMS = 0
	}
	owned := g.Owner != 0 && g.Owner == observer
	return NewW(SCGroundSpawn).
		I32(int32(e.ID)).I32(int32(g.Item)).I32(g.Count).
		I32(int32(e.Pos.X)).I32(int32(e.Pos.Y)).Str(e.Name).
		U8(b2u8(owned)).I32(lockMS).
		U8(0). // kind：两条真实样本均为 0；客户端当前版本只存储、不分支使用
		U8(g.Quality).
		Bytes()
}

// encodeRemotePlayer 生成 0x8023。远端玩家不走 0x800f；客户端为两者维护的
// 对象类型和解析布局完全不同。根字段及已闭合的兼容尾全部写出；骑乘取玩家
// 当前在线状态，阵营字段尚无领域状态而使用中性值；隐身走完整出场快照，地图
// 守卫使用实体当前地图。
func encodeRemotePlayer(e event.EntitySpawned) []byte {
	v := e.Look.EquipView
	return NewW(SCRemotePlayer).
		I32(int32(e.ID)).
		Str(e.Name).
		F32(float32(e.Pos.X)).F32(float32(e.Pos.Y)).
		I32(e.Level).
		U8(uint8(e.Look.Race)). // ignoredRaceCompat：本客户端读取后丢弃
		U8(e.Look.Gender).U8(e.Look.Head).U8(e.Look.Hair).
		Str(e.Look.Title).
		U16(v[0]).U16(v[1]).U16(v[2]).U16(v[3]).U16(v[4]).U16(v[5]).
		U8(e.Look.AtkVariant).
		U8(e.Look.WeaponCType). // ignoredWeaponCTypeCompat
		U16(e.Look.AtkDist).    // legacyAtkDist16
		U8(b2u8(e.HP <= 0)).
		Str(e.Look.Aura).
		Str(e.Look.SoulFX).
		U8(0). // faction
		I32(e.HP).I32(e.MaxHP).
		Str(e.Look.TitleFX).
		U8(b2u8(e.Riding)).I32(e.MountModel).
		U8(0).            // mountTianmo=false
		I32(e.Pos.MapID). // 客户端用它防止把旧地图延迟包生成到新图
		U8(glowModeOf(e.Look.Appearance)).
		U8(e.RideSeat).I32(int32(e.RideAnchor)).
		U8(e.RideSeats).
		U8(b2u8(e.Invisible)).
		Bytes()
}

// encodeRiding 把同一个骑乘事实按接收者翻译：本人走 0x8012 更新 CharData 和
// PlayerController，旁观者走 0x8058 调 RemotePlayer.SetRiding；同乘座位和
// 司机实体使用已确认的兼容尾，结束骑乘时清空旧座位。
func encodeRiding(observer domain.EntityID, e event.RidingChanged) []byte {
	model := e.MountModel
	if !e.Riding {
		model = 0
		e.Seat = 0
		e.Anchor = 0
		e.Seats = 0
	}
	if observer == e.Who {
		w := NewW(SCSelfRiding).U8(b2u8(e.Riding)).I32(model).
			U8(e.Seat).I32(int32(e.Anchor)).U8(0)
		// 1.5.8原生解析器在tianmo后再读可选U8座位数，写入CharData.rideSeats。
		if e.Seats > 0 {
			w.U8(e.Seats)
		}
		return w.Bytes()
	}
	return NewW(SCRemoteRiding).I32(int32(e.Who)).U8(b2u8(e.Riding)).I32(model).
		U8(0).U8(e.Seat).I32(int32(e.Anchor)).U8(e.Seats).Bytes()
}

// encodeRemoteInvisibility 生成 0x805d：I32 玩家实体号 + U8 隐身开关。
// 客户端直接调用 RemotePlayer.SetInvis；本人不接收这条远端实体协议。
func encodeRemoteInvisibility(e event.PlayerVisibilityChanged) []byte {
	return NewW(SCRemoteInvis).
		I32(int32(e.Who)).
		U8(b2u8(e.Invisible)).
		Bytes()
}

func encodeSelfPKState(e event.PKStateSnapshot) []byte {
	return NewW(SCSelfPKState).
		U8(e.Mode).I64(e.Karma).U8(e.State).U8(b2u8(e.MapSafe)).I32(e.ProtectLevel).
		Bytes()
}

func encodeRemotePKState(e event.PlayerPKStateChanged) []byte {
	return NewW(SCRemotePKState).I32(int32(e.Who)).U8(e.State).Bytes()
}

func encodePlayerRenamed(e event.PlayerRenamed) []byte {
	return NewW(SCRenameNotice).I32(int32(e.Who)).Str(e.Name).Bytes()
}

// encodeMove 生成 0x8018。
//
//	I32 实体id | I32 速度 | U8 路径点数 | 每点: I32 x, I32 y
//
// 客户端那边是 `CombatWorld.MovePath(id, xs, ys, speed)` —— **是一条路径**,
// 不是单个坐标。
//
// ⚠️ 以前这里发的是固定 17 字节 `id | -1 | 标志 | x | y`, 而客户端把它读成了
// `id | speed=-1 | 点数=标志 | 一个点`。长度碰巧读满(17 字节), 所以协议裁判
// 报"✓ 零越界" —— **裁判只验长度不验语义**, 这种错它抓不到。
// 表现是别人在你屏幕上瞬移或者不动(速度是 -1)。
//
// 现在只发一个点(服务端本来就是逐点广播的), 但结构按真定义写:
// 点数 = 1, 速度取实体的移动速度。
func encodeMove(e event.EntityMoved) []byte {
	speed := e.Speed
	if speed <= 0 {
		speed = defaultMoveSpeed
	}
	if e.Snap {
		speed = -1 // 客户端立即校正坐标、清空路径，并保留当前 curDir/atkFrame
	} else if e.Stop {
		// 0 不改写客户端已存的移动速度；这条单点路径在目标处自然结束。
		// 若改发默认正速度，客户端会把“停止确认”重新解释成一次移动起步。
		speed = 0
	}
	return NewW(SCEntityMove).
		I32(int32(e.ID)).
		I32(speed).
		U8(1). // 路径点数
		I32(int32(e.To.X)).
		I32(int32(e.To.Y)).
		Bytes()
}

// defaultMoveSpeed 是事件没带速度时用的值。
// 普通路径不能发 0 或负数；-1 只用于已明确要求立即校正并清路径的 Snap。
const defaultMoveSpeed = 100

// encodeDespawn 按实体种类选择客户端对应的移除入口：玩家/怪物/宠物走 0x8010，
// 地面掉落物走 0x8014，NPC 走 0x8047。后两者都只有一个 i32 实体 id。
// 0x8010 实测 3 个样本均为 5 字节: i32 实体id + u8 reason。
// 0x805e 是目标状态快照，拿它离场只会清空状态图标，不会移除实体。
//
// 那个 u8 在实测样本里恒为 0。DespawnReason 目前全部映射到 0 ——
// **诚实记录: 我们不知道它非零时是什么意思**, 猜一个值发过去可能让客户端播错动画。
// 等抓到死亡/拾取的真实样本再分化。
func encodeDespawn(e event.EntityDespawned) []byte {
	switch e.Kind {
	case domain.KindDrop:
		return NewW(SCGroundRemove).I32(int32(e.ID)).Bytes()
	case domain.KindNPC:
		return NewW(SCNPCRemove).I32(int32(e.ID)).Bytes()
	}
	return NewW(SCEntityDespawn).I32(int32(e.ID)).U8(0).Bytes()
}

// encodeDamage 生成 0x8011。实测 17 字节:
//
//	i32 源 | i32 目标 | i32 伤害 | u8 标志 | i32 目标剩余HP
//
// 校验过的不变量: 上次剩余 − 本次剩余 == 伤害(11/11 样本); 致死时伤害可以大于剩余血。
func encodeDamage(e event.DamageDealt) []byte {
	return NewW(SCCombat).
		I32(int32(e.Src)).
		I32(int32(e.Dst)).
		I32(e.Amount).
		U8(wireHitFlag(e.Flag)).
		I32(e.DstHP).
		Bytes()
}

// encodeHealNumber 生成 0x800c 治疗飘字。0x8011 是 CombatWorld.OnCombat；即使
// 伤害值为负，客户端仍会调用 PlayHurt 和受击音效，所以治疗绝不能复用它。
//
// 0x800c 的 target 是接收客户端上的玩家/宠物锚点，不是全局实体号。因此同一条
// HealDone 可以广播，但只有被治疗玩家本人（或宠物主人）会得到这条表现包。
// 权威 HP 仍由紧随治疗事件的 0x8007 属性快照更新。
func encodeHealNumber(observer domain.EntityID, e event.HealDone) []byte {
	if e.Amount <= 0 {
		return nil
	}
	target := uint8(wireNumberPlayer)
	switch {
	case e.DstKind == domain.KindPlayer && observer == e.Dst:
	case e.DstKind == domain.KindPet && observer == e.DstOwner:
		target = wireNumberPet
	default:
		return nil
	}
	return NewW(SCCombatNumber).U8(target).I32(e.Amount).U8(wireNumberHeal).Bytes()
}

// encodeExpGained 按真实击杀流量生成两包，顺序不可交换：
//
//	0x800c target=0 | value=本次经验 | type=4
//	0x8008 "获得了 N 点经验" | displayMode=1
//
// Total 是服务端持久化总经验；当前客户端没有已证实的直接字段，因此不擅自塞入。
func encodeExpGained(e event.ExpGained) [][]byte {
	// 线上字段只有 I32。超出范围时宁可暴露为“事件无映射”日志，也不能截断后
	// 给客户端显示一个错误甚至负数的经验值。
	if e.Delta <= 0 || e.Delta > 1<<31-1 {
		return nil
	}
	value := int32(e.Delta)
	number := NewW(SCCombatNumber).U8(0).I32(value).U8(4).Bytes()
	message := NewW(SCServerText).
		Str("获得了 " + strconv.FormatInt(e.Delta, 10) + " 点经验").
		U8(1).
		Bytes()
	return [][]byte{number, message}
}

const (
	serverTextTaskTrack = 4
	serverTextNpcDialog = 6
)

// 任务接取/完成没有独立的下行 opcode。接取提示使用 0x8008 的
// displayMode=4（PlayerTaskTrack.Show）；完成奖励文本必须使用 displayMode=6
// 进入 DialogUI.ShowSay，才能替换并关闭客户端的交付/领奖子页。0x8027 仍只负责权威任务本。
func encodeQuestAccepted(e event.QuestAccepted) []byte {
	return encodeServerText("接受任务："+e.Name, serverTextTaskTrack)
}

func encodeQuestItemObtained(e event.QuestItemObtained) []byte {
	if e.Name == "" || e.Have <= 0 || e.Need <= 0 || e.Have > e.Need {
		return nil
	}
	return encodeServerText("获得一个"+e.Name+"！", serverTextNewbieTip)
}

func encodeQuestItemBlocked(e event.QuestItemBlocked) []byte {
	if e.Name == "" {
		return nil
	}
	return encodeServerText("背包已满，未获得任务物品："+e.Name, serverTextTaskTrack)
}

func encodeQuestCompleted(e event.QuestCompleted) []byte {
	lines := make([]string, 0, 4)
	if e.Exp > 0 {
		lines = append(lines, "奖励经验："+strconv.FormatInt(e.Exp, 10)+"点")
	}
	if money := formatQuestRewardMoney(e.Money); money != "" {
		lines = append(lines, "奖励金钱："+money)
	}
	if e.Honor > 0 {
		lines = append(lines, "奖励名誉："+strconv.FormatInt(e.Honor, 10)+"点")
	}
	if len(e.Items) > 0 {
		items := make([]string, 0, len(e.Items))
		for _, item := range e.Items {
			if item.Name == "" || item.Qty <= 0 {
				continue
			}
			items = append(items, item.Name+"("+strconv.FormatInt(int64(item.Qty), 10)+")")
		}
		if len(items) > 0 {
			lines = append(lines, "奖励道具："+strings.Join(items, "、"))
		}
	}
	if e.Title != "" {
		lines = append(lines, "获得称号："+e.Title)
	}
	if len(lines) == 0 {
		return nil
	}
	return encodeServerText(strings.Join(lines, "\n"), serverTextNpcDialog)
}

func encodeNpcDialogText(e event.NpcDialogText) []byte {
	return encodeServerText(e.Text, serverTextNpcDialog)
}

func formatQuestRewardMoney(m domain.Money) string {
	parts := make([]string, 0, 3)
	if m.Gold > 0 {
		parts = append(parts, strconv.FormatInt(m.Gold, 10)+"金币")
	}
	if m.Silver > 0 {
		parts = append(parts, strconv.FormatInt(m.Silver, 10)+"银币")
	}
	if m.Copper > 0 {
		parts = append(parts, strconv.FormatInt(m.Copper, 10)+"铜币")
	}
	return strings.Join(parts, " ")
}

func encodeServerText(text string, displayMode uint8) []byte {
	if !wireString(text) {
		return nil
	}
	return NewW(SCServerText).Str(text).U8(displayMode).Bytes()
}

// encodeRejected 把领域拒绝原因翻成正式客户端已有的系统文本动作。命令名只
// 用于服务端日志；面向玩家的文字描述业务原因，不暴露内部类型或枚举值。
func encodeRejected(e event.Rejected) []byte {
	text := map[event.RejectReason]string{
		event.RejectUnknown:               "操作失败，请稍后重试。",
		event.RejectNotInGame:             "尚未进入游戏，无法执行该操作。",
		event.RejectTooFar:                "距离目标太远。",
		event.RejectLevelTooLow:           "当前等级不足。",
		event.RejectBagFull:               "背包空间不足。",
		event.RejectNoTarget:              "没有可用的目标。",
		event.RejectTargetDead:            "目标已经死亡。",
		event.RejectOnCooldown:            "操作冷却中，请稍后再试。",
		event.RejectNotEnoughMP:           "法力不足。",
		event.RejectStaminaEmpty:          "耐力不足。",
		event.RejectNotEquippable:         "该物品无法装备。",
		event.RejectStatTooLow:            "角色属性未达到装备要求。",
		event.RejectWrongSex:              "角色性别不符合装备要求。",
		event.RejectSkillNotLearned:       "尚未学习该技能。",
		event.RejectSkillNotUsable:        "该技能当前无法使用。",
		event.RejectStunned:               "当前无法行动。",
		event.RejectSilenced:              "封印状态下无法使用技能。",
		event.RejectLevelTooHigh:          "当前等级超过允许范围。",
		event.RejectQuestActive:           "该任务已经在进行中。",
		event.RejectQuestDone:             "该任务已经完成。",
		event.RejectQuestNotActive:        "尚未接受该任务。",
		event.RejectQuestNotDone:          "任务条件尚未完成。",
		event.RejectInstanceFull:          "当前副本人数已满。",
		event.RejectPartyRequired:         "请先组队再进入副本。",
		event.RejectDungeonNotClear:       "请先消灭当前楼层的全部怪物。",
		event.RejectAlreadyWorking:        "你已经在打工了。",
		event.RejectNotCapturable:         "该目标无法捕捉。",
		event.RejectTargetNotWeak:         "目标生命过高，请先将其打至虚弱。",
		event.RejectNoCaptureTool:         "缺少对应的捕捉工具。",
		event.RejectPetSlotsFull:          "宠物栏已满。",
		event.RejectNoPet:                 "没有找到对应宠物。",
		event.RejectPetAlreadyOut:         "已经有宠物出战。",
		event.RejectPetDead:               "死亡的宠物无法执行该操作。",
		event.RejectNotFood:               "该物品不是宠物食物。",
		event.RejectNoItem:                "背包中没有该物品。",
		event.RejectWrongFoodTier:         "该食物不适合宠物当前的饥饿程度。",
		event.RejectPetStarving:           "宠物过于饥饿，拒绝出战。",
		event.RejectInvalid:               "当前操作无效。",
		event.RejectNotEnough:             "所需物品、点数或金钱不足。",
		event.RejectMaxLevel:              "已经达到最高等级。",
		event.RejectWrongProf:             "当前职业无法使用该技能。",
		event.RejectAlreadyAlive:          "角色当前不是死亡状态。",
		event.RejectPetUnhatched:          "宠物尚未孵化。",
		event.RejectPetAlreadyHatched:     "宠物已经孵化。",
		event.RejectPetNoTrust:            "宠物信赖度为零，拒绝出战。",
		event.RejectPetRefused:            "宠物信赖度过低，本次拒绝出战。",
		event.RejectPetLearningPending:    "已有待结算的宠物技能领悟。",
		event.RejectPetCannotLearn:        "该宠物无法领悟这个技能。",
		event.RejectPetLifeSkillsFull:     "宠物的生活技能栏已满。",
		event.RejectPetFightSkillsFull:    "宠物的战斗技能栏已满。",
		event.RejectWorkOutfitMissing:     "未穿戴这项工作要求的服装。",
		event.RejectWorkToolMissing:       "未装备这项工作要求的工具。",
		event.RejectWorkToolBroken:        "工作工具已经损坏。",
		event.RejectWorkConsumableMissing: "缺少这项工作需要的消耗品。",
		event.RejectRiding:                "骑乘状态下无法使用技能。",
		event.RejectRequiresInvisible:     "该技能只能在隐身状态下使用。",
		event.RejectSkillPrerequisite:     "请先学习该技能要求的前置技能。",
	}[e.Reason]
	if text == "" {
		text = "当前无法执行该操作。"
	}
	return encodeServerText(text, serverTextChat)
}

const (
	serverTextChat       = 1
	serverTextCancelWork = 3
)

// 0x8008 displayMode=3 会先显示文本，再调用本地 PlayerController.CancelWork。
// 客户端在发送 0x1027 后会立即开始计时和动作，所以会让本次开工失效的驳回都
// 必须走这里。AlreadyWorking 是例外：服务端已有一份合法工作，不能取消它。
func encodeWorkRejected(reason event.RejectReason) []byte {
	text := "当前无法开始打工。"
	displayMode := uint8(serverTextCancelWork)
	switch reason {
	case event.RejectNoTarget:
		text = "无法开始打工：工作项目不存在。"
	case event.RejectTargetDead:
		text = "死亡状态下无法打工。"
	case event.RejectSkillNotLearned:
		text = "尚未学习对应的生活技能，无法打工。"
	case event.RejectLevelTooLow:
		text = "等级不足，无法进行这项工作。"
	case event.RejectLevelTooHigh:
		text = "当前等级超过了这项工作的等级范围。"
	case event.RejectStaminaEmpty:
		text = "耐力不足，无法打工。"
	case event.RejectAlreadyWorking:
		text = "你已经在打工了。"
		displayMode = serverTextChat
	case event.RejectStunned:
		text = "当前无法行动，不能开始打工。"
	case event.RejectWorkOutfitMissing:
		text = "未穿戴这项工作要求的服装，无法开始。"
	case event.RejectWorkToolMissing:
		text = "未装备这项工作要求的工具，无法开始。"
	case event.RejectWorkToolBroken:
		text = "工作工具已经损坏，无法开始。"
	case event.RejectWorkConsumableMissing:
		text = "缺少这项工作需要的消耗品（如鱼饵），无法开始。"
	}
	return encodeServerText(text, displayMode)
}

// 服务端主动收工也要终止客户端本地计时；只对事件本人编码，不能让旁观者的
// WorkStopped 把自己的打工状态一并取消。
func encodeWorkStopped(reason event.WorkStopReason) []byte {
	text := "打工已结束。"
	switch reason {
	case event.WorkStoppedByPlayer:
		text = "已结束打工。"
	case event.WorkStoppedByAction:
		text = "打工已中止：进行其他操作会中断打工。"
	case event.WorkStoppedNoStamina:
		text = "耐力已耗尽，打工已结束。"
	case event.WorkStoppedByDeath:
		text = "角色死亡，打工已结束。"
	case event.WorkStoppedBagFull:
		text = "背包已满，打工已结束。"
	case event.WorkStoppedByCapture:
		text = "打工已中止：开始捕捉宠物。"
	case event.WorkStoppedMissingOutfit:
		text = "工作服已脱下或更换，打工已结束。"
	case event.WorkStoppedMissingTool:
		text = "工作工具已脱下或更换，打工已结束。"
	case event.WorkStoppedToolBroken:
		text = "工作工具耐久已耗尽，打工已结束。"
	case event.WorkStoppedNoConsumable:
		text = "工作消耗品已用完（如鱼饵），打工已结束。"
	}
	return encodeServerText(text, serverTextCancelWork)
}

// encodeInventory 生成 0x8006。字段结构由 InventoryDlg.Item、
// EquipDlg.EquippedItem 的精确字段落点与八条闭合流量共同确认：
//
//	I64 money,caiyu,honor | I32 weight,maxWeight | U16 itemCount
//	itemCount × item | U8 equippedCount | equippedCount × equippedItem
//	U16 lockedFlagCount | U16 petPpPatchCount | U8 equippedQualityCount
//	| U16 fusedDescPatchCount
//
// 当前领域模型没有背包物品与宠物实例的绑定，也没有宠物 PP 或融合描述数据。
// lockedFlag 按背包项原始顺序完整补写；item.hasPet 与其余兼容尾只写已验证的空值。穿戴品质按穿戴行原始顺序逐项补写。不能拿 ItemID、
// PetDef 或其他相似字段猜一个嵌套宠物出来。
func encodeInventory(e event.InventorySnapshot) []byte {
	if len(e.Items) > 1<<16-1 || len(e.Equipped) > 1<<8-1 {
		return nil
	}
	for _, item := range e.Items {
		if !wireString(item.Name) || !wireString(item.Desc) || !wireString(item.FusedDesc) ||
			item.Sell < -1<<31 || item.Sell > 1<<31-1 ||
			item.Slot < -1 || item.Slot > 254 {
			return nil
		}
	}
	for _, item := range e.Equipped {
		if !wireString(item.Name) || !wireString(item.Desc) || item.Slot < 0 || item.Slot > 255 {
			return nil
		}
	}

	w := NewW(SCInventory).
		I64(e.Money).I64(e.Caiyu).I64(e.Honor).
		I32(e.Weight).I32(e.MaxWeight).
		U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.I32(item.BagIndex).
			I32(int32(item.Item)).
			I32(item.Count).
			U8(item.Tab).
			Str(item.Name).
			Str(item.Desc).
			I32(int32(item.Sell)).
			I32(item.CurrentDurability).
			I32(item.MaxDurability).
			I32(item.Avatar).
			U8(b2u8(item.Blocked)).
			U8(b2u8(item.Bound)).
			U8(item.Quality).
			U8(b2u8(item.Cosmetic)).
			U8(uint8(item.Slot + 1)).
			U8(b2u8(item.Pet != nil))
		if item.Pet != nil && !appendPetItem(w, item.Pet) {
			return nil
		}
	}
	w.U8(uint8(len(e.Equipped)))
	for _, item := range e.Equipped {
		w.U8(uint8(item.Slot)).
			I32(int32(item.Item)).
			Str(item.Name).
			Str(item.Desc).
			I32(item.CurrentDurability).
			I32(item.MaxDurability).
			I32(item.Avatar).
			I32(item.SoulAvatar)
	}

	// 四段都是 remaining 守卫的兼容补丁。lockedFlag 与背包项同序；petPp 暂无数据；
	// equippedQuality 是按原始穿戴顺序的索引补丁，所以计数必须与穿戴行一致。
	w.U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.U8(b2u8(item.Locked))
	}
	petCount := 0
	for _, item := range e.Items {
		if item.Pet != nil {
			petCount++
		}
	}
	w.U16(uint16(petCount))
	for i, item := range e.Items {
		if item.Pet != nil {
			w.U16(uint16(i)).I32(item.Pet.PPAiUsed).I32(item.Pet.PPAiCap).I32(0).I32(0)
		}
	}
	w.U8(uint8(len(e.Equipped)))
	for _, item := range e.Equipped {
		w.U8(item.Quality)
	}
	fusedCount := 0
	for _, item := range e.Items {
		if item.FusedDesc != "" {
			fusedCount++
		}
	}
	w.U16(uint16(fusedCount))
	for ordinal, item := range e.Items {
		if item.FusedDesc != "" {
			w.U16(uint16(ordinal)).Str(item.FusedDesc)
		}
	}
	return w.Bytes()
}

// InventoryDelta 生成 1.5.8 的 0x8069 背包增量包。items/equipped 只包含
// 发生变化的槽位；Item=0 的行表示删除原槽位。尾部四段依次是物品锁定、
// 穿戴品质、融合描述和宠物成长扩展字节。
//
// 这里总是写出扩展尾，因此同时覆盖客户端固定声明的“背包增量”和
// “扩展背包增量”，不再由 0x1091 动态门禁。
func InventoryDelta(header event.InventorySnapshot, items []event.InventoryItemView,
	equipped []event.EquippedItemView) []byte {
	if len(items) > 255 || len(equipped) > 255 {
		return nil
	}
	for _, item := range items {
		if item.BagIndex < 0 || item.Slot < -1 || item.Slot > 254 ||
			item.Sell < -1<<31 || item.Sell > 1<<31-1 ||
			!wireString(item.Name) || !wireString(item.Desc) || !wireString(item.FusedDesc) ||
			(item.Item == 0 && (item.Count != 0 || item.Name != "" || item.Desc != "")) ||
			(item.Item != 0 && item.Count <= 0) {
			return nil
		}
	}
	for _, item := range equipped {
		if item.Slot < 0 || item.Slot > 255 || !wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
	}

	w := NewW(SCInventoryDelta).
		I64(header.Money).I64(header.Caiyu).I64(header.Honor).
		I32(header.Weight).I32(header.MaxWeight).
		U16(uint16(len(items)))
	for _, item := range items {
		w.I32(item.BagIndex).I32(int32(item.Item)).I32(item.Count).U8(item.Tab).
			Str(item.Name).Str(item.Desc).I32(int32(item.Sell)).
			I32(item.CurrentDurability).I32(item.MaxDurability).I32(item.Avatar).
			U8(b2u8(item.Blocked)).U8(b2u8(item.Bound)).U8(item.Quality).
			U8(b2u8(item.Cosmetic)).U8(uint8(item.Slot + 1)).U8(b2u8(item.Pet != nil))
		if item.Pet != nil && !appendPetItem(w, item.Pet) {
			return nil
		}
	}
	w.U8(uint8(len(equipped)))
	for _, item := range equipped {
		w.U8(uint8(item.Slot)).I32(int32(item.Item)).Str(item.Name).Str(item.Desc).
			I32(item.CurrentDurability).I32(item.MaxDurability).I32(item.Avatar).I32(item.SoulAvatar)
	}

	// 1.5.8先读物品锁定(U16计数)，再读穿戴品质(U8计数)。
	w.U16(uint16(len(items)))
	for _, item := range items {
		w.U8(b2u8(item.Locked))
	}
	w.U8(uint8(len(equipped)))
	for _, item := range equipped {
		w.U8(item.Quality)
	}
	fusedCount := 0
	for _, item := range items {
		if item.FusedDesc != "" {
			fusedCount++
		}
	}
	w.U16(uint16(fusedCount))
	for ordinal, item := range items {
		if item.FusedDesc != "" {
			w.U16(uint16(ordinal)).Str(item.FusedDesc)
		}
	}
	return w.U16(0).Bytes()
}

func encodePlayerEquipmentInspected(e event.PlayerEquipmentInspected) []byte {
	if len(e.Attrs) > 1<<16-1 || len(e.Equipped) > 1<<8-1 ||
		!wireString(e.Name) || !wireString(e.Title) {
		return nil
	}
	for _, item := range e.Equipped {
		if item.Slot < 0 || item.Slot > 255 || !wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
	}
	ap := e.Appearance
	w := NewW(SCPlayerInspect).
		I32(int32(e.Target)).
		Str(e.Name).I32(e.Level).
		U8(uint8(e.Race)).U8(e.Gender).U8(e.Head).U8(e.Hair).
		Str(e.Title).I64(e.Honor).
		U16(ap.EquipView[0]).U16(ap.EquipView[1]).U16(ap.EquipView[2]).
		U16(ap.EquipView[3]).U16(ap.EquipView[4]).U16(ap.EquipView[5]).
		U8(ap.AtkVariant).U8(ap.WeaponCType).I16(int16(ap.AtkDist)).
		U16(uint16(len(e.Attrs)))
	for _, value := range e.Attrs {
		w.I32(value)
	}
	w.I32(e.MaxAtk).I32(e.MinAtk).I32(e.Def).I32(e.MAtk).I32(e.MDef).I32(e.Hit).
		U8(uint8(len(e.Equipped)))
	for _, item := range e.Equipped {
		w.U8(uint8(item.Slot)).I32(int32(item.Item)).Str(item.Name).Str(item.Desc).
			I32(item.CurrentDurability).I32(item.MaxDurability).I32(item.Avatar).I32(item.SoulAvatar)
	}
	w.U8(uint8(len(e.Equipped)))
	for _, item := range e.Equipped {
		w.U8(item.Quality)
	}
	return w.Bytes()
}

func encodeWarehouse(e event.WarehouseSnapshot) []byte {
	if e.PageCount == 0 || e.Money < 0 || len(e.Items) > 1<<16-1 {
		return nil
	}
	for _, item := range e.Items {
		if item.Item <= 0 || item.Count <= 0 || item.Slot < 0 ||
			!wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
	}
	w := NewW(SCWarehouse).U8(e.PageCount).I64(e.Money).U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.U8(item.Tab).I32(item.Slot).I32(int32(item.Item)).I32(item.Count).
			Str(item.Name).Str(item.Desc).I32(item.CurrentDurability).I32(item.MaxDurability).
			U8(b2u8(item.Blocked))
	}
	w.U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.U8(item.Quality)
	}
	return w.Bytes()
}

// WarehouseDelta 生成 1.5.8 的 0x806a。Item=0 的行按 tab/slot 删除旧格；
// 品质尾与补丁行同序，保证新增、替换和删除都不会沿用旧显示状态。
func WarehouseDelta(header event.WarehouseSnapshot, items []event.WarehouseItemView) []byte {
	if header.PageCount == 0 || header.Money < 0 || len(items) > 1<<16-1 {
		return nil
	}
	for _, item := range items {
		if item.Slot < 0 || !wireString(item.Name) || !wireString(item.Desc) ||
			(item.Item == 0 && (item.Count != 0 || item.Name != "" || item.Desc != "")) ||
			(item.Item != 0 && item.Count <= 0) {
			return nil
		}
	}
	w := NewW(SCWarehouseDelta).U8(header.PageCount).I64(header.Money).U16(uint16(len(items)))
	for _, item := range items {
		w.U8(item.Tab).I32(item.Slot).I32(int32(item.Item)).I32(item.Count).
			Str(item.Name).Str(item.Desc).I32(item.CurrentDurability).I32(item.MaxDurability).
			U8(b2u8(item.Blocked))
	}
	w.U16(uint16(len(items)))
	for _, item := range items {
		w.U8(item.Quality)
	}
	return w.Bytes()
}

// encodeWardrobe 严格对应 Win05.SetWardrobe(cap,items)：
// I32 capacity | U16 count | count × (I32 id,U8 cat,U8 worn,U8 blocked,Str name,Str desc)。
func encodeWardrobe(e event.WardrobeSnapshot) []byte {
	if e.Capacity < 0 || len(e.Items) > 1<<16-1 || e.Capacity == 0 && len(e.Items) != 0 {
		return nil
	}
	w := NewW(SCWardrobe).I32(e.Capacity).U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		if item.Item == 0 || !item.Category.Valid() || !wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
		w.I32(int32(item.Item)).U8(uint8(item.Category)).U8(b2u8(item.Worn)).
			U8(b2u8(item.Blocked)).Str(item.Name).Str(item.Desc)
	}
	return w.Bytes()
}

func encodeStallContents(e event.StallContents) []byte {
	if e.Owner == 0 || !e.Type.Valid() || !wireString(e.OwnerName) || !wireString(e.Name) ||
		len(e.Rows) > 1<<16-1 || len(e.Sales) > 1<<16-1 {
		return nil
	}
	w := NewW(SCStallContents).I32(int32(e.Owner)).Str(e.OwnerName).U8(uint8(e.Type)).
		Str(e.Name).U16(uint16(len(e.Rows)))
	for _, row := range e.Rows {
		if row.Item == 0 || row.Count <= 0 || row.Price <= 0 ||
			!wireString(row.Name) || !wireString(row.Desc) {
			return nil
		}
		w.I32(int32(row.Item)).I32(row.Count).I64(row.Price).Str(row.Name).Str(row.Desc)
	}
	w.U16(uint16(len(e.Sales)))
	for _, sale := range e.Sales {
		if sale.Time <= 0 || sale.Count <= 0 || sale.Money <= 0 || !wireString(sale.Item) {
			return nil
		}
		w.I64(sale.Time).Str(sale.Item).I32(sale.Count).I64(sale.Money)
	}
	// blocked、宠物简表、quality 和 PP 补丁按原客户端顺序闭合。
	w.U16(uint16(len(e.Rows)))
	for _, row := range e.Rows {
		w.U8(b2u8(row.Blocked))
	}
	np := 0
	for _, row := range e.Rows {
		if row.Pet != nil {
			np++
		}
	}
	w.U16(uint16(np))
	for i, row := range e.Rows {
		if row.Pet != nil {
			w.U16(uint16(i))
			if !appendPetItem(w, row.Pet) {
				return nil
			}
		}
	}
	w.U16(uint16(len(e.Rows)))
	for _, row := range e.Rows {
		w.U8(row.Quality)
	}
	w.U16(uint16(np))
	for i, row := range e.Rows {
		if row.Pet != nil {
			w.U16(uint16(i)).I32(row.Pet.PPAiUsed).I32(row.Pet.PPAiCap).I32(0).I32(0)
		}
	}
	return w.Bytes()
}

func encodeStallOwner(e event.StallOwnerChanged) []byte {
	if e.Owner == 0 || !e.Type.Valid() || !wireString(e.Name) {
		return nil
	}
	return NewW(SCStallOwner).I32(int32(e.Owner)).Str("").U8(uint8(e.Type)).Str(e.Name).
		I32(0).I32(0).Bytes()
}

// encodeShopItems 生成 0x801c：
//
//	I32 shopId | U16 count |
//	count × (I32 itemId, I32 price, Str name, Str desc) |
//	U16 blockedCount | blockedCount × U8 blocked
//
// 尾部 blocked 补丁来自客户端 ShopDlg.buyItems 的同序索引；即使全为 false
// 也写满同样条数，避免客户端沿用上一次商店的禁用状态。
func encodeShopItems(e event.ShopItems) []byte {
	if e.Shop <= 0 || len(e.Items) > 1<<16-1 {
		return nil
	}
	for _, item := range e.Items {
		if item.Item <= 0 || item.Price <= 0 || item.Price > 1<<31-1 ||
			!wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
	}
	w := NewW(SCShopItems).I32(e.Shop).U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.I32(int32(item.Item)).I32(int32(item.Price)).Str(item.Name).Str(item.Desc)
	}
	w.U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		w.U8(b2u8(item.Blocked))
	}
	return w.Bytes()
}

func encodeRackCatalog(e event.RackCatalogSnapshot) []byte {
	if len(e.Goods) > 1<<16-1 || len(e.Categories) > 255 {
		return nil
	}
	w := NewW(SCRackCatalog).U16(uint16(len(e.Goods)))
	for _, good := range e.Goods {
		if good.Item <= 0 || good.Price <= 0 || good.Price > 1<<31-1 ||
			!wireString(good.Name) || !wireString(good.Desc) || !wireString(good.Part) {
			return nil
		}
		w.I32(int32(good.Item)).Str(good.Name).Str(good.Desc).I32(good.Weight).
			U8(good.Group).I32(int32(good.Price)).U8(good.Flags).I32(good.Remain).Str(good.Part)
	}
	w.U8(uint8(len(e.Categories)))
	for _, category := range e.Categories {
		w.U8(category.ID).Str(category.Name)
	}
	w.U8(b2u8(e.Holiday)).U16(uint16(len(e.Goods)))
	for _, good := range e.Goods {
		w.U8(b2u8(good.Blocked))
	}
	return w.Bytes()
}

// encodeFittingCatalog 兼容 0x8059 的两条客户端路径：第一尾字节是旧版
// AddFittingCatalog 的 more；1.5.8 再读一个分页标志，非零时改调
// SetFittingPage，并继续读取 total/page。分页路径不重复发送长描述，客户端会把
// 可见物品批量放进 0x1098，再由 0x806e 填入详情缓存。
func encodeFittingCatalog(e event.FittingCatalogSnapshot) []byte {
	if len(e.Items) > 1<<16-1 || e.Total < 0 || e.Page < 0 || int64(e.Total) < int64(len(e.Items)) {
		return nil
	}
	w := NewW(SCFittingCatalog).U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		if item.Item <= 0 || item.Kind > 2 || item.Price <= 0 || item.Model <= 0 ||
			!wireString(item.Name) || !wireString(item.Part) || !wireString(item.Desc) {
			return nil
		}
		desc := item.Desc
		if e.Paged {
			desc = ""
		}
		w.I32(int32(item.Item)).Str(item.Name).U8(item.Kind).Str(item.Part).
			I32(item.Price).I32(item.Model).Str(desc)
	}
	if e.Paged {
		return w.U8(0).U8(1).I32(e.Total).I32(e.Page).Bytes()
	}
	return w.U8(0).Bytes()
}

func encodeFittingDetails(e event.FittingDetailsSnapshot) []byte {
	if len(e.Items) > 1<<16-1 {
		return nil
	}
	w := NewW(SCFittingDetails).U16(uint16(len(e.Items)))
	for _, item := range e.Items {
		if item.Item <= 0 || item.Price <= 0 || !wireString(item.Name) || !wireString(item.Desc) {
			return nil
		}
		w.I32(int32(item.Item)).Str(item.Name).I32(item.Price).Str(item.Desc)
	}
	return w.Bytes()
}

// encodeRackRefunds 对应 RackShopDlg.SetRefunds：
// U8 pct | U16 days | U16 count | count×RefundRow | Str cats | U8 usedOk。
func encodeRackRefunds(e event.RackRefundSnapshot) []byte {
	if e.Percent <= 0 || e.Percent > 100 || e.WindowDays <= 0 || e.WindowDays > 1<<16-1 ||
		len(e.Rows) > 1<<16-1 || !wireString(e.Categories) {
		return nil
	}
	w := NewW(SCRackRefund).U8(uint8(e.Percent)).U16(uint16(e.WindowDays)).U16(uint16(len(e.Rows)))
	for _, row := range e.Rows {
		if row.Item <= 0 || !wireString(row.Name) || row.UnitPrice <= 0 || row.Bundle <= 0 ||
			row.Day < 0 || row.Shares <= 0 || row.Have < 0 || row.Gain <= 0 || row.LeftDays < 0 {
			return nil
		}
		w.I32(int32(row.Item)).Str(row.Name).I32(row.UnitPrice).I32(row.Bundle).
			I32(row.Day).I32(row.Shares).I32(row.Have).I32(row.Gain).I32(row.LeftDays)
	}
	return w.Str(e.Categories).U8(b2u8(e.UsedOK)).Bytes()
}

func encodeFamilyStash(e event.FamilyStashSnapshot) []byte {
	if e.Capacity <= 0 || e.TakePos < 1 || e.TakePos > 10 || len(e.Entries) > 1<<16-1 {
		return nil
	}
	w := NewW(SCFamilyStash).I32(e.Capacity).U8(e.TakePos).
		U8(b2u8(e.CanTake)).U8(b2u8(e.IsLeader)).U16(uint16(len(e.Entries)))
	for _, row := range e.Entries {
		if row.Item <= 0 || row.Count <= 0 || row.CurDur < 0 || row.MaxDur < 0 ||
			row.Quality < 0 || !wireString(row.Name) || !wireString(row.Desc) || !wireString(row.DepositedBy) {
			return nil
		}
		w.I32(int32(row.Item)).I32(row.Count).Str(row.Name).Str(row.Desc).
			I32(row.CurDur).I32(row.MaxDur).I32(row.Quality).Str(row.DepositedBy)
	}
	return w.Bytes()
}

// encodeChangeSet 对应 1.5.8 Win05.SetChangeSet：
// U16 count + count×(cell,item,name,desc,curDur,maxDur,refine,quality,blocked)。
func encodeChangeSet(e event.ChangeSetSnapshot) []byte {
	if len(e.Items) > 1<<16-1 {
		return nil
	}
	w := NewW(SCChangeSet).U16(uint16(len(e.Items)))
	for _, row := range e.Items {
		if row.Cell < domain.SlotFace || row.Cell > domain.SlotTreasure || row.Item <= 0 ||
			row.CurDur < 0 || row.MaxDur < 0 || row.Refine < 0 ||
			!wireString(row.Name) || !wireString(row.Desc) {
			return nil
		}
		w.I32(int32(row.Cell)).I32(int32(row.Item)).Str(row.Name).Str(row.Desc).
			I32(row.CurDur).I32(row.MaxDur).I32(row.Refine).U8(row.Quality).U8(b2u8(row.Blocked))
	}
	return w.Bytes()
}

func encodeRepairQuote(e event.RepairQuoted) []byte {
	if e.Mode > 2 || e.TargetKind > 1 || e.Mode != 2 && e.Slot < 0 || e.Fee < 0 || e.Count < 0 ||
		e.Item < 0 || !wireString(e.Info) {
		return nil
	}
	return NewW(SCRepairQuote).
		U8(e.Mode).I32(e.Slot).I64(e.Fee).I32(e.Count).I32(int32(e.Item)).
		Str(e.Info).U8(e.TargetKind).U8(e.Tab).Bytes()
}

// wireString 检查 Str 的 U16 字节长度前缀。W.Str 是底层无错误写入器；所有
// 可失败判断必须在写包前完成，避免将超长 UTF-8 字符串截断长度后仍追加整段数据。
func wireString(s string) bool { return len(s) <= maxWireStringBytes }

// encodeAttributes 生成 0x8007：两段各自带 U16 长度的 I32 数组。
// 数组位置的业务解释由游戏层负责，协议层只忠实写出，不在这里补零或重排。
func encodeAttributes(e event.StatsChanged) []byte {
	if len(e.Current) > 1<<16-1 || len(e.Total) > 1<<16-1 {
		return nil
	}
	w := NewW(SCAttributes).U16(uint16(len(e.Current)))
	for _, value := range e.Current {
		w.I32(value)
	}
	w.U16(uint16(len(e.Total)))
	for _, value := range e.Total {
		w.I32(value)
	}
	return w.Bytes()
}

// encodeSkillSnapshot 生成 0x8015：
//
//	I32 skillPoints | U16 skillCount | skillCount × (I32 skillId, I32 level)
//
// 超过 U16 的列表必须拒绝，不能截断计数后继续追加行，否则客户端会把多出的
// 字节当成下一帧内容。
func encodeSkillSnapshot(e event.SkillSnapshot) []byte {
	if len(e.Skills) > 1<<16-1 {
		return nil
	}
	w := NewW(SCSkillSnapshot).I32(e.SkillPoints).U16(uint16(len(e.Skills)))
	for _, skill := range e.Skills {
		w.I32(int32(skill.ID)).I32(skill.Level)
	}
	if len(e.DamageBonuses) > 65535 {
		return nil
	}
	ids := make([]int, 0, len(e.DamageBonuses))
	for id := range e.DamageBonuses {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	w.U16(uint16(len(ids)))
	for _, id := range ids {
		w.I32(int32(id)).I32(e.DamageBonuses[domain.SkillID(id)])
	}
	return w.Bytes()
}

func encodeLifeSkills(e event.LifeSkillSnapshot) []byte {
	if len(e.Skills) > 1<<16-1 {
		return nil
	}
	w := NewW(SCLifeSkills).U16(uint16(len(e.Skills)))
	for _, skill := range e.Skills {
		w.I32(int32(skill.ID)).I32(skill.Level).Str(skill.Name).U8(b2u8(skill.CanLearn))
	}
	return w.Bytes()
}

func encodeCraftRecipes(e event.CraftRecipeSnapshot) []byte {
	if len(e.Recipes) > 1<<16-1 {
		return nil
	}
	w := NewW(SCCraftRecipes).U8(e.MakeType).U16(uint16(len(e.Recipes)))
	for _, recipe := range e.Recipes {
		if len(recipe.Materials) > 1<<16-1 {
			return nil
		}
		w.I32(int32(recipe.Product)).Str(recipe.Name).I32(recipe.Cost).
			U8(b2u8(recipe.CanMake)).U16(uint16(len(recipe.Materials)))
		for _, material := range recipe.Materials {
			w.I32(int32(material.Item)).I32(material.Need).I32(material.Have).Str(material.Name)
		}
	}
	return w.Bytes()
}

func encodeRefineInfo(e event.RefineInfo) []byte {
	if len(e.Materials) > 1<<16-1 {
		return nil
	}
	w := NewW(SCRefineInfo).
		U8(e.Mode).U8(e.EquipSlot).I32(e.CurrentLevel).I32(e.Cost).U8(b2u8(e.CanDo)).
		I32(e.SuccessRate).U8(e.Tab).I32(domain.ClientBagPosition(e.BagSlot)).I32(int32(e.Item)).
		U16(uint16(len(e.Materials)))
	for _, material := range e.Materials {
		w.I32(int32(material.Item)).I32(material.Need).I32(material.Have).Str(material.Name)
	}
	return w.I32(e.SafeLevel).U8(b2u8(e.FailDestroys)).Bytes()
}

func encodeSocketInfo(e event.SocketInfo) []byte {
	if len(e.Sockets) > 255 || len(e.Materials) > 1<<16-1 || len(e.SocketNames) > 255 {
		return nil
	}
	w := NewW(SCSocketInfo).U8(0).I32(int32(e.Item)).I32(int32(e.SocketCount)).U8(uint8(len(e.Sockets)))
	for _, item := range e.Sockets {
		w.I32(int32(item))
	}
	w.I32(e.NextCost).I32(e.NextSuccess).U8(b2u8(e.CanDrill)).U16(uint16(len(e.Materials)))
	for _, material := range e.Materials {
		w.I32(int32(material.Item)).I32(material.Need).I32(material.Have).Str(material.Name)
	}
	w.U8(uint8(len(e.SocketNames)))
	for _, named := range e.SocketNames {
		w.I32(int32(named.Item)).Str(named.Name)
	}
	return w.U8(e.MaxHoles).Str(e.Reason).U8(b2u8(e.FailDestroys)).Bytes()
}

func encodeAttrWashInfo(e event.AttrWashInfo) []byte {
	if len(e.Affixes) > 255 {
		return nil
	}
	w := NewW(SCAttrWashInfo).U8(e.Kind).U8(e.Tab).I32(e.Slot).
		I32(int32(e.Item)).U8(e.Quality).U8(uint8(len(e.Affixes)))
	for _, text := range e.Affixes {
		w.Str(text)
	}
	if len(e.AtMax) != len(e.Affixes) {
		return nil
	}
	w.U8(uint8(len(e.AtMax)))
	for _, v := range e.AtMax {
		w.U8(b2u8(v))
	}
	return w.Bytes()
}

// encodeHotbarSnapshot 生成 0x8016。客户端没有计数字段，而是无条件执行恰好
// 20次I32/U8/I32读取、expanded字节，再读取后10格。
func encodeHotbarSnapshot(e event.HotbarSnapshot) []byte {
	w := NewW(SCHotbar)
	for i, slot := range e.Slots {
		if i == 20 {
			w.U8(b2u8(e.Expanded))
		}
		w.I32(slot.ID).U8(slot.Kind).I32(slot.Icon)
	}
	return w.Bytes()
}

// encodePetSnapshot 生成 0x800b。
//
// 结构按最终定义逐段照发，**固定段无论有没有宠都必须发满**：
//
//	root(32 字段) → petSkill × petSkillCount → petSlotCount → petBrief × N
//	→ postPetSlots(17 字节) → 三段可选尾
//
// 先写petBound/槽位锁定/PP使用量，再写1.5.8质量筛选、技能容量与成长尾。
// 未开放的养成能力明确处于未启用状态；已支持的PP与容量来自业务事实。
//
// root 与 petBrief 的六维**顺序不同**(root 是 str/vit/agi/int/dex/spi，
// petBrief 是 str/int/vit/agi/dex/spi)，这不是笔误，照定义写。
//
// petBrief 每行包含 21 个字段。root 与 petBrief 的六维顺序不同：root 是
// str/vit/agi/int/dex/spi，petBrief 是 str/int/vit/agi/dex/spi。
// gender、habit、opened、skillCount 和 bound 均为 U8 字段，必须保留在线格式中。
//
// `ReadPetBrief` 是共用 helper,`0x8036`/`0x803c` 也按这 21 个字段写。
func encodePetSnapshot(e event.PetSnapshot) []byte {
	if len(e.Pets) > 255 || len(e.Active.Skills) > 255 {
		return nil
	}
	for _, pet := range e.Pets {
		if len(pet.Skills) > 255 {
			return nil
		}
	}
	p := e.Active
	if e.ActiveSlot == event.NoActivePet && e.SharedMountModel > 0 {
		// 仅投影真实共享坐骑模型：activeSlot仍是255，宠物栏、属性和技能保持
		// 本人真实状态。该字段在1.5.8中同时是UpdateAnim的骑乘渲染前置条件。
		p.Model = e.SharedMountModel
	}
	w := NewW(SCPetSnapshot).
		I32(p.Model).
		Str(p.Species). // petNameFallback: 只在 petNamePreferred 为空时当名字用
		Str(p.Prename).
		Str(p.Name). // petNamePreferred: 非空时就是宠物名
		U8(p.Gender).
		U8(p.Habit).
		I32(p.Level).
		I32(p.Exp).
		I32(p.ExpToNext).
		I32(p.Starve).
		I32(p.Trust).
		I32(p.FreePoints).
		I32(p.HP).
		I32(p.MaxHP).
		I32(p.MP).
		I32(p.MaxMP).
		I32(p.Base.STR).
		I32(p.Base.VIT).
		I32(p.Base.AGI).
		I32(p.Base.INT).
		I32(p.Base.DEX).
		I32(p.Base.SPI).
		I32(p.MinAtk).
		I32(p.MaxAtk).
		I32(p.Def).
		I32(p.MAtk).
		I32(p.MDef).
		I32(p.Hit).
		I32(p.ActiveSkill).
		U8(0). // ignoredCompatByte: 客户端读完立刻覆盖返回寄存器, 没有业务用途
		U8(activePetWire(e.ActiveSlot)).
		U8(uint8(len(p.Skills)))
	for _, skill := range p.Skills {
		w.I32(skill.ID).
			Str(skill.Name).
			I32(skill.Icon).
			Str(skill.Description).
			U8(b2u8(skill.Fight)).
			U8(b2u8(skill.Active)).
			I32(skill.Level).
			I32(skill.MaxLevel)
	}

	w.U8(uint8(len(e.Pets)))
	for _, s := range e.Pets {
		w.I32(s.Model).
			I32(s.Slot).
			Str(s.Name).
			Str(s.Prename).
			Str(s.Species).
			U8(s.Gender).
			U8(s.Habit).
			U8(b2u8(s.Opened)).
			I32(s.Level).
			I32(s.Base.STR).
			I32(s.Base.INT).
			I32(s.Base.VIT).
			I32(s.Base.AGI).
			I32(s.Base.DEX).
			I32(s.Base.SPI).
			I32(s.Starve).
			I32(s.Trust).
			I32(s.FreePoints).
			I32(s.UsedPoints).
			U8(uint8(len(s.Skills)))
		for _, skill := range s.Skills {
			w.Str(skill.Name).I32(skill.Level)
		}
		w.U8(b2u8(s.Bound))
	}

	// postPetSlots 固定17字节，宠爱PP显示当前出战宠物的成功次数。
	w.U8(b2u8(e.Show)).
		I32(0). // petArousal
		I32(e.Active.PPAiUsed).
		I32(0). // petPpSheng
		U8(0).  // petSanctified
		U8(b2u8(e.Ridable)).
		U8(0). // petTianmo
		U8(0)  // petShowTianmo
	{ // 完整旧尾是追加新版尾的前提。
		// 1.5.8 可选尾：petBound、lockedFlagCount、ppCount，然后按宠物栏顺序
		// 每宠四个I32。计数和位置来自已审计的0x800b客户端解析器。
		lockedCount := 0
		for i, pet := range e.Pets {
			if pet.Locked {
				lockedCount = i + 1
			}
		}
		w.U8(b2u8(e.ActiveBound)).U8(uint8(lockedCount))
		for _, pet := range e.Pets[:lockedCount] {
			w.U8(b2u8(pet.Locked))
		}
		w.U8(uint8(len(e.Pets)))
		for _, pet := range e.Pets {
			w.I32(pet.PPAiUsed).I32(e.PPAiCap).I32(0).I32(0)
		}
	}
	// 质量筛选/成长玩法未开放，其权威值为未启用；容量读取数据库。
	w.U8(0).U8(uint8(e.FightCap)).U8(uint8(e.LifeCap)).U8(0).U8(uint8(len(e.Pets)))
	for range e.Pets {
		w.U8(0)
	}
	return w.Bytes()
}

// activePetWire 把领域里的槽位号翻成线上的 U8。没有出战宠时是哨兵 255。
func activePetWire(slot int) uint8 {
	if slot < 0 || slot >= noActivePetWire {
		return noActivePetWire
	}
	return uint8(slot)
}

// encodeBuffSnapshot 生成 0x8017 的完整本人 Buff/Debuff 快照。
func encodeBuffSnapshot(e event.BuffSnapshot) []byte {
	if len(e.Buffs) > 1<<16-1 {
		return nil
	}
	w := NewW(SCBuffSnapshot).U16(uint16(len(e.Buffs)))
	for _, buff := range e.Buffs {
		w.I32(buff.SkillID).I32(buff.Icon).I32(buff.RemainSec).
			Str(buff.Name).Str(buff.Desc).
			U8(b2u8(buff.Beneficial)).U8(buff.PositionMode).
			U8(b2u8(buff.Invisible)).U8(buff.Immobile)
	}
	return w.U8(b2u8(e.DoubleExp)).Bytes()
}

func encodeEntityStatus(e event.EntityStatusSnapshot) []byte {
	if len(e.Icons) > 255 {
		return nil
	}
	w := NewW(SCEntityStatus).I32(int32(e.Who)).U8(uint8(len(e.Icons)))
	for _, st := range e.Icons {
		w.I32(st.Status).I32(st.Icon).U8(b2u8(st.Beneficial)).I32(st.RemainMS)
	}
	return w.Bytes()
}

// encodeStatusEffect 生成 0x801b：实体号、特效名、挂点模式、剩余毫秒数。
// 客户端以特效名为键；同名且 duration=0 会把已有特效的到期时间推进到当前帧。
func encodeStatusEffect(e event.EntityStatusEffect) []byte {
	if e.Who <= 0 || e.Effect == "" || !wireString(e.Effect) || e.DurationMS < 0 {
		return nil
	}
	return NewW(SCStatusEffect).
		I32(int32(e.Who)).
		Str(e.Effect).
		U8(e.PositionMode).
		I32(e.DurationMS).
		Bytes()
}

// encodeTeleport 生成 0x803a：I32 mapId, I32 x, I32 y。客户端随后把坐标
// 有符号转换成 Single 交给 OnTeleport；wire 本身不是 F32。
func encodeTeleport(e event.Teleported) []byte {
	return NewW(SCTeleport).
		I32(e.To.MapID).
		I32(int32(e.To.X)).
		I32(int32(e.To.Y)).
		Bytes()
}

// encodeTransportDestinations 生成 0x8049：
// I32 transList | U16 count | count × (Str name, I32 mapId, I32 x, I32 y, I32 price)。
func encodeTransportDestinations(e event.TransportDestinations) []byte {
	if e.List <= 0 || len(e.Destinations) > 1<<16-1 {
		return nil
	}
	w := NewW(SCTransportList).I32(e.List).U16(uint16(len(e.Destinations)))
	for _, d := range e.Destinations {
		if d.Name == "" || !wireString(d.Name) || d.Pos.MapID <= 0 || d.Price < 0 {
			return nil
		}
		w.Str(d.Name).
			I32(d.Pos.MapID).I32(int32(d.Pos.X)).I32(int32(d.Pos.Y)).I32(d.Price)
	}
	return w.Bytes()
}

// encodeAppearance 生成 0x800a 的完整 28 字节载荷。
//
// 原服两次穿戴样本(seq 1778/1810)都不是只写 fixed root，而是固定带：
// aura、soulFx、titleFx、glowMode、glowUnlock 都来自角色公开外观状态；没有装备
// 或称号特效时其权威值自然为空，而不是由协议层覆盖成固定占位。
func encodeAppearance(e event.AppearanceChanged) []byte {
	v := e.Appearance.EquipView
	return NewW(SCAppearance).
		I32(int32(e.Who)).
		U16(v[0]).U16(v[1]).U16(v[2]).U16(v[3]).U16(v[4]).U16(v[5]).
		U8(e.Appearance.AtkVariant).
		U8(e.Appearance.WeaponCType).
		U16(e.Appearance.AtkDist).
		Str(e.Appearance.Aura).
		Str(e.Appearance.SoulFX).
		Str(e.Appearance.TitleFX).
		U8(glowModeOf(e.Appearance)).
		U8(b2u8(e.Appearance.GlowUnlocked)).
		Bytes()
}

func glowModeOf(a domain.Appearance) uint8 {
	if !a.GlowModeKnown {
		// 旧的内存构造没有“已设置”标记；保持原服样本的默认模式 1。
		return 1
	}
	return a.GlowMode
}

func encodePlayerDied(e event.EntityDied) []byte {
	var options uint8
	if e.AllowItemRevive {
		options |= 1 << 0
	}
	if e.AllowSkillRevive {
		options |= 1 << 1
	}
	return encodePlayerLife(e.ID, false, options)
}

// encodePlayerLife 生成 0x801a：alive、复活入口位、玩家实体 id。
//
// 首字节的线上语义与旧文档中的 dead 命名相反：0 才会让正式客户端
// 执行 PlayDie 并打开 ReviveDlg，1 会执行 Revive 并关闭复活框。
// options bit0=道具复活，bit1=技能复活；0 仍保留客户端默认复活入口。
func encodePlayerLife(id domain.EntityID, alive bool, options uint8) []byte {
	return NewW(SCPlayerLife).U8(b2u8(alive)).U8(options).I32(int32(id)).Bytes()
}

// SCNpcTasks 是「这个 NPC 身上有什么任务」的回应(对上行 0x1032)。
const SCNpcTasks = 0x8028

// encodeNpcTasks 生成 0x8028。
//
// 布局如下:
//
//	Str NPC名 | U16 条数 | 每条: I32 任务号 | U8 状态 | Str 标题 | Str 描述 | Str 奖励
//
// 循环体里那五个字段与客户端 `Offer` 类的字段(id/state/title/desc/award)
// 一一对应 —— 是对上号的, 不是猜的。
func encodeNpcTasks(e event.NpcTasks) []byte {
	if len(e.Offers) > 1<<16-1 {
		return nil
	}
	w := NewW(SCNpcTasks).Str(e.NPC).U16(uint16(len(e.Offers)))
	for _, o := range e.Offers {
		w.I32(int32(o.ID)).U8(uint8(o.State)).
			Str(o.Title).Str(o.Desc).Str(o.Award)
	}
	return w.Bytes()
}

// encodeNpcChatter 生成 0x804b：
//
//	U8 enabled | I32 minGap | I32 maxGap | I32 showSec | U16 lineCount
//	每条: Str npcName | Str role | Str text
func encodeNpcChatter(e event.NpcChatter) []byte {
	lines := make([]event.NpcChatterLine, 0, len(e.Lines))
	for _, line := range e.Lines {
		if line.NPC == "" || line.Text == "" || !wireString(line.NPC) ||
			!wireString(string(line.Role)) || !wireString(line.Text) {
			continue
		}
		if len(lines) == 1<<16-1 {
			return nil
		}
		lines = append(lines, line)
	}
	w := NewW(SCNpcChatter).
		U8(b2u8(e.Enabled)).I32(e.MinGap).I32(e.MaxGap).I32(e.ShowSec).
		U16(uint16(len(lines)))
	for _, line := range lines {
		w.Str(line.NPC).Str(string(line.Role)).Str(line.Text)
	}
	return w.Bytes()
}

// SCActiveQuests 是玩家的任务本。
const SCActiveQuests = 0x8027

// encodeActiveQuests 生成 0x8027。
//
// 布局分为两段:
//
//	U16 条数 | 每条: I32 任务号 I32 剩余秒 I32 已完成 I32 需要量
//	                 Str 正文 Str 进度 U8 可交 Str 交给谁
//	U16 条数 | 每条: I32 已完成的任务号
//
// 两段各是什么, **是查出来的不是猜的**: handler 收完两段之后尾调用
// `TaskClient.SetState(第一段, 第二段, 0)`, 而 TaskClient 的字段是
//
//	Active   List<ActiveTask>        ← 第一段, 八个字段与 ActiveTask 一一对应
//	Done     HashSet<System.Int32>   ← 第二段
//
// 找一段数据是什么的通法就是这个: 顺着 handler 最后调的那个方法, 去看
// 接收方类的字段名。只盯着读取序列看只能知道"是一串 int32"。
func encodeActiveQuests(e event.ActiveQuests) []byte {
	if len(e.List) > 1<<16-1 || len(e.Done) > 1<<16-1 {
		return nil
	}
	w := NewW(SCActiveQuests).U16(uint16(len(e.List)))
	for _, q := range e.List {
		w.I32(int32(q.ID)).I32(q.RemainSec).I32(q.Step).I32(q.Total).
			Str(q.Text).Str(q.Progress).U8(b2u8(q.Ready)).Str(q.To)
	}
	w.U16(uint16(len(e.Done)))
	for _, id := range e.Done {
		w.I32(id)
	}
	return w.Bytes()
}

// encodeTitleSnapshot 生成 0x804c：Str 当前称号 | U8 数量 | Str[] 已拥有称号。
func encodeTitleSnapshot(e event.TitleSnapshot) []byte {
	if len(e.Owned) > 1<<8-1 || !wireString(e.Current) {
		return nil
	}
	w := NewW(SCTitleSnapshot).Str(e.Current).U8(uint8(len(e.Owned)))
	for _, title := range e.Owned {
		if title == "" || !wireString(title) {
			return nil
		}
		w.Str(title)
	}
	return w.Bytes()
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func wireKind(k domain.EntityKind) uint8 {
	switch k {
	case domain.KindPet:
		return wireKindPet
	case domain.KindTrap:
		return wireKindTrap
	case domain.KindMonster:
		return wireKindMonster
	case domain.KindDrop:
		return wireKindDrop
	}
	return wireKindMonster
}

// wireHitFlag 把服务端伤害标志映射成客户端的表现位掩码。
//
// bit0=暴击、bit1=未命中、bit2=致死，可以组合；例如致死暴击必须发 5，
// 否则客户端只播放死亡表现而不会播放暴击音效和暴击飘字。首击和魔法是
// 服务端结算状态，不占客户端表现位。
func wireHitFlag(f event.DamageFlag) uint8 {
	var out uint8
	if f.Has(event.DamageCrit) {
		out |= wireHitCrit
	}
	if f.Has(event.DamageMiss) {
		out |= wireHitMiss
	}
	if f.Has(event.DamageFatal) {
		out |= wireHitFatal
	}
	return out
}
