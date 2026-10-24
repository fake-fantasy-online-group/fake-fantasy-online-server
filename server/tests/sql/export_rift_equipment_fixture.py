#!/usr/bin/env python3
"""Rebuild the native Rift regression fixture from repository SQL literals.
Uses an isolated temporary SQLite read-model, never an application database.
The migration82 candidate relation is unchanged except for SQLite names and
JSON aggregation; affix weights mirror migration39 and are count-checked.
"""
import re, sqlite3, pathlib, json, collections, math, tempfile
root=pathlib.Path(__file__).resolve().parents[3]
_temp=tempfile.TemporaryDirectory(prefix='rift-native-fixture-')
work=pathlib.Path(_temp.name)
names={'game_equipment','gamedata.ov_arm','gamedata.ov_desc','gamedata.ov_card','gamedata.ov_card_entry','game_monsters','game_monster_drops','map_monster_spawns'}
db=sqlite3.connect(work/'seed.sqlite'); db.create_function('decode',2,lambda a,b:a)
# SQLite is a disposable read-model of repository literals; no user database is contacted.
for n in names: db.execute('DROP TABLE IF EXISTS '+n.replace('.','_'))
text=''.join(p.read_text() for p in [root/'server/migrations'/f'000000_init-{n}.sql' for n in range(33)])
count=collections.Counter()
for m in re.finditer(r'INSERT INTO ((?:"[^"]+"\.)?"[^"]+") \(([^\n]+)\) VALUES\n(.*?)(?=\nON CONFLICT)', text, re.S):
 n=m[1].replace('"','')
 if n not in names:continue
 cols=re.findall('"([^"]+)"',m[2]); table=n.replace('.','_')
 db.execute('CREATE TABLE IF NOT EXISTS '+table+' ('+','.join('"'+c+'"' for c in cols)+')')
 sql='INSERT INTO '+table+' ('+m[2]+') VALUES '+m[3]
 try: db.execute(sql)
 except Exception as e: print('ERROR',n,str(e),sql[:100]); raise
 count[n]+=db.execute('SELECT changes()').fetchone()[0]
db.commit(); print(count,flush=True)
for n in names: print(n, [r[1] for r in db.execute('pragma table_info('+n.replace('.','_')+')')],flush=True)
# Import actual later monster templates and natural-drop literal seeds.
s32=(root/'server/migrations/000000_init-32.sql').read_text()
for m in re.finditer(r'UPDATE game_monsters AS target SET [^\n]+\nFROM \(VALUES\n(.*?)\n\) AS source\(([^)]+)\) WHERE target.id=source.id;',s32,re.S):
 cols=m[2].split(',')
 db.execute('CREATE TABLE IF NOT EXISTS monster_templates ('+','.join('"'+c+'"' for c in cols)+')')
 db.execute('INSERT INTO monster_templates VALUES '+m[1])
for m in re.finditer(r'INSERT INTO game_monster_drops \(([^)]+)\) VALUES\n(.*?)(?=\nON CONFLICT|;\n)',s32,re.S):
 db.execute('CREATE TABLE IF NOT EXISTS game_monster_drops ('+m[1]+')')
 db.execute('INSERT INTO game_monster_drops VALUES '+m[2])
db.commit()
print('monster templates',db.execute('select count(*) from monster_templates').fetchone(),'drops',db.execute('select count(*) from game_monster_drops').fetchone(),flush=True)
# Exact migration 82 candidate relation, adapted only for SQLite JSON grouping below.
s82=(root/'server/migrations/000000_init-82.sql').read_text()
q=s82[s82.index('WITH sources AS'):s82.index('), tiers AS')]+') SELECT level,monster_id,item_id FROM candidates'
for table,col in [('monster_templates','id'),('monster_templates','elite_id'),('map_monster_spawns','monster'),('game_monster_drops','monster_id'),('game_equipment','id'),('gamedata_ov_arm','index')]:
 db.execute('CREATE INDEX IF NOT EXISTS idx_'+table+col+' ON '+table+'(\"'+col+'\")')
q=q.replace('game_monsters','monster_templates').replace('gamedata.ov_arm','gamedata_ov_arm').replace('a.index','a."index"').replace('SELECT FROM','SELECT 1 FROM').replace('btrim(', 'trim(')
rows=db.execute(q).fetchall(); pools={}
for level,monster,item in rows:
 p=pools.setdefault(level,{'MonsterLevel':level,'SourceMonsters':set(),'Items':set()}); p['SourceMonsters'].add(monster); p['Items'].add(item)
pools=[{k:sorted(v) if isinstance(v,set) else v for k,v in p.items()} for level,p in sorted(pools.items())]
(work/'pools.json').write_text(json.dumps(pools,ensure_ascii=False,indent=2))
print('pools',[(p['MonsterLevel'],len(p['Items'])) for p in pools],flush=True)

w=work
db.row_factory=sqlite3.Row
parts={1:'spear',2:'single_sword',4:'dual_sword',8:'dagger',16:'staff',32:'throw',101:'face',102:'hat',103:'necklace',105:'shield',106:'glove',107:'ring',108:'dress',109:'shoe',110:'bag',114:'ear',115:'bangle',116:'belt'}
levels=list(db.execute('select distinct a.level,d.type from gamedata_ov_arm a join gamedata_ov_desc d on d."index"=a."index" where a."index"<>0 and a.no_type_drop=0 and a.level between 1 and 17'))
cards=[dict(r) for r in db.execute('select c.*,e.attr_id,e.value,e.mode from gamedata_ov_card c join gamedata_ov_card_entry e using(row_no) where (c."index" between 5001 and 5997 or c."index" between 13141 and 13191) and c."index"<>0 and c.category=0 and c.fun_type=0 and e.op_type=1 and e.prob=100 and e.mode in(0,1) and e.attr_id in(1,3,5,7,9,61,13,15,17,19,23,25,27,29,31,34,36,38,40,47,66,69,70,118)')]
bykey=collections.defaultdict(list)
for level,typ in levels:
 if typ not in parts:continue
 for c in cards:
  if c['min_level']<=level<=c['max_level'] and c[parts[typ]]!=0:
   bykey[level,typ,c['attr_id'],c['mode']].append(c)
