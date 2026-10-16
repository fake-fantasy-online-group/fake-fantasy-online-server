# Mail operation durability

Apply `migrations/000000_init-87.sql` before enabling mail send or attachment claim
on this binary. It adds `mail_operation_receipts`; it does not rewrite existing
mail or inventory rows. No migration is automatically applied by the new code.
A bounded readiness query runs before asset reservation and caches success per
store instance. A missing table fails the request before assets are reserved.

Each server-generated operation ID identifies one send or claim. The candidate
snapshot, mail change, and committed receipt are in one PostgreSQL transaction.
Both submission and reconciliation first take the same operation advisory lock.
Therefore a missing receipt is considered rollback evidence only after any
previous transaction has finished. Reconciliation then saves the baseline and
an aborted receipt atomically. Retrying a committed ID is a no-op; retrying an
aborted ID cannot perform the transfer. Receipts must not be purged while their
IDs may still be retried.

A lost COMMIT acknowledgement is not treated as failure. The actor and keyed
write queue retain character ownership until a new bounded database attempt
proves the outcome. Other queue keys can continue. During a sustained outage,
mail is intentionally unavailable for that character and warning logs contain
the operation ID. Shutdown may report unresolved operations; do not manually
restore pre-send assets on that basis. After a process restart, the atomic
persisted snapshot and mail/receipt state are authoritative.

The actor excludes mail candidates from periodic saves and defers inventory and
lifecycle effects while settlement is outstanding. Timed-out preparation closes
a cancellation signal and consumes its late actor reply; it never submits that
request to the database. Only a known committed or durably aborted outcome may
complete a submitted reservation.
