# 传奇威能修复与重新复审

基线：`489706a`。完整补丁包含前轮恢复的全部修复以及本轮重新复审后的新增修复，不仅是本轮差异。

本表保留对上轮75条非 implemented记录的闭环。此次另对全部330条当前玩家文案、handler与实际消费者重新核对，见[330条复审矩阵](rereview-matrix-330.json)。逐项源码复核不等于330条所有等级、地图和装备组合的端到端穷举；真实动态覆盖、受控消费者边界及跳过原因见[验证记录](validation.md)与[结构化摘要](verification-summary.json)。

## 证据范围

- 正式链路包括 `onUseSkill → stepCasts`、`onAttack → stepCombat`、陷阱触发、怪物AI、DOT与反射；保留技能原有费用、吟唱、冷却、弹道与概率。
- 原生场景fixture来自提交快照及迁移62/66/68–74与V5元数据；它不是PostgreSQL loader。actor属性及普通装备受控以便精确断言，装备部位、职业、最低等级、耐久与激活规则仍检查。
- 不构造不可达同槽或低等级基底组合：1822为真实26级，CK-A13不放该武器；SS-U02和SS-U08不伪造同装。当前无PvP路径、怪物目录未包含的组合只作为消费者边界，不报告为已发生线上缺陷。
- 数据库、客户端资源、抓包/地图导出及可选压力测试未全部具备；跳过不等于通过，未声称生产加载、客户端联调或线上验收。远端feature推送仍待确切目标确认，未成功推送；未合并main、部署或上传资源。

## 330条独立行为用例验收

当前catalog330个真实ID全部有单威能实际消费者测试，冻结生产代码的默认vet全server运行与随后仅新增两条概率边界的焦点运行均pass、无solo skip，逐条映射和组合分离门禁通过：330/330，100%。这不是Go行/语句/分支覆盖率，后者未测量。组合条款独立列出并验证，不计分母/分子；并未穷举所有装备排列。

证据见[覆盖门禁结果](independent-coverage-result.json)、六组独立manifest、[330条当前效果索引](rereview-matrix-330.json)和[独立证据复核](independent-coverage-review.md)。原修复阶段全量结果保留在verification-repair-stage.json，最新最终结果在verification-summary.json。后续仅文档提交不冒充另一次Go测试。

## 原75项闭环状态

- 修复并有回归：71条。
- 原合同核实：1条。
- 按本轮明确范围收尾：1条。
- 纠正上轮可达性假阳性：1条。
- 原技能/组合条件核实：1条。

## 本轮新增复审结果

- 原单体变自中心范围技能不能借同时携带坐标绕过原选中目标射程；真正地面中心与原生范围仍按各自准入规则。
- ZS-A03不增加未声明的连环枪法学习门槛，且与ZS-A21共用已替代普攻的原生基础倍率，不借主动终倍率或附加基础伤害。
- CK-A18无预淬毒状态时也使用有效淬毒等级；已接受的陷阱不能借之后增加的技能等级或装备升级提高回退来源。
- TY-D11在所有适用纯减伤之后扣法力，再进护盾；直接、持续与原生反射的蓝量充足/不足/零与盾组合均覆盖，不重放反射或回复。
- YS-D09按真实普攻来源识别，不把技能ID为0的裂隙地面危害当普攻。
- 火球副爆炸保留原施法的主目标身份，主/旁目标加成不串位；真正授予新技能的派生仍用其合法新目标。
- 异技能派生、赠送buff、持续区域与剥法借用基准的源装备、技能等级、强化武器与变体来自接受时的只读准备快照，不借命中前换装抬高已接受的施法；SS-A28、YS-A07、YS-U06、SS-A25等非ChildSkill消费者也按同一接受快照。
- 通用新增原生毒/燃烧三件组合、来源卸装后周期快照、施法者与受盾者加成、普攻距离×扇形半径消费者回归。


## 独立用例补齐时发现并修复的边界

