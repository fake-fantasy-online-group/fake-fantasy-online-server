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
