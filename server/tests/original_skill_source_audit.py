#!/usr/bin/env python3
"""Read-only original profession source audit. No database or client assets needed.

The pinned digests are of the earliest tracked server ov_skilldesc profession
rows, not a claim that a particular external client version is the earliest.
Runtime missing-effect repairs may be added without rewriting the raw evidence.
"""
import csv
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
DIGESTS = {
    1: "ecfc12ab0fe5acf912fe2e81d71c89cacc92091a778af54b6417a008465df6d5",
    2: "e7576806956548e36fe430940e39f507b81c8d03b4b4ddcdcb27e1700a98609c",
    3: "cdc2668cb879e6a08509ca2e132a97c8e475367cdd31ba8a283a8bfc27c513b3",
    4: "da628a3e38430e3f05dc07b7451ce19570edbf2be08e83dd888002301ca4a344",
    5: "8ab74d1aa045c51e70228717a5fd1cf7acd194f061d45834414dbd97ef40b3b1",
}
NATIVE_TARGETS = {
    10001: [10002,10003,10004,10005,10006,10007,10009,10013,10015,10017,10021,10022,10023,10027],
    10012: [10009], 10014: [10013], 10019: [10017], 10020: [10006], 10024: [10003],
    10101: [10102,10103,10104,10111,10113,10116,10117,10119,10121,10122,10123,10125,10126],
    10110: [10102], 10112: [10104], 10118: [10117], 10120: [10119],
    10201: [10209], 10207: [10208], 10220: [10210], 10226: [10225],
    10422: [10421], 10423: [10405], 10424: [10401,10402,10405,10408],
    10426: [10401,10402], 10427: [10409,10410,10411,10412], 10429: [10428],
    10430: [10412], 10431: [10409,10410,10411,10412],
}


def original_rows():
    rows, columns, capture = [], [], False
    for line in (ROOT / "server/migrations/000000_init-31.sql").read_text().splitlines():
        if line.startswith("INSERT INTO"):
            capture = '"ov_skilldesc"' in line
            if capture:
                columns = re.findall(r'"([a-z0-9_]+)"', line.split(" VALUES")[0])[2:]
        elif capture and line.lstrip().startswith("("):
            row = line.strip().rstrip(",;")[1:-1]
            row = re.sub(r"decode\('([0-9a-f]+)','hex'\)", lambda m: "'decode:" + m[1] + "'", row)
            values = next(csv.reader([row], quotechar="'", skipinitialspace=True))
            assert len(values) == len(columns), "raw SQL row parsing must not drop columns"
            value = dict(zip(columns, values))
            if int(value["prof"]) in range(1, 6):
                rows.append(value)
    return rows


def audit():
    rows = original_rows()
    assert len(rows) == 750
    for profession, expected in DIGESTS.items():
        selected = [r for r in rows if int(r["prof"]) == profession]
        assert len(selected) == 150
        digest = hashlib.sha256(json.dumps(selected, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        assert digest == expected, f"profession {profession} original raw definitions changed"
    first = {int(r["skill_id"]): r for r in rows if r["skill_level"] == "1"}
    assert len(first) == 150 and 11005 in first
    migration = (ROOT / "server/migrations/000000_init-62.sql").read_text()
    catalog = json.loads(migration.split("$skill_config$")[1])
    assert len(catalog["Skills"]) == 149
    for member in catalog["Skills"]:
        configured = member["Identity"]
        identity = configured["ID"]; raw = first[identity]
        assert configured["Name"] == raw["name"] and configured["Profession"] == int(raw["prof"])
        assert configured["UnlockLevel"] == int(raw["level_need"])
        original_pre = [int(raw[f"depend{i}_id"]) for i in range(1, 5) if int(raw[f"depend{i}_id"])]
        assert (configured["Prerequisites"] or []) == original_pre
    # All identities reuse native rows. One explicit prerequisite exception
    # preserves the current runtime's earlier original-source restoration.
    assert [r["Identity"]["ID"] for r in catalog["Skills"] if not r["UseNativePrerequisites"]] == [10311]
    loaded = json.loads((ROOT / "docs/daoist-endgame/v5/skill-original-ranks.json").read_text())
    assert len(loaded) == 745
    raw_by_key = {(int(r["skill_id"]), int(r["skill_level"])): r for r in rows}
    native_scopes = {}
    for definition in loaded:
        identity, rank = definition["ID"], definition["Level"]
        raw = raw_by_key[identity, rank]
        assert definition["Name"] == raw["name"] and definition["Description"] == raw["desc_"]
        assert definition.get("Kind", 0) == {1: 0, 2: 1, 17: 3, 18: 2}[int(raw["skill_type"])]
        assert definition.get("RequiredWeaponType", 0) == int(raw["arm_type"])
        assert definition.get("TargetEnemy", False) == bool(int(raw["target_mon"]))
        assert definition.get("TargetSelf", False) == bool(int(raw["target_self"]))
        if int(raw["skill_type"]) != 2:
            assert not definition.get("Area", {}), f"single/passive skill {identity}/{rank} gained area geometry"
        if rank == 1:
            targets = set()
            for field in ("PassiveModifiers", "PassiveTriggers", "PassiveStatusAffixes", "PassiveTriggerEffects"):
                targets.update(m["TargetSkill"] for m in definition.get(field, []) if m.get("TargetSkill", 0))
            if targets:
                native_scopes[identity] = sorted(targets)
    assert native_scopes == NATIVE_TARGETS, "original passive affected-skill lists changed"
    live_scopes = {}
    for edge in catalog["Synergies"]:
        if edge["RankMode"] == "effective_raw":
            live_scopes.setdefault(edge["Source"], set()).add(edge["Target"])
    assert {k: sorted(v) for k, v in live_scopes.items()} == NATIVE_TARGETS, "native passive synergy scope expanded or lost"
    sword_area = sorted(identity for identity, r in first.items() if r["prof"] == "2" and r["skill_type"] == "2")
    assert sword_area == [10121, 10125, 10127, 10128]
    print("PASS 750 original rows, 149 combat identities, 745 loaded rank identities/target kinds, 58 exact native passive relationships; 4 genuine sword area skills.")


if __name__ == "__main__":
    audit()
