#!/usr/bin/env python3
"""Read native init source only; fixture for focused PostgreSQL catalog tests.
No network, runtime configuration, assets, or production database is read/written.
"""
import re,json,csv,sys
from pathlib import Path
root=Path(__file__).resolve().parents[2]
names={'game_equipment','game_items','game_pets','ov_petgrow','game_rack_goods','game_item_saddles','game_item_saddle_pets','game_item_pet_eggs','ov_equip','ov_item','ov_refine','ov_petlev','game_pet_profiles','game_item_avatar_fusions','ov_avatararm','ov_desc'}
tables={x:[] for x in names}
def fields(s):
 out=[]; start=0; depth=0;quote=False;i=0
 while i<len(s):
  c=s[i]
  if quote:
   if c=="'":
    if i+1<len(s) and s[i+1]=="'": i+=1
    else: quote=False
  elif c=="'": quote=True
  elif c in '([':depth+=1
  elif c in ')]':depth-=1
  elif c==',' and depth==0:out.append(s[start:i].strip());start=i+1
  i+=1
 out.append(s[start:].strip())
 return out
def val(s):
 if re.fullmatch(r'-?[0-9]+',s):return int(s)
 if s.upper()=='TRUE':return True
 if s.upper()=='FALSE':return False
 if s.startswith("'") and s.endswith("'"):return s[1:-1].replace("''", "'")
 return s
# Init fragments are one SQL stream; some VALUES lists cross file boundaries.
# Parsing each file alone silently omits the first records of the next fragment.
files=sorted((root/'server/migrations').glob('000000_init-*.sql'),key=lambda p:int(p.stem.split('-')[-1]))
for s in ['\n'.join(f.read_text() for f in files)]:
 pattern=r'INSERT INTO\s+((?:"?\w+"?\.)?"?\w+"?)\s*\(([^;]*?)\)\s*VALUES\s*'
 for m in re.finditer(pattern,s):
  name=m.group(1).split('.')[-1].strip('"')
  if name not in names:continue
  cols=[x.strip().strip('"') for x in m.group(2).split(',')]
  pos=m.end()
  while pos<len(s):
   while pos<len(s) and (s[pos].isspace() or s[pos]==','):pos+=1
   if pos>=len(s) or s[pos]!='(':break
   start=pos+1;quote=False;depth=1;pos+=1
   while depth:
    c=s[pos]
    if quote:
     if c=="'":
      if s[pos+1]=="'":pos+=1
      else:quote=False
    elif c=="'":quote=True
    elif c in '([':depth+=1
    elif c in ')]':depth-=1
    pos+=1
   vals=[val(x) for x in fields(s[start:pos-1])]
   assert len(vals)==len(cols),(name,len(vals),len(cols))
   tables[name].append(dict(zip(cols,vals)))
# Reconstruct the existing catalog from its native initialization consumers.
items={r['index']: {'id':r['index'],'name':r['name']} for r in tables['ov_item']}
items.update({r['id']:{'id':r['id'],'name':r['name']} for r in tables['game_items']+tables['game_equipment'] if isinstance(r['id'],int)})
pets={r['pet_id']:r for r in reversed(tables['ov_petgrow']) if r['pet_id']>0}
goods={r['item_id']:r for r in tables['game_rack_goods']}
pet_goods=[]
for k,p in pets.items():
 if p['capture_level']==0 and p['starve_add1']!=0:
  goods[90000000+k]=dict(item_id=90000000+k,category_id=7,price=1,quantity=1,flags=0,remain=-1,part='',active=True)
  pet_goods.append(dict(item_id=90000000+k,pet_id=k))
for k in {r[col] for r in tables['ov_refine'] for col in ['mat_1id','mat_2id','mat_3id','protect_item'] if r[col]>0}:
 goods[k]=dict(item_id=k,category_id=8,price=1,quantity=1,flags=0,remain=-1,part='',active=True)
goods[3633]['active']=False # init-40, already retired QQ egg; never reactivate.
profiles=[]
for r in tables['game_pet_profiles']:
 profiles.append(dict(pet_id=r['pet_id'],blessing_slots=r['blessing_slots'],initial_skill_names=re.findall(r"'([^']+)'",r['initial_skill_names']),forbidden_skill_names=re.findall(r"'([^']+)'",r['forbidden_skill_names'])))
petcols=['row_no','pet_id','name','capture_level','starve_add1','pet_circle','horse_base_spd']+['init_skills%d_id'%n for n in range(1,7)]
fixture=dict(items=list(items.values()),equipment_ids=sorted({r['id'] for r in tables['game_equipment']}),pets=[{k:r[k] for k in petcols} for r in pets.values()],goods=list(goods.values()),pet_goods=pet_goods,profiles=profiles,
 saddles=tables['game_item_saddles'],saddle_pets=tables['game_item_saddle_pets'],eggs=tables['game_item_pet_eggs'],avatars=tables['ov_avatararm'],fusions=tables['game_item_avatar_fusions'])
print(json.dumps(fixture,ensure_ascii=False))
