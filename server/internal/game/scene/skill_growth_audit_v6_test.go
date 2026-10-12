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
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type skillGrowthAuditRow struct {
	ID            domain.SkillID      `json:"id"`
	Name          string              `json:"name"`
	OriginalAxes  []string            `json:"originalAxes"`
	SaturatedAxes []string            `json:"saturatedAxes"`
	Changes       map[string][]string `json:"changes"`
	Limits        string              `json:"limits"`
	PlateauRanks  []int32             `json:"plateauRanks"`
}

// skillGrowthPayload includes numerical gameplay fields, not progression
// metadata. Fractions are normalized, and control pressure is represented by
// all actually possible integer resistance outcomes rather than its counter.
func skillGrowthPayload(t *testing.T, d domain.SkillDef, states domain.StatusTable) map[string]string {
	t.Helper()
	out := map[string]string{}
	skip := map[string]bool{"Kind": true, "Target": true, "TargetSkill": true, "Status": true, "StatusLevel": true, "RequiredEquipType": true, "EffectKind": true, "ID": true, "Level": true, "Name": true, "Desc": true, "Description": true, "Type": true, "TypeKnown": true, "Icon": true, "SourceSkillID": true, "SkillSnapshotID": true, "SkillSnapshotRank": true, "ProgressionRanks": true, "ProgressionFirstLevel": true, "ProgressionFirstDurationSec": true, "ProgressionBaseDurationSec": true, "ControlPower": true, "FormulaVersion": true, "ManaCostReductionRanks": true, "ProgressionOverflow": true}
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
	for i, r := range d.PassiveResists {
		out[fmt.Sprintf("passiveResists[%d].Pct", i)] = strconv.Itoa(int(domain.ResistChance(r.Pct)))
	}
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
	domain.FinalizeSkillManaCost(&d)
	out["mpCost"] = strconv.Itoa(int(d.MPCost))
	out["cooldownTicks"] = strconv.Itoa(int(domain.Ticks(int(d.CooldownMS))))
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
		if benefitsOnly && (k == "mpCost" || strings.HasSuffix(k, ".mpCost") || strings.HasSuffix(k, ".incomingDamage") || k == "cooldownTicks" || k == "prepareMS" || strings.HasSuffix(k, ".prepareMS")) {
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

// Passive reductions are measured on their authored target actions at rank five.
// This isolates the passive and records actual integer costs, never raw ranks.
func skillGrowthRuntimePayload(t *testing.T, s *Scene, p *entity.Entity, d domain.SkillDef) map[string]string {
	out := skillGrowthPayload(t, d, s.statuses)
	if d.Kind != domain.SkillPassive {
		return out
	}
	p.Player.Char.Race = domain.Race(d.Prof - 1)
	p.Player.Char.Skills = domain.Learned{d.ID: min(d.Level, 20)}
	levels := domain.Learned{d.ID: d.Level}
	s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: levels}}
	for _, id := range domain.V5SkillIDs() {
		target, ok := s.skills.Get(id, 5)
		if !ok || !target.Kind.Active() || target.Prof != d.Prof {
			continue
		}
		s.applyPassiveSkillModifiersWithLevels(p, &target, levels)
		domain.FinalizeSkillManaCost(&target)
		out[fmt.Sprintf("target[%d].mpCost", id)] = strconv.Itoa(int(target.MPCost))
		out[fmt.Sprintf("target[%d].prepareMS", id)] = strconv.Itoa(int(target.PrepareMS))
	}
	for i, m := range d.PassiveModifiers {
		switch m.Kind {
		case domain.PassiveTrapHalfDamageChance, domain.PassiveIncomingMagicHalfChance:
			for _, damage := range []int32{1, 10, 100, 1000000} {
				out[fmt.Sprintf("passiveModifiers[%d].hit[%d].incomingDamage", i, damage)] = strconv.Itoa(int(s.passiveHalfDamage(p, m.Kind, damage)))
			}
		case domain.PassiveAttackDurabilityReductionPct:
			out[fmt.Sprintf("passiveModifiers[%d].equippedBasicAttackPct", i)] = strconv.Itoa(int(100 + m.ProgressionOverflow))
		}
	}
	return out
}

