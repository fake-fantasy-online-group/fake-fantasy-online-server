#!/usr/bin/env python3
"""Re-enumerate original SQL equipment; compile individually authored unique catalog.
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
# Authored content is data, never a root/action/slot product. Source metadata is
# still derived from the original SQL so writers cannot change item identities.
AUTHORED = HERE / 'authored'
TRAITS = set("healthy_assault desperate_assault execution ambush elite_hunter critical_focus physical_guard magic_guard desperate_guard healthy_guard healing_grace rescue_heal selfless_heal healing_cost skill_economy spell_economy swift_cast area_expansion trap_expansion fire_mastery ice_mastery holy_mastery single_focus area_focus invisible_focus kill_mend kill_mana basic_focus skill_focus".split())
AUTHOR_REQUIRED = {'BaseID','Name','Lore','LoreRoot','Identity','Benefit','Tradeoff','PowerTier','Affixes','Traits'}
AUTHOR_OPTIONAL = {'ReservedSkills','VariableAffixes','LocalEnhancements'}
POWER_TIERS = {'transition','highvalue','builddefining','godlike'}
MIN_BENEFICIAL_EXTRA_AFFIXES = 5
TRAIT_CLASSES = {**dict.fromkeys(('healing_grace','rescue_heal','selfless_heal','healing_cost','holy_mastery'),8),**dict.fromkeys(('fire_mastery','ice_mastery'),16),**dict.fromkeys(('swift_cast','spell_economy'),24),**dict.fromkeys(('trap_expansion','invisible_focus'),4)}


def supported_endgame_affix(attr, value, mode):
    """Match SupportedEndgameAffix; bools/floats are not integer stat data."""
    return (all(type(v) is int for v in (attr,value,mode))
            and attr in ATTRS - {38,40} and mode in (0,1)
            and -2147483648 <= value <= 2147483647 and value != 0
            and not (attr in (23,66,69,70,118) and mode == 1)
            and not (attr == 25 and mode == 0))


def validate_extra_affixes(rows, label, *, unique=False, keys=('Attr','Value','Mode')):
    """Count only distinct, executable, beneficial extras, never other sources.

    Native base stats, saved yellow rolls, socket runes, conditional traits and
    unimplemented skill-level definitions cannot satisfy this contract. Curses
    remain explicit additional lines. There is no legacy six-line ceiling.
    """
    assert isinstance(rows,list), (label,'extra affixes must be an array')
    seen=set(); percent_attack=False; beneficial=0
    for a in rows:
        assert isinstance(a,dict) and set(a)==set(keys), (label,'invalid affix shape',a)
        attr,value,mode=(a[k] for k in keys)
        assert supported_endgame_affix(attr,value,mode), (label,'unsupported extra affix',a)
        assert attr not in seen, (label,'duplicate extra Attr',attr)
        seen.add(attr)
        # These three percentage forms all enter the same pct.atk bucket.
        if mode == 1 and attr in (29,31,47):
            assert not percent_attack, (label,'duplicate percentage attack effect')
            percent_attack=True
        if unique:
            # Keep every line nonzero and int32-safe at all stored 80–120%
            # potency values, including inverse-scaled curses. Do not reroll.
            assert abs(value)*8000//10000 > 0, (label,'extra affix vanishes at minimum potency',a)
            assert abs(value)*12000//10000 <= 2147483647, (label,'extra affix overflows at maximum potency',a)
        if (value < 0 if attr == 25 else value > 0):
            beneficial += 1
    assert beneficial >= MIN_BENEFICIAL_EXTRA_AFFIXES, (label,'at least five beneficial distinct extra attributes required',beneficial)


def validate_roll_model(affixes, variables, local, label, *, lower=False, intrinsic=None):
    def key(k): return k[0].lower()+k[1:] if lower else k
    akey,vkey,mkey=map(key,('Attr','Value','Mode'))
    rows={a[akey]:a for a in affixes}; seen=set(); count=len(variables)
    assert isinstance(variables,list) and isinstance(local,list) and len(local)<=2, (label,'invalid roll metadata')
    for v in variables:
        assert isinstance(v,dict) and set(v)==set(map(key,('Attr','Min','Max'))), (label,'invalid variable shape')
        attr,lo,hi=(v[key(k)] for k in ('Attr','Min','Max'))
        assert all(type(x) is int for x in (attr,lo,hi)) and attr in rows and attr not in seen and lo<hi, (label,'invalid variable affix')
        seen.add(attr); a=rows[attr]
        assert lo<=a[vkey]<=hi, (label,'variable reference outside range')
        assert all(supported_endgame_affix(attr,x,a[mkey]) and (x<0 if attr==25 else x>0) for x in (lo,hi)), (label,'invalid variable endpoints')
    seen=set()
    for v in local:
        assert isinstance(v,dict) and set(map(key,('Attr','Percent')))<=set(v)<=set(map(key,('Attr','Percent','MinPercent','MaxPercent'))), (label,'invalid local shape')
        attr,pct=(v[key(k)] for k in ('Attr','Percent')); lo=v.get(key('MinPercent'),0); hi=v.get(key('MaxPercent'),0)
        assert all(type(x) is int for x in (attr,pct,lo,hi)) and attr in (47,15,13,17) and attr not in seen and 0<pct<=1000, (label,'invalid local enhancement')
        seen.add(attr)
        if lo or hi:
            count+=1
            assert 0<lo<=pct<=hi<=1000 and lo<hi, (label,'invalid local range')
        if intrinsic is not None:
            names={47:('min_atk','max_atk'),15:('matk',),13:('def',),17:('mdef',)}[attr]
            assert any(intrinsic.get(n,0)>0 for n in names), (label,'local enhancement has no intrinsic base',attr)
    assert count<=2, (label,'at most two variable lines')


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


def read_authored(sources):
    authored = {}
    skills={r['SkillID']:r for r in json.loads((HERE/'skill_scopes.json').read_text())}
    for path in sorted(AUTHORED.glob('*.json')):
        assert path.stem in EXPECTED, f'unknown authored shard {path.name}'
        rows = json.loads(path.read_text())
        assert isinstance(rows,list), f'{path.name} must be an explicit item array'
        for row in rows:
            iid = row.get('BaseID')
            assert iid in sources and iid not in authored, f'invalid/duplicate authored BaseID {iid}'
            assert SEGMENTS[sources[iid][3]] == path.stem, f'wrong authored shard for {iid}'
            assert AUTHOR_REQUIRED <= row.keys() <= AUTHOR_REQUIRED | AUTHOR_OPTIONAL, (iid, row.keys())
            for key in ('Name','Lore','LoreRoot','Identity','Benefit','Tradeoff'):
                assert isinstance(row[key],str) and row[key].strip(), (iid,key)
            assert '演劫残忆中，行者以' not in row['Lore'] and '承载那一刻的心愿' not in row['Lore'], f'legacy template in {iid}'
            assert row['PowerTier'] in POWER_TIERS, (iid,'PowerTier')
            assert 0 <= len(row['Traits']) <= 3, (iid,'trait count')
            validate_extra_affixes(row['Affixes'],iid,unique=True)
            validate_roll_model(row['Affixes'],row.get('VariableAffixes',[]),row.get('LocalEnhancements',[]),iid,intrinsic=sources[iid][0])
            kinds=set()
            for t in row['Traits']:
                assert {'Kind','PowerBP','ThresholdPct'} <= t.keys() <= {'Kind','PowerBP','ThresholdPct','SkillID','SkillName'}, (iid,t)
                assert not sources[iid][3] or sources[iid][3] & TRAIT_CLASSES.get(t['Kind'],31), (iid,'class has no executable trait path',t)
                assert t['Kind'] in TRAITS and (t['Kind'],t.get('SkillID',0)) not in kinds and 0 < t['PowerBP'] <= 10000 and 0 <= t['ThresholdPct'] <= 100, (iid,t)
                if t.get('SkillID',0):
                    scope=skills.get(t['SkillID']); assert scope and t.get('SkillName',scope['Name'])==scope['Name'], (iid,'unverified skill')
                    assert t['Kind'] in scope['AllowedTraits'], (iid,'inert skill-specific effect',t)
                    assert sources[iid][3]==0 or sources[iid][3] & (1<<(scope['Profession']-1)), (iid,'skill class')
                    assert t['Kind'] not in {'basic_focus','kill_mend','kill_mana','physical_guard','magic_guard','healthy_guard','desperate_guard'}, (iid,'skill unavailable at hook')
                else: assert not t.get('SkillName'), (iid,'name without scope')
                kinds.add((t['Kind'],t.get('SkillID',0)))
            for b in row.get('ReservedSkills',[]):
                assert set(b)=={'Scope','Target','Levels'} and b['Scope'] in ('all','class','tree','skill') and 1 <= b['Levels'] <= 3, (iid,b)
                assert (b['Scope']=='all' and b['Target']=='') or (b['Scope']!='all' and b['Target']), (iid,b)
            authored[iid]=row
    names=[r['Name'] for r in authored.values()]; lores=[r['Lore'] for r in authored.values()]
    assert len(names)==len(set(names)), 'duplicate authored name'
    assert len(lores)==len(set(lores)), 'reused authored lore'
    return authored


def generate(rows,sources,authored):
    missing=sorted(set(sources)-set(authored))
    assert not missing, f'{len(missing)} items still need individual authoring; first missing IDs: {missing[:20]}'
    groups=collections.defaultdict(list)
    for row in rows:
        iid=row['base_id']; a,d,e,mask=sources[iid]; segment=SEGMENTS[mask]
        item=dict(ID=iid,BaseID=iid,BaseName=row['base_name'],Role=segment,Professions=mask,BeginnerAllowed=row['beginner_allowed'],Unrestricted=not mask and not row['beginner_allowed'],Sex=row['sex'],Slot=e['slot'],LevelRequirement=row['level_requirement'],ResourceLevel=row['resource_level'],NativeDropEligible=bool(row['drop_kind']),LegacyAffixPoolReady=row['can_roll_affix'],SourceArm=row['source_arm'],SourceDesc=row['source_desc'])
        item.update(authored[iid])
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
    assert all(i['ID']==i['BaseID'] for i in items)
    # Distinct strings/packages cannot prove good writing or balance. Editorial
    # review uses each row's concrete identity, benefit and tradeoff instead.


def main():
    parser=argparse.ArgumentParser(description=__doc__); mode=parser.add_mutually_exclusive_group(); mode.add_argument('--check',action='store_true'); mode.add_argument('--validate-authored',action='store_true'); args=parser.parse_args()
    rows,sources=enumerate_eligible(read_tables())
    # This independently reconstructed list must match the recovered source audit.
    with (HERE/'eligible.csv').open(encoding='utf-8',newline='') as f: recovered=list(csv.DictReader(f))
    assert len(rows)==len(recovered), (len(rows),len(recovered))
    for actual,old in zip(rows,recovered):
        assert {k:str(v) for k,v in actual.items()}==old, (actual,old)
    authored=read_authored(sources)
    if args.validate_authored:
        print(json.dumps(dict(authored=len(authored),remaining=len(rows)-len(authored),total=len(rows)),ensure_ascii=False)); return
    groups=generate(rows,sources,authored); validate(groups,rows)
    outputs={TARGET/'legacy_unique_v4'/f'unique_catalog_{key}.json':(json.dumps(group,ensure_ascii=False,separators=(',',':'))+'\n').encode() for key,group in sorted(groups.items())}
    # Ordinary type-drop classification remains at the original source scope;
    # it is never regenerated from the smaller V5 unique whitelist.
    outputs[TARGET/'native_equipment_drop_bases.json']=(json.dumps([r['base_id'] for r in rows if r['drop_kind']],separators=(',',':'))+'\n').encode()
    assert all(len(data)<500000 for data in outputs.values())
    summary=dict(schema=2,total=len(rows),native_drop_kind=885,conversion_only=734,legacy_affix_pool_ready=298,class_counts=EXPECTED,sex_counts=dict(collections.Counter(str(r['sex']) for r in rows)),catalog_sha256={p.name:hashlib.sha256(data).hexdigest() for p,data in outputs.items() if p.name.startswith('unique_catalog_')},source_csv_sha256=hashlib.sha256(encode_csv(rows)).hexdigest(),max_catalog_bytes=max(map(len,outputs.values())),authored_entry_count=len(authored),legacy_template_count=0,power_tier_counts=dict(collections.Counter(i['PowerTier'] for i in authored.values())),trait_count_distribution=dict(collections.Counter(str(len(i['Traits'])) for i in authored.values())),executable_trait_kind_count=len({t['Kind'] for i in authored.values() for t in i['Traits']}),editorial_note='Completeness and structural checks do not certify literary quality or gameplay balance.')
    outputs[HERE/'coverage_v4.json']=(json.dumps(summary,ensure_ascii=False,indent=2)+'\n').encode()
    for path,data in outputs.items():
        if args.check:
            assert path.exists() and path.read_bytes()==data, f'stale or missing generated file: {path}'
        else: path.write_bytes(data)
    print(json.dumps(summary,ensure_ascii=False,indent=2))
    import subprocess,sys
    subprocess.run([sys.executable,str(HERE/'compile_v5.py')]+(['--check'] if args.check else []),check=True)

if __name__=='__main__': main()
