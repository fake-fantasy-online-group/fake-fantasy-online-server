#!/usr/bin/env python3
"""Disposable PostgreSQL V5 progression checks. Never targets an existing cluster."""
import argparse,os,re,subprocess
from pathlib import Path
from validate_daoist_postgres import write_single_user_commands,concatenate_sources
ROOT=Path(__file__).resolve().parents[3]
p=argparse.ArgumentParser();p.add_argument('--work-dir',type=Path,required=True);args=p.parse_args()
w=args.work_dir.resolve();w.mkdir(parents=True,exist_ok=False)
bins=Path(os.environ['PG_BIN']);data=w/'data'
subprocess.run([str(bins/'initdb'),'-D',str(data),'--no-locale','-E','UTF8','-U','validator','-A','trust'],check=True,stdout=(w/'init.log').open('w'),stderr=subprocess.STDOUT)
def run(label,sql=None,paths=None,expected_error=None):
 raw=w/(label+'.source.sql');inp=w/(label+'.sql');log=w/(label+'.log')
 with raw.open('wb') as f:
  if paths:concatenate_sources(paths,f)
  else:f.write(sql.encode())
 with raw.open() as source,inp.open('w') as target:write_single_user_commands(source,target)
 raw.unlink()
 with inp.open() as source,log.open('w') as out:r=subprocess.run([str(bins/'postgres'),'--single','-j','-D',str(data),'postgres'],stdin=source,stdout=out,stderr=subprocess.STDOUT)
 errors=[]
 with log.open() as f:
  for line in f:
   if re.search(r'\b(?:ERROR|FATAL|PANIC):',line):errors.append(line.strip())
 if expected_error:
  assert any(expected_error in e for e in errors),(label,errors)
 else:assert r.returncode==0 and not errors,(label,errors[:3])
 inp.unlink();print('PASS',label,flush=True)
run('fresh-init',paths=[ROOT/f'server/migrations/000000_init-{i}.sql' for i in range(53)])
run('fresh-assert',sql="""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_progression_versions WHERE version='progression-v5' AND player_cap=100 AND pet_cap=80) THEN RAISE EXCEPTION 'manifest';END IF;
 IF (SELECT max_level FROM game_trial_rules WHERE id=1)<>100 THEN RAISE EXCEPTION 'trial cap';END IF;
 IF (SELECT count(*) FROM game_trial_monster_stats WHERE level=103)<>35 THEN RAISE EXCEPTION 'boss103';END IF;
 IF (SELECT exp FROM game_levels WHERE level=61)<>27027200 THEN RAISE EXCEPTION 'exp61';END IF;
 IF (SELECT exp_need FROM gamedata.ov_petlevelexp WHERE level=80)<>693546710 THEN RAISE EXCEPTION 'pet80';END IF;
END $$;
INSERT INTO accounts(id,username)VALUES(930000001,'progression_legacy');
INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y,level,exp)VALUES(930000001,930000001,0,'legacy60',1,7,10,20,60,234);
CREATE TABLE progression_character_before AS SELECT to_jsonb(c) row FROM characters c WHERE id=930000001;
CREATE TABLE progression_curves_before AS SELECT level,exp,exp_accum,raw FROM game_levels;
-- Model pre-V5 state without altering character or old curve data.
DELETE FROM game_progression_versions WHERE version='progression-v5';
DELETE FROM game_trial_monster_stats WHERE level>=64;
UPDATE game_trial_rules SET max_level=60 WHERE id=1;
CREATE TABLE progression_experience_items_before AS SELECT * FROM game_item_player_experience WHERE player_level=81;
DELETE FROM game_item_player_experience WHERE player_level=81;
DELETE FROM game_levels WHERE level=81;
DELETE FROM gamedata.ov_petlevelexp WHERE level=70;
""")
upgrade=ROOT/'docs/migrations/2026-10-12-v5-progression.sql'
run('upgrade',paths=[upgrade]);run('repeat-upgrade',paths=[upgrade])
run('upgrade-preserves-saves',sql="""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM progression_character_before b JOIN characters c ON c.id=930000001 WHERE b.row=to_jsonb(c)) THEN RAISE EXCEPTION 'character changed';END IF;
 IF EXISTS((SELECT * FROM progression_curves_before EXCEPT SELECT level,exp,exp_accum,raw FROM game_levels) UNION ALL (SELECT level,exp,exp_accum,raw FROM game_levels EXCEPT SELECT * FROM progression_curves_before)) THEN RAISE EXCEPTION 'curves changed';END IF;
 IF (SELECT count(*) FROM gamedata.ov_petlevelexp WHERE level BETWEEN 1 AND 80)<>80 THEN RAISE EXCEPTION 'pet hole not restored';END IF;
END $$;
UPDATE game_levels SET exp=exp+1 WHERE level=61;
""")
run('reject-tuned-curve',paths=[upgrade],expected_error='player curve differs')
run('reject-no-overwrite',sql="DO $$ BEGIN IF (SELECT exp FROM game_levels WHERE level=61)<>27027201 THEN RAISE EXCEPTION 'overwrote tuned curve';END IF; END $$;")
print('PROGRESSION_V5_POSTGRES_PASS',flush=True)
