"""Read generated normalized migration seed rows for offline audits.

This is not a runtime loader and does not read the retired Go/JSON catalogs.
For a deployed database, use its queryable game_endgame_* tables/views instead.
"""
import re
from pathlib import Path
ROOT = Path(__file__).resolve().parents[3]
MIGRATION = ROOT / 'server/migrations/000000_init-60.sql'
TOKEN = re.compile(r"\s*(?:('(?:[^']|'')*')|(-?\d+)|(true|false)|([(),]))", re.S)


def table_rows(table, implicit_columns=None):
    body = MIGRATION.read_text()
    result = []
    # Child batches retain literal seed rows inside an INSERT/SELECT guard so
    # migration replay cannot restore deliberately deleted configured children.
    pattern = (r'INSERT INTO ' + re.escape(table) + r'(?:\(([^)]+)\))? '
               r'(?:VALUES\s*|SELECT seed\.\* FROM \(VALUES\s*)(.*?)'
               r'(?:\) AS seed\([^)]*\) WHERE NOT EXISTS \([^;]*?\))? '
               r'ON CONFLICT[^;]+;')
    for match in re.finditer(pattern, body, re.S):
        columns = match.group(1).split(',') if match.group(1) else implicit_columns
        if not columns:
            raise ValueError('missing generated table column schema: ' + table)
        values, position, row = match.group(2), 0, None
        while position < len(values):
            token = TOKEN.match(values, position)
            if token is None:
                raise ValueError('unsupported generated SQL literal at ' + str(position))
            position = token.end()
            text, number, boolean, punctuation = token.groups()
            if punctuation == '(':
                row = []
            elif punctuation == ')':
                if row is None or len(row) != len(columns):
                    raise ValueError('generated SQL row width mismatch')
                result.append(dict(zip(columns, row)))
                row = None
            elif punctuation == ',':
                continue
            elif row is None:
                raise ValueError('literal outside generated SQL row')
            else:
                row.append(text[1:-1].replace("''", "'") if text is not None
                           else int(number) if number is not None else boolean == 'true')
    if not result and ('CREATE TABLE IF NOT EXISTS ' + table + ' ') not in body:
        raise ValueError('migration60 contains no normalized table: ' + table)
    return result


def definition_rows():
    return table_rows('game_endgame_definitions')


def field_name(column):
    special = {'id': 'ID', 'bp': 'BP', 'ms': 'MS', 'px': 'PX', 'pvp': 'PVP', 'hp': 'HP', 'icd': 'ICD'}
    return ''.join(special.get(part, part.title()) for part in column.split('_'))


def configured_definitions():
    """Reconstruct the SQL view's definition structure from scalar seed rows."""
    records = {}
    owner = ('owner_kind', 'content_version', 'definition_id')
    for row in definition_rows():
        key = tuple(row[k] for k in owner)
        records[key] = {'ID': row['definition_id'], **{
            field_name(k): v for k, v in row.items() if k not in owner}}
    sections = ('affixes', 'variable_affixes', 'local_enhancements', 'traits',
                'reserved_skills', 'skill_bonuses', 'typed_effects', 'runes', 'slots')
    for section in sections:
        name = field_name(section)
        for record in records.values():
            record[name] = []
        rows = table_rows('game_endgame_' + section)
        rows.sort(key=lambda row: tuple(row[k] for k in owner) + (row['ordinal'],))
        for row in rows:
            key = tuple(row[k] for k in owner)
            fields = {field_name(k): v for k, v in row.items() if k not in owner + ('ordinal',)}
            if section == 'runes':
                value = row['item_id']
            elif section == 'slots':
                value = row['slot']
            elif section == 'typed_effects':
                value = {k[0].lower() + k[1:]: v for k, v in fields.items()
                         if not k.startswith(('Proc', 'Aura'))}
                for prefix in ('Proc', 'Aura'):
                    payload = {k[len(prefix):]: v for k, v in fields.items() if k.startswith(prefix)}
                    if payload.get('ID'):
                        value[prefix.lower()] = {
                            ('id' if k == 'ID' else k[0].lower() + k[1:]): v for k, v in payload.items()}
            elif section == 'skill_bonuses':
                value = {k[0].lower() + k[1:]: v for k, v in fields.items()}
            else:
                value = fields
            records[key][name].append(value)
    return records
