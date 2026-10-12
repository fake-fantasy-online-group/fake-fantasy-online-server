# 原职业技能客户端最小恢复方案

状态：方案与服务端契约协商中，尚未修改业务源码。目标是在服务端既有技能曲线不变的前提下，让客户端的名称、完整效果、MP、目标/范围、学习资料统一使用已验证的服务端运行结果；保留20硬点、高等级装备成长和训练协同。

## 取证确认

- 取证包SHA256：`8ad3b25e36bdad4f9b7c45e8dba8f49ecd5ba00dc088fcef73fbf4cf0d7b62da`，本次已核验。
- current源文件SHA256：`ab55d4de9bbba51f2960f7c34dc5803eec0a9d8fc674bafd3de83b0dc9d47886`。
- historical_backup源文件SHA256：`2b85db4ba9de1c9efe48121875d08637b6d1c10a1a7f98783bbe3135b184e3f0`。
- 职业表实际为历史149行、current146行。共同ID中68条变化：战士15、剑客13、刺客10、药师18、术士12；另缺药师10316/10318/10321。不要再称职业表150行。
- 本方案只提交派生语义、ID及必要事实，不复制任何完整资源JSON、逐级原文或素材进GitHub。资源原件及投影留在取证位置。

## 1. 恢复原则与最小边界

### 不是整表回滚

历史备份是1.0/build0、ad-hoc签名的本地材料，未获官方原版认证；里面自身就存在分歧：

| ID | 已读到的内部冲突 | 实施约束 |
|---|---|---|
| 10006 | 原说明半径150，raw radius200 | 由已核对服务端语义/历史版本决定，不照抄raw |
| 10029 | base描述吓跑且尚无法作用，逐级描述震晕5秒，radius150 | 不恢复“无法作用”作为执行逻辑，也不能凭一句文案选效果 |
| 10121 | 逐级说明半径100，raw radius150 | 使用typed实际结算范围 |
| 10130 | 说明为下一次普攻增伤，raw却shape2/radius150/canEnemy1 | 不能回滚成群攻；需保持实际自身蓄力语义 |
| 10212 | base是偷窃概率，逐级desc却是普通伤害 | 不能把逐级desc无条件当真值 |
| 10227 | 声称能跳敌人或队友，但canFriendly0，且附尚无法作用 | 明确选定版本/当前已验证语义，不能机械恢复缺陷 |
| 10321 | base为提高防御，lv1为所受伤害降低 | 按已核对独立防御术契约恢复，不与祝福术合并 |
| 10327 | desc范围300，raw radius350 | 破隐范围与画圈范围需统一于实际typed字段 |
| 10409 | base伤害165%，lv1却100% | 由服务端已验证原始曲线提供，客户端不自己选数字 |

数据优先顺序是“已明确选择版本且有证据的原技能语义→现有服务端实际结算定义→客户端契约呈现”，不是“最早文件名→官方真值”。字段冲突保留记录，由配套来源矩阵裁决。本次不改变服务端曲线，不删除用户要求的扩展成长或hard-point协同。

### 68变化 + 3缺失如何被真正覆盖

- 8个改名：客户端标题不再使用污染资源name，显示服务端identity。
- 25项base desc与35项levels.desc（重叠）：卡片保留server原完整current/next说明，客户端不再用外部desc决定目标/形状，也不BakeDesc重算成长。
- 19个目标/几何项：改用typed输入模式、目标权限、中心、距离、形状及范围；必须区分原raw形状缺失和真实无范围。
- 57项levels变化（覆盖多个子字段）：15吟唱、15冷却、12学习等级、2MP、35逐级desc；现有server效果文本可保留，缺失的typed MP/学习/目标从同一运行def投影。
- 3前置项：10307/10320/10324。只采用server已验证的前置契约；不能因重新加入10321就自动套回历史pre，而忽略版本/技能树变化。
- 缺失3项：独立元数据、图标ID和技能节点要能呈现；不能依赖current中不存在的SkInfo。恢复独立效果而非继续合并在净世/祝祷里。

## 2. 服务端契约：最小增量，保持旧协议

拟协商能力`endgame.skill-contract.v1`。现客户端会拒unknown JSON key，因此server必须仅对声明并接受此能力的会话发扩展字段；旧端收到原skill.ranks结构，原0x8015/0x800e布局不变。

