# 大秘境需求核对表

核对日期：2026-10-13（UTC）  
基线：服务端 `cd43b8f`；客户端 `fa41a5b`。  
核对范围：实际源码、数据种子与本次运行的针对性测试。未修改仓库代码。

## 结论

本表中的大秘境面板、解锁、通关结算、暗金掉落表现、连续换层、种子布局和钥匙经济都有实际实现；对应针对性测试通过。**源码和契约测试通过，不等于实机全流程已经验收。**

“真实八仙使者能够作为入口”不在本表记为通过：基线实现仅改变既有试炼入口外观，不能把换模型等同于真实NPC入口。该入口修正需依据最终代码和测试单独验收，不能用本次基线报告替代。

状态口径：
- **已实现**：实际执行路径存在，所列针对性检查通过。
- **待实机验收**：未运行真实Unity客户端、玩家包及客户端/服务器联机视觉与操作流程。
- **部分完成**：基线未满足完整要求。

所有下述客户端视觉和声音项均仍待实机验收。

## 路径说明

- 服务端仓库：`/workspace/shared/server-development`
- 客户端仓库：`/workspace/shared/client-development`
- 下文服务端文件路径相对于服务端仓库；客户端展示层统一前缀为 `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/`。
- 行号对应上述基线。核对后有其他入口修正工作进行；本表不把随后工作区变化冒充基线结果。

## 逐项核对

### 1. 使用游戏原资源背景，不手绘新面板

**状态：已实现；待实机验收。**

- `EndgamePanel.RiftArtwork.cs:11–54` 通过原资源加载器复用 `res/uires/common/msgboxbk.gsa`，保留四角、扩展边缘和中心。缺资源时不生成替代美术。
- `EndgamePanel.cs:146–166` 在秘境模块实际调用背景和下拉绘制路径，非孤立辅助函数。
- 面板契约运行实际production partial，验证尺寸、边角和事件处理，但使用Unity/artwork替身，并未真正显示Unity窗口。

### 2. 1–150难度下拉选择

**状态：已实现；待实机操作验收。**

- `EndgamePanel.RiftArtwork.cs:57–139` 提供滚轮、滚动条、上下键、Home/End、PageUp/PageDown、Enter/Escape以及鼠标选择。
- 可选范围为 `1..min(150, MaxTier, UnlockedTier)`，不是开局全部开放；缺失解锁信息时最多1阶。
- `server/internal/game/scene/endgame_rift.go:159–161` 在服务端再次拒绝未解锁/越界难度。
- 面板契约包含逐一选择150项、解锁上限收缩、弹层关闭和点击穿透保护。

### 3. 通关按剩余时间开放更高难度，每整3分钟一阶、至少+1

**状态：已实现。**

- `server/internal/domain/rift_progression.go:6–19`：本次跳阶数为 `max(1, floor(remainingSeconds / TierJumpSeconds))`，最终上限150，既有最高解锁不会降低。
- `server/migrations/000000_init-63.sql:193` 将 `TierJumpSeconds` 配为180。
- 精确行为：剩余0～359秒均+1；360秒+2；540秒+3；900秒+5。这符合本次采用的“每整3分钟一阶，至少1阶”规则，不列为缺陷。
- `rift_progression.go:25–39` 在成功完成后使用Boss死亡时间冻结余时，扣除死亡罚时，不因拾取/重连/补领变少。
- `server/internal/game/scene/endgame_rift_progression.go:8–11`、`endgame_rift_completion.go:294–307` 将解锁单调合并进个人持久化资料。
- 测试：`TestRiftTierJumpRemainingTimeBoundaries`、`TestRiftCompletionFreezesTimeAndPenaltyAcrossPersistence`、`TestRiftCompletedTeamUnlocksWithoutLeaderClaimOrPersonalTierGate`、`TestRiftOvertimeCompletionStillAdvancesButFailureNeverDoes`。

### 4. 使用游戏原生进度条资源

**状态：已实现；待实机验收。**

