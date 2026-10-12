# V5 battle skills: implementation and verification

## Runtime contract

- The catalogue contains 149 class battle skills. Alchemy 11005 remains a life skill.
- Permanent allocation is an integer hard rank, maximum 20, paced by `min(20, character level - original unlock level + 1)`.
- The level-derived budget is 99 points at level 100. Equipment cannot unlock an unlearned skill, satisfy a prerequisite, or provide training synergy points.
- Five explicit equipment scopes are all/class/branch/school/single. Profession keys are 1-based, while character races remain 0-based. Source IDs identify individual authored budget entries, so an item's all+single can stack but duplicate collection cannot.
- Raw integral effective ranks use decimal strings backed by arbitrary-precision integer addition. No gameplay ceiling is imposed on soft rank. Conversion to finite engine numeric fields fails explicitly rather than wrapping or silently choosing a lower rank.
- `SkillTable.Get` evaluates the V5 curve only after all 149 real first/fifth anchors pass startup preflight. Item procs use this base evaluator, without gear ranks or training/native synergy.
- New `skill.upgrade` commands use the existing transactional endgame aggregate revision, receipt and candidate snapshot. No point deduction is published before successful commit; retries replay the original receipt. Old allocation packets cannot mutate V5 skills. Old books remain in inventory until a separately approved redemption scheme exists.
- Migration 54 audits legacy rows, records before-images/checksums, resets class battle allocations once to the level-derived budget, preserves life skill rows, and never multiplies five old ranks into twenty points.

## Authored curves

`skill-curves.json` documents each identity, rank 1 and 20 anchors, post-20 slope and actual carrier. These are new balance values, not a claim that they match a historical server. The fourteen priority damage and two healing formulas preserve the approved anchors and exact intermediate rational arithmetic.

Other skills preserve original targets, weapons, statuses and primary behavior, adding a specific non-saturating carrier: same-channel damage; fixed HP/MP; a consumed shield; actual taunt priority; persistent recovery/reflect; flat weapon/defensive attributes; or a targeted native modifier. Probabilities, resource percentages, geometry and control duration retain independent safe bounds.

Seven one-shot or stealth states (10011, 10130, 10208, 10328, 10329, 10419, 10420) capture fixed damage before status consumption, so removing the state does not erase the benefit before the hit. Independent flat status contributions are additive and cannot overwrite original percentage values or another source's flat benefit. Added damage for formerly status-only controls is guaranteed-hit with an explicit zero base-attack multiplier; it cannot introduce a new MISS failure for the original control.

## Synergy graph differences from the design candidates

The implemented graph has 86 edges: 58 retained native relationships using the raw effective source rank, and 28 training relationships using hard points. Every target has at most three unique sources. Bilateral relationships read one immutable snapshot and do not recurse.

Two optional candidate relationships are deliberately omitted instead of advertising a nonexistent effect:

1. 10312 → 10319, bonus against undead/demons: the existing runtime has no audited creature taxonomy for that distinction.
2. 10228 → 10213, reduced trap arming delay: this runtime has no independent trap arming timer; its preparation time is zero.

The 10217 → 10216 freeze-duration relationship reads the actual status-table duration when the skill application omits its own duration, then applies the bonus to the consumed status payload.

## Persistence and source loss

- Projectiles keep their accepted payload. Persistent statuses and traps never grow retroactively from later equipment.
- Removing a skill source or hard allocation downgrades persistent effects using the lower cast/current raw and hard source values. Used shield capacity and elapsed time are never refunded.
- Trap geometry retains an equipped item's originally captured expansion and shrinks when that item is removed.
- An external caster cannot remain a verifiable rank source after either participant leaves the scene. V5 external-source effects are explicitly revoked on departure and omitted from transfer/logout snapshots; legacy effects keep their existing behavior. Self-cast V5 effects are re-bound to the receiving entity and revalidated against loaded equipment and allocation before attributes are exposed.
- Skill rank views are refreshed after relevant stat/loadout changes with duplicate suppression. They include authoritative current and next effective-level effect lines, independent hard-point investment eligibility and readable blockers.

## Verified scope

- Read-only loading of actual seeded PostgreSQL skill/status/overlay data in an isolated copied disposable cluster: 750 class rows, 149 battle identities, 1,890 state definitions.
- Adjacent carrier growth through ranks 1–102 and boundary samples 255/256, 65535/65536 and 1,000,000; original identity and state-level resolution; no source-anchor mutation.
- Actual scene tests for exact damage and healing, six healing skill-bonus equipment items, native/training modifiers, consumed stealth/one-shot damage, original control hit eligibility, table-backed freeze duration, spent-shield downgrade, trap expansion source loss, transactional allocation commit/replay/failure, and detached-source transfer/logout behavior.
- All 28 retained training edges alter a consumed payload field with their source invested.
- 258 legitimate loadout/build and unlearned first-investment preview scenarios across five classes and both sexes verify rank payload encoding. Largest JSON is 25,016 bytes, below the 60,000-byte limit. Healers correctly have 29 battle rows, other classes 30.
- Domain, scene and data package suites plus focused race checks pass. Actual-loader and packet checks have reproducible tests and fixture SHA256 provenance.

This is structural, semantic and numeric regression evidence. It is not production migration approval, gameplay balance certification, live visual acceptance, or a multiplayer tick-latency capacity claim.
