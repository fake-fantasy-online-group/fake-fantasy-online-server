# 演劫洞天与试炼符钥

## Current upgrade

New runs use [seeded multi-floor maps, 1–150 difficulty tiers and telegraphed abilities](SEEDED-RIFTS.md). The single-arena population below describes legacy nil-Floor runs; admission, shared progress and compensation contracts still apply. The current completion flow is guardian experience, personal ground rewards, time-based difficulty unlocks and a native exit NPC. Difficulty is 阶; a map inside one run is 层. See also the [difficulty progression and compatibility notes](rift-tier-progression-2026-10-13.md).

## Scope and evidence

This implementation extends the existing 太上老君「演算之阵」story with original names: 演劫洞天 (instance), 洞天符钥 (entry key), 镇煞灵珠 (progress globe), 镇境劫影 (guardian). It reuses existing trial maps, monster templates, native combat, map readiness and session transfer. It does not replace native positional protocol packets; the negotiated `endgame.v1` extension supplies the panel and overlays.

Design inspiration is the documented key/tier/progression-orb loop in Blizzard's historical official [2.3 release](https://news.blizzard.com/en-gb/article/19859662/patch-2-3-0-now-live) and [2.4 release](https://news.blizzard.com/en-us/article/19998542/patch-2-4-0-now-live). All numeric balancing below is this game's original tuning, not a claim about current Diablo rules.

## Trial key formula

The existing trial quest, six-round cycle, experience curve and special daily prize remain. The server freezes up to five distinct characters, their opening levels and zero-based cycle (`0..5`) at the first actual damage in the instance. No action, damage or healing quota is required: a lower-level character can legitimately be carried.

For credited recipient `i`:

```
K_i = 1 + openingCycle_i
      + sum(over every other credited frozen member j)
          [1 + (openingLevel_i - openingLevel_j >= 10 ? 1 : 0)]
```

A member is credited after being present for at least half the encounter and last seen within 120 seconds of clear. Same-tick clears credit genuinely present members. Levels and cycle cannot be changed to inflate an already-open encounter. The upper bound is exactly `1 + 4*2 + 5 = 14`.

Examples:
- Solo: 1 through 6 keys over the six-round cycle.
- Five equal-level credited players, sixth round: 10 keys each.
- A level-60 mentor with four level-50 players, mentor's sixth round: 14 keys for the mentor.

Original quest completion is not withheld by this additional bonus gate. Otherwise legitimately present, noncredited or late-arriving players receive the original completion and one base key. Offline credited recipients have a durable, character-specific completion entitlement. These keys are permanently bound, item ID `1900028101`; no cube recipe creates them.

Completion persistence:
1. Opening writes `Trial.CompletionID` to each frozen character.
2. Clear produces an immutable `trial-<nonce>` run record with monster level and each credited character's exact key count, independent of online entities.
3. Map-ready and ordinary claim attempts asynchronously recover missing `QuestDeliver`, monster level and key count from that record. A new ordinary-trial entrance waits for the existing nonce's lookup to resolve, so rapid re-entry cannot replace an already-saved offline entitlement while recovery is pending.
4. Native claim atomically commits existing rewards, bound keys, cycle advancement, quest completion and a `trial-claim-<nonce>` receipt. The daily random prize derives from the frozen nonce, so bag-full retries cannot reroll it.
5. A completed trial scene cannot be emptied away before its completion record is durable. Graceful shutdown drains that write before the backend closes.

## Entry and admission

- Speak to the actual 龙城八仙使者 (map 7, `map_npcs` row `qw0007/83`, model 630, native script `Nbaxian`, position 9917/4734). SQL64 binds this existing NPC through `game_rift_entrances`; its original appearance, greeting and services stay intact. Ordinary 妖魔结界守卫 retain model 700 and their native trial service.
- Clients negotiate `endgame.rift-entrance.v1` to receive `hello.riftEntrances` (`mapId`, `npcId`, `npc`, `script`). The menu uses this server-owned binding and supplies the clicked live `npcEntityId` with `rift.start`. Admission validates that entity against its original database placement, scene, name, script and current native dialogue target. Old clients receive no new hello field and require an update for the new entrance.
- Solo is supported. A party's opener must be its leader.
- One opener spends exactly one unlocked, usable key to sponsor the entire run. Guests spend no keys.
- Freeze alive, nearby (500-pixel radius), same-scene party members in the trial's configured level range. Maximum five; no mid-run substitutions.
- Difficulty tier range (阶) is 1–150, independent of the run’s map-floor index (层), capped by the opener's durable unlocked tier or a verified prior completion entitlement. Restoring a higher entitlement does not spend a key or change the profile until the new-run transaction commits.
- Allocate an isolated instance and an immutable admission roster before the economic write; admission remains unpaid until key debit plus run record plus receipt commit succeeds.
- Definite rollback or discarded replay releases the unpaid reservation. Ambiguous storage results retain economic ownership until reconciliation.
- The UI invitation carries the exact run ID; polling must preserve that ID until admission. Joining records the guest's profile pointer atomically, then transfers on the following actor tick.
- A 60-second map-load window starts the encounter when everyone is ready or the window expires. Missing guests lose their admission slot; they cannot be replaced.

