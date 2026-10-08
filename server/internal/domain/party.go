package domain

// 组队。
//
// ⚠️ **原始数据里没有组队配置表。** 翻遍 139 张表，与组队沾边的只有：
//
//	ov_teamcovdesc            3 行，是"协助契约/玫瑰契约/七夕契约"三个道具名
//	ov_yhfpworld_member_entry 团队副本的人数档（10 人，60~150 级，在 MVP 之外）
//
// 所以下面的人数上限、分经验规则、等级差限制**全部是服务端定的**，
// 每一条都写了理由。将来挖到原版数值，改的是这一个文件里的常数。
//
// 但组队要做的**理由是实证的**：`ov_skilldesc.target_team = 1` 的 217 行技能
// **全部属于药师**（prof=4，150 行里 80 行），其余四职业一个都没有。
// 治愈术 / 复法术 / 回春术 / 群疗术全是对队友放的 ——
// 没有队伍，药师就是半个职业。

// PartyID 标识一支队伍。0 表示没组队。
//
// **这是个值，不是指针。** 场景里判断"是不是队友"就是比两个 PartyID，
// 不查表、不加锁、不跨 goroutine —— 队伍名册在 game/party 里由别人管，
// 场景只认识这个号。这样场景 Actor 模型一点没被破坏。
type PartyID uint32

// MaxPartySize 一支队伍最多几个人。
//
// **服务端定。** 团队副本那张表写的是 10 人，但那是"团队"不是"队伍"，
// 而且是 60 级以上的内容，不在 MVP 里。
// 定 5 的理由：五个职业各一个正好凑一队，这是这类游戏最常见的编制；
// 再多就得做团队框（多个小队的层级结构），那是另一套东西。
const MaxPartySize = 5

// PartyExpBonusPerExtra 每多一个队友，队伍总经验多几个百分点。
//
// **服务端定。** 平均分的话组队一定亏：五个人分一份，每人只有五分之一，
// 而怪并没有因为人多而变多。给个加成让"组队打得快"能抵掉"分得少"。
// 10% 是个保守值 —— 五人队总经验 1.4 倍，人均 0.28 份，
// 仍然低于单人，所以组队是为了打得动更硬的怪，不是为了刷得更快。
const PartyExpBonusPerExtra = 10

// PartyExpLevelGap 能分到经验的最大等级差。
//
// **服务端定，而且这一条最需要。** 没有它，一个满级号带一个 1 级号站着，
// 后者几分钟就能到几十级 —— 练级曲线直接作废。
// 20 级这个数字选得比较松：同一张图的怪跨度本来就有十几级，
// 卡太死会让"朋友帮忙打个副本"变得不可能。
const PartyExpLevelGap = 20

// PartyExpShare 算一次击杀在队伍里怎么分。
//
// n 是**在场且够得着**的队员数（含击杀者）。返回每人拿多少。
//
// 规则：总经验 = 基础 × (1 + 10% × (n−1))，然后平分。
// 先乘后除而不是先除后乘 —— 整数除法先做会把零头全部吃掉，
// 五人队每人少拿的那点乘以一天的击杀次数不是小数目。
func PartyExpShare(base int64, n int) int64 {
	if n <= 1 {
		return base
	}
	if n > MaxPartySize {
		n = MaxPartySize
	}
	total := base * int64(100+PartyExpBonusPerExtra*(n-1)) / 100
	return total / int64(n)
}

// CanShareExp 报告某个队员够不够格分这次经验。
//
// killerLevel 是击杀者等级，memberLevel 是这个队员的等级。
// **只卡低的一侧**：高等级队员帮忙打低级怪拿不到多少经验是自然的（怪本来就不值钱），
// 而低等级被带飞才是要防的事。
func CanShareExp(killerLevel, memberLevel int32) bool {
	return memberLevel+PartyExpLevelGap >= killerLevel
}

// PartyReject 是组队操作被拒的原因。
type PartyReject uint8

const (
	PartyOK PartyReject = iota
	PartyFull
	PartyAlreadyIn    // 已经在队伍里了
	PartyNotIn        // 不在任何队伍里
	PartyNotLeader    // 这个操作只有队长能做
	PartyNoSuchMember // 队里没这个人
	PartySelfTarget   // 不能对自己做（邀请自己、踢自己）
)
