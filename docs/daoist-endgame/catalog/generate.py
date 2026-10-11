#!/usr/bin/env python3
"""Re-enumerate original SQL equipment; generate deterministic frozen unique catalog.
No database, network, dependencies, migration execution, or original drop edits.
Run from any directory; --check performs byte-exact output verification.
"""
from __future__ import annotations
import argparse
import collections
import csv
import hashlib
import io
import json
import pathlib
import re

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
TARGET = ROOT / 'server/internal/domain'
TABLES = {'ov_arm', 'ov_desc', 'game_equipment', 'game_equip_extra', 'ov_card', 'ov_card_entry'}
ATTRS = {1,3,5,7,9,61,13,15,17,19,23,25,27,29,31,34,36,38,40,47,66,69,70,118}
PARTS = {1:'spear',2:'single_sword',4:'dual_sword',8:'dagger',16:'staff',32:'throw',101:'face',102:'hat',103:'necklace',105:'shield',106:'glove',107:'ring',108:'dress',109:'shoe',110:'bag',114:'ear',115:'bangle',116:'belt'}
DROP_KINDS = {1:'长枪',2:'长剑',3:'双手剑',4:'法杖',5:'双刃',6:'暗器',7:'盾牌',8:'服装',9:'头盔',10:'鞋子',11:'手套',12:'戒指',13:'背包',14:'项链',15:'面具',49:'耳环',51:'腰带'}
CLASSES = [('warrior','战士'),('swordman','剑客'),('stabber','刺客'),('druggist','药师'),('magician','术士')]
SEGMENTS = {1:'warrior',2:'swordsman',4:'assassin',8:'healer',16:'warlock',24:'sharedcaster',31:'allclasses',0:'unrestricted'}
EXPECTED = {'warrior':249,'swordsman':290,'assassin':269,'healer':237,'warlock':237,'sharedcaster':12,'allclasses':323,'unrestricted':2}
FIELDS = 'base_id,base_name,resource_level,level_requirement,drop_kind,professions,beginner_allowed,sex,base_affix_count,roll_pool_options,can_roll_affix,source_arm,source_desc'.split(',')
ROOTS = {
'warrior':'还丹 青藜 宝扇 石晶 战将 东海 守土 八荒'.split(),
'swordsman':'飞鹤 山石 花簪 妙音 画轴 白牡丹 铜壶 心镜 渡海'.split(),
'assassin':'莲舟 荷影 仙桃 天花 云母 萼绿 花雨 无痕 月隐'.split(),
'healer':'悬壶 通玄 七孔 茱萸 回春 济世 清身 解咒 光盾 护生'.split(),
'warlock':'纸驴 果老 寒山 红莲 寒冰 太极 八相 照劫 星轨'.split(),
'sharedcaster':'丹符 云箓 灵台 星灯'.split(),
'allclasses':'太清 玄津 归墟 望舒 长庚 云岫 松风 玉衡 沧浪 紫微 天门 流霞'.split(),
'unrestricted':'无涯 归一'.split(),
}
# Original memory echoes; none claim to be an original NPC's canonical property.
ACTIONS = [
('守夜','在漫长守夜里护住最后一线灯火'),('渡潮','涉过寒潮后仍记得彼岸的约定'),
('听雨','从檐下雨声中辨出将至的险途'),('照雪','借雪地微光辨清迷失者的归路'),
('问心','在幻境深处留住未改的本心'),('破雾','于迷雾尽头为同行者辟出道路'),
('回澜','在逆卷的浪头前重新站定'),('引星','以一缕星光标记夜行的方向'),
('鸣泉','听山泉复鸣而知生机尚在'),('镇岚','迎着山岚守住残破的关隘'),
('寻火','从冷灰中寻回不肯熄灭的火种'),('洗尘','洗去旅尘后重新踏上远行之路'),
('留霞','将将暮的霞光留给迟归之人'),('叩月','向月下空山问得静守的答案'),
('开云','拨开云幕时看见未曾坍塌的天门'),('赴约','跨过乱石赴一场无人催促的旧约'),
('惜春','把将尽的春意分给受伤的行客'),('踏霜','踏碎薄霜仍不惊醒山间的梦'),
('抱朴','在纷乱人声里收拢朴素的愿望'),('续灯','把将灭的灯焰递向下一位守望者'),
('折浪','从浪间折返带回失落的信物'),('望岳','越过群峰仍向故土回望'),
('沉璧','藏起一枚旧璧以换同行者平安'),('听松','凭松涛辨明山道上无人言说的警讯'),
('定风','在骤起风声中稳住摇晃的渡口'),('迎晓','把伤痕留在夜里而迎向天明'),
('藏锋','把锋芒收进一场不必发生的争斗'),('执炬','举起火炬领众人走过漫长暗巷'),
('解围','从重围缝隙中护送同伴离去'),('访泉','循着荒山泉脉寻找重生的契机'),
('磨镜','磨去心镜浮尘而见自身旧愿'),('叠影','借层叠树影掩护归途的脚步'),
('扶舟','在急流将倾时伸手扶正孤舟'),('缚雷','收束惊雷而不使余波伤及村落'),
('重誓','在断碑前重拾久远的誓言'),('守隙','守住黎明之前最脆弱的片刻'),
('寄远','把未尽的心愿寄给远行之人'),('归岚','带着故山的岚气回到旧日渡口'),
('拾翠','从荒芜庭院拾起一片新生的翠叶'),('停戈','在争斗将起时记得护生的本意'),
('探幽','循微光探入无人记载的幽径'),('越关','在暮色封关前带回同伴的讯息'),
('藏露','将清晨露意藏进干涸的行囊'),('醒梦','从层层旧梦中唤回同行者的名字'),
('听潮','听潮声起落而学会进退之度'),('澄江','让浑浊江水映出重返的星空'),
('寻鹤','循远鹤清音找到山外的去路'),('归鞘','在尘埃落定后把争胜之意收回'),
]
NOUNS = {1:'面',2:'冠',3:'坠',4:'刃',5:'盾',6:'护手',7:'戒',8:'衣',9:'履',10:'囊',11:'宝印',12:'灵饰',13:'长刃',14:'珰',15:'镯',16:'带',17:'兽饰',18:'兽饰',19:'兽饰',20:'徽章',21:'徽章',22:'徽章',23:'徽章',24:'徽章',25:'徽章'}
WEAPON_NOUNS = {1:'枪',2:'剑',4:'巨剑',8:'双刃',16:'杖',32:'飞针'}
PROFILES = {
'warrior': [('healthy_assault','physical_guard'),('desperate_assault','desperate_guard'),('elite_hunter','healthy_guard'),('execution','kill_mend'),('basic_focus','critical_focus'),('skill_focus','magic_guard')],
'swordsman': [('single_focus','critical_focus'),('execution','healthy_guard'),('area_focus','kill_mana'),('skill_focus','magic_guard'),('healthy_assault','basic_focus'),('elite_hunter','physical_guard')],
'assassin': [('ambush','invisible_focus'),('trap_expansion','skill_focus'),('execution','critical_focus'),('single_focus','kill_mana'),('basic_focus','healthy_assault'),('elite_hunter','desperate_guard')],
'healer': [('healing_grace','spell_economy'),('rescue_heal','healthy_guard'),('selfless_heal','magic_guard'),('healing_cost','swift_cast'),('holy_mastery','area_expansion'),('healing_grace','kill_mana')],
'warlock': [('fire_mastery','area_focus'),('ice_mastery','spell_economy'),('area_expansion','swift_cast'),('single_focus','skill_focus'),('critical_focus','kill_mana'),('desperate_assault','magic_guard')],
'sharedcaster': [('spell_economy','swift_cast'),('area_expansion','magic_guard'),('skill_focus','kill_mana')],
'allclasses': [('healthy_guard','physical_guard'),('desperate_guard','magic_guard'),('healthy_assault','kill_mend'),('elite_hunter','kill_mana')],
'unrestricted': [('healthy_guard','physical_guard'),('desperate_guard','magic_guard')],
}
STAT_PROFILES = {
'warrior':[(1,5,47),(1,34,13),(5,47,69),(1,19,34),(47,23,5),(1,17,34)],
'swordsman':[(1,9,23),(47,7,19),(1,36,34),(9,17,47),(1,23,34),(47,5,13)],
'assassin':[(7,9,23),(7,36,47),(47,23,34),(9,36,19),(7,47,34),(9,23,13)],
'healer':[(3,61,36),(3,34,17),(61,15,34),(3,36,61),(15,61,17),(3,61,34)],
'warlock':[(3,15,66),(3,61,36),(15,36,34),(3,15,19),(15,66,36),(3,17,34)],
'sharedcaster':[(3,61,36),(15,34,17),(3,15,36)],
'allclasses':[(5,34,13),(61,34,17),(5,34,19),(5,36,17)],
'unrestricted':[(5,34,13),(61,34,17)],
}