Before starting or joining another run, the server checks the prior profile run. Eligible unclaimed rewards or sponsor compensation block replacement, and an authoritative view of the old run is returned. Bag-full rewards cannot become undiscoverable through a later invitation.

## Combat and progression

Current seeded floors use generator v2, targeting 96–160 monsters according to verified walkable capacity and saved floor plans; see `SEEDED-RIFTS.md`. A small map can have fewer safe placements. The older trial-anchor fallback uses the following 104-monster population; these fixed counts are not the current seeded-floor density:

| Kind | Count | Death result |
|---|---:|---|
| Normal | 80 | 40 direct progress |
| Blue elite | 18 | One globe worth 200 |
| Yellow elite | 6 | Four globes worth 200 each |

Blue and yellow entities use actual `MonsterElite` combat classification as well as native elite ring and extension aura class. Guardian uses `MonsterBoss`. This makes existing elite/guardian traits execute rather than merely drawing an overlay.

- Progress target: 10,000; available pool: 11,600.
- Only registered first server-authoritative deaths contribute. Repeated packets, forged IDs and duplicate corpse callbacks do not.
- Globes are scene-owned world objects, not inventory items. An alive, map-ready admitted member automatically collects one within 75 pixels. Shared progress increments once.
- At 10,000, remove remaining trash and globes without ordinary kill rewards, then summon one real 镇境劫影.
- Rift trash gives no ordinary loot, experience, quest kills or respawn. Completion is the sole reward source.
- Tier scaling: HP ×1.08 per tier, attack ×1.04. Per-entity scaled bases survive native buff/debuff recomputation; per-instance lookup retains trial-level AI, skills and immunities without mutating global templates. Party HP factor: `1 + 0.6*(partySize-1)`.
- Elite HP multipliers: blue 4, yellow 8, guardian 40. Attack multipliers: 1.2, 1.4, 1.5. Integer rounding is upward with int32 saturation. Stat scaling clamps invalid tier, party-size and class inputs to `1..150`, `1..5` and `0..3` respectively before arithmetic; nonpositive base stats stay zero. Reward planning also clamps the difficulty tier.
- Timed target: 15 minutes inclusive. Hard failure: 20 minutes, checked before a guardian death at the boundary.
- Each player death adds five seconds to timed scoring. After five seconds, the native revive request restores at the instance entry checkpoint. Rift deaths consume neither revive items nor death XP/money/durability penalties.

## Rewards and exact identities

A guardian kill freezes eligibility from presence (at least half of elapsed run), last-seen grace and voluntary departure. It generates each eligible character's exact bound stack identities and experience entitlement once, then saves them before publishing ground rewards, automatic experience or the exit NPC. Completion does not require an extra damage/healing quota, a personal-tier prerequisite or being the opener.

Base reward:
- One equipment item
- One of the 28 星宿符文
- 净煞尘: `6 + floor((tier-1)/5)`
- Guardian experience: `floor(TrialRules.BaseExperience(frozenOpeningLevel, levels) * tier / 10)`, without trial-round or XP-card multiplication. Tier 1 is 0.1× the base, tier 10 is 1× and tier 150 is 15×; there is no additional 1× baseline. The server preserves the fraction until final integer division and saturates overflow. Normal character level-cap application still applies.

Timed loot bonus: one additional equipment item, one additional rune and four additional dust. This bonus does not multiply guardian experience.

Difficulty progression is independent of that loot bonus:

```
remainingSeconds = max(0, floor((900000 - (CompletedMS - StartedMS) - PenaltyMS) / 1000))
tierJump = max(1, floor(remainingSeconds / 180))
unlockedTier = min(150, max(previousUnlockedTier, completedTier + tierJump))
```

Remaining 0, 179, 180 or 359 seconds advances one tier; 360 advances two; 540 advances three. A successful overtime clear before the 20-minute hard failure still advances at least one tier. Higher prior unlocks never regress. Failure, voluntary departure before completion and absence of the existing completion eligibility grant no unlock. The completed run stores the earned entitlement; automatic experience, pickup, leave and closed-instance recovery transactions merge it into the character profile. A durable completed snapshot immediately shows the earned `unlockedTier`, using the existing DTO fields. Map-floor count does not affect this calculation.

