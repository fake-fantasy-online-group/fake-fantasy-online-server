// Package store 定义持久化接口, 隔离领域层与具体存储实现。
//
// 设计(见 docs/架构设计.md §3.4): 领域层只依赖本接口, 不知道背后是 PostgreSQL 还是别的。
// 这样"不丢档、可事务、将来换/扩存储"都不影响上层。A0 提供内存实现让骨架可跑可测,
// PostgreSQL 实现接同一接口随后接入。
package store

import (
	"context"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Account 是账号的存储视图(鉴权用最小集; 密码校验细节由 session 层定, 这里存哈希)。
type Account struct {
	Caiyu       int64 // 用户共用的彩玉余额；由在线会话的快照事务写回。
	ID          int64
	Username    string
	PassHash    string // 密码哈希(bcrypt/argon2), 明文不落库
	SecHash     string // 安全码哈希；用于后续密码找回，明文不落库
	Banned      bool
	BannedUntil time.Time // 定时封禁截止；零值表示没有定时封禁
	GMLevel     GMLevel   // 账号级 GM 权限；角色登录后继承，0 表示普通玩家
}

// TimedBanStore 是定时封禁的可选存储能力。保持主 Store 接口稳定，避免与账号
// 封禁无关的测试替身和外围实现被迫感知该功能。
type TimedBanStore interface {
	SetAccountBannedUntil(ctx context.Context, accountID int64, until time.Time) error
}

// CharacterRenamer 是角色改名所需的窄写能力。单列接口避免让与改名无关的
// Store 测试替身被迫实现；生产 PostgreSQL 和内存存储都提供它。
type CharacterRenamer interface {
	RenameCharacter(ctx context.Context, charID int64, newName string) error
}

// AccountSecurityUpdater 是修改账号安全码所需的窄写能力。单列接口避免让与
// 账号安全无关的 Store 替身被迫扩展；生产 PostgreSQL 与 Memory 都实现它。
type AccountSecurityUpdater interface {
	UpdateAccountSecurityHash(ctx context.Context, accountID int64, securityHash string) error
}

// MailStore 是邮件模块需要的持久化能力。发送/领取同时接收角色快照，具体实现
// 必须把背包物权与邮件状态放在同一个数据库事务里。
type MailStore interface {
	LoadMails(ctx context.Context, recipientID int64, rule domain.MailRule) ([]domain.Mail, error)
	SendMail(ctx context.Context, sender domain.Snapshot, draft domain.MailDraft, rule domain.MailRule) (int64, error)
	ClaimMail(ctx context.Context, recipient domain.Snapshot, mailID int64) error
	MarkMailRead(ctx context.Context, recipientID, mailID int64) error
	DeleteMail(ctx context.Context, recipientID, mailID int64) error
	ReturnMail(ctx context.Context, recipientID, mailID int64, rule domain.MailRule) error
}

// FamilyStore 是家族包需要的持久化能力。家族关系跨场景且必须跨登录保留，
// 因此不放场景实体；所有人事操作由 PostgreSQL 事务保证名额和职位一致。
type FamilyStore interface {
	LoadFamily(ctx context.Context, charID int64) (*domain.Family, error)
	BrowseFamilies(ctx context.Context) ([]domain.FamilyBrowseEntry, error)
	FamilyPositions(ctx context.Context, charID int64) ([]domain.FamilyPosition, error)
	FamilyByName(ctx context.Context, name string) (familyID int64, leader domain.CharID, resist bool, err error)
	CreateFamily(ctx context.Context, charID int64, name, proclaim string) error
	JoinFamily(ctx context.Context, charID, familyID int64, invited bool) error
	LeaveFamily(ctx context.Context, charID int64) error
	SetFamilyProclaim(ctx context.Context, actorID int64, proclaim string) error
	SetFamilyResist(ctx context.Context, actorID int64, resist bool) error
	SetFamilyMemberPosition(ctx context.Context, actorID int64, targetName string, position domain.FamilyPositionID) (domain.CharID, error)
	TransferFamilyLeader(ctx context.Context, actorID int64, targetName string) (domain.CharID, error)
	KickFamilyMember(ctx context.Context, actorID int64, targetName string) (domain.CharID, error)
	RenameFamilyPosition(ctx context.Context, actorID int64, position domain.FamilyPositionID, name string) error
	SendFamilyMail(ctx context.Context, actorID int64, senderName, title, body string, expiresAt time.Time) ([]domain.CharID, error)
	FamilyMemberIDs(ctx context.Context, charID int64) ([]domain.CharID, error)
}

type FamilyStashReader interface {
	LoadFamilyStash(ctx context.Context, charID int64) (*domain.FamilyStash, error)
}

// FamilyStashWriter 只在 Store.WithTx 内使用，共享仓库物权与玩家背包快照同事务。
type FamilyStashWriter interface {
	DepositFamilyStash(ctx context.Context, charID int64, stack domain.Stack, by string) error
	WithdrawFamilyStash(ctx context.Context, charID int64, index int32, expected domain.Stack) error
	SetFamilyStashTakePosition(ctx context.Context, charID int64, position domain.FamilyPositionID) error
}

type ApprenticeStore interface {
	LoadApprenticeSnapshot(ctx context.Context, charID int64) (domain.ApprenticeSnapshot, error)
	CheckApprenticeship(ctx context.Context, masterID, apprenticeID int64) error
	CreateApprenticeship(ctx context.Context, masterID, apprenticeID int64) error
	DeleteApprenticeship(ctx context.Context, masterID, apprenticeID int64) error
}

// BlockStore 是 1.5.8 屏蔽名单需要的窄持久化能力。屏蔽是单向关系：
// A 屏蔽 B 不会自动让 B 屏蔽 A。
type BlockStore interface {
	LoadBlocks(ctx context.Context, charID int64) ([]domain.BlockEntry, error)
	UpsertBlock(ctx context.Context, charID, blockedCharID int64, scope uint8) error
	RemoveBlock(ctx context.Context, charID, blockedCharID int64) error
}

type CommerceStore interface {
	RecordDeposit(ctx context.Context, charID int64, chain string, amount int32, caiyu int64) (int64, error)
}

// RackRefundReader 是货架回收页的只读购买流水入口。
type RackRefundReader interface {
	LoadRackRefundRule(ctx context.Context) (domain.RackRefundRule, error)
	LoadRackRefundOffers(ctx context.Context, charID int64, rule domain.RackRefundRule) ([]domain.RackRefundOffer, error)
}

// RackLedgerWriter 只在 WriteBack 的 Store.WithTx 回调中使用，使角色快照与
// 购买/退货流水处于同一个 PostgreSQL 事务。
type RackLedgerWriter interface {
	RecordRackPurchase(ctx context.Context, charID int64, item domain.ItemID,
		unitPrice, bundle, shares int32, purchasedAt time.Time) error
	ConsumeRackRefund(ctx context.Context, charID int64, item domain.ItemID,
		unitPrice, bundle, shares int32, purchasedAfter time.Time) error
}

// GMLevel 是账号的 GM 权限等级。数值沿用旧客户端 gmcommand.lua 的四级命名，
// 但权限判定只发生在服务端；客户端是否显示某个入口不能成为鉴权依据。
type GMLevel int16

const (
	GMNone GMLevel = iota
	GMApp
	GMWizard
	GMArch
	GMAdmin
)

func (l GMLevel) String() string {
	switch l {
	case GMApp:
		return "APP"
	case GMWizard:
		return "WIZARD"
	case GMArch:
		return "ARCH"
	case GMAdmin:
		return "ADMIN"
	default:
		return "PLAYER"
	}
}

// Store 是持久化总接口。方法都带 context, 便于超时/取消/追踪。
//
// 一致性约定: 写方法返回 nil 即表示已持久化成功(不丢档的边界)。涉及多实体的原子操作
// (交易/掉落归属)走 Tx。
type Store interface {
	// ── 账号 ──
	AccountByName(ctx context.Context, username string) (*Account, error)
	CreateAccount(ctx context.Context, username, passHash string) (*Account, error)
	CreateRegisteredAccount(ctx context.Context, username, passHash, secHash string) (*Account, error)
	UpdateAccountPasswordHash(ctx context.Context, accountID int64, passHash string) error
	// CompareAndSwapAccountCredentials changes secrets only while both verified hashes
	// still match, so concurrent recovery/change requests cannot overwrite each other.
	CompareAndSwapAccountCredentials(ctx context.Context, accountID int64, oldPass, oldSec, newPass, newSec string) (bool, error)
	SetAccountBanned(ctx context.Context, accountID int64, banned bool) error

	// ── 角色 ──
	CharsByAccount(ctx context.Context, accountID int64) ([]*domain.Character, error)
	CharByName(ctx context.Context, name string) (*domain.Character, error)
	CreateChar(ctx context.Context, c *domain.Character) error // 成功后 c.ID 被填充
	SaveChar(ctx context.Context, c *domain.Character) error   // 只存角色行(登录时间之类)
	// SaveSnapshot 把角色行与背包**在一个事务里**一起存。
	//
	// 分开存会出现"经验存了、物品没存"这种崩溃后的半截状态 ——
	// 那正是玩家会拿去申诉的那一类。
	SaveSnapshot(ctx context.Context, s domain.Snapshot) error
	// LoadBag 读一个角色的背包。角色没有背包行时返回一个空背包, 不是 nil。
	LoadBag(ctx context.Context, charID int64, slots int) (*domain.Bag, error)
	// LoadWarehouse 读取角色独立个人仓库；无存档行时按规则创建初始空仓库。
	LoadWarehouse(ctx context.Context, charID int64, rule domain.WarehouseRule) (*domain.Warehouse, error)
	// LoadWardrobe 读取角色已永久收藏的变装卡与穿戴选择。
	LoadWardrobe(ctx context.Context, charID int64, rule domain.WardrobeRule) (*domain.Wardrobe, error)
	// LoadStall 读取崩溃前尚未归还的摆摊托管物权；正常登录会先恢复进背包再入场。
	LoadStall(ctx context.Context, charID int64) (*domain.Stall, error)
	// LoadEquips 读一个角色身上穿的。没有时返回空的一套, 不是 nil。
	LoadEquips(ctx context.Context, charID int64) (*domain.EquipSet, error)
	// LoadChangeSet 读取 1.5.8 快速换装面板托管的备用装备。
	LoadChangeSet(ctx context.Context, charID int64) (*domain.ChangeSet, error)
	// LoadSkills 读一个角色学会的技能。没有时返回空表, 不是 nil。
	LoadSkills(ctx context.Context, charID int64) (domain.Learned, error)
	// LoadQuests 读一个角色的任务本。没有时返回空表, 不是 nil。
	LoadQuests(ctx context.Context, charID int64) (domain.QuestLog, error)
	// LoadPets 读一个角色养的宠物。没有时返回 nil。
	LoadPets(ctx context.Context, charID int64) ([]domain.PetInstance, error)
	// LoadSeenTips/MarkTipSeen 构成 NewbieTipBox 的跨登录去重状态。
	// MarkTipSeen 必须幂等：客户端可能因重发或同帧重复触发上报同一编号。
	LoadSeenTips(ctx context.Context, charID int64) ([]int32, error)
	MarkTipSeen(ctx context.Context, charID int64, tipID int32) error

	// ── 好友。双向存两行, 加/删都是同事务写两条 ──
	LoadFriends(ctx context.Context, charID int64) ([]domain.Friend, error)
	AddFriend(ctx context.Context, a, b int64) error
	RemoveFriend(ctx context.Context, a, b int64) error
	SetFriendRemark(ctx context.Context, charID, friendID int64, remark string) error
	CountFriends(ctx context.Context, charID int64) (int, error)
	DeleteChar(ctx context.Context, id int64) error

	// ── 事务 ──
	// WithTx 在一个事务内执行 fn, fn 返回 error 则回滚。用于跨实体原子操作。
	WithTx(ctx context.Context, fn func(Store) error) error

	// Close 释放资源。
	Close() error
}

// 哨兵错误由具体实现映射到 domain 层错误, 上层只认 domain 错误。
