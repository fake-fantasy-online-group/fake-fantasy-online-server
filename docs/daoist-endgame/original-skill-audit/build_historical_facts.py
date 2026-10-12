import json
from pathlib import Path
P=Path('/workspace/shared/daoist-endgame/original-skill-audit')
r=json.load(open('/workspace/shared/server-development/docs/daoist-endgame/v5/skill-original-ranks.json'))
first={x['ID']:x for x in r if x['Level']==1}
source_by_prof={1:'https://fo.17173.com/character/character_2.html',2:'https://fo.17173.com/character/character_1.html',3:'https://fo.17173.com/character/character_3.html',4:'https://fo.17173.com/character/character_4.html',5:'https://fo.17173.com/character/character_5.html'}
# Manual factual transcription of classification in historical table, not a claim
# that target includes enemies or that every static-table value was live.
areas={10005,10006,10007,10021,10022,10028,10029,10030,10121,10125,10127,10128,10206,10213,10214,10215,10216,10217,10218,10225,10228,10304,10307,10308,10311,10312,10313,10315,10321,10326,10327,10402,10405,10410,10412,10413}
passives={10001,10008,10012,10014,10019,10020,10024,10025,10026,10101,10106,10108,10110,10112,10118,10120,10201,10202,10203,10204,10207,10220,10222,10226,10229,10230,10314,10415,10416,10422,10423,10424,10426,10427,10429,10430,10431}
dmg={10002:(130,5),10003:(180,10),10004:(200,10),10005:(95,5),10006:(135,5),10007:(175,10),10013:(100,0),10015:(165,4),10017:(105,5),10021:(145,5),10022:(155,5),10023:(160,10),10027:(100,0),10102:(130,3),10103:(145,0),10104:(150,5),10113:(155,0),10111:(145,0),10116:(175,10),10117:(95,5),10119:(150,5),10121:(130,None),10122:(160,0),10123:(165,10),10125:(150,10),10126:(100,0),10205:(150,5),10206:(145,5),10209:(300,20),10210:(400,10),10213:(200,10),10214:(130,0),10215:(150,0),10216:(170,0),10217:(185,5),10219:(130,0),10221:(130,5),10223:(160,5),10225:(175,8),10312:(280,10),10313:(300,10),10317:(240,10),10319:(210,15),10326:(260,5),10401:(208,8),10402:(195,5),10405:(250,6),10408:(300,15),10409:(100,5),10410:(175,5),10411:(250,8),10412:(200,8),10414:(350,10)}
radius={10006:150,10007:150,10022:150,10028:100,10029:100,10121:100,10125:150,10127:100,10128:100,10206:200,10213:100,10214:100,10215:100,10216:100,10217:100,10225:200,10304:300,10307:200,10308:150,10311:150,10312:150,10313:150,10315:150,10321:200,10326:150,10327:300,10402:100,10405:150,10410:100,10412:150,10413:200}
# Independently transcribed from the five web pages. Current repo is used only
# for the ID join; neither name nor level defaults to current repo data.
historical={
1:"""连环枪法10 穿刺之枪10 破防之枪15 冲锋15 破甲20 飞翔之枪20 破法之枪25 全力投掷25 震地之枪30 破魔30 裂空之枪36 震空枪气36 流虹之枪43 维护43 心眼之枪51 野战八荒51 石化之枪60 战神之枪60 荆棘之枪70 死翔之枪70 枪法修炼15 意志20 重击25 强健30 战吼36 抗魔43 震慑51 缴械60 怒火杀意70 狂暴术80""",
2:"""罡气剑10 真空剑10 通背15 风斩15 幻影剑20 金缚剑20 饮血剑25 噬魂剑25 血影30 魔光30 封法剑36 旋风剑36 锁灵剑43 混沌剑43 狮心剑51 陨星剑51 火字诀60 山字诀60 龙杀剑70 血雨剑70 剑术修炼15 心镜20 强击25 盾防修炼30 挑衅36 铁城神盾43 煞气51 致残60 雷霆之怒70 仙风云体80""",
3:"""击晕10 狙击10 双刃修炼15 暗器修炼15 反击术20 连续投掷20 隐身术25 暗器回收25 进阶隐身30 雷火弹30 多重投掷36 背刺36 进阶背刺43 烟雾弹43 偷窃术51 环形爆发51 暗杀术60 爆发冲击60 进阶暗杀70 阴影跳跃70 淬毒术15 侦测陷阱20 陷阱：爆弹25 陷阱：毒雾30 解除陷阱36 陷阱：封印43 回避陷阱51 陷阱：冻气60 陷阱：新星70 回避魔法80""",
4:"""治愈术10 祝福术15 炼丹士15 防御术20 净化术20 复法术25 衰弱术25 复活术30 光盾术30 群疗术36 加速术36 减速术43 清身术43 回春术51 针刺护体51 解咒术60 枷锁术60 光之奇迹70 瞬击术70 神圣庇护80 光弹术10 术法精研15 圣光术20 神术：鹰眼25 神术：集中30 驱魔术36 神威术51 神术：释能51 神术：瞬发60 天罚70""",
5:"""火弹术10 冰箭术10 火系掌握15 冰系掌握15 火球术20 冰爆术20 燃烧25 冰封25 燃烧强化30 火盾术36 冰封强化36 冰盾术36 风焰术43 暴雪术43 高热风暴51 淬冰风暴51 红莲之魂60 寒冰之魂60 陨石术70 绝对零度70 魔法精研15 秘术：集中20 魔法修炼25 秘术：消除36 秘术：释能43 魔镜术51 秘术：瞬发60 虚空70 时之封印80"""}
import re
repo_name_to_id={x['Name']:sid for sid,x in first.items()}
aliases={'偷窃术':10212,'回避陷阱':10229,'回避魔法':10230,'炼丹士':11005}
rows=[]
for prof,items in historical.items():
 for item in items.split():
  m=re.fullmatch(r'(.+?)([0-9]+)',item); name,lv=m[1],int(m[2])
  sid=aliases.get(name,repo_name_to_id.get(name))
  assert sid is not None,name
  q={'id_joined_by_name':sid,'historical_name':name,'level':lv,'source_kind':'area' if sid in areas else 'passive' if sid in passives or sid==11005 else 'charged' if sid==10105 else 'target','source_url':source_by_prof[prof],'source_version':'undated legacy table; demonstrably retains pre-2005-12-22 values','authority':'historical reference page; not authenticated Tencent raw data','id_mapping_is_inference':sid in {10212,10229,10230},'web_name_level_kind_verified':True}
  if sid in dmg:
   q['damage_percent_first'],q['damage_percent_add_per_rank']=dmg[sid]
   if q['damage_percent_add_per_rank']==0:q['damage_percent_add_per_rank']=None
   q['web_damage_verified']=True
  if sid in radius:q['radius']=radius[sid];q['web_geometry_verified']=True
  if sid in [10005,10021]:q['rectangle']=[50,150] if sid==10005 else [80,250];q['web_geometry_verified']=True
  if sid in [10006,10206]:q['cone_angle']=120
  rows.append(q)