def split_sql(text):
    """Split only top-level commas; preserve PostgreSQL escaped quotes/decode()."""
    parts, start, depth, quoted, i = [], 0, 0, False, 0
    while i < len(text):
        c = text[i]
        if c == "'":
            if quoted and i+1 < len(text) and text[i+1] == "'": i += 2; continue
            quoted = not quoted
        elif not quoted:
            if c == '(': depth += 1
            elif c == ')': depth -= 1
            elif c == ',' and depth == 0: parts.append(text[start:i].strip()); start = i+1
        i += 1
    parts.append(text[start:].strip())
    def value(s):
        if s.startswith("'") and s.endswith("'"): return s[1:-1].replace("''", "'")
        if re.fullmatch(r'-?\d+', s): return int(s)
        return s
    return [value(x) for x in parts]


def read_tables():
    out = {k:[] for k in TABLES}
    header = re.compile(r'^INSERT INTO (?:(?:"gamedata"|gamedata)\.)?"?(\w+)"?\s*\((.*?)\) VALUES\s*$')
    table, columns = None, []
    pending, row_start = '', 0
    paths = sorted((ROOT/'server/migrations').glob('000000_init*.sql'), key=lambda p: int(re.search(r'-(\d+)\.sql$', p.name).group(1)) if '-' in p.name else 0)
    for path in paths:
        with path.open(encoding='utf-8') as f:
            for line_no,line in enumerate(f,1):
                if line.startswith('INSERT INTO '):
                    match = header.match(line.rstrip())
                    table = match.group(1) if match and match.group(1) in TABLES else None
                    if table: columns = [s.strip().strip('"') for s in match.group(2).split(',')]
                elif table and (pending or line.lstrip().startswith('(')):
                    if not pending: row_start = line_no
                    pending += line
                    raw = pending.strip().rstrip(',;')
                    if not raw.endswith(')') or raw.count("'") % 2: continue
                    pending = ''
                    values = split_sql(raw[1:-1])
                    assert len(values) == len(columns), (path,line_no,len(values),len(columns))
                    row = dict(zip(columns,values)); row['_source'] = f'{path.relative_to(ROOT)}:{row_start}'
                    out[table].append(row)
                elif table and line.startswith('ON CONFLICT'): table = None
    return out


