# Active client/world requirements audit

Audit basis: client `fa41a5b`, server `cd43b8f`, plus concurrent uncommitted work in the shared cloud checkouts. Checked on 2026-10-13. This audit deliberately uses the active user requirements, including the later instruction restoring the complete original pregame appearance. It does not treat old restoration counts or canceled work as current goals.

## Important conclusions

1. **Map transition is still open and not accepted.** The user's latest message explicitly reports it remains broken. The old 200 managed assertions pass, but do not establish real frame time, absence of a second black screen, or entity completeness. A separate worker is now fixing the lifecycle. No map code was changed by this audit.
2. **A real first-missing monster-status texture bug was reproduced and fixed.** Original status mapping and the HUD retry cache were already correct, but `MonsterEntity.StatusIcons` cached a border-only composition forever for an unchanged row signature. The active `ReferenceWorldMonsterActor` calls this owner. See the focused fix below.
3. **Five portal cases are inconclusive evidence, not five confirmed bugs or missing assets.** Their raw-object evidence, scene compositions and layer records exist in the cloud. Correct solid-foreground occlusion must be distinguished from wrong background depth. The current repair intentionally does not erase ice wings, trees or platforms just to make every portal pixel visible.
4. No Unity Player, live login, audible playback, runtime screenshot acceptance, production deployment, or all-map live correctness was established. No Mac work, new downloads, commits, pushes or server source edits were performed here.

## Requirement matrix

