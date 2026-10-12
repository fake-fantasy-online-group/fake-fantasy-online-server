package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

var restoredNativeSkillIDs = []domain.SkillID{10009, 10103, 10109, 10111, 10113, 10122, 10124, 10219, 10221, 10224, 10325}

func nativeRestorationFixtures(t *testing.T) (domain.SkillTable, domain.StatusTable) {
	t.Helper()
	read := func(name string, into any) {
		b, err := os.ReadFile("../../../docs/daoist-endgame/v5/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, into); err != nil {
			t.Fatal(err)
		}
	}
	var rows []domain.SkillDef
	var states []domain.StatusDef
	read("skill-original-ranks.json", &rows)
	read("skill-status-anchors.json", &states)
	if len(rows) != 745 || len(states) != 1890 {
		t.Fatal("original source fixture coverage changed", len(rows), len(states))
	}
	skills, statuses := domain.SkillTable{}, domain.StatusTable{}
	for _, row := range rows {
		if apps := restoredNativeSkillApplications(row.ID, row.Level); apps != nil {
			if len(row.Statuses) != 0 {
				t.Fatalf("expected missing original mapping %d/%d", row.ID, row.Level)
			}
			row.Statuses = apps
		}
		skills[domain.SkillKey{ID: row.ID, Level: row.Level}] = row
	}
	for _, row := range states {
		completeRestoredNativeStatus(&row)
		statuses[domain.StatusKey{ID: row.ID, Level: row.Level}] = row
	}
	if err := installRestoredNativeStatuses(statuses); err != nil {
		t.Fatal(err)
	}
	if err := domain.InstallV5SkillProgression(skills); err != nil {
		t.Fatal(err)
	}
	return skills, statuses
}

func nativeRestorationResolved(t *testing.T, skills domain.SkillTable, statuses domain.StatusTable, id domain.SkillID, rank int32, index int) domain.StatusDef {
	t.Helper()
	skill, ok := skills.Get(id, rank)
	if !ok || index >= len(skill.Statuses) {
		t.Fatalf("missing application %d/%d index%d", id, rank, index)
	}
	app := skill.Statuses[index]
	state, ok := statuses.ResolveSkillApplication(app)
	if !ok {
		t.Fatalf("unresolved application %d/%d: %+v", id, rank, app)
	}
	if app.ProgressionRanks == 0 && app.DurationSec > 0 {
		state.DurationSec = app.DurationSec
	}
	return state
}

func nativeRestorationAffix(d domain.StatusDef, attr int32) int32 {
	for _, a := range d.Affixes {
		if a.Attr == attr && a.Mode == domain.ModePercent {
			return a.Value
		}
	}
	return 0
}

func TestNativeRestorationAllOriginalRanksAndScopedPayloads(t *testing.T) {
	skills, statuses := nativeRestorationFixtures(t)
	assertNativeOriginalRestoration(t, skills, statuses)
}

