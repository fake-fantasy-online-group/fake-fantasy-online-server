package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

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
	skip := map[string]bool{"Kind": true, "Target": true, "TargetSkill": true, "Status": true, "StatusLevel": true, "RequiredEquipType": true, "EffectKind": true, "ID": true, "Level": true, "Name": true, "Desc": true, "Description": true, "Type": true, "TypeKnown": true, "Icon": true, "SourceSkillID": true, "SkillSnapshotID": true, "SkillSnapshotRank": true, "ProgressionRanks": true, "ProgressionFirstLevel": true, "ProgressionFirstDurationSec": true, "ProgressionBaseDurationSec": true, "ControlPower": true, "FormulaVersion": true, "ManaCostReductionRanks": true, "ProgressionOverflow": true, "ProgressionRestoreMP": true, "ManaCostReductionApplied": true, "EffectHitBP": true, "SourceEffectHitBP": true, "FreezeAttackerEffectHitBP": true, "SourceSkill": true, "PoisonOnHitProgressionRanks": true, "PoisonOnHitFirstLevel": true, "PoisonOnHitFirstDurationSec": true, "PoisonOnHitBaseDurationSec": true}
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
		delete(out, fmt.Sprintf("passiveResists[%d].Pct", i))
		out[fmt.Sprintf("passiveResists[%d].resistedProbability", i)] = strconv.FormatFloat(1-domain.EffectSuccessProbability(10000, 0, int64(r.Pct)*100), 'g', -1, 64)
	}
	flat("passiveStatusAffixes", reflect.ValueOf(d.PassiveStatusAffixes))
	flat("passiveTriggerEffects", reflect.ValueOf(d.PassiveTriggerEffects))
	flat("passiveCounter", reflect.ValueOf(d.PassiveCounter))
	apps := append([]domain.StatusApplication(nil), d.Statuses...)
	for _, p := range d.PassiveTriggers {
		apps = append(apps, domain.StatusApplication{ID: p.Status, Level: p.StatusLevel, ChanceBP: p.ChanceBP, DurationSec: p.DurationSec, ProgressionRanks: p.ProgressionRanks, ProgressionFirstLevel: p.ProgressionFirstLevel, ProgressionFirstDurationSec: p.ProgressionFirstDurationSec, ProgressionBaseDurationSec: p.ProgressionBaseDurationSec})
	}
	for i, a := range apps {
		a.SourceSkill = d.ID
		resolver := &Scene{statuses: states}
		resolved, ok := resolver.resolveSkillStatusDef(d, a, 0)
		if !ok {
			t.Fatalf("skill %d status %d unresolved", d.ID, a.ID)
		}
		p := fmt.Sprintf("status[%d:%d]", i, a.ID)
		flat(p, reflect.ValueOf(resolved))
		if resolved.Harmful() {
			base := domain.EffectiveStatusChanceBP(a.ChanceBP, a.ChanceExplicit)
			for _, r := range []int32{0, 50, 100, 200, 1000, 10000} {
				out[fmt.Sprintf("%s.success[resist=%d]", p, r)] = strconv.FormatFloat(domain.EffectSuccessProbability(base, int64(resolved.EffectHitBP), int64(r)*100), 'g', -1, 64)
			}
		}
		if resolved.PoisonOnHitProgressionRanks > 0 {
			poison, ok := states.ResolveSkillApplication(domain.StatusApplication{ID: domain.StatusPoison, Level: resolved.PoisonOnHitLevel, ProgressionFirstLevel: resolved.PoisonOnHitFirstLevel, ProgressionRanks: resolved.PoisonOnHitProgressionRanks, ProgressionFirstDurationSec: resolved.PoisonOnHitFirstDurationSec, ProgressionBaseDurationSec: resolved.PoisonOnHitBaseDurationSec})
			if !ok {
				t.Fatal("unresolved poison trigger", d.ID)
			}
			flat(p+".triggeredPoison.HPChanges", reflect.ValueOf(poison.HPChanges))
			flat(p+".triggeredPoison.Affixes", reflect.ValueOf(poison.Affixes))
		}
	}
	for i, f := range effects {
		if d.ID == 10210 && f.Kind == domain.EffectInstantDeath {
			for _, r := range []int64{0, 100, 200, 1000, 10000} {
				out[fmt.Sprintf("effects[%d].success[resist=%d]", i, r)] = strconv.FormatFloat(domain.EffectSuccessProbability(f.ChanceBP, int64(f.EffectHitBP), r*100), 'g', -1, 64)
			}
		}
	}
	flat("area", reflect.ValueOf(d.Area))
	if attr, value := domain.SkillGrowthAttribute(d.ID, d.Level, catalogfixture.SkillProgressionCatalog()); value > 0 {
		out[fmt.Sprintf("character.attribute[%d]", attr)] = strconv.Itoa(int(value))
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
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		target, ok := s.skills.Get(id, 5)
		if !ok || !target.Kind.Active() || target.Prof != d.Prof {
			continue
		}
		s.applyPassiveSkillModifiersWithLevels(p, &target, levels)
		domain.FinalizeSkillManaCost(&target)
		out[fmt.Sprintf("target[%d].mpCost", id)] = strconv.Itoa(int(target.MPCost))
		out[fmt.Sprintf("target[%d].prepareMS", id)] = strconv.Itoa(int(target.PrepareMS))
		for key, value := range skillGrowthPayload(t, target, s.statuses) {
			if strings.HasPrefix(key, "status[") {
				out[fmt.Sprintf("target[%d].%s", id, key)] = value
			}
		}
		for i, effect := range target.DirectEffects() {
			if effect.Kind == domain.EffectDamage {
				n, den := effect.DamagePctNumerator, effect.DamagePctDenominator
				if den <= 0 {
					n, den = int64(effect.DamagePct), 1
				}
				out[fmt.Sprintf("target[%d].effects[%d].damagePct", id, i)] = new(big.Rat).SetFrac(big.NewInt(n), big.NewInt(den)).RatString()
			}
		}
	}
	for i, m := range d.PassiveModifiers {
		switch m.Kind {
		case domain.PassiveTrapHalfDamageChance, domain.PassiveIncomingMagicHalfChance:
			for _, damage := range []int32{1, 10, 100, 1000000} {
				out[fmt.Sprintf("passiveModifiers[%d].hit[%d].incomingDamage", i, damage)] = strconv.Itoa(int(s.passiveHalfDamage(p, m.Kind, damage)))
			}
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
		for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
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
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		get := func(rank int32) map[string]string {
			d, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("skill %d rank %d missing", id, rank)
			}
			if rank > 5 {
				base, _ := skills.Get(id, 5)
				if d.Dist != base.Dist || d.Area.Shape != base.Area.Shape || d.Area.Center != base.Area.Center || d.Area.Angle != base.Area.Angle || len(d.DirectEffects()) != len(base.DirectEffects()) || len(d.Statuses) != len(base.Statuses) {
					t.Fatalf("skill %d rank%d changed native targeting or effect/status count", id, rank)
				}
				if _, expands := redesignAreaCaps[id]; !expands && (d.Area != base.Area || d.Radius != base.Radius) {
					t.Fatalf("skill %d rank%d expanded outside original non-damage area whitelist", id, rank)
				}
			}
			return skillGrowthRuntimePayload(t, s, p, d)
		}
		base, _ := skills.Get(id, 5)
		if changes := skillGrowthChanges(get(5), get(200), true); len(changes) == 0 {
			t.Errorf("skill %d has no consumed-payload growth anywhere between ranks 5 and 200", id)
		}
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
	// Integer grids and finite probabilities can plateau in individual contexts.
	// Record them; do not turn metadata or an unconsumed counter into benefits.
	t.Logf("149 skills; 745 sampled transitions; %d contextual plateaus: %s", len(missing), strings.Join(missing, ", "))
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

// These maps are independently transcribed from the approved design, rather
// than read from the production policy being tested.
var redesignAreaCaps = map[domain.SkillID]int32{
	10028: 5, 10029: 11, 10127: 5, 10128: 11, 10228: 8, 10304: 20,
	10311: 17, 10315: 39, 10321: 23, 10327: 11, 10413: 6,
}
var redesignUtilityCaps = map[domain.SkillID]int32{10018: 5, 10227: 5, 10301: 19, 10305: 11, 10309: 10, 10432: 5}

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
		{10103, "status[0:1008].Affixes[0].Value", 15, "-100"},
		{10217, "effects[2].MaxResourcePct", 8, "100"},
		{10218, "effects[0].ChanceBP", 7, "10000"},
		{10228, "effects[0].ChanceBP", 8, "10000"},
		{10301, "effects[0].HealPctOfMax", 19, "100"},
		{10304, "effects[0].HealPctOfMax", 20, "100"},
		{10305, "effects[0].HealPctOfMax", 11, "100"},
		{10306, "effects[0].DispelChanceBP", 5, "10000"},
		{10311, "status[0:1016].Affixes[0].Value", 17, "100"},
		{10316, "effects[0].DispelChanceBP", 5, "10000"},
		{10318, "effects[0].DispelChanceBP", 5, "10000"},
		{10329, "status[0:1028].NextCastTimeReductionPct", 5, "100"},
		{10330, "status[0:1010].PhysicalReflectPct", 10, "100"},
		{10417, "status[0:1011].MagicFullReflectChancePct", 15, "100"},
		{10420, "status[0:1028].NextCastTimeReductionPct", 5, "100"},
		{10425, "status[0:1037].PhysicalReflectPct", 20, "100"},
		{10432, "effects[0].ChanceBP", 5, "10000"},
		{10433, "effects[0].DispelChanceBP", 5, "10000"},
		{10435, "status[0:1038].FreezeAttackerChance", 15, "100"},
		{10016, "status[0:1005].DurationSec", 11, "15"},
		{10027, "status[0:1002].DurationSec", 10, "15"},
		{10029, "status[0:1002].DurationSec", 11, "15"},
		{10113, "status[0:1003].DurationSec", 15, "20"},
		{10216, "status[0:1004].DurationSec", 13, "60"},
		{10327, "status[0:10327].DurationSec", 11, "60"},
		{10413, "status[0:1034].DurationSec", 6, "15"},
	}
	for _, c := range cases {
		get := func(rank int32) string {
			d, ok := skills.Get(c.id, rank)
			if !ok {
				t.Fatal("missing", c.id, rank)
			}
			return skillGrowthPayload(t, d, states)[c.path]
		}
		if at, after, before := get(c.rank), get(c.rank+1), get(c.rank-1); at != c.value || after != c.value || before == c.value {
			t.Errorf("%d %s cap%d before=%s at=%s after=%s want%s", c.id, c.path, c.rank, before, at, after, c.value)
		}
	}
}

