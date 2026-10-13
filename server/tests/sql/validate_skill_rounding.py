#!/usr/bin/env python3
"""Verify migration 67 on a copied idle test fixture; no listener or game database."""
import argparse, os, re, shutil, subprocess
from pathlib import Path
from validate_daoist_postgres import write_single_user_commands
p=argparse.ArgumentParser()
p.add_argument('--seed-dir',type=Path,required=True)
p.add_argument('--work-dir',type=Path,required=True)
a=p.parse_args(); seed=a.seed_dir.resolve(); work=a.work_dir.resolve()
if (seed/'postmaster.pid').exists(): raise SystemExit('refusing a running source cluster')
work.mkdir(parents=True,exist_ok=False); data=work/'data'; shutil.copytree(seed,data)
root=Path(__file__).resolve().parents[3]; pg=Path(os.environ['PG_BIN'])/'postgres'
body=(root/'server/migrations/000000_init-67.sql').read_text()
def run(name,sql,error=None):
 import io
 cmd=work/(name+'.sql'); log=work/(name+'.log')
 with cmd.open('w') as o: write_single_user_commands(io.StringIO(sql),o)
 with cmd.open() as i,log.open('w') as o:
  r=subprocess.run([str(pg),'--single','-j','-D',str(data),'postgres'],stdin=i,stdout=o,stderr=subprocess.STDOUT)
 es=[l for l in log.read_text().splitlines() if re.search(r'\b(ERROR|FATAL|PANIC):',l)]
 if error: assert any(error in e for e in es),(name,es)
 else: assert r.returncode==0 and not es,(name,es)
 print('PASS',name,flush=True)
run('legacy-fixture',"""DO $$ BEGIN IF EXISTS(SELECT FROM pg_attribute WHERE attrelid='characters'::regclass AND attname='skill_rounding_credits' AND NOT attisdropped) THEN RAISE EXCEPTION 'already migrated'; END IF; END $$;
INSERT INTO accounts(username) VALUES('rounding_migration_test');
INSERT INTO characters(account_id,slot,name,race,map_id,pos_x,pos_y) SELECT id,0,'rounding_migration_test',0,7,100,200 FROM accounts WHERE username='rounding_migration_test';
""")
run('transaction-rollback','BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rounding failure'; END $$;\nCOMMIT;",'deliberate rounding failure')
run('rollback-preserves-schema',"DO $$ BEGIN IF EXISTS(SELECT FROM pg_attribute WHERE attrelid='characters'::regclass AND attname='skill_rounding_credits' AND NOT attisdropped) THEN RAISE EXCEPTION 'rollback failed'; END IF; END $$;")
run('migrate-existing-character',body+"""
DO $$ BEGIN IF NOT EXISTS(SELECT FROM characters WHERE name='rounding_migration_test' AND skill_rounding_credits='{}'::jsonb AND map_id=7 AND pos_x=100 AND pos_y=200) THEN RAISE EXCEPTION 'legacy character changed'; END IF; END $$;
UPDATE characters SET skill_rounding_credits='{"10018":{"mana":{"n":37,"d":146},"cooldown":{"n":41,"d":14600}}}'::jsonb WHERE name='rounding_migration_test';
""")
run('replay-preserves-credit',body+"""
DO $$ BEGIN IF NOT EXISTS(SELECT FROM characters WHERE name='rounding_migration_test' AND skill_rounding_credits='{"10018":{"mana":{"n":37,"d":146},"cooldown":{"n":41,"d":14600}}}'::jsonb) THEN RAISE EXCEPTION 'replay lost prepaid credit'; END IF; END $$;
""")
for bad in ['[]','null','7']:
 run('reject-'+bad.replace('[]','array'),"UPDATE characters SET skill_rounding_credits='"+bad+"'::jsonb WHERE name='rounding_migration_test';",'characters_skill_rounding_object')
run('reject-sql-null',"UPDATE characters SET skill_rounding_credits=NULL WHERE name='rounding_migration_test';",'not-null constraint')
run('new-character-default',"""INSERT INTO characters(account_id,slot,name,race,map_id,pos_x,pos_y) SELECT id,1,'rounding_migration_new',0,7,100,200 FROM accounts WHERE username='rounding_migration_test';
DO $$ BEGIN IF NOT EXISTS(SELECT FROM characters WHERE name='rounding_migration_new' AND skill_rounding_credits='{}'::jsonb) THEN RAISE EXCEPTION 'new default missing'; END IF; END $$;
""")
print('Isolated fixture:',data)
