# Verified five-profession asset change ledger

Read-only comparison of local historical backup0a25fd… and current resourcecbe007…. These are proven asset changes, not blanket approval to roll back every changed field. Raw resources stay cloud-local.

68 modified skills and three removed skills. Exact per-rank values and SQL corroboration: `verified-asset-differences.json`.

## 10006 震地之枪 → 震地之枪 (战士)

Decision: radius_text_table_conflict_requires_field_level_decision

- General description:
  - Old: 对120度角，半径150扇形范围内的敌人造成130%的伤害，每升一级伤害提升5%。
  - Current: 对120度角，半径200扇形范围内的敌人造成130%的伤害，每升一级伤害提升5%。
- Rank1 description:
  - Old: 对120度角，半径150扇形范围内的敌人造成130%的伤害。
  - Current: 对120度角，半径200扇形范围内的敌人造成130%的伤害。
- Rank5 description:
  - Old: 对120度角，半径150扇形范围内的敌人造成150%的伤害。
  - Current: 对120度角，半径200扇形范围内的敌人造成150%的伤害。

## 10008 意志 → 意志 (战士)

Decision: custom_effect_or_scope_added

- General description:
  - Old: 昏迷抗性+10%，每次升级抗性提升10%。
  - Current: 昏迷抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank1 description:
  - Old: 昏迷抗性+10%。
  - Current: 昏迷抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank5 description:
  - Old: 昏迷抗性+50%。
  - Current: 昏迷抗性+50%；并使自身对所有不良状态的抵抗能力提高13%。

## 10012 破甲 → 破甲 (战士)

Decision: native_effect_numeric_or_tradeoff_changed

- Rank1 description:
  - Old: 破防之枪可以使对方15秒内多受20%的物理伤害和魔法伤害。
  - Current: 破防之枪可以使对方15秒内多受25%的物理伤害和魔法伤害。
- Rank5 description:
  - Old: 破防之枪可以使对方15秒内多受32%的物理伤害和魔法伤害。
  - Current: 破防之枪可以使对方15秒内多受40%的物理伤害和魔法伤害。

## 10014 破魔 → 枪势 (战士)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 破法之枪伤害法力的效果增加10%，每次升级再次增加10%。
  - Current: 连续攻击同一目标时，每次命中使其后续受到的枪类技能伤害提高4%，最多叠加3层（持续8秒，更换目标或停手后消失）。
- name: 破魔 → 枪势
- Rank1 description:
  - Old: 破法之枪伤害法力的效果增加10%。
  - Current: 连续攻击同一目标时，每次命中使其后续受到的枪类技能伤害提高4%，最多叠加3层（持续8秒，更换目标或停手后消失）。
- Rank5 description:
  - Old: 破法之枪伤害法力的效果增加50%。
  - Current: 连续攻击同一目标时，每次命中使其后续受到的枪类技能伤害提高10%，最多叠加3层（持续8秒，更换目标或停手后消失）。

## 10015 心眼之枪 → 心眼之枪 (战士)

Decision: native_effect_numeric_or_tradeoff_changed

- Rank1 description:
  - Old: 100%命中敌人，造成165%的伤害。
  - Current: 100%命中敌人，造成180%的伤害。
- Rank5 description:
  - Old: 100%命中敌人，造成181%的伤害。
  - Current: 100%命中敌人，造成260%的伤害。

## 10016 石化之枪 → 石化之枪 (战士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.lreq: rank1 60→51; rank2 62→53; rank3 64→55; rank4 66→57; rank5 68→59

## 10022 野战八荒 → 野战八荒 (战士)

Decision: current_geometry_repairs_old_conflict_do_not_blindly_revert

- radius: 250 → 150

## 10023 战神之枪 → 战神之枪 (战士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 2000→800; rank2 2000→800; rank3 2000→800; rank4 2000→800; rank5 2000→800

## 10024 维护 → 破军 (战士)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 降低裂空枪法使用时消耗的武器耐久度，升级使此效果增强。
  - Current: 裂空之枪、流虹之枪、野战八荒每多命中一个敌人，该次伤害提高5%，最多提高50%。