在现有rank行内增加可选只读view即可，不必重新发送资源表或重建149份客户端公式。准确字段名待服务端工作同伴对齐，但信息必须涵盖：

- name、iconId、known/active/是否可上快捷栏；ID已有。
- current实际MP、吟唱/冷却毫秒；未学时预览使用next已存在段，不伪装成当前可施法成本。
- inputMode：instant/single/ground/direction；targetMask；anchor/center；rangePx；areaShape/radiusPx/lengthPx/widthPx/angleDegrees；isTrap；是否自中心。不能仅传SQL target数字让客户端继续猜。
- 学习与前置：既有CanInvest、NextRequiredLevel继续用；补typed prerequisites/blocked reason code或明确结构，而非从中文说明猜前置。
- definitionVersion/sourceProfile与有效状态revision。有效rank仍保留十进制字符串，不能缩到byte或按资源最大lv截断。

投影源是实际施法的Get(effective rank)并应用training/passive/unique修正后的def。不要从CurrentEffectLines里解析“消耗”和“半径”。原卡片文字仍server生成并保留native布局；typed字段供行为。已经正确的V5 curves、soft/grant/hard分离、incoming协同原封不动。

## 3. 客户端一次性改动范围

新增一个轻量`ResolvedProfessionSkill`/provider（最终命名可调整），集中选择“已协商且当前server契约”与旧协议路径；不原地改raw SkInfo，避免静态cache跨角色/服务器污染。

| 代码表面 | 最小变更 |
|---|---|
| A/EndgameSkills.cs、EndgameClientState.cs、EndgameCommands.cs | 新cap与可选typed DTO，严格验证数值/枚举/单位，generation/revision原子发布 |
| P/Win05.GrowthSkills.cs | name来自server，保留原current/next完整块、配色、footer、协同位置；active信息来自typed view |
| P/Win05.SkillDialog.cs、Win05.SkillData.cs | growth入口不先要求raw info；缺3项可取view身份/icon；正常原有节点布局仍用素材 |
| P/GameHud.HotbarAvailability.cs、HotIcon.cs、HotSlotTip.cs | 同一view判定学会/active，支持缺raw行图标与卡片；不缩有效rank |
| P/NetClient.SkillCommands.cs | 有当前typed MP时用实际成本；新能力下未知/过期成本不准退旧资源硬拦截，交server最终判断并刷新 |
| P/SkillTargeting.Classification.cs、Validation.cs、Entry.cs | 新能力从typed模式/权限/几何决定输入，不读中文desc；原协议兼容分支保留 |
| P/GameHud.HotbarUseAdmission.cs、CombatWorld.SkillRequests.cs、PluginGameApi.Actions.cs | 同一输入语义；友方单体/自身/地面不各自猜targetId和range |
| P/ReferenceWorldReceiveOwner.SkillCastReceipt.cs、CombatWorld.Cast.cs、CombatWorld.Traps.cs | 朝向、支援特效、范围圈用view；live cooldown/chant继续以server回执为准 |
| P/DialogUI.Ascend.cs | 新成长路径不显示资源驱动过时“1/2亿+N丹”承诺；走已有成长学习或清晰说明该入口不适用 |
| P/NativeSwordSkillTargeting.cs相关调用 | typed路径不再需要30-ID改写；旧兼容路由是否保留由来源裁决，不在本次盲删 |

`P/`=`Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/`，`A/`=`.../Application/`。

### 缺3项的当前真实表现与修复

已经确认current不存在10316/10318/10321；它们仍在149 whitelist和server identity里。仅补名称远远不够：

- 如果skilldlg_layout仍含slot，`Win05.SkillDialog:636-644`会用icon0绘制；V5等级标签可在645出现。
- 加号检查在660、info-null检查在677，且CanShowUpgrade:429先走V5，所以节点可能仍能升级，但没有正常图标/hover/pickup。不能笼统说整技能完全隐藏。
- `SkillTipText:415-417`在raw info为空时直接return，热键tooltip在`HotSlotTip:49`降成“技能 10316”等。
- 树pickup被info-null和SkillUsableForHotbar挡住。已保存热键若旧技能快照仍有等级，可以继续施法，但Resolve因info-null返回Instant，targetId0；LocalNotEnoughMp缺info直接放行，选队友语义丢失。
- HotIcon有server给定icon可继续画；无给定icon时SkillIconId=0。修复provider需保证三项的icon ID及资源是否存在，不能用ID猜路径。
- 当前证据包不含skilldlg_layout，主任务已委托现有本机只读任务核对3项slot/tab/tree/位置；若slot缺失，须通过经过核对的3项最小节点语义补充显示，不复制整张资源表或杜撰坐标。