Equipment uses native-drop-eligible bases only, the highest available requirement tier compatible with the recipient's frozen level, profession and sex, reserved socket count, and ordinary affix rolling. Original unique conversion-only bases do not become new native reward drops. Unique chance is `min(60,20+2*tier)%`; at tier 10 or above the first equipment item is guaranteed unique. Exact equipment/card UIDs, socket counts, rolls and bound flags are saved in the run's grant payload before publication; retries never regenerate them.

### Live pickup and exit

- After completion is durable, each online, map-ready eligible member receives guardian experience through an automatic transaction. Experience and its one-time marker, saved pet entitlement and tier progression commit together; bag capacity cannot block this step.
- Equipment, runes and dust appear as real native ground entities near the guardian's death location, retaining their saved identities and personal character ownership. Manual pickup and pet pickup use the same atomic per-stack settlement. A full bag leaves the entire stack and entitlement untouched. Picked indices are persisted, so retries, reconnects and later recovery cannot award them again.
- Placement uses stable character/grant order, verified walkability and separated positions. A 64-pixel exclusion around the exit NPC prevents its collider from covering a reward. Narrow maps use a denser fallback; an item without a safe position stays in the durable entitlement for closed-instance recovery rather than being placed in a wall.
- The instance's completion view has `claimable=false`. A live `rift.claim` cannot bypass ground pickup. Reconnect reconstructs only outstanding ground items and does not replay the initial landing effect/sound.
- The real native 归元道使 appears after durable completion, using model 630, sprite `npc630` and portrait `N630`. Its dialogue's `trialleave` action requires a living admitted character within 200 pixels and routes through the durable `rift.leave` transaction. It exits without silently claiming unpicked items. The existing direct `rift.leave` operation remains as the compatibility/abandonment fallback; it is not newly gated on the NPC.

A member is marked claimed only after experience and every item are settled. Once all eligible members are settled, the logical run state becomes `closed`; this does not immediately destroy the live instance or shorten its existing cleanup deadline.

### Closed-instance recovery

After the live instance no longer exists, `rift.claim` is the bulk recovery path for that character's still-unpicked items and any not-yet-settled experience/pet grant. Already collected items and already settled experience are skipped. A full bag rolls the recovery back without consuming any remaining entitlement. Character inventory/experience, tier progression, run claim marker and request receipt commit atomically. Duplicate requests replay their original receipts; a different request cannot settle an already-claimed member. All reward operations serialize against run checkpoints and other economic mutations.

## State machine and recovery

```
reserved -> active -> boss -> completed -> closed
               |        |
               +--------+--> failed (hard timeout)
reserved/active/boss -> aborted (voluntary abandonment or empty grace)
```

Client-only projections:
- `reward_pending`: guardian is dead but exact grants are not yet durable; poll, do not publish rewards or a new unlock yet.
- `interrupted`: recoverable sponsor compensation after server interruption, or infrastructure loading/spawn failure.
- `closed`: the durable run has all eligible grants settled, or the client projects a durable completion whose instance no longer exists. A live closed instance still uses its normal exit/cleanup flow. Once the instance is gone, outstanding grants remain remotely recoverable; the next run can open after those rewards are settled.

`rift.start(runId)` reenters a live admitted run, including completed runs. Completed runs must be polled so cleanup becomes a `closed` projection. Completed exits preserve reentry admission while peers keep the instance alive; active voluntary leave permanently withdraws admission and reward eligibility. During an active challenge, native portal/teleport exits cannot bypass the durable leave operation. The new completion NPC's native `trialleave` routes through it; the old direct `rift.leave` path remains supported.

Disconnects preserve the frozen character ACL regardless of later party changes. An active encounter permits a 120-second reconnect grace after the last seen member; passing that grace aborts it. A durable terminal instance can instead stop immediately once no players remain. Session login rejoins only an existing admitted instance; it never lazily recreates a lost run. Party changes do not alter the frozen roster.

Server restart does not resume a saved monster population. Missing nonterminal instances expose an explicit interruption action. Only the original payer receives exactly one bound replacement key, atomically marked `failed/compensated` with a receipt. Other members receive no compensation key. Ordinary timeout, voluntary abandonment and completed runs are not refundable.

