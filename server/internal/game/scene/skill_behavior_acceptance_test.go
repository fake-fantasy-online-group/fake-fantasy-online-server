package scene

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func behaviorAcceptancePlayer(t *testing.T, profession int32) (*Scene, *entity.Entity) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "行为契约验收", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	ch.Race, ch.Level, ch.SkillAllocationVersion = domain.Race(profession-1), 100, 5
	ch.SkillPoints, ch.SkillPointsKnown = 99, true
	ch.Skills = domain.Learned{}
	p.Player.EndgameGrowthV5, p.Player.EndgameSkillBehavior = true, true
	p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
	s.skills, s.statuses = nativeRestoredSceneFixtures(t)
	return s, p
}

func behaviorAcceptanceRows(t *testing.T, s *Scene, p *entity.Entity) []domain.SkillRankView {
	t.Helper()
	r, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]domain.SkillRankView, 0, len(r))
	for _, v := range r {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func TestSkillBehaviorAcceptanceAll149IdentityAndNullHold(t *testing.T) {
	var all []domain.SkillRankView
	names := map[domain.SkillID]string{10014: "破魔", 10024: "维护", 10118: "风斩", 10218: "解除陷阱", 10229: "陷阱回避", 10306: "净化术", 10307: "祝福术", 10327: "神术：鹰眼", 10316: "清身术", 10318: "解咒术", 10321: "防御术"}
	for profession := int32(1); profession <= 5; profession++ {
		s, p := behaviorAcceptancePlayer(t, profession)
		for _, id := range domain.V5SkillIDs() {
			ident, _ := domain.V5SkillIdentity(id)
			if ident.Profession == profession {
				p.Player.Char.Skills[id] = 1
				p.Player.Char.SkillPoints--
			}
		}
		rows := behaviorAcceptanceRows(t, s, p)
		wantCount := 30
		if profession == 4 {
			wantCount = 29
		}
		if len(rows) != wantCount {
			t.Fatal("profession identity count", profession, len(rows), wantCount)
		}
		for _, r := range rows {
			wire, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err = json.Unmarshal(wire, &object); err != nil {
				t.Fatal(err)
			}
			if r.ID == 10212 {
				if string(object["contract"]) != "null" || r.Contract != nil {
					t.Fatal("unresolved theft must be explicit per-row hold", string(wire))
				}
				continue
			}
			c := r.Contract
			first := s.skills[domain.SkillKey{ID: r.ID, Level: 1}]
			ident, _ := domain.V5SkillIdentity(r.ID)
			if c == nil || c.Name != first.Name || c.Icon != first.Icon || c.Profession != profession || c.UnlockLevel != ident.UnlockLevel || !reflect.DeepEqual(c.PrerequisiteSkillIDs, append([]domain.SkillID{}, ident.Prerequisites...)) {
				t.Fatal("authoritative identity mismatch", r.ID, c, first.Name, ident)
			}
			if want, ok := names[r.ID]; ok && c.Name != want {
				t.Fatal("renamed/missing skill not restored", r.ID, c.Name, want)
			}
			if c.Current == nil || c.Next == nil || r.EffectiveRank != "1" || r.NextEffectiveRank != "2" {
				t.Fatal("learned current/next unavailable", r)
			}
			if r.ID == 10316 || r.ID == 10318 || r.ID == 10321 {
				if c.Icon <= 0 || !c.Current.Usable || !c.Current.TargetSelf || !c.Current.TargetTeam {
					t.Fatal("missing healer identity lacks real friendly behavior", r.ID, c)
				}
			}
		}
		encoded, err := json.Marshal(domain.EndgameResponse{Kind: "skill.ranks", OK: true, Revision: "1", SkillRanks: rows, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "formulaVersion": "5", "unspentPoints": fmt.Sprint(p.Player.Char.SkillPoints)}})
		if err != nil || len(encoded) >= 61440 {
			t.Fatal("profession typed packet exceeds strict client bound", profession, len(encoded), err)
		}
		t.Logf("profession%d rows%d wireJSON%d", profession, len(rows), len(encoded))
		if path := os.Getenv("SKILL_BEHAVIOR_FIXTURE_PATH"); path != "" {
			profile := fmt.Sprintf("%s.profession-%d.json", strings.TrimSuffix(path, ".json"), profession)
			if err = os.WriteFile(profile, encoded, 0600); err != nil {
				t.Fatal(err)
			}
		}
		all = append(all, rows...)
	}
	if len(all) != 149 {
		t.Fatal("wrong catalogue", len(all))
	}
	if path := os.Getenv("SKILL_BEHAVIOR_FIXTURE_PATH"); path != "" {
		b, err := json.Marshal(all)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkillBehaviorAcceptanceUnlearnedCurrentNeverBorrowsNext(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	rows := behaviorAcceptanceRows(t, s, p)
	for _, r := range rows {
		if r.CanCast || r.EffectiveRank != "0" || r.Contract == nil || r.Contract.Current != nil || r.Contract.Next == nil {
			t.Fatal("unlearned skill acquired active cost/permission", r)
		}
	}
}

func TestSkillBehaviorAcceptanceHelloGatingAndRenegotiation(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "协商", 100, 100)
	p := s.players[1]
	p.Player.Char.Race, p.Player.Char.Level, p.Player.Char.SkillAllocationVersion = domain.Healer, 100, 5
	p.Player.Char.Skills = domain.Learned{10306: 1}
	s.skills, s.statuses = nativeRestoredSceneFixtures(t)
	s.endgamePlayer(p).loaded = true
	cases := []struct {
		caps             []string
		behavior, growth bool
	}{
		{[]string{domain.EndgameV1}, false, false},
		{[]string{domain.EndgameV1, domain.EndgameGrowthV5}, false, true},
		{[]string{domain.EndgameV1, domain.EndgameSkillBehaviorV1}, false, false},
		{[]string{domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}, false, true},
		{[]string{domain.EndgameV1, domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}, true, true},
		{[]string{domain.EndgameV1, domain.EndgameGrowthV5, domain.EndgameSkillBehaviorV1}, true, true},
		{[]string{domain.EndgameV1, domain.EndgameGrowthV5}, false, true},
	}
	for i, tc := range cases {
		sink.take()
		s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: tc.caps})
		messages := endgameMessages(s, sink)
		hello, ranks := -1, -1
		for index, m := range messages {
			if m.Kind == "hello" {
				hello = index
				seen := false
				for _, c := range m.Capabilities {
					seen = seen || c == domain.EndgameSkillBehaviorV1
				}
				if seen != tc.behavior {
					t.Fatal("wrong hello capability", i, m)
				}
			}
			if m.Kind == "skill.ranks" {
				ranks = index
				b, _ := json.Marshal(m)
				if strings.Contains(string(b), `"contract":`) != tc.behavior {
					t.Fatal("old client sees unknown key or new client lacks key", i, string(b))
				}
			}
		}
		if hello < 0 || tc.growth && ranks <= hello || !tc.growth && ranks >= 0 || p.Player.EndgameSkillBehavior != tc.behavior || p.Player.EndgameGrowthV5 != tc.growth {
			t.Fatal("handshake ordering/state", i, hello, ranks, p.Player.EndgameSkillBehavior)
		}
	}
}