- TY-D11/D19的纯承伤修饰不再被威能来源DOT生成守卫跳过；显式可暴击周期载荷由TY-D10按同加法减伤桶处理。生产性反击/派生仍隔离，settled复制不重算。
- 真正成功且有效的硬控立即取消未释放吟唱，受压制控制因保护解除/到期重新有效时也取消，短控制结束不能复活旧施法；免疫/抵抗失败不取消，已释放弹道保留。仅玩家有cast windup，怪物不额外扫描该列表。
- 扇形角度点积正确处理超过180度及360度、旋转后的精确边界；移除错误前半平面限制，仅加入1e-9世界坐标单位比较容差。常见原生120→168不受缺陷影响，宽角用例明确是消费者合同边界。
- ZS-A08百分比Def削弱延续原生属性下限：低阶仍增强，已有低于-100的原始特例保留；Def≤0命中早已同为100%，不宣称此次修复改变额外伤害。
- 侧位陷阱有accepted来源时直接复用，避免先复制整套来源又丢弃；hard/raw准备快照沿根/child只读复用，不按每个受击目标克隆。见source-preparation-allocation-review.md。
- SS-U01措辞明确保留原技能施法与落点规则，没有为了测试给原生强制瞬移新增走路/定身限制。

## 原75项逐ID结果

### ZS-A02 横扫枪花：修复并有回归

语义主目标与表现中心分离；只补入原施法距离内合法敌人，保留原控制和一次费用。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendarySharedPrimaryControlAttachmentBoundary`。

### ZS-A07 开隙：修复并有回归

真实10009负防御状态先于命中计算；保留学习、抵抗、免疫、来源与时长规则。
- 回归：`TestLegendaryWarriorAuditArmorBeforeNativeHit`；`TestLegendaryWarriorAuditArmorImmunityAndUnlearned`。

### ZS-A08 裂甲传染：修复并有回归

合法主目标边界已修；未学donor使用原生一级范围/射程，已学使用有效等级并继承形态，且不解锁主动技能。
- 边界：不推广到已有明确学习前提的A07/A24/A25。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendaryWarriorAuditBorrowedGeometryWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedReachWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedRankAndNativeRows`。

- 后续独立用例闭环：负百分比Def延续原生floor=min(-100,原始值)，保留原始<-100特例，非百分比/其他属性不改；低阶仍增强。Def≤0命中原已相同，此修复是状态/面板原有边界，不是额外物理增伤。

### ZS-A10 噤声余响：修复并有回归

理论削蓝换算移至所有修饰汇总后，TY-A19/U02/D04均纳入；敌方空蓝不缩水。
- 回归：`TestLegendaryWarriorAuditManaConversionCombinations`。

### ZS-A16 裂阵：修复并有回归

语义主目标与表现中心分离；只补入原施法距离内合法敌人，保留原控制和一次费用。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendarySharedPrimaryControlAttachmentBoundary`。

### ZS-A17 铸魂：修复并有回归

同次施法同时免百分比耐久和普通攻击磨损，法力及套装生命代价保持。
- 回归：`TestLegendaryWarriorAuditNoDurabilityWholeCast`。

### ZS-A20 猎首：修复并有回归

语义主目标与表现中心分离；只补入原施法距离内合法敌人，保留原控制和一次费用。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendarySharedPrimaryControlAttachmentBoundary`。

### ZS-A23 独战：修复并有回归

自身圆形保留被选敌人；无选中时按实际命中敌人生命比例选追加目标。
- 回归：`TestLegendarySharedSelfCirclePrimaryAndAdditiveBuilds`；`TestLegendaryWarriorAuditNativeA23PrimaryAndSet`。

### ZS-A24 百兵归一：修复并有回归

