# Warrior and swordsman audit repairs

Scope: local server behavior and migration-derived scene fixtures. No deployment,
production database access, client asset publication, or balancing changes.

## Behavior repaired

- ZS-A07 / ZS-A25: apply the learned native armor-break payload before
  `CalculateStrike`, so the current hit uses the reduced defense/evasion.
  Native source, status resistance, immunity, strength, and duration remain in
  the ordinary status pipeline. A miss can still occur; armor does not imply a
  guaranteed hit. No learned donor means no borrowed state.
- ZS-A08 / ZS-A27 / ZS-A30: remove an undocumented donor-learning gate.
  These geometry/range grants use the native rank-one donor when unlearned and
  the owner's effective donor rank when learned, including current equipment and
  legendary composition. Nested A27 + A08 + A04 inherits the full rectangle once.
  Borrowing does not grant learning, permit an active donor cast, copy donor
  damage/status payloads, or bypass the recipient's costs and range limits.
- ZS-A10: convert theoretical mana drain after every skill-preparation modifier
  has contributed. TY-A19 is now included alongside ZS-U02 / ZS-D04, regardless
  of remaining target mana. Added base damage still enters ordinary scaling.
- ZS-A17: suppress both native percentage durability cost and ordinary attack
  wear for this cast, without changing its mana or class-set blood cost.
- JK-A23: a successful native instant-death segment can trigger the same one-per-
  cast burst as direct lethal damage or a pre-hit half-health target. The burst
  uses the preceding native damage coefficient, not remaining HP, and does not
  repeat on the subsequent death branch or rerun other on-hit handlers.
- ZS-D03: require an actual negative defense affix on the owner's armor-break
  state, not only matching skill metadata. Native passive vulnerability 10012
  shares the 10009 casting snapshot and can independently land after defense
  reduction is resisted. That reachable branch no longer grants false reduction.

Shared target/geometry work separately repairs native ZS-A23 primary selection.
Its selected/no-selected and class-set combinations are covered here.

## Preserved semantics and approved completion

- JK-A14: preserve the original primary-only shield-break rule. The first
  implementation specification explicitly bases the extra loss on the primary's
  actual damage plus absorption. A13 expansion retains native damage and silence
  on side targets, but does not invent extra shield destruction on them. Tests
  distinguish primary shield loss, secondary ordinary absorption, unshielded
  damage, the 400%-attack cap, and a third power.
- JK-U04: native Taunt is caster-centered with distance zero. Its 100-unit radius
  becomes 135, including the boundary, and retains taunt, A25 damage, D04 eligibility
  and the swordsman set stack. The approved completion removes the inert distance modifier and describes
  the real radius-only benefit. No remote center or immediate pull is invented.
- JK-U08: the user approved replacement with equipment-active movement-slow
  and root immunity. Equipping immediately strips old negative MoveSpeed affixes
  and pure BlockMove roots, following the existing equipment-immunity convention.
  New applications strip the same components. Unequipping admits new impairments
  but does not resurrect cleansed ones. ControlStun, freeze, petrify and attack-
  speed penalties remain. In particular StatusSlow levels 11..15 are native
  Shackle attack-speed penalties, not movement slows. Mixed statuses retain all
  other payloads, source, original expiry/tick schedule and shield balances.
  No timer, new skill, PvP eligibility, equipment slot/level or item name changes.
  Native monster 1099/skill12007 is a verified movement-slow source; pure root
  component tests use the restored 10111 and scoped 1034/41 definitions, not a
  fabricated monster root. Mixed payload tests are consumer-boundary tests.
- Explicit learning contracts remain unchanged: A07 / A25 require learned armor
  break; A24 explicitly retains native range when its donor is unlearned; A21
  retains its explicit unlearned basic-attack fallback. Unlike these powers,
  A08 / A27 / A30 do not state a donor-learning requirement.

## Verification

`legendary_melee_audit_regression_test.go` uses the native repair fixture and the
actual `onUseSkill` / `stepCasts` entry, preserving authored costs, windups,
projectile timing and the meteor's 2% instant-death chance. Deterministic random
input visits the success branch without raising that probability.

The new cases cover native controls, original skills, one/two/three powers,
warrior and swordsman sets, full/zero enemy mana, immunity, unlearned armor,
misses, primary/secondary shields, nonrecursive explosions, and ownership/expiry.
The frozen original rows are amended from the current native migrations through
104 by the shared fixture. These are not PostgreSQL loader or live-client tests.

The older D03 synthetic test was corrected from a vulnerability-only state to
actual native negative defense. The new independent-resistance test failed before
the fix (100 incoming damage became 50) and passes afterward (remains 100).

`legendary_warrior_borrow_audit_test.go` additionally verifies unlearned and
learned donors, native rank 1 / 5 / 20, no active skill unlock, native row
immutability, baseline and one/two/three powers, dragon set costs, native armor
and stun on actual secondary targets, 150x50 and 180x95 rectangle boundaries,
and exact 300-unit admission versus 301-unit rejection.
