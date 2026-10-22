#!/usr/bin/env python3
"""Offline native-gate proof, Python standard library only (SciPy optional accelerator).
Usage: python3 verify_offline.py --fixture native-gate-offline-fixture.json.gz \
 --manifest all-native-gates-manifest.json --repairs approved-recovery-rows.json
No network, database, mutable repository, or separately downloaded assets are used.
"""
import argparse,base64,collections,gzip,hashlib,json,pathlib,struct,time
try:
 import numpy as np
 from scipy.ndimage import label
except ImportError:np=None

def sha(b):return hashlib.sha256(b).hexdigest()
def parse_prc(b):
 assert b[:8]==b'QQF\x1aPROC';n=struct.unpack_from('<i',b,16)[0];assert len(b)>=20+n*396;out=collections.defaultdict(list)
 for i in range(n):
  o=20+i*396;pid=struct.unpack_from('<i',b,o)[0];pn=struct.unpack_from('<i',b,o+8)[0]
  if 3<=pn<=16:
   xs=struct.unpack_from('<'+'i'*pn,b,o+44);ys=struct.unpack_from('<'+'i'*pn,b,o+108);out[pid].append([v for p in zip(xs,ys) for v in p])
 return out

def parse_link(b):
 out=[];r=None
 for line in b.decode('gbk').splitlines():
  p=line.strip().split()
  if len(p)<2 or p[0].startswith('#'):continue
  k=p[0]
  if k=='proc_id':
   if r is not None:out.append(r)
   r=dict(proc_id=int(p[1]),map_to=0,to_x=0,to_y=0,to_dir=0,back_x=0,back_y=0,back_dir=0,script='')
  elif r is not None:
   if k in ('map_to','map_to_dir','map_back_dir'):r[{'map_to':'map_to','map_to_dir':'to_dir','map_back_dir':'back_dir'}[k]]=int(p[1])
   elif k in ('map_to_pos','map_back_pos') and len(p)>=3:
    prefix='to' if k=='map_to_pos' else 'back';r[prefix+'_x'],r[prefix+'_y']=map(int,p[1:3])
   elif k in ('script','cmd'):r['script']=' '.join(p[1:])
 if r is not None:out.append(r)
 return out

def inside(x,y,poly):
 p=list(zip(poly[::2],poly[1::2]));odd=False
 for i,(ax,ay) in enumerate(p):
  bx,by=p[(i+1)%len(p)]
  if (ay>y)!=(by>y) and x<(bx-ax)*(y-ay)/(by-ay)+ax:odd=not odd
 return odd

def component(raw,start):
 assert raw[:8]==b'QQF\x1aMASK';w,h,cw,ch=struct.unpack_from('<4I',raw,16);assert len(raw)==32+w*h and min(w,h,cw,ch)>0;cells=raw[32:];seed=int(start[1]//ch)*w+int(start[0]//cw);assert not cells[seed]&128
 if np is not None:
  labels,n=label((np.frombuffer(cells,dtype=np.uint8).reshape(h,w)&128)==0);selected=int(labels.flat[seed]);hit=lambda i:int(labels.flat[i])==selected;count=int(np.sum(labels==selected))
 else:
  reached=bytearray(len(cells));reached[seed]=1;q=collections.deque([seed]);count=0
  while q:
   i=q.popleft();count+=1
   for j in (i-1 if i%w else -1,i+1 if i%w<w-1 else -1,i-w if i>=w else -1,i+w if i+w<len(cells) else -1):
    if j>=0 and not reached[j] and not cells[j]&128:reached[j]=1;q.append(j)
  hit=lambda i:bool(reached[i])
 def point_ok(x,y):return 0<=x<w*cw and 0<=y<h*ch and hit(int(y//ch)*w+int(x//cw))
 return point_ok,count*cw*ch

def main():
 root=pathlib.Path(__file__).parent;p=argparse.ArgumentParser(description=__doc__);p.add_argument('--fixture',type=pathlib.Path,default=root/'native-gate-offline-fixture.json.gz');p.add_argument('--manifest',type=pathlib.Path,default=root/'all-native-gates-manifest.json');p.add_argument('--repairs',type=pathlib.Path,default=root/'approved-recovery-rows.json');p.add_argument('--output',type=pathlib.Path,default=root/'offline-verification.json');a=p.parse_args();start=time.time()
 fixture=json.loads(gzip.decompress(a.fixture.read_bytes()));manifest=json.loads(a.manifest.read_text());repairs=json.loads(a.repairs.read_text());mi={(r['map_id'],r['proc_id']):r for r in manifest};ri={(r['map_id'],r['proc_id']):r for r in repairs};assert len(mi)==len(manifest)==354;assert len(ri)==len(repairs)==67;seen=set();seen_repairs=set();maps=[]
 for m in fixture['maps']:
  mid=m['map_id'];prc=base64.b64decode(m['prc_base64']);link=base64.b64decode(m['link_base64']);mask=gzip.decompress(base64.b64decode(m['mask_gzip_base64']));assert sha(prc)==m['prc_sha256'];assert sha(link)==m['link_sha256'];assert sha(mask)==m['mask_sha256'];polys=parse_prc(prc);links=parse_link(link);point_ok,area=component(mask,m['selected_anchor_points'][0]);assert area==m['selected_walkable_area'],mid;assert all(point_ok(*xy) for xy in m['selected_anchor_points']),mid;server={r['proc_id']:r for r in m['server_rows_before_repair']};count=repair_count=0
  for r in links:
   if r['map_to']<=0 or r['script']:continue
   key=(mid,r['proc_id']);assert key not in seen;seen.add(key);row=mi[key];source=server[r['proc_id']];assert len(polys[r['proc_id']])==1;poly=polys[r['proc_id']][0];assert poly==row['poly'];assert row['source_hash']==sha(prc) and row['link_hash']==sha(link) and row['mask_hash']==sha(mask);assert not source['script'];assert all(source[k]==r[k] for k in ('proc_id','map_to','to_x','to_y','to_dir','back_x','back_y','back_dir'));xy=row['point'];assert inside(xy['X'],xy['Y'],poly) and point_ok(xy['X'],xy['Y']);count+=1
   if source['poly'] is None:
    fix=ri[key];assert fix['verifiedPoly']==poly;assert all(fix[k]==source[k] for k in ('proc_id','map_to','to_x','to_y','to_dir','back_x','back_y','back_dir'));assert fix['nativePRCSha256']==sha(prc) and fix['nativeLINKSha256']==sha(link);seen_repairs.add(key);repair_count+=1
   else:assert source['poly']==poly,key
  assert count==len(server);maps.append({'map_id':mid,'ordinary_gates':count,'recovered_null_polygons':repair_count,'passed':True})
 assert seen==set(mi) and seen_repairs==set(ri);assert len(maps)==168
 result={'passed':True,'maps':168,'ordinary_gates':354,'recovered_null_polygons':67,'recovered_maps':sum(m['recovered_null_polygons']>0 for m in maps),'unchanged_maps':sum(m['recovered_null_polygons']==0 for m in maps),'excluded_maps':0,'source_fixture_sha256':sha(a.fixture.read_bytes()),'seconds':round(time.time()-start,3),'accelerator':'scipy' if np is not None else 'standard-library BFS','details':maps};a.output.write_text(json.dumps(result,ensure_ascii=False,indent=2));print(json.dumps({k:v for k,v in result.items() if k!='details'}))
if __name__=='__main__':main()