| Active requirement | Result | Current evidence and remaining boundary |
|---|---|---|
| Reuse original artwork for the 26 UI groups | Implemented in source; visual acceptance unverified | `ArtworkSkin.cs` is resource-backed; `Tests/UiArtwork/README.md` lists 26 groups. Fresh 41 artwork/input contracts and 200 inventory/social source/geometry assertions passed. The requested historical `/workspace/shared/ui-harmony/BRIEF.md` and task list are not present at that location, so this is not a fresh pixel audit of each of all 26 groups. The later pregame revert takes precedence. |
| Login/server/role selection restore the entire initial appearance, not another new style | Implemented in source; runtime appearance unverified | Fresh `pregame_original_regression.py`: 42 provenance/equivalence checks across 22 files against baseline `99c800c`, itself matching initial `f8d8885`. Includes login, server selection, character selection/creation, delete confirmation and native drawing sequence. Only allowed functional bridge/lifecycle differences remain. |
| Plugin agent can log in, select server and role, enter world through CLI/workbench testing | Implemented API/CLI; real-server acceptance unverified | `Documentation/Plugins/TITLE.md`, `TitleFlow.PluginBridge*`, title action adapters and CLI toolkit describe guarded title observation/actions plus ephemeral sensitive-login. Fresh native bridge: 32 execution checks and source invariants pass. No real login or credential submission was attempted. A standalone Python client-test invocation without its required fixture host arguments was invalid; it is not counted as a transport test pass. |
| No forced F8 global panel, tab basket, or recipe encyclopedia | Implemented source architecture; final managed acceptance reported passing | `Documentation/Endgame/MODULE-ENTRYPOINTS.md` and `EndgamePanel*`: independent NPC workbenches; no F8/global launcher/cross-module tabs; 66 recipe IDs retained behind 12 actionable families. The initial concurrent-edit run stopped at `live completed run reentry`; the NPC worker subsequently fixed the recovery gate and preserved admitted/invited reentry. The parent reports final 2,152 UI and 212 endgame / 377 total plugin assertions passed. That intermediate failure is resolved, not a current failure. |
| Correct portal depth on all maps, including 地下王宫 | Targeted defects repaired; complete-world correctness unverified | `ReferencePortalApertureProfile.cs`, `ReferenceLegacyMapObjectRenderer.PortalAperture.cs`, `Documentation/Rendering/PORTAL-APERTURE-OCCLUSION.md`. Exact asset profiles operate wherever their model/path/geometry/portal pair match, not a list of map names. They preserve foreground silhouettes rather than raising every portal. Cloud main manifest has 639 positions plus nine embedded positions. Five cases are inconclusive as detailed below. No new portal changes here. |
| One map load/no double black screen; fix client initialization and short stalls | **Still open; user explicitly says still broken** | `GameBootstrap.cs`, `ReferenceFirstWorldWindowOwner.cs`, `ReferenceFirstWorldInitialFrame.cs`, `PlayerController.TransferFrame.cs`. Fresh original contract: 200 assertions/39 scenarios pass, including same-landing receipts and entity arrival. Still synchronous `BuildWorldCore`, synchronous preflight work and resource-unload wait were identified as real timing boundaries. Map worker reports an additional concrete uncovered case: comparing server landing against a player who moves during fade can trigger another load. No server double-publish workaround is endorsed. |
| Experience flytext must not replay after map changes | Revision dedup and confirmed old-visual defects fixed locally; Player acceptance unverified | `WorldCombatNumberPresentation.cs` retains consumption in the session; only production frame call passes that session. The two identified defects are now fixed: scene-owned pending visuals retire with their old live frame; already-created EXP objects are returned to their pool on actual scene retirement or final player detach. Fresh 107 queue/frame/spawn assertions, 23 full pool/animation assertions and lifecycle guards pass. Unknown first-time loading/in-flight notifications are deliberately preserved. Actual XP values, gateway and wire are unchanged. See follow-up below. |
| Skills must not play the ordinary attack sound | Implemented/tested source and wire classification; live audio unverified | Client actual cast/combat/attack owners: 252 fresh assertions. Server `protocol/skill_audio_test.go` passes, checking physical/magic/proc/DOT classifications and interleaved basic attacks while preserving crit/miss/fatal bits. Ordinary non-skill attacks retain their sound. |
| Assassin visually holds weapons in both hands | Implemented; tested with existing real asset data, no Player acceptance | `ReferenceAvatarWeaponPresentation.cs`, `ReferenceAvatarAssetPlan.cs`, `PlayerAvatar.Build.cs`. Fresh original uploaded evidence test: 1,470 poses across genders/five art directions and 16,028 assertions. Explicit left weapon, switches and action paths retained. Source pixels/pose geometry do not establish live animation/shader acceptance. |
| Positive attack-speed bonuses displayed positively, not as a negative interval | Implemented/tested presentation | Client `EndgamePanel.Tooltips.cs` and server `inventory.go`/`wash.go`. Focused three server attack-speed tests pass: percent -20 storage displays 攻击速度 +20%; true flat interval values remain correctly labeled in milliseconds. Underlying combat intervals are not sign-flipped. |
| Actual native per-status icons, especially poison; no generic fallback | Native map/wire implemented; confirmed composition bug fixed locally | Server per-level `level_small_maps` and `IconKnown` protect authored zero. Poison 1001→5030, 使毒1018→7037, freeze1004→7087, with level-dependent mappings. Fresh client 28 exact mapping/real poison packet assertions and focused server scene/protocol tests pass. Newly fixed monster composition test: 62 assertions. Existing-db deployment still requires migration `000000_init-57.sql`; this audit did not recheck a running database/Player. |
| Equipped/character tooltip matches inventory for the real item/UID | Implemented source with publication identity fence; final managed UI acceptance reported passing | `TooltipContent.Get.cs:25,34`, `EquipDlg.OnGUI.cs:353+`, inventory comparison and `EndgamePanel.BagTooltip/EquippedTooltip` share one overlay. `EndgameClientState.Find` requires `InventorySnapshotCurrent`; native publication invalidates old sidecar identities, preventing same-base replacement using a stale UID. Existing targeted cases remain. The earlier reentry halt was fixed by the NPC worker; final 2,152 UI assertions were reported passing by the parent. |
| Armor/attack local enhancement included in character totals | Implemented/tested domain computation; runtime panel unverified | `domain/endgame_item_local.go`: local increments use unrefined item base and enter `UniqueItemBonuses`/`AstralItemBonuses`. Fresh tests include local scope, actual `Compute` delta and winning formation. Scene native enhanced-base color/merge tests passed. Item preview and character computation remain distinct but share actual local increments; not a tooltip-only arithmetic addition. |
| Equipment traits shown orange after all attributes | Implemented source; final managed UI acceptance reported passing | `EndgamePanel.Tooltips.cs:98-121` gathers remaining ordinary attributes, inserts them at the native attribute anchor, then inserts orange effects immediately afterward. Socket metadata, story and binding sections follow. Source is present and existing tests cover it; the NPC worker subsequently resolved the intermediate aggregate reentry failure, with final 2,152 UI assertions reported passing. |

## Status composition defect and repair

Original source evidence:
- `MonsterEntity.StatusIcons.cs` stored `siSig` before the icon lookup, composed borders when the lookup returned null, and then returned early for unchanged signatures.
- `TickStatusIcons` only re-entered application when timed statuses expired. The `GameHud.BuffIcon` no-null-cache change could not help because it was never called again.
- `ReferenceWorldMonsterActor.cs:643` calls `sourceOwner.SetStatusIcons`, making this an active path. The separate `ReferenceWorldMonsterStatusIconView` contains a similar cache pattern but its exposed actor property returns null; it was not edited as an active owner.

Reproduction before fix is preserved:
- `/tmp/client-world-audit/MonsterEntity.StatusIcons.before.cs`
- `/tmp/client-world-audit/repro-missing-status-texture.cs`
- `/tmp/client-world-audit/missing-status-texture.log`: after resources are ready, tick plus same-status refresh still leaves icon load count at one.
- `/tmp/client-world-audit/status-before-regression.log`: the new full-owner regression fails against the old source with `same-signature authoritative packet retries exact resource` (exit 1).