### 两个可复现的MP错误案例

current在10306的rank1..5都写MP100，10307都写MP120。服务器基础rank1成本由同伴交叉核对为48与35，且实际还可能受协同减耗：无额外修正的rank1角色MP60使用10306，或者MP50使用10307，当前客户端会先拒绝，而服务器成本条件本可通过。实际回归测试应直接构造server最终def fixture与CharData，不依赖这些中文说明。

## 4. 保留成长与协同的验收约束

1. Base skill restoration与progression layer分离：不把maxlv回滚为5，不复制备份lreq6..10替换V5硬点政策；选定base unlock/pre后仍通过现有20硬点/100角色封顶规则生成next资格。
2. 高等级效果依旧由server V5曲线生成；装备soft/grant可高于20；协同仅吃hard points，不把装备等级当训练投入。原incoming协同版式保持。
3. 不能用modified技能名/新机制替换原技能。已核对的枪势/破军/剑意/布雷/净世/祝祷/照亮等改写不应作为基线；但新增协同独立附加存在，不与旧技能自身被动混淆。
4. 68个changed ID和3个removed ID全部需有行级执行结果，不能只测两剑客。其余78共同未变行也必须回归确认。
5. 新新、新旧、旧新三种协议组合，hello/rank延迟/丢包、换装、跨图、重连和不同角色必测；资料未知不混回旧成本或猜范围。
6. 原卡片Render/Layout/ComposeTip golden保持；名称、当前/下一全文、主动开销、前置与剩余点数正确，长卡片仍用现有滚轮UI。

## 5. 71项实现覆盖清单

这是待实现的派生分工表，标记修复应触及的消费者，不宣称全部历史值必须原样回滚。精确字段差异和来源裁决由配套全职业矩阵保存。

