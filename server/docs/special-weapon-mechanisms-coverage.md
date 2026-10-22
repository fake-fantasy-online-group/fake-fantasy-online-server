# Enhanced weapon mechanism implementation coverage

Status: implemented in the cloud feature branch only. Not merged, deployed, or released.

## Scope and activation

- 60 original weapon templates: 12 below level 10 have no mechanism; 48 higher-level weapons have 56 approved profession branches.
- All 56 branches are represented in the approved test fixture and in the authoritative skill membership catalog. All referenced skills exist in the real native fixture and begin no later than the weapon requirement.
- Runtime activation uses domain.EnhancedWeaponBaseID and functional equipment checks, never a displayed name or a guessed ID offset.
- After the user-authorized reward/drop additions, all 48 higher-level weapons and all 56 profession branches are enabled: 42 true dungeon weapon templates plus the 6 original level-14 task weapon templates. The 18 original level-3/8/14 task templates receive their approved extra-affix changes; the 12 level-3/8 templates still have no mechanism identity. All ordinary dungeon originals remain mechanism-inert.
- Base attributes and positive extra-affix multiplication are handled by the separate dungeon equipment manifest, not these combat hooks.

## What the tests establish

- AllApprovedBranchesActivateWithCorrectItemIdentity checks all 56 profession branches through actual functional equipment and learned-skill modifiers, validates the 42 true / 6 task-original identity split, and checks broken equipment, ordinary dungeon originals, and nonexistent task hard variants remain inert. This verifies activation, not 56 independent full combat scenarios.
- EveryApprovedBranch checks every profession branch at native skill ranks 1, 5, 20, and 100, plus learn-level eligibility, profession isolation, and native definition immutability. This is catalogue/definition coverage, not 56 independent end-to-end casts.
- FinalRequestedChanges checks 32 approved numeric rows at four ranks. AllApprovedGeometryRatios checks 15 geometry rows at four ranks. ControlAndGeometry checks targeted control durations and target shapes/limits.
- Dedicated runtime tests cover resource overflow, actual MP destruction, burn ownership and remaining-DOT settlement, shield conversion, exact stolen status balance, conditional damage/secondary effects, smoke lifetime, active icebind consumption, one-shot bonuses, warcry collection/swap invalidation, and native/low-level inactivity.
- AcceptedGroupCleanseMatchesPreviewAndBrokenGate exercises real acceptance and execution. HealingActualCastSetIntegration exercises 8 real casts across the two weapon-healing mechanisms, 2/4 healer set pieces, and positive/negative conditions; it verifies normal set synergy without recursive payloads.
- PersistentStatusDowngradeKeepsWeaponSnapshot checks 5 real status branches under effective skill-rank reduction while the weapon remains equipped, including the consumed concentration power snapshot.
- All test names below have the prefix TestSpecialWeapon. The branch table distinguishes scalar/geometry coverage from runtime tests; it does not claim full client integration or a 56-weapon live gameplay playtest.

## Branch matrix

