package scene

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type skillGrowthAuditRow struct {
	ID            domain.SkillID      `json:"id"`
	Name          string              `json:"name"`
	OriginalAxes  []string            `json:"originalAxes"`
	SaturatedAxes []string            `json:"saturatedAxes"`
	Changes       map[string][]string `json:"changes"`
	Limits        string              `json:"limits"`
}

// skillGrowthPayload includes numerical gameplay fields, not progression
// metadata. Fractions are normalized, and control pressure is represented by
// all actually possible integer resistance outcomes rather than its counter.
func skillGrowthPayload(t *testing.T, d domain.SkillDef, states domain.StatusTable) map[string]string {
	t.Helper()
	out := map[string]string{}
	skip := map[string]bool{"Kind": true, "Target": true, "TargetSkill": true, "Status": true, "StatusLevel": true, "RequiredEquipType": true, "EffectKind": true, "ID": true, "Level": true, "Name": true, "Desc": true, "Description": true, "Type": true, "TypeKnown": true, "Icon": true, "SourceSkillID": true, "SkillSnapshotID": true, "SkillSnapshotRank": true, "ProgressionRanks": true, "ProgressionFirstLevel": true, "ProgressionFirstDurationSec": true, "ProgressionBaseDurationSec": true, "ControlPower": true, "FormulaVersion": true}
	var flat func(string, reflect.Value)
	flat = func(path string, v reflect.Value) {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Name
				if skip[name] || strings.HasSuffix(name, "Numerator") || strings.HasSuffix(name, "Denominator") {
					continue
				}
				flat(path+"."+name, v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				flat(fmt.Sprintf("%s[%d]", path, i), v.Index(i))
			}
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint8:
			if v.CanInt() {
				out[path] = strconv.FormatInt(v.Int(), 10)
			} else {
				out[path] = strconv.FormatUint(v.Uint(), 10)
			}
		}
	}
	ratio := func(path string, value int32, n, den int64) {
		if den <= 0 {
			n, den = int64(value), 1
		}
		out[path] = new(big.Rat).SetFrac(big.NewInt(n), big.NewInt(den)).RatString()
	}
	effects := d.DirectEffects()
	flat("effects", reflect.ValueOf(effects))
	for i, f := range effects {
		p := fmt.Sprintf("effects[%d]", i)
		ratio(p+".DamagePct", f.DamagePct, f.DamagePctNumerator, f.DamagePctDenominator)
		ratio(p+".HealFlat", f.HealFlat, f.HealFlatNumerator, f.HealFlatDenominator)
		ratio(p+".HealPctOfMax", f.HealPctOfMax, f.HealPctNumerator, f.HealPctDenominator)
	}
	flat("passiveModifiers", reflect.ValueOf(d.PassiveModifiers))
	flat("passiveStats", reflect.ValueOf(d.PassiveStats))
	flat("passiveResists", reflect.ValueOf(d.PassiveResists))
	flat("passiveStatusAffixes", reflect.ValueOf(d.PassiveStatusAffixes))
	flat("passiveTriggerEffects", reflect.ValueOf(d.PassiveTriggerEffects))
	flat("passiveCounter", reflect.ValueOf(d.PassiveCounter))
	apps := append([]domain.StatusApplication(nil), d.Statuses...)
	for _, p := range d.PassiveTriggers {
		apps = append(apps, domain.StatusApplication{ID: p.Status, Level: p.StatusLevel, ChanceBP: p.ChanceBP, DurationSec: p.DurationSec, ProgressionRanks: p.ProgressionRanks, ProgressionFirstLevel: p.ProgressionFirstLevel, ProgressionFirstDurationSec: p.ProgressionFirstDurationSec, ProgressionBaseDurationSec: p.ProgressionBaseDurationSec})
	}
	for i, a := range apps {
		s, ok := states.ResolveSkillApplication(a)
		if !ok {
			t.Fatalf("skill %d status %d unresolved", d.ID, a.ID)
		}
		if a.ProgressionRanks == 0 && a.DurationSec > 0 {
			s.DurationSec = a.DurationSec
		}
		s.Affixes = append(s.Affixes, a.ExtraAffixes...)
		s.Affixes = append(s.Affixes, a.ProgressionAffixes...)
		s.StealthDamagePct += a.StealthDamagePct
		s.PhysicalDamageTakenPct += a.PhysicalDamageTakenPct
		p := fmt.Sprintf("status[%d:%d]", i, a.ID)
		flat(p, reflect.ValueOf(s))
		out[p+".ChanceBP"] = strconv.Itoa(int(a.ChanceBP))
		for r := int32(1); r <= 90; r++ {
			out[fmt.Sprintf("%s.resist[%d]", p, r)] = strconv.Itoa(int(domain.SkillStatusResistChance(r, s.ControlPower)))
		}
	}
	for i, p := range d.PassiveTriggers {
		out[fmt.Sprintf("passiveTriggers[%d].ManaEfficiencyRanks", i)] = strconv.Itoa(int(p.ManaEfficiencyRanks))
	}
	if d.ManaEfficiencyRanks > 0 {
		out["averageMana"] = new(big.Rat).SetFrac(big.NewInt(int64(d.MPCost)*50), big.NewInt(50+int64(d.ManaEfficiencyRanks))).RatString()
	} else {
		out["averageMana"] = strconv.Itoa(int(d.MPCost))
	}
	out["cooldownMS"] = strconv.Itoa(int(d.CooldownMS))
	out["prepareMS"] = strconv.Itoa(int(d.PrepareMS))
	return out
}