def first_index(rows, key='index'):
    result = {}
    for row in sorted(rows, key=lambda x:x.get('row_no',0)):
        result.setdefault(row[key],row)
    return result


def enumerate_eligible(tables):
    arms = first_index(tables['ov_arm']); descs = first_index(tables['ov_desc'])
    equips = first_index(tables['game_equipment'],'id')
    cards = {r['row_no']:r for r in tables['ov_card'] if (5001<=r['index']<=5997 or 13141<=r['index']<=13191) and r['category']==0 and r['fun_type']==0}
    entries = [e for e in tables['ov_card_entry'] if e['row_no'] in cards and e['op_type']==1 and e['prob']==100 and e['mode'] in (0,1) and e['attr_id'] in ATTRS]
    pools = collections.Counter()
    for e in entries:
        c = cards[e['row_no']]
        for level in range(max(1,c['min_level']),min(17,c['max_level'])+1):
            for typ,part in PARTS.items():
                if c[part]: pools[level,typ] += 1
    affixes = collections.Counter(e['arm_id'] for e in tables['game_equip_extra'] if e['prob']>=100 and e['attr_name'])
    rows, sources = [], {}
    for iid,a in sorted(arms.items()):
        if not iid or a['no_type_drop'] != 0 or iid not in equips: continue
        assert iid in descs, f'eligible base {iid} lacks original descriptor'
        d = descs[iid]; e = equips[iid]
        mask = sum(1<<i for i,(key,_) in enumerate(CLASSES) if a[key] or a['sr_'+key])
        kind = DROP_KINDS.get(a['query_type'],'') or {114:'耳环',116:'腰带'}.get(a['position_'],'')
        count = pools[a['level'],d['type']]
        row = dict(zip(FIELDS,[iid,a['name'],a['level'],a['level_need'],kind,'|'.join(n for i,(_,n) in enumerate(CLASSES) if mask & 1<<i),bool(a['newbie']),a['sex'],affixes[iid],count,bool(count),a['_source'],d['_source']]))
        rows.append(row); sources[iid] = (a,d,e,mask)
    return rows,sources


