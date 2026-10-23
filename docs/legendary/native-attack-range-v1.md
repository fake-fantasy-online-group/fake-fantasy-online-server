# Native weapon classification versus effective attack reach

Pre-world-entry `OpClientCaps` (`0x1091`) bit64 enables one final little-endian
U16 native attack-distance tail on these packets, after all existing tails:
- SelfData (`0x8003`): after petViewMask I32.
- Appearance (`0x800a`): after glowUnlocked U8.
- Remote player spawn (`0x8023`): after invisible U8.

The existing attack distance continues carrying effective reach. Native distance
is sampled by the scene from positive character Appearance.AtkDist, falling back
to Look.AtkDist only for legacy/uninitialized native data, matching server weapon
classification. The client uses native distance for animation/cadence/projectiles
and effective distance for targeting reach. Absent tail keeps legacy behavior.

The capability is connection-local, accepted before world entry, and frozen while
in-game. Late capability packets do not retrofit or remove it. Existing refresh,
map-rebind and remote-spawn events retain the negotiated format; fresh connections
must advertise again. Unnegotiated packet bytes remain unchanged. No protocol
version bump or secondary capability request is used.

Server tests cover final-tail value boundaries, unchanged prefix/unrelated frames,
all three opted-in and legacy packet paths, ignored late changes, map rebind,
fresh connection reset, native/effective separation and equipment refresh. Client
validation includes rejecting a truncated single-byte optional tail. Unity
rendering/animation is not claimed as tested by these managed/protocol checks.