options=[]
for (lv,typ,attr,mode),cc in bykey.items():
 lo=min(abs(c['value']) for c in cc);hi=max(abs(c['value']) for c in cc)
 scores=[1 if hi==lo else math.exp(-(math.log(1+(abs(c['value'])-lo)/(hi-lo))-(math.log(2)+.45*.45))**2/(2*.45*.45))/(1+(abs(c['value'])-lo)/(hi-lo)) for c in cc]
 total=sum(scores); modes=len({m for l,t,a,m in bykey if (l,t,a)==(lv,typ,attr)})
 for c,score in zip(cc,scores):
  options.append({'Level':lv,'Type':typ,'CardID':c['index'],'Affix':{'Attr':attr,'Value':c['value'],'Mode':mode},'Weight':max(1,math.floor(1000000*score/total/modes+.5))})
print('options',len(options),'keys',len({(o['Level'],o['Type']) for o in options}))
assert len(options)==16583
assert len({(o['Level'],o['Type']) for o in options})==135
(w/'options.json').write_text(json.dumps(options,ensure_ascii=False))
# Loader-exact fields needed by the generation, eligibility, names and level consumers.
items=[]
for r in db.execute('select e.*, a.*,d.type desc_type from game_equipment e join gamedata_ov_arm a on a."index"=e.id left join gamedata_ov_desc d on d.row_no=(select min(row_no) from gamedata_ov_desc where "index"=e.id) order by e.id'):
 r=dict(r);mask=sum((1<<i) for i,job in enumerate(['warrior','swordman','stabber','druggist','magician']) if r[job] or r['sr_'+job]); native=not r['no_type_drop'] and r['level']>=0 and (r['query_type'] in set(range(1,16))|{49,51} or r['position_'] in (114,116))
 equip={'Slot':r['slot'],'LevelReq':r['level_req'],'ResourceLevel':r['level'],'Type':r['desc_type'] or 0,'Position':r['position_'],'Tier':r['level'],'NoTypeDrop':bool(r['no_type_drop']),'NativeDropEligible':native,'AscensionOnly':not any(r[k] for k in ['newbie','warrior','swordman','stabber','druggist','magician']) and bool(mask),'Durable':r['duration']//300,'Need':{'Level':r['level_need'],'Sex':r['sex'],'BeginnerAllowed':bool(r['newbie']),'ProfessionRestricted':bool(mask or r['newbie']),'Professions':mask}}
 items.append({'ID':r['id'],'Name':r['name'],'Level':r['level_req'],'InstanceKind':1,'Equip':equip})
(w/'items.json').write_text(json.dumps(items,ensure_ascii=False));print('items',len(items))

# Native fixed-affix facts consumed by the item quality generator.
db.row_factory=sqlite3.Row
for m in re.finditer(r'INSERT INTO game_equip_extra \(([^)]+)\) VALUES\n(.*?)(?=\nON CONFLICT|;\n)',s32,re.S):
 db.execute('CREATE TABLE IF NOT EXISTS extra ('+','.join('"'+x.strip()+'"' for x in m[1].split(','))+')')
 db.execute('INSERT INTO extra VALUES '+m[2])
by_id={i['ID']:i for i in items}
for row in db.execute('SELECT * FROM extra ORDER BY arm_id,seq'):
 a=dict(row); item=by_id.get(a['arm_id'])
 if item is None: continue
 if a['prob']<100 or not a['attr_name']: item['Equip']['UnknownInnateProperties']=True
 else: item['Equip'].setdefault('Affixes',[]).append({'Attr':a['attr_id'],'Value':a['value'],'Mode':a['mode']})
# Exact pending migration105 fallback source. These IDs remain separate from
# all native tier Items; only the new rule's missing-slot logic can select them.
s105=(root/'server/migrations/000000_init-105.sql').read_text()
q=s105[s105.index('WITH fallback_candidates AS'):s105.index(' SELECT jsonb_agg(id ORDER BY id)')]+ ' SELECT id FROM fallback_candidates ORDER BY id'
q=q.replace('gamedata.ov_arm','gamedata_ov_arm').replace('a.index','a."index"').replace('game_equip_extra','extra').replace('SELECT FROM','SELECT 1 FROM')
fallbacks=[r[0] for r in db.execute(q)]
assert fallbacks==list(range(2601,2611))+list(range(2701,2708)),fallbacks
print('slot fallbacks',fallbacks,flush=True)
ids={i for p in pools for i in p['Items']} | set(fallbacks)
items=[i for i in items if i['ID'] in ids]
keys={(i['Equip']['ResourceLevel'],i['Equip']['Type']) for i in items}
options=[o for o in options if (o['Level'],o['Type']) in keys]
assert (len(pools),len(items),len(options))==(60,318,15458)
output=root/'server/internal/game/scene/testdata/rift_native_equipment_pools.json'
output.write_text(json.dumps({'Items':items,'Pools':pools,'Options':options,'SlotFallbacks':fallbacks},ensure_ascii=False,separators=(',',':'))+'\n')
print('wrote',output)
