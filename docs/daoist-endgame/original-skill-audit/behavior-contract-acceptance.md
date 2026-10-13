# Five-profession behavior-contract acceptance

Scope: `endgame.skill-behavior.v1`, gated by growth V5. These are independent acceptance criteria, not permission to copy the historical resource table over current gameplay. The local backup is unverified historical evidence; contradictory fields need an explicit row-level decision.

## Evidence and identity gates

- E01: 149 historical profession combat IDs; 146 current resource IDs; profession counts 30/30/30/29/30 versus 30/30/30/26/30.
- E02: Exactly 68 modified common IDs plus three absent healer IDs: 10316 清身术, 10318 解咒术, 10321 防御术. Every one of the 71 differences must have a disposition; the other 78 IDs also receive identity/behavior regression coverage.
- E03: Restore the eight server-authoritative identities 10014 破魔, 10024 维护, 10118 风斩, 10218 解除陷阱, 10229 陷阱回避, 10306 净化术, 10307 祝福术, 10327 神术：鹰眼.
- E04: Missing raw-resource rows must still have independent identity, icon, hover/card, hotbar and allowed target behavior. Existing audited tree coordinates remain unchanged; no new position or artwork is invented.
- E05: Decisions for conflicting backup descriptions/geometry remain explicit. In particular, 10212 cannot be called a historically restored theft mechanism merely because its name or prose changes. Its negotiated contract is explicitly null while its native resource identity remains available; old connections omit the contract key. This hold must not invalidate the other148 contracts.

## Protocol and lifecycle gates

| ID | Scenario | Required result |
|---|---|---|
| P01 | Old hello / no growth | No behavior capability or contract key; original packet layout unchanged. |
| P02 | Growth-only hello | Existing V5 rank rows remain parseable by old strict clients; contract key is completely absent. |
| P03 | Behavior requested without growth | Behavior is not negotiated. No accidental partial feature activation. |
| P04 | Growth plus behavior | Successful hello echo precedes the first usable typed rank snapshot. All profession rows contain identity. |
| P05 | Repeated hello, downgrade, failed hello | No stale/new capability mixture or partial state publication. Downgrade drops typed contracts. A failed refresh preserves the previous valid state. |
| P06 | Snapshot before hello, missing/delayed snapshot, malformed row | New client does not use stale external MP or infer targeting from prose; invalid snapshots publish nothing. |
| P07 | Map transfer | Connection capability and coherent current data survive as designed; map-specific target state does not leak. |
| P08 | Reconnect / other character / other server | Old capability, rank contracts, target data and freshness state are cleared until the new handshake. |
| P09 | Old client to new server / new client to old server | Legacy behavior continues without unknown-key decode failure; no forged behavior readiness. |

## Runtime and presentation gates

| ID | Scenario | Required result |
|---|---|---|
| R01 | All 149 identities | Exact ID membership and profession/name/icon/kind; no life skill added to combat budget. |
| R02 | Current and next | Learned current rank and next-effective-rank payloads use the same resolved definitions as cast admission. Unlearned current is null, never borrowed from next. |
| R03 | Known broken MP cases | 10306 rank1 costs48, not polluted100; 10307 rank1 costs35, not polluted120. Character MP60/50 is admitted when all other conditions allow. |
| R04 | Equipment / high ranks | Actual +skill items change typed MP and behavior according to effective rank. Broken/removed gear loses soft ranks without changing hard points. |
| R05 | Native passives / training / unique modifiers | Typed MP, preparation, cooldown and geometry match accepted cast payload and actual MP debit after all approved modifiers. |
| R06 | Temporary instant-cast / other one-shot status | Preview does not consume or mutate the status. Accepted cast consumes once and matches the preview; subsequent refreshed data loses the expired modifier. |
| R07 | Target semantics | Self, same-party living/dead, hostile monster, ground/direction and visible trap entity inputs match actual server resolution. No arbitrary friendly-player permission or fabricated default cast distance. |
| R08 | Range and geometry | Typed shape/center/radius/angle/rectangle dimensions use actual runtime definitions, including true trap/reveal/detect effects. Description text cannot turn a single-target skill into an area skill. |
| R09 | Original presentation | Native card composition/layout, current/next blocks, active costs, native-passive section, incoming training section and scrolling retain existing golden checks. |
| R10 | Progression retained | 20 manual points, equipment-effective ranks above20, hard-only nonrecursive training, original1–5 parity and accepted-cast/source-loss tests remain green. |

## Validation artifacts and limits

- `server/tests/skill_behavior_acceptance.py` checks the evidence envelope and produces a 149-ID semantic acceptance matrix without committing resource tables or descriptions. It can validate exported typed rank snapshots and the audit owner's 71-row decisions.
- `server/internal/game/scene/skill_behavior_acceptance_test.go` owns independent server handshake, membership, current/next, actual MP, temporary-preview and lifecycle checks. It may export safe semantic contract fixtures for the client tests.
- Existing managed client DTO/state, native tooltip/layout and source integration suites remain required, plus the dedicated behavior-capability suite. Managed tests do not claim Unity/Player or visual acceptance.
- Final report must distinguish: implementation/runtime parity, historical evidence coverage, and unresolved historical mechanics. Passing a contract test alone does not certify all historical behavior as authentic.
- No Mac, DB mutation, Unity/Player build, commit, push or deployment is part of this acceptance run.

