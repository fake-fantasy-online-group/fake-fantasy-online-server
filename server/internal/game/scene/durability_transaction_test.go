package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"testing"
)

func durabilityTransactionScene(t *testing.T) (*Scene, *entity.Entity, *astralTestBackend) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	s.items = map[domain.ItemID]domain.ItemDef{
		4002: {ID: 4002, Name: "钢剑", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment,
			Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), Durable: 100, Bonus: domain.Stats{MinAtk: 8, MaxAtk: 14}, AttackDurabilityCostRaw: 75, BeHitDurabilityCostRaw: 100, DeathDurabilityCostRaw: 150}},
		5001: {ID: 5001, Name: "铁甲", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment,
			Equip: &domain.EquipDef{Slot: int32(domain.SlotBody), Durable: 100, BeHitDurabilityCostRaw: 275, DeathDurabilityCostRaw: 300}},
	}
	p.Player.Worn = domain.NewEquipSet()
	p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{UID: 1001, Item: 4002, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Durability: 100, MaxDurability: 100, DurabilityWearRaw: 200})
	p.Player.Worn.Set(domain.SlotBody, domain.Stack{UID: 1002, Item: 5001, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Durability: 100, MaxDurability: 100, DurabilityWearRaw: 25})
	b := &astralTestBackend{}
	s.endgameBackend = b
	s.endgamePlayer(p).loaded = true
	s.refreshStats(p)
	p.Player.TakeDirty()
	return s, p, b
}
func beginDurabilityTransaction(t *testing.T, s *Scene, p *entity.Entity, candidate domain.Snapshot) {
	t.Helper()
	if !s.endgameCommit(p, domain.EndgameRequest{Op: "cube.execute", RequestID: "durability-transaction-test", ExpectedRevision: "0"}, candidate, domain.EndgameResponse{Message: "test"}, nil, 0, nil) {
		t.Fatal("failed to reserve")
	}
}
func TestEndgameDurabilityDeferredAcrossEverySettlement(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "recovery", "receipt_replay"} {
		t.Run(outcome, func(t *testing.T) {
			s, p, b := durabilityTransactionScene(t)
			before := p.Player.Worn.Clone()
			candidate := s.snapshotOf(p)
			weapon := candidate.Worn.At(domain.SlotWeapon)
			weapon.RefineLevel = 2
			candidate.Worn.Set(domain.SlotWeapon, weapon)
			beginDurabilityTransaction(t, s, p, candidate)
			s.consumeAttackDurability(p)
			s.consumeAttackDurability(p)
			s.consumeHitDurability(p, event.DamageDealt{Amount: 10})
			s.consumeDeathDurability(p)
			s.consumeSkillWeaponDurability(p, skillWeaponDurabilityCost{slot: domain.SlotWeapon, amount: 3}, 11001)
			s.consumeHitDurability(p, event.DamageDealt{})
			s.consumeHitDurability(p, event.DamageDealt{Amount: 10, Flag: event.DamageMiss})
			for i := 0; i < 3; i++ {
				s.finishEndgame(false)
			}
			if !p.Player.TradeBusy || !reflect.DeepEqual(p.Player.Worn, before) || !reflect.DeepEqual(b.plan.Before.Worn, before) || !reflect.DeepEqual(b.plan.Snapshot.Worn, candidate.Worn) || len(s.tradeDeferred[p.ID]) != 5 {
				t.Fatal("pending wear mutated reserved state")
			}
			r := domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			wantDur, wantRefine := int32(95), int32(0)
			switch outcome {
			case "commit":
				wantRefine = 2
			case "rollback":
				r.Err = errors.New("definite rollback")
			case "recovery":
				recovered := b.plan.Before.Clone()
				st := recovered.Worn.At(domain.SlotWeapon)
				st.Durability = 80
				recovered.Worn.Set(domain.SlotWeapon, st)
				r.Authoritative = &recovered
				r.RequiresReload = true
				r.Err = errors.New("stale revision")
				wantDur = 75
			case "receipt_replay":
				r.Replayed = true
			}
			b.result <- r
			s.finishEndgame(false)
			weapon, body := p.Player.Worn.At(domain.SlotWeapon), p.Player.Worn.At(domain.SlotBody)
			if p.Player.TradeBusy || weapon.Durability != wantDur || weapon.DurabilityWearRaw != 0 || weapon.RefineLevel != wantRefine || body.Durability != 98 || body.DurabilityWearRaw != 0 || len(s.tradeDeferred[p.ID]) != 0 {
				t.Fatalf("lost/duplicated wear: weapon=%+v body=%+v", weapon, body)
			}
			if !p.Player.TakeDirty() {
				t.Fatal("wear not marked for persistence")
			}
			after := p.Player.Worn.Clone()
			s.finishEndgame(false)
			s.runDeferredTradeMutations(p.ID)
			if !reflect.DeepEqual(after, p.Player.Worn) {
				t.Fatal("wear replayed twice")
			}
		})
	}
}
func TestEndgameDeferredDurabilityDoesNotWearReplacement(t *testing.T) {
	for _, replacement := range []string{"new_uid", "new_base", "moved", "removed"} {
		t.Run(replacement, func(t *testing.T) {
			s, p, b := durabilityTransactionScene(t)
			candidate := s.snapshotOf(p)
			st := candidate.Worn.At(domain.SlotWeapon)
			switch replacement {
			case "new_uid":
				st.UID++
				candidate.Worn.Set(domain.SlotWeapon, st)
			case "new_base":
				st.Item = 5001
				candidate.Worn.Set(domain.SlotWeapon, st)
			case "moved":
				candidate.Worn.Set(domain.SlotWeapon, domain.Stack{})
				candidate.Worn.Set(domain.SlotTwoHand, st)
			case "removed":
				candidate.Worn.Set(domain.SlotWeapon, domain.Stack{})
			}
			beginDurabilityTransaction(t, s, p, candidate)
			s.consumeAttackDurability(p)
			s.consumeSkillWeaponDurability(p, skillWeaponDurabilityCost{slot: domain.SlotWeapon, amount: 3}, 11001)
			b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			s.finishEndgame(false)
			if !reflect.DeepEqual(p.Player.Worn, candidate.Worn) || len(s.tradeDeferred[p.ID]) != 0 {
				t.Fatal("old hit wore substituted gear")
			}
		})
	}
}
func TestEndgameDeferredDurabilityBreaksEquipment(t *testing.T) {
	for _, mode := range []string{"raw", "skill"} {
		for _, disappear := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "_broken", true: "_disappears"}[disappear], func(t *testing.T) {
				s, p, b := durabilityTransactionScene(t)
				s.items[4002].Equip.DisappearIfZero = disappear
				s.items[4002].Equip.AttackDurabilityCostRaw = 300
				st := p.Player.Worn.At(domain.SlotWeapon)
				st.Durability, st.DurabilityWearRaw = 1, 0
				p.Player.Worn.Set(domain.SlotWeapon, st)
				beginDurabilityTransaction(t, s, p, s.snapshotOf(p))
				for i := 0; i < 2; i++ {
					if mode == "raw" {
						s.consumeAttackDurability(p)
					} else {
						s.consumeSkillWeaponDurability(p, skillWeaponDurabilityCost{slot: domain.SlotWeapon, amount: 1}, 11001)
					}
				}
				if p.Player.Worn.At(domain.SlotWeapon) != st {
					t.Fatal("premature break")
				}
				b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
				s.finishEndgame(false)
				st = p.Player.Worn.At(domain.SlotWeapon)
				if st.Durability != 0 || st.DurabilityWearRaw != 0 || st.Empty() != disappear {
					t.Fatal(st)
				}
				after := p.Stats
				p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
				s.refreshStats(p)
				if after != p.Stats {
					t.Fatal("broken equipment contributed stats")
				}
			})
		}
	}
}

