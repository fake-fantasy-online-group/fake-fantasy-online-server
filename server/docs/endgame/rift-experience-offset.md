# Rift boss experience offset

Boss completion experience is now `floor(personalTrialBase × (1 + tier / 10))`:
1/10/30/150 give 110%/200%/400%/1600%. Previously these were
10%/100%/300%/1500%. Arithmetic uses quotient/remainder and saturates at int64
maximum, without prematurely truncating the fractional multiplier.

Only the multiplier changes. Existing member eligibility, completion-time
personal level (or historical admission-level rules), offline level evidence,
level cap, pet-share rules, and durable exactly-once settlement remain intact.
Already-generated persisted grants keep their stored XP, even after restart or
claiming at a different level. Runs without generated grants use the new formula.
No database migration, client change, or native monster XP award is needed.

Map selection is already seeded over the whole viable geometry catalog, without
character-level or challenge-tier filtering; adjacent repeats are avoided when
another viable map exists. Keep those rules unchanged. A fresh static audit of
this checkout finds 189 candidates. This is not a live-server count: the runtime
loader additionally verifies loaded monster definitions and usable assets.
Cities, island aliases, missing masks, disconnected/insufficient anchors, and
unusable safe populations remain excluded. Combat bases still come from the
frozen roster's trial level, then tier/party/class scaling; map-native levels do
not set difficulty. Population count instead follows usable map area and the
published population multiplier.
