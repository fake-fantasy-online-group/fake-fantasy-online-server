# Skill status growth review, v6

## Scope and evidence

This is a status-axis audit, not a claim that a whole skill is empty when its status stops changing. Direct damage, passive effects, mana efficiency and cooldowns are outside this comparison.

The original fixture contains 149 battle skills and 1,890 status rows. 54 skills have direct status applications or passive status triggers. Each was evaluated at every integer rank 6–200 against the previous rank. Status identifiers, metadata and raw progression counters were excluded; control pressure was compared by its actual integer resistance output for every resistance value 0–90. Application chance, extra affixes, stealth bonus and vulnerability overrides were included. Native ranks 1–5 are covered by exact-row parity tests.

Important: these exported fixtures predate the separate native-restoration overlay. Seven additional runtime skills acquire statuses there, for 61 status-bearing skills and 88 without direct/trigger status payloads. The overlay differences are listed below. Do not present the old fixture as an exact current-loader snapshot.

Sources: `v5/skill-original-ranks.json`, `v5/skill-status-anchors.json`, `server/internal/domain/skill_status_scaling.go`, `server/internal/data/native_skill_restoration.go`.

## Full 54-skill fixture audit

“First empty” is the first rank in 6–200 whose resolved status payload and numerical resistance outcomes are identical to the previous rank. `—` means none in this interval. “Last growth” is the last changing rank within the interval, not a universal cap. “Empty count” counts repeated transitions in that interval. Effects can remain useful at their cap even though buying another rank no longer improves that axis.

| Skill | Name | First empty before | Empty count before | Last growth before | First empty after | Empty count after |
|---|---|---:|---:|---:|---:|---:|
| 10010 | 狂暴术 | — | 0 | 200 | — | 0 |
| 10011 | 怒火杀意 | — | 0 | 200 | — | 0 |
| 10012 | 破甲 | 29 | 172 | 28 | 29 | 172 |
| 10016 | 石化之枪 | 91 | 11 | 200 | 91 | 11 |
| 10018 | 冲锋 | — | 0 | 200 | — | 0 |
| 10019 | 全力投掷 | 7 | 194 | 6 | 7 | 194 |
| 10027 | 重击 | 90 | 12 | 199 | 90 | 12 |
| 10029 | 震慑 | 91 | 11 | 200 | 91 | 11 |
| 10030 | 缴械 | 96 | 11 | 200 | 96 | 11 |
| 10107 | 铁城神盾 | 32 | 169 | 31 | 32 | 169 |
| 10109 | 仙风云体 | — | 0 | 200 | — | 0 |
| 10110 | 通背 | 90 | 12 | 199 | 90 | 12 |
| 10114 | 锁灵剑 | 6 | 195 | 5 | 6 | 195 |
| 10115 | 火字诀 | — | 0 | 200 | — | 0 |
| 10124 | 山字诀 | — | 0 | 200 | — | 0 |
| 10126 | 强击 | 90 | 12 | 199 | 90 | 12 |
| 10128 | 煞气 | 91 | 11 | 200 | 91 | 11 |
| 10129 | 致残 | 96 | 11 | 200 | 96 | 11 |
| 10130 | 雷霆之怒 | — | 0 | 200 | — | 0 |
| 10208 | 隐身术 | — | 0 | 200 | — | 0 |
| 10211 | 淬毒术 | 16 | 185 | 15 | 16 | 185 |
| 10214 | 陷阱：毒雾 | — | 0 | 200 | — | 0 |
| 10215 | 陷阱：封印 | 85 | 12 | 200 | 85 | 12 |
| 10216 | 陷阱：冻气 | 93 | 11 | 200 | 93 | 11 |
| 10224 | 烟雾弹 | 85 | 12 | 200 | 85 | 12 |
| 10303 | 回春术 | — | 0 | 200 | — | 0 |
| 10307 | 祝福术 | — | 0 | 200 | — | 0 |
| 10308 | 神圣庇护 | — | 0 | 200 | — | 0 |
| 10310 | 减速术 | 9 | 192 | 8 | 9 | 192 |
| 10311 | 加速术 | 18 | 183 | 17 | 18 | 183 |
| 10315 | 瞬击术 | 40 | 161 | 39 | 40 | 161 |
| 10320 | 光盾术 | — | 0 | 200 | — | 0 |
| 10321 | 防御术 | 32 | 169 | 31 | 32 | 169 |
| 10322 | 神术：集中 | — | 0 | 200 | — | 0 |
| 10324 | 衰弱术 | 10 | 191 | 9 | 10 | 191 |
| 10325 | 枷锁术 | 6 | 195 | 5 | 6 | 195 |
| 10327 | 神术：鹰眼 | 91 | 11 | 200 | 91 | 11 |
| 10328 | 神术：释能 | — | 0 | 200 | — | 0 |
| 10329 | 神术：瞬发 | 6 | 195 | 5 | 6 | 195 |
| 10330 | 针刺护体 | 11 | 190 | 10 | 11 | 190 |
| 10410 | 冰爆术 | 6 | 195 | 5 | 6 | 195 |
| 10411 | 绝对零度 | 6 | 195 | 5 | 6 | 195 |
| 10413 | 时之封印 | 86 | 12 | 200 | 86 | 12 |
| 10417 | 魔镜术 | 16 | 185 | 15 | 16 | 185 |
| 10418 | 秘术：集中 | — | 0 | 200 | — | 0 |
| 10419 | 秘术：释能 | — | 0 | 200 | — | 0 |
| 10420 | 秘术：瞬发 | 6 | 195 | 5 | 6 | 195 |
| 10421 | 燃烧 | 11 | 114 | 200 | — | 0 |
| 10423 | 高热风暴 | 6 | 117 | 200 | — | 0 |
| 10425 | 火盾术 | 21 | 180 | 20 | 21 | 180 |
| 10427 | 冰系掌握 | 10 | 191 | 9 | 10 | 191 |
| 10428 | 冰封 | 89 | 12 | 200 | 89 | 12 |
| 10430 | 淬冰风暴 | 105 | 8 | 200 | 105 | 8 |
| 10435 | 冰盾术 | 16 | 185 | 15 | 16 | 185 |