- name: 维护 → 破军
- Rank1 description:
  - Old: 降低裂空枪法使用时消耗的武器耐久度，升级使此效果增强。
  - Current: 流虹之枪、野战八荒每多命中一个敌人，该次伤害提高5%，最多提高50%。
- Rank5 description:
  - Old: 降低裂空枪法使用时消耗的武器耐久度，升级使此效果增强。
  - Current: 流虹之枪、野战八荒每多命中一个敌人，该次伤害提高15%，最多提高50%。

## 10025 强健 → 强健 (战士)

Decision: custom_effect_or_scope_added

- General description:
  - Old: 减速抗性+10%，每次升级抗性提升10%。
  - Current: 减速抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank1 description:
  - Old: 减速抗性+10%。
  - Current: 减速抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank5 description:
  - Old: 减速抗性+50%。
  - Current: 减速抗性+50%；并使自身对所有不良状态的抵抗能力提高13%。

## 10026 抗魔 → 抗魔 (战士)

Decision: custom_effect_or_scope_added

- General description:
  - Old: 冰冻，石化，定身，攻击力下降抗性+10%，每次升级抗性提升10%。
  - Current: 冰冻、石化、定身、攻击力下降抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank1 description:
  - Old: 冰冻，石化，定身，攻击力下降抗性+10%。
  - Current: 冰冻、石化、定身、攻击力下降抗性+10%；并使自身对所有不良状态的抵抗能力提高3%。
- Rank5 description:
  - Old: 冰冻，石化，定身，攻击力下降抗性+50%。
  - Current: 冰冻、石化、定身、攻击力下降抗性+50%；并使自身对所有不良状态的抵抗能力提高13%。

## 10027 重击 → 重击 (战士)

Decision: native_effect_numeric_or_tradeoff_changed

- Rank1 description:
  - Old: 造成100%伤害，使敌人昏迷6秒，昏迷中的敌人无法回避攻击。
  - Current: 造成160%伤害，使敌人昏迷6秒，昏迷中的敌人无法回避攻击。
- Rank5 description:
  - Old: 造成100%伤害，使敌人昏迷10秒，昏迷中的敌人无法回避攻击。
  - Current: 造成160%伤害，使敌人昏迷10秒，昏迷中的敌人无法回避攻击。

## 10028 战吼 → 战吼 (战士)

Decision: radius_text_table_conflict_requires_field_level_decision

- radius: 200 → 100
- levels.lreq: rank1 36→30; rank2 38→32; rank3 40→34; rank4 42→36; rank5 44→38

## 10029 震慑 → 震慑 (战士)

Decision: old_top_description_obsolete_after_2005_12_22_official_labeled_update

- General description:
  - Old: 将以自己为中心半径100的圆范围内的敌人吓跑，此技能尚无法作用。
  - Current: 将以自己为中心半径150的圆范围内的敌人震晕5秒。
- levels.cd: rank1 3000→120; rank2 3000→120; rank3 3000→120; rank4 3000→120; rank5 3000→120
- levels.lreq: rank1 51→42; rank2 53→44; rank3 55→46; rank4 57→48; rank5 59→50

## 10030 缴械 → 缴械 (战士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.lreq: rank1 60→45; rank2 62→47; rank3 64→49; rank4 66→51; rank5 68→53

## 10102 罡气剑 → 罡气剑 (剑客)

Decision: confirmed_single_to_area_with_SQL_and_historical_support

- General description:
  - Old: 造成130%伤害，震退敌人，每次升级提升伤害3%。
  - Current: 对120度角，半径120扇形范围内的敌人造成130%伤害，震退敌人。
- radius: 0 → 120
- rwidth: 0 → 120
- shape: 0 → 3
- Rank1 description:
  - Old: 造成130%伤害，震退敌人。
  - Current: 对120度角，半径120扇形范围内的敌人造成130%伤害，震退敌人。
- Rank5 description:
  - Old: 造成142%伤害，震退敌人。
  - Current: 对120度角，半径120扇形范围内的敌人造成142%伤害，震退敌人。

## 10104 饮血剑 → 饮血剑 (剑客)

Decision: native_effect_numeric_or_tradeoff_changed

