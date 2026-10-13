# 客户端全五职业技能覆盖与行为链审计

初始审计时间：2026-10-12 UTC。下述结论为只读基线审计；随后获批实现的状态列于文末与配套实施方案。客户端 `feature/daoist-endgame` / `66c7712170d4a6eb73da828f870026af7089c401`；交叉参照服务端 `d9c06ddd53bbb777bc87248b881871ff64d2f947`。客户端初始提交 `f8d8885` 已纳入审计，不把 V5 前后 diff 当全部范围。

## 结论

1. 最新改动并非只有 10102/10122 两个单体转群攻问题。`NativeSwordSkillTargeting` 对完整剑客 30-ID 进行资源加载后覆盖，同时改变两个 Loader、瞄准分类与兼容效果目录；来源是另一私服的初始 SQL，不足以自行证明官方原版。
2. 五职业仍有跨源混合：技能卡效果由服务器文本提供，但名称、是否可上热键、MP 本地拦截、目标/范围/几何、特殊瞄准、飞升学习等仍读外部静态资源。多数机制在 `f8d8885` 已存在。
3. 最高优先级一致性缺陷是 `LocalNotEnoughMp`：按 0x8015 的永久硬等级查询外部 `levels[].mp`，而真实结算按有效等级和协同/被动/装备修正。当前 V5 DTO 根本没有 typed MP/target/geometry 可供正确替代。任意新说明文字都不能解决发包前的错误拒绝。
4. 应在资源 projection/版本定位后建立服务端实际结算派生的能力协商 typed contract，再由单一客户端解析入口供卡片、热键、MP 与目标逻辑使用。保持原卡片版式。不能把说明字符串当数据协议，也不能把最早 SQL 自动改名为官方数据。

## 证据边界与等级

- **A：运行源码确定**。已读取目标 HEAD 的执行分支。描述“会如何执行”确定，不等于已运行 Unity 验证。
- **B：历史确定**。已用 `git show f8d8885:path` 与 HEAD 字节比较/逐文件日志确认引入边界。
- **C：来源待认证**。代码中的 Official/Original/Native 注释及 server 初始 SQL，仅表明作者声明或交叉参照，不是独立官方证据。
- **R：资源待 projection**。每个 ID 的实际资源字段/命中 fallback 需本机只读导出的版本、hash、字段投影；本审计没有下载、提取或分发资源二进制，也没有把缺失的字段臆造为 0。

README 明确说该项目为独立实现，源码完整不等于与参考客户端全量等价。源码注释同时引用 1.5.8 和 1.6.0，目标资源版本还需对齐。未发现适用 AGENTS.md。07:27 UTC 云环境替换后由主任务重新 clone 两个相同 HEAD，之后继续审计；下述路径/行均已在恢复后的代码核对。

下文客户端短路径 `P/` 指 `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/`；`A/` 指 `Assets/FakeFantasyOnline/Runtime/Application/`；`N/` 指 `Assets/FakeFantasyOnline/Runtime/Protocol/`。服务端路径从 server repo 根开始。

## 全职业覆盖边界

`A/EndgameSkills.cs:10-26` 的运行 whitelist 为以下 **149** 个 ID；后续完整projection已确认历史149/current146，三项缺失均为药师；不存在需要臆造的第150条职业技能。特别是 10323 不在该名单，术士 10403/10404/10406/10407/10434 也不在；不能无来源补齐连续区间。

| 职业 | whitelist 数量 | 完整 ID 清单 |
|---|---:|---|
| 战士 | 30 | 10001, 10002, 10003, 10004, 10005, 10006, 10007, 10008, 10009, 10010, 10011, 10012, 10013, 10014, 10015, 10016, 10017, 10018, 10019, 10020, 10021, 10022, 10023, 10024, 10025, 10026, 10027, 10028, 10029, 10030 |
| 剑客 | 30 | 10101, 10102, 10103, 10104, 10105, 10106, 10107, 10108, 10109, 10110, 10111, 10112, 10113, 10114, 10115, 10116, 10117, 10118, 10119, 10120, 10121, 10122, 10123, 10124, 10125, 10126, 10127, 10128, 10129, 10130 |
| 刺客 | 30 | 10201, 10202, 10203, 10204, 10205, 10206, 10207, 10208, 10209, 10210, 10211, 10212, 10213, 10214, 10215, 10216, 10217, 10218, 10219, 10220, 10221, 10222, 10223, 10224, 10225, 10226, 10227, 10228, 10229, 10230 |
| 药师 | 29 | 10301, 10302, 10303, 10304, 10305, 10306, 10307, 10308, 10309, 10310, 10311, 10312, 10313, 10314, 10315, 10316, 10317, 10318, 10319, 10320, 10321, 10322, 10324, 10325, 10326, 10327, 10328, 10329, 10330 |
| 术士 | 30 | 10401, 10402, 10405, 10408, 10409, 10410, 10411, 10412, 10413, 10414, 10415, 10416, 10417, 10418, 10419, 10420, 10421, 10422, 10423, 10424, 10425, 10426, 10427, 10428, 10429, 10430, 10431, 10432, 10433, 10435 |

