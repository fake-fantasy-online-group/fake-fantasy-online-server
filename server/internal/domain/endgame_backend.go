package domain

// EndgameBackend keeps IO out of scene actors. Callers hold inventory/lifecycle
// ownership until the buffered result arrives, including after disconnect.
type EndgameBackend interface {
 Load(charID int64) <-chan EndgameLoadResult
 LoadReceipt(charID int64, requestID string) <-chan EndgameReceiptLoadResult
 LoadRun(runID string) <-chan EndgameRunLoadResult
 Commit(EndgameCommitPlan) <-chan EndgameCommitResult
}
type EndgameRunBackend interface { SaveRun(EndgameRunRecord, int64) <-chan EndgameRunSaveResult }
type EndgameState struct { Revision int64; Payload string }
type EndgameReceipt struct { CharID int64; RequestID, Operation, Digest string; Revision int64; Response string }
type EndgameRunRecord struct { RunID string; Revision int64; State, Payload string }
type EndgameLoadResult struct { State EndgameState; Err error }
type EndgameReceiptLoadResult struct { Receipt *EndgameReceipt; Err error }
type EndgameRunLoadResult struct { Run *EndgameRunRecord; Err error }
type EndgameRunSaveResult struct { Run *EndgameRunRecord; Err error }
type EndgameCommitPlan struct {
 Before Snapshot
 Snapshot Snapshot
 ExpectedRevision int64
 State EndgameState
 Receipt EndgameReceipt
 Run *EndgameRunRecord
 ExpectedRunRevision int64
}
type EndgameCommitResult struct {
 State EndgameState
 Receipt EndgameReceipt
 Replayed bool
 // A newer durable revision must replace the actor's durable aggregate before
 // unlocking. Never autosave the stale Before/candidate over this snapshot.
 RequiresReload bool
 Authoritative *Snapshot
 Err error
}