def trait(kind, seed):
    power = 400+(seed%9)*100
    if kind in {'spell_economy','swift_cast','area_expansion','trap_expansion','healing_cost'}: power = 500+(seed%11)*100
    if kind in {'kill_mend','kill_mana'}: power = 100+(seed%4)*100
    threshold = 0
    if kind.startswith('healthy_'): threshold = 70
    if kind.startswith('desperate_'): threshold = 35
    if kind in {'execution','rescue_heal'}: threshold = 30
    if kind == 'ambush': threshold = 80
    return dict(Kind=kind,PowerBP=power,ThresholdPct=threshold)


def generate(rows,sources):
    groups = collections.defaultdict(list); ordinals = collections.Counter(); names = set(); packages = set()
    for row in rows:
        iid = row['base_id']; a,d,e,mask = sources[iid]; segment = SEGMENTS[mask]
        slot = e['slot']; noun = WEAPON_NOUNS.get(d['type'],NOUNS.get(slot,'灵器'))
        # Stable source ID order, distinct original root/action/slot combinations.
        order = ordinals[segment,noun]; ordinals[segment,noun] += 1
        roots = ROOTS[segment]; root = roots[order%len(roots)]
        action,story = ACTIONS[order//len(roots)]
        name = root+action+noun
        assert name not in names, name
        names.add(name)
        seed = int(hashlib.sha256(str(iid).encode()).hexdigest()[:8],16)
        p = seed%len(PROFILES[segment]); kinds = PROFILES[segment][p]
        stat_ids = STAT_PROFILES[segment][p]
        rank = min(80,max(1,row['level_requirement']))
        stats = []
        for j,attr in enumerate(stat_ids):
            value = 3+rank//5+(seed>>(j*5))%7
            if attr in (34,36): value = 20+rank*3+(seed>>(j*4))%31
            elif attr in (23,66): value = 100+((seed>>(j*4))%7)*25
            elif attr in (47,15): value = 5+rank//3+(seed>>(j*4))%9
            stats.append(dict(Attr=attr,Value=value,Mode=0))
        lore = f'演劫残忆中，行者以{root}为记，{story}。这件{noun}承载那一刻的心愿。'
        item = dict(ID=iid,BaseID=iid,BaseName=row['base_name'],Name=name,Lore=lore,LoreRoot=root,Role=segment,Professions=mask,BeginnerAllowed=row['beginner_allowed'],Unrestricted=not mask and not row['beginner_allowed'],Sex=row['sex'],Slot=slot,LevelRequirement=row['level_requirement'],ResourceLevel=row['resource_level'],NativeDropEligible=bool(row['drop_kind']),LegacyAffixPoolReady=row['can_roll_affix'],Affixes=stats,Traits=[trait(kinds[0],seed),trait(kinds[1],seed>>8)],SourceArm=row['source_arm'],SourceDesc=row['source_desc'])
        signature = lambda: json.dumps([item['Affixes'],item['Traits']],sort_keys=True)
        while signature() in packages: item['Affixes'][-1]['Value'] += 1
        packages.add(signature())
        groups[segment].append(item)
    return groups


def encode_csv(rows):
    stream=io.StringIO(newline=''); writer=csv.DictWriter(stream,fieldnames=FIELDS,lineterminator='\n'); writer.writeheader(); writer.writerows(rows)
    return stream.getvalue().encode()


def validate(groups,rows):
    items=[i for group in groups.values() for i in group]
    assert len(items)==1619 and len({i['ID'] for i in items})==1619
    assert {k:len(v) for k,v in groups.items()} == EXPECTED
    assert sum(i['NativeDropEligible'] for i in items)==885
    assert sum(i['LegacyAffixPoolReady'] for i in items)==298
    assert all(len(i['Affixes'])==3 and len(i['Traits'])==2 for i in items)
    assert all(len({a['Attr'] for a in i['Affixes']})==3 and all(a['Attr'] in ATTRS and a['Mode']==0 and a['Value']>0 for a in i['Affixes']) for i in items)
    traits={t['Kind'] for i in items for t in i['Traits']}
    assert len(traits)==28, (len(traits),traits)
    assert all(i['Name']!=i['BaseName'] and len(i['Name'])<=12 for i in items)
    assert len({i['Lore'] for i in items})==1619
    assert len({json.dumps([i['Affixes'],i['Traits']],sort_keys=True) for i in items})==1619
    assert all(i['ID']==i['BaseID'] for i in items)


def main():
    parser=argparse.ArgumentParser(description=__doc__); parser.add_argument('--check',action='store_true'); args=parser.parse_args()
    rows,sources=enumerate_eligible(read_tables())
    # This independently reconstructed list must match the recovered source audit.
    with (HERE/'eligible.csv').open(encoding='utf-8',newline='') as f: recovered=list(csv.DictReader(f))
    assert len(rows)==len(recovered), (len(rows),len(recovered))
    for actual,old in zip(rows,recovered):
        assert {k:str(v) for k,v in actual.items()}==old, (actual,old)
    groups=generate(rows,sources); validate(groups,rows)
    outputs={TARGET/f'unique_catalog_{key}.json':(json.dumps(group,ensure_ascii=False,separators=(',',':'))+'\n').encode() for key,group in sorted(groups.items())}
    assert all(len(data)<500000 for data in outputs.values())
    summary=dict(schema=1,total=len(rows),native_drop_kind=885,conversion_only=734,legacy_affix_pool_ready=298,class_counts=EXPECTED,sex_counts=dict(collections.Counter(str(r['sex']) for r in rows)),catalog_sha256={p.name:hashlib.sha256(data).hexdigest() for p,data in outputs.items()},source_csv_sha256=hashlib.sha256(encode_csv(rows)).hexdigest(),max_catalog_bytes=max(map(len,outputs.values())),unique_name_count=1619,unique_lore_count=1619,unique_effect_package_count=1619,executable_trait_kind_count=28)
    outputs[HERE/'coverage.json']=(json.dumps(summary,ensure_ascii=False,indent=2)+'\n').encode()
    for path,data in outputs.items():
        if args.check:
            assert path.exists() and path.read_bytes()==data, f'stale or missing generated file: {path}'
        else: path.write_bytes(data)
    print(json.dumps(summary,ensure_ascii=False,indent=2))

if __name__=='__main__': main()