- Rank2 description:
  - Old: 造成155%伤害，攻击力的22%转化为生命。
  - Current: 造成155%伤害，攻击力的26%转化为生命。
- Rank5 description:
  - Old: 造成170%伤害，攻击力的28%转化为生命。
  - Current: 造成170%伤害，攻击力的44%转化为生命。

## 10105 龙杀剑 → 龙杀剑 (剑客)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 4000→0; rank2 4000→0; rank3 4000→0; rank4 4000→0; rank5 4000→0
- levels.cd: rank1 0→40; rank2 0→40; rank3 0→40; rank4 0→40; rank5 0→40

## 10108 心镜 → 心镜 (剑客)

Decision: custom_effect_or_scope_added

- General description:
  - Old: 昏迷抗性+10%，每次升级抗性提升10%。
  - Current: 昏迷抗性+10%；并使自身对所有不良状态的抵抗能力提高8%。
- Rank1 description:
  - Old: 昏迷抗性+10%。
  - Current: 昏迷抗性+10%；并使自身对所有不良状态的抵抗能力提高8%。
- Rank5 description:
  - Old: 昏迷抗性+50%。
  - Current: 昏迷抗性+50%；并使自身对所有不良状态的抵抗能力提高40%。

## 10115 火字诀 → 火字诀 (剑客)

Decision: native_effect_numeric_or_tradeoff_changed; old_top_15s_vs_old_rank_30s_conflict_do_not_choose_duration_from_top_alone

- General description:
  - Old: 15秒内，防御降低10%，攻击力提升10%，每次升级防御降低3%，攻击力提升3%。
  - Current: 30秒内，攻击力提升10%。（与山字诀不能同时存在）
- levels.cast: rank1 3000→800; rank2 3000→800; rank3 3000→800; rank4 3000→800; rank5 3000→800
- Rank1 description:
  - Old: 30秒内，防御降低10%，攻击力提升10%。
  - Current: 30秒内，攻击力提升10%。（与山字诀不能同时存在）
- Rank5 description:
  - Old: 30秒内，防御降低22%，攻击力提升22%。
  - Current: 30秒内，攻击力提升25%。（与山字诀不能同时存在）
- levels.lreq: rank1 60→43; rank2 62→45; rank3 64→47; rank4 66→49; rank5 68→51

## 10118 风斩 → 剑意 (剑客)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 真空斩威力增加5%，每次升级威力增加5%。
  - Current: 每使用一条不同的剑类技能积攒1层剑意（最多5层），每层使剑类技能伤害提高4%；5秒内没有出手则清空。
- name: 风斩 → 剑意
- Rank1 description:
  - Old: 真空斩威力增加5%。
  - Current: 每使用一条不同的剑类技能积攒1层剑意（最多5层），每层使剑类技能伤害提高4%；5秒内没有出手则清空。
- Rank5 description:
  - Old: 真空斩威力增加25%。
  - Current: 每使用一条不同的剑类技能积攒1层剑意（最多5层），每层使剑类技能伤害提高8%；5秒内没有出手则清空。

## 10121 旋风剑 → 旋风剑 (剑客)

Decision: radius_text_table_conflict_requires_field_level_decision

- Rank1 description:
  - Old: 对以自己为中心半径100的圆范围内的敌人造成130%的伤害。
  - Current: 对以自己为中心半径150的圆范围内的敌人造成130%的伤害。
- Rank5 description:
  - Old: 对以自己为中心半径100的圆范围内的敌人造成150%的伤害。
  - Current: 对以自己为中心半径150的圆范围内的敌人造成150%的伤害。
- levels.lreq: rank1 36→25; rank2 38→27; rank3 40→29; rank4 42→31; rank5 44→33

## 10122 混沌剑 → 混沌剑 (剑客)

Decision: confirmed_single_to_area_with_SQL_and_historical_support

- radius: 0 → 150
- shape: 0 → 2
- Rank1 description:
  - Old: 对敌人造成160%伤害，10秒内，敌人无法受到任何有益的状态的效果。
  - Current: 对以自己为中心半径150的圆范围内的敌人造成160%伤害，10秒内，敌人无法受到任何有益的状态的效果。