对全部上述 ID 已覆盖以下代码路径：数据加载与缓存、成长/旧式卡片、学习及前置、热键 availability/pickup/use、直发/地面发包前 MP、普通/插件目标请求、瞄准几何与描述分类、回执的朝向/范围圈/支援特效、脚本/表格效果选择、技能元素与声音。资源值逐 ID 实际偏差属于配套 projection 差异矩阵，而不是本报告已证实全149被改写。

## 1. 硬编码与资源覆盖

### C01：整套剑客加载后改写（A+B；“原版”归属 C）

`P/Win05.SkillData.cs:60-62,113-115`：`LoadSkillData` 和 `LoadStatic` 都在反序列化后、写入字典前调用 `NativeSwordSkillTargeting.Apply`。

`P/NativeSwordSkillTargeting.cs:27-50,54-74` 的完整规则：

| IDs | 强制规则 |
|---|---|
| 10101,10106,10110,10112,10118,10120 | tgt18，castkind0；清除范围几何 |
| 10108 | tgt18，castkind0；声明 distance10，但 Apply 不覆盖此非敌方 dist |
| 10102,10103,10104,10111,10113,10116,10119,10122,10123,10126,10129 | tgt1、敌方单体、dist75、castkind1 |
| 10105 | tgt17、敌方单体、dist400、castkind1 |
| 10117 | tgt1、敌方单体、dist300、castkind1 |
| 10107,10109,10114,10130 | tgt1、canSelf1、castkind2；声明 distance1，不写入资源 dist |
| 10115 | tgt1、canSelf1、castkind2；声明 distance0，不写入资源 dist |
| 10124 | tgt1、canSelf1、castkind2；声明 distance100，不写入资源 dist |
| 10121,10125,10127,10128 | tgt2、canEnemy1、shape2；保留资源 dist/radius/castkind/rwidth/rectl/rectw |

所有命中 ID 都覆盖 `tgt,canSelf,canFriendly=0,canEnemy`。非自中心者还覆盖 `castkind,shape=0,radius=0,rwidth=rectl=rectw=0`；敌方单体还覆盖 dist。对四个自中心技能，Rule 中半径 100/150/200/100 只是记录，**不会写入实际 radius**，因此仍是混合来源。

`P/SkillTargeting.Classification.cs:31-40,103-106` 又按该 ID 表决定 Single/Instant 和 SelfCenteredCircle，绕过描述/shape。来源注释 :4-7 指 `server a9c747c/init-31`，不是已认证官方数据。

`P/ReferenceWorldLocalSkillCast.cs:229-240` 在另一目录重复剑客覆盖，保留自中心 dist/castkind。**可达性限制：全 Assets/FakeFantasyOnline 静态调用搜索未找到 `ReferenceSkillEffectCatalog.TryCreate`、`ShouldAimAtResolvedTarget`、`ResolveImmediateTargetEntityId` 的生产调用点；当前主要由测试直接调用。**它是必须维护的兼容/诊断表面，但不能把它当成已经证明影响当前 Player 的第二结算入口。

建议：待 typed contract 完备后，能力开启的路径不再修改原始 `SkInfo`，而读取不可变 resolved view。旧兼容路径是否保留此补丁须由版本证据决定，不能一刀删除造成已修复场景回退，也不能扩大到全部职业靠猜。

### C02：读取路径和缓存可能产生不同资源视图（A+B；实际冲突 R）

