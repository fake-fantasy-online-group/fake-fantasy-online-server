# 数据源与原有开发约定审计（2026-10-13）

基线：开始审计时 HEAD `04f9775`。范围为 server-development 中新增演劫、仙遗、宿阵、魔盒、技能成长及装备特效；不把原有引擎算法全部搬入 SQL。以下行号以基线为准，后续整改可能移动。

## 结论

根本问题不止 32 项物品目录漏入库。原项目约定是 PostgreSQL 静态配置 → `internal/data` 解析、检查 → world/Scene 只读表；新增内容却有多份生产硬编码目录。目录迁回 SQL 后，显示、结算、持久化验证和离线工具必须同源，不能只是数据库网页能够搜索到名称。

原架构证据：

- README 第 5 行要求地图、碰撞、NPC 和其他业务配置统一存入 PostgreSQL。
- `cmd/gameserver/main.go:99-106,775-792,870-910,989-1013`：启动加载 world、items、skills、craft/socket/drop tables，运行中查只读定义。
- `internal/data/gamedb.go:770-794,1129-1173,2290-2323,2341-2429,2478-2630`：物品、卡片实例默认效果、装备固定/随机属性与掉落候选均有原数据库加载路径。
- `internal/data/craft.go:10-78`：新增业务配方已有 `game_craft_extra_recipes` 先例，不需要伪造客户端原始行；`socket.go:10-61` 保留有限玩法算法，只装配方参数。
- `internal/arch/layers_test.go:26-47`：domain 纯模型、data/store 均只能依赖 domain。不能由 domain 调 SQL 或让 store import data。其引用的 docs/架构 文件当前不在本 checkout，不能据此声称已读。

## 逐项发现与整改边界

### 1. 32 项普通物品与卡片：优先级 P0

`cmd/gameserver/endgame.go:21-30` 在 LoadItems 后追加钥匙、28 星篆和三种材料；DB 查询无法看到它们。名称、等级、图标、库存分类、交易与堆叠标记属于数据。星篆的卡片默认效果亦应走 `ov_card/ov_card_entry → loadSocketAffixes`，不能从代码回填而形成双源。

整改：增量 migration 59；复用 `game_items` 及已验证的原始物品投影/特殊图标/卡片效果路径。不得更改 IDs、UIDs、现有卡片持久化效果。`CardDefaults` 只用于新实例，旧实例仍读取保存的卡片状态。由独立 worker 负责。

### 2. 仙遗与宿阵目录：优先级 P0

- `domain/unique_catalog.go:44-60,62-102`：生产使用 go:embed JSON，在 package 初始化加载当前 259 条、旧 V5 259 条、旧 V4 1619 条。
- `domain/endgame_runes_catalog.go:1-5,37`：28 个星篆和 40 个宿阵生成成 Go 字面量；legacy V4/V5 另有 Go 目录。
- `domain/unique_catalog.go:165-179` 与 `endgame_runes.go:172-181`：已有装备用保存的 content version 选择旧定义，**版本 0 不等于当前版本**。
- `domain/unique_catalog.go:261-280`：旧 V5 的三种 aura 运行时已转换为个人防护。迁移必须保存迁移前的实际有效语义，不能意外恢复旧团队 aura；该兼容行为需要单独 fixture 证明。
- `store/endgame_item.go:19-30,85-99`：持久化读写验证也消费全局 UniqueOf。只改 Scene 会造成存档路径仍旧依赖另一目录。
- `internal/showcase/factory.go:150-241`、`showcase/grant.go:270-274`、`cmd/showcase-grant/main.go:80` 是真实离线消费者，不是测试可忽略的代码。

可落地契约：domain 提供 `EndgameCatalog`，私有 version + ID 索引，构造后只读、返回定义时深拷贝；data 提供 `LoadEndgameCatalog(ctx,q,items)`。目录用普通列和有序子表表达文案/身份、词条、浮动词条、本件强化、traits、skill bonuses、typed effects、proc/aura、宿阵 rune 顺序和 slots。允许有限 typed effect 枚举，不允许 SQL 表达式或脚本。JSON 仅可作为明确测试/迁移输入，不是生产第二权威。迁移 60 不更新任何玩家或实例数据。