- `EndgamePanel.RiftArtwork.cs:12–13` 指向 `expbarbk.gsa` / `expbar.gsa`。
- `EndgamePanel.Rift.cs:97–107` 根据权威进度比裁切填充UV和宽度，保持原条高度；面板和HUD均调用。
- 面板契约验证生产绘制调用及裁切参数；不证明真实客户端资源已加载成功。

### 5. Boss死后出现真实出口NPC，有对话并能离开

**状态：已实现；待实机联机验收。**

- `server/internal/game/scene/endgame_rift_combat.go:238–242` 在Boss完成事件记录奖励位置并冻结奖励。
- `server/internal/game/scene/endgame_rift_completion.go:172–191` 等待完成/奖励持久化后，生成真正的 `KindNPC` 实体“归元道使”，进入AOI并广播，而非只有UI文字。
- `DialogUI.RiftExit.cs:10–19` 添加原生NPC对话“送我离开洞天”和确认提示，发送既有 `trialleave` 命令。
- `server/internal/game/scene/trial.go:53–59` 将该命令转为持久化 `rift.leave`；`endgame_rift_completion.go:67–80` 验证完成状态、当前对话目标、入场资格及200px距离。
- 测试：`TestRiftExitDialogueAndNativeModelScope`、`TestRiftNativeExitDialogueCommandFlow`，以及客户端出口对话/实际组合钩子契约。

### 6. Boss奖励掉地上拾取

**状态：已实现；待实机操作验收。**

- `server/internal/game/scene/endgame_rift_completion.go:193–249` 把已保存的每位成员奖励投影为个人地面物品，没有第二次随机生成。
- `endgame_rift_completion.go:319–364` 手动/宠物拾取走原生拾取包和事务；满包不会提前消耗奖励。
- `endgame_rift_completion.go:98–167` 选择可达、不重叠、避开出口NPC的落点。极窄地图放不下的物品保留为补领权益，不强行放进墙内。
- `server/internal/game/scene/endgame_rift.go:261–268` 阻止活跃完成实例内用整包领取绕过地面拾取，实例收束后恢复未领部分。
- 测试：`TestRiftGroundOwnershipDistanceBagAndRecovery`、`TestRiftPendingPickupDisconnectAndOfflineRemainder`、`TestRiftFiveMemberGroundLayoutAndExitDoNotOverlap`、狭窄地图和隔墙布局测试。

### 7. Boss经验＝对应试炼基础经验×0.1×秘境阶数

**状态：已实现。**

- `server/internal/game/scene/endgame_rift_rewards.go:116` 使用每名成员冻结等级的 `s.trial.BaseExperience(m.Level, s.levels)`，不是领取时等级，也不是怪物击杀经验或试炼轮次倍率。
- `server/internal/domain/endgame_rift_experience.go:7–20` 计算 `base * tier / divisor`，最终取整并处理溢出；`000000_init-63.sql:208` 中divisor为10。
- `endgame_rift_completion.go:368–393` 自动事务结算，不受背包容量影响，并标记经验已领取。
- 测试：经验边界测试、`TestRiftFrozenPersonalExperienceDoesNotUseClaimLevel`、`TestRiftCompletionGroundNPCAndExperienceExactlyOnce`。

### 8. 所有暗金地面掉落有光柱和落地音效

**状态：已实现全局路径；待实机画面/声音验收。**