func TestSkillBehaviorAcceptanceTransferNotReconnect(t *testing.T) {
	source, p := behaviorAcceptancePlayer(t, 4)
	snap := source.snapshotOf(p).Clone()
	runtime := p.CapturePlayerRuntime(source.tick)
	target := newTestScene(t, nil)
	transferred := reconnectRiftPlayer(t, target, snap, 2, 0, &runtime, &fakeSink{})
	if !transferred.Player.EndgameSkillBehavior || !transferred.Player.EndgameGrowthV5 {
		t.Fatal("map transfer lost negotiated behavior")
	}
	fresh := newTestScene(t, nil)
	reconnected := reconnectRiftPlayer(t, fresh, snap, 3, 0, nil, &fakeSink{})
	if reconnected.Player.EndgameSkillBehavior || reconnected.Player.EndgameGrowthV5 {
		t.Fatal("new connection inherited capability")
	}
}

func behaviorAcceptanceCast(t *testing.T, s *Scene, p *entity.Entity, id domain.SkillID, target domain.EntityID) *domain.SkillBehaviorPayload {
	t.Helper()
	views, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	if views[id].Contract == nil || views[id].Contract.Current == nil {
		t.Fatal("missing current behavior", id)
	}
	view := views[id].Contract.Current
	before := p.MP
	s.onUseSkill(UseSkill{ID: p.ID, Skill: id, Target: target})
	if len(s.casts) != 1 {
		t.Fatal("typed usable skill not admitted", id, view, p.MP, before)
	}
	def := s.casts[0].def
	if before-p.MP != view.MPCost || def.MPCost != view.MPCost || def.PrepareMS != view.PrepareMS || def.CooldownMS != view.CooldownMS || def.Dist != view.CastDistance {
		t.Fatal("typed view disagrees with accepted cast", id, before-p.MP, view, def.MPCost, def.PrepareMS, def.CooldownMS)
	}
	return view
}