- `P/GameTables.cs:14-48`：只从 `StreamingAssets/gameassets/tables/{name}.json` 优先读，失败才 `Resources.Load<TextAsset>(name)`；根数组缓存。没有版本/hash验证。
- `P/Win05.SkillData.cs:33-38,45-69,99-118`：一次加载；缺文件后可留下空字典且不重试。重复 ID 后行覆盖前行。更换服务器/角色不会自动换该静态表。
- `P/ReferenceResourceLoader.cs:401-418,508-522`：`ReadSourceOwned("tables/skills.json")` **先** `core.ReadRes`，只有无 bytes 才尝试 loose tables。
- `P/ResLoader.cs:317-345,348-408,782-787`：优先 `gameassets/respatch`，然后 Pkg，再 res24/Pkg24；first patch duplicate wins，索引一次建立。因此不是注释所暗示无条件与 GameTables 共享一个来源。
- `P/ReferenceWorldLocalSkillCast.cs:17-19,189-217` 硬编码要求 skills222、skillshow675、physical230；缺行/重复/物理数不同即目录创建失败。它的“Hash-locked”注释没有对应的表 hash 校验。需区分本兼容目录与现行可达路径。
- `P/ReferenceFirstWorldHotbar.cs:13-15,76-101` 的旧图标兼容目录也独立读 `ReadSourceOwned`；文件注释指出 live slots 使用 GameHud.HotIcon，不应把兼容目录视作活跃 UI。

建议：只保留一个明确记录 provenance/version/hash 的静态资源 provider；使用 server contract 的 gameplay view 与本地图标/美术分离。不得用缺失后缓存空表掩盖错误。

## 2. 五职业施法准入与目标/范围

### C03：MP 拦截永远沿用硬等级外部表（A+B；高优先级）

`P/NetClient.SkillCommands.cs:37-56,59-65,78-84`：所有职业的 `UseSkill` 与 `UseSkillAt` 在发包前调用 `LocalNotEnoughMp`。

- 查 `CharData.skills[skillId]`，只匹配 `info.levels` 中 `row.lv == level`。
- 命中且 `row.mp > 0 && CharData.mp < row.mp` 立即返回“法力不足”，服务器根本收不到指令。
- 无等级、缺表、缺精确等级行均返回 false，没有成本检查；V5 当前20硬点或高装备等级与旧表并不闭合。
- 没有 `GrowthV5` / effective rank / 训练减耗 / 装备修正分支。

交叉参照并非官方依据：server `server/internal/game/scene/dispatch.go:668-699` 的 `clientSkillLevels` 为硬点 clone + 法宝1级；`skill_upgrade.go:104-125` 编成0x8015。真实服务器 `skill.go:52-93` 使用 `skillLevelForUse → skills.Get → applyTrainingSynergies → applyPassiveSkillModifiers → applyUniqueSkillModifiers → MPCost检查`。训练减耗还明确在 `skill_progression_v5.go:130-146` 影响 MPCost/吟唱/范围。

因此可以有两类可证明的条件反例：旧资源成本高于实际修正成本时，客户端错误拒绝；资源较低/缺行时，客户端放行再由服务器拒绝。此处未凭空选某个实际玩家状态，具体可复现实例由资源 projection/实际装备 fixture 提供。

### C04：热键、树和插件的“已学/可用”源不同（A+B）

- `P/GameHud.HotbarAvailability.cs:8-20`：普通热键只认 CharData.skills正等级；`HotbarUseAdmission.cs:12-23` 先以它拦截，再检查回执驱动 cooldown。
- `P/Win05.SkillDialog.cs:362-373,637-643,682-685`：树 pickup/canUse 在成长模式用 rank.CanCast；`SkillUsableForHotbar:937-939` 仍以资源 tgt!=18/type!=19/20 决定能否上栏。
- `P/PluginGameApi.Actions.cs:442-445`：成长模式按 rank.CanCast，未要求资料 revision 当前；未达 rank 时当未学。
- `A/EndgameSkills.cs:59-65` 的 `CanCast` 实际约束只是 EffectiveRank非0，**不是**“MP足够、非沉默、冷却已到、武器符合等实时准入全通过”。不能把字段名当所有可施法条件。

建议：为 known/learned/active/use eligibility 设置明确语义，一份来源驱动树、栏、插件；仍由服务器在施法时校验动态条件。

### C05：描述解析和默认几何改变输入行为（A+B；哪些实际 ID 命中为 R）