语义主目标与表现中心分离；只补入原施法距离内合法敌人，保留原控制和一次费用。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendarySharedPrimaryControlAttachmentBoundary`。

### ZS-A25 封疆：修复并有回归

真实10009负防御状态先于命中计算；保留学习、抵抗、免疫、来源与时长规则。
- 回归：`TestLegendaryWarriorAuditArmorBeforeNativeHit`；`TestLegendaryWarriorAuditArmorImmunityAndUnlearned`。

### ZS-A27 崩枪：修复并有回归

合法主目标边界已修；未学donor使用原生一级范围/射程，已学使用有效等级并继承形态，且不解锁主动技能。
- 边界：不推广到已有明确学习前提的A07/A24/A25。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendaryWarriorAuditBorrowedGeometryWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedReachWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedRankAndNativeRows`。

### ZS-A30 合锋：修复并有回归

合法主目标边界已修；未学donor使用原生一级范围/射程，已学使用有效等级并继承形态，且不解锁主动技能。
- 边界：不推广到已有明确学习前提的A07/A24/A25。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`；`TestLegendaryWarriorAuditBorrowedGeometryWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedReachWithoutLearningGate`；`TestLegendaryWarriorAuditBorrowedRankAndNativeRows`。

### ZS-D03 卸力：修复并有回归

从不确定转为真实反例后修复：10009被抵抗而10012易伤成功不再误减伤；只认可本人真实负防御。
- 回归：`TestLegendaryWarriorAuditD03NativeVulnerabilityWithoutArmor`；`TestLegendaryWarriorAuditD03ActualArmorSource`。

### JK-A14 封法断垒：原合同核实

原始规格明确额外削盾按主目标结算；验证旁侧仅正常吸收，主目标120%与400%攻击上限不变。
- 边界：未扩展旁侧额外削盾；不是本轮新增此玩法。
- 回归：`TestLegendarySwordsmanAuditA14PrimaryShieldContract`。

### JK-A23 陨星余烬：修复并有回归

真实即死成功分支补一次剑爆；不重复死亡回调、不再产生即死或剑爆。
- 回归：`TestLegendarySwordsmanAuditNativeInstantDeathBurst`。

### JK-U04 挑衅集结：按本轮明确范围收尾

按本轮收尾方向，只保留真实挑衅半径+35%（100→135），移除对原生0施法距离无效的乘数；没有新增远程中心挑衅。
- 回归：`TestLegendarySwordsmanAuditNativeTauntReachAndSet`。

### JK-U08 旋风定足：修复并有回归

装备期间免移动减速与定身，并清理已有相应组件；混合状态中的眩晕/冻结/石化、攻速降低及其他伤害/护盾/易伤载荷保留；卸装后新减速/定身恢复正常。
- 边界：原生怪物减速经正式施法链验证；混合状态与根免疫边界含受控构造，不宣称全部当前怪物目录都具备这些组合。
- 回归：`TestLegendarySwordsmanMovementImmunityMonsterSlowCast`；`TestLegendarySwordsmanMovementImmunityEquipmentLifecycle`；`TestLegendarySwordsmanMovementImmunityMixedStatusPreservation`；`TestLegendarySwordsmanMovementImmunityPreservesNativeAttackSlow`；`TestMovementImmunityKeepsActiveBalancesAndCoexistingLayers`。

### CK-A07 孤星聚刃：修复并有回归

原生扇形、A06圆形和1926武器圆形均保留合法被选敌人身份；原生几何内目标不误按Dist拒绝。
- 回归：`TestLegendarySharedAssassinPrimaryBuilds`；`TestLegendarySharedNativeWeaponCircleKeepsSelection`；`TestLegendarySharedNativeAreaPrimaryUsesAdmittedGeometry`；`TestLegendaryRepairCirclePrimaryTripleComposition`。

### CK-A13 串联引信：修复并有回归

1822真实即时爆弹改用陷阱敌方目标选择，三枚各承继已受理技能和来源；一次费用，无残留或递归布雷。
- 边界：同次施法内按顺序触发，不宣称三份目标状态同时快照。1822为26级，A13不能放该武器。
- 回归：`TestAssassinRegressionInstantBombNativeCombinations`；`TestUniversalRegressionTrapConditionalDoubleAndTriple`；`TestAssassinRegressionAcceptedBombDoesNotBorrowNewTriplePower`。

### CK-A27 暗器延展：修复并有回归

A05最终溅射半径先形成再消费加成；与其他范围增幅按同一桶相加。
- 回归：`TestAssassinRegressionThrowFinalSplashNativeCast`。

### CK-A28 收网环击：修复并有回归

取消无依据的已学爆发冲击前置：未学也获得45%增伤与有界牵引；已学时保留原先击退。A30仍保留它原有的学习前置，只在真正生效时覆盖A28的牵引。
- 回归：`TestAssassinFinalReviewPullLearningAndGeometryCombinations`；`TestAssassinRegressionPullNativePassiveAndA30`。

### CK-D01 出影护幕：修复并有回归

隐身原状态生效期间，纯减伤覆盖直接、派生、持续及反射伤害；按原破隐时序立即失效，来源未知也可按自身状态判定。
- 边界：跨伤害类别矩阵含受控来源/状态的消费者测试，不能据此宣称玩家PvP或原生怪物已具备所有攻击组合。
- 回归：`TestLegendaryIncomingAssassinAllDamageConsumers`；`TestLegendaryIncomingAssassinCombinationAndShieldOnce`；`TestLegendaryIncomingAssassinNativeStealthTiming`；`TestLegendaryIncomingAssassinUnknownSourceDoesNotBorrowCondition`。

### CK-D02 烟幕掩体：修复并有回归

仍以自己烟雾弹的有效负命中状态为条件；覆盖来自该敌人的直接、派生、持续及反射伤害，不借用他人或未知来源状态。
- 边界：跨伤害类别矩阵含受控来源/状态的消费者测试，不能据此宣称玩家PvP或原生怪物已具备所有攻击组合。
- 回归：`TestLegendaryIncomingAssassinAllDamageConsumers`；`TestLegendaryIncomingAssassinCombinationAndShieldOnce`；`TestLegendaryIncomingAssassinDebuffOwnerAndRemoval`；`TestLegendaryIncomingAssassinActualPayloadMirror`。

### CK-D06 布雷退守：修复并有回归

纯远程减伤覆盖已知来源射程大于100的直接、派生及持续伤害；保存来源定义，不用当前距离、技能ID=0或无关根施法猜测远程。
- 边界：跨伤害类别矩阵含受控来源/状态的消费者测试，不能据此宣称玩家PvP或原生怪物已具备所有攻击组合。
- 边界：旧存档缺失的新来源字段按未知处理；未知来源不猜测远程或陷阱。已验证JSON保存/恢复，未运行数据库往返。
- 回归：`TestLegendaryIncomingAssassinAllDamageConsumers`；`TestLegendaryIncomingAssassinCombinationAndShieldOnce`；`TestLegendaryIncomingAssassinRangedProvenance`；`TestLegendaryIncomingAssassinDOTProvenancePersistence`；`TestLegendaryIncomingAssassinActualRiftHazardIsNotRangedBasicOrTrap`。

### CK-D07 截腕卸力：修复并有回归

自己击晕的真实眩晕状态存在时，对该来源的直接、派生、持续及反射伤害均减伤；移除、免控、别人施加及未知来源均不假造条件。
- 边界：跨伤害类别矩阵含受控来源/状态的消费者测试，不能据此宣称玩家PvP或原生怪物已具备所有攻击组合。
- 回归：`TestLegendaryIncomingAssassinAllDamageConsumers`；`TestLegendaryIncomingAssassinCombinationAndShieldOnce`；`TestLegendaryIncomingAssassinDebuffOwnerAndRemoval`；`TestLegendaryIncomingAssassinActualPayloadMirror`。

### CK-D10 避阱余裕：修复并有回归

已学陷阱回避时，固定减伤覆盖有明确陷阱来源的直接、派生与持续伤害；派生只保留承伤分类，绝不重放原生概率半伤或进攻陷阱倍率。
- 边界：跨伤害类别矩阵含受控来源/状态的消费者测试，不能据此宣称玩家PvP或原生怪物已具备所有攻击组合。
- 边界：旧存档缺失的新来源字段按未知处理；未知来源不猜测远程或陷阱。已验证JSON保存/恢复，未运行数据库往返。
- 回归：`TestLegendaryIncomingAssassinAllDamageConsumers`；`TestLegendaryIncomingAssassinCombinationAndShieldOnce`；`TestLegendaryIncomingAssassinRealMonsterTrapDOT`；`TestLegendaryIncomingAssassinDOTProvenancePersistence`；`TestLegendaryIncomingAssassinPersistentRefreshPreservesCategory`；`TestLegendaryIncomingAssassinTrapLineageDoesNotReplayNativeHalf`。

### CK-U03 拆阱回馈：修复并有回归

拆阱复用专用准入并过滤本人/同队陷阱；敌方可见陷阱保留成功率和免费。
- 回归：`TestAssassinRegressionDisarmPreservesVisibleFriendlyTraps`。

### CK-U04 探路静行：修复并有回归

侦测最终半径实际消费增幅，常驻移动增幅保留。
- 回归：`TestAssassinRegressionDetectNativeFinalRadius`。

### SS-A04 静候爆芯：修复并有回归

子爆炸/复诵显式承接原施法中心与主目标，中心/环带不再误用世界原点；瞬发原生属性刷新保留。
- 边界：原生瞬发刷新MAtk后245+171+196=612，不按旧手填100属性强改期望。
- 回归：`TestLegendaryRepairFireballChildCenterTriple`。

### SS-A05 聚焦炎核：修复并有回归

分档仅纳入可实际承受该火球的合法目标，技能免疫旁观者不降主目标档。
- 回归：`TestCasterRepairFireballCountsOnlyLegalHits`。

### SS-A06 焚圈：修复并有回归

子爆炸/复诵显式承接原施法中心与主目标，中心/环带不再误用世界原点；瞬发原生属性刷新保留。
- 边界：原生瞬发刷新MAtk后245+171+196=612，不按旧手填100属性强改期望。
- 回归：`TestLegendaryRepairFireballChildCenterTriple`。

### SS-A08 远寒凝尖：修复并有回归

远距增幅门槛按本次最终射程计算，包含U10常驻射程。
- 回归：`TestCasterRepairRangeThresholdAndWideAreaComposition`。

### SS-A09 霜纹刻痕：修复并有回归

保证减速/燃烧绕过概率抵抗，但保留硬免疫；泛化识别负移速，原生时长与来源保留。
- 边界：“必定”不绕过硬免疫/禁止叠加；A17旧版全等级总收益等价未验证。
- 回归：`TestCasterRepairGuaranteedStatusRetainsImmunity`；`TestCasterRepairNativeControlSnapshots`。

### SS-A10 寒针破抗：修复并有回归

冰箭标识派生结算同样消费50%冰系百分比减伤穿透，其他防御不穿透。
- 边界：数值沿现有规格；未宣称找到逐条更早用户设计批准。
- 回归：`TestCasterRepairIceSettlementInheritsOnlyPurePenetration`。

### SS-A12 遥爆冰棺：修复并有回归

保留原冻结实例、时长和负移速；A15按受理前控制状态快照，不读命中时临时状态。
- 回归：`TestCasterRepairNativeControlSnapshots`；`TestLegendaryRepairFreezeChildBurnSettlementPreserved`。

### SS-A14 凝霜牵引：修复并有回归

先过滤不可位移者，再取最远5个，避免免疫目标占名额；障碍与原效果保留。
- 边界：当前合法敌人消费者仍为怪物，未引入PvP位移。
- 回归：`TestCasterRepairPullSkipsImmuneBeforeLimit`。

### SS-A15 破冻重锤：修复并有回归

保留原冻结实例、时长和负移速；A15按受理前控制状态快照，不读命中时临时状态。
- 回归：`TestCasterRepairNativeControlSnapshots`；`TestLegendaryRepairFreezeChildBurnSettlementPreserved`。

### SS-A17 风焰续薪：修复并有回归

保证减速/燃烧绕过概率抵抗，但保留硬免疫；泛化识别负移速，原生时长与来源保留。
- 边界：“必定”不绕过硬免疫/禁止叠加；A17旧版全等级总收益等价未验证。
- 回归：`TestCasterRepairGuaranteedStatusRetainsImmunity`；`TestCasterRepairNativeControlSnapshots`。

### SS-A18 风焰宽带：修复并有回归

保留圆形，将原80%作用宽度按直径对应到实际半径150→270；再与TY-A04统一范围桶组合。
- 边界：几何解释经本轮确认，未把原圆形换成新矩形。
- 回归：`TestCasterRepairFlameWidthAndNativeBurnCombination`；`TestCasterRepairRangeThresholdAndWideAreaComposition`。

### SS-A19 焰终回响：修复并有回归

仅计算本次新施加的燃烧初始总伤害65%，不重算共存旧燃烧，原燃烧继续。
- 回归：`TestCasterRepairNewBurnOnlyAndElementalSharing`。

### SS-A22 暴雪凝域：修复并有回归

保留原冻结实例、时长和负移速；A15按受理前控制状态快照，不读命中时临时状态。
- 回归：`TestCasterRepairNativeControlSnapshots`；`TestLegendaryRepairFreezeChildBurnSettlementPreserved`。

### SS-A25 法式终章：修复并有回归

元素被动共享延伸到原生燃烧每跳和剩余结算；已有修饰只消费一次。
- 回归：`TestCasterRepairNewBurnOnlyAndElementalSharing`。

### SS-A26 镜中讨还：修复并有回归

被抵抗的派生/追加魔法也用A26系数和18%生命上限，D03倍率在生成保护内保留；反射不再产反射。
- 边界：子伤害攻击玩家为伤害消费者边界测试，不代表开放玩家PvP。
- 回归：`TestLegendaryIncomingActualPayloadAndChildReflection`；`TestLegendaryIncomingMirrorProcCapAndNoReflectionLoop`；`TestLegendaryIncomingMirrorAmplifierInsideGenerationGuard`。

### SS-A30 瞬发复诵：修复并有回归

子爆炸/复诵显式承接原施法中心与主目标，中心/环带不再误用世界原点；瞬发原生属性刷新保留。
- 边界：原生瞬发刷新MAtk后245+171+196=612，不按旧手填100属性强改期望。
- 回归：`TestLegendaryRepairFireballChildCenterTriple`。

### SS-D01 烫手法衣：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-D02 冰甲承峰：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-D03 镜面偏转：修复并有回归

跨入口魔法减伤补齐；真实规范化火/冰盾减伤字段放大，不仅放大已移除的旧Affix字段。
- 回归：`TestLegendaryIncomingNativeShieldMagnitudeConsumer`；`TestLegendaryIncomingActualNativeFireShieldCombination`；`TestLegendaryIncomingUntypedPoisonIsNotMirrorMagic`。

### SS-D05 凝神护界：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-D06 寒封隔绝：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-D09 焚息护身：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-D10 释能护息：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### SS-U08 净镜通行：纠正上轮可达性假阳性

直接施放魔镜的正式入口本已清旧减速并免疫新减速。原审计偷镜组合不可装备：U02与U08同武器槽，另一转移武器1324原生38级低于U08最低51级。撤去为不可达组合新增的钩子。
- 边界：未将直接状态注入当可达组合，未更改武器等级或威能部位来制造通过。
- 回归：`TestCasterRepairNativeMirrorAcquisitionCleansesExistingSlow`；`TestLegendaryCasterSorcererRemainingMirrorSlow`。

### YS-A01 散光棱镜：修复并有回归

主目标周围按距主目标最近选3旁侧；副段禁重复威能，治疗派生也遵守总4目标。
- 回归：`TestLegendaryRepairLightSplitPrimarySecondaryAndNearest`；`TestLegendaryRepairHealChildImplicitSelfAndSetIsolation`。

### YS-A05 借位圣光：修复并有回归

合法友军中心在正式受理路径可用；死亡/陌生/loading/超600范围被拒，友军中心不变伤害目标。
- 回归：`TestLegendarySharedFriendlyCenterAdmissionAndVictims`；`TestLegendarySharedFriendlyCenterDoesNotRestoreFriendlyVictim`；`TestLegendarySharedFriendlyLoadingCenterRejected`。

### YS-A08 圣光贯阵：修复并有回归

矩形/自身圆形变形保留语义主目标和合法形外主目标，不把施法者替换为伤害主目标。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`。

