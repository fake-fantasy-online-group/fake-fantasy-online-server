#!/usr/bin/env python3
"""Compile the fixed 27-item display plan from current catalogs. Never grants items.

Selection is deliberately explicit. This is not a power-tier selector or balance
solver. --check is read-only; default rewrites the three documented artifacts.
"""
from __future__ import annotations
import argparse
import copy
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
SOURCE_BASELINE = "d9c06ddd53bbb777bc87248b881871ff64d2f947"
SLOTS = {1: "面饰", 2: "头饰", 3: "项链", 6: "手套", 7: "戒指", 8: "衣服", 9: "鞋", 10: "背饰", 13: "双手武器"}
# Tuple: candidate BaseIDs (male, female where needed), formation ID (0=unique), rationale.
# This is the exact selection previously validated before the workspace reset.
SELECTIONS = [
    (42, "春霞姐姐", "assassin", 2, "先手会心／普攻输出", [
        ([2707], 38, "低白字面饰承载辅阵，保留项链救命触发与鞋的防护。"),
        ([2049], 0, "普攻增伤18%，配合会心与攻速；不是简单选择最高等级的防御头巾。"),
        ([2509], 0, "523生命与低血自疗触发，补足武器和手套的生命代价。"),
        ([2410], 0, "物攻与7%普攻间隔缩短；低血强攻只计条件收益。2408的会心16%会被2590较强同名18%覆盖，故不选2408。"),
        ([2590], 0, "会心特性18%、会心底盘与攻速7%，本件戒指白字物攻强化最高80%。"),
        ([2895, 2896], 36, "守阵增加生命、防御、双平减与低血保护；选真实性别匹配的白底。"),
        ([2108], 0, "高血线防护12%，法防、生命和异常抗性补足近战容错。"),
        ([2604], 0, "背饰选择命中、物攻、力量和物理会心同向的路线。"),
        ([1810], 33, "5孔短刺战阵：物攻117%、本件白字物攻最高180%、攻速32%、初锋26%；承担生命-12%。"),
    ]),
    (183, "术士姐姐", "warlock", 4, "冰系／暴雪范围施法", [
        ([2707], 28, "4孔辅阵提供45法攻、施法节蓝20%和移速，保留冰戒及技能手套。"),
        ([2059], 0, "术士职业技能+1，法攻与智慧服务施法。"),
        ([2506], 0, "法术会心450bp、法攻与智慧，不将物理或火系属性算作冰系收益。"),
        ([2409], 0, "全部已学技能+1，并带法力、精神。"),
        ([2560], 0, "冰伤10%、敌冰抗穿透6个百分点，匹配暴雪与冰箭。"),
        ([2829, 2830], 36, "保留底材57法攻，守阵增加物理安全边际；不将女袍的火弹+2算作冰系收益。"),
        ([2108], 0, "高血线防护12%，法防、生命、异常抗性补足战阵生命短板。"),
        ([2610], 0, "冰系技能+1、冰伤8%；低等级仍是此构筑的有效核心。"),
        ([1310], 34, "5孔法杖战阵：法攻65%、本件白字法攻最高220%、暴雪范围22%及范围伤害12%，生命-10%。"),
    ]),
    (206, "骚骚霞", "healer", 3, "救急／长战治疗支援", [
        ([2705], 0, "目标生命不高于25%时治疗提高19%，与等级和节蓝独立。"),
        ([2056], 0, "治疗系技能+2，不把攻击系当作治疗。"),
        ([2509], 0, "523生命、自救触发和续航，保证治疗者存活。"),
        ([2409], 0, "全部已学技能+1，覆盖治疗及辅助，保留精神与法力。"),
        ([2559], 0, "治愈术10301+2，精神与法力符合治疗路线。"),
        ([2899, 2900], 36, "守阵的生命、防御与双平减提高站场能力；按角色原性别选底材。"),
        ([2108], 35, "5孔辅阵：治疗系+1、治疗节蓝22%、精神26%与法力36%；鞋位保留救急面饰和技能戒指。"),
        ([2603], 0, "圣羽护佑：小队圣抗8个百分点，240像素半径，最多6名玩家并覆盖活动宠物；不是全抗。"),
        ([1310], 40, "5孔战阵：全部已学技能+1、生命320、法力300、精神26；相较纯法攻神杖更贴合支援，且无生命负值。"),
    ]),
]
POLICY = {
    "execution_status": "deferred_by_user_pending_five_class_original_skill_restoration",
    "max_catalog_roll_bp": 12000,
    "max_roll_scope": "仅显式可变词条与局部强化取合法最高档；固定词条、诅咒、技能、触发和光环原值保留，不能整体乘1.2。",
    "destination": "backpack_only", "auto_equip": False,
    "preserve_existing_items": True,
    "change_character_level_skills_stats_or_free_points": False,
    "unique_natural_affixes": "新建0普通随机词条的目录展示件，不叠满黄装附加或目录外卡片。",
    "unique_socket_policy": "0孔；同一唯一装备不能再结宿阵。",
    "formation_socket_policy": "天生零附加、无套装、属性已确认白底；精确4/5孔及配方顺序，各孔真实新UID星篆和固定Card快照，无多余空孔。",
    "new_item_upgrades": "加工0、融合0；不触碰旧件加工、融合、孔位、绑定、UID或位置。",
    "wearability": "职业、等级、性别必须匹配；六维门槛原样展示，可因现有加点未满足，不因此降级替换、不洗点、不强穿。",
    "power_claim": "按所选玩法的强力目录组合，不宣称全敌人、全加点、黄装词条或加工条件下的数学全局最优。",
    "inventory": "各9件、共27件；每条衣服仅选实际性别的一件。重新查空格，不能覆盖、删除或换掉旧物品。",
}
FACTORY = {
    "unique": [
        "加载实际已部署ItemDef，核对职业、等级、性别及目录版本。",
        "domain.NewStack(def,1)生成新装备UID和满耐久，不复制现有UID。",
        "domain.ApplyUnique(&stack,def,func(n int) int { return n-1 })，确认成功、UniqueID=BaseID、UniqueContentVersion=5、UniqueRollBP=12000。",
        "不伪造Affixes；目录属性由实际ItemDef和生产域函数解析。",
    ],
    "formation": [
        "从真实白底ItemDef调用domain.NewStack(def,1)；确认AstralBaseEligible与EquipmentSocketCapacity。",
        "新展示件固定FormationContentVersion=5、FormationRollBP=12000，UniqueID=0。",
        "精确孔数和顺序；每枚星篆用真实Rune ItemDef调用NewStack，保留其Card快照、独立UID及正确槽序。",
        "MatchAstralRecipe必须匹配指定ID；ActiveAstralRecipes模拟中每频道只有一个所选宿阵。",
    ],
    "persistence": "复用本地已存在的隔离展示发放工具及服务端物品持久化路径；不用修改业务代码，不手编SQL绕过实例、归属、插槽不变量。",
    "preflight": [
        "本计划暂缓执行；待用户五职业原版技能复原工作允许发放后再继续。",
        "确认目标仍为test001及角色42/183/206，重读名称、职业、60级、实际性别及装备背包空格。",
        "正式环境及实际加载目录复核；若不符，停下对应发放，不改其他账号或角色。",
        "复用已有安全在线同步或离线维护流程及原子保存，避免旧会话覆盖新增库存。",
        "保存前后快照及此次27装备UID、所有镶嵌星篆UID；完成后重读归属、版本、roll、属性、孔序、唯一性和位置。",
    ],
}


