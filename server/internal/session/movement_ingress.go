package session

import "time"

// movementIngress is owned by the connection reader. Do not merge MoveTo events:
// facing, stop, portal and correction ordering are gameplay semantics. Bound the
// producer instead, before it can consume the shared scene mailbox.
type movementIngress struct {
	at     time.Time
	tokens float64
}

func (b *movementIngress) allow(now time.Time, rate float64, burst int) bool {
	if rate <= 0 {
		rate = 40
	}
	if burst <= 0 {
		burst = 80
	}
	if b.at.IsZero() {
		b.tokens = float64(burst)
	} else if now.After(b.at) {
		b.tokens += now.Sub(b.at).Seconds() * rate
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
