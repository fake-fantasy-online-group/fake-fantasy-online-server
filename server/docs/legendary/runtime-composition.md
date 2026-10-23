# Legendary synergy runtime

This implements the approved synergy revision, not the superseded timed-trigger catalogue. All 330 identities remain stable. Current text, typed registration and tested eligibility are separate facts.

## Composition order

1. Capture the accepted native cast, equipment source identities, paid mana and target geometry.
2. Run class transformations before universal final-shape transformations.
3. Complete base-formula additions across all equipped powers, then gather additive modifiers and finalize once.
4. Resolve legal native targets, impact-time selectors and per-target definitions.
5. Settle native damage/status/healing, including precise post-status and effective/overflow callbacks.

Native base additions enter before target fixed defense and critical calculation. Fixed-defense and elemental penetration modify only a strike-local value; entity stats are never temporarily changed. Same-category skill/stat/incoming modifiers use a shared baseline. Resource-consuming incoming absorption runs after pure reductions.

Declared child skills retain native skill identity, element and geometry modifiers without consuming another player action, mana payment or cooldown. A producer lineage prevents the same generator from recursively producing itself while allowing distinct equipment producers to compose, with a bounded depth. Pure receiving defenses and shield efficiency still apply to children. Damage-only echoes can explicitly prohibit status application and further generation.

Accepted source identity bounds delayed/persistent skills. Newly equipped powers do not upgrade already accepted casts; removing a source prevents its continuing equipment contribution. Persistent native status reconciliation preserves remaining charges rather than refilling them.

## Stateful exceptions and presentation

Ordinary synergy effects use native skill/status lifetimes, not replacement timers. Explicitly retained fields such as warrior aftershock use simulation time and source-owned state. A14 alone snapshots one accepted native attack/critical roll and source output for its initial quake and field; legal targets supply their own defenses. Target-conditioned unique bonuses are evaluated at the initial impact, not baked from whichever target happened to be visited first. Empty casts retain the same source-defined field semantics.

Source-bound permanent generated shields disappear when their originating item is no longer functional. Foreign permanent equipment shields additionally require source and recipient to remain in the same scene: source departure or recipient transfer removes them, while self-owned states retain their ordinary lossless transfer behavior. Native timed healing and defense buffs are not affected by this departure rule. Timed enemy debuffs are not silently removed by unrelated equipment reconciliation. Transfer receipts distinguish active permanent states, expired states and consumed placeholders.

Status text is taken from the current catalogue and authentic icon metadata. Fixed-ground effects use the separately negotiated GroundEffect capability and actual world coordinates; they are not actor-follow effects. Existing clients receive no unknown packet. Approved art stage selection is explicit in a handler; a catalogue entry or asset recipe alone does not mean an effect is emitted.

## Verification and scope

Run `go run ./cmd/legendarycoverage` to inspect registration, current revision and per-ID tested eligibility independently. The output is an inventory, not a substitute for behavioral tests. Class coverage ledgers list native-event and multi-item tests. Regression tests separately cover child shield efficiency, receiver control reduction, child flat scaling, source transfer, fixed payloads and native base defense.

Native skill target admission remains authoritative. Player-versus-player skill attacks are not enabled by this change; a PvE test is not PvP certification. Six affixes, original level pools, dyes, normal/hard dungeon boundaries and Rift ceilings remain independent of power transformations.

No production deployment, push, merge or version change is part of this implementation.