- Rank5 description:
  - Old: 对敌人造成160%伤害，14秒内，敌人无法受到任何有益的状态的效果。
  - Current: 对以自己为中心半径150的圆范围内的敌人造成160%伤害，14秒内，敌人无法受到任何有益的状态的效果。

## 10123 陨星剑 → 陨星剑 (剑客)

Decision: native_effect_numeric_or_tradeoff_changed

- Rank2 description:
  - Old: 对敌人造成175%的伤害，2%的几率出现即死判定。
  - Current: 对敌人造成175%的伤害，3%的几率出现即死判定。
- Rank5 description:
  - Old: 对敌人造成205%的伤害，2%的几率出现即死判定。
  - Current: 对敌人造成205%的伤害，6%的几率出现即死判定。

## 10124 山字诀 → 山字诀 (剑客)

Decision: native_effect_numeric_or_tradeoff_changed; old_top_15s_vs_old_rank_30s_conflict_do_not_choose_duration_from_top_alone

- General description:
  - Old: 15秒内，攻击力下降10%，防御力上升10%，每次升级攻击力下降3%，防御力提升3%。
  - Current: 30秒内，受到的伤害减少10%。（与火字诀不能同时存在）
- levels.cast: rank1 3000→800; rank2 3000→800; rank3 3000→800; rank4 3000→800; rank5 3000→800
- Rank1 description:
  - Old: 30秒内，攻击力下降10%，防御力上升10%。
  - Current: 30秒内，受到的伤害减少10%。（与火字诀不能同时存在）
- Rank5 description:
  - Old: 30秒内，攻击力下降22%，防御力上升22%。
  - Current: 30秒内，受到的伤害减少25%。（与火字诀不能同时存在）
- levels.lreq: rank1 60→43; rank2 62→45; rank3 64→47; rank4 66→49; rank5 68→51

## 10125 血雨剑 → 血雨剑 (剑客)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 4000→0; rank2 4000→0; rank3 4000→0; rank4 4000→0; rank5 4000→0
- levels.cd: rank1 0→40; rank2 0→40; rank3 0→40; rank4 0→40; rank5 0→40

## 10127 挑衅 → 挑衅 (剑客)

Decision: radius_text_table_conflict_requires_field_level_decision

- radius: 200 → 100

## 10130 雷霆之怒 → 雷霆之怒 (剑客)

Decision: current_geometry_repairs_old_conflict_do_not_blindly_revert

- radius: 150 → 0
- shape: 2 → 0

## 10203 反击术 → 反击术 (刺客)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 成功回避敌人攻击的时候40%的几率反击对手，每次升级反击的几率增加3%。
  - Current: 成功回避敌人攻击的时候40%的几率积攒1层反击势（最多5层）；你下一次攻击命中时，每层追加一次反击伤害。
- Rank1 description:
  - Old: 成功回避敌人攻击的时候40%的几率反击对手。
  - Current: 成功回避敌人攻击的时候40%的几率积攒1层反击势（最多5层）；你下一次攻击命中时，每层追加一次反击伤害。
- Rank5 description:
  - Old: 成功回避敌人攻击的时候52%的几率反击对手。
  - Current: 成功回避敌人攻击的时候52%的几率积攒1层反击势（最多5层）；你下一次攻击命中时，每层追加一次反击伤害。

## 10212 偷窃术 → 偷窃术 (刺客)

Decision: old_levels_corrupt_current_theft_direction_supported_exact_restrictions_unproven

- Rank1 description:
  - Old: 造成100%的伤害。
  - Current: 50%的几率发生偷窃判定，按照该怪物的掉宝几率获得物品。（每只怪只能被成功偷窃一次）
- Rank5 description:
  - Old: 造成112%的伤害。
  - Current: 74%的几率发生偷窃判定，按照该怪物的掉宝几率获得物品。（每只怪只能被成功偷窃一次）

## 10213 陷阱：爆弹 → 陷阱：爆弹 (刺客)

Decision: trap_geometry_is_routing_metadata_requires_semantic_resolution

- radius: 0 → 100
- shape: 0 → 2

## 10214 陷阱：毒雾 → 陷阱：毒雾 (刺客)

Decision: trap_geometry_is_routing_metadata_requires_semantic_resolution

- radius: 0 → 100
- shape: 0 → 2

