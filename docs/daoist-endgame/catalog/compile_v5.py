#!/usr/bin/env python3
"""Compile explicitly reviewed V5 stats and the approved 259 identities.
No generation by ID, invented yellow references, or mutations of old instances.
"""
from __future__ import annotations
import argparse,collections,copy,hashlib,json,pathlib
from generate import validate_extra_affixes, validate_roll_model
HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parents[2]
DOMAIN=ROOT/'server/internal/domain'
def encode(value): return (json.dumps(value,ensure_ascii=False,separators=(',',':'))+'\n').encode()
def benefit_text(proposal, numeric):
    parts=[proposal['intended_lane_zh']+'底盘：'+'、'.join(a['attr_name'] for a in proposal['core_attribute_candidates'])]
    labels={47:'物攻',15:'法攻',13:'回避防御',17:'魔防'}
    elements={'fire':'火','ice':'冰','holy':'圣','dark':'暗'}
    effect_labels={'element_damage_bp':'伤害','element_resist_bp':'抗性','enemy_element_pierce_bp':'穿透','element_mitigation_bp':'额外减伤','element_absorb_bp':'吸收'}
    for row in numeric['LocalEnhancements']:
        value=f"{row['MinPercent']}–{row['MaxPercent']}" if row.get('MinPercent') else str(row['Percent'])
        parts.append(f"本件未加工白字{labels[row['Attr']]}强化{value}%，不放大其它来源")
    for row in proposal['advanced_affixes']:
        family=row['family']
        if family=='magic_find_pct':parts.append(f"MF +{row['value']}%，让出战斗预算")
        elif family.startswith('skill_rank_'):parts.append(f"{row['target_label']} +{row['levels']}级，仅已学技能")
        elif family in effect_labels:parts.append(f"{elements[row['element']]}{effect_labels[family]} +{row['value_bp']/100:g}%")
        elif family=='proc_skill':
            target='存活且生命不高于30%时自救' if proposal['base_id']==2509 else '直接法术命中单体补击' if proposal['base_id']==1306 else '直接物理命中单体补击，不带定身'
            parts.append(f"{target}：{row['chance_bp']/100:g}%触发固定{row['fixed_skill_rank']}级{row['skill_label']}，冷却{row['icd_ms']/1000:g}秒")
        elif family=='equipped_aura':parts.append(f"240像素组队光环：{elements.get(row.get('element'),'物理')}{'减伤' if row['effect_family']=='physical_mitigation_bp' else '抗性'} +{row['amount_bp']/100:g}%，同名取最高")
    return '；'.join(parts)