### YS-A12 断咏裁决：修复并有回归

打断真实怪物AI吟唱与timer序号；控制免疫只阻止打断，不阻止此伤害。
- 回归：`TestCasterRepairHealerInterruptsMonsterAI`。

### YS-A18 枷锁重压：原技能/组合条件核实

原生枷锁无直接伤害；单件攻速减益30→45，和A19赋伤组合后伤害200→320。
- 边界：单件不凭空新增伤害，伤害增幅需已有伤害来源；不是新增一段攻击。
- 回归：`TestCasterRepairHealerShackleAndHolyGeometry`。

### YS-A20 针光护体：修复并有回归

矩形/自身圆形变形保留语义主目标和合法形外主目标，不把施法者替换为伤害主目标。
- 回归：`TestLegendarySharedPreservePrimaryRangeAdmission`。

### YS-A23 仁心裁光：修复并有回归

未学光弹也用原生一级形态承载授予攻击；原始应有治疗量作基础，不看有效治疗；组合A01限额。
- 回归：`TestLegendaryRepairHealChildImplicitSelfAndSetIsolation`；`TestCasterRepairGrantedLightWithoutLearningBolt`。

### YS-A26 防御共鸣：修复并有回归

实际防御字段70%强度已消费，默认target0及附带被忽略At均保留自身主目标并扩展250范围内队友。
- 回归：`TestLegendaryIncomingNativeBuffMagnitudeCombinations`；`TestLegendarySharedImplicitSelfRetainsSupportCenter`。

