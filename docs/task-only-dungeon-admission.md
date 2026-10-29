# 单人纯任务副本准入（2026-10-29）

## 本次规则与边界

仅对有明确原任务单人用途证据的四张地图增加规则：角色本人任务仍未交付（Active=0 或 Deliver=1）才允许显示及进入；没有记录、已放弃或 Finished=2 均隐藏并拒绝。正常反复刷且有产出的副本不因关联剧情任务增加门槛。没有足够证据的“纯任务组队副本”不硬凑白名单，本次没有新增组队任务门控。

| 地图 | 原任务 | 原任务证据 | 生效入口 |
|---|---|---|---|
| 29200 异龙之巢 | 355 九转（法系） | init-32.sql:51189 明确独自前往 | 原入口表老道士；直接切图；恢复旧实例 |
| 29400 隐魂之森 | 356 净幢（法系） | init-32.sql:51190 明确独自前往 | 原生表、仙人安期生 list112、直接切图、旧实例 |
| 29900 月牙塔 | 359 天塔（物理系） | init-32.sql:51193 明确独自前往 | 原生表、仙人刘丹 list111、直接切图、旧实例 |
| 30000 月轮天经塔 | 361 凌霄（物理系） | init-32.sql:51195 明确独自前往 | 原生表、古仙陈烜 list113、直接切图、旧实例 |

地图及 Solo 配置见 init-0.sql:8803–8806；任务发布 NPC 的实际落位见 init-0.sql:7378、7437、7606，传送绑定见8895–8897。所有 init 引用位于 server/migrations。客户端/服务端精确映射已逐项核对一致，不从名字或等级猜规则。

原文另含“一次机会”“出本不能再进”等叙述，本次**没有**把它们扩成新计数、重入锁或清场规则。未交付期间仍服从既有实例规则；完成后不因本改动即时踢出正在场内的角色，但新入场及普通实例重新登录会复核资格。特殊试炼233仍严格 Active=0，保留原重接/完成资格恢复逻辑。演劫洞天的服务端 RiftRunID 交接及现成实例恢复不继承复用地图模板的任务限制。

## 355 缺定义不能成为无任务开放的理由

现有 init-41.sql:93–137 会因发布/步骤 NPC 无落位禁用任务；无泪公没有 map_npcs 记录，data/questoverrides.go 的 disabled 循环会删除任务355定义。原入口表的老道士也没有当前落位。本次没有补造 NPC 或恢复任务接取、交付、奖励。

但 store/inventory.go 的 loadQuests 查询读取所有状态，**并未删除旧角色的355任务记录**。此前 domain.ActiveQuestsOf 遇到缺定义跳过，导致服务端有活动记录而客户端 Active 没有。

现在只在缺少355定义且真实存档是 Active/Deliver 时，沿既有0x8027投影其 ID、原任务名称、保存的 Progress 和对应 Ready 状态。未知步骤总数保持0，目标 NPC 为空，不生成 QuestDef/Offer/奖励；其他未知任务继续跳过。无记录、Finished 或非法状态绝不通过缺定义 fallback 获准。若将来加载到真实定义，正常定义投影优先。此补丁修准入和状态一致，不代表355已可新接或完整通关。

## 权威执行与列表索引

- domain.DungeonEntryQuest / CanEnterQuestDungeon 是唯一服务端白名单及状态判定。
- scene.openPlayerDungeonDifficulty：普通入口、困难命令、权威列表进入的共用 guard；按每个请求角色自己的 QuestLog 检查，不能借队长状态。
- scene.onTeleport：普通跨场景直接请求、客户端先切图后上报、显式实例目标均复核；拒绝后恢复客户端权威来源图。服务端已有 Rift 交接保留自己的资格验证。
- session.resolveLoginScene：加载完成记录后再恢复普通副本；不合格落原配置出口并保存，不创建新实例。
- 客户端 DungeonQuestAccess：原生表和权威行均在显示/点击时判定，任务回包刷新菜单并使旧回调失效；不会破坏任务详情子页。
- 0x8049 **不删行、不改顺序**；显示过滤后仍发送原0x1072索引。新协议字段、任务权限包和自造传送协议均未添加。价格、任务/清场旧 guard 和困难追加规则不改。