接线：world/Config/Scene 注入同一快照；ItemDef 可挂同一只读目录指针，让已有接受 ItemDef 的纯算法无需新增数据库依赖。仅接收 Stack 的查询须显式获得 catalog。Store/离线工具必须显式依赖相同目录，不可通过全局 install/singleton 兜底。当前/旧版目录应同时验证存在；未完成配置时拒绝受影响操作或启动，不能默默使用 embedded 数据。

### 3. 魔盒：优先级 P0，独立迁移 61

`domain/endgame_cube.go:31-158` 写死 66 配方名称、operation、材料、铜币和文案。`endgame_cube_plan.go:246-269` 另写死分解产量；改前者而遗漏后者不能称全部配置化。

新增 `CubeRecipeDef/CubeRuleTable`，`data.LoadCubeRules`；`CubeContext` 注入规则与 EndgameCatalog。同一 recipe 对象供目录、quote、execute 使用。把费用、产物、分解曲线参数和转换材料数迁为 typed 数据；保留 Go 中明确 operation handler、物权、锁定、孔身份校验、容量、随机候选缓存和原子提交。不要把魔盒强塞 `ov_combine`，它涉及 UID、原样退孔、版本化装备等原表无法表达的语义。

`scene/endgame_cube.go:69-104` 的报价期限/指纹/候选重试机制须保留。此次仅启动装载，不增加热更新；未来若增加，quote 要绑定规则版本，避免显示旧费用执行新费用。

### 4. 技能成长与恢复补丁：优先级 P1

- 原始技能与状态仍来自数据库：`data/gamedb.go:2739-2798,3506-3589`。`InstallV5SkillProgression`（domain/skill_progression_v5.go:335-359）是预检并设置 FormulaVersion，并非把全部原始 1–5 级替换成嵌入 JSON。
- `domain/skill_curves_v5.go:19-67` 保留所有原始 1–5 级；高阶通过纯 evaluator 推导。**插值、精确整数/溢出、范围几何、效果语义应继续是 Go**。
- `skill_catalog_v5.go:4-154` 的 149 技能身份、分支、开启等级、前置和 `:156` 后协同边是数据；`skill_growth_policy_v7.go:18-66` 每技能 caps/属性/攻击目标是数据。应新增 typed policy/synergy 表，复用已有技能 ID 并启动交叉验证；不要重复一份原始伤害/耗蓝数值。
- `data/native_skill_restoration.go:19-42,48-64,67-89` 的指定技能状态、持续时间/倍率和两组新状态是数据补丁；已有 `game_skill_statuses`、`game_custom_statuses`、`game_custom_status_affixes` 可复用。若缺移动封锁等字段，增量扩展结构；不要同时保留 Go 后覆盖 SQL。
- `data/progression_v5.go`、migration 52 的级别曲线/checksum 是正确 DB 方向；迁移 57 的原始状态图标也应保留，不要重做回代码。

### 5. 装备 proc/aura/技能变体：P1，与目录迁移衔接

每个装备的 proc chance、rank、ICD、family、radius、lease、visual key 都是目录字段（`domain/equipment_effects_v5.go:7-47`），应随版本化装备目录入库。有限 payload/trigger 枚举、目标合法性、根事件防递归、CD 持久化、同组择强是引擎。

额外漂移点：`scene/equipment_aura_v5.go:85` 写死 500ms，虽 Aura 有 RefreshMS；`:236-257` 与 icon 映射是 aura ID→状态/图标硬编码。`domain/equipment_effects_v5.go:116-127` 把部分平衡值固定在验证器等式中；迁移须区分引擎安全上限和可配置值，不要把 SQL 字段加载后又强制等于代码字面量。

`domain/equipment_skill_variants_v1.go:19-40` 含名称/技能/描述/目标数；五种变体的动作 handler 仍应有限分派，文本与参数同源，不接受客户端公式。统一战斗行为由另一 worker 专审。

### 6. 掉落与大秘境：P1/P2