func skillGrowthChanges(a, b map[string]string, benefitsOnly bool) []string {
	out := []string{}
	for k, v := range b {
		old := a[k]
		if old == "" {
			old = "0"
		}
		if old == v {
			continue
		}
		if benefitsOnly && (k == "averageMana" || k == "cooldownMS" || k == "prepareMS") {
			x, _ := new(big.Rat).SetString(old)
			y, _ := new(big.Rat).SetString(v)
			if x == nil || y == nil || y.Cmp(x) >= 0 {
				continue
			}
		}
		out = append(out, k+": "+old+" → "+v)
	}
	sort.Strings(out)
	return out
}

func TestSkillGrowthAuditAll149NativeFixtures(t *testing.T) {
	skills, states := nativeRestoredSceneFixtures(t)
	rows := []skillGrowthAuditRow{}
	missing := []string{}
	for _, id := range domain.V5SkillIDs() {
		get := func(rank int32) map[string]string {
			d, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("skill %d rank %d missing", id, rank)
			}
			return skillGrowthPayload(t, d, states)
		}
		base, _ := skills.Get(id, 5)
		row := skillGrowthAuditRow{ID: id, Name: base.Name, OriginalAxes: skillGrowthChanges(get(1), get(5), false), Changes: map[string][]string{}, Limits: "Native-overlay fixture and resolved payload verification; not a per-skill combat execution assertion."}
		for _, rank := range []int32{20, 21, 30, 50, 100} {
			changes := skillGrowthChanges(get(rank), get(rank+1), true)
			row.Changes[strconv.Itoa(int(rank))] = changes
			if len(changes) == 0 {
				missing = append(missing, fmt.Sprintf("%d rank %d", id, rank))
			}
		}
		// Report terminal numerical plateaus within the explicit 6..200 window.
		prev := get(5)
		last := map[string]int{}
		for r := int32(6); r <= 200; r++ {
			cur := get(r)
			for k, v := range cur {
				if v != prev[k] {
					last[k] = int(r)
				}
			}
			prev = cur
		}
		for k, r := range last {
			if r < 200 && !strings.Contains(k, ".resist[") {
				row.SaturatedAxes = append(row.SaturatedAxes, fmt.Sprintf("%s: last change rank %d (stable through 200)", k, r))
			}
		}
		sort.Strings(row.SaturatedAxes)
		switch id {
		case 10212:
			row.Limits = "伤害轴已验证；偷窃/战利品合同禁用，偷窃机制未通过。"
		case 10102:
			row.Limits = "伤害轴已验证；原版击退距离证据缺口，击退机制未通过。"
		case 10201:
			row.Limits = "现有被动收益轴已验证；原版背刺眩晕时长证据缺口，眩晕机制未通过。"
		case 10317:
			row.Limits = "伤害轴已验证；原版致盲幅度/时长证据缺口，致盲机制未通过。"
		case 10313:
			row.Limits = "伤害轴已验证；原版随机负面池证据缺口，随机负面机制未通过。"
		case 10326:
			row.Limits = "伤害轴已验证；原版定身概率证据缺口，定身机制未通过。"
		case 10312:
			row.Limits = "伤害轴已验证；亡灵/恶魔分类未建立，种族专属效果未通过。"
		}
		rows = append(rows, row)
	}
	if len(rows) != 149 {
		t.Fatalf("audited %d skills, want149", len(rows))
	}
	if path := os.Getenv("SKILL_GROWTH_AUDIT_PATH"); path != "" {
		b, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		t.Errorf("no meaningful payload delta: %s", strings.Join(missing, ", "))
	}
}

func TestSkillGrowthAuditRejectsMetadataAndEquivalentFractions(t *testing.T) {
	states := domain.StatusTable{{ID: 999, Level: 1}: {ID: 999, Level: 1, DurationSec: 10}}
	first := domain.SkillDef{ID: 10401, Level: 20, Description: "old", Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 50, DamagePctNumerator: 100, DamagePctDenominator: 2}}, Statuses: []domain.StatusApplication{{ID: 999, Level: 1, ProgressionFirstLevel: 1, ProgressionRanks: 15}}}
	next := first
	next.Level, next.Description = 21, "new"
	next.Effects = append([]domain.SkillEffect(nil), first.Effects...)
	next.Effects[0].DamagePctNumerator, next.Effects[0].DamagePctDenominator = 50, 1
	next.Statuses = append([]domain.StatusApplication(nil), first.Statuses...)
	next.Statuses[0].ProgressionRanks++
	if changes := skillGrowthChanges(skillGrowthPayload(t, first, states), skillGrowthPayload(t, next, states), true); len(changes) != 0 {
		t.Fatal("metadata or an equivalent fraction counted as growth", changes)
	}
	if changes := skillGrowthChanges(map[string]string{"averageMana": "10", "cooldownMS": "10", "prepareMS": "10"}, map[string]string{"averageMana": "11", "cooldownMS": "11", "prepareMS": "11"}, true); len(changes) != 0 {
		t.Fatal("increasing costs counted as benefits", changes)
	}
}