### YS-A29 释能广照：修复并有回归

原单体圣光+100形成圆形；最终形状确定后再消费TY-A04。原矩形长/宽各+100，再受同类比例加成，A08+TY-A06为700×416。
- 回归：`TestCasterRepairHealerShackleAndHolyGeometry`；`TestCasterRepairChargedHolyFinalShapeComposition`；`TestCasterRepairChargedHolyRectangleComposition`。

### YS-D03 群盾覆阵：修复并有回归

隐式自身成为治疗/辅助语义主目标，低生命比例队友选取与完整光盾保留。
- 回归：`TestLegendarySharedImplicitSelfRetainsSupportCenter`。

### YS-D05 回春护咏：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

- 后续独立用例闭环：真正有效硬控立即取消未释放吟唱，受压制控制重新生效时也取消，短控制结束不复活旧cast；免疫失败、仍压制、原生瞬发无windup及已释放弹道不误取消。

### YS-D06 群疗定心：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

- 后续独立用例闭环：有效硬控及保护解除/到期后重新有效的旧控制取消未释放吟唱，普通受伤保护不变；免疫失败与已释放弹道保留，短控制消退不复活旧cast。

### YS-D08 复苏守望：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### YS-D10 枷锁息战：修复并有回归

明确的纯承伤修饰覆盖正常、追加、子伤害、DOT与反射，并在原减伤后/盾前只应用一次；不放开递归生成。
- 边界：受理后复制已结算伤害不重复承伤；未知DOT类型不猜魔法；PVP派生为消费者边界而非可达玩法。
- 回归：`TestLegendaryIncomingAllDamageConsumerMatrix`；`TestLegendaryIncomingCombinationShieldAndManaOnce`；`TestLegendaryRepairAreaNativeShieldsAndMonsterAI`；`TestLegendaryIncomingActualHealerWindupCombination`。