`P/SkillTargeting.Classification.cs`，除了剑客特判外：

| 行 | 规则 | 影响 |
|---|---|---|
| 26-29,107 | 10211 强制 Instant，SelfCenteredCircle=false | 特定刺客技能无视资源类别 |
| 8-11,42-48 | desc含“放置” | Circle；range至少150，radius缺失100；isTrap=true |
| 9-11,49-54 | desc含“移动至指定” | Circle；range至少500，radius缺失40 |
| 11,55-59 | desc含“跳跃至选中” | Single；range缺失500 |
| 61-64 | castkind0或canEnemy0 | Instant |
| 69-76 | shape1 | Direction；rectL缺失150，rectW缺失70，range缺失用rectL |
| 77-84 | shape2且dist>0且描述非自中心 | Circle；radius缺失140，再加自定义地图bonus |
| 85-93 | shape3 | Direction；angle缺失120，radius缺失140，range缺失用radius |
| 94-99 | 其他shape | Single；range缺失600 |
| 118-131 | desc含“以自己为中心/自己为中心/自身为中心/自己周围/自身周围/身边/周围” | 自中心；泛词“周围”也会命中 |
| 134-136 | CustomMaps.IsCustom | radius加ZoneOverlay.faShenPx |

数值常量在 `P/SkillTargeting.cs:24-26`（DefaultRange400、DefaultRadius140、ServerSingleRangeFallback600）。`Entry.cs:25,57` 对非正range再fallback400；`Geometry.cs:66-67` 绘制端再fallback半径140/半角1.0471976弧度。

所有这些非剑客规则在 f8d8885 已存在，不能从“最近没有修改”推断其官方正确。文本可能说“对目标周围”而被判“自己周围”，但未拿到具体资源行前只标风险，不宣称某个 ID 已发生此错。

### C06：入口之间还存在不同目标和距离规则（A+B）

- `P/CombatWorld.SkillRequests.cs:27-70`：PvP target与友方 RemotePlayer 分支直接发包；普通队列把非正 range fallback为100，并等待8秒。不同于 classifier缺失range600/地面400。
- `P/SkillTargeting.Validation.cs:6-12`：NPC禁止；敌对对象不检查 canEnemy直接通过；友方需canFriendly==1。
- `P/GameHud.HotbarUseAdmission.cs:57-84`：Instant友方技从当前玩家或队伍选择找目标；敌方且shape非0才从当前怪物带ID。若选择敌对玩家不允许会提示，但仍可能继续 targetId0发包，最终语义由server判定。
- `P/PluginGameApi.Actions.cs:466-480`：`skill.request` 直接把info.dist交给RequestCharacterSkill，不调用Resolve；坐标/实体直发走不同入口。缺资源时range0会在队列成为100。
- `P/SkillTargeting.Entry.cs:68-84,132-148`：Circle远距离会接近；Direction不做同样的距离接近检查；传UseSkillAt目标ID0。

建议：归一化输入契约应显式包含 select/input mode、anchor/center、target permissions、range，而不是只有一个shape枚举。hotkey/plugin/direct request共用适配层，避免只修Resolve留下其他入口。

## 3. 卡片、等级、前置与技能学习

### C07：当前成长卡片仍是部分权威、部分外部资源（A）

`P/Win05.GrowthSkills.cs:9-68`：成长state存在+149 whitelist即切换；标题一直`info.name`，current/next effect lines来自server；有效等级是字符串，未转int。rank缺失显示“正在等待服务器技能资料”。

- :28-38：未学习显示next全文，已学习current全文；下一等级只在HardRank<SkillHardCap且NextEffectLines非空时画，符合20硬点扩展的行为，但不是资源maxlv。
- :71-74：学习要求只显示NextRequiredLevel；BlockedReasons未展示具体前置原因。
- :47-52：Pending/Revision差异只在技能点footer标“确认中/更新中”，旧effects仍可显示。
- :76-77,79-100：按中文前缀“已学被动使本技能伤害”将NativeModifierLines分组；是描述驱动格式逻辑，不是typed effect类别。
- `Win05.SkillDialog.cs:636-643`：有效等级标签超过5字符截前4+省略号，卡片内完整字符串仍保留。
- `SkillTipText:415-417`：没有info返回null，即使server已有rank仍无法显示；树slots/技能icon也依赖外部skilldlg_layout和skills。

