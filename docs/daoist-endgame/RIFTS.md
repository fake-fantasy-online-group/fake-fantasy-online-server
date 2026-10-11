# 演劫洞天与试炼符钥

## Current upgrade

New runs now use [seeded multi-floor maps, 1–150 difficulty tiers and telegraphed abilities](SEEDED-RIFTS.md). The single-arena population below describes legacy nil-Floor runs; entry, shared progress, rewards and compensation contracts still apply. Difficulty is 阶; a map inside one run is 层.

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

- Speak to the actual 妖魔结界守卫 on an existing trial entrance map.
- Solo is supported. A party's opener must be its leader.
- One opener spends exactly one unlocked, usable key to sponsor the entire run. Guests spend no keys.
- Freeze alive, nearby (500-pixel radius), same-scene party members in the trial's configured level range. Maximum five; no mid-run substitutions.
- Difficulty tier range (阶) is 1–150, independent of the run’s map-floor index (层), capped by the opener's durable unlocked tier.
- Allocate an isolated instance and an immutable admission roster before the economic write; admission remains unpaid until key debit plus run record plus receipt commit succeeds.
- Definite rollback or discarded replay releases the unpaid reservation. Ambiguous storage results retain economic ownership until reconciliation.
- The UI invitation carries the exact run ID; polling must preserve that ID until admission. Joining records the guest's profile pointer atomically, then transfers on the following actor tick.
- A 60-second map-load window starts the encounter when everyone is ready or the window expires. Missing guests lose their admission slot; they cannot be replaced.

Before starting or joining another run, the server checks the prior profile run. Eligible unclaimed rewards or sponsor compensation block replacement, and an authoritative view of the old run is returned. Bag-full rewards cannot become undiscoverable through a later invitation.

## Combat and progression

The live instance creates 104 real native monsters at existing trial spawn anchors:

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

A guardian kill freezes eligibility from presence (at least half of elapsed run), last-seen grace and voluntary departure. It generates each eligible character's exact bound stack identities once, then saves them before publishing a claimable completion.

Base reward:
- One equipment item
- One of the 28 星宿符文
- 净煞尘: `6 + floor((tier-1)/5)`
- Exact experience: half the ordinary trial's base experience, without XP-card multiplication

Timed bonus: one additional equipment item, one additional rune and four additional dust. Completion unlocks the completed tier; timely completion unlocks the next tier, capped at 150.

Equipment uses native-drop-eligible bases only, the highest available requirement tier compatible with the recipient's frozen level, profession and sex, reserved socket count, and ordinary affix rolling. Original unique conversion-only bases do not become new native reward drops. Unique chance is `min(60,20+2*tier)%`; at tier 10 or above the first equipment item is guaranteed unique. Exact equipment/card UIDs, socket counts, rolls and bound flags are saved in the run's grant payload before claim; retries never regenerate them.

Bag-full claim leaves the exact grant untouched. Character inventory/experience, run claim marker and request receipt commit atomically. A duplicate request replays its receipt; another request cannot claim an already-claimed member. Claims serialize against run checkpoints and other economic operations.

## State machine and recovery

```
reserved -> active -> boss -> completed -> closed
               |        |
               +--------+--> failed (hard timeout)
reserved/active/boss -> aborted (voluntary abandonment or empty grace)
```

Client-only projections:
- `reward_pending`: guardian is dead but exact grants are not yet durable; poll, do not claim.
- `interrupted`: recoverable sponsor compensation after server interruption, or infrastructure loading/spawn failure.
- `closed`: durable completion with no live instance. Rewards remain claimable remotely; the client may open the next run after outstanding rewards are settled.

`rift.start(runId)` reenters a live admitted run, including completed runs. Completed runs must be polled so cleanup becomes a `closed` projection. Completed exits preserve reentry admission while peers keep the instance alive; active voluntary leave permanently withdraws admission and reward eligibility. Native portal/teleport exits cannot bypass the durable leave operation; native trialleave routes through it.

Disconnects preserve the frozen character ACL regardless of later party changes. The existing instance is retained for 120 seconds when empty. Session login rejoins only an existing admitted instance; it never lazily recreates a lost run. Party changes do not alter the frozen roster.

Server restart does not resume a saved monster population. Missing nonterminal instances expose an explicit interruption action. Only the original payer receives exactly one bound replacement key, atomically marked `failed/compensated` with a receipt. Other members receive no compensation key. Ordinary timeout, voluntary abandonment and completed runs are not refundable.

Run-only checkpoints use asynchronous revision CAS. There is only one in-flight checkpoint or run economic mutation. Gameplay deaths arriving during a leave/claim commit are captured by value and processed after resolution, even if native corpse cleanup already removed the entity. Lost-ACK/replayed character commits reload the durable run before releasing the actor reservation, preventing stale claim markers or run revisions. Unknown storage outcomes never trigger a speculative refund.

Terminal instances allow 120 seconds for exit/claim; an all-claimed run closes promptly. Durable grants remain available after instance destruction. Graceful shutdown drains accepted character commits and completed run/trial entitlements before the storage backend closes.

## Source map

- Pure formula, state machine, scaling and reward plan: `internal/domain/rift.go`
- Persisted trial fields: `internal/domain/trial.go`
- Operations/start/join/claim/leave: `internal/game/scene/endgame_rift.go`
- Durable run DTO, profiles and client projections: `endgame_rift_state.go`
- Router admission and reconnect ACL: `endgame_rift_router.go`, `router.go`, `internal/session/session.go`
- Native spawn/death/revive integration: `endgame_rift_combat.go`, `scene.go`, `status.go`, `revive.go`
- Timer/globes/checkpoints: `endgame_rift_tick.go`
- Saved equipment/rune/material grants: `endgame_rift_rewards.go`
- Shutdown write drain: `endgame_rift_shutdown.go`
- Ordinary trial cohort, offline entitlement and native claim: `endgame_trial.go`, `endgame_trial_claim.go`, `trial.go`
- Native admission, loading, lifecycle and bypass guards: `dispatch.go`, `map_ready.go`, `dungeon.go`, `transfer.go`
- Shared economic helpers and backend: `endgame.go`, `endgame_commit.go`, `internal/store/endgame*.go`

## Verification and limits

Automated tests cover formula bounds and mentorship threshold, invalid rosters, real native zero-progress-to-guardian combat, duplicate deaths/globes, elite classification, timed/death boundaries, sponsor debit/rollback, bag/receipt commit publication, lost-ACK run recovery, durable leave, native exit blocking, one-time orphan compensation, completed-to-closed UI projection, pending-old-reward protection, same-tick carried trial members, offline durable entitlement and frozen 14-key native claim.

The implementation has not been deployed or tested against a live PostgreSQL database, live players or a Unity runtime. Numeric balance and rendered overlays still require gameplay verification. The supported restart contract is compensation plus durable completed grants, not monster simulation resumption.