Run-only checkpoints use asynchronous revision CAS. There is only one in-flight checkpoint or run economic mutation. Gameplay deaths arriving during an economic commit are captured with their authoritative event timestamp and, for monster deaths, the present-member snapshot. Monster and player deaths replay in their original order after resolution, even if native corpse cleanup removed the entity. Thus storage latency cannot reduce frozen completion time, turn a timely kill into a timeout or apply a post-guardian death penalty to the completed score. Lost-ACK/replayed character commits reload the durable run before releasing the actor reservation, preventing stale claim markers or run revisions. Unknown storage outcomes never trigger a speculative refund.

Once terminal state is durable and no run economic write is pending, the instance sets a 120-second cleanup deadline. On an eligible tick at or after that deadline, remaining players are returned and the instance stops; unresolved persistence/economic writes defer cleanup, and an empty eligible terminal instance can stop earlier. Settling every grant changes the logical phase to `closed` but no longer sets a one-second forced-exit deadline. Durable unpicked grants and unsettled experience remain recoverable after instance destruction. Graceful shutdown drains accepted character commits and completed run/trial entitlements before the storage backend closes.

## Saved-record and protocol compatibility

- Run JSON versions 1 and 2 remain readable. Existing exact grants, including previously saved experience amounts, are honored rather than rerolled or recomputed under the new reward formula. New grants use the current experience rule.
- Optional `RewardPos`, per-grant `Collected` and `ExperienceClaimed` fields use the existing run JSON storage; old records default to no separately recorded pickup/experience settlement, while the existing member `Claimed` marker still prevents duplicate awards.
- `CompletedMS` already exists in saved runs. A legacy successful record without it receives no invented bonus time: progression conservatively uses zero remaining seconds and the minimum one-tier jump. Historical higher unlocks are retained.
- If a previously claimed record yields a higher verified unlock than its old profile projection, the next admission uses that entitlement and persists it atomically with the new run. Missing keys or failed commits do not mutate the profile early.
- This reward/progression update adds no SQL migration, table, item or art resource. It preserves existing player data and reuses `tier`, `floorIndex`, `maxTier`, `unlockedTier` and `remainingSeconds`; it adds no rift DTO keys or schema version. Completion and closed views freeze `remainingSeconds` at guardian death rather than continuing to count down.

## Source map

- State machine, scaling and loot reward plan: `internal/domain/rift.go`
- Frozen completion time and tier progression: `internal/domain/rift_progression.go`, `internal/game/scene/endgame_rift_progression.go`
- Guardian experience formula: `internal/domain/endgame_rift_experience.go`
- Persisted trial fields: `internal/domain/trial.go`
- Operations/start/join/claim/leave: `internal/game/scene/endgame_rift.go`
- Durable run DTO, profiles and client projections: `endgame_rift_state.go`
- Router admission and reconnect ACL: `endgame_rift_router.go`, `router.go`, `internal/session/session.go`
- Native spawn/death/revive integration: `endgame_rift_combat.go`, `scene.go`, `status.go`, `revive.go`
- Timer/globes/checkpoints: `endgame_rift_tick.go`
- Saved equipment/rune/material grants: `endgame_rift_rewards.go`
- Ground reward placement/pickup, automatic experience and native completion NPC: `endgame_rift_completion.go`, `loot.go`, `pet.go`, `trial.go`
- Shutdown write drain: `endgame_rift_shutdown.go`
- Ordinary trial cohort, offline entitlement and native claim: `endgame_trial.go`, `endgame_trial_claim.go`, `trial.go`
- Native admission, loading, lifecycle and bypass guards: `dispatch.go`, `map_ready.go`, `dungeon.go`, `transfer.go`
- Shared economic helpers and backend: `endgame.go`, `endgame_commit.go`, `internal/store/endgame*.go`

## Verification and limits

Automated tests cover remaining-time boundaries (0/179/180/359/360/540 seconds), tier-150 saturation, no regression of higher unlocks, legacy claimed-run admission, delayed-death ordering, guardian experience scaling, automatic experience exactly once, live pickup/closed recovery, ownership/range/bag guards, completion NPCs and safe reward layout, in addition to formula bounds and mentorship threshold, invalid rosters, real native zero-progress-to-guardian combat, duplicate deaths/globes, elite classification, timed/death boundaries, sponsor debit/rollback, bag/receipt commit publication, lost-ACK run recovery, durable leave, native exit blocking, one-time orphan compensation, completed-to-closed UI projection, pending-old-reward protection, same-tick carried trial members, offline durable entitlement and frozen 14-key native claim.

The implementation has not been deployed or tested against a live PostgreSQL database, live players or a Unity runtime. Numeric balance and rendered overlays still require gameplay verification. The supported restart contract is compensation plus durable completed grants, not monster simulation resumption.