建议：不换UI框架、不改变原中文行/颜色/下一等级全文结构；名称/成本/已学/前置/属性仅换数据源，仍用现有ComposeTip/Layout/Render。仅在资料确实未知时标明未知，不伪造为0/无/瞬发。

### C08：旧卡片数字替换、吟唱及前置fallback是初始版本自带（A+B）

- `P/Win05.SkillLevelBlock.cs:15-20`：硬编码 `0x284a=10314` 与 `0x28af=10415`（药师/术士），已学等级>0时 reduction=max(15+5*level)，上限80%。:39-43 仅卡片端cast至少200ms，cd以0.1秒单位显示。
- :30-38：精确lv行优先；找不到desc用info.desc；缺row的MP显示“无”、cast显示“瞬发”、cd显示0秒，不保证服务端真实。
- `P/Win05.SkillTipText.cs:41-56`：Regex匹配“，每次升级…N%”，找此前最后一个百分比，改成base+step*(level-1)，删除升级短语。针对任意命中描述的技能，可能重复处理已按等级写好的文案；需projection确定。
- `P/Win05.SkillDialog.cs:426-437,874-915`：非V5的maxlv/pre/lreq均来自资源；pre只需前置任意正等级；已学后跳过前置；下一等级缺行则允许，lreq999当无要求。提示只有泛化不能升级。
- `P/DialogUI.Ascend.cs:8-64`：无V5分支，按resource maxlv/lreq + CharData硬等级构建高阶学习菜单；70入口接受70..79，但剩余统计只数lreq==70；至多12选项；固定1/2亿经验和N丹说明。即使server拒绝该V5学习方式，UI仍可能显示过时承诺。

建议：旧协议保留兼容语义并显式版本化；新契约下前置/升级资格及成本只由server typed state决定。恢复目标是原内容与实际行为一致，不是把未知数据降级为“无”。

### C10：缺失资源行会使服务器资料也无法画卡片（A；具体缺行待取证包复核）

主任务于07:37 UTC报告本机只读projection：当前资源缺药师10316/10318/10321，并在1.0/build0历史备份与current全职业比较中发现68条差异；该备份仍属ad-hoc历史材料，不能先叫官方基准。本审计尚未直接读取完整取证包，不把这些数目列作独立验证结果。

可以立刻由已读源码确定的后果：`Win05.SkillDialog.cs:415-417`先查SkillInfo并在null时return，之后才看GrowthState；`:677-679`在info缺失时跳过hover；`SkillUsableForHotbar:937-939`要求info非null。因此这三个ID若确实缺行，即使V5 rank完整也无法由当前路径补出卡片/正常从技能树拾取。已存在的热键发包是另一条路径，不能因此宣称该技能所有入口必定不可用。

最小修复应允许权威或经认证的原始profile提供缺失的identity/name/active元数据；原始资源icon/layout可用时复用，不得以缺SkInfo为由把可用server契约整条丢弃。原始profile应完整列出五职业支持ID、来源版本/hash和已确认差异，再提供统一view；不要只逐行修复已有JSON中的两个ID。若仅有历史备份可信度不足，先以“历史候选profile”命名并保留未认证标记。

## 4. 施法效果、音效和表现

### C09：视觉效果不能只看技能卡描述（A+B）

