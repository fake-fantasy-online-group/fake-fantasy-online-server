package net

import (
	"errors"
	"log/slog"
	"net"
	"time"
)

var errIngressLimit = errors.New("net: ingress budget exceeded")

// tokenBucket is owned by one connection reader. Bursts absorb normal loading and
// mobile jitter; sustained excess closes only the offending connection.
type tokenBucket struct {
	at     time.Time
	tokens float64
}

func (b *tokenBucket) allow(now time.Time, cost, rate, burst float64) bool {
	if b.at.IsZero() {
		b.tokens = burst
	} else if now.After(b.at) {
		b.tokens += now.Sub(b.at).Seconds() * rate
		if b.tokens > burst {
			b.tokens = burst
		}
	}
	b.at = now
	if cost > b.tokens {
		return false
	}
	b.tokens -= cost
	return true
}

func normalizeOptions(o Options) Options {
	if o.HandshakeLen <= 0 {
		o.HandshakeLen = 260
	}
	if o.OutBuffer <= 0 {
		o.OutBuffer = 1024
	}
	if o.WriteWait <= 0 {
		o.WriteWait = 10 * time.Second
	}
	if o.HandshakeWait <= 0 {
		o.HandshakeWait = 10 * time.Second
	}
	if o.AuthWait <= 0 {
		o.AuthWait = 2 * time.Minute
	}
	if o.HandshakeConcurrency <= 0 {
		o.HandshakeConcurrency = 16
	}
	if o.ReadWait <= 0 {
		o.ReadWait = 2 * time.Minute
	}
	if o.MaxConnections <= 0 {
		o.MaxConnections = 4096
	}
	if o.MaxConnectionsPerIP <= 0 {
		o.MaxConnectionsPerIP = 128
	}
	if o.PacketsPerSecond <= 0 {
		o.PacketsPerSecond = 100
	}
	if o.PacketBurst <= 0 {
		o.PacketBurst = 200
	}
	if o.BytesPerSecond <= 0 {
		o.BytesPerSecond = 2 << 20
	}
	if o.ByteBurst <= 0 {
		o.ByteBurst = 4 << 20
	}
	if o.ShutdownWait <= 0 {
		o.ShutdownWait = 10 * time.Second
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return o
}

func peerIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}
