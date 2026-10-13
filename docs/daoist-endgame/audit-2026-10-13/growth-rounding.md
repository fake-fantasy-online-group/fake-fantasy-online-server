# 高等级整数余量累计：实施与验证

更新：2026-10-13 UTC。范围：已批准的六个省蓝/冷却例外，以及燃烧强化的原持续伤害。保留原连续曲线、原低等级结果、至少1点蓝/100毫秒冷却；不增加属性词条或无关补偿。本文记录云端验证；未操作用户本机、未执行Unity构建。

## 本次确实修好的问题

原本冲锋及秘术：幻影100→101，单次真实扣蓝、实际69帧冷却均不变；燃烧自身1或5级时，燃烧强化100→101每跳也均为54。它们不是仅说明精度问题，燃烧亦不是审计器漏字段。

现保留这些连续公式的小数：蓝耗/冷却累计此前多付的余量，燃烧在每个实际状态实例内累计不足1点的伤害。没有退款、随机舍入，也没有更改世界100ms帧精度。

真实生产入口测试结果：

| 样本 | 等级100 | 等级101 |
|---|---:|---:|
| 冲锋100次成功施法总扣蓝 | 828 | 822 |
| 秘术：幻影100次成功施法总扣蓝 | 4690 | 4658 |
| 100个实际冷却周期合计帧数 | 6897 | 6850 |
| 燃烧1或5级，强化100/101级，10次实际跳伤总量 | 540 | 544 |

这些数字来自`onUseSkill`实际扣蓝/独立冷却、`applySkill`和`stepStatus`实际掉血；不是只比较浮点公式或说明。测试使用独立高等级装备边界fixture驱动真实装备/软等级解析，未声称现网某件装备能提供测试中的巨额软等级。

六例外（冲锋、阴影跳跃、秘术：幻影、治愈术、复活术、光之奇迹）另各跑20次真实成功施法，对照精确连续公式的总扣款和实际等待帧。既有原始边界、原1–5数值、原伤害、状态、成长回归一并通过。

## 结算与生命周期

- 蓝耗与冷却余量归角色，按技能ID、量纲分别存；没有跨技能、跨角色共用。
- 预览不改变余量。所有准入条件通过后，余量与实际扣费一起提交一次；法力不足、无效目标、同帧重复包均不能消费或增加余量。保留原中断不退款规则。
- 换等级/装备后，已有不足1单位的已付信用按新分母向下换算，不能向上制造信用；一次换算最多损失不到`1/新分母`单位。不是无限精度承诺。
- 原生段冻结高阶信用，不让原1–5施法获得新折扣。已接受施法的费用/结束帧不随之后换装重算。
- `Character.Clone`深拷贝账本；角色快照随跨图、正常重登保留。迁移67提供旧角色空对象默认；数据库读写由同一角色事务处理，异常结构拒绝加载，避免把坏数据静默当免费信用。
- 燃烧余量归每个`Active`实例，只有真实周期到期才推进；多人、叠层、目标各自独立。替换/驱散/死亡时旧实例余量随实例结束，不转给新目标。`SavedStatus`与状态克隆保留应保留的余量，仍遵守既有来源离场撤销及离线不补跳伤规则。
- 燃烧切装掉级仍按旧的“施放时与当前等级取较低值”规则收缩；重新穿回高装不升级已存在的旧燃烧，也不能保留原高等级倍率。
- 装备形态的最低冷却也按真实帧向上取整，累计余量不能突破它。

## 客户端与展示

没有新增wire字段。既有`skill.ranks` sidecar在下一次实际整数耗蓝/冷却变化时推送更新，说明和行为契约取同一只读预览。phase0事件下发本次实际帧冷却，排队定义和`ready-at`一致。

客户端只对六例外的高阶段容忍缓存耗蓝最多相差1点，仍保留至少1蓝门槛和其他原准入。服务器始终按本次精确整数费用判断。此容错避免网络中尚未收到新预览时，把本可支付的较低费用错拦；不是把所有技能降成1蓝。

## 明确边界

不能把本结果说成“任何等级、每一发都变强”。本规则保证的是同条件持续战斗中的累计收益；单次整数结果可以相同或交替。

当前无额外修正的冲锋/幻影到9955级都达到1蓝/1帧，下一级仍不变。这次明确保留这些下限。没有引入0蓝、同帧连发或额外属性来掩盖边界；超出引擎数值表示范围仍沿用原显式拒绝。

原审计里的其他技能机制/参数问题不由本报告签署完成。本次没有真实联网Unity、线上数据库配置或生产部署验收。

## 回归与运行证据