- `P/ReferenceWorldReceiveOwner.SkillCastReceipt.cs:24-28,38-65,71-106`：动画physical来自SkillEffectDB；cooldown和吟唱直接取服务器回执，**当前live cooldown并非从资源cd起算**。朝向用resource shape/dist/tgt/castkind/canEnemy，范围圈用SelfCenteredCircle与resource.radius，支援FX用canFriendly/canEnemy。因此不要为修MP顺带把回执驱动cooldown替换成预测文本。
- :48和:76的centeredShape=`shape2 && dist<1` 与SelfCenteredCircle(可含描述/ID覆盖)不是同一定义；10125被硬编码视自中心但dist可保留100，回执朝向条件仍可能按另一规则运行。实际是否显著影响取决于castkind/目标情况，需集成测试。
- `P/SkillCaster.cs:53-68,325-349`：Pkg内`res/skillevent.txt`有`Skill{id}`脚本则**优先于**skillshow；未有脚本才CastData。此脚本读取绕开ResLoader.ReadRes的respatch/res24层，不同资源来源有不同优先级。
- `SkillCaster.cs:79-95,122-147`：找不到效果无显示；无target默认caster；支援型atk-only特效移到目标；飞行最少120ms；hit延迟叠加最大fly延迟。解析器还有ATK_PRE_TIME固定500（:32）；均为表现而非服务器伤害结算。
- `P/SkillEffectDB.cs:54-124`：默认skillshow（Use05Fx readonly未见赋值，默认false）；缺表一次空缓存；phy1为物理动画；phase30/50归hit，20归fly，其余atk；首个有phase同ID记录胜出。没有按职业重写数值伤害的客户端逻辑。
- `P/SkillSoundDB.cs:58-75,85-128`：默认skillsound；fire delay缺失fallback0；首重复ID胜出；缺表空缓存。
- `P/SkillElementDB.cs:15-23,34-70`：六元素固定映射冰/火/电/地/暗/风；缺条目Element=-1，不播聚气；classifier轮廓默认青色。是表现分类，不可等同服务器元素伤害。
- `P/CombatWorld.Traps.cs:16-24`：资源radius或100作缩放基准；server半径/基准夹到1..4，所以较小真实范围不会缩小视觉；所有陷阱Load(581)。`CombatWorld.Removal.cs:259-270`固定10214“中毒”、10215“封印”、10216“冰冻”，其他TFX032_30。
- `P/ReferenceWorldLocalSkillRelease.cs` 是带10307祝福专用绘制的兼容/诊断实现；全生产目录调用搜索未发现外部TryCreate入口，不将其当live战斗已执行的证据。

建议：保留素材并对齐其选择/尺寸到typed input contract；不要把视觉扩大解读成结算AoE。版本核对要同时覆盖script/show/sound/element，而不是只skills.json。

## 5. 新权威资料的可用时序

1. 本人旧协议快照0x8015先填 `CharData.skills`（`P/CharData.SkillState.cs:53-69,75-108`）；它与Endgame技能state独立。
2. 入世界后 `N/PrivateServerGateway.Endgame.cs:22-26` 发hello；成功 `A/EndgameClientState.cs:131-151` 立即设置GrowthV5。`P/EndgamePanel.cs:15` 的GrowthState随即非null，没有要求HasSkillRanks。
3. server `server/internal/game/scene/endgame.go:138-141` 在hello后立即推snapshot与skill.ranks；客户端同一hello处理又主动请求snapshot/rift.state/skill.ranks（`N/PrivateServerGateway.Endgame.cs:77-87`）。两端已尝试刷新，但不是原子发布，接收中有无rank空窗。
4. `A/EndgameSkills.cs:139-145` 接受非旧revision的skill.ranks，发布HasSkillRanks和149/该职业rows；`CanInvestSkill:131`要求同revision且非Pending。卡片/Hotbar/插件没有同等当前性门槛。
5. `A/EndgameClientState.cs:90-108`有hello、snapshot、quote、catalog超时机制，没有skill.ranks专用inflight/timeout/自动重拉；失败可能一直等用户点击“刷新技能”（`Win05.SkillDialog.cs:725-736`）。stale_revision有主动刷新。
6. server `skill_views_v5.go:345-373`在装备/状态刷新按digest推技能rank，涉及状态/技能/装备/revision；旧0x8015仍不等于有效级。
7. 断线重连 `ResetEndgameConnection → Endgame.Reset → ResetSkillRanks`清state（`N/...:17-20`,`A/...:47-56`）。静态资源cache仍无同级生命周期。

不能承诺“握手后卡片+施法立即全部权威”；也不能在unknown/stale时偷偷退到另一套旧资源MP/几何而声称一致。

## 6. 最小兼容修复方案（待全职业来源判定后实施，本次未改源码）

### 6.1 建立明确的新能力，避免破坏旧协议

- 增加协商能力，例如`endgame.skill-contract.v1`；只有客户端声明且server接受后才发新typed对象。可在现有skill.ranks行里增加可选`definition/current/next`对象，或独立只读消息，选最小实现。
- 现有客户端 `A/EndgameData.cs:15-18` 拒绝未知key；`A/EndgameSkills.cs:45`也列精确字段。所以**不能**无条件给旧客户端追加字段。新client解析器接纳并验证新字段；老client继续收到字节结构兼容的旧payload。
- `A/EndgameCommands.cs:37` 已支持hello capabilities数组，server按客户端cap回显（`scene/endgame.go:111-139`），可复用；不改原生0x8015/0x800e的布局。