## 10215 陷阱：封印 → 陷阱：封印 (刺客)

Decision: trap_geometry_is_routing_metadata_requires_semantic_resolution

- radius: 0 → 100
- shape: 0 → 2

## 10216 陷阱：冻气 → 陷阱：冻气 (刺客)

Decision: trap_geometry_is_routing_metadata_requires_semantic_resolution

- radius: 0 → 100
- shape: 0 → 2

## 10218 解除陷阱 → 拆解 (刺客)

Decision: custom_design_replaces_native_identity_or_mechanic

- castkind: 0 → 1
- General description:
  - Old: 解除看到的陷阱，每次升级成功率增加。
  - Current: 解除指定位置附近的陷阱，并对该位置半径150的圆形范围内的敌人造成150%的伤害。
- name: 解除陷阱 → 拆解
- radius: 0 → 150
- shape: 0 → 2
- Rank1 description:
  - Old: 解除看到的陷阱，成功率55%。
  - Current: 解除指定位置附近的陷阱，并对该位置半径150的圆形范围内的敌人造成150%的伤害。
- Rank5 description:
  - Old: 解除看到的陷阱，成功率87%。
  - Current: 解除指定位置附近的陷阱，并对该位置半径150的圆形范围内的敌人造成250%的伤害。

## 10227 阴影跳跃 → 阴影跳跃 (刺客)

Decision: friendly_target_change_matches_authored_prose_do_not_blindly_revert

- canFriendly: 0 → 1
- General description:
  - Old: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离，此技能尚无法作用，此技能尚无法作用。
  - Current: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离。
- Rank1 description:
  - Old: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离，此技能尚无法作用。
  - Current: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离。
- Rank5 description:
  - Old: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离，此技能尚无法作用。
  - Current: 跳跃至选中敌人或队友旁边，每次升级增加跳跃距离。

## 10228 侦测陷阱 → 侦测陷阱 (刺客)

Decision: trap_geometry_is_routing_metadata_requires_semantic_resolution

- radius: 200 → 0
- shape: 2 → 0

## 10229 陷阱回避 → 布雷 (刺客)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 当触发陷阱时有20%的几率只受一半伤害，每次升级几率增加8%，此技能尚无法作用。
  - Current: 可同时布置的陷阱数量上限提高1个。
- name: 陷阱回避 → 布雷
- Rank1 description:
  - Old: 当触发陷阱时有20%的几率只受一半伤害，此技能尚无法作用。
  - Current: 可同时布置的陷阱数量上限提高1个。
- Rank5 description:
  - Old: 当触发陷阱时有52%的几率只受一半伤害，此技能尚无法作用。
  - Current: 可同时布置的陷阱数量上限提高3个。

## 10302 复法术 → 复法术 (药师)

Decision: native_effect_numeric_or_tradeoff_changed

- General description:
  - Old: 目标恢复少量法力，每次升级恢复量增加。
  - Current: 目标恢复500点法力。
- levels.cd: rank1 0→50; rank2 0→50; rank3 0→50; rank4 0→50; rank5 0→50
- Rank1 description:
  - Old: 目标恢复100点法力。
  - Current: 目标恢复500点法力。
- Rank5 description:
  - Old: 目标恢复500点法力。
  - Current: 目标恢复1500点法力。

## 10303 回春术 → 回春术 (药师)

Decision: native_effect_numeric_or_tradeoff_changed

- General description:
  - Old: 目标一段时间内缓慢的回血，每次升级作用时限，回复效果增加。
  - Current: 120秒内，目标每秒恢复最大生命的0.5%。
- Rank1 description:
  - Old: 120秒内，目标每秒恢复生命值14点。
  - Current: 120秒内，目标每秒恢复最大生命的0.5%。
- Rank5 description:
  - Old: 180秒内，目标每秒恢复生命值30点。
  - Current: 180秒内，目标每秒恢复最大生命的1.2%。

## 10304 群疗术 → 群疗术 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 3600→1200; rank2 3600→1200; rank3 3600→1200; rank4 3600→1200; rank5 3600→1200
- levels.cd: rank1 0→80; rank2 0→80; rank3 0→80; rank4 0→80; rank5 0→80

