# Dungeon difficulty and equipment split (unpublished work)

Base: origin/main a0ede0a. This branch does not include the separate density or
invulnerability/reflection changes. No deployment, publication, or player-data
migration has been performed.

## Difficulty

The shipped seven native menus (20100, 20300, 20502, 20600, 20701, 20801,
20901) use ordinary gameplay routing, not GM privileges. Floors inherit a run's
selected difficulty. Six additional verified native grouped instances (23800,
24400, 24500, 26800, 26900, 30600) gain normal/hard options through their existing
server-provided NPC transport menus. Original destination indices and unrelated
options, including wedding entry, remain unchanged. The complete supported
scope is 13 dungeon families / 17 maps. Existing party runs cannot silently change difficulty.
Multi-floor runs retain their existing lifetime, so changing difficulty may
require waiting for that run to end, not merely leaving one floor.

Normal: boss HP 50%, outgoing damage 50%; other hostile monster HP 50%.
Hard: boss outgoing damage 150%, hit 200%; other hostile HP and damage 150%.
HP/hit derive from the immutable native monster definition. Damage is applied
once to the delivered hit or DOT tick, including magic/fixed additions, never to
both attack stats and damage. DOT captures difficulty when applied so source
despawn cannot change its multiplier. Experience is unchanged. Rift, trial,
public maps, and event instances outside this admission family are not scaled.

## Stable catalog identities and provenance

The embedded domain/dungeon_equipment_manifest.json is an explicit ID manifest:

- 42 dungeon special weapons (including eight newly assigned temple ancients): hard ID = 1800000000 + original ID.
- 47 confirmed class-set members: the same hard item ID rule.
- Five hard suit IDs: 1800000001 through 1800000005.
- Original item IDs and suit IDs remain ordinary variants.
- 18 level-3/8/14 quest weapons keep original IDs and receive the approved
  explicit extra-affix values. Level 14 activates six weapons / eight profession
  branches; the 12 level-3/8 weapons have no mechanisms. Only the six level-3
  weapons gain beginner eligibility; existing requirement fields stay unchanged.

Names gain the literal prefix `真`. They are presentation only, never lookup
keys. Equipment appearance, icon, requirements, durability and base stats are
cloned from native definitions. Hard weapon innate affixes are replaced by the
approved explicit values, not multiplied again on each load. Original class-set
bonuses restore the imported 2/4/6/8-piece matrix. Hard sets retain the current
52 numerical rows and server-owned 2/4-piece mechanisms; `EnhancedClass` isolates
those mechanisms and auras from ordinary and mixed gear.

Registration occurs in LoadItems after all native definitions are loaded. It
rejects ID conflicts and stages changes before applying them. New IDs are
registered on every startup, independent of player instance serialization.

411 explicit original item/map/monster source triples are allowlisted. An
original drop must first roll normally; only then may a hard instance replace
that exact item with its hard variant. This does not change unrelated loot or
add equipment to monsters which did not originally drop it. Sources on maps
without an enabled hard admission remain ordinary pending entrance work.

## Rift exclusion

HardDungeonOnly is a dedicated source tag, not a name test or no_type_drop.
Enforcement covers generic native eligibility, explicit Rift pool validation,
completion reward selection, normal-boss extras, inherited native loot, ground
spawning, persisted reward ground reconstruction, pickup, and direct claim.
Old ordinary IDs can remain in Rift pools. Even a malformed pool with a hard ID
must not grant it. No Rift difficulty/tier is used as dungeon difficulty.

## Player-data boundary / release gate

There is no automatic player migration. Restoring old suit templates changes
what already-owned old-ID suits do even without changing their saved rows.
Therefore this work must not be deployed until the owner decides whether to
convert already-owned class-set instances to hard IDs. A migration, if later
authorized, must retain UID, refinement, sockets, rolled affixes, binding,
durability, ownership, and every other instance field, including warehouse,
mail, stall, and other durable inventory locations. No such migration ran here.

## Known remaining validation / scope

- Weapon mechanisms implement and activate all 48 approved higher-level weapons /
  56 profession branches: 42 true dungeon IDs plus six level-14 quest original IDs.
  See special-weapon-mechanisms-coverage.md for branch-level evidence and the
  distinction between definition tests and actual-cast integration tests.
- Acquisition previously unresolved is now explicitly assigned by the user:
  twelve ancients belong in temple small-monster drops; level 3/8/14 belong to
  the specified quest chains. This is new design assignment, not a claim that
  every missing historical implementation was recovered.
- All 89 registered hard items have at least one supported acquisition source.
  Of 467 explicit item/map/monster tuples, 407 are in the supported 17 maps.
  The remaining 60 tuples are on 21601–21604 (unverified external admission)
  or 24800 (activity/sentinel entry); these maps keep original drops and do not
  acquire hard mode merely because a DungeonDef exists.
- Static client verification confirms signed 32-bit item IDs, server-driven
  name/description/quality/slot/avatar, and signed 32-bit 804d icon-map keys.
  All 89 item icons have positive actual small_map values and nonempty native
  avatars; new IDs reuse them without new model resources.
  An actual Unity client play/display smoke test has not been run. Existing
  difficulty text incorrectly promises extra XP and up to 24x boss HP;
  functionality alone does not correct those descriptions.
- Relevant sparse-checkout fixtures were restored from existing HEAD, without
  synthesizing test data. Full data, domain, scene, session, and gameserver tests pass;
  combined quest/dungeon/weapon/set focused race tests pass across four packages.
  Real database LoadItems
  integration remains unverified: local PostgreSQL 127.0.0.1:55432 is unavailable.

## Follow-up acquisition completion

The four existing temple ancients (1326,1626,1826,1928) retain their original
0.12% rows. Eight newly assigned ancients (1027,1028,1327,1526,1527,1627,1827,1929)
are appended at 0.12% each only to exact temple map/monster pairs: 20901 with
3111–3114; 20902 with 3111–3113. Each supported small-monster pool now has twelve
ancients at the same configured individual rate. Aggregate ancient opportunity
increases; no item is guaranteed. Existing level penalties and total loot cap
remain unchanged. Runtime augmentation does not mutate shared monster tables,
skips already present items, and never extends Rift or activity reuse of the
same monster IDs. Hard conversion uses the same exact source manifest.

Quest rewards use existing completion, not a new protocol: level 3 at 85/87;
level 8 at 36/37; level 14 only at the final quest 42 of 40→41→42. Sword and
assassin routes each receive both weapon-type alternatives; other routes receive
one matching weapon. Character Race is the preselected growth route even before
employment, so beginner rewards are not random or missing. Migration 100 and the
new list-capable reward loader must ship together. No completed-quest backfill
or player-inventory rewrite is included.

Integration sequence: init/upgrade 99 publishes the paging-capable rift rules
without activating them, 100 adds quest special weapons, and 101 restores pet
merchant supplies. Each has matching fresh-init and standalone upgrade bodies.
No existing player items are converted and no deployment is performed.

Final completion verification after freezing production and tests: five affected
packages passed, including scene in 167.916 seconds; combined four-package race
passed, including scene in 18.289 seconds. The logs are dungeon-complete-five-packages.log
and dungeon-complete-race.log. PostgreSQL-backed integration tests remain skipped
without the database, so package success is not evidence of a live SQL migration.