def compile_catalog(proposal=None, authored=None):
    if proposal is None: proposal=json.loads((HERE/'v5/assignment.json').read_text())['items']
    if authored is None: authored=json.loads((HERE/'v5/authored_stats.json').read_text())
    stats={r['BaseID']:r for r in authored}
    legacy={r['BaseID']:r for p in (DOMAIN/'legacy_unique_v4').glob('*.json') for r in json.loads(p.read_text())}
    assert len(proposal)==259 and len(stats)==259 and set(stats)=={r['base_id'] for r in proposal}
    groups=collections.defaultdict(list)
    for p in proposal:
        iid=p['base_id']; old=legacy[iid]; numeric=stats[iid]
        assert old['Name']==p['unique_name'] and p['wear_level']<=60 and p['is_fly_arm']==0
        assert p['yellow_reference']['status']=='actual_same_base_native_pool' and p['yellow_reference']['base_id']==iid
        assert abs(sum(p[k] for k in ('budget_core_u','budget_local_u','budget_advanced_u','budget_retained_trait_u'))-p['total_budget_u'])<1e-8
        d=copy.deepcopy(old)
        d.update(PowerTier=p['tier'],Identity=p['proposed_identity'],Tradeoff='；'.join(p['tradeoffs']),NativeDropEligible=True,LegacyAffixPoolReady=True,ReservedSkills=[])
        d.update({k:numeric[k] for k in ('Affixes','VariableAffixes','LocalEnhancements','TypedEffects','SkillBonuses')})
        d['Traits']=p['retained_existing_traits']
        d['Benefit']=benefit_text(p,numeric)
        effects=[x for x in p['advanced_affixes'] if not x['family'].startswith('skill_rank_')]
        skills=[x for x in p['advanced_affixes'] if x['family'].startswith('skill_rank_')]
        assert len(d['TypedEffects'])==len(effects) and len(d['SkillBonuses'])==len(skills), (iid,'missing/unbudgeted advanced effect')
        for original,actual in zip(effects,d['TypedEffects']):
            family=original['family'];expected={'proc_skill':'proc','equipped_aura':'aura'}.get(family,family)
            assert actual['kind']==expected, (iid,'advanced family drift')
            if expected not in ('proc','aura'): assert actual['value']==original.get('value_bp',original.get('value')) and actual.get('element')==original.get('element'), (iid,'advanced value drift')
            elif expected=='proc':
                a=actual['proc']
                assert a['skillId']==original['skill_id'] and a['fixedRank']==original['fixed_skill_rank'] and a['chanceBP']==original['chance_bp'] and a['icdMs']==original['icd_ms'] and a['pvpScaleBP']==3500 and a['maxTargets']==1 and a['scaleBP']==10000, (iid,'proc payload drift')
                assert a['trigger']==('on_direct_damage_taken' if iid==2509 else 'on_direct_hit') and a['payload']==('heal' if iid==2509 else 'damage') and a['targetRule']==('self' if iid==2509 else 'hit_target'), (iid,'proc eligibility drift')
                assert a.get('requireMagic',False)==(iid==1306) and a.get('hpBelowPct',0)==(30 if iid==2509 else 0), (iid,'proc condition drift')
            else:
                a=actual['aura']
                assert a['id']==original['aura_id'] and a['family']==a['id'] and a['modifier']==original['effect_family'] and a['value']==original['amount_bp'] and a.get('element','')==original.get('element',''), (iid,'aura payload drift')
                assert a['radiusPx']==240 and a['refreshMs']==500 and a['leaseMs']==1000 and a['maxPlayers']==6 and a['includeActivePet'] is True and a['pvpScaleBP']==5000 and a['visualKey'], (iid,'aura scope drift')
        for original,actual in zip(skills,d['SkillBonuses']):
            target=original['target']
            assert actual['scope']==original['family'].removeprefix('skill_rank_') and actual['levels']==original['levels'], (iid,'skill rank drift')
            for before,after in [('prof','profession'),('school_id','school'),('skill_id','skillId'),('branch','branch')]:
                assert actual.get(after)==target.get(before), (iid,'skill target drift')

        validate_extra_affixes(d['Affixes'],iid,unique=True)
        validate_roll_model(d['Affixes'],d['VariableAffixes'],d['LocalEnhancements'],iid,intrinsic=p['native_intrinsic'])
        assert len(d['VariableAffixes'])+sum(bool(x.get('MinPercent')) for x in d['LocalEnhancements'])==p['variable_line_count']
        assert {x['Attr'] for x in d['Affixes'] if x['Value']>0 or x['Attr']==25}=={x['attr'] for x in p['core_attribute_candidates']}
        groups[d['Role']].append(d)
    items=[r for rows in groups.values() for r in rows]
    assert dict(collections.Counter(r['PowerTier'] for r in items))=={'transition':183,'highvalue':54,'builddefining':18,'godlike':4}
    return groups

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--check',action='store_true');args=ap.parse_args()
    groups=compile_catalog()
    # Preserve empty shards so embedding never accidentally retains a stale V4 row.
    for role in ('warrior','swordsman','assassin','healer','warlock','sharedcaster','allclasses','unrestricted'):
        path=DOMAIN/f'unique_catalog_{role}.json';data=encode(groups.get(role,[]))
        if args.check: assert path.read_bytes()==data, f'stale V5 catalog {path}'
        else:path.write_bytes(data)
    items=[r for rows in groups.values() for r in rows]
    summary={'schema':5,'total':259,'legacy_frozen_total':1619,'new_generation_excluded':1360,'native_drop_kind':259,'legacy_affix_pool_ready':259,'class_counts':dict(collections.Counter(r['Role'] for r in items)),'power_tier_counts':dict(collections.Counter(r['PowerTier'] for r in items)),'variable_count_distribution':dict(collections.Counter(str(len(r['VariableAffixes'])+sum(bool(v.get('MinPercent')) for v in r['LocalEnhancements'])) for r in items)),'typed_effect_counts':dict(collections.Counter(e['kind'] for r in items for e in r['TypedEffects'])),'skill_item_count':sum(bool(r['SkillBonuses']) for r in items),'catalog_sha256':{f'unique_catalog_{role}.json':hashlib.sha256(encode(groups.get(role,[]))).hexdigest() for role in ('warrior','swordsman','assassin','healer','warlock','sharedcaster','allclasses','unrestricted')},'validation_boundary':'Structural compilation; runtime static budget and combat/client acceptance are separate tests.'}
    path=HERE/'coverage.json';data=(json.dumps(summary,ensure_ascii=False,indent=2)+'\n').encode()
    if args.check: assert path.read_bytes()==data,'stale V5 coverage'
    else:path.write_bytes(data)
    print('Verified V5: 259 exact identities; tiers183/54/18/4; same-base yellow evidence; budget ledger; meaningful static fields; at most two rolls.')
if __name__=='__main__':main()
