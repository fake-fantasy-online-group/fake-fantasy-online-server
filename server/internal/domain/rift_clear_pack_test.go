package domain

import (
	"reflect"
	"testing"
)

func TestRiftClearPackPotionTableUsesExistingCatalogItems(t *testing.T) {
	rewards := RiftClearPackPotionRewards()
	if len(rewards) != 14 {
		t.Fatalf("potion rows %d", len(rewards))
	}
	seen := map[ItemID]bool{}
	for _, r := range rewards {
		if seen[r.Item] {
			t.Fatalf("duplicate potion %d", r.Item)
		}
		seen[r.Item] = true
		if r.ChanceBP != 10000 || r.MinQty <= 0 || r.MaxQty < r.MinQty {
			t.Fatalf("invalid potion row %+v", r)
		}
	}
	if !seen[3121] || !seen[3151] {
		t.Fatal("天山雪莲 and 长白参 must both be present as distinct items")
	}
	item := NewRiftClearPackItem()
	if item.ID != RiftClearPackItemID || item.Name != RiftClearPackName || item.FixedBinding != ItemBindingFixedBound || item.GachaBox == nil || item.GachaBox.MinFreeSlots != RiftClearPackMinFreeSlots {
		t.Fatalf("pack definition %+v", item)
	}
	if !reflect.DeepEqual(item.GachaBox.Rewards, rewards) {
		t.Fatal("pack gacha table mismatch")
	}
}

func TestRiftClearPackTrueSetHitIsExtremelyRareAndTestable(t *testing.T) {
	if RiftClearPackTrueSetChanceBP != 1 {
		t.Fatal("rare chance must stay the lowest representable rate")
	}
	if RiftClearPackTrueSetHit(nil) || !RiftClearPackTrueSetHit(func(int) int { return 0 }) || RiftClearPackTrueSetHit(func(int) int { return 1 }) {
		t.Fatal("deterministic true-set roll")
	}
}

func TestTrueSetPiecesForCharacterAreClassAndSexLocked(t *testing.T) {
	warriorPiece := HardDungeonItemOffset + 8005
	warlockMale := HardDungeonItemOffset + 8401
	warlockFemale := HardDungeonItemOffset + 8451
	items := map[ItemID]ItemDef{
		warriorPiece:  hardPiece(warriorPiece, "真*战士甲", 1<<Warrior, 0),
		warlockMale:   hardPiece(warlockMale, "真*术士男袍", 1<<Warlock, 1),
		warlockFemale: hardPiece(warlockFemale, "真*术士女袍", 1<<Warlock, 2),
	}
	warlock := &Character{Race: Warlock, Employed: true, EmploymentKnown: true, Appear: Appearance{Gender: 1}}
	got := TrueSetPiecesForCharacter(items, warlock)
	if !reflect.DeepEqual(got, []ItemID{warlockMale}) {
		t.Fatalf("warlock male pieces %v", got)
	}
	warlock.Appear.Gender = 0
	got = TrueSetPiecesForCharacter(items, warlock)
	if !reflect.DeepEqual(got, []ItemID{warlockFemale}) {
		t.Fatalf("warlock female pieces %v", got)
	}
	warrior := &Character{Race: Warrior, Employed: true, EmploymentKnown: true, Appear: Appearance{Gender: 1}}
	got = TrueSetPiecesForCharacter(items, warrior)
	if !reflect.DeepEqual(got, []ItemID{warriorPiece}) {
		t.Fatalf("warrior pieces %v", got)
	}
	if got := TrueSetPiecesForCharacter(items, nil); len(got) != 0 {
		t.Fatal("nil character")
	}
}

func hardPiece(id ItemID, name string, professions uint8, sex uint8) ItemDef {
	return ItemDef{ID: id, Name: name, HardDungeonOnly: true, Equip: &EquipDef{Need: Requirement{Professions: professions, ProfessionRestricted: true, Sex: sex}}}
}

func TestRiftRunResumeAllowedKeepsCompletedButNotCancelled(t *testing.T) {
	allowed := []RiftPhase{RiftReserved, RiftActive, RiftBoss, RiftBossArriving, RiftCompleted}
	blocked := []RiftPhase{RiftFailed, RiftAborted, RiftClosed, ""}
	for _, phase := range allowed {
		if !(&RiftRun{Phase: phase}).ResumeAllowed() {
			t.Fatalf("phase %s should still resume", phase)
		}
	}
	for _, phase := range blocked {
		if (&RiftRun{Phase: phase}).ResumeAllowed() {
			t.Fatalf("phase %s must not resume", phase)
		}
	}
	if (*RiftRun)(nil).ResumeAllowed() {
		t.Fatal("nil run")
	}
}
