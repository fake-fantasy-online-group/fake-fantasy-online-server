// Package showcase implements the bounded, offline test001 showcase grant.
package showcase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const ReceiptKey = "showcase-test001-three-v5-20261012"
const operation = "showcase.grant"

var targetIDs = []int64{42, 183, 206}

type ItemAudit struct {
	Slot            int
	Stack           domain.Stack
	DisplayName     string         `json:",omitempty"`
	RequiredLevel   int32          `json:",omitempty"`
	ResolvedAffixes []domain.Affix `json:",omitempty"`
}
type CharacterAudit struct {
	ID                      int64
	Name                    string
	Gender                  uint8
	BeforeCharacterSHA256   string
	AfterCharacterSHA256    string
	BeforeBag               []ItemAudit
	AfterBag                []ItemAudit
	Granted                 []ItemAudit
	UnrelatedStateUnchanged bool
}
type Report struct {
	Account    string
	PlanSHA256 string
	Status     string
	Characters []CharacterAudit
}

func digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func bagAudit(b *domain.Bag) (out []ItemAudit) {
	b.Each(func(slot int, s domain.Stack) { out = append(out, ItemAudit{Slot: slot, Stack: s}) })
	return
}

// RunTransaction requires the caller's exclusive offline maintenance transaction.
// It never commits. Dry-run calls no write API. All three targets preflight before
// any save; the caller must rollback ANY error, including verification failures.
func RunTransaction(ctx context.Context, tx store.Store, defs map[domain.ItemID]domain.ItemDef, apply bool, warehouseRule domain.WarehouseRule) (Report, error) {
	r := Report{Account: "test001", Status: "dry-run"}
	d, err := PlanDigest(defs)
	if err != nil {
		return r, err
	}
	r.PlanSHA256 = d
	eg, ok := tx.(store.EndgameStore)
	if !ok {
		return r, fmt.Errorf("transaction lacks authoritative snapshot/receipt support")
	}
	account, err := tx.AccountByName(ctx, "test001")
	if err != nil {
		return r, err
	}
	if account == nil || account.Username != "test001" {
		return r, fmt.Errorf("test001 account missing")
	}
	warehouses := make([]*domain.Warehouse, 3)
	before := make([]domain.Snapshot, 3)
	next := make([]domain.Snapshot, 3)
	states := make([]domain.EndgameState, 3)
	receipts := make([]*domain.EndgameReceipt, 3)
	seen := map[int64]bool{}
	receiptCount := 0
	for i, id := range targetIDs {
		if err = eg.LockEndgameCharacter(ctx, id); err != nil {
			return r, err
		}
		before[i], err = eg.LoadEndgameSnapshot(ctx, domain.Snapshot{Char: &domain.Character{ID: id}})
		if err != nil {
			return r, err
		}
		warehouses[i], err = tx.LoadWarehouse(ctx, id, warehouseRule)
		if err != nil {
			return r, err
		}
		ch := before[i].Char
		if ch == nil || ch.AccountID != account.ID {
			return r, fmt.Errorf("character %d does not belong to test001", id)
		}
		if err = ValidateTarget(ch); err != nil {
			return r, err
		}
		if before[i].Bag == nil {
			return r, fmt.Errorf("character %d has no bag", id)
		}
		receipts[i], err = eg.LoadEndgameReceipt(ctx, id, ReceiptKey)
		if err != nil {
			return r, err
		}
		if receipts[i] != nil {
			receiptCount++
			if receipts[i].Operation != operation || receipts[i].Digest != d {
				return r, fmt.Errorf("existing receipt mismatch; refusing another grant")
			}
		}
		states[i], err = eg.LoadEndgameState(ctx, id)
		if err != nil {
			return r, err
		}
		a := CharacterAudit{ID: id, Name: ch.Name, Gender: ch.Appear.Gender, BeforeCharacterSHA256: digest(ch.Clone()), BeforeBag: bagAudit(before[i].Bag)}
		r.Characters = append(r.Characters, a)
	}
	if receiptCount != 0 {
		if receiptCount != 3 {
			return r, fmt.Errorf("partial prior receipts; refuse duplicate or repair automatically")
		}
		for i, receipt := range receipts {
			var a CharacterAudit
			if err = json.Unmarshal([]byte(receipt.Response), &a); err != nil {
				return r, fmt.Errorf("invalid receipt payload")
			}
			if a.ID != targetIDs[i] || len(a.Granted) != 9 {
				return r, fmt.Errorf("invalid prior grant receipt")
			}
			for _, item := range a.Granted {
				if !reflect.DeepEqual(before[i].Bag.At(item.Slot), item.Stack) {
					return r, fmt.Errorf("prior grant item moved or changed for %d; receipt prevents reissue", a.ID)
				}
			}
			r.Characters[i].Granted = a.Granted
			r.Characters[i].AfterBag = bagAudit(before[i].Bag)
			r.Characters[i].AfterCharacterSHA256 = digest(before[i].Char.Clone())
			r.Characters[i].UnrelatedStateUnchanged = true
		}
		r.Status = "already-applied"
		return r, nil
	}
	for i := range before {
		next[i] = before[i].Clone()
		stacks, e := Build(before[i].Char, defs)
		if e != nil {
			return r, e
		}
		if len(stacks) != 9 {
			return r, fmt.Errorf("expected nine items")
		}
		for _, st := range stacks {
			ids := append([]int64{st.UID}, st.SocketUIDs[:]...)
			for _, uid := range ids {
				if uid == 0 {
					continue
				}
				if uid < 0 || seen[uid] {
					return r, fmt.Errorf("duplicate/invalid new UID")
				}
				seen[uid] = true
			}
			def := defs[st.Item]
			slot := next[i].Bag.FirstEmptyFor(def)
			if slot < 0 || !next[i].Bag.At(slot).Empty() {
				return r, fmt.Errorf("character %d needs nine free equipment bag slots", targetIDs[i])
			}
			if next[i].Bag.AddStack(def, st) != 0 || !reflect.DeepEqual(next[i].Bag.At(slot), st) {
				return r, fmt.Errorf("production bag insertion failed")
			}
			r.Characters[i].Granted = append(r.Characters[i].Granted, newItemAudit(slot, st, def))
		}
		r.Characters[i].AfterBag = bagAudit(next[i].Bag)
		r.Characters[i].AfterCharacterSHA256 = digest(next[i].Char.Clone())
		if err = verifyDelta(before[i], next[i], r.Characters[i].Granted); err != nil {
			return r, err
		}
		r.Characters[i].UnrelatedStateUnchanged = true
	}
	if len(seen) != TotalInstanceCount {
		return r, fmt.Errorf("expected %d equipment + %d embedded rune UIDs, got %d", EquipmentCount, EmbeddedRuneCount, len(seen))
	}
	checker, ok := tx.(store.ShowcaseUIDChecker)
	if !ok {
		return r, fmt.Errorf("transaction lacks global UID collision checks")
	}
	uids := make([]int64, 0, len(seen))
	for uid := range seen {
		uids = append(uids, uid)
	}
	if err = checker.CheckShowcaseUIDsUnused(ctx, uids); err != nil {
		return r, err
	}
	if !apply {
		return r, nil
	}
	for i := range next {
		if err = tx.SaveSnapshot(ctx, next[i]); err != nil {
			return r, err
		}
		state := states[i]
		state.Revision++
		if err = eg.SaveEndgameState(ctx, targetIDs[i], state, states[i].Revision); err != nil {
			return r, err
		}
	}
	// Round-trip every field/instance before recording receipts or committing.
	for i := range next {
		got, e := eg.LoadEndgameSnapshot(ctx, domain.Snapshot{Char: &domain.Character{ID: targetIDs[i]}})
		if e != nil {
			return r, e
		}
		if err = verifyDelta(before[i], got, r.Characters[i].Granted); err != nil {
			return r, err
		}
		warehouse, e := tx.LoadWarehouse(ctx, targetIDs[i], warehouseRule)
		if e != nil {
			return r, e
		}
		if !reflect.DeepEqual(warehouses[i], warehouse) {
			return r, fmt.Errorf("unrelated warehouse changed; rollback required")
		}
		for j := range r.Characters[i].Granted {
			a := &r.Characters[i].Granted[j]
			a.Stack = got.Bag.At(a.Slot)
		}
		r.Characters[i].AfterBag = bagAudit(got.Bag)
		r.Characters[i].AfterCharacterSHA256 = digest(got.Char.Clone())
		payload, e := json.Marshal(r.Characters[i])
		if e != nil {
			return r, e
		}
		if err = eg.SaveEndgameReceipt(ctx, domain.EndgameReceipt{CharID: targetIDs[i], RequestID: ReceiptKey, Operation: operation, Digest: d, Revision: states[i].Revision + 1, Response: string(payload)}); err != nil {
			return r, err
		}
	}
	r.Status = "verified-pending-commit"
	return r, nil
}

