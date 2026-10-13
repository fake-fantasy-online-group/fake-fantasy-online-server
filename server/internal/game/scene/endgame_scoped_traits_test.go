package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"reflect"
	"strings"
	"testing"
)

func scopedArrayFixture(t *testing.T, id int32) (*Scene, *entity.Entity, domain.Stack, domain.AstralRecipe) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "星阵", 100, 100)
	p := s.players[1]
	r, ok := domain.AstralRecipeByID(id, catalogfixture.EndgameCatalog())
	if !ok {
		t.Fatal(id)
	}
	s.items = map[domain.ItemID]domain.ItemDef{}
	for _, d := range astralRuneItemsFixture(7) {
		s.items[d.ID] = d
	}
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999091, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(r.Slots[0]), LevelReq: 60, Durable: 100, Need: domain.Requirement{Level: 60}}}
	s.items[def.ID] = def
	st := domain.NewStack(def, 1)
	// Scoped old-trait assertions explicitly exercise frozen V4 equipment.
	st.Endgame.FormationContentVersion = 0
	st.SocketCount = uint8(len(r.Runes))
	for i, rune := range r.Runes {
		st.Sockets[i] = rune
		st.SocketUIDs[i] = int64(900010 + i)
		st.SocketCards[i] = s.items[rune].CardDefaults
	}
	r, ok = domain.MatchAstralRecipe(def, st)
	if !ok {
		t.Fatal("legacy formation missing")
	}
	p.Player.Worn = domain.NewEquipSet()
	p.Player.Worn.Set(r.Slots[0], st)
	p.Player.Char.Level = 120
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Employed = true
	for race := domain.Warrior; race <= domain.Warlock; race++ {
		if r.Professions == 0 || r.Professions&(1<<race) != 0 {
			p.Player.Char.Race = race
			break
		}
	}
	p.Player.Char.Base = domain.Base{STR: 100, INT: 100, VIT: 100, SPI: 100, AGI: 100, DEX: 100}
	p.HP, p.MaxHP, p.MP, p.MaxMP = 1000, 1000, 1000, 1000
	if len(s.uniqueTraits(p)) == 0 {
		t.Fatal("array traits not active")
	}
	return s, p, st, r
}
func TestScopedPhysicalSkillManaAtAdmission(t *testing.T) {
	s, p, st, r := scopedArrayFixture(t, 21)
	d := domain.SkillDef{ID: 10021, Level: 1, Prof: 1, Kind: domain.SkillArea, HurtType: domain.HurtPhysical, MPCost: 100, PrepareMS: 1000, TargetEnemy: true, Dist: 500, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	s.skills = skillTable(d)
	p.Player.Char.Skills = map[domain.SkillID]int32{d.ID: 1}
	m := putMonster(s, 901, 1000, domain.Stats{}, 110, 100)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
	if p.MP != 925 || len(s.casts) != 1 || s.casts[0].def.MPCost != 75 {
		t.Fatal("named physical skill did not receive mana reduction at admission", p.MP, len(s.casts))
	}
	original, _ := s.skills.Get(d.ID, 1)
	if original.MPCost != 100 || p.Player.Worn.At(r.Slots[0]) != st {
		t.Fatal("scope mutated shared definition or durable sockets")
	}
	d.ID = 10022
	s.applyUniqueSkillModifiers(p, &d)
	if d.MPCost != 100 {
		t.Fatal("another skill inherited scope")
	}
	p.Player.Worn.Set(r.Slots[0], domain.Stack{})
	d.ID = 10021
	s.applyUniqueSkillModifiers(p, &d)
	if d.MPCost != 100 {
		t.Fatal("unequipped scope remained")
	}
}
func TestScopedStrikeAndBasicAttackIdentity(t *testing.T) {
	s, p, _, _ := scopedArrayFixture(t, 11)
	m := putMonster(s, 900, 1000, domain.Stats{}, 110, 100)
	for _, tc := range []struct {
		id   int32
		want int32
	}{{10002, 112}, {10021, 100}, {0, 100}} {
		effects := domain.EvaluateUniqueTraits(s.uniqueTraits(p), uniqueStrikeContext(p, m, combat.Blow{SkillID: tc.id}, event.DamageDealt{}, m.HP))
		got := (domain.CombatDamageBuckets{DamageIncreaseBP: int64(effects.DamageBP)}).OutgoingDamage(100)
		if got != tc.want {
			t.Fatalf("skill%d damage%d want%d", tc.id, got, tc.want)
		}
	}
}
func TestScopedHealingUsesActualSpellAndAlly(t *testing.T) {
	s, p, _, _ := scopedArrayFixture(t, 25)
	m := putMonster(s, 900, 1000, domain.Stats{}, 110, 100)
	m.HP = 1
	d := domain.SkillDef{ID: 10304, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 100}}
	if got := s.uniqueHealing(p, m, 100, d); got != 122 {
		t.Fatal("group heal scope absent", got)
	}
	s.applySkill(p, m, d, 0, false)
	if m.HP != 123 {
		t.Fatal("scoped bonus missed actual restoration", m.HP)
	}
	d.ID = 10301
	if got := s.uniqueHealing(p, m, 100, d); got != 100 {
		t.Fatal("single heal inherited group scope", got)
	}
	d.ID = 10304
	if got := s.uniqueHealing(p, p, 100, d); got != 100 {
		t.Fatal("self heal inherited ally-only bonus", got)
	}
}
func TestScopedCastAreaAndPlayerDescription(t *testing.T) {
	s, p, st, r := scopedArrayFixture(t, 34)
	original := domain.SkillDef{ID: 10412, Kind: domain.SkillArea, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 100}, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage}}}
	d := original
	s.applyUniqueSkillModifiers(p, &d)
	if d.Area.Radius != 122 || original.Area.Radius != 100 {
		t.Fatal("named area cast did not expand independently", d.Area.Radius)
	}
	d = original
	d.ID = 10402
	s.applyUniqueSkillModifiers(p, &d)
	if d.Area.Radius != 100 {
		t.Fatal("wrong spell radius modified")
	}
	view := s.endgameItem(st, "equipped", int32(r.Slots[0]))
	if !strings.Contains(view.Trait, "暴雪术") || !strings.Contains(view.Trait, "22.00%") || r.Description() != r.Lore {
		t.Fatal("owned scope must use existing effect field and lore must stay separate", view.Trait, r.Description())
	}
	before := st
	p.Player.Worn.Set(r.Slots[0], st)
	again := s.uniqueTraits(p)
	if !reflect.DeepEqual(again, r.Traits) || p.Player.Worn.At(r.Slots[0]) != before {
		t.Fatal("rederived snapshot changed scope/identity")
	}
}

func TestAuthoredAttackSpeedTextMatchesPlayerPercentage(t *testing.T) {
	for _, tc := range []struct {
		value int32
		want  string
	}{{-20, "攻击速度 +20%"}, {15, "攻击速度 -15%"}} {
		line, ok := equipmentAffixLine(domain.Affix{Attr: 25, Mode: 1, Value: tc.value})
		if !ok || line != tc.want {
			t.Fatal("attack speed sign does not match player benefit", line)
		}
	}
}