func assertNativeOriginalRestoration(t *testing.T, skills domain.SkillTable, statuses domain.StatusTable) {
	t.Helper()
	for rank := int32(1); rank <= 5; rank++ {
		for _, id := range restoredNativeSkillIDs {
			state := nativeRestorationResolved(t, skills, statuses, id, rank, 0)
			var duration int32
			switch id {
			case 10009:
				duration = 15
				if got := nativeRestorationAffix(state, domain.AttrDef); got != -(20 + 10*rank) {
					t.Fatal("armor break", rank, got)
				}
			case 10103:
				duration = 15
				if got := nativeRestorationAffix(state, domain.AttrHit); got != -(25 + 5*rank) {
					t.Fatal("accuracy loss", rank, got)
				}
			case 10109, 10111, 10221:
				duration = 10
				if id == 10111 {
					duration = rank + 5
				}
				if id == 10221 {
					duration = 5
				}
				if !state.BlockMove || state.Control != domain.ControlNone || state.CannotAttack || nativeRestorationAffix(state, domain.AttrDef) < 0 {
					t.Fatal("movement-only lock gained unrelated restriction", id, rank, state)
				}
			case 10113:
				duration = rank + 5
				if state.Control != domain.ControlSilence || state.BlockMove {
					t.Fatal("silence", state)
				}
			case 10122:
				duration = rank + 9
				if !state.BlockBeneficial || state.Control != domain.ControlNone {
					t.Fatal("chaos", state)
				}
			case 10124:
				duration = 30
				if nativeRestorationAffix(state, domain.AttrAtk) != -(7+3*rank) || nativeRestorationAffix(state, domain.AttrDef) != 7+3*rank {
					t.Fatal("mountain stance lost cost or defense", rank, state)
				}
			case 10219:
				duration = rank + 4
				if state.Control != domain.ControlStun {
					t.Fatal("stun", state)
				}
			case 10224:
				duration = 60
				magic := nativeRestorationResolved(t, skills, statuses, id, rank, 1)
				if nativeRestorationAffix(state, domain.AttrHit) != -(25+5*rank) || nativeRestorationAffix(magic, domain.AttrMAtk) != -(25+5*rank) || nativeRestorationAffix(magic, domain.AttrMDef) != 0 || len(magic.Affixes) != 1 {
					t.Fatal("smoke changed non-magic-defense payload", rank, state, magic)
				}
			case 10325:
				duration = 30
				if nativeRestorationAffix(state, domain.AttrAtkSpeed) != 25+5*rank {
					t.Fatal("attack slow sign/value", rank, state)
				}
				stats := domain.ApplyStatusStats(domain.Stats{AtkSpeedMS: 1000}, state.Affixes)
				if !state.AttackSpeedIsFrequency || stats.PlayerAttackSpeedPercent() != 100-(25+5*rank) {
					t.Fatal("attack slow accelerated attacks", rank, stats)
				}
				set := domain.NewStatusSet()
				set.Apply(state, 0, 1)
				monster := domain.ApplyMonsterStatusStats(domain.Stats{AtkSpeedMS: 1000}, set)
				speed := 100 - (25 + 5*rank)
				if monster.AtkSpeedMS != (100000+speed-1)/speed {
					t.Fatal("monster shackle used interval instead of frequency", rank, monster)
				}
			}
			if state.DurationSec != duration {
				t.Fatalf("%d/%d duration=%d want%d", id, rank, state.DurationSec, duration)
			}
		}
	}
	for rank := int32(1); rank <= 10; rank++ {
		d, _ := statuses.Get(domain.StatusDampen, rank)
		if nativeRestorationAffix(d, domain.AttrMAtk) != nativeRestorationAffix(d, domain.AttrMDef) {
			t.Fatal("unrelated dampen lost magic defense", rank, d)
		}
	}
}

func TestNativeRestorationContinuesOriginalCurvesAndControls(t *testing.T) {
	skills, statuses := nativeRestorationFixtures(t)
	assertNativeRestorationContinuation(t, skills, statuses)
}

func assertNativeRestorationContinuation(t *testing.T, skills domain.SkillTable, statuses domain.StatusTable) {
	t.Helper()
	for _, rank := range []int32{6, 20, 21, 100, 256, 65536, 1000000} {
		for _, id := range restoredNativeSkillIDs {
			state := nativeRestorationResolved(t, skills, statuses, id, rank, 0)
			if id == 10221 && (state.DurationSec != 5 || state.ControlPower != 0) {
				t.Fatal("fixed sniper duration drifted", rank, state)
			}
			if id == 10109 && state.DurationSec != 10 {
				t.Fatal("self root duration drifted", rank, state)
			}
			if (id == 10111 || id == 10219) && state.DurationSec > 15 {
				t.Fatal("control exceeded existing cap", rank, state)
			}
			if id == 10113 && state.DurationSec > 20 {
				t.Fatal("silence exceeded existing cap", rank, state)
			}
		}
	}
	checks := []struct {
		id         domain.SkillID
		attr, want int32
	}{
		{10009, domain.AttrDef, -80}, {10103, domain.AttrHit, -55},
		{10124, domain.AttrAtk, -25}, {10325, domain.AttrAtkSpeed, 55},
	}
	for _, c := range checks {
		d := nativeRestorationResolved(t, skills, statuses, c.id, 6, 0)
		if got := nativeRestorationAffix(d, c.attr); got != c.want {
			t.Fatal("rank6 original slope", c.id, got, c.want)
		}
	}
	root := nativeRestorationResolved(t, skills, statuses, 10111, 20, 0)
	if root.DurationSec != 15 || root.ControlPower != 10 {
		t.Fatal("root did not inherit resistance pressure", root)
	}
	if domain.SkillStatusResistChance(90, root.ControlPower) >= domain.SkillStatusResistChance(90, 0) {
		t.Fatal("control ranks failed resistance extension")
	}
}

