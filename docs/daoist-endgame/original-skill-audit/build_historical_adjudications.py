"""Summarize independently reviewed external evidence. Not executable skill configuration."""
import argparse, json, pathlib
D=pathlib.Path(__file__).parent
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('evidence_bundle', type=pathlib.Path, help='Path to five-professions-skill-evidence-20261012.json; raw evidence is not bundled in the repo.')
args=parser.parse_args()
bundle=json.load(args.evidence_bundle.open())
files={x['filename']:x['content'] for x in bundle['files']}
old={s['id']:s for s in files['historical_backup-full-metadata.json']['skills']}
new={s['id']:s for s in files['current-full-metadata.json']['skills']}
urls={100:'https://fo.17173.com/character/character_2.html',101:'https://fo.17173.com/character/character_1.html',102:'https://fo.17173.com/character/character_3.html',103:'https://fo.17173.com/character/character_4.html',104:'https://fo.17173.com/character/character_5.html'}
patch='https://fo.17173.com/gengxin/20051222.shtml'
theft='https://fo.17173.com/content/2006-06-09/1149819653.shtml?_platform=PC'
R={}
def add(ids,decision,fields,fact,limits='',extra=None):
 for i in ids:
  R.setdefault(i,[]).append(dict(decision=decision,field_scope=fields,external_fact=fact,limits=limits,source_urls=[urls[i//100]]+(extra or [])))
add([10008,10025,10026,10108],'restore_old_supported',['resistance scope'], '原设计为指定状态抗性；旧备份与网页一致，未包含新增的全部不良状态抗性。','只支持移除额外广谱效果，不能由网页未提及证明任何年代都从未调整。')
add([10014],'restore_old_supported',['name','passive mechanism','rank effect'],'破魔只增强破法之枪的法力伤害，10个百分点起、每阶加10；无同目标枪势叠层。')
add([10024],'restore_old_supported',['name','passive mechanism'],'维护减少裂空之枪的武器损耗；不是按范围敌人数提升伤害。','具体逐阶减损百分比未外部核验。')
add([10015],'restore_old_supported',['rank damage'],'心眼之枪165%起，每阶增加4个百分点，五阶181%；不是180–260%。')
add([10027],'restore_old_supported',['rank damage'],'重击为100%伤害，升级延长昏迷；旧五阶100%与网页一致。')
add([10102,10122],'restore_old_supported',['single-target identity','shape','radius','area descriptions'],'罡气剑和混沌剑均为目标技能；旧备份单体与历史技能表吻合。','混沌禁益效果须保留；不能只删除形状而遗漏实现中的群体目标枚举。')
add([10104],'restore_old_supported',['rank life conversion'],'饮血生命转化20%起、每阶加2个百分点，五阶28%；当前五阶44%无所引历史依据。','血影被动独立增幅，不能把其加成揉入饮血基础值。')
add([10118],'restore_old_supported',['name','passive mechanism','rank effect'],'风斩只增强真空剑，5%起、每阶加5%；不是多种剑技积攒剑意。')
add([10108],'restore_old_supported',['rank resistance'],'心镜仅昏迷抗性10%起、每阶加10%。')
add([10115,10124],'mixed_do_not_blanket_restore',['name','tradeoff mechanism','rank stat change'],'火/山字诀原为攻击与防御一增一减，10%起、每阶3个百分点；旧备份保留代价，当前纯增益改写。','网页15秒，旧备份/SQL30秒；外部不能给30秒正式认证。当前山诀伤害减免不能等同防御力。')
add([10123],'restore_old_supported',['instant-death chance'],'陨星即死判定为2%；网页只让伤害随阶增加，旧五阶均2%与之相符。','仍须保留Boss免疫等共用规则。')
add([10203],'restore_old_supported',['passive trigger','rank trigger chance'],'成功回避时直接概率反击，40%起、每阶3个百分点；没有回避叠层后下一击释放的机制。')
add([10218],'mixed_do_not_blanket_restore',['name','trap-disarm mechanism'],'解除陷阱是清除已发现陷阱；历史资料没有拆解范围伤害150–250%。','网页归为范围，但未给半径、施法种类、具体成功率；不能据此自动恢复全部旧形状。')
add([10229],'mixed_do_not_blanket_restore',['name','passive mechanism','rank probability'],'陷阱回避：触发陷阱时20%起、逐阶8个百分点概率仅受一半伤害；不是增加布雷上限。','网页与旧备份均有尚未实现文字；证实旧设计身份，不证实当时正式服实现。')
add([10306,10316,10318],'restore_old_supported',['separate skill identities','dispel categories'],'净化、清身、解咒是分别学习的技能，各自处理不同状态族；不支持并入净世后删除两项。','逐阶可清状态强度、消耗和前置仅有本地数据，未外部验证。')
add([10307,10321],'restore_old_supported',['separate skill identities','attack versus damage absorption'],'祝福与防御独立；祝福增强攻击，防御提供伤害吸收；不支持合并祝祷并删除防御。','祝福魔攻成长：网页10+2/阶，旧备份5+5/阶有冲突；防御旧总文案与阶级文案冲突，网页支持阶级伤害吸收10+3/阶。')
add([10312],'restore_old_supported',['eligible monster families'],'驱魔范围伤害限不死和恶魔系；旧备份与网页一致，当前取消种类限制属于作用范围改变。','冷却值没有外部独立验证。')
add([10317],'restore_old_supported',['single-target identity','shape','radius'],'圣光是带致盲的目标攻击；旧单体与网页一致，当前圆形110不获支持。')
add([10327],'mixed_do_not_blanket_restore',['name','reveal-only mechanism','radius'],'鹰眼用于反隐；没有新照亮易伤8–20%。网页半径300与当前数值300相合。','旧备份半径350；不能恢复姓名/反隐同时盲回退半径。')
add([10314],'mixed_do_not_blanket_restore',['added spell damage and blind-target damage'],'网页为抗打断被动；旧备份含抗打断及缩吟唱，当前再加魔伤/致盲目标增伤没有所引证据。','网页抗打断20+20/阶，旧备份20+5/阶且双效果，历史差异未解决，不能全称官版。')
add([10409],'mixed_do_not_blanket_restore',['rank damage','added guaranteed slow','general description'],'冰箭基础100%起、每阶5个百分点，外部支持旧五阶100–120%；减速来自独立冰系掌握概率被动。','旧总文案165%起、每阶10%与旧五阶及网页冲突，不能复原该错误文案；当前150–210%及保证减速不获支持。')
add([10433],'restore_old_supported',['dispel mechanism','added damage'],'秘术消除原为解除防御魔法；旧备份无附加伤害，当前120–200%魔伤没有所引证据。')
# External pages independently verify FIRST acquisition level only, not every rank requirement.
first={10016:60,10028:36,10029:51,10030:60,10115:60,10121:36,10124:60,10417:51,10424:60,10431:60}
for i,lv in first.items(): add([i],'restore_old_supported',['first-rank unlock level'],f'旧备份首学{lv}级与外部职业表一致；当前首学{new[i]["levels"][0]["lreq"]}级。','其余阶级需求等级未逐阶外部核验。')
add([10022],'retain_current_supported',['radius'],'野战八荒半径150；当前150与网页及旧文案一致，旧数值250矛盾。')
add([10130],'retain_current_supported',['self-buff identity','non-AoE shape'],'雷霆之怒是给下一普通攻击充能的自身增益；当前移除旧圆形150更符合网页和文案。','这并不证明数值shape枚举的具体实现。')
add([10308,10311,10315],'retain_current_supported',['friendly area identity','radius','shape'],'庇护、加速、瞬击均为自身周围半径150友方群体；当前圆形150符合网页及旧文案，旧shape0/radius0矛盾。','施法/冷却更改是另外的问题，未由该范围证据验证。')
add([10227],'retain_current_supported',['canFriendly'],'阴影跳跃设计允许跳到敌人或队友身边；当前canFriendly=1符合网页及旧文案。','老网页注明尚未实现；这证明目标设计，不证明70级当时可用。')
add([10029],'retain_current_supported',['stun general description'],'12/22正式公告将旧恐惧设计改为半径150群晕5秒+1秒/阶；旧总文案仍恐惧未实现，当前总文案纠正。','旧各阶已经是群晕，不能用旧总文案覆盖；当前降学习等级与改冷却另论。',[patch])
add([10212],'mixed_do_not_blanket_restore',['name','rank description','theft probability'],'真实旧备份及当前均为偷窃术；旧总文案、历史表、2006年6月记录均50%+6个百分点/阶，当前前五阶50/56/62/68/74符合。','旧前五阶错误写100–112%伤害，不能回退；当前每怪仅成功一次的限制未获独立历史确认；SQL游龙掌不能压倒这组证据。',[theft])
add([10006],'mixed_do_not_blanket_restore',['radius text versus numeric radius'],'网页及旧文案半径150；当前文案200，而旧/新实际数值均200。','这是文案与旧数值早已冲突，不是本次发现radius被150改200；网页伤害135与旧/新130也冲突。')
add([10121],'mixed_do_not_blanket_restore',['radius text versus numeric radius'],'旋风网页及旧五阶文案半径100；当前文案150，旧/新数值均150。','不能将修改文案与修改数值混为一项；首学36级有单独外证。')
add([10028,10127],'mixed_do_not_blanket_restore',['radius'],'战吼/挑衅网页及旧文案半径100；当前数值100，旧数值200。','当前数值更符合外部资料但旧数值来源不明，须逐字段选定，不能恢复旧200后称官版。')
add([10213,10214,10215,10216],'mixed_do_not_blanket_restore',['trap explosion radius','shape'],'这四种陷阱爆发半径100，当前radius100与网页相符，旧shape0/radius0不能证明陷阱没有范围。','必须保留放置/触发机制，不应因形状2把陷阱错误当即时直接群攻。冻气学习45级以12/22公告优先。',[patch])
add([10228],'restore_old_supported',['area detection identity'],'侦测陷阱作用于自身周围较大范围；旧圆形符合描述，当前无范围不获支持。','网页没有给精确半径，旧200不能称外部核验值。')
add([10313],'mixed_do_not_blanket_restore',['area damage identity','radius','target center'],'天罚是范围伤害；旧shape0/radius0与旧文案和网页矛盾，当前恢复范围方向正确。','网页和旧文案150，当前及SQL数值200；施法中心/距离、时间、冷却均需另证。')
add([10012],'external_insufficient',['rank vulnerability'],'网页仅说明破甲增加受伤，未给20–32%或25–40%的逐阶值。','可按项目已知旧备份恢复，但不得称该逐阶值获网页独立验证。')
add([10302,10303],'external_insufficient',['rank heal/mana amount','flat versus percent formula'],'网页说明回蓝/持续回血而无完整数值；本地备份可证明修改，但外部资料不足裁定精确恢复值。')
add([10307,10320,10324],'external_insufficient',['predecessor'],'旧备份前置10321、当前0，但原SQL这三项depend1_id也为0；本次未独立识读技能树图箭头。','前置hold，不得仅因防御术被删就断定当前0错误或回滚10321。')
# All pure cast/cooldown changes remain explicitly unverified externally.
for d in files['field-differences.json']:
 i=d['id'];c=d.get('changes',{});lv=c.get('levels',{}).get('byLevel',{})
 timing=sorted({k for r in lv.values() for k in r if k in ['cast','cd']})
 if timing:add([i],'external_insufficient',[f'levels.*.{k}' for k in timing],'所引历史资料没有核验此项施法时长/冷却精确值；旧备份差异成立但不是腾讯官方认证。','不能把资料缺失误写成当前值得到支持；可按已确认的项目恢复范围采用旧备份。')
rows=[]
for d in files['field-differences.json']:
 i=d['id']
 rows.append(dict(id=i,old_name=old[i]['name'],current_name=new.get(i,{}).get('name'),change_kind=d['kind'],assessments=R.get(i,[dict(decision='external_insufficient',field_scope=['all changed fields'],external_fact='未建立独立外部裁决。',limits='仅本地变更证据。',source_urls=[])])))
out=dict(schema_version=1,purpose='Research-only field-level evidence; not an automatic restoration or balancing configuration',evidence_bundle='five-professions-skill-evidence-20261012.json',backup_md5=files['historical_backup-full-metadata.json']['md5'],current_md5=files['current-full-metadata.json']['md5'],caveats=['Backup is local ad-hoc signed, not an authenticated Tencent release.','Static pages preserve stale pre-2005-12-22 descriptions; dated official patch mirrors take priority.','A supported decision applies only to field_scope, never all fields of a skill.','Web first-rank requirements do not certify later rank requirements or prerequisite IDs.','No external certification of every executable behavior, cast timing, cooldown, cost or economy rule.'],records=rows)
(D/'historical-asset-adjudications.json').write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n')
print('records',len(rows),'assessments',sum(len(r['assessments']) for r in rows))