## Exact bounded-component thresholds

The ranks below are the first rank reaching the component's numerical bound, not the first empty rank. Other components can continue growing after that rank.

| Skill(s) | Bounded component | First capped rank |
|---|---|---:|
| 10010 | Defense penalty -100% | 5 (authored) |
| 10012 | Physical and magic damage taken +100% | 28 |
| 10019 | Move-speed penalty -100% | 6 |
| 10107, 10321 | Existing damage reduction 100% | 31 |
| 10115 | Defense penalty -100%; attack still grows | 31 |
| 10211 | Poison-on-hit chance 100% | 15 |
| 10224 | Blind/magic-attack penalties -100% | 15 |
| 10310 | Slow -100% | 8 |
| 10311 | Move speed +100% | 17 |
| 10315 | Attack interval percentage -100% | 39 |
| 10324 | Attack penalty -100% | 9 |
| 10325 (runtime overlay) | Attack-frequency penalty 100% | 15 |
| 10329, 10420 | Next-cast reduction 100% | 5 (authored) |
| 10330 | Physical reflection 100% | 10 |
| 10417 | Full magic reflection chance 100% | 15 |
| 10425 | Physical reduction 100%; reflection 100% | 15; 20 |
| 10427 | Move-speed penalty -100% | 9 |
| 10435 | Freeze-attacker chance and physical reduction 100% | 15 |

Zero fifth-rank affixes remain absent; for example stealth's zero movement penalty must not be extrapolated into a positive speed buff.

### Control duration and resistance

| Skill(s) | Duration bound | First capped rank | First rank with all numerical resistance outcomes zero |
|---|---:|---:|---:|
| 10016, 10029, 10128 | 15 s | 11 | 1792 |
| 10027, 10110, 10126 | 15 s | 10 | 1791 |
| 10030, 10129 | 20 s | 16 | 1797 |
| 10215 | 34 s (long authored silence retained) | 5 | 1786 |
| 10216 | 60 s, breaks on damage | 13 | 1794 |
| 10224 | 60 s | 5 | 1786 |
| 10327 | 60 s | 11 | 1792 |
| 10413 | 15 s | 6 | 1787 |
| 10428 | 60 s, breaks on damage | 9 | 1790 |
| 10430 | 60 s, breaks on damage | 25 | 1806 |

The consumed resistance rule is floor(resistChance × 100 / (100 + 5 × pressure)), with legacy resistChance bounded to 90. At pressure 1780 its maximum is 1; at 1781 every allowed integer resistance gives zero. Thus an ever-increasing pressure integer is not an ever-improving gameplay effect. The first integer plateaus occur well before this terminal point, as the 6–200 audit shows. Actual immunity/status-blocking gates remain independent and unchanged.

