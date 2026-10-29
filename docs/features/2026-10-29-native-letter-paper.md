# Native letter-paper restoration (unpublished)

Base: `6f01fdcf0051ff6b204acc60f73ea96de2a25df4`.

Native source evidence:
- `000000_init-3.sql`: item 3309, 信纸, original price 1000.
- `000000_init-30.sql`: `ov_item` item 3309, script `paper`; book-merchant greetings
  explicitly sell skill books, paper and parcels.
- `000000_init-29.sql` help “发送邮件” says to double-click the specific bag item;
  letters use paper, and “群发邮件” states paper count equals recipient count.
- The existing server sends to one recipient per request. Thus a letter reserves one
  paper; no group-send feature, parcel material, fee, price or capacity is invented.

The client removes the added mailbox compose shortcut and routes paper to the existing
composer. Opening/canceling spends nothing. At `ReserveMailSend`, contents with neither
money nor attachments require one unlocked paper. Classification is based on actual
contents, not an untrusted client type byte. The paper is included in the existing before/
after inventory snapshot and durable mail transaction. Failed sends refund it; uncertain
outcomes retain the existing MailBusy fence; repeated sends cannot double-reserve while
busy. Parcel behavior and all existing GM/family APIs are unchanged.

Migration **127** appends only paper to source bookshop 1002. Existing aliases 1302, 1402,
1502, 1902 share that stock. It validates item identity/positive price, the five exact
native map/NPC identities, stock source, slot continuity and capacity; preserves all old
stock and prices; and is idempotent. There is no live database operation in this work.
The source's old use-audit view still describes direct `UseItem` paper as unsupported:
this implementation intentionally uses the client composer route and consumes at send,
not the general server `UseItem` command. It does not claim all parcel scripts are restored.

```sh
source /workspace/shared/server-toolchains/env.sh
(cd server && go test ./internal/game/scene ./internal/session ./internal/data ./internal/domain)
PGLITE_MODULE=/path/to/@electric-sql/pglite \
  node server/tests/native_letter_paper_migration.mjs
```

The migration runner executes 23 assertions in independent in-memory PostgreSQL-WASM
instances, including replay, existing paper, custom price preservation, five merchant
aliases, and atomic rejection of bad item/price/source/NPC/map/stock/capacity fixtures.
It is not an external PostgreSQL server or live database test.

**Integration dependency:** init requires contiguous numbering. Public main ends at 125;
126 is reserved for an already-existing icon-mapping correction. Merge its exact original
source before 127/128 and run `go test ./cmd/migrate`; never insert an empty substitute or
renumber/overwrite the historical migration. Standalone upgrade 127 is independently
transactional. Do not run the full initializer against an existing database.

Family creation audit: the existing request contains only name/declaration, not an NPC.
The server enforces in-game state, no existing family, transactional storage, level40,
honor300, ten gold and FamilyBusy; it has no general last-NPC/position guard for creation.
This task changes the client entry to eight exact city leaders, not that protocol/security
contract. No new server NPC restriction is asserted.