## 10306 净化术 → 净世 (药师)

Decision: custom_design_replaces_native_identity_or_mechanic

- General description:
  - Old: 消除队友身上昏迷和中毒的效果，每次升级消除的效果增强。
  - Current: 消除队友身上的昏迷、中毒效果。
- name: 净化术 → 净世
- Rank1 description:
  - Old: 消除队友身上昏迷和中毒的效果，每次升级消除的效果增强。
  - Current: 消除队友身上的昏迷、中毒效果。
- Rank5 description:
  - Old: 消除队友身上昏迷和中毒的效果，每次升级消除的效果增强。
  - Current: 消除队友身上的昏迷、中毒、减速、防御力下降、攻击力下降、定身、冻结、石化、封印效果。
- levels.mp: rank1 48→100; rank2 48→100; rank3 48→100; rank4 48→100; rank5 48→100
- levels.lreq: rank2 22→30; rank3 24→43; rank4 26→51; rank5 28→60

## 10307 祝福术 → 祝祷 (药师)

Decision: custom_design_replaces_native_identity_or_mechanic

- name: 祝福术 → 祝祷
- pre: 10321 → 0
- Rank1 description:
  - Old: 以自己为中心半径200圆形范围内的队友5分钟内攻击力增加10%；魔法攻击力增加5%。
  - Current: 以自己为中心半径200圆形范围内的队友（含自己）5分钟内：攻击力提高10%，魔法攻击力提高5%，受到的伤害降低10%。
- Rank5 description:
  - Old: 以自己为中心半径200圆形范围内的队友5分钟内攻击力增加18%；魔法攻击力增加25%。
  - Current: 以自己为中心半径200圆形范围内的队友（含自己）5分钟内：攻击力提高18%，魔法攻击力提高25%，受到的伤害降低22%。
- levels.mp: rank1 35→120; rank2 35→120; rank3 35→120; rank4 35→120; rank5 35→120
- levels.lreq: rank2 16→20; rank3 17→24; rank4 18→26; rank5 19→28

## 10308 神圣庇护 → 神圣庇护 (药师)

Decision: current_geometry_repairs_old_conflict_do_not_blindly_revert

- radius: 0 → 150
- shape: 0 → 2
- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→1200; rank2 0→1200; rank3 0→1200; rank4 0→1200; rank5 0→1200

## 10309 光之奇迹 → 光之奇迹 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→600; rank2 0→600; rank3 0→600; rank4 0→600; rank5 0→600

## 10311 加速术 → 加速术 (药师)

Decision: current_geometry_repairs_old_conflict_do_not_blindly_revert

- dist: 450 → 0
- radius: 0 → 150
- shape: 0 → 2

## 10312 驱魔术 → 驱魔术 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- General description:
  - Old: 对半径150范围内的不死系和恶魔系敌人造成280%的伤害，每次升级伤害增加10%。
  - Current: 对半径150范围内的敌人造成280%的伤害。
- levels.cd: rank1 40→20; rank2 40→20; rank3 40→20; rank4 40→20; rank5 40→20
- Rank1 description:
  - Old: 对半径150范围内的不死系和恶魔系敌人造成280%的伤害。
  - Current: 对半径150范围内的敌人造成280%的伤害。
- Rank5 description:
  - Old: 对半径150范围内的不死系和恶魔系敌人造成320%的伤害。
  - Current: 对半径150范围内的敌人造成320%的伤害。

## 10313 天罚 → 天罚 (药师)

Decision: radius_text_table_conflict_requires_field_level_decision

- General description:
  - Old: 对半径150范围内的敌人造成300%的伤害，每次升级伤害增加10%，随机使敌人中各种负面状态。
  - Current: 对半径200范围内的敌人造成300%的伤害，每次升级伤害增加10%，随机使敌人中各种负面状态。
- dist: 0 → 450
- radius: 0 → 200
- shape: 0 → 2
- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→50; rank2 0→50; rank3 0→50; rank4 0→50; rank5 0→50
- Rank1 description:
  - Old: 对半径150范围内的敌人造成300%的伤害，随机使敌人中各种负面状态。
  - Current: 对半径200范围内的敌人造成300%的伤害，随机使敌人中各种负面状态。
