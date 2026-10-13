# Project working agreements

## Development base

- Before implementation, fetch origin/main and verify that the feature branch contains the latest origin/main. Recheck the base before final validation.

## Behavior recovery

- Prioritize usable recovered behavior, demonstrated user-blocker fixes, and content needed by actual consumers. Review is a gate for implementation, not the default deliverable.
- An unmatched static record or stale data alone does not establish that a feature is missing. Identify a real consumer and the missing behavior before selecting an implementation.
- Once evidence establishes a bounded change, implement it or close it. Do not repeatedly review an accurate implementation without new counterevidence or expand into neighboring features just to produce work.
- Repeated investigations with no new findings require changing the evidence source or approach, not repeating adjacent reviews.
- Preserve partial status and state genuine external blockers. Do not widen scope to claim success, and honor user pauses and explicit scope limits.