Repair scope:
- Unresolved positive icons can retry on an authoritative status publication or an actual loader/package-owner identity change.
- Stable unresolved resources have no timer poll, no resource reads and no allocations per frame. Successful same-signature compositions remain cached.
- Original icon zero remains deliberately blank and never falls back to skill/generic artwork.
- The next active expiry deadline is tracked, preventing an already-expired row from making the surviving status list allocate every frame.
- Replaced/released sprite and composed texture are destroyed; borrowed source artwork is retained.

Only owned repository files changed by this worker:
1. `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/MonsterEntity.StatusIcons.cs`
2. `Tests/Endgame/run-status-icon-contract.py` (test-double signature accepts the retry flag)
3. `Tests/Endgame/MonsterStatusIconReadinessContract.cs` (new)
4. `Tests/Endgame/run-monster-status-icon-readiness-contract.sh` (new)

Verification: 62 new assertions, including zero allocated bytes over stable 1,000-tick loops, plus the existing 28 mapping/wire assertions. Focused diff whitespace check passes. No commit or push.

## Five inconclusive portal positions

Evidence directory: `/workspace/scratch/1cc826cfb9aa/portal-evidence/remaining-audit/`.
The audit read `review-b-summary.json`, final layer/render records and inspected representative final images.

- `qw0086-library-572` and `573`: the effect intersects solid ice side wings. Existing original/final imagery does not establish a wrongly foregrounded back wall inside the transparent aperture. The original foreground is intentionally retained.
- `qw0154-library-1207` and `1469`: tree-house/stump platforms cover most of the effect along their actual contours. Intended surface/path depth and fade behavior are needed to judge this.
- `qw0349-library-433`: layered flower-tree branches/canopy cover most of the effect. Intended path/surface depth and fade behavior remain unresolved.

These are not missing-resource claims. Original object evidence and final composed images are available locally. They are not repaired automatically by the exact-model aperture profiles and are not marked confirmed erroneous by this audit. Rendering all portal pixels above all foreground objects would hide the uncertainty by introducing incorrect depth.

## Test record

Fresh successful checks:
- Original pregame: 42 checks / 22 source files.
- Native title bridge: 32 checks plus source invariants.
- Shared artwork/input: 41 checks, run directly with existing cloud Mono because the wrapper expects a Unity-shaped path.
- Inventory/social: 200 source/geometry checks.
- Prior map transition: 200 assertions / 39 scenarios; explicitly insufficient as acceptance.
- Experience final: 107 queue/frame/spawn assertions, 23 full production popup pool/animation assertions, and lifecycle/wiring guards.
- Assassin real asset poses: 1,470 poses / 16,028 assertions.
- Skill audio client: 252 assertions.
- Native status mapping: 28 assertions with real poison packet fixture.
- New monster status lifecycle: 62 assertions.
- Focused Go scene/protocol/domain tests: attack-speed presentation, native icons, wire encoding, audio and local-enhancement computation passed.

Incomplete/failed executions:
- Intermediate `run-ui-contract.sh` run stopped at `live completed run reentry` during concurrent edits. Resolved afterward by the NPC worker; the parent reports final 2,152 UI and 212 endgame / 377 total plugin assertions passing. It is not a final failure.
- First artwork wrapper invocation required `UNITY_EDITOR_DIR`; direct equivalent managed compilation and execution subsequently passed.
- Standalone `test_agent_client.py` needs its host/runtime arguments and was not a valid transport execution. No transport pass claimed.

Full logs are in `/tmp/client-world-audit/`. No full-suite or Player-build claim.


## Follow-up: experience visual scene ownership repair

The scope was explicitly narrowed to the two demonstrated visual defects. No NetClient, packet layout, gateway, server, experience ledger or reward values were changed.

### Mechanism

- `WorldCombatNumberPresentation` now retains a local visual generation on pending entries, separately from the existing connection revision used for exactly-once consumption.
- A failed attempt on the current, non-switching frame has proven visual ownership. The actual scene-retirement hook drops only such owned pending entries and retains all previously observed revisions, so old snapshots cannot resurrect them.
- A notification first seen during switching or on a superseded frame has unknown ownership. It is preserved. If it later fails on a real current frame, it acquires that frame's generation. This avoids treating every loading-time reward as a duplicate.
- Successfully rendered EXP objects register in a dedicated active set. Scene retirement returns these objects to the existing pool, disposing only owned font textures/sprites; normal popup expiration and destruction unregister them. Other popup types are unchanged.
- Final local-player detachment also clears active EXP. Existing frame disposal checks that it still owns the registered transform before detaching, so a retired frame cannot clear a newer frame's player/popups.
- The map worker owns and added the two hooks after successful preflight and before old-world retirement: `session?.RetireCombatNumberSceneForIndependentRuntime()` and `CombatNumber.ReleaseExperienceForSceneTransition()`. A duplicate same-landing ready acknowledgment does not call them. No success/MapReady epoch hook is required.

