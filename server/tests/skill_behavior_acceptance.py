#!/usr/bin/env python3
"""Independent five-profession evidence/contract acceptance; no gameplay writes.

Outputs only IDs, short identity facts, field names, dispositions and checks.
It does not copy resource tables, per-level descriptions, artwork or credentials.
"""
from argparse import ArgumentParser
from collections import Counter
from hashlib import sha256
import json
from pathlib import Path

SERVER = Path(__file__).resolve().parents[2]
RENAMES = {10014:'破魔',10024:'维护',10118:'风斩',10218:'解除陷阱',10229:'陷阱回避',10306:'净化术',10307:'祝福术',10327:'神术：鹰眼'}
MISSING = {10316:'清身术',10318:'解咒术',10321:'防御术'}
KIND = {0:'single',1:'area',2:'passive',3:'special'}


def rows_by_id(rows):
    out = {int(r['id']): r for r in rows}
    if len(out) != len(rows):
        raise ValueError('duplicate skill IDs')
    return out


def changed_fields(row):
    if row['kind'] == 'removed_from_current':
        return {('presence', None)}
    out = set()
    for field, change in row.get('changes', {}).items():
        if field != 'levels':
            out.add((field, None))
        else:
            for rank, fields in change.get('byLevel', {}).items():
                for subfield in fields:
                    out.add(('levels.' + subfield, int(rank)))
    return out