### 6.2 从同一个实际结算解析器投影，不从文案解析

将最终技能def解析为只读结果并用于施法与展示：有效级 → Get → 训练协同 → 被动 → 装备；复用或抽取server现有逻辑，不另实现第二套公式。包含：

- 稳定identity/name/profession、definitionVersion/sourceVersion或hash；来源标为“该服务端规则”，通过版本取证才能标“原版”。
- `known, learned, usableActive`等准确语义；有效等级维持十进制字符串，不能截成byte/short；engine不可表示时明确错误，不能悄悄clamp。
- 实际`mpCost,prepareMs,cooldownMs`，typed numeric且明确范围/单位；不要解析“消耗: ...”等说明。
- `inputMode(single/ground/direction/instant), anchor(self/selected/ground), targetMask, rangePx`；以及`shape,radiusPx,lengthPx,widthPx,angleDegrees`。区分零距离、无范围、不适用、未知，不能全当<=0给默认值。
- 学习`hardCap,nextRequiredLevel,prerequisites,canInvest,blockedReasonCodes`；current/next descriptor来自同一解析结果，next不能偷用错误当前前置状态。
- 仍可保留server-authored原完整效果文本供排版；native modifier分组提供typed kind代替中文前缀。格式层保留原标题、当前/下一等级完整段、原颜色、技能点和快捷栏footer；协同仅追加原有块。

这里“实际”必须涵盖场景/装备/被动修正，不能只序列化基础catalog。visibility等瞬态施法条件最终server判断，快照不是永远有效的授权。

### 6.3 客户端只加一个薄的契约选择层

- 保留raw资源只负责图标/布局/素材与旧协议兼容，不再在读取raw行时批量改写技能字段。
- 新能力下，Win05卡片、SkillUsableForHotbar、HotSlotUsable、LocalNotEnoughMp、SkillTargeting/ValidateTarget、普通/插件request、回执朝向/范围圈、trap视觉基准都读取同一个resolved view。
- `LocalNotEnoughMp`只在typed成本已知且当前时作提示性提前拒绝。新能力已协商但成本未知/过期时不能拿旧表拒绝；可交服务器最终判定，并请求刷新。瞄准模式未知时不要编造范围/单群目标，暂缓需要选择器的入口并提示资料加载。
- server回执cooldown/chant继续为最终权威；前端不因卡片值自行预扣MP/改变结算。
- 非职业、宠物、生活/初行者与无新能力服务器保留旧协议路径；V5但无新cap时明确是旧兼容行为，不能宣称修复完整。等待原始证据后再决定是否移除旧剑客补丁。

### 6.4 原子状态与轻量刷新

- 以generation/character/session + definitionVersion + revision发布完整技能view，旧会话绝不沿用；若拆成两条消息，只有匹配pair完整才切换新数据源。
- 同一snapshot更新name/rank/MP/target/geometry/可上栏及学习资格，不允许每个消费者独自决定fallback。
- 增加skill资料request correlation、合并inflight、有界timeout/backoff，资料缺失/变旧时自动刷新；仍保留人工刷新。新rank不能回退旧revision。
- 上游有装备损坏、换装、跨图、属性/被动改变时更新或显式使view失效。若经济revision不能代表所有影响技能的状态，增加独立skillContractRevision/hash。

### 6.5 实施前/后必须验证

1. 按实际资源projection完整150候选（包括whitelist外项）与149支持契约逐ID记录：原始值、来源、server运行值、客户端resolved值、允许差异理由。
2. 五职业ID全覆盖；单体、自中心、地面圈、扇形、矩形、友方、陷阱、位移、被动；两个已知剑客案例不能代替全职业。
3. MP临界值 actual-1/actual/actual+1，硬级1/5/20、有效级超资源最大行、软级/授予级、减耗协同/装备、缺行；从UI热键、插件request、direct UseSkill、UseSkillAt逐入口验证无错误前置阻拦。
4. 当前/下一全文+中文名称/前置/数值精度；原native卡片版式golden、常规尺寸和超长滚动；不添加诊断字段扰乱卡片。
5. hello先到、rank延迟/丢失/失败、重复/乱序、旧revision、新角色重连、换装/损坏、空表/资源缺失时的数据一致性。
6. 老client-newserver、newclient-oldserver、仅growth.v5但无新cap，以及新新组合；未知typed字段按cap隔离。
7. 脚本/视觉半径对比结算命中，不把特效等价当技能行为等价。

