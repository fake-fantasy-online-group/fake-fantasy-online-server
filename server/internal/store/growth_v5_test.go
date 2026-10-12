package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
	"time"
)

func TestGrowthV5CharacterCloneOwnsCooldownMap(t *testing.T) {
	c := &domain.Character{SkillAllocationVersion: 5, EquipmentProcCooldowns: map[string]int64{"global": 1234, "effect:flame": 5678}}
	cp := c.Clone()
	cp.EquipmentProcCooldowns["global"] = 0
	delete(cp.EquipmentProcCooldowns, "effect:flame")
	if c.EquipmentProcCooldowns["global"] != 1234 || c.EquipmentProcCooldowns["effect:flame"] != 5678 {
		t.Fatal("snapshot aliases live proc cooldowns")
	}
	if cp.SkillAllocationVersion != 5 {
		t.Fatal("allocation version lost")
	}
}

func TestGrowthV5PersistentProcCooldownRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	acc, err := st.CreateAccount(ctx, "v5cooldown", "")
	if err != nil {
		t.Fatal(err)
	}
	ch := &domain.Character{AccountID: acc.ID, Name: "冷却测试", Level: 60, Pos: domain.Pos{MapID: 7}, SkillAllocationVersion: 5}
	if err = st.CreateChar(ctx, ch); err != nil {
		t.Fatal(err)
	}
	ch.EquipmentProcCooldowns = map[string]int64{"global": time.Now().Add(time.Second).UnixMilli(), "group:fire": time.Now().Add(15 * time.Second).UnixMilli()}
	if err = st.SaveSnapshot(ctx, domain.Snapshot{Char: ch}); err != nil {
		t.Fatal(err)
	}
	got, err := st.CharByName(ctx, ch.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.EquipmentProcCooldowns, ch.EquipmentProcCooldowns) || got.SkillAllocationVersion != 5 {
		t.Fatalf("persistent v5 state lost: %+v", got)
	}
	ch.EquipmentProcCooldowns = nil
	if err = st.SaveSnapshot(ctx, domain.Snapshot{Char: ch}); err != nil {
		t.Fatal(err)
	}
	got, err = st.CharByName(ctx, ch.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.EquipmentProcCooldowns) != 0 {
		t.Fatal("cleared cooldown resurrected")
	}
}
