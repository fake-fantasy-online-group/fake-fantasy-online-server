package scene

import "sync/atomic"

// RackRequest owns the prepare-result handoff, not the database transaction.
// Cancellation and publication have exactly one winner. The session must use a
// fresh buffered reply channel so the actor never waits for a network goroutine.
type RackRequest struct{ state atomic.Int32 }

// Cancel returns true only when the caller still owns cancellation. If false,
// publication won and the caller must consume and settle the prepared result.
func (r *RackRequest) Cancel() bool    { return r.state.CompareAndSwap(0, 2) }
func (r *RackRequest) cancelled() bool { return r != nil && r.state.Load() == 2 }
func (r *RackRequest) claim() bool     { return r == nil || r.state.CompareAndSwap(0, 1) }
