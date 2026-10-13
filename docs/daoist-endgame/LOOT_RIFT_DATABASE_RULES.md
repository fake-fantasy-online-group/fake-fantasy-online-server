# 补充掉落与大秘境规则回归数据库

## 范围与生产数据链

迁移 `000000_init-63.sql` 新增具名、强类型关系表，而不是通用 KV 或运行脚本：

- `game_loot_rule_versions`、`game_loot_rule_active`：符文掉落等级门槛、自然暗金概率及 MF 曲线参数、发布版本与当前版本。
- `game_loot_rune_pool`、`game_loot_socket_levels`、`game_loot_grades`：28 项等概率选择池、装备等级孔数上限、普通/精英/BOSS/秘境掉孔权重及补充符文概率。
- `game_rift_rule_versions`、`game_rift_rule_active`：进度、时间、成长、参与资格、奖励、经验、结算出口 NPC 外观及技能调度参数，每个版本引用固定掉落版本。
- `game_rift_class_rules`、`game_rift_ability_rules`：四类怪物生命/伤害倍率，以及三种既有技能的名称、词缀、警告、几何参数、延迟和伤害倍率。

`LoadItems → LoadLootRules → LoadRiftRules` 在开服时加载、验证和冻结，经 world/Scene.Config 注入；所有规则参数必填，nil 只表示配置缺失并关闭对应功能，不会回退到 Go 种子。加载错误令开服失败。真实自然掉落路径在 `scene/loot.go`、`loot_context_v5.go`、`endgame_rune_loot.go` 消费规则，秘境战斗/经验/奖励/NPC/技能路径消费该 run 的规则快照。

补充符文仍复用 `DropEntry.RollWithMult` 的百分比/等级差惩罚，先掷一次是否掉落，再在固定 28 项池中选一项。没有把 28 行解释成 28 次独立掉落。自然暗金仍限定原 259 项暗金目录；普通奖励装备仍保留原 885 项原生掉落资格。

普通装备资格现在由 `LoadItems` 根据 `ov_arm` 的 `query_type`、位置、资源等级及禁掉标志计算并写入 `EquipDef.NativeDropEligible`，与既有类型掉落查询同源。旧 885 项 JSON 只作为测试金样，不再被生产 go:embed 加载；真实数据库逐项对照已通过，包括 66 级普通装备。

## 版本、迁移与存档

发布后的 header 和子行由数据库触发器禁止 UPDATE/DELETE/追加子行；修改时插入一个新的未发布版本及全部子行，验证完整后发布，再切换单独的 active 行。运行中的进程使用开服快照，变更按既有流程重启生效。引用版本必须存在且已发布。

新 run 持久化 `RulesVersion` 和完整规则快照（含固定 LootVersion）。旧 run 没有该字段时只能绑定数据库的 v1，不能读取当前 active。恢复时要求快照与发布版本一致，否则拒绝恢复，避免篡改或配置漂移。新 active 只影响之后创建的 run。奖励仍先冻结再按原事务一次性提交，不改既有实例 UID 或已落地奖励。

迁移必须事务性应用，并与匹配代码成对部署/回退。重复迁移不覆盖已有版本、active 选择或管理员的新版本，也不改角色/物品实例/背包。v1 保持原数值；旧算法金样覆盖全部 150 层奖励/成长、等级 0–70 的每个掉孔随机桶及 MF 边界。

校验拒绝缺失 v1/active、重复版本、未闭合物品/掉落引用、坏概率、零除数、时间/数量溢出、无长度直线技能。名称按客户端 UTF-16 长度校验，另以真实 JSON 编码验证旧/新楼层最坏保守快照不超过 61440 字节。

## 验证与边界

真实 PostgreSQL：63 首次与重复应用，published header/子行不可变，发布 v2 + 切 active2 + 重复迁移仍保留 active2/v1，全部测试探针事务回滚；实际 `LoadItems` 得 22162 项、28 符文、4 掉孔档、3 技能，885 项资格逐项不变。

只读 loader 测试入口：设置 `FANTASY_BALANCE_PG_DATA`、可选 `FANTASY_BALANCE_PG_DATABASE` 及受信 `PG_BIN` 后执行 `go test ./internal/data -run '^TestBalanceActualPostgresLoader$' -count=1 -v`。集群必须为独立且空闲的测试集群，使用 single-user 只读查询，不连接应用数据库。

本批明确不改变楼层生成器版本/密度/出生点/安全半径/编队数量及已保存 FloorPlan；这些参数仍是后续规范项。试炼符钥奖励计算及试炼参与判定也未迁入 63。随机算法、碰撞/落点、实体身份、归属/容量、事务与回执机制保留 Go。固定技能枚举对应执行内核，数据库不能增加任意脚本技能。

## 原生大秘境入口（SQL64）

`game_rift_entrances` 以地图与 `map_npcs` 复合主键引用现存 NPC。当前绑定龙城（地图 7）的八仙使者（qw0007/83）；不修改原 NPC 数据，不再把普通试炼守卫换成八仙外观。入口是世界服务配置，独立于已冻结的秘境平衡版本和历史 run 快照。loader 校验实体存在且可见，再随已协商的 `endgame.rift-entrance.v1` hello 下发地图、配置号、姓名和脚本。开境时按客户端点击的原生实体 ID 反查数据库配置号、场景与对话上下文，不接受只凭同名 NPC 或同模型冒充入口。