### YS-U02 专注圣契：修复并有回归

原生规范化防御/反射等正面强度字段可消费50%乘数，时限与一次性关系保留。
- 边界：YS-D04固定+35百分点与U02倍率分开，避免错误放大固定百分点。
- 回归：`TestLegendaryIncomingNativeBuffMagnitudeCombinations`。

### YS-U04 治愈术群体化：修复并有回归

默认自身扩散成立；新增副治疗禁止2件套/伤害/护盾转换，根治疗的4件套正常一次转换保留。
- 边界：群疗35%返法仍要求至少一名真正恢复生命；全满血无返还。
- 回归：`TestLegendaryRepairHealChildImplicitSelfAndSetIsolation`；`TestLegendaryRepairNativeBaseAndSetControls`。

### TY-A04 广域：修复并有回归

圆形原生陷阱半径纳入最终范围桶，组合A21和职业改形仍一次生效。
- 回归：`TestUniversalRegressionPhysicalTrapEquipmentBaseAndGeometry`；`TestLegendarySharedFinalGeometryBuckets`；`TestCasterRepairChargedHolyFinalShapeComposition`。

- 后续独立用例闭环：扇形消费者不再额外截断前半平面，60/120/168/180/240/360四向及精确角度/半径边界验证；常见原生小角未受旧问题影响，宽角是明确支持的消费者边界。