| Weapon | Profession | Skills | Activation | Implementation | Additional focused tests |
|---|---:|---|---|---|---|
| 1021 利器·火精 | 2 | 10102 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1022 利器·飞鸟斩 | 2 | 10103 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1023 名器·愚者笑 | 2 | 10104 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1024 名器·却邪 | 2 | 10111 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1025 名器·灭魂 | 2 | 10113 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1026 名器·七星 | 2 | 10107 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; PersistentStatusDowngradeKeepsWeaponSnapshot |
| 1027 古器·紫影 | 2 | 10122 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1028 古器·青霜 | 2 | 10116 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1520 利器·真武剑 | 2 | 10117 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1521 利器·大旋风 | 2 | 10111 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1522 名器·千古 | 2 | 10119 | true template enabled | Definition/status/combat hooks wired | OverflowAndActualManaShield |
| 1523 名器·转魄 | 2 | 10119 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1524 名器·真刚 | 2 | 10121 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1525 名器·修罗道 | 2 | 10122 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1526 古器·百兽王 | 2 | 10127 | true template enabled | Definition/status/combat hooks wired | AllApprovedGeometryRatios; ConditionalAndSecondaryHooks |
| 1527 古器·破山剑 | 2 | 10123 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1620 利器·朔风枪 | 1 | 10005 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1621 利器·逐鹿枪 | 1 | 10017 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1622 名器·飞龙枪 | 1 | 10013 | true template enabled | Definition/status/combat hooks wired | OverflowAndActualManaShield |
| 1623 名器·无双戟 | 1 | 10006 | true template enabled | Definition/status/combat hooks wired | AllApprovedGeometryRatios |
| 1624 名器·龙鄂闪 | 1 | 10028 | true template enabled | Definition/status/combat hooks wired | WarcryAndFollowupAreConsumedOnce; WarcryAfterWindowAndWeaponSwap |
| 1625 名器·霸王枪 | 1 | 10021 | true template enabled | Definition/status/combat hooks wired | AllApprovedGeometryRatios |
| 1626 古器·破军蛇矛 | 1 | 10027, 10009 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry; WarcryAndFollowupAreConsumedOnce |
| 1627 古器·豪龙胆 | 1 | 10022 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1820 利器·弦月 | 3 | 10219 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1821 利器·暗杀刃 | 3 | 10203 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1822 名器·两极星 | 3 | 10213 | true template enabled | Definition/status/combat hooks wired | AllApprovedGeometryRatios; InstantTrapDoesNotRemainPlaced |
| 1823 名器·虎牙断 | 3 | 10208 | true template enabled | Definition/status/combat hooks wired | PersistentStatusDowngradeKeepsWeaponSnapshot; StealthBonusMultipliesNativeBonusOnlyOnce |
| 1824 名器·魔斩牙 | 3 | 10209, 10208 | true template enabled | Definition/status/combat hooks wired | ConditionalAndSecondaryHooks |
| 1825 名器·纷飞燕 | 3 | 10224, 10208, 10209 | true template enabled | Definition/status/combat hooks wired | ControlAndGeometry; PersistentStatusDowngradeKeepsWeaponSnapshot; SmokePersistsAndCleansUp |
| 1826 古器·仆虎斗狼 | 3 | 10209 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1827 古器·干将莫邪 | 3 | 10209 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1922 利器·流星 | 3 | 10221 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1923 利器·孔雀翎 | 3 | 10205 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1924 名器·五瓣兰 | 3 | 10213 | true template enabled | Definition/status/combat hooks wired | AllApprovedGeometryRatios |
| 1925 名器·业火 | 3 | 10223 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1926 名器·破术牙 | 3 | 10206 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios |
| 1927 名器·夜妖光 | 3 | 10224 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ControlAndGeometry |
| 1928 古器·苍月 | 3 | 10216 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1929 古器·麒麟牙 | 3 | 10225 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1320 利器·镇妖杖 | 4 | 10301 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ConditionalAndSecondaryHooks |
| 1320 利器·镇妖杖 | 5 | 10409 | task original enabled | Definition/status/combat hooks wired | FinalRequestedChanges; ConditionalAndSecondaryHooks |
| 1321 利器·祈风杖 | 4 | 10306 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios; ControlAndGeometry; AcceptedGroupCleanseMatchesPreviewAndBrokenGate |
| 1321 利器·祈风杖 | 5 | 10410 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios; ControlAndGeometry; AcceptedGroupCleanseMatchesPreviewAndBrokenGate |
| 1322 名器·南仙杖 | 4 | 10302 | true template enabled | Definition/status/combat hooks wired | OverflowAndActualManaShield |
| 1322 名器·南仙杖 | 5 | 10421, 10401 | true template enabled | Definition/status/combat hooks wired | BurnCashoutPreservesOrConsumesOwnBurn |
| 1323 名器·九玲珑 | 4 | 10320 | true template enabled | Definition/status/combat hooks wired | StealAndShieldHealing; HealingActualCastSetIntegration |
| 1323 名器·九玲珑 | 5 | 10428, 10410 | true template enabled | Definition/status/combat hooks wired | RemoteIceBlastConsumesOnlyActiveIcebind |
| 1324 名器·龙息 | 4 | 10304 | true template enabled | Definition/status/combat hooks wired | EveryApprovedBranch; shared target-centered area engine (no independent heal-center cast test) |
| 1324 名器·龙息 | 5 | 10433 | true template enabled | Definition/status/combat hooks wired | StealPreservesExactInstanceBalance |
| 1325 名器·太极 | 4 | 10316 | true template enabled | Definition/status/combat hooks wired | HealingActualCastSetIntegration |
| 1325 名器·太极 | 5 | 10405, 10421 | true template enabled | Definition/status/combat hooks wired | BurnCashoutPreservesOrConsumesOwnBurn |
| 1326 古器·幻仙 | 4 | 10322 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios; PersistentStatusDowngradeKeepsWeaponSnapshot; BuffPowerPreservesDurationAndBoolean |
| 1326 古器·幻仙 | 5 | 10412 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges; AllApprovedGeometryRatios; PersistentStatusDowngradeKeepsWeaponSnapshot; BuffPowerPreservesDurationAndBoolean |
| 1327 古器·轰雷神 | 4 | 10303 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |
| 1327 古器·轰雷神 | 5 | 10432 | true template enabled | Definition/status/combat hooks wired | FinalRequestedChanges |

## Regression command

```sh
PATH=/tmp/ffo-toolchains/go/bin:$PATH \
GOCACHE=/workspace/shared/go-cache GOPATH=/tmp/ffo-go-path \
TMPDIR=/workspace/shared/go-build-tmp \
go test ./internal/game/scene -run '^(TestSpecialWeapon|TestClassSet|TestCasterSets)' -count=1
```

Final frozen-point regression passed across data/domain/scene/session/gameserver; the combined quest/dungeon/weapon/set race suite passed across data/domain/scene/session. No deployment is authorized by these test results.
