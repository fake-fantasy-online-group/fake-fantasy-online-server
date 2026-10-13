# 技能配置 v9：实际 PostgreSQL 快照差异核验

状态：最终配置快照审计通过。迁移 70 的功能封顶后成本修正已施加，v9 与真实数据库 golden 已复跑通过。

日期：2026-10-13。发布基线：`88249e1`。本次范围：迁移 69–72 的四项已批准技能规则；背刺未启用。

## 结论

使用实际 `LoadSkills` 与 `LoadStatuses`，通过 26 次只读 PostgreSQL 查询导出 1490 条完整技能与解析状态快照。没有替换 SQL、模拟加载结果或重新生成 v7 基线。

- 覆盖原 149 个技能，每个技能固定 10 个等级：1、2、3、4、5、20、21、60、255、65536。
- 新旧每一条 Payload 重新计算 SHA-256，全部与导出内的哈希一致。
- 迁移 68 的旧导出全部 1490 条哈希严格等于保留的 v7 加 v8 delta。
- 本轮恰好 40 条变化：10102、10212、10312、10313 各 10 条。
- 其余 1450 条哈希完全相同，包含背刺。四个目标技能内，未列出的字段也逐项相同。
- v9 只新增上述 40 条显式差异，逐条保留 `PreviousSHA256`；10313 的前置锚是已经应用 v8 的哈希。
- `skill-config-golden-v7.json` 和 `skill-config-golden-v8-delta.json` 原文件均未改变。

这是一项配置与载荷回归审计，不替代战斗边界、随机分支或玩法平衡验证。

## 逐技能字段审核

| 技能 | 唯一变化范围 | 精确核验 |
|---|---|---|
| 10102 罡气剑 | `Skill.Effect.ChanceExplicit` 与 `Skill.Effects` | 原伤害段数值不变，只改为显式配置；有序链为原伤害后接击退。击退段与同等级雷火弹 10223 的真实导出完全相同：100 世界单位、10000 BP、依赖前一段命中。 |
| 10212 偷窃术 | `Skill.Name`、`Skill.Description`、`Skill.Effect`、`Skill.Effects`，高等级的 `Skill.MPCost`、`Skill.CooldownMS`、`Skill.ManaCostReductionRanks` | 从游龙掌占位伤害改为 `EffectStealItem`；零伤害，一生成功上限 1；1–5 级概率依次为 5000/5600/6200/6800/7400 BP，所采样高等级为 10000 BP。单段效果等于 `Skill.Effect`；1–5 级成本不变，10 级功能封顶后使用既有降耗/冷却曲线。 |
| 10312 驱魔术 | `Skill.TargetFamilies` | 仅新增 `['demon', 'undead']`。名称、原文、伤害、范围、蓝耗与冷却等均保持不变。 |
| 10313 天罚 | `Skill.Statuses` 与对应顶层 `Statuses` | 新增四项候选，ID 为 1006、1007、1008、1034；每项均为 `RandomGroup=1`、`RandomWeight=1`、`ChanceBP=4000`。原伤害、原完整文案、半径 150、蓝耗、施法时间、冷却等均不变。 |

偷窃高等级的原始 `Skill.Description` 按现有 V5 规则继承第 5 级 74% 原文；玩家技能说明由实际效果概率生成。偷窃工作项另有真实 PostgreSQL 测试，覆盖高等级显示实际 100% 且不再显示伤害。本审计没有为更新显示而改写其它技能的原文。

天罚四项基础时长在 1–5 级分别为：前三项 6–10 秒，定身 3–7 秒；高等级保留各自第 5 级基础锚并记录 `ProgressionRanks`。所有解析状态均有已知非零图标。定身解析结果为纯移动禁止，不附带属性词条。顶层解析状态定义与技能应用的覆盖时长是不同层，实际施加取值另由战斗测试覆盖。


## 偷窃成本层级说明

导出的 `SkillDef` 是实际加载配置，但尚未经过本次施法成本结算。功能封顶后 `MPCost=177` 是第 10 级锚，`ManaCostReductionRanks` 驱动原有降耗；`ManaCostReductionApplied=false` 没有改变。不能将原始 177 误读成高等级施法始终收取 177 蓝。

| 等级 | 旧蓝耗字段 → 新锚 | 降耗等级 | 旧冷却字段 → 新冷却字段（毫秒） |
|---|---|---|---|
| 20 | 297 → 177 | 10 | 5000 → 4167 |
| 21 | 309 → 177 | 11 | 5000 → 4099 |
| 60 | 777 → 177 | 50 | 5000 → 2500 |
| 255 | 3117 → 177 | 245 | 5000 → 848 |
| 65536 | 786489 → 177 | 65526 | 5000 → 4 |

