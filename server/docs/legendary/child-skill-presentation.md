# Native child-skill presentation

Child skills keep their existing immediate gameplay settlement. Their presentation is a separate art-only release, not a second actor cast and not a gameplay delay.

Clients opt in before world entry with `OpClientCaps` bit 128. A connection without this capability never receives opcode `0x80F2`; existing cast packets remain unchanged. The capability is frozen after entry and resets with a new connection.

The payload contains eight little-endian i32 fields: caster entity ID, native skill ID, primary target ID, map ID, launch X, launch Y, resolved center X, resolved center Y, followed by one U8 mode (0 release, 1 impact-only). Coordinates are authoritative map pixels. The child release must not trigger actor cast animation, cooldown, mana changes, or implicit native hit art. The server separately emits the existing `SkillHitEffect` once per actual landed target when the server has an explicit effect name. Otherwise mode 1 asks the existing client SkillEffectDB for that native skill’s hit phases; it never generates another release. Actual shipped skill-table contents still require resource/runtime verification. An immune target may receive release art but no hit art. Invalid/dead/missing targets and recursion-rejected children emit neither.

Synthetic payloads that intentionally reuse a skill ID only for damage composition set `SuppressVisual` and emit their producer-specific approved art instead. This prevents teleport/support/theft art being repeated once per damage target. Ordinary child skills keep the native release path.

These event/protocol tests do not constitute Unity render acceptance.