func TestSkillGrowthAuditAll149NativeFixtures(t *testing.T) {
	skills, states := nativeRestoredSceneFixtures(t)
	s := newTestScene(t, nil)
	s.skills, s.statuses = skills, states
	join(t, s, 1, 100, "growth audit", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 100
	if path := os.Getenv("SKILL_GROWTH_ANCHORS_PATH"); path != "" {
		var anchors []map[string]any
		for _, id := range domain.V5SkillIDs() {
			first, _ := skills.Get(id, 1)
			fifth, _ := skills.Get(id, 5)
			anchors = append(anchors, map[string]any{"id": id, "first": first, "fifth": fifth, "firstPayload": skillGrowthPayload(t, first, states), "fifthPayload": skillGrowthPayload(t, fifth, states)})
		}
		data, err := json.MarshalIndent(anchors, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows := []skillGrowthAuditRow{}
	missing := []string{}
	for _, id := range domain.V5SkillIDs() {
		get := func(rank int32) map[string]string {
			d, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("skill %d rank %d missing", id, rank)
			}
			if rank > 5 {
				base, _ := skills.Get(id, 5)
				if d.Area != base.Area || d.Dist != base.Dist || d.Radius != base.Radius || len(d.DirectEffects()) != len(base.DirectEffects()) || len(d.Statuses) != len(base.Statuses) {
					t.Fatalf("skill %d rank%d changed native range or effect/status count", id, rank)
				}
			}
			return skillGrowthRuntimePayload(t, s, p, d)
		}
		base, _ := skills.Get(id, 5)
		row := skillGrowthAuditRow{ID: id, Name: base.Name, OriginalAxes: skillGrowthChanges(get(1), get(5), false), Changes: map[string][]string{}, Limits: "Native-overlay fixture and resolved payload verification; not a per-skill combat execution assertion."}
		for _, rank := range []int32{20, 21, 30, 50, 100} {
			changes := skillGrowthChanges(get(rank), get(rank+1), true)
			row.Changes[strconv.Itoa(int(rank))] = changes
			if len(changes) == 0 {
				missing = append(missing, fmt.Sprintf("%d rank %d", id, rank))
				row.PlateauRanks = append(row.PlateauRanks, rank)
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
	const reviewedPlateaus = "10012 rank 30, 10012 rank 50, 10012 rank 100, 10019 rank 20, 10019 rank 30, 10019 rank 50, 10019 rank 100, 10020 rank 100, 10024 rank 21, 10024 rank 100, 10114 rank 100, 10201 rank 21, 10201 rank 100, 10211 rank 50, 10211 rank 100, 10218 rank 21, 10218 rank 100, 10220 rank 100, 10226 rank 100, 10228 rank 20, 10228 rank 30, 10228 rank 50, 10228 rank 100, 10306 rank 30, 10306 rank 50, 10310 rank 100, 10316 rank 100, 10422 rank 100, 10429 rank 50, 10432 rank 100, 10433 rank 100"
	if got := strings.Join(missing, ", "); got != reviewedPlateaus {
		t.Fatalf("integer/contextual plateau set changed; inspect consumed behavior before updating expectations:\ngot %s\nwant %s", got, reviewedPlateaus)
	}
	t.Logf("149 skills; 745 sampled transitions; %d contextual plateaus: %s", len(missing), reviewedPlateaus)
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
	if changes := skillGrowthChanges(map[string]string{"mpCost": "10", "cooldownTicks": "10", "prepareMS": "10"}, map[string]string{"mpCost": "11", "cooldownTicks": "11", "prepareMS": "11"}, true); len(changes) != 0 {
		t.Fatal("increasing costs counted as benefits", changes)
	}
}

// Full exact source-component boundary regression. Field-level boundaries are
// deliberately not assertions of whole-skill or every-combat-context growth.
func TestSkillGrowthAuditExactNativeBoundaries(t *testing.T) {
	skills, states := nativeRestoredSceneFixtures(t)
	cases := []struct {
		id    domain.SkillID
		path  string
		rank  int32
		value string
	}{
		{10009, "status[0:10009].Affixes[0].Value", 8, "-100"},
		{10010, "status[0:1013].Affixes[1].Value", 5, "-100"},
		{10012, "status[0:10012].PhysicalDamageTakenPct", 28, "100"},
		{10012, "status[0:10012].MagicDamageTakenPct", 28, "100"},
		{10019, "status[0:10019].Affixes[0].Value", 6, "-100"},
		{10103, "status[0:1008].Affixes[0].Value", 15, "-100"},
		{10107, "status[0:1012].PhysicalDamageReductionPct", 31, "100"},
		{10115, "status[0:10115].Affixes[1].Value", 31, "-100"},
		{10124, "status[0:1019].Affixes[1].Value", 31, "-100"},
		{10207, "passiveModifiers[0].Value", 1800, "3600"},
		{10207, "passiveModifiers[1].Value", 20, "100"},
		{10210, "effects[1].ChanceBP", 49, "10000"},
		{10211, "status[0:1018].PoisonOnHitChance", 15, "100"},
		{10217, "effects[2].MaxResourcePct", 8, "100"},
		{10218, "effects[0].ChanceBP", 7, "10000"},
		{10224, "status[0:1008].Affixes[0].Value", 15, "-100"},
		{10224, "status[1:1007].Affixes[0].Value", 15, "-100"},
		{10228, "effects[0].ChanceBP", 8, "10000"},
		{10301, "effects[0].HealPctOfMax", 19, "100"},
		{10304, "effects[0].HealPctOfMax", 20, "100"},
		{10305, "effects[0].HealPctOfMax", 11, "100"},
		{10305, "effects[0].LossRefundPct", 5, "100"},
		{10306, "effects[0].DispelChanceBP", 5, "10000"},
		{10309, "effects[1].HealPctOfMax", 10, "100"},
		{10309, "effects[2].HealPctOfMax", 10, "100"},
		{10309, "effects[2].LossRefundPct", 5, "100"},
		{10310, "status[0:1009].Affixes[0].Value", 8, "-100"},
		{10311, "status[0:1016].Affixes[0].Value", 17, "100"},
		{10315, "status[0:1020].Affixes[0].Value", 39, "-100"},
		{10316, "effects[0].DispelChanceBP", 5, "10000"},
		{10318, "effects[0].DispelChanceBP", 5, "10000"},
		{10321, "status[0:10321].PhysicalDamageReductionPct", 31, "100"},
		{10321, "status[0:10321].MagicDamageReductionPct", 31, "100"},
		{10324, "status[0:1006].Affixes[0].Value", 9, "-100"},
		{10325, "status[0:1009].Affixes[0].Value", 15, "100"},
		{10329, "status[0:1028].NextCastTimeReductionPct", 5, "100"},
		{10330, "status[0:1010].PhysicalReflectPct", 10, "100"},
		{10417, "status[0:1011].MagicFullReflectChancePct", 15, "100"},
		{10420, "status[0:1028].NextCastTimeReductionPct", 5, "100"},
		{10422, "passiveStatusAffixes[0].Affix.Value", 15, "-100"},
		{10425, "status[0:1037].PhysicalDamageReductionPct", 15, "100"},
		{10425, "status[0:1037].PhysicalReflectPct", 20, "100"},
		{10432, "effects[0].ChanceBP", 5, "10000"},
		{10433, "effects[0].DispelChanceBP", 5, "10000"},
		{10435, "status[0:1038].PhysicalDamageReductionPct", 15, "100"},
		{10435, "status[0:1038].FreezeAttackerChance", 15, "100"},
		{10427, "status[0:10427].Affixes[0].Value", 9, "-100"},
		{10427, "status[1:10427].Affixes[0].Value", 9, "-100"},
		{10427, "status[2:10427].Affixes[0].Value", 9, "-100"},
		{10427, "status[3:10427].Affixes[0].Value", 9, "-100"},
		{10016, "status[0:1005].DurationSec", 11, "15"},
		{10016, "status[0:1005].resist[90]", 1792, "0"},
		{10027, "status[0:1002].DurationSec", 10, "15"},
		{10027, "status[0:1002].resist[90]", 1791, "0"},
		{10029, "status[0:1002].DurationSec", 11, "15"},
		{10029, "status[0:1002].resist[90]", 1792, "0"},
		{10030, "status[0:1006].DurationSec", 16, "20"},
		{10030, "status[0:1006].resist[90]", 1797, "0"},
		{10110, "status[0:1002].DurationSec", 10, "15"},
		{10110, "status[0:1002].resist[90]", 1791, "0"},
		{10111, "status[0:10111].DurationSec", 10, "15"},
		{10111, "status[0:10111].resist[90]", 1791, "0"},
		{10113, "status[0:1003].DurationSec", 15, "20"},
		{10113, "status[0:1003].resist[90]", 1796, "0"},
		{10122, "status[0:1029].DurationSec", 51, "60"},
		{10122, "status[0:1029].resist[90]", 1832, "0"},
		{10126, "status[0:1002].DurationSec", 10, "15"},
		{10126, "status[0:1002].resist[90]", 1791, "0"},
		{10128, "status[0:1002].DurationSec", 11, "15"},
		{10128, "status[0:1002].resist[90]", 1792, "0"},
		{10129, "status[0:10129].DurationSec", 16, "20"},
		{10129, "status[0:10129].resist[90]", 1797, "0"},
		{10215, "status[0:1003].DurationSec", 5, "34"},
		{10215, "status[0:1003].resist[90]", 1786, "0"},
		{10216, "status[0:1004].DurationSec", 13, "60"},
		{10216, "status[0:1004].resist[90]", 1794, "0"},
		{10219, "status[0:1002].DurationSec", 11, "15"},
		{10219, "status[0:1002].resist[90]", 1792, "0"},
		{10327, "status[0:10327].DurationSec", 11, "60"},
		{10327, "status[0:10327].resist[90]", 1792, "0"},
		{10413, "status[0:1034].DurationSec", 6, "15"},
		{10413, "status[0:1034].resist[90]", 1787, "0"},
		{10428, "status[0:1004].DurationSec", 9, "60"},
		{10428, "status[0:1004].resist[90]", 1790, "0"},
		{10430, "status[0:1004].DurationSec", 25, "60"},
		{10430, "status[0:1004].resist[90]", 1806, "0"},
		{10018, "status[0:10018].DurationSec", 1799, "3600"},
		{10208, "status[0:1032].DurationSec", 1786, "3600"},
		{10303, "status[0:10303].DurationSec", 233, "3600"},
		{10308, "status[0:1021].DurationSec", 355, "3600"},
		{10320, "status[0:1021].DurationSec", 355, "3600"},
		{10421, "status[0:1036].DurationSec", 9, "60"},
		{10224, "status[0:1008].resist[90]", 1786, "0"},
		{10224, "status[1:1007].resist[90]", 1786, "0"},
		{10008, "passiveResists[0].Pct", 9, "90"},
		{10025, "passiveResists[0].Pct", 9, "90"},
		{10026, "passiveResists[0].Pct", 9, "90"},
		{10108, "passiveResists[0].Pct", 9, "90"},
	}
	for _, c := range cases {
		get := func(rank int32) string {
			d, ok := skills.Get(c.id, rank)
			if !ok {
				t.Fatalf("missing %d/%d", c.id, rank)
			}
			return skillGrowthPayload(t, d, states)[c.path]
		}
		if at, after, before := get(c.rank), get(c.rank+1), get(c.rank-1); at != c.value || after != c.value || before == c.value {
			t.Errorf("%d %s cap%d before=%s at=%s after=%s want%s", c.id, c.path, c.rank, before, at, after, c.value)
		}
	}
	t.Logf("verified %d exact source-component boundaries", len(cases))
}

func TestSkillGrowthAuditAllFunctionalIntegerFloors(t *testing.T) {
	skills, _ := nativeRestoredSceneFixtures(t)
	count := 0
	for _, id := range domain.V5SkillIDs() {
		base, _ := skills.Get(id, 5)
		if !base.Kind.Active() || base.HasDamageEffect() {
			continue
		}
		count++
		for _, field := range []struct {
			name  string
			value int32
		}{{"mana", base.MPCost}, {"cooldown", base.CooldownMS}, {"prepare", base.PrepareMS}} {
			if field.value <= 0 {
				continue
			}
			rank := field.value*50 - 44
			want := int32(0)
			if field.name == "mana" {
				rank--
				want = 1
			}
			if rank <= 5 {
				continue
			}
			get := func(r int32) int32 {
				d, ok := skills.Get(id, r)
				if !ok {
					t.Fatalf("%d/%d missing", id, r)
				}
				domain.FinalizeSkillManaCost(&d)
				switch field.name {
				case "mana":
					return d.MPCost
				case "cooldown":
					return d.CooldownMS
				default:
					return d.PrepareMS
				}
			}
			if before, at, after := get(rank-1), get(rank), get(rank+1); before <= want || at != want || after != want {
				t.Errorf("%d %s floor %d: %d/%d/%d", id, field.name, rank, before, at, after)
			}
		}
	}
	if count != 56 {
		t.Fatalf("functional active count %d, expected56", count)
	}
}

// Kept separate from the domain/payload audit: this documents the known
// unconnected runtime path and must be revisited when that business gap is fixed.
func TestSkillGrowthAuditEagleActualPathGap(t *testing.T) {
	for _, rank := range []int32{5, 11, 100, 1792} {
		s, p := behaviorAcceptancePlayer(t, 4)
		join(t, s, 2, 200, "eagle target", 120, 100)
		target := s.players[2]
		target.HP = 1000
		target.MaxHP = 1000
		inv, _ := s.statuses.Get(1032, 5)
		target.Status.Apply(inv, s.tick, target.ID)
		d, _ := s.skills.Get(10327, rank)
		resolved, _ := s.statuses.ResolveSkillApplication(d.Statuses[0])
		s.revealInvisible(p, d, 300, 0)
		found := false
		target.Status.Each(func(a domain.Active) {
			if a.Def.ID == 10327 {
				found = true
				t.Logf("rank%d resolver duration=%d pressure=%d actual duration=%d pressure=%d", rank, resolved.DurationSec, resolved.ControlPower, a.Def.DurationSec, a.Def.ControlPower)
				if a.Def.DurationSec != 30 || a.Def.ControlPower != 0 {
					t.Fatal("actual behavior changed")
				}
			}
		})
		if !found {
			t.Fatal("eagle state absent")
		}
	}
}
