#!/usr/bin/env python3
"""Check SQL63 in a fresh rollback-only schema on an isolated, idle test cluster."""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

p = argparse.ArgumentParser()
p.add_argument('--data', type=Path, required=True)
p.add_argument('--log', type=Path, required=True)
a = p.parse_args()
root = Path(__file__).resolve().parents[3]
body = (root / 'server/migrations/000000_init-63.sql').read_text()
checks = '''
DO $$ BEGIN
 IF (SELECT count(*) FROM game_loot_rune_pool)<>28
 OR (SELECT count(*) FROM game_loot_grades)<>4
 OR (SELECT count(*) FROM game_loot_socket_levels)<>4
 OR (SELECT count(*) FROM game_rift_class_rules)<>4
 OR (SELECT count(*) FROM game_rift_ability_rules)<>3
 THEN RAISE EXCEPTION 'incomplete balance seed'; END IF;
END $$;
DO $$ DECLARE sql TEXT; BEGIN
 FOREACH sql IN ARRAY ARRAY[
 'UPDATE game_rift_rule_versions SET normal_progress=99 WHERE version=1',
 'UPDATE game_rift_ability_rules SET damage_pct=99 WHERE version=1',
 'DELETE FROM game_loot_grades WHERE version=1',
 'INSERT INTO game_loot_socket_levels VALUES(1,80,5)'] LOOP
  BEGIN
   EXECUTE sql;
   RAISE EXCEPTION 'published mutation unexpectedly succeeded';
  EXCEPTION WHEN OTHERS THEN
   IF SQLERRM NOT LIKE '%immutable%' THEN RAISE; END IF;
  END;
 END LOOP;
END $$;
'''
version2 = '''
DO $$ DECLARE t TEXT; BEGIN
 FOREACH t IN ARRAY ARRAY['game_loot_rule_versions','game_loot_rune_pool',
 'game_loot_socket_levels','game_loot_grades','game_rift_rule_versions',
 'game_rift_class_rules','game_rift_ability_rules'] LOOP
  EXECUTE format('INSERT INTO %I SELECT (jsonb_populate_record(NULL::%I,to_jsonb(s)||''{"version":2,"published":false}'')).* FROM %I s WHERE version=1',t,t,t);
 END LOOP;
 UPDATE game_loot_rule_versions SET unique_base_ppb=20000000,published=true WHERE version=2;
 UPDATE game_rift_rule_versions SET normal_progress=99,timed_ms=800000,loot_version=2,published=true WHERE version=2;
 UPDATE game_loot_rule_active SET version=2;
 UPDATE game_rift_rule_active SET version=2;
END $$;
'''
preserve = '''
DO $$ BEGIN
 IF (SELECT version FROM game_loot_rule_active)<>2 OR (SELECT version FROM game_rift_rule_active)<>2
 THEN RAISE EXCEPTION 'repeat overwrote active'; END IF;
 IF (SELECT normal_progress FROM game_rift_rule_versions WHERE version=1)<>40
 OR (SELECT timed_ms FROM game_rift_rule_versions WHERE version=1)<>900000
 OR (SELECT loot_version FROM game_rift_rule_versions WHERE version=1)<>1
 THEN RAISE EXCEPTION 'old version drifted'; END IF;
 IF (SELECT normal_progress FROM game_rift_rule_versions WHERE version=2)<>99
 OR (SELECT loot_version FROM game_rift_rule_versions WHERE version=2)<>2
 THEN RAISE EXCEPTION 'new version lost'; END IF;
END $$;
'''
source = ('''BEGIN;
CREATE SCHEMA balance63_fresh_validation;
SET LOCAL search_path=balance63_fresh_validation;
CREATE TABLE game_items AS SELECT id FROM public.game_items;
ALTER TABLE game_items ADD PRIMARY KEY(id);
''' + body + checks + body + checks + version2 + body + preserve + '\nROLLBACK;\n')
commands = io.StringIO()
write_single_user_commands(io.StringIO(source), commands)
with a.log.open('w') as out:
    result = subprocess.run([str(Path(os.environ['PG_BIN']) / 'postgres'), '--single', '-j', '-D', str(a.data), 'postgres'], input=commands.getvalue(), text=True, stdout=out, stderr=subprocess.STDOUT)
errors = [s.strip() for s in a.log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', s)]
if result.returncode or errors:
    raise SystemExit(f'balance validation failed: {result.returncode}: {errors[:3]}')
print('ENDGAME_BALANCE_POSTGRES_PASS: fresh, repeat, immutable published rows, active2 switch, v1 unchanged, rollback')
