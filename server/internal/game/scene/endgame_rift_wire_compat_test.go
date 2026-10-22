package scene

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// Match the unchanged strict client's optional-field contract, not merely the Go
// struct shape. A single short nonempty ability list rejects the whole snapshot.
func assertRiftLegacyAuraWire(t *testing.T, raw []byte) {
	t.Helper()
	var wire struct {
		Rift struct {
			Auras []map[string]json.RawMessage `json:"auras"`
		} `json:"rift"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, a := range wire.Rift.Auras {
		var class int
		json.Unmarshal(a["class"], &class)
		var ids []int32
		if value, present := a["abilityIds"]; present {
			if err := json.Unmarshal(value, &ids); err != nil || len(ids) != class {
				t.Fatalf("strict client rejects class=%d abilityIds=%s", class, value)
			}
		}
		seen := map[int32]bool{}
		for _, id := range ids {
			if id < 0 || id > 7 || seen[id] {
				t.Fatal("invalid/duplicate ID", ids)
			}
			seen[id] = true
		}
		var golden int64
		json.Unmarshal(a["goldenUntilMS"], &golden)
		if golden > 0 && !seen[0] {
			t.Fatal("golden deadline without its ability ID")
		}
	}
}

func TestRiftWireLegacyShortAbilitiesDoNotMutateCombat(t *testing.T) {
	for _, tc := range []struct {
		name    string
		class   int32
		pattern []endgameRiftAbility
		golden  bool
	}{
		{"filtered_blue", 1, nil, false}, {"filtered_gold", 2, []endgameRiftAbility{3}, false},
		{"filtered_guardian", 3, []endgameRiftAbility{3, 6}, false},
		{"short_active_golden", 3, []endgameRiftAbility{0, 3}, true},
		{"complete_active_golden", 1, []endgameRiftAbility{0}, true},
		{"complete_agile", 2, []endgameRiftAbility{3, 5}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, p, now := riftEliteFixture(t, 0)
			s.rift.data.Run.Monsters[m.ID].Class = tc.class
			state := s.rift.abilities[m.ID]
			state.pattern = append([]endgameRiftAbility(nil), tc.pattern...)
			if tc.golden {
				s.stepRiftElite(*now)
				if s.riftGoldenUntil(m.ID, *now) == 0 {
					t.Fatal("golden fixture not active")
				}
			}
			before, _ := json.Marshal(s.rift.data)
			pattern := append([]endgameRiftAbility(nil), state.pattern...)
			view := s.riftView(p, s.rift.data, true, true)
			raw, _ := json.Marshal(domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: view})
			assertRiftLegacyAuraWire(t, raw)
			if len(view.Auras) != 1 || view.Auras[0].Class != tc.class {
				t.Fatal("lost elite identity")
			}
			if len(tc.pattern) < int(tc.class) {
				if len(view.Auras[0].AbilityIDs) != 0 || view.Auras[0].GoldenUntilMS != 0 {
					t.Fatal("short list/dependent metadata leaked")
				}
			} else if len(view.Auras[0].AbilityIDs) != len(tc.pattern) {
				t.Fatal("complete abilities hidden")
			}
			after, _ := json.Marshal(s.rift.data)
			if string(before) != string(after) || !reflect.DeepEqual(pattern, state.pattern) {
				t.Fatal("wire bridge changed gameplay or persisted state")
			}
		})
	}
}

func TestRiftGoldenIsReductionNotInvulnerability(t *testing.T) {
	s, m, _, now := riftEliteFixture(t, 0)
	s.stepRiftElite(*now)
	if !m.Status.Has(riftGoldenStatus) || m.Status.DamageReductionPct(false) != 95 || !m.Status.SuppressesHarmful() {
		t.Fatal("golden semantics not restored")
	}
	if got := s.mitigateSecondaryStatusDamageScaled(m, false, 100, false, 10000); got != 5 {
		t.Fatalf("golden should remain hittable at 5%%, got %d", got)
	}
	s.clearDisabledRiftMonsterStatuses(m)
	if !m.Status.Has(riftGoldenStatus) {
		t.Fatal("true immunity/reflection filter removed 95% golden")
	}
	if s.applyStatusDef(m, domain.StatusDef{ID: 890004, Level: 1, DurationSec: 30, Invulnerable: true}, m.ID) {
		t.Fatal("true invulnerability restored")
	}
	if s.applyStatusDef(m, domain.StatusDef{ID: 890005, Level: 1, DurationSec: 30, PhysicalReflectPct: 100}, m.ID) {
		t.Fatal("reflection restored")
	}
}

func TestRiftWireGeneratedAndExistingRunsKeepProgress(t *testing.T) {
	for _, version := range []int{9, 10, 11} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("version%d_legacy%v", version, legacy), func(t *testing.T) {
				var s *Scene
				if version == 9 {
					s, _ = denseCoverageSceneFixture(t, 80, 98, 1, 1)
				} else if version == 10 {
					s = fullCoverageSceneFixture(t, 88, 88)
				} else {
					s = fullCoverageSceneFixture(t, 224, 224)
				}
				p := s.players[1]
				p.Player.EndgameRiftPages = version != 9
				s.rift.ready[100] = true
				s.populateEndgameRift(false)
				if legacy {
					for id, state := range s.rift.abilities {
						if s.rift.data.Run.Monsters[id].Class == 2 {
							state.pattern = []endgameRiftAbility{3}
							break
						}
					}
				}
				label := fmt.Sprintf("rules%d-count%d-legacy%v", s.rift.data.Run.RulesVersion, len(s.monsters), legacy)
				dump := func(phase string) {
					s.flush()
					sink := p.Player.Sink.(*fakeSink)
					sink.take()
					s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
					s.flush()
					var raw []byte
					var lines []string
					for _, ev := range sink.take() {
						if m, ok := ev.(event.EndgameMessage); ok {
							packet := protocol.EncodeEndgame(m.Message)
							if len(packet) == 0 {
								t.Fatal("empty wire")
							}
							lines = append(lines, hex.EncodeToString(packet))
							if m.Message.Kind == "rift.page" {
								part, err := base64.StdEncoding.DecodeString(m.Message.Data)
								if err != nil {
									t.Fatal(err)
								}
								raw = append(raw, part...)
							} else {
								raw, _ = json.Marshal(m.Message)
							}
						}
					}
					assertRiftLegacyAuraWire(t, raw)
					if dir := os.Getenv("RIFT_COMPAT_FIXTURE_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
						for ext, data := range map[string][]byte{".json": raw, ".wire.hex": []byte(strings.Join(lines, "\n") + "\n")} {
							if err := os.WriteFile(filepath.Join(dir, label+"-"+phase+ext), data, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					t.Logf("%s %s: progress=%d frames=%d bytes=%d", label, phase, s.rift.data.Run.Progress, len(lines), len(raw))
				}
				dump("before")
				killed := 0
				for id, m := range s.monsters {
					if s.rift.data.Run.Monsters[id].Class == 0 {
						m.HP = 0
						s.onDeath(m, p.ID)
						killed++
						if killed == 3 {
							break
						}
					}
				}
				if s.rift.data.Run.Progress != 3*s.rift.data.Run.Rules.NormalProgress {
					t.Fatal("authoritative normal kill progress lost")
				}
				dump("after")
			})
		}
	}
}
