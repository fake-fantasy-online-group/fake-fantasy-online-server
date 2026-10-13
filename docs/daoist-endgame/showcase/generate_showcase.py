#!/usr/bin/env python3
"""Compile the fixed 27-item display plan from migration59/60 SQL seeds. Never grants items.

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
# Reviewed V6 routes; historical V5 selection remains under v5/.
SELECTIONS = [(42,
  '春霞姐姐',
  'assassin',
  2,
  '精英猎杀／神暗金短刺',
  [([2707], 38, '面饰辅阵提供命中、物攻、移动与MF，保留高价值输出/自救槽。'),
   ([2049], 0, '普攻特性与攻速支持持续猎杀。'),
   ([2509], 0, '生命与低血自疗触发提供近战容错，触发有概率和内置冷却。'),
   ([2410], 0, '物攻、攻速与力量支持短刺；保留生命负值，不故意残血。'),
   ([2590], 0, '强会心特性与攻速；同名会心只取较强值。'),
   ([2895, 2896], 36, '守阵生命、双平减与低血防护；按实际性别选白底。'),
   ([2108], 0, '高血防护、生命、魔防与异常抗性。'),
   ([2604], 0, '命中、物攻、力量和会心同向。'),
   ([1810], 0, 'V6神暗金高局部白字、精英特性与原生10219软等级；普通敌人不享受精英倍率。')]),
 (183,
  '术士姐姐',
  'warlock',
  4,
  '火弹专精／神暗金法杖',
  [([2707], 28, '辅阵补法攻、蓝量与施法节蓝，支持持续火弹。'),
   ([2059], 0, '术士已学技能+1，法攻与智慧同向。'),
   ([2506], 0, '法术会心、法攻与智慧；圣系固定触发单独结算，不把圣伤算成火伤。'),
   ([2409], 0, '全部已学技能+1，精神与蓝量。'),
   ([2558], 0, '火伤和火抗穿透匹配神杖火弹，替换旧冰戒。'),
   ([2829, 2830], 36, '守阵支撑法师生存；女袍原生火弹等级如存在由生产技能路径解析，不作为男女共同收益。'),
   ([2108], 0, '高血防护、生命、魔防与异常抗性。'),
   ([2602], 0, '法攻、智慧和法术会心替换旧冰背；暗吸收只对应暗伤。'),
   ([1310], 0, 'V6神暗金局部法攻、火弹软等级、火伤/火穿与施法节奏；实耗蓝，不与1306瞬火形态混用。')]),
 (206,
  '骚骚霞',
  'healer',
  3,
  '护持治疗／救急团队支援',
  [([2707], 35, '辅阵移到低白字面饰：治疗技能、节蓝、低血救急与团队圣抗；替换被同名强救急覆盖的2705。'),
   ([2056], 0, '治疗系+2与V6护持治疗形态，治疗/护持细则由生产技能处理。'),
   ([2509], 0, '生命与条件自疗提高治疗者站场。'),
   ([2409], 0, '全部已学技能+1，精神与蓝量。'),
   ([2559], 0, '治愈术+2、精神与蓝量。'),
   ([2899, 2900], 36, '守阵生命、双平减与低血防护；按实际性别选白底。'),
   ([2108], 0, '从辅阵白底鞋改暗金，恢复高血防护与生命，避免重复弱救急特性。'),
   ([2601], 0, '精神24支持治疗，比旧2603精神8更贴合此路线；MF不算治疗倍率。'),
   ([1310], 40, '全已学技能+1、精神、生命和蓝量；不用火弹神暗金冒充治疗神装。')])]
POLICY = {
    "execution_status": "prepared_v6_pending_review_no_grant",
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
        "domain.ApplyUnique(&stack,def,func(n int) int { return n-1 })，确认成功、UniqueID=BaseID、UniqueContentVersion=6、UniqueRollBP=12000。",
        "不伪造Affixes；目录属性由实际ItemDef和生产域函数解析。",
    ],
    "formation": [
        "从真实白底ItemDef调用domain.NewStack(def,1)；确认AstralBaseEligible与EquipmentSocketCapacity。",
        "新展示件固定FormationContentVersion=6、FormationRollBP=12000，UniqueID=0。",
        "精确孔数和顺序；每枚星篆用真实Rune ItemDef调用NewStack，保留其Card快照、独立UID及正确槽序。",
        "MatchAstralRecipe必须匹配指定ID；ActiveAstralRecipes模拟中每频道至多一个所选宿阵；神暗金武器路线无战阵。",
    ],
    "persistence": "复用本地已存在的隔离展示发放工具及服务端物品持久化路径；不用修改业务代码，不手编SQL绕过实例、归属、插槽不变量。",
    "preflight": [
        "本计划仅准备；完成V6目录冻结、隔离PG回归及实际环境预检后，由主任务处理后续发放。",
        "确认目标仍为test001及角色42/183/206，重读名称、职业、60级、实际性别及装备背包空格。",
        "正式环境及实际加载目录复核；若不符，停下对应发放，不改其他账号或角色。",
        "复用已有安全在线同步或离线维护流程及原子保存，避免旧会话覆盖新增库存。",
        "保存前后快照及此次27装备UID、所有镶嵌星篆UID；完成后重读归属、版本、roll、属性、孔序、唯一性和位置。",
    ],
}


def load_sources():
    # This documentation generator reads committed normalized SQL seeds. Actual
    # grants load the deployed database through cmd/showcase-grant, never this file.
    import sys
    sys.path.insert(0, str(ROOT / "docs/daoist-endgame/catalog"))
    from database_catalog import MIGRATION, configured_definitions, table_rows
    definitions = configured_definitions()
    uniques = {key[2]: row for key, row in definitions.items() if key[:2] == ('unique', 6)}
    def lower(value):
        if isinstance(value, list):
            return [lower(v) for v in value]
        if isinstance(value, dict):
            return {('id' if k == 'ID' else k[0].lower() + k[1:]): lower(v) for k, v in value.items()}
        return value
    formations = {key[2]: lower(row) for key, row in definitions.items() if key[:2] == ('formation', 6)}
    item_path = ROOT / "server/migrations/000000_init-59.sql"
    items = {r['id']: r for r in json.loads(item_path.read_text().split('$endgame_items$')[1])}
    rules = table_rows('game_endgame_rune_rules', ['item_id', 'mansion', 'quadrant', 'regeneration_interval_ms'])
    runes = {r['item_id']: {'id': r['item_id'], 'mansion': r['mansion'], 'quadrant': r['quadrant'],
              'name': items[r['item_id']]['name'], 'minLevel': items[r['item_id']]['level'],
              'affixes': items[r['item_id']]['effects']} for r in rules}
    for row in formations.values():
        row['channel'] = {1: '战阵', 2: '守阵', 3: '辅阵'}[row['channel']]
        row['order'] = [runes[r]['mansion'] for r in row['runes']]
    bases_path = ROOT / "docs/daoist-endgame/balance/native_equipment_budget_references.json"
    bases = {r["base_id"]: r for r in json.loads(bases_path.read_text())["bases"]}
    if len(uniques) != 259 or len(formations) != 40 or len(runes) != 28:
        raise ValueError("Catalog coverage changed; review the fixed showcase selection.")
    paths = [MIGRATION, item_path, bases_path]
    hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}
    return uniques, formations, runes, bases, hashes


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
                "content_version": 6, "socket_count": len(formations[fid]["runes"]) if fid else 0, "sockets": [],
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
        channels = [i["channel"] for i in items if i["kind"] == "formation"]
        assert len(channels) == len(set(channels)) and {"守阵", "辅阵"}.issubset(channels)
        loadouts.append({"character_id": role_id, "character_name": name, "class": cls, "race_zero_based": race,
            "profession_one_based": race + 1, "expected_level": 60,
            "gender": "resolve locally; never infer from name", "build": build, "count": 9, "items": items})
    return {
        "schema_version": 1, "account": "test001", "source_baseline_commit": SOURCE_BASELINE,
        "source_catalogs": {"unique_catalog_version": "endgame-v6-scope259", "unique_count": 259, "formation_count": 40, "rune_count": 28, "content_version": 6},
        "intent": "各新增9件背包装备供看属性，保留旧物品；不自动穿戴，不改角色等级、技能或六维。",
        "status": "prepared_v6_pending_review_no_grant", "grant_count": 27,
        "embedded_rune_instance_count": sum(i["socket_count"] for s in loadouts for i in s["items"]), "total_new_item_instances_if_executed": 27 + sum(i["socket_count"] for s in loadouts for i in s["items"]),
        "slots_per_character": sorted(SLOTS),
        "excluded_slots": {"4": "与双手武器冲突", "5": "与双手武器冲突", "11": "259目录无对应法宝，不能虚构", "14-25": "不扩大到目录外飞升槽"},
        "policy": POLICY, "factory_contract": FACTORY, "source_sha256": hashes, "loadouts": loadouts,
        "validation": {"rerun_test": "TestV6ShowcaseThreeCatalogLoadouts", "python_check": "python3 docs/daoist-endgame/showcase/generate_showcase.py --check",
            "historical_verification": "旧V5证据保存在v5/；当前V6必须单独重跑。",
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
    lines = ["# test001 V6 三角色毕业装备计划", "", "仅准备并离线验证，未实际发放。此文档读迁移59/60规范SQL；实际发放工具会重新读取部署数据库。各9件共27件放背包；保留旧物品、等级60、技能和六维，不自动穿戴。", "",
        f"当前目录259暗金／40宿阵／28星篆，版本6、合法最高roll12000；共{m['embedded_rune_instance_count']}枚独立UID/Card星篆，{m['total_new_item_instances_if_executed']}个新实例。", "",
        "此处毕业指所选玩法的高档协同路线，不声称所有敌人、加点与装备组合的全局最优；MF、防护、技能形态不折算成虚构DPS。", "",
        "## 文件与验证", "- grant-plan.json：可审阅选号；manifest.json：全量目录、白字、门槛、孔序和哈希。",
        "- V6-SELECTION.md：相对V5的构筑变更、互斥和取舍。v5/保留旧源码/产物及复現说明。",
        "- python3 -B docs/daoist-endgame/showcase/generate_showcase.py --check",
        "- python3 -B server/tests/showcase_manifest_test.py",
        "- cd server && go test ./internal/showcase ./internal/domain -run 'Test.*Showcase|TestBuild|TestFixedSelection|TestRejects|TestDigest' -count=1",
        "- SHOWCASE_V6_VALIDATION_REPORT指定报告位置后运行TestV6ShowcaseThreeCatalogLoadouts。合成面板不是test001现况或实战倍率。", ""]
    for s in m["loadouts"]:
        lines += [f"## {s['character_name']}（{s['character_id']}） {s['build']}", "", "| 槽 | 装备/底材 | 选择理由 |", "|---|---|---|"]
        for i in s["items"]:
            ids = " / ".join(str(b["base_id"]) for b in i["candidates"])
            lines.append(f"| {i['slot_name']} | {i['display_name']}（{ids}） | {i['rationale']} |")
        lines += ["", "孔序："]
        for i in s["items"]:
            if i["kind"] == "formation":
                lines.append(f"- {i['slot_name']} 宿阵#{i['formation_id']}：" + " → ".join(str(r["rune_item_id"]) for r in i["sockets"]))
        lines += [""]
    lines += ["## 执行边界", "角色gender=1男/0女，装备sex=1男/2女；不按姓名推断。六维不够照常展示原门槛，不洗点、不强穿。",
        "唯一件由实际ItemDef→NewStack→ApplyUnique构造，0随机附加/0孔/0加工/0融合；宿阵必须真实零附加白底、精确孔序、每孔NewStack生成真实UID/Card。固定属性/诅咒/技能/特性不整体乘1.2。",
        "安全工具默认dry-run、expected digest绑定、三角色原子、幂等、离线维护及其他连接拒绝不变。真实执行前重读test001/三角色身份、性别、实际目录、背包空格；新物品只进空格，不改旧物品。",
        "旧V5结果不代表V6通过；VALIDATION.md及runtime-validation.json记录当前实测范围，隔离PG回归也不等于真实发放。", ""]
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
