# 三角色展示计划验证记录

2026-10-12：工作区回收后从已核验方案重建，保持原27件装备、三组衣服性别候选和44枚镶嵌星篆的精确顺序。

## 此次重跑通过

- `python3 -B docs/daoist-endgame/showcase/generate_showcase.py --check`
- `python3 -B server/tests/showcase_manifest_test.py`：6项通过
- `cd server && go test ./internal/domain -run '^TestShowcaseThreeCatalogLoadouts$' -count=1 -v`：通过，3职业×2性别，54件装备检查
- `git diff --check`：通过

使用已恢复的官方Go工具链；执行路径为 `/workspace/shared/server-toolchains/go/bin/go`。测试不依赖该路径，任何满足项目要求的Go工具链可重跑。`/usr/bin/go` 在该工作区不是Go语言工具链，不能仅凭命令同名判断可用。

`runtime-validation.json` 来自当前生产domain测试，不是重建的历史日志。通过 `SHOWCASE_VALIDATION_REPORT` 指定输出位置可重新生成。

## 验证内容

- 固定ID未按PowerTier重选；每个角色9个槽、一个双手武器，无盾／单手冲突
- 唯一装备生产NewStack、ApplyUnique、版本5、roll12000与实际目录名称
- 宿阵真实原始来源白底、零天然附加、无套装、孔容量、固定星篆Card快照和新UID
- 生产MatchAstralRecipe精确匹配，错误孔序拒绝
- 每套战／守／辅各一个实际生效频道
- 等级≤60、职业及男女分支资格
- 60级自然成长＋合法自由点预算可以构成同时满足全套门槛的合成角色
- 冰系软等级+3／其他术士+2；治愈术+7／其他治疗系+5／非治疗药师+2
- 固定物攻117%、生命-12%未被最高roll擅自整体缩放；显式会心变量和局部强化取真实最高档

## 不代表的结果

- 没有访问数据库、读取实际test001角色或执行发放
- 没有修改业务逻辑、目录、平衡或真实角色属性
- 合成角色面板不是用户当前角色面板
- 未验证完整实战DPS/HPS全局最优，不包含额外黄装词条、目录外卡片、加工或融合
- 未来发放前仍须在实际环境重读身份、性别、真实ItemDef、背包与状态，遵循既有安全保存路径

当前用户已将工作扩大为五职业原版技能复原，装备发放暂缓。此清单和报告不产生新的执行授权。
