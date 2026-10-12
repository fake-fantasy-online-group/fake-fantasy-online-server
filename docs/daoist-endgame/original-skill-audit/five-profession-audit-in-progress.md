# Five-profession skill audit: verified source paths and current limits

Status: READ-ONLY AUDIT, not a restoration claim. Inspected client 66c7712 and server d9c06dd on 2026-10-12. At 07:27 UTC the shared execution workspace disappeared; repositories were restored at identical commits and the imported row audit was regenerated afterward. No business source changes were made in this audit phase.

## Baselines must be kept separate

1. Earliest available client Git commit f8d8885 is an initial source release targeting 1.6.0. It is not proof that bundled external game tables are historically original.
2. Earliest available server Git commit a9c747c contains imported ov_skilldesc rows and authored semantic supplements. Imported rows and authored supplements are different evidence layers.
3. Current external skills resource MD5 cbe007d8955b60a992dd4ef3da682c37 matches the 1.6.0 manifest and a read-only local open-source-20261010 copy. A matching hash dates continuity between these copies; it does not establish canonical historical mechanics.
4. The Git source skill loaders and target classifier were unchanged between initial source release and pre-V5. Therefore a change already present in external skills.json predates the V5 source changes, even if Git contains no earlier version of the asset.
5. The stage commits 66c7712/d9c06dd correct known issues. They are not a final five-profession restoration.

## External-table execution influence, all professions

- Win05.SkillData.cs: two independent native load entry points, LoadSkillData and LoadStatic. Both deserialize external skills.json and publish SkInfo. Names, descriptions, levels, prerequisites, maximum rank, shape, range and target permissions enter here. Stage fix normalizes only original sword targeting.
- SkillTargeting.Classification.cs: except for the stage sword rule, the classifier trusts external shape, dist, radius and castkind/canEnemy. Description words also select traps (放置), teleport (移动至指定), leap (跳跃至选中), or self-centered area behavior (周围 and related words). Consequently changing prose can change execution routing, even if tgt stays single.
- GameHud.HotbarUseAdmission.cs: Single routes to CombatWorld.RequestSkill (entity-target); Circle/Direction routes to cursor/ground targeting and NetClient.UseSkillAt; Instant uses NetClient.UseSkill. This is actual request routing, not merely tooltip styling.
- CombatWorld.SkillRequests.cs and CombatWorld.Approach.cs: request range is divided by pixels-per-unit and tested against actual transformed world coordinates. Approach inset is 0.9*range minus 2 pixels. Server admission uses its own SkillDef.Dist. A resource range discrepancy can stop the client outside server range.
- ReferenceWorldLocalSkillCast.cs: ReferenceSkillEffectCatalog separately parses skills.json into a cast/aim cache. Stage sword corrections are explicitly applied here too. A full Assets/FakeFantasyOnline call-site search found no production calls to TryCreate, ShouldAimAtResolvedTarget or ResolveImmediateTargetEntityId; this is currently a compatibility/diagnostic surface, not a proven reachable gameplay override.
- NetClient.SkillCommands.cs: LocalNotEnoughMp reads external levels[].mp at CharData.skills rank before sending either entity or ground cast. This path does not read the V5 authoritative skill view. An altered resource mana cost can therefore block a valid server cast locally. Exact affected IDs await the full current resource projection.
- Win05.GrowthSkills.cs: V5 descriptions and rank/cost/effect lines come from the server, but the card title still uses external info.name. Fixing server descriptions does not automatically restore names.
- Win05.SkillLevelBlock.cs and Win05.SkillDialog.cs: legacy/no-growth tooltips, upgrade prerequisite checks, rank cap and rank requirements continue to read external levels/pre/maxlv. Passive/hotbar classification uses tgt/type.
- ReferenceFirstWorldHotbar.cs has another skills.json reader for id/icon only. It does not select targeting but remains relevant to identity/icon restoration.
- ReferenceWorldReceiveOwner.SkillCastReceipt.cs: acknowledged server chant/cooldown timings are used, but target-facing, self-area rings, support cast selection and effect routing still consume external SkInfo fields.
- SkillEffectDB.cs and SkillCaster.cs: skillshow/skillshow05 tables control physical animation classification and fly/attack/hit visual phases; optional res/skillevent.txt scripts take precedence for matching Skill<ID>. These are a separate visual resource layer, not authoritative server damage results.
- SkillSoundDB.cs and SkillElementDB.cs: separate skillsound/skillsound24/skillelement tables control sounds, charge effects and targeting colors. Their current and historical field projections are not yet available.

## Confirmed narrow resource deviations already established

- 10102 罡气剑: current external tgt=1/castkind=1 but cone shape=3, radius=120, angle/rwidth=120, distance=100; imported server rows classify single, shape=0, distance=75. Stage client correction restores single routing and 75-pixel range.
- 10122 混沌剑: current external tgt=1/castkind=1 but circle shape=2, radius=150, distance=75; imported server rows classify single, shape=0, distance=75. Stage correction restores single routing.
- Genuine sword areas are 10121 旋风剑, 10125 血雨剑, 10127 挑衅, 10128 煞气 in the imported baseline. Their numeric radii conflict across raw server, resource and prose; current stage fix preserves existing true-area resource geometry rather than silently resolving the version conflict.

## Proven repository-level invariants, not historical official certification

The previous read-only audit verified 750 imported profession rank rows, 150 IDs (149 combat plus 11005 life), and 58 exact native passive source-to-target pairs. V5 catalog names/professions/unlocks/prerequisites agree with the imported raw rows. Loaded 745 combat rows did not change single-vs-area kind relative to those imported rows. These facts establish internal consistency only; they cannot prove that imported rows were untouched historical originals.

## Pending evidence

- Full five-profession current skills.json projection including all levels, description, identity and execution fields from the authorized read-only local evidence task.
- Historical/public source cross-check with a defensible version boundary. A separate research worker is examining Tencent and contemporary archived skill documentation.
- Full visual/cast-event resource comparison where changes affect perceived skill behavior.
- Exhaustive row-by-row comparison and a decision matrix distinguishing confirmed modifications, version differences, parser defects and unresolved evidence.

The authorized cloud resource endpoint returned HTTP 403 earlier; no alternate route or repeat was attempted. No resource files were uploaded and no database state was changed.

## Newly located local historical asset (projection pending)

The parent’s authorized read-only local task reports a separate backup marked 1.0/build0 with 149 combat entries, and substantial differences from the currently used resource: reported 68 skill entries differ; 57 level-data differences, 25 description differences, eight renamed skills, and 19 target/shape differences. It reports that current healer data omits 10316 清身术, 10318 解咒术 and 10321 防御术. These counts and exact fields remain provisional until the full hash-bearing projection is joined. The older backup is direct evidence of an asset-change chain, but its ad-hoc version marker does not independently authenticate an official historical release.

Reported sword changes include 风斩→剑意 with stack mechanics; 饮血剑 rank5 life steal28→44%; 心镜 adds40% all-state resistance; 火字诀 removes the defense cost; 山字诀 becomes25% damage reduction; and 陨星剑 death chance2→6%. These observations are separate from the already-fixed targeting issue, and are not yet adopted as restoration values.