原始定义的极高等级冷却可以低于 100 毫秒；1 蓝/100 毫秒的实际施法下限与跨次余数在 `PreviewSkillRounding`、`RoundSkillCost` 和真实施法路径验证。该下限不应凭本表的未结算字段推断。已交叉核对偷窃集成测试最终日志：65536 级通过真实 `onUseSkill`、动态说明、付款与实际冷却，结果为 1 蓝/100 毫秒；11、20、100、10000 级各 51 次连续施法及中途角色快照重入的总蓝耗、总冷却 tick 均精确通过。

## 保护措施

`loadSkillConfigurationGolden` 顺序加载 v7、v8、v9。每轮要求确切技能 ID 与确切 10 个等级，不接受重复、遗漏、未审阅身份/等级、无变化 delta、无效 SHA-256 或前置哈希不符。旧 fixture 没有被覆盖。

目录身份与策略比较单独显式审阅 10212：先要求旧 fixture 名称仍是“游龙掌”、`EfficiencyCapRank` 仍为 0，再将本轮预期设为“偷窃术”与 10；其余所有身份和策略字段、149 个成员与 128 条协同仍走原有严格比较。

## 复跑

在迁移 69–72 已全部施加、空闲且一次只有一个读取者的可丢弃测试副本上执行：

```sh
source /workspace/shared/cloud-test-env.sh
source /workspace/shared/pg-toolchain/env.sh
export GOTMPDIR=/workspace/shared/verify-v9-gotmp
export FANTASY_SKILL_CONFIG_PG_DATA=/path/to/idle/disposable/data
cd server

go test ./internal/data \
  -run '^TestSkillConfiguration(GoldenDeltaIntegrity|DatabaseGolden|DatabaseRejectsMissingAndUsesConfiguredPolicy)$' \
  -count=1 -v

FANTASY_SKILL_CONFIG_EXPORT_OUTPUT=/path/to/snapshots72.json \
  go test ./internal/data -run '^TestExportSkillConfigurationSnapshots$' -count=1 -v
```

新的独立导出测试默认跳过；指定输出后导出完整 Payload，使用排他创建禁止覆盖任何已有文件，包括 golden、delta 和旧导出，并拒绝有 `postmaster.pid` 的活动集群。输出文件必须尚不存在。

## 证据摘要

| 文件 | SHA-256 |
|---|---|
| `000000_init-69.sql` | `2f69ff94710c317903373b7b2ef1b3a7f8e897daab212efef11e1c80aece7fa9` |
| `000000_init-70.sql` | `a956e25e6c380dfeaa3959a0d63589e8010487a1b5f66f87327718a4d67c055c` |
| `000000_init-71.sql` | `b369431710215beef86d34d06b509c13024b1014558643a25b1f94de3b07c45d` |
| `000000_init-72.sql` | `b474a9542bf60e45cbec325fdb89a6bac30bf11f9a1b5be6bd9068c6c5d146fd` |
| v7 golden | `a35696407f4608b4f97bd86e0086a54fb1d54dce437d82bbb7332fa340d9ef3b` |
| v8 delta | `9f841b4e1452b002be05a31c1b47411b04cde998a6fb439112c19b419ced3dc4` |
| v9 delta | `4f5fe0c7366d65d8ded64beca60ed47bb0932c96a39edb6c8311d211aa63a5d6` |
| 迁移 68 完整旧导出 | `7b9e8cf2bb349e8491a101a94ed627d038078b8c85a240695cac7a6359f85b12` |
| 迁移 72 完整新导出 | `f5d50c91cad15d8825468731b7a1956d15bf8baf5b448163bec6224abea93399` |

## 最终验证记录

- `TestExportSkillConfigurationSnapshots`：PASS；1490 条，26 次只读 PostgreSQL 查询。
- `TestSkillConfigurationGoldenDeltaIntegrity`：PASS；v7 → v8 → v9 锚链与严格 ID/等级范围成立。
- `TestSkillConfigurationDatabaseGolden`：PASS；149 个身份/策略、128 条协同、1490 条技能/状态快照，26 次真实查询。
- `TestSkillConfigurationDatabaseRejectsMissingAndUsesConfiguredPolicy`：PASS；缺少 manifest、成员/策略、策略、协同、restoration，以及不合法低 cap，六项全部拒绝；独立数据库策略被实际读取，39 次真实查询。
- 完整新旧 Payload SHA-256 重算、1490 条原链比对、40 条授权字段白名单以及逐项精确值断言：PASS。
- `git diff --check`：PASS；v7/v8 文件相对发布基线无变化。

初次 golden 曾因旧名称预期未审阅而提前失败；保留旧 fixture 后加入窄范围名称 delta。随后发现偷窃功能封顶后的成本策略缺失，依原有用户授权补上 `EfficiencyCapRank=10`。仅在可丢弃集成副本精确恢复迁移 70 所属的旧 10212 前置夹具，重新执行修订迁移的首次导入；没有绕过或放宽产品迁移的重放保护。最终再次导出全部 1490 条并复跑上述测试。

修订成本前后只再改变 10212 的五条高等级样本；最终相对 v7+v8 仍然只有四个技能共 40 条变化。