### Verified before and after

- `/tmp/client-world-audit/xp-pending-before-after.log`: original production pending queue displays the known old-scene reward after a new scene becomes ready; repaired queue with the actual retirement API does not.
- `/tmp/client-world-audit/xp-object-before-after.log`: original production popup object remains active across retirement; the repaired production pool/spawn owner deactivates it.
- `Tests/WorldEntry/run-experience-replay-contract.py`: 107 assertions, including already-shown dedup, known old pending retirement, unknown transition receipts, late old immutable snapshots, genuine new-map rewards, equal-valued unknown first-time late receipts, subsequent scene generations, disconnect and reconnect.
- `Tests/WorldEntry/run-experience-scene-lifetime-contract.sh`: 23 assertions executing production spawn/pool/animation code, including already-visible object cleanup, no double-pooling, new-map reuse, natural completion, fallback texture disposal, missing-art failure, stack offsets, destruction and disconnect/reconnect.
- `Tests/Endgame/run-managed-compile.sh`: seven engine-free assemblies compile from all 652 current source files. Presentation changes are compiled by the focused production-owner contracts, not a complete Unity build.
- The status-icon 62-assertion regression was rerun after the XP changes and still passes.

### Explicit receive-order boundary

The existing XP wire carries target/value/type and no map identity. Server-initiated transfers contain an ordered Teleport packet, but client-initiated already-ready transfers intentionally omit it, and `sendAllEntities` is a sequence of individual entity packets rather than a guaranteed independent scene marker. Therefore an arbitrary first-time notification still in flight cannot always be classified as old or new from this client alone. It is preserved, not silently discarded. This repair makes no claim to remove all such genuinely late first-time notifications. A proposed ordered-ack redesign was considered and explicitly deferred; no server/protocol changes were made.

### Additional files changed by this worker for XP

- `Assets/FakeFantasyOnline/Runtime/Application/WorldCombatNumberPresentation.cs`
- `Assets/FakeFantasyOnline/Runtime/Application/ClientSessionCoordinator.CombatNumberPresentation.cs`
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/ReferenceFirstWorldInitialFrame.cs` (only ownership helper / presentation invocation)
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/CombatNumber.Spawn.cs`
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/CombatNumber.Pool.cs`
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/CombatNumber.PlayerTransform.cs`
- `Assets/FakeFantasyOnline/Runtime/Presentation/ReferenceAssets/CombatNumber.SceneLifetime.cs` and `.meta` (new)
- `Tests/WorldEntry/ExperienceReplayContract.cs`
- `Tests/WorldEntry/run-experience-replay-contract.py`
- `Tests/WorldEntry/ExperienceSceneLifetimeContract.cs` (new)
- `Tests/WorldEntry/run-experience-scene-lifetime-contract.sh` (new)

GameBootstrap hooks are the map worker's changes, not this worker's file ownership. No commit or push.


Final review: an independent read-only reviewer found no confirmed issue in the queue/frame/spawn/pool/disconnect cleanup. Its suggested missing ownership-acquisition case was added: unknown loading receipt → failed live-scene retry → next retirement. Final focused count is 107 queue/frame/spawn and 23 pool/animation/lifetime assertions. The earlier 104 result is superseded only by these added checks.

Final reproducible commands (cloud managed toolchain, not Unity):

```sh
source /workspace/shared/client-toolchains/env.sh
cd /workspace/shared/client-development
python3 Tests/WorldEntry/run-experience-replay-contract.py
bash Tests/WorldEntry/run-experience-scene-lifetime-contract.sh
bash Tests/Endgame/run-monster-status-icon-readiness-contract.sh
FFO_STATUS_ICON_WIRE_FIXTURE=/workspace/scratch/1cc826cfb9aa/status-icon-evidence/monster-poison-buff.bin python3 Tests/Endgame/run-status-icon-contract.py
FFO_COMPILE_DIR=/tmp/client-world-audit/managed-xp-final bash Tests/Endgame/run-managed-compile.sh
```

Only one complete map initialization is the active user expectation. These XP changes do not add a second load, resend scene snapshots or modify map readiness; cleanup occurs once at the actual retirement hook, and final player detach is idempotent.

Final worker freeze: 2026-10-13 06:48 UTC. All 16 worker-owned changed/new paths are enumerated in /tmp/client-world-audit/owned-files.txt. The report is ready to copy.