func TestSkillBehaviorAcceptanceMPAndOneShotMatchAcceptedCast(t *testing.T) {
	for _, tc := range []struct {
		id       domain.SkillID
		mp, cost int32
	}{{10306, 60, 48}, {10307, 50, 35}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, p := behaviorAcceptancePlayer(t, 4)
			p.Player.Char.Skills = domain.Learned{tc.id: 1}
			p.MP = tc.mp
			view := behaviorAcceptanceCast(t, s, p, tc.id, p.ID)
			if view.MPCost != tc.cost {
				t.Fatal("polluted resource cost returned", tc.id, view.MPCost, tc.cost)
			}
		})
	}
	t.Run("native_passive_and_instant_cast", func(t *testing.T) {
		s, p := behaviorAcceptancePlayer(t, 5)
		p.Player.Char.Skills = domain.Learned{10401: 3, 10415: 6}
		s.refreshEntityStats(p)
		p.MP = p.MaxMP
		p.Status = domain.NewStatusSet()
		p.Status.Apply(domain.StatusDef{ID: domain.StatusInstantCast, Level: 1, DurationSec: 20, NextCastTimeReductionPct: 60}, s.tick, p.ID)
		monster := putMonster(s, 900, 100000, domain.Stats{}, 110, 100)
		before, _ := json.Marshal(p.Status.SaveForLogout(s.tick, s.now(), p.ID))
		views, err := s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(p.Status.SaveForLogout(s.tick, s.now(), p.ID))
		if !reflect.DeepEqual(before, after) || !p.Status.Has(domain.StatusInstantCast) {
			t.Fatal("preview consumed/mutated one-shot status")
		}
		if views[10401].Contract.Current.PrepareMS != 220 {
			t.Fatal("temporary cast preview not final", views[10401].Contract.Current)
		}
		behaviorAcceptanceCast(t, s, p, 10401, monster.ID)
		if p.Status.Has(domain.StatusInstantCast) {
			t.Fatal("accepted cast failed to consume one-shot")
		}
		views, err = s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		if views[10401].Contract.Current.PrepareMS != 550 {
			t.Fatal("stale one-shot remained in refreshed current", views[10401].Contract.Current)
		}
	})
	for _, kind := range []string{"healing_cost", "spell_economy", "swift_cast"} {
		t.Run("unique_"+kind, func(t *testing.T) {
			s, p, _ := uniqueTestGear(t, kind)
			p.Player.Char.Race = domain.Healer
			p.Player.Char.Level = 100
			p.Player.Char.SkillAllocationVersion = 5
			p.Player.Char.Skills = domain.Learned{10304: 5, 10314: 2, 10301: 10}
			p.Player.EndgameGrowthV5, p.Player.EndgameSkillBehavior = true, true
			s.skills, s.statuses = nativeRestoredSceneFixtures(t)
			behaviorAcceptanceCast(t, s, p, 10304, p.ID)
		})
	}
}

