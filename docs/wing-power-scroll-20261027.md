# 翅膀威能改用卷轴（未发布）

本改动仅基于已含万象秘典/威能卷轴的 combined 服务端与配套客户端；不可把删除旧配方单独移植到 life-only 服务端。

- 撤销代码配方 900000（羽翼 + 暗金），包括格子匹配、直接 quote/execute、旧缓存执行重新规划。旧 ID 永不重新利用。
- 翅膀升阶、换系、羽毛换算不变。任何成长翅膀均不能被批量/单件分解回退或直接分解 ID 销毁。
- 威能卷轴仍用 `powerbook.apply`；背包/已穿戴成长翅膀都支持。依据服务端 `WingRules.Item`，不凭客户端名称、外观、部位猜测。
- 三槽沿用既有 `WingPowers`：攻击/防御/通用分槽，同类覆盖；另外两槽、各自数值、强化、镶嵌、原生属性、绑定及 UID 原样保留。升阶换系仍继承。
- 卷轴保存的是提取时的 ID/数值快照。后续秘典变强不反向更改已提取卷轴；同威能已有更高/相同数值则拒绝且不耗卷轴。当前职业限制仍有效；不新增头/衣/武器限制，也不新增人物/翅膀等级门槛。
- 秘典与卷轴没有供体暗金的 tier/level，故新槽的 SourceTier 使用普通值，SourceLevel 为威能目录最低合法适用等级（至少 1）。这是适用元数据，不是假称原供体等级，不用于威能倍率或穿戴限制。已有槽的真实供体元数据不改。
- 消耗卷轴与更新装备继续在原 endgameCommit 一次提交，失败不耗、重复点击不重扣，回执重放不再次附着。旧位置表载入的 kind=0 羽翼在成功附着时规范化为装备实例 kind=1，使首次附着能通过真实存储校验；失败不改原实例。
- 客户端只撤掉“必须已有 Legendary 字段”的错误前置限制；复用 AvatarFuse kind3，不新增协议、不添加展示格式。
- 119 仅修正服务端任务/物品文字；900000 从来不是数据库配方，无需删除 DB 配方行。先有118才可应用119。新库同样包含119；未执行线上迁移。

## 来源和验证

服务端以批量分解提交 `09f0fcb9d59265d995ec87fd62cd8506ff97ce7c` 为父提交（其父为明确标记的 ae2 源归档合成提交，见批量分解文档）。

目标测试包括 `TestWingScrollSlotsOverwriteRollsAndPreserveUpgrade`、`TestRemovedWingRecipeDirectAndGridNeverConsumes`、`TestWingScrollSelectionAndCubeInheritance`、`TestBatchSalvageRejectsRemovedWingExtraction`、`TestWingPowerScrollBagWornAtomicFailureReplay`；运行状态见交付日志，不将静态检查视作实机联调。