| ID | 历史技能身份 | 差异 | 必须覆盖的数据/行为 |
|---|---|---|---|
| 10006 | 震地之枪 | 变化 | 原完整效果段/语义 |
| 10008 | 意志 | 变化 | 原完整效果段/语义 |
| 10012 | 破甲 | 变化 | 原完整效果段/语义 |
| 10014 | 破魔 | 变化 | 权威名称；原完整效果段/语义 |
| 10015 | 心眼之枪 | 变化 | 原完整效果段/语义 |
| 10016 | 石化之枪 | 变化 | 学习等级 |
| 10022 | 野战八荒 | 变化 | typed目标/几何 |
| 10023 | 战神之枪 | 变化 | 吟唱 |
| 10024 | 维护 | 变化 | 权威名称；原完整效果段/语义 |
| 10025 | 强健 | 变化 | 原完整效果段/语义 |
| 10026 | 抗魔 | 变化 | 原完整效果段/语义 |
| 10027 | 重击 | 变化 | 原完整效果段/语义 |
| 10028 | 战吼 | 变化 | typed目标/几何；学习等级 |
| 10029 | 震慑 | 变化 | 原完整效果段/语义；冷却；学习等级 |
| 10030 | 缴械 | 变化 | 学习等级 |
| 10102 | 罡气剑 | 变化 | 原完整效果段/语义；typed目标/几何 |
| 10104 | 饮血剑 | 变化 | 原完整效果段/语义 |
| 10105 | 龙杀剑 | 变化 | 吟唱；冷却 |
| 10108 | 心镜 | 变化 | 原完整效果段/语义 |
| 10115 | 火字诀 | 变化 | 原完整效果段/语义；吟唱；学习等级 |
| 10118 | 风斩 | 变化 | 权威名称；原完整效果段/语义 |
| 10121 | 旋风剑 | 变化 | 原完整效果段/语义；学习等级 |
| 10122 | 混沌剑 | 变化 | 原完整效果段/语义；typed目标/几何 |
| 10123 | 陨星剑 | 变化 | 原完整效果段/语义 |
| 10124 | 山字诀 | 变化 | 原完整效果段/语义；吟唱；学习等级 |
| 10125 | 血雨剑 | 变化 | 吟唱；冷却 |
| 10127 | 挑衅 | 变化 | typed目标/几何 |
| 10130 | 雷霆之怒 | 变化 | typed目标/几何 |
| 10203 | 反击术 | 变化 | 原完整效果段/语义 |
| 10212 | 偷窃术 | 变化 | 原完整效果段/语义 |
| 10213 | 陷阱：爆弹 | 变化 | typed目标/几何 |
| 10214 | 陷阱：毒雾 | 变化 | typed目标/几何 |
| 10215 | 陷阱：封印 | 变化 | typed目标/几何 |
| 10216 | 陷阱：冻气 | 变化 | typed目标/几何 |
| 10218 | 解除陷阱 | 变化 | 权威名称；原完整效果段/语义；typed目标/几何 |
| 10227 | 阴影跳跃 | 变化 | 原完整效果段/语义；typed目标/几何 |
| 10228 | 侦测陷阱 | 变化 | typed目标/几何 |
| 10229 | 陷阱回避 | 变化 | 权威名称；原完整效果段/语义 |
| 10302 | 复法术 | 变化 | 原完整效果段/语义；冷却 |
| 10303 | 回春术 | 变化 | 原完整效果段/语义 |
| 10304 | 群疗术 | 变化 | 吟唱；冷却 |
| 10306 | 净化术 | 变化 | 权威名称；原完整效果段/语义；MP预检；学习等级 |
| 10307 | 祝福术 | 变化 | 权威名称；原完整效果段/语义；MP预检；学习等级；前置 |
| 10308 | 神圣庇护 | 变化 | typed目标/几何；吟唱；冷却 |
| 10309 | 光之奇迹 | 变化 | 吟唱；冷却 |
| 10311 | 加速术 | 变化 | typed目标/几何 |
| 10312 | 驱魔术 | 变化 | 原完整效果段/语义；冷却 |
| 10313 | 天罚 | 变化 | 原完整效果段/语义；typed目标/几何；吟唱；冷却 |
| 10314 | 术法精研 | 变化 | 原完整效果段/语义 |
| 10315 | 瞬击术 | 变化 | 原完整效果段/语义；typed目标/几何 |
| 10316 | 清身术 | 缺失 | 补身份/图标/节点；恢复独立效果；学习/友方施法 |
| 10317 | 圣光术 | 变化 | 原完整效果段/语义；typed目标/几何 |
| 10318 | 解咒术 | 缺失 | 补身份/图标/节点；恢复独立效果；学习/友方施法 |
| 10320 | 光盾术 | 变化 | 前置 |
| 10321 | 防御术 | 缺失 | 补身份/图标/节点；恢复独立效果；学习/友方施法 |
| 10324 | 衰弱术 | 变化 | 前置 |
| 10326 | 神威术 | 变化 | 冷却 |
| 10327 | 神术：鹰眼 | 变化 | 权威名称；原完整效果段/语义；typed目标/几何 |
| 10330 | 针刺护体 | 变化 | 吟唱 |
| 10401 | 火弹术 | 变化 | 吟唱 |
| 10402 | 火球术 | 变化 | 吟唱 |
| 10405 | 风焰术 | 变化 | 冷却 |
| 10408 | 陨石术 | 变化 | 吟唱；冷却 |
| 10409 | 冰箭术 | 变化 | 原完整效果段/语义 |
| 10411 | 绝对零度 | 变化 | 吟唱；冷却 |
| 10412 | 暴雪术 | 变化 | 冷却 |
| 10414 | 虚空 | 变化 | 吟唱；冷却 |
| 10417 | 魔镜术 | 变化 | 学习等级 |
| 10424 | 红莲之魂 | 变化 | 学习等级 |
| 10431 | 寒冰之魂 | 变化 | 学习等级 |
| 10433 | 秘术：消除 | 变化 | 原完整效果段/语义 |

## 待确认/实施门槛

- 服务端同伴锁定capability与typed schema，确保发送给旧端的结构完全不变。
- 本机确认三个缺失技能的layout/art节点是否仍存在；无需重复下载/提取全部资源。
- 主任务确认以已验证服务端原技能语义为base、保持V5成长/协同的恢复策略后再修改业务源码。

本方案未把任何资源原件或完整metadata dump写入repo，未对业务源码做修改。