- Rank5 description:
  - Old: 对半径150范围内的敌人造成340%的伤害，随机使敌人中各种负面状态。
  - Current: 对半径200范围内的敌人造成340%的伤害，随机使敌人中各种负面状态。

## 10314 术法精研 → 术法精研 (药师)

Decision: custom_effect_or_scope_added

- Rank1 description:
  - Old: 技能吟唱被打断的几率下降20%（最小值为5%），同时技能吟唱时间降低20%。
  - Current: 技能吟唱被打断的几率下降20%（最小值为5%），同时技能吟唱时间降低20%；魔法攻击力提高3%；对处于致盲状态的目标造成的伤害提高5%。
- Rank5 description:
  - Old: 技能吟唱被打断的几率下降40%（最小值为5%），同时技能吟唱时间降低40%。
  - Current: 技能吟唱被打断的几率下降40%（最小值为5%），同时技能吟唱时间降低20%；魔法攻击力提高15%；对处于致盲状态的目标造成的伤害提高25%。

## 10315 瞬击术 → 瞬击术 (药师)

Decision: current_geometry_repairs_old_conflict_do_not_blindly_revert

- General description:
  - Old: 以自己为中心半径150圆形范围内的队友一分钟内攻击速度增加25%，每次升级效果增加2%。
  - Current: 以自己为中心半径150圆形范围内的队友1分钟内攻击速度增加25%，每次升级效果增加2%。
- radius: 0 → 150
- shape: 0 → 2
- Rank1 description:
  - Old: 以自己为中心半径150圆形范围内的队友一分钟内攻击速度增加25%。
  - Current: 以自己为中心半径150圆形范围内的队友1分钟内攻击速度增加25%。
- Rank5 description:
  - Old: 以自己为中心半径150圆形范围内的队友一分钟内攻击速度增加33%。
  - Current: 以自己为中心半径150圆形范围内的队友1分钟内攻击速度增加33%。

## 10316 清身术 → [removed] (药师)

Decision: missing_native_skill_restore_identity_and_links_with_SQL_historical_support

- presence: true → false

## 10317 圣光术 → 圣光术 (药师)

Decision: confirmed_single_to_area_with_SQL_and_historical_support

- General description:
  - Old: 对敌人造成240%的伤害，并使其致盲，每次升级伤害提升10%。
  - Current: 对半径110的圆形范围内的敌人造成240%的伤害，并使其致盲。
- radius: 0 → 110
- shape: 0 → 2
- Rank1 description:
  - Old: 对敌人造成240%的伤害，并使其致盲。
  - Current: 对半径110的圆形范围内的敌人造成240%的伤害，并使其致盲。
- Rank5 description:
  - Old: 对敌人造成280%的伤害，并使其致盲。
  - Current: 对半径110的圆形范围内的敌人造成280%的伤害，并使其致盲。

## 10318 解咒术 → [removed] (药师)

Decision: missing_native_skill_restore_identity_and_links_with_SQL_historical_support

- presence: true → false

## 10320 光盾术 → 光盾术 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- pre: 10321 → 0

## 10321 防御术 → [removed] (药师)

Decision: missing_native_skill_restore_identity_and_links_with_SQL_historical_support

- presence: true → false

## 10324 衰弱术 → 衰弱术 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- pre: 10321 → 0

## 10326 神威术 → 神威术 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cd: rank1 300→120; rank2 300→120; rank3 300→120; rank4 300→120; rank5 300→120; rank6 300→120; rank7 300→120; rank8 300→120; rank9 300→120; rank10 300→120

## 10327 神术：鹰眼 → 神术：照亮 (药师)

Decision: custom_design_replaces_native_identity_or_mechanic; radius_text_table_conflict_requires_field_level_decision

- General description:
  - Old: 使以自己为中心半径300的圆形范围内的隐身敌人现形，现行的敌人无法隐身一段时间，每次升级效果增强。
  - Current: 使以自己为中心半径300的圆形范围内的敌人被照亮，15秒内受到的伤害提高8%；同时使范围内的隐身敌人现形，并使其短时间内无法隐身。
