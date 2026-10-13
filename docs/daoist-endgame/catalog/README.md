# Current configuration: database-backed catalogs (2026-10-13)

The production source is PostgreSQL, loaded once before actors start. Migration 59 seeds native item/card definitions; migration 60 seeds normalized versioned unique/formation rows, rune metadata and finite skill-variant parameters. Migrations 61–63 supply cube, skills and loot/rift rules. No Go embed or process-global installer supplies production catalog data.

- Edit/review configuration through versioned SQL migrations and the queryable `game_endgame_*` tables/views. Preserve historical version 0/5 definitions and saved instance IDs/rolls. A restart loads one immutable snapshot. Missing rows, references, child order or manifest entries prevent startup.
- `compile_database_catalog.py` reproduces the initial migration 60 from its frozen, pre-migration golden export under `server/internal/testfixture/catalog`. That export exists only for regression comparison and reproducing this historical migration; changing it alone never changes a deployed game. Subsequent changes belong in a new SQL migration, not in the frozen export or an old initialization migration.
- `database_catalog.py`, `audit_icons.py` and the current `../showcase/generate_showcase.py` read normalized migration 59/60 seed SQL for offline documentation. They do not claim that a deployed database has no later/operator changes. Actual `cmd/showcase-grant` dry-run and execution load the live database and bind the reviewed plan digest.
- The old standalone `generate.py`, `compile_v5.py`, `compile_v6.py`, `compile_astral.py` and V5 showcase generator are retired and fail clearly. Their pure historical validation functions remain importable by archival tests. They cannot overwrite production-looking Go or JSON catalogs.
- `server/internal/domain/unique_catalog_*.json`, `legacy_unique_v4/`, `legacy_unique_v5/`, authoring files and old showcase artifacts are frozen provenance only. Nothing in the production binary reads them.

Verification:

```sh
python3 -B docs/daoist-endgame/catalog/audit_icons.py --check
python3 -B docs/daoist-endgame/showcase/generate_showcase.py --check
python3 -B server/tests/showcase_manifest_test.py
cd server && go test ./internal/domain ./internal/data ./internal/store ./internal/showcase ./internal/arch
```

Actual PostgreSQL validation requires the documented isolated test cluster. `tests/sql/validate_endgame_catalog.py` verifies fresh schema, repeat application and preservation of configured values. `TestEndgameCatalogActualReadOnlyPostgres` exercises the real 59–63 loader sequence and compares every 0/5/6 definition against the pre-migration golden export.

## Historical authoring and balance evidence

The sections below describe earlier catalog stages and are not current editing or deployment instructions.

# 仙遗装备：独立撰写源与冻结编译

## V5 历史规则（2026-10-12，已由V6接替）

新产物目录已收敛为确认的259件，价值档183/54/18/4；完整执行结果和边界见 [V5_CATALOG.md](V5_CATALOG.md)。`v5/assignment.json` 保留逐件设计与来源；`v5/authored_stats.json` 是经过实际 Compute 校准后冻结的明确数值，不会在游戏运行时按ID生成。`generate.py` 校验1619条V4冻结来源；`compile_v5.py` 校验259条V5冻结产物。

现有版本0实例从 `legacy_unique_v4` 和冻结宿阵目录读取；V5阶段新唤醒或新装备实例曾写版本5（现为版本6）。旧目录、旧变量roll、普通装备和历史物权都保留，不把缺少版本字段的旧实例改造成新数值。下文1619范围和 ReservedSkills 仅描述历史V3/V4来源，不能覆盖本节。


## 边界

原始装备身份来自初始化 SQL；名称、故事、定位和数值来自 `authored/<职业>.json` 的逐件显式记录。编译器不再使用词根×行动×部位排列，不再按哈希挑选数值，不再为让属性组合“唯一”而给末尾属性加一。不同装备可以诚实地共享机械属性；字符串或属性包不重复不能证明文案质量与平衡。

```sh
python3 docs/daoist-endgame/catalog/generate.py --validate-authored
python3 docs/daoist-endgame/catalog/generate.py
python3 docs/daoist-endgame/catalog/generate.py --check
python3 docs/daoist-endgame/catalog/compile_astral.py --check
```

第一条只审查已提交的撰写记录，输出已写/待写数量，不替换运行目录。历史源检查必须覆盖全部 1,619 个基础 ID；缺少一件也失败，不以旧文案、自动词组或重复背景补齐。V4已完整编译1,619条独立记录，V5另编译259条，输出和源记录通过字节一致性检查；不能只据文件存在宣称改写完成。

## 写作记录

每件必填 `BaseID, Name, Lore, LoreRoot, Identity, Benefit, Tradeoff, PowerTier, Affixes, Traits`；`ReservedSkills`、`VariableAffixes`、`LocalEnhancements` 可省略。装备的门槛、性别、职业、图标与 ID 不由作者填写。`writing_context/*.jsonl` 提供原名、原始描述、等级、槽位、来源和旧随机词条资格。