所有命令先`source /workspace/shared/cloud-test-env.sh`，Go工作目录为`server/`。

- `go test ./internal/domain ./internal/game/scene -run 'TestSkillRounding|TestGrowthRounding' -count=1 -v`：domain4+scene5顶层专项通过；含700次实际成功施法及对应重复请求，21组真实燃烧累计、状态/角色生命周期、sidecar及phase0。
- 随后新增`TestSkillRoundingHonorsEquipmentMinimumAtRoundingBoundary`，domain相关5项`-race -count=3`通过。
- 随后新增`TestGrowthRoundingAllSixAcceptedCostStreams`，六例外各20次共120次真实施法通过。
- 最后新增真实复活术吟唱中断测试，验证扣款/信用只提交一次、中断不返还、不清冷却、接收者不取得施法者信用；与跨图/重登、sidecar、换装、六例外连续施法组合重跑通过，scene1.822s。
- `go test -race ./internal/domain ./internal/game/scene -run 'TestSkillRounding|TestGrowthRounding' -count=1`：在增加上述最后三个边界专项前通过，scene68.561s。
- `go test ./internal/domain ./internal/game/scene -run 'Test.*(Skill|Status|Growth|Burn|Original)' -count=1`：通过，domain0.588s、scene184.434s。共享工作区上执行，不冒充clean-commit全量构建。
- 客户端`Tests/Endgame/run-skill-behavior-presentation-acceptance.sh`：392个服务器样本，最终5074个实际生产方法断言通过；托管替身测试，不冒充Unity画面。
- 持久化：`Character.SkillRoundingCredits`写入`characters.skill_rounding_credits` JSONB，迁移67默认`{}`，重放不覆盖已付小数。`scanChar/saveChar`真实路径的mock验证末尾列序、第60个保存参数和`Clone`深拷贝。
- 并行集成任务在真实隔离PostgreSQL单用户模式跑迁移十项，覆盖事务rollback、旧角色不变、新角色默认、写入后重放保存、JSON类型约束，全部通过。`null`、技能key别名、`N/Mana`等非规范大小写JSON均拒绝，且不赋值/不执行保存SQL；store `-race -count=10`与独立复审探针通过。未运行pgx联网driver端到端，不混称该层已验证。

原始日志位于`/tmp/skill-growth-plateaus-20261013/`：`rounding-complete.log`、`rounding-all-six.log`、`rounding-race.log`、`rounding-variant-floor.log`、`broad-skills.log`、`client-rounding.log`、`rounding-final-lifecycle.log`。目录同时保留改前真实探针和纯算术方案，不能将它们与改后结果混淆。

## 提交文件分工

成长独占新增：
- `server/internal/domain/skill_rounding.go`
- `server/internal/domain/skill_rounding_test.go`
- `server/internal/game/scene/growth_rounding.go`
- `server/internal/game/scene/growth_rounding_test.go`
- 本报告

成长修改既有文件：
- `server/internal/domain/character.go`：账本字段与深拷贝
- `server/internal/domain/skill.go`：服务器内部精确分数/预览字段
- `server/internal/domain/skill_curves_v5.go`：仅`continueOriginal`清零分数/预览字段
- `server/internal/domain/skill_growth_policy_v7.go`：保留冷却精确分数
- `server/internal/domain/skill_mana_cost_v6.go`：保留蓝耗精确分数
- `server/internal/domain/skill_status_scaling.go`：持续绝对伤害不足1点部分
- `server/internal/domain/status.go`：余量字段、存档/恢复及`DueTicks`（其他机制字段/覆盖优先级为并行任务）
- `server/internal/game/scene/skill.go`：仅`onUseSkill`成功扣款前提交、入队后更新下一次费用（目标/效果区段为并行任务）
- `server/internal/game/scene/skill_behavior_v1.go`：只读预览
- `server/internal/game/scene/skill_mana_cost_v6_test.go`：每次施法对照当前纯预览，替换已过时的固定整数假设
- `server/internal/game/scene/skill_views_v5.go`：私有变化摘要计入信用，未加wire字段
- `server/internal/game/scene/status.go`：`tickStatus`消费本跳额外整数伤害

客户端：
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/NetClient.SkillCommands.cs`
- `Tests/Endgame/SkillBehaviorPresentationAcceptance.cs`

并行集成任务所有，统一提交需纳入这5个文件：
- `server/internal/store/postgres.go`
- `server/internal/store/skill_rounding.go`
- `server/internal/store/skill_rounding_test.go`
- `server/migrations/000000_init-67.sql`
- `server/tests/sql/validate_skill_rounding.py`

迁移66和七缺机制文件不属于成长实现。
