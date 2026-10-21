package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"testing"
	"time"
)

func meleeSetFixture(t *testing.T, suitID int32, pieces int) (*Scene, *entity.Entity) {
	t.Helper()
	s := newCombatScene(t)
	join(t, s, 1, 100, "set", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Warrior
	if suitID == 4 {
		p.Player.Char.Race = domain.Swordsman
	}
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Employed = true
	p.Player.Char.Level = 60
	p.Player.Char.Base = domain.Base{STR: 100, VIT: 100, INT: 100, SPI: 100, AGI: 100, DEX: 100}
	slots := []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotWeapon, domain.SlotGlove, domain.SlotRing, domain.SlotBody, domain.SlotShoe, domain.SlotBag}
	suit := &domain.EquipmentSuit{ID: suitID}
	s.items = map[domain.ItemID]domain.ItemDef{}
	for i, slot := range slots {
		id := domain.ItemID(800001 + i)
		suit.Groups = append(suit.Groups, []domain.ItemID{id})
		s.items[id] = domain.ItemDef{ID: id, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit}}
		if i < pieces {
			p.Player.Worn.Set(slot, domain.Stack{Item: id, Count: 1, Durability: 100, MaxDurability: 100})
		}
	}
	s.refreshStats(p)
	return s, p
}
func TestClassSetWarriorThresholdStatsAndCache(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		s, p := meleeSetFixture(t, 5, n)
		stats := p.Stats
		_, cached, _ := s.combatGrowthStats(p)
		if cached != stats {
			t.Fatalf("n%d panel/cache mismatch", n)
		}
		s.refreshStats(p)
		if p.Stats != stats {
			t.Fatal("recalculation compounded multipliers")
		}
		if s.classSetDragonActive(p) != (n >= 4) {
			t.Fatalf("dragon threshold %d", n)
		}
		p.HP = p.MaxHP / 2
		want := int32(1000)
		if n >= 2 {
			want = 1500
		}
		if got := s.classSetMissingHealthDamage(p, 1000); got != want {
			t.Fatalf("threshold%d damage%d want%d", n, got, want)
		}
	}
	s, p := meleeSetFixture(t, 5, 4)
	before := p.Stats
	st := p.Player.Worn.At(domain.SlotGlove)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotGlove, st)
	s.refreshStats(p)
	if before.MaxHP != 2*p.MaxHP || before.MinAtk != p.Stats.MinAtk*160/100 || before.MaxAtk != p.Stats.MaxAtk*160/100 || before.MoveSpeed != p.Stats.MoveSpeed*120/100 {
		t.Fatalf("transform restore before%+v after%+v", before, p.Stats)
	}
}
func TestClassSetWarriorActualStrikeAndCost(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	p.HP, p.MaxHP = 5000, 10000
	p.MP = 0
	p.Stats.MAtk = 1000
	p.Stats.MDef = 0
	target := putMonster(s, 900, 100000, domain.Stats{MAtk: 1000}, 110, 100)
	ev, _, _ := s.resolveStrikeStatuses(p, target, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 1500 {
		t.Fatalf("actual missingHP strike %d", ev.Amount)
	}
	ev, _, _ = s.resolveStrikeStatuses(target, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 1500 {
		t.Fatalf("actual dragon vulnerability %d", ev.Amount)
	}
	p.HP = 1000
	s.classSetPaySkillCost(p, domain.SkillDef{MPCost: 200})
	if p.HP != 900 || p.MP != 0 {
		t.Fatal("cost used wrong resource")
	}
	for _, hp := range []int32{1, 2, 9, 10, 19, 1000} {
		c := classSetBloodCost(hp)
		if c >= hp || c < 0 {
			t.Fatal("lethal cost", hp, c)
		}
	}
	s.skills = skillTable(连环枪法)
	p.Player.Char.Skills = domain.Learned{连环枪法.ID: 1}
	p.HP = 1000
	def := 连环枪法
	s.applyResolvedSkillModifiers(p, &def, p.Player.Char.Skills, p.Player.Char.Skills)
	if s.skillBehaviorPayload(p, def).MPCost != 0 {
		t.Fatal("client still sees MP cost")
	}
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 连环枪法.ID, Target: target.ID})
	if len(s.casts) != 1 || p.HP != 900 || p.MP != 0 {
		t.Fatalf("real accepted skill hp%d mp%d casts%d", p.HP, p.MP, len(s.casts))
	}
}
func TestClassSetSwordStacksLifetimeAndWhirlwind(t *testing.T) {
	s, p := meleeSetFixture(t, 4, 4)
	taunt := domain.SkillDef{ID: classSetTaunt}
	for i := 0; i < 15; i++ {
		s.classSetMeleeCastReleased(p, &taunt)
	}
	if s.classSetUnrulyStacks(p) != 10 {
		t.Fatal("stack cap")
	}
	if s.classSetMeleeIncomingDamage(p, 1000) != 500 {
		t.Fatal("ten stacks not 50 percent")
	}
	def := domain.SkillDef{ID: classSetWhirlwind, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 150}}
	scale := s.classSetMeleeCastReleased(p, &def)
	if s.classSetUnrulyStacks(p) != 9 || scale != 20000 {
		t.Fatalf("consume/damage stacks%d effect%+v", s.classSetUnrulyStacks(p), def.DirectEffects())
	}
	s.tick += domain.Ticks(30000)
	if s.classSetUnrulyStacks(p) != 0 {
		t.Fatal("expired stacks")
	}
	s.classSetMeleeCastReleased(p, &taunt)
	p.HP = 0
	if s.classSetUnrulyStacks(p) != 0 {
		t.Fatal("dead stacks")
	}
	p.HP = 100
	s.classSetMeleeCastReleased(p, &taunt)
	for _, slot := range []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotGlove} {
		st := p.Player.Worn.At(slot)
		st.Durability = 0
		p.Player.Worn.Set(slot, st)
	}
	s.refreshStats(p)
	if s.classSetUnrulyStacks(p) != 0 {
		t.Fatal("broken equipment retained stacks")
	}
}
func TestClassSetSwordActualCastSingleStackAndAllTargets(t *testing.T) {
	s, p := meleeSetFixture(t, 4, 4)
	p.Stats.MAtk = 1000
	p.HP = p.MaxHP
	a := putMonster(s, 900, 100000, domain.Stats{MDef: 100}, 110, 100)
	b := putMonster(s, 901, 100000, domain.Stats{MDef: 200}, 120, 100)
	baseA := combat.CalculateStrike(p, a, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire}, s.rng).Amount
	baseB := combat.CalculateStrike(p, b, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire}, s.rng).Amount
	taunt := domain.SkillDef{ID: classSetTaunt, Kind: domain.SkillArea, Effect: domain.SkillEffect{Kind: domain.EffectTaunt, TauntPower: 5}}
	s.casts = []pendingCast{{src: p.ID, def: taunt, targets: []domain.EntityID{a.ID, b.ID}}}
	s.stepCastsAt(time.Now(), false)
	if s.classSetUnrulyStacks(p) != 1 {
		t.Fatal("taunt must stack once per cast")
	}
	def := domain.SkillDef{ID: classSetWhirlwind, Kind: domain.SkillArea, HurtType: domain.HurtFire, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.casts = []pendingCast{{src: p.ID, def: def, targets: []domain.EntityID{a.ID, b.ID}}}
	s.stepCastsAt(time.Now(), false)
	if a.HP != 100000-2*baseA || b.HP != 100000-2*baseB || s.classSetUnrulyStacks(p) != 0 {
		t.Fatalf("AOE double damage a%d b%d stacks%d", a.HP, b.HP, s.classSetUnrulyStacks(p))
	}
}