## Executed acceptance, 2026-10-12

- Evidence/decision matrix: 1,885 checks passed; 149 historical identities,
  146 current resource identities, 68 changed rows plus three missing healer
  rows, eight renamed identities, all 71 decision rows and 472 changed fields.
  Ten identities still carry historical evidence holds. These are dispositions,
  not a claim that every historical mechanic has been restored.
- New capability is gated on both `endgame.v1` and `endgame.growth.v5`. Scene and
  actual session handshake tests cover negotiation, repeated hello, downgrade,
  ordering, old-key omission, transfer and reconnect reset. Skill 10212 has an
  explicit null new contract and retains the approved legacy client path,
  including its native identity and cast request. No new prohibition or hold
  tooltip is imposed on it; its server damage placeholder remains unresolved.
- The separate 55-row correction layer is used by the new runtime and behavior
  tests. All eleven corrected skills at original ranks 1–5 reach actual entity
  attributes/control through `applySkill`, including attack costs on defense
  buffs, hit/defense/magic-attack penalties, silence, stun, movement-only roots,
  and rejected beneficial status applications under chaos.
- Movement tests run player `MoveTo`, monster locomotion/wandering, real basic
  attacks and real skill execution. Movement-only roots do not prohibit attacks
  or spells, roots and silence compose independently, cleanse/removal/expiry
  release movement, and wandering emits no false movement path. Source rank
  gains do not strengthen an accepted root; rank loss reduces it and loss of the
  source skill removes it. High control power does not bypass categorical immunity.
- Shackle now has independently checked actual attack admission for players and
  monsters. Original ranks 1–5 yield player speeds 70/65/60/55/50 percent. With a
  1000ms monster baseline the corresponding intervals are
  1429/1539/1667/1819/2000ms, rounded upward to 100ms scene ticks. Player admission
  retains its existing downward tick quantization. Ranks 20 and 100 preserve the
  existing minimum 50 percent frequency; repeated refresh does not compound the
  penalty. Unmarked equipment/status interval affixes retain their old semantics.
- Actual cast MP debit, accepted preparation/cooldown/distance, native passives,
  training and unique modifiers agree with typed current data. One-shot cast-time
  preview neither consumes the status nor publishes stale preparation prose;
  accepted casting consumes once. Real equipped rank bonuses retain hard cap 20,
  next-effective-rank previews and source-loss recalculation.
- The standalone managed Presentation acceptance compiles the real authoritative
  provider, skill resource loader, targeting adapter and NetClient skill-command
  preflight. It passes 1,361 assertions with production Application DTO/state and
  BoundedJson. Checks include all 149 identities, the three resource-absent healer
  skills, MP boundary admission, party/dead-target rules, declared self fallback,
  current-null/stale data, high equipment ranks, and cross-profession observer
  metadata fallback. This is not a Unity/Player or visual execution test.

### Complete packet size and parser checks

The production scene obtains grants only from the one `SlotTreasure` item.
Unique and astral equipment provide profession-scoped rank bonuses, not multiple
cross-profession grants. The test enumerates all authored legal bonus-equipment
combinations for five level-100 characters with validated 99-point allocations,
then grants each of all 149 profession skills (plus no grant) through the real
single-slot resolver. The latter is a conservative grant-identity superset, not
an assertion that 149 corresponding treasure items exist in the authored data.

| Profession | Equipment/grant scenarios | Maximum rows | Maximum complete JSON bytes |
|---|---:|---:|---:|
| Warrior | 750 | 31 | 55,798 |
| Swordsman | 900 | 31 | 55,159 |
| Assassin | 1,200 | 31 | 55,460 |
| Healer | 2,400 | 30 | 55,899 |
| Warlock | 1,200 | 31 | 56,902 |

All 6,450 scenarios serialize the complete response, including a 64-character
request ID and maximum signed revision. Every `protocol.EncodeEndgame` result
is nonempty and contains the full JSON unchanged. All are below the 61,440-byte
client limit; the largest leaves 4,538 bytes. Each profession's largest actual
response also passes production C# BoundedJson, DTO parsing and atomic state
publication, in addition to the five regular complete profession responses.
The combined 149-row export is an audit artifact, never a single wire response.
This scan exhausts the enumerated equipment/grant combinations for its legal
allocations; it does not enumerate every possible distribution of 99 hard points.

### Reproduction

```sh
source /workspace/shared/cloud-test-env.sh
cd server
SKILL_BEHAVIOR_FIXTURE_PATH=/tmp/skill-behavior-restored149.json \
  go test ./internal/game/scene -run TestSkillBehaviorAcceptance -count=1 -v
go test -race ./internal/game/scene ./internal/session \
  -run 'Test(Verified|NativeMovement|SkillBehaviorAcceptance(MP|High|Hello|Transfer|Unlearned|All149|Session)|SkillBehaviorDragonkill)' \
  -count=1
# In the client repository:
bash Tests/Endgame/run-skill-behavior-presentation-acceptance.sh \
  /tmp/skill-behavior-restored149.profession-{1,2,3,4,5}.json \
  /tmp/skill-behavior-restored149.worst-profession-{1,2,3,4,5}.json
```

The exhaustive grant test takes approximately 2.5 minutes in this cloud setup.
Targeted final scene/session race tests pass. The integration owner separately
runs the aggregate Go suite, vet, build and existing client contracts; the
acceptance results here do not substitute for that final aggregate.