- 原始普通掉落和词条矩阵已在 SQL，应复用。
- `domain/equipment_native_drop_scope.go:11-33` 另嵌入 native equipment ID 列表，而 `data/gamedb.go:2572-2600` 已有原 DB 分类候选提取。应从相同 SQL 分类生成只读资格集合，避免两个独立权威；仙遗 259 入选清单和普通可掉装备集合必须保持不同。
- `scene/endgame_rune_loot.go:24-47` 的等级门槛/0.6、8、20%/28 等权是补充掉落规则数据。
- `domain/endgame_runes_drop.go:23-35` 的开孔等级档位/权重是数据；随机抽取与天然+孔+随机预算算法保留 Go。
- `domain/loot_quality_v5.go:26-65,79-105` 的 MF 运算、精确概率计算保留 Go，可把已确认的平衡系数放版本化规则；当前真实普通仙遗掉落走 RollNaturalUniqueWithMagicFindV5，不能只改已无生产调用的 RollUniqueDrop 1% helper。
- `domain/rift.go:9-19,280-334` 的时限、积分、难度倍率、组队倍率、奖励参数；`rift_progression.go:6-7` 跳阶间隔；`endgame_rift_experience.go:7-17` 经验比例属于规则参数。
- `scene/endgame_rift_rewards.go:65,86,98,104,111` 又含10阶保底/60级入选/奖励物品 IDs。资格检查与保底池算法留 Go，阈值/产物由规则表读取。
- `data/rift_floor.go:20-83` 使用真实 DB 地图/出生点/碰撞，是正确实现；连通性洪泛、确定性 PRNG、完整保存 plan、重连复用旧 plan 都应保留。`domain/rift_floor.go:11-21` 的人口密度等是有限生成器参数，改变它们必须伴随生成器版本。
- `scene/endgame_rift_abilities.go:52-80,277-300` 三种技能名称/警告/尺寸/等待/伤害系数是数据；几何命中、预警对象快照、取消/结算时序是代码。不要为每个怪物复制特判。

## 验收与风险控制

1. 先导出当前全部有效定义做规范化快照；SQL 装载结果必须逐行同值，覆盖 current 259 + legacy 259 + legacy 1619 uniques 和每版本 40 formations。
2. 对 0/5/6 装备保存/重读/换装/交易/邮件/仓库/离线发放做回归，原 UID、revision、roll、socket UID、CardEffects、bound/locked 均不改变。
3. CurrentVersion 仅用于新建/明确成阵操作；只读、登录、tooltip、存档不得升级旧装备。现 `endgame_runes.go:350-366` 的成阵采用规则保留。
4. loader 拒绝缺行、重复键、无序/重复 rune position、未知 effect kind、悬空物品/技能、非法数值；生产禁止静默 embedded fallback。
5. DB 的受控测试变更应真实影响重启后的 catalog/tooltip/quote；未知配置应清晰阻止启动，而不是 SQL 看得到、游戏继续用旧常量。
6. 全部配置在 actors 启动前读取。战斗热路径不得访问 PostgreSQL。`domain` 不 import data/store，不能新增可变全局安装目录。
7. 每批迁移都是新的编号、可重复执行且仅改变该批业务配置；不重跑全库初始化、不重平衡、不批量重写旧存档。
8. 最后跑 arch、domain、data、store、scene、session/protocol、离线工具专项及全量可运行检查；DB 实测需独立空库和旧库迁移两套证据。此次审计本身不声称已跑 DB 或完整测试。

## 批次安排

A（59）：32 物品、原卡片效果通路、查询可见性。
B（60）：unique/formation/rune metadata 正规目录、只读依赖注入、版本保留、store/offline一致性。
C（61）：cube typed rules/配方/产量、quote/execute同源。
D：技能 identity/synergy/growth policy 和新增 native restore 参数搬入已有/扩展的技能配置表。
E：补充掉落/普通装备资格/洞天平衡与能力参数；沿用已正确 DB 化的地图、碰撞、怪物、原掉落资料。

B/C 不需要等待 D/E 才能独立验收，但完成 A/B/C 不能宣称全部新增业务已回归数据库规范。