func TestSkillGrowthAuditOnlySixUtilitySavings(t *testing.T) {
	skills, _ := nativeRestoredSceneFixtures(t)
	for _, id := range domain.V5SkillIDs(catalogfixture.SkillProgressionCatalog()) {
		base, _ := skills.Get(id, 5)
		first, _ := skills.Get(id, 1)
		original := func(a, b, rank int32) int32 {
			if b == 0 {
				return 0
			}
			return max(int32(0), b+(b-a)*(rank-5)/4)
		}
		for _, rank := range []int32{6, 10, 20, 21, 50, 100} {
			d, ok := skills.Get(id, rank)
			if !ok {
				t.Fatal("missing", id, rank)
			}
			cap, allowed := redesignUtilityCaps[id]
			want := int32(0)
			if allowed {
				want = max(int32(0), rank-cap)
			}
			if d.ManaCostReductionRanks != want {
				t.Errorf("skill %d/%d savings=%d want=%d", id, rank, d.ManaCostReductionRanks, want)
			}
			if !base.Kind.Active() || base.HasDamageEffect() {
				continue
			}
			wantPrepare := original(first.PrepareMS, base.PrepareMS, rank)
			if d.PrepareMS != wantPrepare {
				t.Errorf("skill %d/%d introduced utility chant savings: %d != %d", id, rank, d.PrepareMS, wantPrepare)
			}
			wantCD := original(first.CooldownMS, base.CooldownMS, rank)
			if want > 0 {
				anchorCD := original(first.CooldownMS, base.CooldownMS, cap)
				wantCD = int32((int64(anchorCD)*50 + 49 + int64(want)) / (50 + int64(want)))
			}
			if d.CooldownMS != wantCD {
				t.Errorf("skill %d/%d cooldown=%d want=%d", id, rank, d.CooldownMS, wantCD)
			}
		}
	}
}