type durabilitySnapshotSaver struct{ snapshots []domain.Snapshot }

func (s *durabilitySnapshotSaver) Save(snapshot domain.Snapshot) {
	s.snapshots = append(s.snapshots, snapshot.Clone())
}
func TestEndgameDeferredDurabilityPrecedesLogoutSave(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[rollback], func(t *testing.T) {
			s, p, b := durabilityTransactionScene(t)
			saver := &durabilitySnapshotSaver{}
			s.saver = saver
			beginDurabilityTransaction(t, s, p, s.snapshotOf(p))
			s.consumeHitDurability(p, event.DamageDealt{Amount: 10})
			s.consumeSkillWeaponDurability(p, skillWeaponDurabilityCost{slot: domain.SlotWeapon, amount: 2}, 11001)
			p.Player.MarkDirty()
			s.saveDirty()
			s.exec(Leave{ID: p.ID, FinalSaver: saver})
			if len(saver.snapshots) != 0 || s.players[p.ID] == nil {
				t.Fatal("pending transaction escaped through logout")
			}
			r := domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
			if rollback {
				r.Err = errors.New("definite rollback")
			}
			b.result <- r
			s.finishEndgame(false)
			if len(saver.snapshots) != 1 || s.players[p.ID] != nil {
				t.Fatal("logout not finalized once")
			}
			saved := saver.snapshots[0]
			if saved.Worn.At(domain.SlotWeapon).Durability != 97 || saved.Worn.At(domain.SlotBody).Durability != 99 {
				t.Fatal("final save lost wear")
			}
		})
	}
}
func TestEndgameDeferredDurabilityUsesOriginalCost(t *testing.T) {
	s, p, b := durabilityTransactionScene(t)
	beginDurabilityTransaction(t, s, p, s.snapshotOf(p))
	s.consumeAttackDurability(p)
	s.items[4002].Equip.AttackDurabilityCostRaw = 3000
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
	st := p.Player.Worn.At(domain.SlotWeapon)
	if st.Durability != 100 || st.DurabilityWearRaw != 275 {
		t.Fatal("recomputed old cost", st)
	}
}