def load_sources():
    paths = sorted((ROOT / "server/internal/domain").glob("unique_catalog_*.json"))
    uniques = {r["ID"]: r for p in paths for r in json.loads(p.read_text())}
    astral_path = ROOT / "server/docs/endgame/astral_runes.json"
    astral = json.loads(astral_path.read_text())
    bases_path = ROOT / "docs/daoist-endgame/balance/native_equipment_budget_references.json"
    bases = {r["base_id"]: r for r in json.loads(bases_path.read_text())["bases"]}
    paths += [astral_path, bases_path, ROOT / "server/internal/domain/endgame_runes_catalog.go"]
    if len(uniques) != 259 or len(astral["formations"]) != 40 or len(astral["runes"]) != 28:
        raise ValueError("Catalog coverage changed; review the fixed showcase selection.")
    hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}
    return uniques, {r["id"]: r for r in astral["formations"]}, {r["id"]: r for r in astral["runes"]}, bases, hashes


def build_manifest():
    uniques, formations, runes, bases, hashes = load_sources()
    loadouts = []
    for role_id, name, cls, race, build, selection in SELECTIONS:
        items = []
        for ids, fid, rationale in selection:
            first = bases[ids[0]]
            slot = first["slot"]
            candidates = []
            for base_id in ids:
                b = bases[base_id]
                assert b["slot"] == slot and race in b["professions"] and b["wearer_level"] <= 60
                assert base_id in uniques, "Every chosen base must remain in the approved 259-ID catalog"
                candidates.append({
                    "base_id": base_id, "base_name": b["base_name"],
                    "equipment_sex_restriction": b["sex"],
                    "character_gender_when_selected": 1 if b["sex"] == 1 else 0 if b["sex"] == 2 else None,
                    "base_required_level": b["wearer_level"], "requirements": b["requirements"],
                    "intrinsic_white_stats": b["intrinsic"], "resource_tier": b["resource_tier"], "equipment_type": b["type"],
                    "natural_affix_count": b["natural_affix_count"], "fixed_affixes": b["fixed_affixes"],
                    "unknown_innate_properties": b["unknown_innate_properties"], "suit_id": b["suit_id"],
                    "source_arm": b["source_arm"], "source_desc": b["source_desc"],
                })
            item = {
                "slot": slot, "slot_name": SLOTS[slot], "quantity": 1,
                "kind": "formation" if fid else "unique", "candidates": candidates,
                "rationale": rationale, "destination": "backpack", "auto_equip": False,
                "rolled_affix_count": 0, "rolled_affixes": [], "refine_level": 0,
                "fused_appearance": 0, "fused_soul": 0, "fused_dragon": 0,
                "content_version": 5, "socket_count": len(formations[fid]["runes"]) if fid else 0, "sockets": [],
            }
            if fid:
                d = formations[fid]
                assert slot in d["slots"] and (not d["professions"] or d["professions"] & (1 << race))
                for b in candidates:
                    assert b["natural_affix_count"] == 0 and not b["fixed_affixes"]
                    assert not b["unknown_innate_properties"] and b["suit_id"] == 0
                item.update({
                    "display_name": d["name"] + " · " + first["base_name"].replace("（男）", "").replace("（女）", ""),
                    "formation_id": fid, "unique_id": 0, "channel": d["channel"], "formation_roll_bp": 12000,
                    "required_level": max(first["wearer_level"], d["minLevel"]), "catalog_definition": copy.deepcopy(d),
                })
                for idx, rune_id in enumerate(d["runes"]):
                    r = runes[rune_id]
                    item["sockets"].append({"index": idx, "rune_item_id": rune_id, "rune_name": r["name"],
                        "minimum_level": r["minLevel"], "new_distinct_uid_required": True,
                        "card_snapshot_source": "domain.NewStack(actual rune ItemDef,1).Card", "fixed_card_affixes": r["affixes"]})
                affixes = [{"Attr": a["attr"], "Value": a["value"], "Mode": a["mode"]} for a in d["affixes"]]
                variables = [{"Attr": a["attr"], "Min": a["min"], "Max": a["max"]} for a in d.get("variableAffixes", [])]
                local = [{"Attr": a["attr"], "Percent": a["percent"], "MaxPercent": a.get("maxPercent", 0)} for a in d.get("localEnhancements", [])]
            else:
                assert len(ids) == 1
                d = uniques[ids[0]]
                item.update({"display_name": d["Name"], "unique_id": d["ID"], "formation_id": 0,
                    "unique_roll_bp": 12000, "required_level": d["LevelRequirement"], "catalog_definition": copy.deepcopy(d)})
                affixes, variables, local = copy.deepcopy(d["Affixes"]), d["VariableAffixes"], d["LocalEnhancements"]
            assert item["required_level"] <= 60
            for a in affixes:
                for v in variables:
                    if v["Attr"] == a["Attr"]:
                        a["Value"] = v["Min"] if a["Attr"] == 25 else v["Max"]
            item["max_roll_authored_affixes"] = affixes
            item["max_roll_local_enhancement_percent"] = [{"attr": a["Attr"], "percent": a["MaxPercent"] or a["Percent"],
                "scope": "this item's unrefined intrinsic white stats only"} for a in local]
            items.append(item)
        assert sorted(i["slot"] for i in items) == sorted(SLOTS)
        assert {i["channel"] for i in items if i["kind"] == "formation"} == {"战阵", "守阵", "辅阵"}
        loadouts.append({"character_id": role_id, "character_name": name, "class": cls, "race_zero_based": race,
            "profession_one_based": race + 1, "expected_level": 60,
            "gender": "resolve locally; never infer from name", "build": build, "count": 9, "items": items})
    return {
        "schema_version": 1, "account": "test001", "source_baseline_commit": SOURCE_BASELINE,
        "source_catalogs": {"unique_catalog_version": "endgame-v5-scope259", "unique_count": 259, "formation_count": 40, "rune_count": 28, "content_version": 5},
        "intent": "各新增9件背包装备供看属性，保留旧物品；不自动穿戴，不改角色等级、技能或六维。",
        "status": "deferred_by_user_pending_five_class_original_skill_restoration", "grant_count": 27,
        "embedded_rune_instance_count": 44, "total_new_item_instances_if_executed": 71,
        "slots_per_character": sorted(SLOTS),
        "excluded_slots": {"4": "与双手武器冲突", "5": "与双手武器冲突", "11": "259目录无对应法宝，不能虚构", "14-25": "不扩大到目录外飞升槽"},
        "policy": POLICY, "factory_contract": FACTORY, "source_sha256": hashes, "loadouts": loadouts,
        "validation": {"rerun_test": "TestShowcaseThreeCatalogLoadouts", "python_check": "python3 docs/daoist-endgame/showcase/generate_showcase.py --check",
            "historical_verification": "2026-10-12原工作区在d9c06dd生产domain通过6套/54件检查；该历史结果不是本次重跑的替代。",
            "actual_character_wearability": "not read; do not change character allocation"},
    }