func verifyDelta(before, after domain.Snapshot, added []ItemAudit) error {
	if before.Bag.Cap() != after.Bag.Cap() {
		return fmt.Errorf("bag capacity changed")
	}
	// Clone both sides symmetrically: production readers may return empty
	// slices while production Clone normalizes them to nil. No values are ignored.
	stripped := after.Clone()
	for _, a := range added {
		if !before.Bag.At(a.Slot).Empty() || !sameGrantedStack(after.Bag.At(a.Slot), a.Stack) {
			return fmt.Errorf("new item round-trip/slot mismatch")
		}
		stripped.Bag.Set(a.Slot, domain.Stack{})
	}
	if !reflect.DeepEqual(before.Clone(), stripped) {
		return fmt.Errorf("unrelated character or old inventory changed; rollback required")
	}
	return nil
}

func newItemAudit(slot int, st domain.Stack, def domain.ItemDef) ItemAudit {
	a := ItemAudit{Slot: slot, Stack: st, DisplayName: def.Name, RequiredLevel: domain.EquipmentRequiredLevel(def, st)}
	if u, ok := domain.UniqueOf(st); ok {
		a.DisplayName = u.Name
		a.ResolvedAffixes = domain.UniqueItemBonuses(def, st)
	} else if r, ok := domain.MatchAstralRecipe(def, st); ok {
		a.DisplayName = r.Name + " · " + def.Name
		a.ResolvedAffixes = domain.AstralItemBonuses(def, st, r)
	}
	return a
}

// The production store's hydrateStack preserves the historical location-table
// equipment representation (InstanceKindNone) even though item_instances is
// Equipment. Accept exactly that documented alias for a newly built equipment;
// all UID, item, roll, socket, Card and other values remain exact.
func sameGrantedStack(actual, expected domain.Stack) bool {
	if expected.InstanceKind == domain.ItemInstanceEquipment && actual.InstanceKind == domain.ItemInstanceNone {
		actual.InstanceKind = domain.ItemInstanceEquipment
	}
	return reflect.DeepEqual(actual, expected)
}
