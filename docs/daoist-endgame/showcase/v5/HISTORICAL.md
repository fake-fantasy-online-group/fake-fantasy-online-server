# Frozen V5 evidence

This directory preserves the previous 27-equipment / 44-rune plan, original generator, Python test, Go catalog-test source, factory source snapshots, and previous runtime report. These are historical artifacts, not the current grant plan.

The original tracked files were captured from the current checkout's HEAD before V6 showcase edits. The grant factory was untracked development work at capture time, so its exact pre-V6 bytes were copied directly. `factory.go.txt`, `factory_test.go.txt`, and `showcase_catalog_test.go.txt` are non-built source evidence.

The generator and Python test retain their original relative paths and V5 source expectations; do not run them in this nested archive and call a failure a V6 regression. Reproduction of the unmodified original tool requires restoring the files at their original paths in a separate V5 source checkout, using frozen V5 catalogs and matching original native-equipment fixtures. It is intentionally not an instruction to overwrite the active V6 workspace.

In the current V6 source tree, the executable historical compatibility check is:

    cd server
    go test ./internal/domain -run '^TestHistoricalV5ShowcaseThreeCatalogLoadouts$' -count=1

That test reads this manifest and explicitly resolves content version 5 against frozen catalogs. It validates historical construction/compatibility and does not grant anything. New generation remains V6. The original report was not rewritten to imply a new execution.