- `server/internal/game/scene/ground_presentation.go:15–30` 以真实暗金实例身份判定，不把普通黄色装备误当暗金；生成落地token和fresh标志。
- `server/internal/game/scene/loot.go:337–386` 覆盖普通地图自然掉落与玩家丢弃；秘境奖励走 `endgame_rift_completion.go:238–246` 同一路径。
- `server/internal/protocol/ground_presentation.go` 与客户端 `Assets/FakeFantasyOnline/Runtime/Protocol/PrivateServerGroundItemCodec.cs` 传递/解码暗金身份及新落地事件。
- `GroundItemEntity.UniqueDrop.cs:18–48` 实际创建附着地面光柱、播放1002号音效；`EffectPlayer.GroundAttachment.cs` 复用 `object/DH/DH_FY_1501.gso`。
- `Assets/FakeFantasyOnline/Runtime/Presentation/ClientRootView.GroundReceive.cs:11–12` 与 `GroundItems.Spawn.cs:18–40` 接通收包→实体。重建/快照只保留光柱，不重复播放落地声音。
- 66项生产协议/音频边缘/特效生命周期契约通过；真实声卡、资源包和Unity绘制未运行。

### 9. 地图原传送门直接进入下一层，不先回大世界

**状态：已实现；待实机联机验收。**

- `PlayerController.TransferFrame.cs:50–55` 在原地图传送或脚本调用之前拦截秘境门。
- `EndgamePanel.RiftPortal.cs:9–29` 发出 `rift.advance`；loading、pending、Boss阶段等不会退回执行原地图门逻辑。
- `server/internal/game/scene/endgame_rift_floor.go:25–73` 识别服务器加载的原门几何以及种子出口；75–135行准备并持久化下一私有层。
- 只有开启者能触发全队换层；不重置难度、时间、累计进度，不再耗钥。
- `server/internal/game/scene/endgame_rift_portal_test.go:22–85` 的 `TestRiftNativePortalDirectFloorHandoff` 明确验证未创建大世界目标地图、未发大世界teleport、队伍一起转移及重复请求不跳两层。
- 客户端27项生产portal/context/transfer契约通过。

### 10. 种子多层地图与增加刷怪密度

**状态：已实现；待实机战斗密度体验验收。**

- `server/internal/domain/rift_floor.go:13–21` 为生成器v2；目标96～160只，实际数量受安全落点数量限制。
- `rift_floor.go:59–67,114–182` 每层由run/floor产生确定性种子，在实际合格地图中选择，存在其他地图时避开相邻重复。
- `rift_floor.go:183–229,251–277` 根据可走面积及种子变化选择数量，按4～8只的近邻组群方式排列。
- `server/internal/data/rift_floor.go:16–73,80–160` 使用真实地图、敌对出生点及碰撞MASK，验证连通分量；不是凭空猜测可走区域。
- 完整布局保存后才准入；楼层切换保留同一run的经济与时间状态。
- 测试：`TestRiftFloorDeterministicCanonicalPlan`、`TestRiftFloorSeedGolden`、`TestRiftFloorPopulationVariesAndUsesOnlySafeAnchors`、`TestRiftFloorDensityScalesWithAreaWithoutProgressCap`、存档兼容/重连/切层回滚测试。

### 11. 原试炼钥匙奖励规则，单次最高14把

**状态：已实现。**

- `server/internal/domain/rift.go:38–68` 计算：`1 + 本人Cycle(0..5) + 每位其他有效成员1 + 每位比本人低至少10级的成员再1`。
- 名册最多5人，因此最大 `1+5+4+4=14`；等级差恰好10计带新加成，差9不计。
- `server/internal/game/scene/endgame_trial.go:158–211` 在实际试炼完成路径冻结合资格成员与数量，187行调用此函数。
- `server/internal/game/scene/endgame_trial_claim.go:74–85` 消费冻结结果，数量防御性限制1～14，奖励钥匙绑定。
- 本次补跑 `TestTrialKeyMaximumAndMentorThreshold`、`TestTrialKeyExhaustiveBounds`、`TestTrialKeyInvalidRoster`，全部通过；原Rift suite同时覆盖 `TestRiftTrialClaimFrozenKeysAtomic`。

### 12. 每场只消耗开启者一把钥匙

**状态：已实现。**

