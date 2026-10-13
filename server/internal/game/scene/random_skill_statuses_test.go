package scene

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestRandomStatusSelectOneWeightedAndScoped(t *testing.T) {
	apps := []domain.StatusApplication{
		{ID: 1006, RandomGroup: 1, RandomWeight: 1, TargetType: domain.SkillStatusTargetNonPlayer},
		{ID: 1008, RandomGroup: 1, RandomWeight: 3, TargetType: domain.SkillStatusTargetNonPlayer},
		{ID: 1034, RandomGroup: 2, RandomWeight: 1, TargetType: domain.SkillStatusTargetPlayer},
		{ID: 1014},
	}
	original := append([]domain.StatusApplication(nil), apps...)
	s := newCombatScene(t)
	s.rng = rand.New(rand.NewSource(7200))
	want := rand.New(rand.NewSource(7200))
	counts := map[domain.StatusID]int{}
	for i := 0; i < 1024; i++ {
		selected := s.selectSkillStatuses(apps, domain.KindMonster)
		id := domain.StatusID(1008)
		if want.Int63n(4) == 0 {
			id = 1006
		}
		if len(selected) != 2 || selected[0].ID != id || selected[1].ID != 1014 {
			t.Fatal("weight/scope/ordinary selection", selected)
		}
		counts[id]++
	}
	if counts[1006] < 200 || counts[1006] > 320 {
		t.Fatal("weighted distribution", counts)
	}
	if s.rng.Int63() != want.Int63() {
		t.Fatal("selection made extra random draws")
	}
	selected := s.selectSkillStatuses(apps, domain.KindPlayer)
	if len(selected) != 2 || selected[0].ID != 1034 || selected[1].ID != 1014 {
		t.Fatal(selected)
	}
	selected[0].RandomWeight = 999
	if !reflect.DeepEqual(apps, original) {
		t.Fatal("selection mutated shared definitions")
	}
}

func TestRandomStatusTooltipFallback(t *testing.T) {
	s := newCombatScene(t)
	s.statuses = domain.StatusTable{}
	def := domain.SkillDef{ID: 79991, Level: 1}
	for i, name := range []string{"衰弱", "降幅", "致盲", "定身"} {
		id := domain.StatusID(9701 + i)
		s.statuses[domain.StatusKey{ID: id, Level: 1}] = domain.StatusDef{ID: id, Level: 1, Name: name, DurationSec: 6}
		def.Statuses = append(def.Statuses, domain.StatusApplication{ID: id, Level: 1, RandomGroup: 42, RandomWeight: 1, ChanceBP: 4000, ChanceExplicit: true})
	}
	s.statuses[domain.StatusKey{ID: 9800, Level: 1}] = domain.StatusDef{ID: 9800, Level: 1, Name: "普通增益", DurationSec: 12}
	def.Statuses = append(def.Statuses, domain.StatusApplication{ID: 9800, Level: 1})
	lines := s.skillOriginalEffectLines(def)
	text := strings.Join(lines, "\n")
	if strings.Count(text, "随机施加") != 1 || !strings.Contains(text, "普通增益：持续12秒") {
		t.Fatal("missing single pool or ordinary details", lines)
	}
	for _, name := range []string{"衰弱", "降幅", "致盲", "定身"} {
		if strings.Contains(text, name+"：持续") {
			t.Fatal("fallback presents independent simultaneous states", lines)
		}
	}
}

func TestRandomStatusOnlyEffectDeadTargetDoesNotDraw(t *testing.T) {
	s := newCombatScene(t)
	s.rng = rand.New(rand.NewSource(7242))
	reference := rand.New(rand.NewSource(7242))
	src := putMonster(s, 800, 1000, domain.Stats{}, 100, 100)
	dst := putMonster(s, 801, 0, domain.Stats{}, 120, 100)
	def := domain.SkillDef{ID: 79991, Statuses: []domain.StatusApplication{
		{ID: 9701, Level: 1, RandomGroup: 42, RandomWeight: 1, ChanceBP: 4000, ChanceExplicit: true},
		{ID: 9702, Level: 1, RandomGroup: 42, RandomWeight: 1, ChanceBP: 4000, ChanceExplicit: true},
	}}
	s.applySkill(src, dst, def, 0, false)
	if dst.Status != nil && dst.Status.Count() != 0 || s.rng.Int63() != reference.Int63() {
		t.Fatal("status-only pool drew on an already dead target")
	}
}

func TestRandomStatusAfterSuccessfulResurrection(t *testing.T) {
	s := newCombatScene(t)
	src := putMonster(s, 800, 1000, domain.Stats{}, 100, 100)
	join(t, s, 1, 1, "revive target", 120, 100)
	dst := s.players[1]
	dst.HP = 0
	s.statuses = domain.StatusTable{
		{ID: 9701, Level: 1}: {ID: 9701, Level: 1, Name: "first", Type: domain.StatusGood, TypeKnown: true, DurationSec: 6},
		{ID: 9702, Level: 1}: {ID: 9702, Level: 1, Name: "second", Type: domain.StatusGood, TypeKnown: true, DurationSec: 6},
	}
	def := domain.SkillDef{ID: 79991, Effects: []domain.SkillEffect{{Kind: domain.EffectResurrect, HealPctOfMax: 50}}, Statuses: []domain.StatusApplication{
		{ID: 9701, Level: 1, RandomGroup: 42, RandomWeight: 1, ChanceBP: 10000, ChanceExplicit: true},
		{ID: 9702, Level: 1, RandomGroup: 42, RandomWeight: 1, ChanceBP: 10000, ChanceExplicit: true},
	}}
	if !s.applySkill(src, dst, def, 0, false) || !dst.Alive() || dst.Status.Count() != 1 {
		t.Fatal("successful resurrection no longer receives one configured status")
	}
}
