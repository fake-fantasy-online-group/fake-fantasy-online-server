#!/usr/bin/env python3
"""Authored cube-only progression. Native template identities, not legacy recipes."""
import json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
items=[]; recipes=[]
chains={
 ('light',False,'physical'):[20609,20611,20613,20615,20617,20621,20625,21905],
 ('light',False,'magic'):[20609,20611,20613,20615,20618,20622,20626,21906],
 ('light',True,'physical'):[20610,20612,20614,20616,20619,20623,20627,21905],
 ('light',True,'magic'):[20610,20612,20614,20616,20620,20624,20628,21906],
 ('dark',False,'physical'):[22863,22864,22865,22866,22867,22869,22871,21907],
 ('dark',False,'magic'):[22863,22864,22865,22866,22868,22870,22872,21908],
 ('dark',True,'physical'):[23669,23670,23671,23672,23673,23675,23677,21907],
 ('dark',True,'magic'):[23669,23670,23671,23672,23674,23676,23678,21908],
}
costs=[20,60,160,400,800,1500,2500,4000]
def mats(units):
 return [dict(ItemID=id,Count=n) for id,n in [(25029,units//25),(25028,units%25//5),(25027,units%5)] if n]
seen=set(); edges=set()
def recipe(source,target,op,materials,mask=0,outputs=None):
 if op!='wing.feather' and (source,target) in edges:return
 edges.add((source,target));d=dict(ID=900001+len(recipes),Source=source,Target=target,Operation=op,Materials=materials)
 if mask:d['ProfessionMask']=mask
 if outputs:d['Outputs']=outputs
 recipes.append(d)
for (series,ascended,variant),chain in chains.items():
 for index,id in enumerate(chain):
  if id not in seen:
   seen.add(id);items.append(dict(Item=id,Tier=index+1,Series=series,Variant=variant if index>=4 else '',Ascended=ascended or index==7))
  if index:
   recipe(chain[index-1],id,'wing.upgrade',mats(costs[index]),(7 if variant=='physical' else 24) if index==4 else 0)
for (series,ascended,variant),chain in chains.items():
 target=chains[('dark' if series=='light' else 'light',ascended,variant)]
 for source,dest in zip(chain,target):
  recipe(source,dest,'wing.change',[dict(ItemID=25029,Count=8),dict(ItemID=11682 if series=='light' else 25027,Count=1)])
# Only common white feathers exchange denominations. No common-to-rare minting.
for low,high in [(25027,25028),(25028,25029)]:
 recipe(0,0,'wing.feather',[dict(ItemID=low,Count=5)],outputs=[dict(ItemID=high,Count=1)])
 recipe(0,0,'wing.feather',[dict(ItemID=high,Count=1)],outputs=[dict(ItemID=low,Count=5)])
cfg=dict(Version=1,Entrance=dict(mapId=7,npcId=96,npc='幻翼域使者',script='N2619LC'),Quest=1900030002,Starter=20609,Items=items,FeatherIDs=[25027,25028,25029,11682],Recipes=recipes)
(root/'config/cube-wing-progression-v1.json').write_text(json.dumps(cfg,ensure_ascii=False,indent=2)+'\n')
print(f'{len(items)} native wings; {len(recipes)} cube recipes')