func TestClassSetMeleeSecondaryDamageAndLifesteal(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	p.HP = 1000
	p.MaxHP = 2000
	dot := domain.Active{Def: domain.StatusDef{ID: 1001, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -100}}}, Source: 99}
	s.tickStatus(p, dot)
	if p.HP != 850 {
		t.Fatalf("dragon DOT vulnerability %d", p.HP)
	}
	foe := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
	s.emitReflectedDamage(foe, p, 100, false)
	if p.HP != 700 {
		t.Fatalf("reflection multiplier must apply once %d", p.HP)
	}
	p.Stats.MAtk = 1000
	p.HP = 1000
	ev, raw, reflect := s.resolveStrikeStatuses(p, foe, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	s.afterStrikeStatuses(p, foe, ev, raw, reflect)
	if ev.Amount != 1500 || p.HP != 1300 {
		t.Fatalf("life steal should restore 20 percent of damage amount%d hp%d", ev.Amount, p.HP)
	}
	s, p = meleeSetFixture(t, 4, 4)
	p.HP = 1000
	taunt := domain.SkillDef{ID: classSetTaunt}
	for i := 0; i < 10; i++ {
		s.classSetMeleeCastReleased(p, &taunt)
	}
	s.tickStatus(p, dot)
	if p.HP != 950 {
		t.Fatalf("Unruly DOT reduction %d", p.HP)
	}
	foe = putMonster(s, 900, 10000, domain.Stats{MAtk: 1000}, 110, 100)
	p.Stats.MDef = 0
	ev, _, _ = s.resolveStrikeStatuses(foe, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 500 {
		t.Fatalf("actual ten stack mitigation %d", ev.Amount)
	}
}

func TestClassSetSwordThresholdsAndCanceledCast(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		s, p := meleeSetFixture(t, 4, n)
		taunt := domain.SkillDef{ID: classSetTaunt}
		s.classSetMeleeCastReleased(p, &taunt)
		expected := int32(0)
		if n >= 2 {
			expected = 1
		}
		if s.classSetUnrulyStacks(p) != expected {
			t.Fatalf("taunt threshold%d", n)
		}
		def := domain.SkillDef{ID: classSetWhirlwind, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
		scale := s.classSetMeleeCastReleased(p, &def)
		want := int32(10000)
		if n >= 4 {
			want = 20000
		}
		if scale != want {
			t.Fatalf("whirlwind threshold%d", n)
		}
	}
	s, p := meleeSetFixture(t, 4, 4)
	taunt := domain.SkillDef{ID: classSetTaunt}
	s.casts = []pendingCast{{src: p.ID, def: taunt, windup: &castWindup{interrupted: true}}}
	s.stepCastsAt(time.Now(), false)
	if s.classSetUnrulyStacks(p) != 0 {
		t.Fatal("canceled chant gained stack")
	}
}

func TestClassSetUnrulyBuffProjection(t *testing.T) {
	s, p := meleeSetFixture(t, 4, 4)
	s.skills = skillTable(domain.SkillDef{ID: classSetTaunt, Level: 1, Icon: 123})
	taunt := domain.SkillDef{ID: classSetTaunt}
	s.classSetMeleeCastReleased(p, &taunt)
	found := false
	for _, b := range s.buffSnapshot(p).Buffs {
		if b.SkillID == classSetUnrulyViewID {
			found = true
			if b.Name != "不羁（1层）" || b.Icon != 123 || b.RemainSec != 30 {
				t.Fatalf("wrong projection %+v", b)
			}
		}
	}
	if !found {
		t.Fatal("missing Unruly buff")
	}
	s.tick += domain.Ticks(30000)
	for _, b := range s.buffSnapshot(p).Buffs {
		if b.SkillID == classSetUnrulyViewID {
			t.Fatal("expired projection")
		}
	}
}

func TestClassSetDragonToggleNeverGrantsHealth(t *testing.T) {
	for _, startHP := range []int32{0, 1, 100, 1800} {
		s, p := meleeSetFixture(t, 5, 4)
		st := p.Player.Worn.At(domain.SlotGlove)
		p.HP = startHP
		want := min(startHP, int32(900))
		for i := 0; i < 10; i++ {
			p.Player.Worn.Set(domain.SlotGlove, domain.Stack{})
			s.refreshStats(p)
			if p.HP != want || p.MaxHP != 900 || p.Player.ClassSetDragonApplied {
				t.Fatalf("remove start%d cycle%d got%d/%d flag%v", startHP, i, p.HP, p.MaxHP, p.Player.ClassSetDragonApplied)
			}
			p.Player.Worn.Set(domain.SlotGlove, st)
			s.refreshStats(p)
			if p.HP != want || p.MaxHP != 1800 || !p.Player.ClassSetDragonApplied {
				t.Fatalf("restore start%d cycle%d got%d/%d flag%v", startHP, i, p.HP, p.MaxHP, p.Player.ClassSetDragonApplied)
			}
		}
	}
}

func TestClassSetDragonAdmissionAndFirstRefresh(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	// A freshly created runtime with worn dragon equipment must not be treated
	// as an ordinary HP equipment delta on its first refresh.
	p.Player.ClassSetDragonApplied = false
	p.HP = 100
	p.MaxHP = 900
	s.refreshStats(p)
	if p.HP != 100 || p.MaxHP != 1800 || !p.Player.ClassSetDragonApplied {
		t.Fatalf("first refresh %d/%d", p.HP, p.MaxHP)
	}
	for _, transfer := range []bool{false, true} {
		next := newCombatScene(t)
		next.items = s.items
		ch := *p.Player.Char
		ch.ID = 200
		ch.Name = "readmitted"
		ch.Pos = domain.Pos{MapID: 7, X: 100, Y: 100}
		cmd := Enter{ID: 2, Char: &ch, Worn: p.Player.Worn.Clone(), Sink: &fakeSink{}}
		if transfer {
			runtime := p.CapturePlayerRuntime(s.tick)
			cmd.Runtime = &runtime
		}
		next.onEnter(cmd)
		q := next.players[2]
		if q == nil || !q.Player.ClassSetDragonApplied {
			t.Fatalf("admission flag missing transfer%v", transfer)
		}
		before := q.HP
		next.refreshStats(q)
		if q.HP != before || q.MaxHP != 1800 {
			t.Fatalf("admission refresh changed HP transfer%v before%d after%d/%d", transfer, before, q.HP, q.MaxHP)
		}
	}
}