## 刻意保留的常驻入口

四个天魔子图已有实质装备产出，保持原规则：26900↔307夺玉、24400↔309逆鳞、24500↔311仙劫、26800↔313终之试炼（关联来自 init-32.sql:51156/51158/51160/51162；311不是312）。当前掉落追加机制见 init-39.sql:363–485，init-98 只撤花岗石模板行而不撤其他装备。不能因原剧情写组队就把现在可刷装备的入口关掉。

22901/23601 有技能书/奥义资源，40900/41100 有须弥资源产出，均不新增任务 gate。40700 未证实为专属任务本，保持未知且不改。八仙、纯阳、天空等常规产出本不改。具体掉落分类由并行 SQL 审计提供；本实现只改变上列四图，未改任何掉落数据。

## 验证

基于已集成前6项的 client95b51f5 / server29236d2，包含已核 main36aab0ec / 6f01fdcf。2026-10-29 04:25 UTC 再查 main 未变化。

- domain：四图状态矩阵、弃任务/重接、Finished不能被弃成许可；普通产出/试炼地图排除。
- store：生产 loadQuests 查询+受控 pgx Rows 验证全状态读取，Memory 保存/重载完成历史；不等于真实 PostgreSQL 集成测试。
- scene：普通创建/加入资格、直接请求和 ClientMapReady 拒绝回原图、权威混合菜单打开后弃任务/交付/完成复核、355缺定义真实回包及禁止凭投影接取/领奖。
- session：四图×无记录/Active/Deliver/Finished，实际保存→载入QuestLog→恢复已存在的自有实例或落出口并持久化。
- 反例：已完成309的大号与未接309的小号仍可同队进入24400普通产出本；现有困难入口与试炼/Rift定点回归。
- 客户端：394入口与状态断言；206真实 BuildMenu/分派断言；206试炼；46原生sender；35任务回执；64经典交互。
- 完整 managed 编译：core418文件、Presentation1237文件，使用已验证 Unity 参考程序集；未执行 Unity/设备/线上游戏服。

服务端执行命令（server目录）：

```sh
source /workspace/shared/server-toolchains/env.sh
go test -race ./internal/domain ./internal/store ./internal/game/scene ./internal/session \
  -run 'TestQuestOnlyDungeonAdmission|TestQuestDungeon|TestDungeonQuestEntryStateMatrix|TestDungeonTransport|TestEnterDungeon|TestRiftTrialReentryWaitsForOldEntitlement|TestNativeDungeonReady|TestRiftEntranceRequiresExactNativePlacementAndDialogue|TestRiftStartWrongEntranceNeverReservesOrDebits|TestRiftHelloAdvertisesDatabaseBindings' \
  -count=1 -timeout=120s
```

仅聚焦回归通过，不宣称整个服务端全量通过。此前全scene/domain race运行在既有耗时用例超时，未将其包装为成功。不发布、不升版、不写线上DB。

## 独立复审追加：非法保存状态

已先复现两项红测再修复：有定义的四任务 state3 原先仍进入 Active 回包；数据库 raw256/257/-256/-255 原先会在 int32→uint8 时别名为0/1。现在仅355/356/359/361在投影前排除非Active/Deliver，且仅这四项在加载窄化前把非0/1/2保留为非法3。非法值不会生成入口资格，也不会别名为Finished完成历史。有效0/1/2、真实355旧记录以及其他任务原行为不变；未改schema/协议，也未批量修改数据库。新增定义存在/不存在、无记录、非法值、原始整数边界及其他任务负控。
