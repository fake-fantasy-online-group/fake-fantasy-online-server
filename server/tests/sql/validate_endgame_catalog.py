#!/usr/bin/env python3
"""Validate migration 60 on a disposable PG cluster using no socket or daemon.

The supplied cluster must be an isolated test cluster. All checks run in a new
schema inside a transaction that is rolled back; production tables are untouched.
"""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

parser = argparse.ArgumentParser()
parser.add_argument('--data', type=Path, required=True)
parser.add_argument('--log', type=Path, required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[3]
body = (root / 'server/migrations/000000_init-60.sql').read_text()
checks = """
DO $$ BEGIN
 IF (SELECT count(*) FROM game_endgame_definitions WHERE owner_kind='unique') <> 2137
 OR (SELECT count(*) FROM game_endgame_definitions WHERE owner_kind='formation') <> 120
 OR (SELECT count(*) FROM game_endgame_rune_rules) <> 28
 OR (SELECT count(*) FROM game_equipment_skill_variants) <> 5
 THEN RAISE EXCEPTION 'seed completeness mismatch'; END IF;
 IF EXISTS(SELECT FROM game_endgame_catalog d WHERE
   jsonb_array_length(d.definition->'TypedEffects') < 0)
 THEN RAISE EXCEPTION 'invalid projection'; END IF;
END $$;
"""
# Re-running must preserve an operator's valid edits rather than resetting them.
edit = """
UPDATE game_endgame_definitions SET name='operator-configured-name'
 WHERE owner_kind='unique' AND content_version=6 AND definition_id=1001;
UPDATE game_endgame_rune_rules SET regeneration_interval_ms=1500
 WHERE item_id=1900028028;
-- These optional trait removals are valid catalog edits when the completeness
-- manifest is changed with them. Replay must not resurrect the old child rows.
DELETE FROM game_endgame_traits WHERE content_version=6
 AND ((owner_kind='unique' AND definition_id=2013)
   OR (owner_kind='formation' AND definition_id=1));
UPDATE game_endgame_definition_manifest SET expected_rows=0
 WHERE content_version=6 AND section='Traits'
 AND ((kind='unique' AND definition_id=2013)
   OR (kind='formation' AND definition_id=1));
"""
preserve = """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_endgame_definitions WHERE owner_kind='unique'
 AND content_version=6 AND definition_id=1001 AND name='operator-configured-name')
 OR NOT EXISTS(SELECT FROM game_endgame_rune_rules WHERE item_id=1900028028
 AND regeneration_interval_ms=1500)
 THEN RAISE EXCEPTION 'rerun overwrote configured data'; END IF;
 IF EXISTS(SELECT FROM game_endgame_traits WHERE content_version=6
 AND ((owner_kind='unique' AND definition_id=2013)
   OR (owner_kind='formation' AND definition_id=1)))
 OR (SELECT count(*) FROM game_endgame_definition_manifest
 WHERE content_version=6 AND section='Traits' AND expected_rows=0
 AND ((kind='unique' AND definition_id=2013)
   OR (kind='formation' AND definition_id=1))) <> 2
 THEN RAISE EXCEPTION 'rerun resurrected deleted configured children'; END IF;
END $$;
"""
source = ('BEGIN;\nCREATE SCHEMA catalog60_fresh_validation;\n'
          'SET LOCAL search_path=catalog60_fresh_validation;\n'
          + body + checks + edit + preserve + body + checks + preserve + '\nROLLBACK;\n')
commands = io.StringIO()
write_single_user_commands(io.StringIO(source), commands)
with args.log.open('w') as out:
    result = subprocess.run([str(Path(os.environ['PG_BIN']) / 'postgres'),
                             '--single', '-j', '-D', str(args.data), 'postgres'],
                            input=commands.getvalue(), text=True, stdout=out,
                            stderr=subprocess.STDOUT)
errors = [line.strip() for line in args.log.read_text().splitlines()
          if re.search(r'\b(?:ERROR|FATAL|PANIC):', line)]
if result.returncode or errors:
    raise SystemExit(f'migration60 validation failed: {result.returncode}: {errors[:3]}')
print('ENDGAME_CATALOG_POSTGRES_PASS: fresh schema, complete projection, repeat, configured values and child deletions preserved, rollback')