assert len(rows)==149 and len({x['id_joined_by_name'] for x in rows})==149
obj={'checked_at':'2026-10-12','purpose':'Read-only historical evidence for joining; never apply as blanket gameplay overrides. Source target labels include self/friendly actions; Null damage growth means not quantified in source, not proven constant mechanics. Damage/radius facts are concise factual transcription, not full page reproduction.','baseline':'All facts here are from linked historical reference pages, NOT copied from current runtime values. IDs are joined from repository by name, with explicit alias hypotheses.','rows':rows,'official_deltas':[{'date':'2005-12-22','source_url':'https://fo.17173.com/gengxin/20051222.shtml','publisher_label':'官方','skills':[10029,10128],'changes':{'radius':150,'effect':'stun','seconds_first':5,'seconds_add_per_rank':1}},{'date':'2005-12-22','source_url':'https://fo.17173.com/gengxin/20051222.shtml','publisher_label':'官方','skills':[10209,10210],'changes':{'hit_percent':100}},{'date':'2005-12-22','source_url':'https://fo.17173.com/gengxin/20051222.shtml','publisher_label':'官方','skills':[10216],'changes':{'level':45,'requires_skill_book':False}}]}
(P/'historical-facts.json').write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n')
print(len(rows),'historical entries written; static mage omits 10432; life included')
