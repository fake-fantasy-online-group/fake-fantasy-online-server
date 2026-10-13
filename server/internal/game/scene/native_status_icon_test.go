package scene

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestOriginalStatusIconBeatsSkillItemAndSavedArt(t *testing.T) {
	for _, tc := range []struct {
		id          domain.StatusID
		level, icon int32
		name        string
	}{
		{1001, 1, 5030, "中毒"}, {1018, 1, 7037, "使毒"}, {1004, 1, 7087, "冰冻"},
		{1176, 2, 0, "长白参"}, {1036, 7, 10179, "毒牙"},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.id, tc.level), func(t *testing.T) {
			canonical := domain.StatusDef{ID: tc.id, Level: tc.level, Name: tc.name, Icon: tc.icon, IconKnown: true, DurationSec: 10}
			for _, skill := range []int32{0, 10211, 10435} { // actual item and skill source helper
				def := canonical
				applyStatusSource(&def, 9999, skill)
				if def.Icon != tc.icon || def.SourceSkillID != 0 {
					t.Fatalf("authored icon replaced: %+v", def)
				}
			}
			s := newCombatScene(t)
			join(t, s, 1, 100, "target", 100, 100)
			p := s.players[1]
			s.statuses = statusTable(canonical)
			// Restore an old persisted state containing the former source skill artwork.
			old := canonical
			old.Icon = 9999
			old.IconKnown = false
			old.SourceSkillID = 10211
			p.Status = domain.RestoreSavedStatuses([]domain.SavedStatus{{Def: old, RemainingTicks: 100, SourceSelf: true, ExpiresAtUnixMS: time.Now().Add(time.Minute).UnixMilli()}}, s.tick, time.Now(), p.ID)
			rows := s.visibleStatuses(p)
			if len(rows) != 1 {
				t.Fatalf("missing restored state: %+v", rows)
			}
			if rows[0].buff.Icon != tc.icon || rows[0].icon.Icon != tc.icon || rows[0].buff.SkillID != int32(tc.id) {
				t.Fatalf("stale saved icon leaked: %+v", rows[0])
			}
			p.Status.Each(func(a domain.Active) {
				if a.Def.Icon != 9999 || a.Def.DurationSec != 10 {
					t.Fatal("presentation mutated saved mechanics")
				}
			})
		})
	}
}

// Feed actual LoadMonsters/LoadStatuses output through the live cast settlement,
// source application, both snapshots and actual protocol encoder. No positive
// poison icon is fabricated in this integration fixture.
func TestOriginalPoisonIconsLoadedCombat(t *testing.T) {
	path := os.Getenv("FANTASY_STATUS_ICON_RUNTIME_FIXTURE")
	if path == "" {
		t.Skip("set runtime fixture exported by TestNativeStatusIconsActualReadOnlyLoader")
	}
	var fixture struct {
		Source   string
		Statuses []domain.StatusDef
		Sources  []struct {
			MonsterID   domain.MonsterID
			MonsterName string
			Rule        domain.MonsterSkillRule
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source == "" || len(fixture.Sources) < 9 {
		t.Fatal("incomplete actual loader fixture")
	}
	table := domain.StatusTable{}
	for _, d := range fixture.Statuses {
		table[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	packets := 0
	for _, source := range fixture.Sources {
		rule := source.Rule
		t.Run(fmt.Sprintf("monster%d_skill%d_%d", source.MonsterID, rule.Skill.ID, rule.Skill.Level), func(t *testing.T) {
			s := newCombatScene(t)
			sink := join(t, s, 1, 100, "target", 100, 100)
			p := s.players[1]
			s.statuses = table
			m := putMonster(s, 900, 1000000, sureHit, 110, 100)
			m.Monster.TypeID = source.MonsterID
			m.Name = source.MonsterName
			sink.take()
			// Preserve original effect probabilities and random-status lists; repeat casts
			// until poison is selected, rather than replacing them with a forced status.
			for attempt := 0; attempt < 500 && !p.Status.Has(domain.StatusPoison); attempt++ {
				p.HP = 1000000000
				p.MaxHP = p.HP
				p.Status = domain.NewStatusSet()
				s.casts = append(s.casts, pendingCast{src: m.ID, def: rule.Skill, targets: []domain.EntityID{p.ID}, extras: rule.Extra, released: true})
				s.stepCasts()
			}
			if !p.Status.Has(domain.StatusPoison) {
				t.Fatalf("actual monster rule did not poison after settlement: %+v", rule)
			}
			snapshot := s.buffSnapshot(p)
			found := false
			for _, row := range snapshot.Buffs {
				if row.Name == "中毒" {
					found = true
					if row.SkillID != 1001 || row.Icon != 5030 {
						t.Fatalf("wrong original poison identity/art %+v", row)
					}
				}
			}
			if !found {
				t.Fatal("poison absent from actual self snapshot")
			}
			entityFound := false
			for _, row := range s.visibleStatuses(p) {
				if row.icon.Status == 1001 {
					entityFound = true
					if row.icon.Icon != 5030 {
						t.Fatal("wrong entity poison icon")
					}
				}
			}
			if !entityFound {
				t.Fatal("missing entity poison")
			}
			s.flush()
			emitted := false
			for _, ev := range sink.take() {
				if snap, ok := ev.(event.BuffSnapshot); ok {
					for _, r := range snap.Buffs {
						if r.Name == "中毒" && r.Icon == 5030 {
							emitted = true
						}
					}
				}
			}
			if !emitted {
				t.Fatal("live event publication omitted poison icon")
			}
			wire := protocol.Encode(p.ID, snapshot)
			if len(wire) != 1 || binary.LittleEndian.Uint16(wire[0]) != protocol.SCBuffSnapshot {
				t.Fatal("wrong production wire")
			}
			if rule.Skill.ID == 12001 && rule.Skill.Level == 1 {
				if dir := os.Getenv("FANTASY_STATUS_ICON_WIRE_DIR"); dir != "" {
					if err = os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(dir, "monster-poison-buff.bin"), wire[0][2:], 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			packets++
		})
	}
	t.Logf("PASS %d actual loaded monster skill-level rules through cast settlement, event publication and protocol encoder", packets)
	// Player basic/counter attacks retain original poison art instead of carrying
	// the visually different native 7037 '使毒' icon onto 1001 '中毒'.
	for _, origin := range []domain.DamageOrigin{domain.DamageNormalOrigin, domain.DamageCounterOrigin} {
		s := newCombatScene(t)
		join(t, s, 1, 100, "source", 100, 100)
		join(t, s, 2, 200, "target", 110, 100)
		a, b := s.players[1], s.players[2]
		s.statuses = table
		carrier, ok := table.Get(domain.StatusPoisonHit, 1)
		if !ok || carrier.Icon != 7037 {
			t.Fatal("original poison carrier not loaded")
		}
		a.Status.Apply(carrier, s.tick, a.ID)
		for n := 0; n < 500 && !b.Status.Has(domain.StatusPoison); n++ {
			s.afterStrikeStatuses(a, b, event.DamageDealt{Src: a.ID, Dst: b.ID, Amount: 1, Origin: origin}, 1, 0)
		}
		if !b.Status.Has(domain.StatusPoison) {
			t.Fatal("original poison on hit did not proc")
		}
		for _, row := range s.visibleStatuses(b) {
			if row.icon.Status == 1001 && row.icon.Icon != 5030 {
				t.Fatal("carrier icon replaced original poison art")
			}
		}
	}
}
