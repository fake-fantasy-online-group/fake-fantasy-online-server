"""Read original SQL for the itemization audit; no database/network/mutations."""
from __future__ import annotations
import collections
import pathlib
import re
ROOT = pathlib.Path(__file__).resolve().parents[3]
TABLES = {'ov_arm', 'ov_desc', 'ov_card', 'ov_card_entry'}
ATTRS = {1, 3, 5, 7, 9, 61, 13, 15, 17, 19, 23, 25, 27, 29, 31, 34, 36, 38, 40, 47, 66, 69, 70, 118}
PARTS = {1: 'spear', 2: 'single_sword', 4: 'dual_sword', 8: 'dagger', 16: 'staff', 32: 'throw', 101: 'face', 102: 'hat', 103: 'necklace', 105: 'shield', 106: 'glove', 107: 'ring', 108: 'dress', 109: 'shoe', 110: 'bag', 114: 'ear', 115: 'bangle', 116: 'belt'}

def split_sql(text):
    """Split only top-level commas; preserve PostgreSQL escaped quotes/decode()."""
    parts, start, depth, quoted, i = ([], 0, 0, False, 0)
    while i < len(text):
        c = text[i]
        if c == "'":
            if quoted and i + 1 < len(text) and (text[i + 1] == "'"):
                i += 2
                continue
            quoted = not quoted
        elif not quoted:
            if c == '(':
                depth += 1
            elif c == ')':
                depth -= 1
            elif c == ',' and depth == 0:
                parts.append(text[start:i].strip())
                start = i + 1
        i += 1
    parts.append(text[start:].strip())

    def value(s):
        if s.startswith("'") and s.endswith("'"):
            return s[1:-1].replace("''", "'")
        if re.fullmatch('-?\\d+', s):
            return int(s)
        return s
    return [value(x) for x in parts]

def read_tables():
    out = {k: [] for k in TABLES}
    header = re.compile('^INSERT INTO (?:(?:"gamedata"|gamedata)\\.)?"?(\\w+)"?\\s*\\((.*?)\\) VALUES\\s*$')
    table, columns = (None, [])
    pending, row_start = ('', 0)
    paths = sorted((ROOT / 'server/migrations').glob('000000_init*.sql'), key=lambda p: int(re.search('-(\\d+)\\.sql$', p.name).group(1)) if '-' in p.name else 0)
    for path in paths:
        with path.open(encoding='utf-8') as f:
            for line_no, line in enumerate(f, 1):
                if line.startswith('INSERT INTO '):
                    match = header.match(line.rstrip())
                    table = match.group(1) if match and match.group(1) in TABLES else None
                    if table:
                        columns = [s.strip().strip('"') for s in match.group(2).split(',')]
                elif table and (pending or line.lstrip().startswith('(')):
                    if not pending:
                        row_start = line_no
                    pending += line
                    raw = pending.strip().rstrip(',;')
                    if not raw.endswith(')') or raw.count("'") % 2:
                        continue
                    pending = ''
                    values = split_sql(raw[1:-1])
                    assert len(values) == len(columns), (path, line_no, len(values), len(columns))
                    row = dict(zip(columns, values))
                    row['_source'] = f'{path.relative_to(ROOT)}:{row_start}'
                    out[table].append(row)
                elif table and line.startswith('ON CONFLICT'):
                    table = None
    return out

def first_index(rows, key='index'):
    result = {}
    for row in sorted(rows, key=lambda x: x.get('row_no', 0)):
        result.setdefault(row[key], row)
    return result