def compact_plan(m):
    out = {k: m[k] for k in ["schema_version", "account", "source_baseline_commit", "source_catalogs", "status", "intent", "grant_count", "embedded_rune_instance_count", "total_new_item_instances_if_executed", "policy", "factory_contract"]}
    out["loadouts"] = []
    for s in m["loadouts"]:
        row = {k: s[k] for k in ["character_id", "character_name", "class", "race_zero_based", "profession_one_based", "expected_level", "gender", "build", "count"]}
        row["items"] = []
        for i in s["items"]:
            item = {k: i[k] for k in ["slot", "slot_name", "kind", "display_name", "quantity", "required_level", "content_version", "unique_id", "formation_id", "socket_count"]}
            item["candidates"] = [{k: b[k] for k in ["base_id", "base_name", "equipment_sex_restriction", "character_gender_when_selected", "requirements"]} for b in i["candidates"]]
            for k in ["unique_roll_bp", "formation_roll_bp", "channel"]:
                if k in i:
                    item[k] = i[k]
            item["ordered_rune_ids"] = [r["rune_item_id"] for r in i["sockets"]]
            row["items"].append(item)
        out["loadouts"].append(row)
    return out


def markdown(m):
    lines = ["# test001 三角色装备展示计划", "", "## 状态", "装备发放已延后，等待五职业原版技能复原工作。此目录只保存方案和只读验证工具，不执行数据库操作。", "",
        "## 范围", "- 固定27件，每角色9件，仅放背包看属性；不自动穿戴，不改等级、技能、六维或自由点。",
        "- 保留所有旧件及其UID、位置、加工、孔位、绑定；每条衣服只选真实性别匹配的一件。",
        "- 当前259唯一／40宿阵目录；最高合法roll12000、内容版本5。固定属性、诅咒、技能、光环、触发不整体乘1.2。",
        "- 新唯一件0普通随机词条／0孔／加工0／融合0；宿阵用零天然附加白底，精确孔数和顺序，真实独立UID星篆与Card快照。",
        "- 六维门槛可因当前加点而未满足，照常展示原门槛；不因看属性任务改点或换成低阶件。",
        "- 九槽为面饰、头、项链、手套、戒指、衣服、鞋、背饰、双手武器。无盾、单手、目录外法宝或飞升装备。",
        "- 按玩法选强配置，不宣称所有敌人、加点和加工／黄装附加组合下全局最优。", "",
        "## 文件与重跑", "- grant-plan.json：简明执行计划；manifest.json：完整源属性、白字、技能、特性、孔序、门槛与出处。",
        "- generate_showcase.py：固定选号的确定性生成器；不按PowerTier重新选择，不调用数据库。",
        "- 从仓库根运行：python3 docs/daoist-endgame/showcase/generate_showcase.py --check",
        "- Python契约测试：python3 -B server/tests/showcase_manifest_test.py",
        "- runtime-validation.json是重建后生成的离线报告；VALIDATION.md记录此次验证结果与边界。",
        "- 有Go工具链时：cd server && go test ./internal/domain -run '^TestShowcaseThreeCatalogLoadouts$' -count=1 -v",
        "- 可选报告：设置SHOWCASE_VALIDATION_REPORT为期望输出JSON路径再运行同一Go测试。报告中的面板只属于合成合法测试角色，不能当作test001实况或实战DPS/HPS。", "",
        "## 性别", "角色gender=1男／0女；装备sex=1男／2女／0不限。不可按角色名推断。旧空格快照不作写入依据，未来执行必须重读。", ""]
    for s in m["loadouts"]:
        lines += [f"## {s['character_name']}（ID {s['character_id']}）：{s['build']}", "", "| 槽 | 装备与底材 | 类型 | 选择理由 |", "|---|---|---|---|"]
        for i in s["items"]:
            ids = " / ".join(f"{b['base_id']} {b['base_name']}" for b in i["candidates"])
            typ = f"唯一 #{i['unique_id']}" if i["kind"] == "unique" else f"{i['channel']} #{i['formation_id']}，{i['socket_count']}孔"
            lines.append(f"| {i['slot_name']}({i['slot']}) | {i['display_name']}；{ids} | {typ} | {i['rationale']} |")
        if s["class"] == "assassin":
            lines += ["", "普攻18%、会心18%是不同触发条件；初锋26%仅目标生命≥85%，绝境26%仅自己≤25%。同名会心特性只取较强值。攻速／物攻为主路线，没有伪造技能等级。", "代价：战阵生命-12%、手套生命-120；不建议为低血特性故意维持残血。"]
        elif s["class"] == "warlock":
            lines += ["", "冰系已学技能合计+3（职业+1／全技能+1／冰系+1），其他术士已学技能+2；冰伤18%、敌冰抗穿透6个百分点。暴雪范围+22%、范围技能伤害+12%。", "代价：战阵生命-10%。不把火弹等级或火伤作为冰系收益。"]
        else:
            lines += ["", "治愈术合计+7，其他治疗系已学技能+5，非治疗药师已学技能+2；治疗节蓝22%，目标生命≤25%时救急治疗+19%。装备不解锁未学技能，装备等级不计硬点协同。", "圣羽护佑为圣抗+8个百分点、240像素、最多6名玩家并可带活动宠物；不是全抗或直接治疗光环。"]
        lines += ["", "宿阵孔序（从第1孔开始，不可换序）："]
        for i in s["items"]:
            if i["kind"] == "formation":
                lines.append(f"- {i['slot_name']} #{i['formation_id']}：" + " → ".join(f"{r['rune_item_id']} {r['rune_name']}" for r in i["sockets"]))
        lines += [""]
    lines += ["## 后续授权执行时的步骤", "1. 先完成当前五职业复原工作，确认用户仍要执行展示发放，且正式服务端／目录版本正确。",
        "2. 重读test001与三个角色ID、名称、职业、等级、性别和装备背包空格；遇到不符停止对应发放。",
        "3. 唯一件：实际ItemDef → NewStack → ApplyUnique最高合法roll，校验UniqueID、版本5、roll12000。",
        "4. 宿阵：真实白底 → NewStack → 版本5、roll12000、UniqueID0 → 精确孔位、真实NewStack星篆UID/Card → MatchAstralRecipe。",
        "5. 使用现有隔离展示工具与支持的服务端持久化、原子保存、安全在线同步／离线维护路径；不要求修改业务代码，不直接手编SQL绕过不变量。",
        "6. 新物品仅写空格。保存前后快照和27个新装备UID及星篆UID，重读核对归属、版本、roll、名字／属性／白字、孔序和唯一性；旧物品不变。", "",
        "## 验证覆盖与限制", "Go验证器覆盖3职业×2性别共6套／54件：生产NewStack、ApplyUnique、唯一解析、真实原表白底资格、孔容量、配方匹配和错误孔序拒绝、职业／等级／性别、三频道激活及双手冲突。",
        "另以60级自然成长和合法自由点预算构造同时满足门槛的合成测试角色；不读取或修改真实角色状态，不证明实际角色当前可穿。测试不认证全套实战DPS/HPS最优。",
        "2026-10-12原工作区在d9c06dd已通过同方案6套／54件验证。工作区回收后工具与计划重建入仓库；历史通过不替代当前重跑日志。", "",
        "## 来源", f"原验证服务端基线：{SOURCE_BASELINE}。具体目录内容SHA-256记录在manifest.json，当前源变化会令--check失败。", ""]
    return "\n".join(lines)


def outputs():
    m = build_manifest()
    return {"manifest.json": json.dumps(m, ensure_ascii=False, indent=2) + "\n",
        "grant-plan.json": json.dumps(compact_plan(m), ensure_ascii=False, indent=2) + "\n", "README.md": markdown(m)}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--check", action="store_true")
    args = p.parse_args()
    for name, body in outputs().items():
        path = OUT / name
        if args.check:
            if not path.exists() or path.read_text() != body:
                raise SystemExit(f"Stale or missing generated showcase artifact: {path.relative_to(ROOT)}")
        else:
            path.write_text(body)
    print("Verified fixed 27-item showcase plan: 3 classes, 9 slots each, 3 gender branches, 259/40/28 source coverage. No grant executed.")

if __name__ == "__main__":
    main()
