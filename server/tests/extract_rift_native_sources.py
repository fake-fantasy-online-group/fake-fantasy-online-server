from pathlib import Path
import re,sqlite3,json,bisect,sys
# Usage: python3 server/tests/extract_rift_native_sources.py <complete migration directory> <output directory>
root=Path(sys.argv[1]); out=Path(sys.argv[2]); out.mkdir(parents=True,exist_ok=True)
files=sorted(root.glob('*.sql'),key=lambda f:int(f.stem.split('-')[-1]));files=[f for f in files if int(f.stem.split('-')[-1])<=38]
parts=[f.read_text() for f in files]; offsets=[];i=0
for p in parts:offsets.append(i);i+=len(p)+1
text='\n'.join(parts)
names={'map_monster_spawns','game_monsters','game_monster_drops','game_equipment','gamedata.ov_arm'}
conn=sqlite3.connect(':memory:');conn.create_function('decode',2,lambda a,b:a)
rows={};sources={};sql={}
pat=r'INSERT\s+INTO\s+((?:"?\w+"?\.)?"?\w+"?)\s*\(([^;]*?)\)\s*VALUES\b'
for m in re.finditer(pat,text,re.I):
 name=m[1].replace('"','')
 if name not in names:continue
 cols=[c.strip().strip('"') for c in m[2].split(',')]
 end=m.end();quoted=False
 while end<len(text):
  if text[end]=="'":
   if quoted and end+1<len(text) and text[end+1]=="'":end+=2;continue
   quoted=not quoted
  if text[end]==';' and not quoted:break
  end+=1
 values=re.split(r'\bON\s+CONFLICT\b',text[m.end():end],flags=re.I)[0]
 conn.execute('DROP TABLE IF EXISTS temp_extract');conn.execute('CREATE TABLE temp_extract ('+','.join('"'+c+'"' for c in cols)+')')
 try:conn.executescript('INSERT INTO temp_extract VALUES '+values+';')
 except Exception as e:print(name,'ERROR',e);raise
 block=[dict(zip(cols,row)) for row in conn.execute('SELECT * FROM temp_extract')];j=bisect.bisect_right(offsets,m.start())-1
 src={'file':files[j].name,'line':parts[j].count('\n',0,m.start()-offsets[j])+1,'rows':len(block),'columns':cols}
 rows.setdefault(name,[]).extend(block);sources.setdefault(name,[]).append(src);sql.setdefault(name,[]).append('-- '+src['file']+':'+str(src['line'])+'\n'+text[m.start():end+1])
for name,ss in sql.items():(out/(name.replace('.','_')+'-inserts.sql')).write_text('\n'.join(ss))
json.dump(rows,open(out/'native-catalog.json','w'),ensure_ascii=False,indent=2);json.dump(sources,open(out/'native-sources.json','w'),ensure_ascii=False,indent=2)
print({k:len(v) for k,v in rows.items()})

missing=names-set(sql)
if missing: raise SystemExit('Incomplete source checkout; missing native INSERT tables: '+', '.join(sorted(missing)))