### TY-A25 铁铸锋：修复并有回归

物理陷阱HurtNone归入真实物理伤害；装备攻击贡献先入攻击基础再乘原技能系数，TY-A26同语义并测。
- 回归：`TestUniversalRegressionPhysicalTrapEquipmentBaseAndGeometry`；`TestUniversalRegressionMagicAttackBaseNativeCombination`；`TestUniversalRegressionTrapAttackBaseCannotUpgradeAfterPlacement`。

### TY-A30 制敌：修复并有回归

普攻无额外变形也执行目标修饰；陷阱含1822使用同目标阶段；三威能/套装/来源快照均验证。
- 回归：`TestUniversalRegressionTargetBonusRealBasicAndTrap`；`TestUniversalRegressionTrapConditionalDoubleAndTriple`；`TestUniversalRegressionOwnPoisonBasicWithAssassinSet`；`TestLegendaryRepairBasicFanTargetModifiers`。

### TY-A31 毒蚀破绽：修复并有回归

普攻无额外变形也执行目标修饰；陷阱含1822使用同目标阶段；三威能/套装/来源快照均验证。
- 回归：`TestUniversalRegressionTargetBonusRealBasicAndTrap`；`TestUniversalRegressionTrapConditionalDoubleAndTriple`；`TestUniversalRegressionOwnPoisonBasicWithAssassinSet`；`TestLegendaryRepairBasicFanTargetModifiers`。

### TY-A32 灼热破绽：修复并有回归

自身燃烧目标上的普通攻击执行目标修饰，不再因普攻未变形提前返回。
- 边界：未把未证明可达的跨职业本人燃烧+刺客陷阱算验收。
- 回归：`TestUniversalRegressionTargetBonusRealBasicAndTrap`。

### TY-D06 卸招：修复并有回归

普通攻击按真实来源识别，裂隙地面危害的SkillID0不再误判普攻。
- 回归：`TestLegendaryIncomingRiftHazardNotBasic`。

### TY-U04 济耗：修复并有回归

原生持续治疗状态也识别为治疗段，回春术实际MP208→145，和减耗同桶。
- 回归：`TestUniversalRegressionHealingStatusManaNativeCast`。