## 7. 已执行验证与现有测试盲区

- 已执行 `python3 Tests/Endgame/skill_tooltip_source_contract.py`：**PASS 45**。这是源码guard，不是Unity渲染或真实施法测试。
- 已完成下表23个关键文件与f8d8885的字节/历史核对，并全Runtime搜索技能ID、关键字段读写与调用点。
- 当前恢复后的云环境没有mono/csc/dotnet；未重新运行managed harness或Unity Player。本报告不沿用上次环境消失前未知的测试状态。
- `Tests/Endgame/run-skill-tooltip-contract.sh`编译原growth文本/UITip/render，stub替代Win05/CharData等，不包含LocalNotEnoughMp、HotbarAvailability、真实资源加载、实际技能前置与完整客户端。
- `NativeSwordTargetingContract.cs`只对剑客30条+模拟其它ID测试，assert的“原始”target正是硬编码表本身；不能证明四个其它职业，也不能给规则来源背书。其两个实际资源偏差样例和MD5注释是线索，仍待本次projection核对。
- 本次未修改业务源码、未push、未运行资源提取/下载。

## 8. f8d8885 原始提交追溯清单（B）

下表“相同”仅证明机制初始就存在；绝不证明其官方正确。覆盖后续归责时必须区分来源污染与本轮改动。

| P/文件 | 与f8d8885比较 | 涉及提交（新到旧） |
|---|---|---|
| NetClient.SkillCommands.cs | 与初始提交逐字节相同 | f8d8885 |
| GameHud.HotbarAvailability.cs | 与初始提交逐字节相同 | f8d8885 |
| Win05.SkillLevelBlock.cs | 与初始提交逐字节相同 | f8d8885 |
| Win05.SkillTipText.cs | 与初始提交逐字节相同 | f8d8885 |
| Win05.SkillDamageBonus.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillTargeting.Classification.cs | 初始提交已存在，后续有修改 | 66c7712, f8d8885 |
| SkillTargeting.Entry.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillTargeting.Geometry.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillTargeting.Validation.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillTargeting.cs | 与初始提交逐字节相同 | f8d8885 |
| CombatWorld.Traps.cs | 与初始提交逐字节相同 | f8d8885 |
| CombatWorld.Removal.cs | 与初始提交逐字节相同 | f8d8885 |
| GameTables.cs | 与初始提交逐字节相同 | f8d8885 |
| ReferenceResourceLoader.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillEffectDB.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillSoundDB.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillElementDB.cs | 与初始提交逐字节相同 | f8d8885 |
| SkillCaster.cs | 与初始提交逐字节相同 | f8d8885 |
| DialogUI.Ascend.cs | 与初始提交逐字节相同 | f8d8885 |
| ReferenceWorldReceiveOwner.SkillCastReceipt.cs | 与初始提交逐字节相同 | f8d8885 |
| ReferenceWorldLocalSkillCast.cs | 初始提交已存在，后续有修改 | 66c7712, f8d8885 |
| Win05.SkillDialog.cs | 初始提交已存在，后续有修改 | 66c7712, 348f578, f8d8885 |
| Win05.SkillData.cs | 初始提交已存在，后续有修改 | 66c7712, f8d8885 |

`NativeSwordSkillTargeting.cs`新增于66c7712；`Win05.GrowthSkills.cs`/`A/EndgameSkills.cs`成长相关新增于348f578，卡片后经1ad281c/66c7712调整。所有基础描述正则、目标fallback、旧MP拦截、旧学习、声音/效果分支均不能只靠V5 diff审计。


## 实施状态更新（2026-10-12）

本报告前述path/line和源码来源证据以基线66c7712为准，后续修改不回写历史证据。现已批准并实现`endgame.skill-behavior.v1`统一客户端provider；具体字段、消费者、兼容策略与验收边界见[client-restoration-plan.md](client-restoration-plan.md)。10212保留现有名称、施法与投资路径，其服务端普通伤害占位仍是未恢复项，客户端不添加待核禁用文案。其他148项支持逐行typed身份与行为。资源原件、完整dump和美术均未写入repo。
