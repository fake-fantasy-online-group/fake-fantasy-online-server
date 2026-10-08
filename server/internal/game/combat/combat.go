// Package combat 是战斗结算 —— 命中、伤害、暴击。
//
// **本包的每一个数字都必须能在 docs/战斗公式.md 里找到出处。**
// 那份文档是考据出来的（抓包 + 客户端表 + 贴吧六路印证），不是拍的；
// 改这里的常量之前先改文档，并且把置信度标注一起改掉。
//
// 本作与大多数 MMO 最大的不同：**防御走闪避，不减伤**。
// 这一条由项目方确认，且有一条硬反证 ——「武器上的 7% 概率无视对方防御」
// 在玩家实测里等于「7% 绝对命中率」；如果防御是减伤，那个词缀就该是「无视减伤」。
//
// 分层：本包只认 domain / entity / event，**不认识协议**。
// 结算出来的是「造成了 42 点伤害」这个事实，不是 0x8011 的十七个字节。
package combat

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Rand 是本包要的全部随机能力。抽成接口是为了让战斗**可复现** ——
// 出了 bug 能用同一个种子重放，测试也不必迁就随机数。
// *math/rand.Rand 直接满足它。
type Rand interface {
	Intn(n int) int
}

const (
	// CritMultiplier 暴击伤害倍率。**[确认]** 项目方明确：固定 2 倍，没有等级差修正。
	CritMultiplier = 2

	// critScale 是暴击率的单位。ov_card_entry 的 attr23 是**万分比**，
	// 10000 = 100%（docs/战斗公式.md〇.2：「爆击率% = Σattr23 / 100」）。
	critScale = 10000

	// MinDamage 命中之后至少掉 1 点血。**[实证]** 公式文档：命中伤害 = max(1, …)。
	// 没有这条的话，高抗性会把伤害压成 0，客户端表现成「打了但没反应」。
	MinDamage = 1

	// MagicDefenseReductionNumerator/Denominator 是魔防的固定减伤换算：
	// 每 1 点魔防减少 0.63 点魔法伤害。用整数分数保存，避免浮点累计误差。
	MagicDefenseReductionNumerator   = 63
	MagicDefenseReductionDenominator = 100

	// 以下两个常量只供旧比例模型的兼容计算使用；正式战斗已经改为每点魔防
	// 固定减伤 0.63，不再读取攻击者等级，也不再使用百分比上限。
	// magicDefLevelFactor 是旧公式分母里的 X = 攻方等级 × 10。
	//
	// **[推导·待实测]** 这不是查到的，是从「物理/魔法期望对称」推出来的，过了三项检验：
	//   数量级   60 级魔法减伤 22~28%，与物理侧同量级（取 ×1 或 ×2 都强到上限形同虚设）
	//   自洽     60×10 = 600，正好是贴吧反复出现的「命中上 600 飞前基本无 MISS」那个阈值，
	//            也就是说「等级×10」就是该等级的典型命中值，魔法侧借用同一基准
	//   不矛盾   93 级「100 点魔防抵挡 63 点魔法伤害」反推出的伤害量级吻合
	// 见 docs/战斗公式.md 第二节。抓包实测之前，这个数是本包置信度最低的一个。
	magicDefLevelFactor = 10

	// MaxMagicMitigation 是旧比例模型的魔防减伤上限 75%。
	// 贴吧 8621738851 明确「按比例转换的值，75% 上限」。
	//
	// 60 级以内够不着：要触到 75% 需要魔防 = 3 × 等级 × 10 = 1800。
	MaxMagicMitigation = 0.75
)

// HitChance 返回长期命中率，供测试与数值调试用。**结算不走这里**，走步进器。
//
//	命中率 = 攻方命中 / (攻方命中 + 守方防御)
//
// **[单源·四约束]** 四条玩家实测约束定出这个形状，且**不需要任何上下限**：
// 永不满命中（防御>0 时比值恒 <1）、命中≈防御时恰好 50%、边际递减、高防御难打。
func HitChance(hit, def int32) float64 {
	if def <= 0 {
		return 1 // 没有闪避可言
	}
	if hit <= 0 {
		return 0
	}
	return float64(hit) / float64(hit+def)
}

// HitTracker 是命中的**步进器**，不是掷骰子。
//
// 由项目方的游戏体感提出：「刚开始、包括很长一段练级时光，都是很稳定的，
// 打一下 miss 一下」—— 纯随机产生不出这种手感。
//
//	每次攻击：累加器 += 攻方命中
//	         若 累加器 >= 命中 + 防御，则命中并扣掉 (命中+防御)
//
// **长期命中率与 命中/(命中+防御) 完全相同**，差别只在分布：
// 50% 时步进器就是精确的「打一下 miss 一下」，纯随机会出现 5 连中或 3 连 miss。
//
// **[推导·待实测]** 验法在 docs/战斗公式.md 第 1.5 步：连打 50~100 下，
// 看 miss **间隔的方差**——不要看命中率，两种模型的命中率是一样的，区分不了。
//
// 每个攻击者一个，只在场景 goroutine 内访问，无需加锁。
type HitTracker struct {
	target domain.EntityID
	acc    int32
}

// Target 返回当前交战对象。0 表示没在打谁。
func (t *HitTracker) Target() domain.EntityID { return t.target }

// Engage 把矛头转向 target，返回这是不是本次交战的第一击。
//
// 换目标时给累加器一个 [0, track) 的随机起点 —— 否则每次开打的命中序列
// 完全一样，玩家能数着拍子躲。track = 命中 + 防御。
func (t *HitTracker) Engage(target domain.EntityID, track int32, rng Rand) (opening bool) {
	if t.target == target {
		return false
	}
	t.target = target
	t.acc = 0
	if track > 0 && rng != nil {
		t.acc = int32(rng.Intn(int(track)))
	}
	return true
}

// Roll 推进一步并判定这一击中没中。调用前必须先 Engage。
//
// 注意「必中」的三种情况（暴击 / 法系技能 /「无视防御」词缀）**不该调这里** ——
// 它们跳过命中判定，也就不该消耗累加器的步进，否则等于白拿一次命中额度。
func (t *HitTracker) Roll(hit, def int32) bool {
	if def <= 0 {
		return true // 没有闪避可言
	}
	if hit <= 0 {
		return false // 一点命中都没有
	}
	track := hit + def
	t.acc += hit
	if t.acc >= track {
		t.acc -= track
		return true
	}
	return false
}

// Reset 清空交战状态。目标死了或走出视野时调。
func (t *HitTracker) Reset() { t.target, t.acc = 0, 0 }