func TestSkillBehaviorAcceptanceHighEquippedRanksAndRemoval(t *testing.T) {
	s, p := behaviorAcceptancePlayer(t, 4)
	p.Player.Char.Skills = domain.Learned{10301: 20, 10306: 20}
	u, ok := domain.UniqueForBase(2032)
	if !ok {
		t.Fatal("equipment fixture missing")
	}
	item := domain.ItemDef{ID: 2032, Equip: &domain.EquipDef{Slot: u.Slot, Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{2032: item}
	st := domain.NewStack(item, 1)
	st.UID = 12345
	st.MaxDurability, st.Durability = 100, 100
	st.Endgame = domain.EndgameItemState{UniqueID: 2032, UniqueRollBP: 10000, UniqueContentVersion: 5}
	p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
	views, err := s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	if views[10301].HardRank != 20 || views[10301].EffectiveRank != "21" || views[10301].NextEffectiveRank != "22" || views[10301].CanInvest {
		t.Fatal("20 hard cap/uncapped equipment identity changed", views[10301])
	}
	before := views[10301].Contract.Current.MPCost
	behaviorAcceptanceCast(t, s, p, 10301, p.ID)
	st.Durability = 0
	p.Player.Worn.Set(domain.EquipSlot(u.Slot), st)
	views, err = s.skillRanksForDisplay(p)
	if err != nil {
		t.Fatal(err)
	}
	if views[10301].EffectiveRank != "20" || views[10301].HardRank != 20 || views[10301].Contract.Current.MPCost >= before {
		t.Fatal("broken source retained effective cost", before, views[10301])
	}
}

func TestSkillBehaviorAcceptanceWorstCaseLegalPacketsAndVariantUpperBound(t *testing.T) {
	skills, statuses := nativeRestoredSceneFixtures(t)
	for profession := int32(1); profession <= 5; profession++ {
		s, p := behaviorAcceptancePlayer(t, profession)
		s.skills, s.statuses, s.items = skills, statuses, v5PacketActualEquipment(t)
		ch := p.Player.Char
		ch.Base = domain.StartingBase(ch.Race)
		for level := int32(2); level <= 100; level++ {
			ch.Base = ch.Base.Add(domain.GrowthAt(ch.Race, level))
		}
		ch.Base.STR += 40
		ch.Base.INT += 40
		ch.Base.SPI += 40
		ch.Base.VIT += 40
		ch.Base.AGI += 20
		ch.Base.DEX += 18
		ch.Appear.Gender = 0
		ch.EmploymentKnown, ch.Employed = true, true
		for _, id := range domain.V5SkillIDs() {
			identity, _ := domain.V5SkillIdentity(id)
			if identity.Profession == profession {
				ch.Skills[id] = 1
			}
		}
		// Spend the finite 99-point budget without altering any prerequisite identity.
		ch.SkillPoints = 99
		for _, rank := range ch.Skills {
			ch.SkillPoints -= rank
		}
		for _, id := range domain.V5SkillIDs() {
			if ch.Skills[id] == 0 {
				continue
			}
			add := min(int32(19), ch.SkillPoints)
			ch.Skills[id] += add
			ch.SkillPoints -= add
			if ch.SkillPoints == 0 {
				break
			}
		}
		if err := domain.ValidateSkillAllocation(100, profession, ch.Skills, ch.SkillPoints); err != nil {
			t.Fatal(err)
		}
		groups := v5PacketGearCandidates(t, s, p)
		maxBytes := 0
		combos := 0
		var largest []byte
		var largestGrant domain.SkillID
		maxRows := 0
		var scan func(int)
		scan = func(index int) {
			if index < len(groups) {
				for _, g := range groups[index] {
					p.Player.Worn.Set(g.slot, g.stack)
					scan(index + 1)
				}
				return
			}
			// The only production cross-profession grant is one SlotTreasure.
			// Enumerating every profession ID is a conservative authorization
			// upper bound, not a claim that 149 such treasure bases are authored.
			treasure := domain.ItemDef{ID: 1999999999, Equip: &domain.EquipDef{Slot: int32(domain.SlotTreasure), Durable: 100}}
			for _, grant := range append([]domain.SkillID{0}, domain.V5SkillIDs()...) {
				if grant != 0 {
					treasure.Equip.Skill = &domain.EquipmentSkill{ID: grant}
					s.items[treasure.ID] = treasure
					st := domain.NewStack(treasure, 1)
					st.UID = 9223372036854770000
					st.Durability = 100
					st.MaxDurability = 100
					p.Player.Worn.Set(domain.SlotTreasure, st)
				} else {
					p.Player.Worn.Set(domain.SlotTreasure, domain.Stack{})
				}
				// Keep all legal numeric loadouts/grants, then conservatively add
				// this profession's one reviewed variant with longest source IDs.
				// This is a payload upper bound, not an assertion that a legal
				// equipped item in each enumerated loadout carries the variant.
				p.Player.EndgameSkillVariants = true
				variantID := map[int32]string{1: "warrior_piercing_lance", 2: "sword_gale_fan", 3: "assassin_bomb_fuse", 4: "healer_guarded_mend", 5: "warlock_flash_fire"}[profession]
				cached := s.equipmentEffects(p)
				kept := cached.effects[:0]
				for _, active := range cached.effects {
					if active.Effect.Kind != "skill_variant" {
						kept = append(kept, active)
					}
				}
				cached.effects = append(kept, domain.ActiveEquipmentEffect{SourceUID: 9223372036854775807, Item: 2147483647, Effect: domain.EquipmentEffect{Kind: "skill_variant", VariantID: variantID}})
				s.equipmentState().cache[p.ID] = cached
				rows := behaviorAcceptanceRows(t, s, p)
				response := domain.EndgameResponse{Kind: "skill.ranks", OK: true, RequestID: strings.Repeat("r", 64), Revision: "9223372036854775807", SkillRanks: rows, Result: map[string]string{"hardCap": "20", "playerCap": "100", "petCap": "80", "formulaVersion": "5", "unspentPoints": "0"}}
				b, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				encoded := protocol.EncodeEndgame(response)
				if len(b) >= 61440 || encoded == nil || !bytes.Contains(encoded, b) || len(rows) > 31 {
					t.Fatal("equipped/granted packet violates strict bounds", profession, grant, len(rows), len(b))
				}
				maxRows = max(maxRows, len(rows))
				combos++
				if len(b) > maxBytes {
					maxBytes = len(b)
					largest = append([]byte(nil), b...)
					largestGrant = grant
				}
			}
			p.Player.Worn.Set(domain.SlotTreasure, domain.Stack{})
		}
		scan(0)
		t.Logf("profession%d combinations%d maxRows%d maxVariantUpperBoundJSON%d grant%d", profession, combos, maxRows, maxBytes, largestGrant)
		if path := os.Getenv("SKILL_BEHAVIOR_FIXTURE_PATH"); path != "" {
			if err := os.WriteFile(fmt.Sprintf("%s.worst-profession-%d.json", strings.TrimSuffix(path, ".json"), profession), largest, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