func TestNativeRestorationActualReadOnlyLoader(t *testing.T) {
	var reader Querier
	if path := os.Getenv("FANTASY_V5_SKILL_PG_DATA"); path != "" {
		bin := os.Getenv("PG_BIN")
		if bin == "" {
			t.Fatal("PG_BIN required for explicit single-user validation")
		}
		if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
			t.Fatal("refusing active/stale cluster; use an idle disposable copy")
		}
		db := os.Getenv("FANTASY_V5_SKILL_PG_DATABASE")
		if db == "" {
			db = "fresh_test"
		}
		reader = &v5SingleUserQuerier{bin: bin, path: path, database: db}
	} else {
		reader = testPool(t)
	}
	skills, _, err := LoadSkills(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	statuses, _, err := LoadStatuses(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	assertNativeOriginalRestoration(t, skills, statuses)
	assertNativeRestorationContinuation(t, skills, statuses)
	// The independently consumed scene fixture is derived from immutable source
	// snapshots plus the correction layer. Verify it matches real loader rows.
	fixtureSkills, fixtureStatuses := nativeRestorationFixtures(t)
	for _, id := range restoredNativeSkillIDs {
		for rank := int32(1); rank <= 5; rank++ {
			key := domain.SkillKey{ID: id, Level: rank}
			if !reflect.DeepEqual(skills[key], fixtureSkills[key]) {
				t.Fatalf("actual restored skill differs from correction fixture %d/%d\ngot%+v\nwant%+v", id, rank, skills[key], fixtureSkills[key])
			}
			for _, app := range skills[key].Statuses {
				sk := domain.StatusKey{ID: app.ID, Level: app.Level}
				actual, historical := statuses[sk], fixtureStatuses[sk]
				// The immutable gameplay fixture predates the recovery of the
				// original per-level SmallMap. Compare all mechanics unchanged;
				// native artwork is independently checked against the original
				// binary by TestNativeStatusIconsActualReadOnlyLoader.
				if actual.IconKnown && !historical.IconKnown {
					actual.Icon, actual.IconKnown = historical.Icon, false
				}
				if !reflect.DeepEqual(actual, historical) {
					t.Fatalf("actual restored status differs from correction fixture %v\ngot%+v\nwant%+v", sk, statuses[sk], fixtureStatuses[sk])
				}
			}
		}
	}
	t.Log("PASS actual read-only LoadSkills/LoadStatuses: all 11 restored skills, 55 original ranks, high-rank continuation and independent acceptance-fixture parity")
}

func TestNativeRestorationCollisionIsAtomic(t *testing.T) {
	for _, id := range []domain.StatusID{nativeArmorBreak, nativeMoveLock} {
		original := domain.StatusDef{ID: id, Level: 99, Name: "future source"}
		key := domain.StatusKey{ID: id, Level: original.Level}
		table := domain.StatusTable{key: original}
		if err := installRestoredNativeStatuses(table); err == nil || len(table) != 1 || !reflect.DeepEqual(table[key], original) {
			t.Fatal("collision silently replaced source", err, table)
		}
	}
}

// Optional test artifact for independent scene acceptance. This is explicitly
// the original snapshot PLUS the verified restoration layer, never a rewritten
// original-source anchor. The production loader is checked separately with PG.
func TestNativeRestorationExportAcceptanceFixture(t *testing.T) {
	path := os.Getenv("FANTASY_NATIVE_RESTORATION_FIXTURE")
	if path == "" {
		t.Skip("optional acceptance fixture export")
	}
	skills, statuses := nativeRestorationFixtures(t)
	var skillRows []domain.SkillDef
	var statusRows []domain.StatusDef
	used := map[domain.StatusKey]bool{}
	for _, id := range restoredNativeSkillIDs {
		for rank := int32(1); rank <= 5; rank++ {
			d := skills[domain.SkillKey{ID: id, Level: rank}]
			skillRows = append(skillRows, d)
			for _, a := range d.Statuses {
				used[domain.StatusKey{ID: a.ID, Level: a.Level}] = true
			}
		}
	}
	for key := range used {
		statusRows = append(statusRows, statuses[key])
	}
	sort.Slice(statusRows, func(i, j int) bool {
		if statusRows[i].ID != statusRows[j].ID {
			return statusRows[i].ID < statusRows[j].ID
		}
		return statusRows[i].Level < statusRows[j].Level
	})
	payload := struct {
		Source   string             `json:"source"`
		Skills   []domain.SkillDef  `json:"skills"`
		Statuses []domain.StatusDef `json:"statuses"`
	}{
		"Immutable original LoadSkills/LoadStatuses snapshots plus native_skill_restoration.go; not a new original-source export", skillRows, statusRows,
	}
	full, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(full, &decoded); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(v5OmitZeroFields(decoded), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
