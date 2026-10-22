# Quest special weapons (100)

User-approved additions, not a claim that the missing original server choice scripts were recovered.

- 3-level weapons: quest 85 and 87, 小试身手 in the two starting villages.
- 8-level weapons: quest 36 为民除害 and 37 带牙的花.
- 14-level weapons: quest 42 巡城三, after existing prerequisites 40 → 41 → 42. No weapon is added to 40 or 41.
- Race is the character's saved creation route (0 warrior, 1 swordsman, 2 assassin, 3 healer, 4 warlock), even before employment.
- Warrior receives one spear; swordsman receives both single sword and double sword; assassin receives both blades and throwing weapon; healer/warlock each receive one staff. The completion protocol has no reward-choice field. Nothing is randomized across professions.
- All grants are unrefined original item IDs. Existing 86/90 single-weapon +3 rewards are preserved.
- All five quests are nonrepeatable. The two village alternatives are not mutually exclusive in existing data: a character can finish each once, receiving two same-level batches if they do both. No new cross-village lock or retrospective player grant is introduced.
- The entire reward batch is planned against a cloned bag before any bag, quest completion, XP or currency commit. Insufficient space causes zero weapons to be granted; retry and replay cannot duplicate completion rewards.

Deploy the new binary and migration together. Migration 100 widens the profession reward primary key to `(task_id,race,item_id)`; the old binary rejects multiple rows per route. Do not replay old initialization fragment 41 after upgrade. Fresh initialization applies 41 before 100. The standalone upgrade owns its transaction and can be replayed, preserving unrelated profession rows.

Tests: `go test ./internal/data ./internal/domain ./internal/game/scene -run Quest -count=1`. The data tests check fresh/upgrade body parity, all 35 exact route/item mappings, invalid/missing/duplicate rows and legacy +3 rewards. Scene tests cover all five unemployeed creation routes, one/two weapon awards, capacity rollback, final-step-only awards, repeat completion, template isolation and patrol prerequisite gating.

Boundary: baseline 3-level weapon templates have `newbie=0`, unlike 8-level templates. Distribution itself does not change wear eligibility. Equipment initialization must address this separately if the new 3-level reward must be usable before employment. No production database migration was executed by this task.
