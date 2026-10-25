package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"strings"
	"testing"
)

func TestWingPowerThreeRuntimeEffectsAndDuplicate(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A01")
	weapon := p.Player.Worn.At(domain.SlotWeapon)
	weapon.Legendary = domain.LegendaryInstance{}
	p.Player.Worn.Set(domain.SlotWeapon, weapon)
	wing := domain.Stack{UID: 1001, Item: 20609, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Durability: 100, MaxDurability: 100}
	for i, id := range []string{"ZS-A01", "ZS-D02", "TY-U01"} {
		wing.WingPowers[i] = domain.WingPower{PowerID: id, SourceLevel: 100, SourceTier: domain.LegendaryTierSecret}
	}
	def := domain.ItemDef{ID: wing.Item, Name: "光之翼", Equip: &domain.EquipDef{Slot: int32(domain.SlotBag), Durable: 100, Bonus: domain.Stats{Def: 10}}}
	s.items[wing.Item] = def
	p.Player.Worn.Set(domain.SlotBag, wing)
	if got := domain.ActiveLegendaryPowers(p.Player.Worn, s.itemDef, p.Player.Char); len(got) != 3 {
		t.Fatal(got)
	}
	d := domain.SkillDef{ID: 10002, Level: 1, TargetEnemy: true, Kind: domain.SkillSingle, HurtType: domain.HurtPhysical, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
	before := m.HP
	warriorTestCast(t, s, p, m, d)
	if got := before - m.HP; got != 180 {
		t.Fatalf("wing attack damage=%d", got)
	}
	utility := domain.SkillDef{ID: 123, Kind: domain.SkillSingle, TargetEnemy: true, PrepareMS: 1000, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
	s.legendaryPrepareSkill(p, &utility, m.ID)
	if utility.PrepareMS != 700 {
		t.Fatal("wing utility not applied", utility.PrepareMS)
	}
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillPct: 100, GuaranteedHit: true})
	if ev.Amount != 84 {
		t.Fatal("wing defense not applied", ev.Amount)
	}
	weapon.Legendary.PowerID = "ZS-A01"
	p.Player.Worn.Set(domain.SlotWeapon, weapon)
	active := domain.ActiveLegendaryPowers(p.Player.Worn, s.itemDef, p.Player.Char)
	if len(active) != 3 || active[0].SourceUID == 0 {
		t.Fatal("duplicate not deduped", active)
	}
	base, stats := domain.LegendaryIntrinsic(def, wing)
	if base != (domain.Base{}) || stats.Def != 10 {
		t.Fatal("donor tier multiplied wing white stats", base, stats)
	}
	tip := s.ownedItemTooltip(def, wing, p)
	for _, label := range []string{"羽翼攻击威能", "羽翼防御威能", "羽翼通用威能", "翼魂", "威能 ·", "空槽"} {
		if strings.Contains(tip, label) {
			t.Fatalf("invented power presentation %q: %s", label, tip)
		}
	}
	for _, v := range wing.WingPowers {
		power, _ := domain.LegendaryPower(v.PowerID)
		if strings.Count(tip, power.Effect) != 1 || !strings.Contains(tip, "\x06"+power.Name+"："+power.Effect) {
			t.Fatal("missing/or duplicated wing tooltip", tip)
		}
	}
	if strings.Contains(itemDisplayName(def, wing), "秘*") || equipmentQuality(def, wing) == 5 {
		t.Fatal("wing was falsely converted to darkgold")
	}
}
