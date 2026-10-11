#!/usr/bin/env python3
"""Read-only audit of native rift geometry from checked-in SQL fixtures.

Usage: python3 tests/rift_layout_audit.py --output docs/endgame/rift-layout-audit.json
No PostgreSQL or extracted client files are required. NumPy/SciPy are optional
accelerators; the standard-library fallback performs the same 4-neighbor fill.
This is an importer-specific static audit, not a general SQL interpreter or a
substitute for the runtime catalog's loaded-definition/asset checks.
"""
import argparse
from array import array
from collections import defaultdict, deque
import gzip
import hashlib
import json
from pathlib import Path
import re
import struct

try:
    import numpy as np
    from scipy.ndimage import label as component_labels
except ImportError:
    np = None


def largest_component(raw, points):
    if raw[:8] != b"QQF\x1aMASK":
        raise ValueError("invalid native collision magic")
    width, height, cw, ch = struct.unpack_from("<IIII", raw, 16)
    if len(raw) != 32 + width * height or not width or not height or not cw or not ch:
        raise ValueError("invalid native collision dimensions")
    cells = raw[32:]
    groups, seen = defaultdict(list), set()
    if np is not None:
        grid = np.frombuffer(cells, dtype=np.uint8).reshape((height, width))
        labels, _ = component_labels((grid & 128) == 0)
        get_label = lambda cell: int(labels[cell // width, cell % width])
    else:
        labels, counter = array("i", [0]) * len(cells), [0]

        def get_label(cell):
            if cells[cell] & 128:
                return 0
            if labels[cell]:
                return labels[cell]
            counter[0] += 1
            value = counter[0]
            labels[cell] = value
            queue = deque([cell])
            while queue:
                here = queue.popleft()
                neighbors = []
                if here % width:
                    neighbors.append(here - 1)
                if here % width < width - 1:
                    neighbors.append(here + 1)
                if here >= width:
                    neighbors.append(here - width)
                if here + width < len(cells):
                    neighbors.append(here + width)
                for neighbor in neighbors:
                    if not labels[neighbor] and not cells[neighbor] & 128:
                        labels[neighbor] = value
                        queue.append(neighbor)
            return value

    for point in sorted(points):
        x, y = point[2:4]
        if (x, y) in seen or x < 0 or y < 0 or x // cw >= width or y // ch >= height:
            continue
        component = get_label((y // ch) * width + x // cw)
        if component:
            groups[component].append(point)
            seen.add((x, y))
    return max(groups.values(), key=len) if groups else []


def separated_safe_points(points):
    if len(points) < 18:
        return False
    def distance(a, b):
        return (a[2] - b[2]) ** 2 + (a[3] - b[3]) ** 2
    if max(distance(a, b) for a in points for b in points) < 1024 ** 2:
        return False
    for entry in points:
        exit_point = max(points, key=lambda point: distance(entry, point))
        if sum(distance(entry, point) >= 160 ** 2 and distance(exit_point, point) >= 80 ** 2
               for point in points) >= 8:
            return True
    return False


def audit(fixtures):
    sources = {number: fixtures / f"000000_init-{number}.sql" for number in (0, 2, 32, 46, 47, 48, 49)}
    texts = {number: path.read_text() for number, path in sources.items()}
    maps = {}
    for block in re.findall(r'INSERT INTO "map_defs".*?VALUES\s*(.*?);', texts[0], re.S):
        for identity, resource, name, city in re.findall(r"\((\d+), '([^']+)', '([^']*)', (TRUE|FALSE)\)", block):
            maps[int(identity)] = (resource, name, city == "TRUE")
    for block in re.findall(r'INSERT INTO map_defs\(id,file,name\) VALUES\s*(.*?)ON CONFLICT', texts[46], re.S):
        for identity, resource, name in re.findall(r"\((\d+),'([^']+)','([^']*)'\)", block):
            identity = int(identity)
            maps[identity] = (resource, name, maps.get(identity, (None, None, False))[2])
    spawns = defaultdict(list)
    for block in re.findall(r'INSERT INTO "map_monster_spawns".*?VALUES\s*(.*?);', texts[0], re.S):
        for resource, identity, monster, x, y, direction in re.findall(r"\('([^']+)', (\d+), (\d+), (\d+), (\d+), (\d+)\)", block):
            spawns[resource].append(tuple(map(int, (identity, monster, x, y, direction))))
    models = {}
    for block in re.findall(r'INSERT INTO "game_monsters".*?VALUES\s*(.*?);', texts[2], re.S):
        models.update({int(identity): int(sprite) for identity, sprite in re.findall(r'^\s*\((\d+), (\d+),', block, re.M)})
    combat = {}
    for block in re.findall(r'UPDATE game_monsters AS target.*?FROM \(VALUES\s*(.*?)\) AS source\(id,level,hp', texts[32], re.S):
        for identity, level, hp, kind in re.findall(r"\((\d+),(\d+),(\d+),\d+,\d+,\d+,\d+,'([^']*)'", block):
            if int(hp) > 0 and int(level) > 0 and kind in ("普通", "精英", "BOSS") and models.get(int(identity), 0) > 0:
                combat[int(identity)] = kind
    for identity in re.findall(r'UPDATE game_monsters SET can_attack\s*=\s*FALSE WHERE id\s*=\s*(\d+)', texts[32]):
        combat.pop(int(identity), None)
    masks = {}
    for number in (47, 48, 49):
        for resource, compressed in re.findall(r"\(convert_from\(decode\('([0-9a-f]+)','hex'\),'UTF8'\),decode\('([0-9a-f]+)','hex'\),\d+\)", texts[number]):
            masks[bytes.fromhex(resource).decode()] = compressed
    eligible = []
    for identity, (resource, name, city) in sorted(maps.items()):
        if city or identity in (60, 21000) or resource not in masks:
            continue
        points = [point for point in spawns[resource] if point[1] in combat]
        if len(points) < 18:
            continue
        points = largest_component(gzip.decompress(bytes.fromhex(masks[resource])), points)
        if not separated_safe_points(points) or not any(combat[point[1]] != "BOSS" for point in points):
            continue
        eligible.append({"mapId": identity, "mapName": name, "connectedAnchors": len(points),
                         "monsterIds": sorted({point[1] for point in points})})
    return {"generatorVersion": 1, "auditKind": "static SQL fixtures and native collision masks",
            "runtimeCaveat": "The live loader separately verifies its loaded monster definitions, sprites, attack eligibility and non-city map catalog. This audit does not establish live PostgreSQL or client runtime behavior.",
            "criteria": ["Non-city native map; island aliases excluded", "Existing hostile positive-HP positive-sprite attack-capable definitions",
                         "At least 18 unique walkable anchors in one native 4-neighbor component", "At least 1024 pixels anchor diameter",
                         "Entry and exit separated by at least 512 pixels; at least 8 safe population anchors"],
            "sourceMaps": len(maps), "sourceMasks": len(masks), "eligibleMaps": len(eligible),
            "sourceSHA256": {path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in sources.values()},
            "maps": eligible}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixtures", type=Path, default=Path(__file__).resolve().parents[1] / "migrations")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = audit(args.fixtures)
    encoded = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(encoded)
        print(f"Validated {result['eligibleMaps']} static candidate maps; wrote {args.output}")
    else:
        print(encoded, end="")

if __name__ == "__main__":
    main()