func TestSkillGrowthAuditApprovedAreaTransitions(t *testing.T) {
	skills, _ := nativeRestoredSceneFixtures(t)
	for id, cap := range redesignAreaCaps {
		base, _ := skills.Get(id, 5)
		at, _ := skills.Get(id, cap)
		next, _ := skills.Get(id, cap+1)
		far, _ := skills.Get(id, cap+200)
		if base.HasDamageEffect() || base.Area.Shape == domain.SkillAreaNone {
			t.Fatal("invalid area whitelist", id)
		}
		if at.Area != base.Area {
			t.Errorf("%d expanded before final native cap", id)
		}
		if next.Area.Radius <= at.Area.Radius && next.Area.Length <= at.Area.Length && next.Area.Width <= at.Area.Width {
			t.Errorf("%d no first post-cap spatial benefit", id)
		}
		if far.Area.Radius <= next.Area.Radius && far.Area.Length <= next.Area.Length && far.Area.Width <= next.Area.Width {
			t.Errorf("%d area stopped at short-range boundary", id)
		}
	}
}

func TestSkillGrowthAuditEagleActualPathUsesResolvedGrowth(t *testing.T) {
	for _, rank := range []int32{5, 11, 12, 100} {
		s, p := behaviorAcceptancePlayer(t, 4)
		join(t, s, 2, 200, "eagle target", 120, 100)
		target := s.players[2]
		target.HP, target.MaxHP = 1000, 1000
		inv, _ := s.statuses.Get(1032, 5)
		target.Status.Apply(inv, s.tick, target.ID)
		d, _ := s.skills.Get(10327, rank)
		resolved, _ := s.resolveSkillStatusDef(d, d.Statuses[0], 0)
		s.revealInvisible(p, d, 300, 0)
		found := false
		target.Status.Each(func(a domain.Active) {
			if a.Def.ID == 10327 {
				found = true
				if a.Def.DurationSec != resolved.DurationSec || a.Def.EffectHitBP != resolved.EffectHitBP {
					t.Errorf("rank%d eagle actual/resolved mismatch: duration %d/%d hit %d/%d", rank, a.Def.DurationSec, resolved.DurationSec, a.Def.EffectHitBP, resolved.EffectHitBP)
				}
			}
		})
		if !found {
			t.Fatal("eagle state absent", rank)
		}
	}
}
