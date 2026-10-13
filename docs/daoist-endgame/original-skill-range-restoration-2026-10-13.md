# 三处原技能半径恢复与验证

日期：2026-10-13。范围：云端代码与独立 PostgreSQL 测试数据；没有操作用户电脑、在线游戏数据库或真实客户端，没有提交或推送。

## 已实现

迁移 `server/migrations/000000_init-68.sql` 仅修正以下原 1–5 级导入字段冲突：

| 技能 | 原完整说明的半径 | 修正前 typed/runtime 半径 | 修正后 |
| --- | --- | --- | --- |
| 战吼 10028 | 100 | 200 | 100 |
| 挑衅 10127 | 100 | 200 | 100 |
| 旧群攻天罚 10313 | 150 | 200 | 150 |

依据：原说明以及[腾讯旧战士指南](https://ffo.qq.com/new/newerData/zs.htm)、[腾讯旧剑客指南](https://ffo.qq.com/new/newerData/jk.htm)、[匹配旧群攻版本的 FF 药师表](https://ffo.17173.com/information/2_1%2C6.shtml)。来源细节及版本限制见 [FF 参数研究](free-fantasy-skill-parameters-2026-10-13.md)。这是依据原说明及对应旧表解决项目运行字段冲突，不声称实测过腾讯原版客户端半径；现代挑衅200或单体天罚没有混入。

无需修改生产 Go。现有 loader 从 `ov_skilldesc.radius` 同时生成 `SkillDef.Radius` 与 `Area.Radius`；现有目标选择及说明展示已经读取该数据。原 SQL 源文件、完整 `desc_`、施法资源、伤害、状态、中心规则、成长政策、玩家资料均未改。

迁移复用66的 `game_skill_mechanism_sources`，增加3条 `original_radius` 来源记录。首次只改当前半径为200的原1–5级行，其他半径保留管理员值；原五级缺失会明确报错。重复执行不覆盖后续编辑，包括管理员改回200，也不恢复被删除的行。须按迁移顺序在66之后执行。

## 高等级与 golden 影响

战吼、挑衅继续既定5级后范围成长：每级原半径的2%，新增50级后接连续平方根曲线，无硬上限。只把基线恢复为100，不把所有级别固定成100。旧天罚没有获准范围成长，维持150。

| 技能等级 | 战吼/挑衅原派生半径 | 恢复后的派生半径 | 天罚恢复后 |
| --- | --- | --- | --- |
| 1–5 | 200 | 100 | 150 |
| 6 | 204 | 102 | 150 |
| 20 | 260 | 130 | 150 |
| 21 | 264 | 132 | 150 |
| 55 | 400 | 200 | 150 |
| 56 | 403 | 201 | 150 |
| 60 | 419 | 209 | 150 |
| 100 | 551 | 275 | 150 |
| 255 | 894 | 447 | 150 |
| 65,536 | 14,480 | 7,240 | 150 |

对比149技能×10个既有 golden 等级（1–5、20、21、60、255、65,536）的有效 `SkillDef` JSON，仅这3招×10级共30份发生变化，且字段严格限于 `Radius` 和 `Area.Radius`。对比使用只读 SQL 适配器复现迁移前半径200，其他配置相同；没有重写历史 golden。数据库原行仅15个 radius 字段变化。

## 已通过

1. `tests/sql/validate_original_skill_ranges.py`：独立 PostgreSQL 17.11，闲置65测试集群的复制品先加载最新66/67，再测试68；首次/重放、事务失败回滚、缺少原级拒绝、初始非200管理员值保留、重放时管理员改200/改123/删除保留。原表精确15个 radius 字段差异，其他13张配置/角色表快照不变，66来源记录保留。
2. `TestOriginalSkillRangesPostgres`：真实只读 `LoadSkills`、`LoadStatuses`，42组技能/等级的几何、完整原文及派生显示校验，1490份有效技能差异校验，8.327秒通过。
3. 上述42组中39组进行真实 AOI 边界选择：原1–5级覆盖99/100/101及149/150/151；各高阶继续测边界点、对角线圆形排除和死亡目标排除。战吼/挑衅的自中心不受远处选中目标影响，天罚的目标中心及地点中心一致。65,536级只检查数值/展示，未做大半径 AOI 扫描。
4. 既有领域 `TestSkillGrowthRangeBoundariesAndNoHardPlateau`，0.148秒通过。
5. 既有场景范围专项 `TestAreaSkillHitsMultiple`、`TestSkillOutOfRange`、`TestRedesignIndependentApprovedSupportRangeReachesActualNewTargets`、`TestSkillGrowthAuditApprovedAreaTransitions`、`TestScopedCastAreaAndPlayerDescription`、`TestEquipmentSkillVariantsGeometryAndBudget`，0.233秒通过。
6. `git diff --check`、Python语法编译通过。

## 重跑

使用受信 PostgreSQL 工具目录配置 `PG_BIN`（必要时配置 `LD_LIBRARY_PATH`），工作路径必须尚不存在。允许省略 seed 参数从空库加载0–67；提供 seed 时必须是闲置且已至65、尚未66的独立测试集群。

```sh
python3 server/tests/sql/validate_original_skill_ranges.py \
  --work-dir /path/to/new-isolated-range-check \
  --seed-dir /path/to/idle-migration65-test-data
cd server
FANTASY_ORIGINAL_RANGES_PG_DATA=/path/to/new-isolated-range-check/data \
  go test ./internal/game/scene -run '^TestOriginalSkillRangesPostgres$' -count=1 -v
```

## 未验证的范围

本专项没有运行完整全仓 Go 测试、线上迁移、真实客户端圈绘制/网络协议回归、客户端截图或全部149技能联网实测。PostgreSQL使用无socket的single-user只读loader查询；不能据此声称pgx网络连接或多连接并发事务验证。天罚随机状态池仍属缺参，本次仅恢复范围，没有补出未知负面状态机制。全量66/67/68串联与综合 golden 审核另行记录。