- `server/internal/game/scene/endgame_rift.go:189,202–229` 指定开启者为payer，只在其快照副本扣1把，将背包、run创建和个人资料交同一提交；成功回调后才把预留实例标为paid并传送。
- 236–256行的加入/重进路径不扣钥匙；下一层路径也不改背包。
- `server/internal/store/endgame_backend.go:95–138` 在同一数据库事务中锁角色、查重receipt、校验revision、保存角色快照、run和receipt；发生错误不允许只扣钥匙而没有run。
- 已跑过的 `TestRiftStartOneKeyAndDefiniteFailureReservation` 验证提交前原包不变、成功只扣1、失败保留钥匙且清理未付费实例；`TestRiftNativePortalDirectFloorHandoff` 验证换层不再次扣钥匙。
- 本次检查了store事务代码，未单独重跑store包故障注入套件，不声称数据库部署集成测试已过。

### 13. 防止钥匙/通关奖励重复计算、重复领取

**状态：已实现。**

- 试炼完成 `endgame_trial.go:158–163` 通过Completed守卫只冻结一次；领取 `endgame_trial_claim.go:38,57–65,89–122` 使用固定nonce receipt、claimed标记及同一事务完成任务/发奖励。
- 秘境 `endgame_rift_rewards.go:20–36` 冻结确定性奖励；`endgame_rift_combat.go:238` 仅在GrantsReady为false时生成；地面只是同一已存奖励的投影。
- `endgame_rift_completion.go:196–246,262–307,319–393` 逐件Collected和ExperienceClaimed状态与角色背包/经验一起持久化。补领只处理未收部分。
- 本次已通过 `TestRiftTrialClaimFrozenKeysAtomic`、`TestRiftCompletionGroundNPCAndExperienceExactlyOnce`；后者明确检查重复同步、重复拾取、地面收完再claim均不追加奖励/经验。

## 测试执行记录

工具链：`source /workspace/shared/cloud-test-env.sh`。

1. 服务端：`go test ./internal/domain ./internal/game/scene ./internal/protocol ./internal/data -run 'Rift|Unique.*Drop|Ground.*Unique' -count=1`，四包通过。
2. 服务端：`go test ./internal/game/scene ./internal/protocol -run 'Ground|Unique.*Presentation' -count=1`，两包通过。
3. 客户端：`bash Tests/Endgame/run-ui-contract.sh`，2148项面板动作契约及149项source/metadata检查通过。
4. 客户端：`bash Tests/Endgame/run-module-contract.sh`，152项模块契约及同一149项source/metadata检查通过。149项不是另外一组不同测试，不重复计数。
5. 客户端：`bash Tests/Endgame/run-rift-exit-dialog-contract.sh`，对话和实际组合钩子通过。
6. 客户端：`python3 Tests/Endgame/run-rift-portal-contract.py --help`，实际运行了27项契约并通过；该脚本无参数解析，`--help`被忽略，并非只打印帮助。
7. 客户端：`python3 Tests/WorldEntry/run-unique-drop-presentation-contract.py`，66项契约及原生接线检查通过。
8. 补充钥匙：`go test ./internal/domain -run TrialKey -count=1 -v`，3个测试通过。

日志文件：
- `/tmp/rift-audit-20261013-focused-tests.log`：上述1～7的实际工具输出留档（先运行、后从本次输出保存，非重新执行结果）。
- `/tmp/rift-audit-20261013-key-tests.log`：第8项直接tee保存的完整输出。

编译警告仅来自契约替身字段未赋值（CS0649）；命令成功退出。没有运行新增长时间全量套件。

## 尚不能打勾的验收

- 基线真实八仙使者入口行为：部分完成，需单独补正与核对。
- 真正Unity资源加载、渲染、音效、输入体验：未运行。
- Unity PlayMode/player build：未运行。
- 同一真实客户端从入口→消耗钥匙→连续换层→Boss→拾取→出口→重连补领的完整联机验收：未运行。
- 线上数据库已应用规则版本/新构建已部署：本次未检查，不能从源码推断。

因此推荐结论为：**主要需求已落入源码并通过针对性测试；实际入口缺口和实机端到端验收仍应分别确认，不能笼统说全部做完。**
