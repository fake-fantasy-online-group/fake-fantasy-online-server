# Five-profession field-level restoration decisions

All71 changed/removed skill entries are covered. A decision applies to the named field, not to the entire historical resource row. This is an audit proposal, not implemented source code.

Evidence: hash-verified local backup and current resource; root SQL/actual loaded rank fixture; separately versioned historical documentation. Exact numeric differences are in verified-asset-differences.json.

Important: all745 rank1–5 descriptions in the historical backup match imported SQL exactly. The backup still contains malformed shape fields, flattened MP, inconsistent general text and16 unused rank6–10 blocks. It is not an authenticated official release.

## 10006 震地之枪 (战士)

- desc → **hold_geometry_contract**. Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **hold_geometry_contract**. Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10008 意志 (战士)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10012 破甲 (战士)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10014 破魔 (战士)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "破魔"
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10015 心眼之枪 (战士)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10016 石化之枪 (战士)

- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10022 野战八荒 (战士)

- radius → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 150
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10023 战神之枪 (战士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:2000, rank2:2000, rank3:2000, rank4:2000, rank5:2000
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10024 维护 (战士)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "维护"
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10025 强健 (战士)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10026 抗魔 (战士)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10027 重击 (战士)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10028 战吼 (战士)

- radius → **keep_client_hold_server_radius**. Current client radius matches original authored descriptions and historical page. Old raw radius and SQL disagree with that prose; do not reintroduce old radius solely by blanket rollback. Server radius still needs explicit evidence-based reconciliation.
  - Value: 100
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:36, rank2:38, rank3:40, rank4:42, rank5:44
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10029 震慑 (战士)

- desc → **keep**. Old general fear/unimplemented text is obsolete. Dated2005-12-22 official-labeled update explicitly establishes radius150 stun5..9s; SQL and old rank text match that update.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:3000, rank2:3000, rank3:3000, rank4:3000, rank5:3000
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:51, rank2:53, rank3:55, rank4:57, rank5:59
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html; https://fo.17173.com/gengxin/20051222.shtml

## 10030 缴械 (战士)

- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_2.html

## 10102 罡气剑 (剑客)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- radius → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- rwidth → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- shape → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10104 饮血剑 (剑客)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10105 龙杀剑 (剑客)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:4000, rank2:4000, rank3:4000, rank4:4000, rank5:4000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10108 心镜 (剑客)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10115 火字诀 (剑客)

- desc → **restore_from_verified_ranks**. Remove redesigned stance/tradeoff changes, but do not copy obsolete15s general text: both backup rank descriptions and SQL say30s. Generate general original summary from verified ranks with attack/defense tradeoff and10/13/16/19/22% progression.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:3000, rank2:3000, rank3:3000, rank4:3000, rank5:3000
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10118 风斩 (剑客)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "风斩"
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10121 旋风剑 (剑客)

- levels.desc → **hold_geometry_contract**. Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:36, rank2:38, rank3:40, rank4:42, rank5:44
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10122 混沌剑 (剑客)

- radius → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- shape → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10123 陨星剑 (剑客)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10124 山字诀 (剑客)

- desc → **restore_from_verified_ranks**. Remove redesigned stance/tradeoff changes, but do not copy obsolete15s general text: both backup rank descriptions and SQL say30s. Generate general original summary from verified ranks with attack/defense tradeoff and10/13/16/19/22% progression.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:3000, rank2:3000, rank3:3000, rank4:3000, rank5:3000
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10125 血雨剑 (剑客)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:4000, rank2:4000, rank3:4000, rank4:4000, rank5:4000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10127 挑衅 (剑客)

- radius → **keep_client_hold_server_radius**. Current client radius matches original authored descriptions and historical page. Old raw radius and SQL disagree with that prose; do not reintroduce old radius solely by blanket rollback. Server radius still needs explicit evidence-based reconciliation.
  - Value: 100
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10130 雷霆之怒 (剑客)

- radius → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 0
- shape → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_1.html

## 10203 反击术 (刺客)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10212 偷窃术 (刺客)

- levels.desc → **keep_with_unverified_clause**. Old rank text is incorrectly pure damage despite old top text and name describing theft. Current50..74% theft agrees with historical source direction; per-monster-once restriction is not independently verified. Do not restore damage text; server identity/effect still needs a separately approved theft implementation.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10213 陷阱：爆弹 (刺客)

- radius → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 100
- shape → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10214 陷阱：毒雾 (刺客)

- radius → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 100
- shape → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10215 陷阱：封印 (刺客)

- radius → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 100
- shape → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10216 陷阱：冻气 (刺客)

- radius → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 100
- shape → **keep**. Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10218 解除陷阱 (刺客)

- castkind → **restore_native_trap_disarm**. Remove invented AoE attack and rename. Native action selects a visible trap; classify explicitly as trap-disarm, not generic castkind0 Instant. Restore original probability55/63/71/79/87% from verified ranks.
  - Value: 0
- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "解除陷阱"
- radius → **restore_native_trap_disarm**. Remove invented AoE attack and rename. Native action selects a visible trap; classify explicitly as trap-disarm, not generic castkind0 Instant. Restore original probability55/63/71/79/87% from verified ranks.
  - Value: 0
- shape → **restore_native_trap_disarm**. Remove invented AoE attack and rename. Native action selects a visible trap; classify explicitly as trap-disarm, not generic castkind0 Instant. Restore original probability55/63/71/79/87% from verified ranks.
  - Value: 0
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10227 阴影跳跃 (刺客)

- canFriendly → **keep**. Enemy-or-teammate leap is explicitly described by backup, SQL and historical table. Friendly targeting1 repairs old contradictory0 flag.
  - Value: 1
- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10228 侦测陷阱 (刺客)

- radius → **restore**. Backup and SQL preserve native self-centered detection area200. Restore detector radius/metadata while retaining self-action routing, not an enemy AoE attack.
  - Value: 200
- shape → **restore**. Backup and SQL preserve native self-centered detection area200. Restore detector radius/metadata while retaining self-action routing, not an enemy AoE attack.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10229 陷阱回避 (刺客)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "陷阱回避"
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_3.html

## 10302 复法术 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10303 回春术 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10304 群疗术 (药师)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:3600
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
- levels.cast → **restore_server_contract**. Both client versions flatten some native timing progressions. Synchronize request/display with server actual rank timing; official historical slope is not independently authenticated.
  - Value: rank2:3850, rank3:4100, rank4:4350, rank5:4600
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10306 净化术 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "净化术"
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.mp → **restore_server_contract**. Current100/120 local gate exceeds server native rank cost. Use current authoritative server rank cost; backup flattens MP so do not copy old level2..5 values. Historical MP curve authenticity remains separate.
  - Value: rank1:48, rank2:52, rank3:56, rank4:60, rank5:64
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank2:22, rank3:24, rank4:26, rank5:28
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10307 祝福术 (药师)

- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "祝福术"
- pre → **hold**. Backup requires10321 but raw SQL for this skill has no prerequisite. Current0 matches SQL; no independently verified historical prerequisite. Restore missing10321 first, keep current prerequisite until version evidence resolves it.
  - Value: null
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.mp → **restore_server_contract**. Current100/120 local gate exceeds server native rank cost. Use current authoritative server rank cost; backup flattens MP so do not copy old level2..5 values. Historical MP curve authenticity remains separate.
  - Value: rank1:35, rank2:37, rank3:39, rank4:41, rank5:43
- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank2:16, rank3:17, rank4:18, rank5:19
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10308 神圣庇护 (药师)

- radius → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 150
- shape → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 2
- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10309 光之奇迹 (药师)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10311 加速术 (药师)

- dist → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 0
- radius → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 150
- shape → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 2
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10312 驱魔术 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented. Server lacks audited undead/demon taxonomy; enforce that original target restriction before claiming behavior restored.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:40, rank2:40, rank3:40, rank4:40, rank5:40
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented. Server lacks audited undead/demon taxonomy; enforce that original target restriction before claiming behavior restored.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10313 天罚 (药师)

- desc → **hold_geometry_contract**. Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.
  - Value: See exact rank/general values in verified-asset-differences.json.
- dist → **keep**. Original description and SQL identify targeted circular AoE, whereas backup shape0/dist0 is incomplete. Current shape2/dist450 restores native target mode.
  - Value: 450
- radius → **hold_geometry_contract**. Current200 agrees with raw SQL radius; historical/backup/SQL prose says150. Preserve current numeric setting pending a deliberate canonical-radius decision; never relabel this as certified original.
  - Value: null
- shape → **keep**. Original description and SQL identify targeted circular AoE, whereas backup shape0/dist0 is incomplete. Current shape2/dist450 restores native target mode.
  - Value: 2
- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
- levels.desc → **hold_geometry_contract**. Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10314 术法精研 (药师)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10315 瞬击术 (药师)

- desc → **keep**. This text change only spells one minute as1分钟 rather than一分钟; no mechanic difference.
  - Value: See exact rank/general values in verified-asset-differences.json.
- radius → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 150
- shape → **keep**. Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.
  - Value: 2
- levels.desc → **keep**. This text change only spells one minute as1分钟 rather than一分钟; no mechanic difference.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10316 清身术 (药师)

- presence → **restore**. Skill exists in backup, imported SQL, loaded server catalog and historical profession documentation. Restore complete client metadata and selection, icon and tooltip identity; do not copy flattened backup MP.
  - Value: true
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10317 圣光术 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- radius → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- shape → **restore**. Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.
  - Value: 0
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10318 解咒术 (药师)

- presence → **restore**. Skill exists in backup, imported SQL, loaded server catalog and historical profession documentation. Restore complete client metadata and selection, icon and tooltip identity; do not copy flattened backup MP.
  - Value: true
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10320 光盾术 (药师)

- pre → **hold**. Backup requires10321 but raw SQL for this skill has no prerequisite. Current0 matches SQL; no independently verified historical prerequisite. Restore missing10321 first, keep current prerequisite until version evidence resolves it.
  - Value: null
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10321 防御术 (药师)

- presence → **restore**. Skill exists in backup, imported SQL, loaded server catalog and historical profession documentation. Restore complete client metadata and selection, icon and tooltip identity; do not copy flattened backup MP.
  - Value: true
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10324 衰弱术 (药师)

- pre → **hold**. Backup requires10321 but raw SQL for this skill has no prerequisite. Current0 matches SQL; no independently verified historical prerequisite. Restore missing10321 first, keep current prerequisite until version evidence resolves it.
  - Value: null
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10326 神威术 (药师)

- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:300, rank2:300, rank3:300, rank4:300, rank5:300
- levels.cd → **restore_server_contract**. Both client versions flatten some native timing progressions. Synchronize request/display with server actual rank timing; official historical slope is not independently authenticated.
  - Value: rank6:null, rank7:null, rank8:null, rank9:null, rank10:null
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10327 神术：鹰眼 (药师)

- desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
- name → **restore**. Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.
  - Value: "神术：鹰眼"
- radius → **keep_client_hold_server_radius**. Current client radius matches original authored descriptions and historical page. Old raw radius and SQL disagree with that prose; do not reintroduce old radius solely by blanket rollback. Server radius still needs explicit evidence-based reconciliation.
  - Value: 300
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10330 针刺护体 (药师)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:3600, rank2:3600, rank3:3600, rank4:3600, rank5:3600
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_4.html

## 10401 火弹术 (术士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:1000, rank2:1000, rank3:1000, rank4:1000, rank5:1000
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10402 火球术 (术士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:1000, rank2:1000, rank3:1000, rank4:1000, rank5:1000
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10405 风焰术 (术士)

- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:100, rank2:100, rank3:100, rank4:100, rank5:100
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10408 陨石术 (术士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10409 冰箭术 (术士)

- desc → **restore_from_verified_ranks**. Remove unconditional slow and added damage. Old general165% text contradicts old/SQL five-rank100/105/110/115/120% and historical100%+5%. Regenerate summary from corroborated ranks.
  - Value: See exact rank/general values in verified-asset-differences.json.
- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10411 绝对零度 (术士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10412 暴雪术 (术士)

- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:100, rank2:100, rank3:100, rank4:100, rank5:100
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10414 虚空 (术士)

- levels.cast → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:5000, rank2:5000, rank3:5000, rank4:5000, rank5:5000
- levels.cd → **restore**. Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.
  - Value: rank1:0, rank2:0, rank3:0, rank4:0, rank5:0
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10417 魔镜术 (术士)

- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:51, rank2:53, rank3:55, rank4:57, rank5:59
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10424 红莲之魂 (术士)

- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10431 寒冰之魂 (术士)

- levels.lreq → **restore**. Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.
  - Value: rank1:60, rank2:62, rank3:64, rank4:66, rank5:68
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

## 10433 秘术：消除 (术士)

- levels.desc → **restore**. Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.
  - Value: See exact rank/general values in verified-asset-differences.json.
  - Sources: historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37; server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json; https://fo.17173.com/character/character_5.html