- name: 神术：鹰眼 → 神术：照亮
- radius: 350 → 300
- Rank1 description:
  - Old: 使以自己为中心半径300的圆形范围内的隐身敌人现形，现行的敌人无法隐身一段时间，每次升级效果增强。
  - Current: 使以自己为中心半径300的圆形范围内的敌人被照亮，15秒内受到的伤害提高8%；同时使范围内的隐身敌人现形，并使其短时间内无法隐身。
- Rank5 description:
  - Old: 使以自己为中心半径300的圆形范围内的隐身敌人现形，现行的敌人无法隐身一段时间，每次升级效果增强。
  - Current: 使以自己为中心半径300的圆形范围内的敌人被照亮，15秒内受到的伤害提高20%；同时使范围内的隐身敌人现形，并使其短时间内无法隐身。

## 10330 针刺护体 → 针刺护体 (药师)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 3600→1000; rank2 3600→1000; rank3 3600→1000; rank4 3600→1000; rank5 3600→1000

## 10401 火弹术 → 火弹术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 1000→400; rank2 1000→400; rank3 1000→400; rank4 1000→400; rank5 1000→400

## 10402 火球术 → 火球术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 1000→400; rank2 1000→400; rank3 1000→400; rank4 1000→400; rank5 1000→400

## 10405 风焰术 → 风焰术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cd: rank1 100→60; rank2 100→60; rank3 100→60; rank4 100→60; rank5 100→60

## 10408 陨石术 → 陨石术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→50; rank2 0→50; rank3 0→50; rank4 0→50; rank5 0→50

## 10409 冰箭术 → 冰箭术 (术士)

Decision: custom_effect_or_scope_added; old_top_165pct_vs_old_rank_100pct_conflict

- General description:
  - Old: 对敌人造成165%的伤害，每次升级伤害提升10%。
  - Current: 对敌人造成150%的伤害，并使其移动速度降低20%，持续5秒。
- Rank1 description:
  - Old: 对敌人造成100%的伤害。
  - Current: 对敌人造成150%的伤害，并使其移动速度降低20%，持续5秒。
- Rank5 description:
  - Old: 对敌人造成120%的伤害。
  - Current: 对敌人造成210%的伤害，并使其移动速度降低40%，持续5秒。

## 10411 绝对零度 → 绝对零度 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→50; rank2 0→50; rank3 0→50; rank4 0→50; rank5 0→50

## 10412 暴雪术 → 暴雪术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cd: rank1 100→60; rank2 100→60; rank3 100→60; rank4 100→60; rank5 100→60

## 10414 虚空 → 虚空 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.cast: rank1 5000→0; rank2 5000→0; rank3 5000→0; rank4 5000→0; rank5 5000→0
- levels.cd: rank1 0→50; rank2 0→50; rank3 0→50; rank4 0→50; rank5 0→50

## 10417 魔镜术 → 魔镜术 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.lreq: rank1 51→42; rank2 53→44; rank3 55→46; rank4 57→48; rank5 59→50

## 10424 红莲之魂 → 红莲之魂 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.lreq: rank1 60→51; rank2 62→53; rank3 64→55; rank4 66→57; rank5 68→59

## 10431 寒冰之魂 → 寒冰之魂 (术士)

Decision: parameter_change_verified_official_version_approval_still_required

- levels.lreq: rank1 60→51; rank2 62→53; rank3 64→55; rank4 66→57; rank5 68→59

## 10433 秘术：消除 → 秘术：消除 (术士)

Decision: custom_effect_or_scope_added

- Rank1 description:
  - Old: 消除敌人的防御魔法，包括光盾术，火盾术，冰盾术，魔镜术，针刺护体，每次升级消除的效果增加。
  - Current: 消除敌人的防御魔法，包括光盾术，火盾术，冰盾术，魔镜术，针刺护体，每次升级消除的效果增加，并对其造成120%的魔法伤害。
- Rank5 description:
  - Old: 消除敌人的防御魔法，包括光盾术，火盾术，冰盾术，魔镜术，针刺护体，每次升级消除的效果增加。
  - Current: 消除敌人的防御魔法，包括光盾术，火盾术，冰盾术，魔镜术，针刺护体，每次升级消除的效果增加，并对其造成200%的魔法伤害。