Constant durations below their mechanism bounds do not acquire pressure: 10019 (5 s), 10310 (20 s), 10324/10325 (30 s), 10410 (15 s), 10411 (5 s), and runtime-restored 10221 (5 s). Preserving the native constant duration does not claim that its rank progression is complete.

## Native-restoration overlay differences

Seven skills omitted by the old fixture's status lists have these runtime status axes:

| Skill | Restored axis | Exact threshold / progression |
|---|---|---|
| 10009 破防之枪 | Defense penalty, fixed 15 s | -100% at rank 8; status-only empty from 9 |
| 10103 幻影剑 | Hit penalty, fixed 15 s | -100% at rank 15; status-only empty from 16 |
| 10111 金缚剑 | Movement-only root | 15 s at rank 10; pressure thereafter, fully zero resistance at rank 1791 |
| 10113 封法剑 | Silence | 20 s at rank 15; pressure thereafter, fully zero resistance at rank 1796 |
| 10122 混沌剑 | Refuse beneficial states | 60 s at rank 51; pressure thereafter, fully zero resistance at rank 1832 |
| 10219 击晕 | Native nonordinal stun rows | 15 s at rank 11; pressure thereafter, fully zero resistance at rank 1792 |
| 10221 狙击 | Fixed 5-second movement-only root | No duration/pressure growth; status-only empty from 6 |

Additional overlay changes: 10325 acquires its actual attack-frequency penalty and therefore first becomes status-only empty at rank 16, replacing the obsolete fixture result of 6. 10109 gains its native self-root but retains unbounded positive defense growth. 10124 gains its native attack penalty (bounded at -100%, rank 31) but retains positive defense growth. 10224 no longer acquires the unrelated magic-defense penalty; its hit/magic-attack penalty thresholds and pressure outcomes are unchanged.

These overlay thresholds are source/formula cross-checks, separately identified from the executable old-fixture sweep above. Current-loader integration tests remain required before claiming production equivalence.

## Implemented status fix

Only existing nonzero constant damage/healing/shield/reflect/next-hit magnitudes change: extra magnitude is max(extraRanks, floor(abs(rank5Magnitude) × extraRanks / 50)). This retains the 2% continuation where it advances at least one native unit and removes quantized empty ranks below that slope. Signs and existing effect channels are preserved. Zero effects, chances, duration, targets, area, intervals, break conditions and boolean mechanics are unchanged.

For burning status 1036, the native 20 damage per tick now increases by one per extra rank. Both 10421 and 10423 have zero empty status transitions in ranks 6–200; before the fix they had 114 and 117 respectively. No second damage segment or new periodic effect was added. Integer overflow is rejected rather than silently capped.

## Unbounded existing axes and functional fallback

Within representable engine fields, these native magnitude axes continue: 10010 attack; 10011/10130 next-basic damage; 10109/10124 defense; 10115 attack; 10208 stealth damage; 10214 poison damage; 10303 regeneration; 10307 attack/magic attack; 10308/10320 shield pool; 10322/10418 next-duration multiplier; 10328/10419 next-skill damage; 10421/10423 burning damage. Duration multipliers may still encounter the receiving control's duration cap; the raw multiplier is not proof of an effective improvement in every target context. Sprint 10018 continues duration up to its 3600-second bound at rank 1799 but is not an unbounded axis.

Functional active skills use mana efficiency rather than unrelated damage or shields: native ranks 1–5 remain exact, while rank >5 retains the fifth-rank base mana and adds rank−5 efficiency. The rational average at efficiency E is baseMana × 50 / (50 + E). The domain regression checks strict average improvement at ranks 20, 21, 30, 50 and 100 without pretending each individual integer charge must differ. It also verifies unchanged area, status count and effect count. Authoritative fractional settlement is a separate scene integration responsibility.

Passive bounded-status continuations must improve only their existing target skills. Specifically, 10012 targets 10009; 10019 targets 10017; 10427 targets its existing 10409/10410/10412/10411 list. Their frozen status values cannot be treated as successful uncapped progression merely because a progression counter changes.

## Regression evidence

- All original status rows return unchanged when no extended rank is requested.
- Constant positive/negative native magnitudes strictly advance every rank across 1–200 added ranks; zero stays zero and unrepresentable arithmetic is rejected.
- Actual burning fixture advances every rank 6–200 without changing timing or control semantics.
- Existing status continuation, duration-boundary, resistance, source-nonmutation and large-rank bounded-probability tests pass.
- Functional active domain averages strictly improve at 20/21/30/50/100 with no new status, direct effect or area.
- Full domain suite passed after the status magnitude change; focused status and mana-efficiency tests passed after the functional progression changes.