def main():
    p = ArgumentParser(description=__doc__)
    p.add_argument('--evidence', type=Path, required=True)
    p.add_argument('--decisions', type=Path, required=True)
    p.add_argument('--contracts', type=Path, help='Exported 149-row skillRanks array or response')
    p.add_argument('--output', type=Path, required=True)
    a = p.parse_args()
    errors, checks = [], []
    def check(name, ok, detail=''):
        checks.append({'check':name, 'passed':bool(ok), 'detail':detail})
        if not ok: errors.append(name + (': '+detail if detail else ''))
    evidence = json.loads(a.evidence.read_text())
    files = {x['filename']: x['content'] for x in evidence['files']}
    historic = rows_by_id(files['historical_backup-full-metadata.json']['skills'])
    current = rows_by_id(files['current-full-metadata.json']['skills'])
    diffs = rows_by_id(files['field-differences.json'])
    decisions = rows_by_id(json.loads(a.decisions.read_text()))
    native = {x['ID']:x for x in json.loads((SERVER/'docs/daoist-endgame/v5/skill-original-ranks.json').read_text()) if x['Level']==1}
    check('E01.catalogue', len(historic)==149 and len(current)==146 and set(historic)==set(native))
    check('E01.profession_counts', Counter(x['Prof'] for x in native.values())=={1:30,2:30,3:30,4:29,5:30})
    check('E02.missing_healer', set(historic)-set(current)==set(MISSING))
    check('E02.modified', sum(x['kind']=='modified' for x in diffs.values())==68 and len(diffs)==71)
    actual_renames = {i:historic[i]['name'] for i in historic.keys()&current.keys() if historic[i]['name']!=current[i]['name']}
    check('E03.eight_renames', actual_renames==RENAMES)
    check('E05.all_decision_ids', set(decisions)==set(diffs))
    field_count = 0
    for i, row in diffs.items():
        expected = changed_fields(row)
        actual = {(d['field'], d.get('rank')) for d in decisions.get(i,{}).get('decisions',[])}
        field_count += len(expected)
        check('E05.fields.'+str(i), expected==actual, 'missing='+str(sorted(expected-actual,key=str))+' extra='+str(sorted(actual-expected,key=str)))
        for d in decisions.get(i,{}).get('decisions',[]):
            check('E05.reason.'+str(i)+'.'+d['field']+'.'+str(d.get('rank')), bool(d.get('action') and d.get('reason') and d.get('sources')))
    check('E05.field_total', field_count==472, str(field_count))
    layouts = evidence.get('skill_tree_layout_supplement',{}).get('layouts',[])
    positions = {}
    for layout in layouts:
        positions[layout['label']] = {int(slot['id']):(sw['tab'],sw['sw'],slot['x'],slot['y']) for sw in layout['content']['switches'] for slot in sw['slots'] if int(slot['id']) in MISSING}
    check('E04.existing_slots', positions.get('current')==positions.get('historical_backup') and set(positions.get('current',{}))==set(MISSING))
    contracts = {}
    if a.contracts:
        payload=json.loads(a.contracts.read_text())
        contracts=rows_by_id(payload['skillRanks'] if isinstance(payload,dict) else payload)
        check('R01.membership',set(contracts)==set(historic))
    matrix=[]
    for i in sorted(historic):
        n=native[i]
        disposition=decisions.get(i,{}).get('decisions',[])
        held=[{'field':d['field'],'rank':d.get('rank'),'action':d['action']} for d in disposition if 'hold' in d['action'] or 'unverified' in d['action']]
        row={'id':i,'profession':n['Prof'],'expected_identity':('偷窃术' if i==10212 else n['Name']),
             'resource_state':diffs.get(i,{}).get('kind','unchanged'),
             'changed_fields':[{'field':f,'rank':r} for f,r in sorted(changed_fields(diffs[i]),key=str)] if i in diffs else [],
             'disposition_count':len(disposition),'historical_holds':held,
             'acceptance_surfaces':['identity','current_next','targeting','rank_growth','native_tooltip'],
             'contract_checked':bool(a.contracts)}
        if i in MISSING: row['acceptance_surfaces'] += ['existing_tree_slot','raw_row_absent','icon','hover','hotbar','same_party_target']
        if i in RENAMES: row['acceptance_surfaces'].append('renamed_title')
        if a.contracts:
            rank=contracts.get(i,{})
            c=rank.get('contract')
            check('R01.contract.'+str(i), ('contract' in rank and c is None) if i==10212 else isinstance(c,dict))
            if isinstance(c,dict):
                check('R01.identity.'+str(i),c.get('name')==row['expected_identity'] and c.get('profession')==n['Prof'] and c.get('icon')==n['Icon'] and c.get('kind')==KIND.get(n.get('Kind',0)))
                check('R02.current.'+str(i), isinstance(c.get('current'),dict) if int(rank.get('effectiveRank','0'))>0 else c.get('current') is None)
                for which in ('current','next'):
                    b=c.get(which)
                    if b is None: continue
                    check('R02.integer_fields.'+str(i)+'.'+which,all(type(b.get(k)) is int and 0<=b[k]<=2147483647 for k in ('mpCost','prepareMs','cooldownMs','castDistance','requiredWeaponType')))
                    check('R07.input.'+str(i)+'.'+which,b.get('inputMode') in ('none','self','entity','point','direction','trapEntity'))
                    area=b.get('area',{})
                    check('R08.area.'+str(i)+'.'+which,area.get('shape') in ('none','rectangle','circle','cone') and area.get('center') in ('none','caster','target') and all(type(area.get(k)) is int and area[k]>=0 for k in ('radius','length','width','angle')))
        matrix.append(row)
    report={'schema_version':1,'purpose':'independent acceptance, not historical authenticity certification',
            'sources':{'evidence_sha256':sha256(a.evidence.read_bytes()).hexdigest(),'decisions_sha256':sha256(a.decisions.read_bytes()).hexdigest(),'contracts_sha256':sha256(a.contracts.read_bytes()).hexdigest() if a.contracts else None},
            'summary':{'historical':len(historic),'current':len(current),'modified':68,'missing':3,'renames':8,'decision_rows':len(decisions),'field_decisions':field_count,'rows_with_historical_holds':sum(bool(r['historical_holds']) for r in matrix),'checks':len(checks),'errors':len(errors),'runtime_and_client_tests_required':True},
            'checks':checks,'rows':matrix,'errors':errors}
    a.output.parent.mkdir(parents=True,exist_ok=True)
    a.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(report['summary'],ensure_ascii=False))
    for error in errors[:20]: print('FAIL',error)
    raise SystemExit(bool(errors))

if __name__=='__main__': main()