- Name 是具体物件的身份，不是给原名换同一套前后缀。
- Lore 写一段独立事件、谜团、执念或物件来历；原作事实与新作故事分开。不得把许栖岩和山石先生擅自断言为同一人。
- Identity 说明这件物品为何存在；Benefit/Tradeoff 说明它适合谁、为何不通吃。减少慈善小故事、训诫结尾或同一叙事句法的连续复用。
- PowerTier 为 transition/highvalue/builddefining/godlike：过渡升级、高价值装备、少量流派核心、极少数神装。v3 共 1,080/449/84/6 件；旧 niche/specialist/chase 仅属于历史版本。过渡装也必须在其职业与用途上胜过可比较黄装，不能以偏门为理由保留低强度。神装指目录占比，不改变既有掉落概率或升真配方。
- 每件仙遗和每个宿阵至少 5 条有益、实际生效且 Attr 不重复的额外属性，允许更多；仙遗现为 5–8 条有益属性，另可有明确诅咒。最低浮动档仍须保留至少 5 条。基础面板、星篆、条件特性、诅咒和未启用技能等级不计入五条；同一 Attr 的绝对/百分比两行不能凑数，29/31/47 的百分比同桶也不能重复。仍允许 0–3 条特性，特性不是必填装饰。
- 词条数、去重、来源和模式校验属于结构检查；需要逐件编辑审读和实际角色比较，不能当作文学质量或平衡达标证书。

## 属性与技能

Attr/Mode 必须被 `SupportedEndgameAffix` 支持。Mode0 为绝对值，Mode1 为百分比；23/66 暴击本身是基点，不支持 Mode1。13 为闪避公式中的防御，17 是魔法防御，69/70 是平减，不能写成百分比减伤；新目录 69/70/118 仅允许绝对模式，避免对零基础抗性做无效百分比。25仅支持Mode1，存储负值对应玩家正向攻击速度百分点，实际间隔按速度反算，玩家普攻调度再按 100ms 向下取整；叠加后的档位必须另测，不能把面板加速直接当成等额 DPS。40负重只有面板字段而无服务器负重约束，不允许作为新生效属性。普通装备38恢复仍不属于仙遗/宿阵新词条的可用范围；轸星篆的旧实例38另由专用恢复路径执行。

`skill_scopes.json` 记录可执行的原技能 ID/中文名/职业与 SQL 来源。特性可加 `SkillID`，可选 `SkillName` 必须匹配原名；描述用服务端权威映射。命中、施法和治疗都按当前技能过滤，再做同名特性取最高值。`skill_economy` 支持物理技能节蓝。不能给无技能上下文的平砍、击杀恢复、受击护体通道硬加 SkillID。

未来技能等级仅保留 `ReservedSkills` 定义：Scope(all/class/tree/skill)、Target、Levels。预定硬点上限20，装备软等级可超过20，先决条件与协同看硬点。当前不改技能树、学习等级、耗点、技能表、战斗数值或玩家 tooltip；未启用的+技能不能计入装备强度。

## 覆盖与兼容

`eligible.csv` 是原始恢复基线，共1,619行：战士249、剑客290、刺客269、药师237、术士237、双法系12、五职业共用323、不限职业2。885有原类型掉落分类，734仅转换入口覆盖，298可使用旧随机词条池。所有源 SQL 行重新枚举并逐列核对，不执行数据库迁移或修改原掉落表。

仙遗仍是原基础 ID 上的衍生形态，UniqueID=BaseID；实例 UID、已有词条、孔位、卡 UID/效果快照、精炼、洗炼与融合不变。已有80%–120%灵性原值保留；v4只把原值映射到11个共享品质档，并作用于明确VariableAffixes/LocalEnhancements中的至多两条浮动行。其余属性、诅咒和特性固定，最高档约1/11。这是V4历史行为。V5改为按实例内容版本选表，现有V4实例不会因当前目录改为259条而改变派生数值或丢失。

必须分别比较“新落地无黄词条的仙遗”和“同一黄色装备保留词条后升真”。v3 以原池中至多五条（自带一条时至多四条）相容、对指定用途最强的自然词条为参考，80% 灵性也必须达到显式收益下限；P90 仅保留为历史对照，不是整件黄装分位数。298 之外没有旧池，采用声明过的实际原池锚点进行暂定外推，不能称作已存在的同基础同等级黄装。见 `../balance/V3_POWER_BUDGETS.md` 的 7,902 组运行时比较、治疗续航口径与验证边界。

28张星篆的原身份、门槛及卡片词条由 `rune_identity_baseline.json` 冻结核对。编译器和Go均验证星篆词条；仅第28张允许正值、绝对模式的38，经专用快照恢复路径生效。改变既有星篆定义必须另作版本/迁移设计，不能夹带进宿阵改写。`python3 -m unittest discover -s docs/daoist-endgame/catalog -p "test_*.py"` 检查坏模式、伪技能及身份改写被拒绝。

本件强化、持久roll、旧客户端兼容与升级/回退见[ITEM_LOCAL_ENHANCEMENT.md](../ITEM_LOCAL_ENHANCEMENT.md)。
