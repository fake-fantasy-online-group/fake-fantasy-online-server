# TY universal independent behavior case coverage

- Manifest: `universal-behavior-coverage.json`, 80 / 80 power IDs, 311 exact solo test references, 310 prose clause fragments. Every listed reference passed in the completed focused package, with no skips/failures.
- 37 combination references are separate and never contribute to the 80-power numerator. A settled-payload native shield safeguard has no powers and is separately listed under `safety_tests`.
- This is independent power behavior **case** coverage, not Go statement/branch coverage, exhaustive combinations, a deployed build, PostgreSQL-loader certification, or client visual verification.
- Main effects use actual HP/MP, selected targets, committed states/lifetimes, payment/cooldown admission, shield consumption, or authoritative entity/protocol output. Native source-backed skill/status fixtures coexist with explicitly labeled controlled consumer boundaries. Eligible level-100 equipment is checked by the shared fixture; solo cases have only their listed legendary. Baseline comparisons may have zero powers.

## Corrections demonstrated

1. Power-produced periodic damage retains pure receiver D19 reduction / D11 mana absorption; source generation guards do not erase these defensive consumers.
2. Explicitly critical periodic damage consumes D10 reduction, in the same additive reduction bucket before mana and shield. This is a receiver contract boundary, not a claim of enabled PvP or an observed current PvE producer.
3. Cone geometry above 180 degrees is no longer incorrectly cut to the front hemisphere. The angle test includes a 1e-9 coordinate-unit roundoff tolerance, retaining exact 180-degree side boundaries. Rectangle/circle geometry is unchanged. Controlled 60/120/168/180/240/360-degree final cones are tested in four directions at exact angular/radial boundaries; this does not assert all bases exist in authored PvE.
4. Reviewer-found false-green risks are closed: U11 shield/HoT exclusions use fixed 10-second oracle and before/at-expiry checks; A11 uses one selected seed with baseline100 versus enhanced200, not a roll already below baseline80%.

## Source/settlement limits

D05/D06/D07/D08/D10/D11/D17/D19 distinguish direct physical/magic, area, basic, critical, proc/child, periodic, reflection and self-damage consumers. D11 tests reduction, limited mana, then one shield; reflection emits once. The settled safeguard uses a reachable monster recipient, proving native reduction is not repeated and shield is consumed once. Current PvP admission prevents a settled legendary payload targeting an equipped player; no such fixture is fabricated or certified.

TY-A35 certifies `AppearanceChanged` reach output 100→150→100, normal damage/cadence, and equipment removal. `onAttack` has no separate range-admission rejection. A01+A03+A35 has separate real target geometry evidence; client range enforcement is not claimed.

## Reproduction and evidence

Use Go `/workspace/shared/toolchains/go/bin/go`, `GOPATH=/workspace/shared/go`, `GOCACHE=/workspace/scratch/1cc826cfb9aa/astra-audit/go-cache`, `GOTMPDIR=/workspace/scratch/1cc826cfb9aa/go-tmp`.

Focused command script: `/workspace/scratch/1cc826cfb9aa/run-universal-focused.sh` (fresh `go test ./internal/game/scene -count=1 -json` with all exact manifest references and existing incoming/mana/settled regressions).

Completed JSONL: `/workspace/scratch/1cc826cfb9aa/universal-coverage-focused.jsonl`.

Manifest gate result: `/workspace/scratch/1cc826cfb9aa/universal-coverage-gate.json`: TY qualified80; zero TY reference/overlap/pass errors. Group-only gate exits1 solely for the other250 IDs, which are integrated separately. The gate is traceability/pass validation; independent semantic review remains required.

Final base recheck: fetched `origin/main` at `489706ac1649247ace9d8cfc74e8bf8b9cd630f0`, confirmed ancestor. No main merge, force push, deployment, client/assets/hash changes.

## 集成后的最终证据

本文件上方为作者阶段focused记录。最终全server生产/测试树、仅新增A11精确概率边界的后续test-only提交、单独补充执行、最终race与全330门禁以verification-summary.json及independent-coverage-result.json为准。最终认证不混用此阶段日志；新增概率两叶使TY完整solo引用为313。
