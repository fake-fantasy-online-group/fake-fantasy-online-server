package scene

// End-to-end combat measurements: commands, admission, MP, cast/windup,
// cooldowns, strikes, HP loss and death all use the production scene paths.
// No attribute sum is used as a damage proxy.
import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type rebalanceV6Input struct {
	items          map[domain.ItemID]domain.ItemDef
	skills         domain.SkillTable
	statuses       domain.StatusTable
	pools          []rebalanceV6Pool
	upgrades       map[domain.ItemID]rebalanceV6Upgrade
	common         map[domain.Race]*domain.EquipSet
	card           domain.ItemID
	sockets        int
	refine         bool
	fullHealth     bool
	pressureDamage int32
}
type rebalanceV6Pool struct {
	Tier, Type, Attr, Mode, Max int32
	Sign                        int32 `json:"value_sign"`
}
type rebalanceV6Case struct {
	id       domain.ItemID
	race     domain.Race
	skill    domain.SkillID
	pressure bool
}
type rebalanceV6Result struct {
	damage, burst, skillDamage, mpSpent int64
	casts, hits, misses, rejected       int
	lowTicks, aliveTicks                int
	endHP, endMP                        int32
}

func rebalanceV6Inputs(t *testing.T) rebalanceV6Input {
	in := rebalanceV6Input{items: v5PacketActualEquipment(t)}
	in.skills, in.statuses = v5PacketActualSkills(t)
	v5PacketJSON(t, "balance/yellow_affix_benchmarks.json", &in.pools)
	b, err := os.ReadFile("testdata/equipment_rebalance_v6_upgrades.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &in.upgrades); err != nil {
		t.Fatal(err)
	}
	for id, u := range in.upgrades {
		d := in.items[id]
		e := d.Equip
		e.Position, e.Tier, e.RefineLimit = u.Position, u.Tier, u.RefineLevel
		e.Durable = u.Durable
		e.AttackDurabilityCostRaw = u.AttackWear
		e.BeHitDurabilityCostRaw = u.HitWear
		e.DeathDurabilityCostRaw = u.DeathWear
		bonuses, ok := domain.ResolveRefineBonuses(e, u.RawRefine)
		if !ok {
			t.Fatal("invalid source refine", id)
		}
		e.RefineEffects = map[int32]domain.RefineEffect{u.RefineLevel: {Bonuses: bonuses}}
	}
	var cards struct {
		Cards []struct {
			ID      domain.ItemID `json:"card_id"`
			Name    string
			Effects []domain.CardEffect
		}
	}
	v5PacketJSON(t, "balance/native_card_inventory.json", &cards)
	for _, c := range cards.Cards {
		d := domain.ItemDef{ID: c.ID, Name: c.Name, InstanceKind: domain.ItemInstanceSocketCard}
		d.CardDefaults.Initialized = true
		d.CardDefaults.Count = uint8(len(c.Effects))
		copy(d.CardDefaults.Effects[:], c.Effects)
		in.items[c.ID] = d
	}
	return in
}
func rebalanceV6Base(t *testing.T, d domain.ItemDef, race domain.Race) domain.Base {
	b := domain.StartingBase(race)
	for n := int32(2); n <= 54; n++ {
		b = b.Add(domain.GrowthAt(race, n))
	}
	points := int32(domain.FreePointsPerLevel * 53)
	req := d.Equip.Need.Base
	for _, v := range []struct {
		p *int32
		n int32
	}{{&b.STR, req.STR}, {&b.INT, req.INT}, {&b.VIT, req.VIT}, {&b.SPI, req.SPI}, {&b.AGI, req.AGI}, {&b.DEX, req.DEX}} {
		delta := max(int32(0), v.n-*v.p)
		*v.p += delta
		points -= delta
	}
	if points < 0 {
		t.Fatal("illegal stat allocation", d.ID)
	}
	if race >= domain.Healer {
		b.INT += points
	} else {
		b.STR += points
	}
	return b
}
func rebalanceV6Stack(d domain.ItemDef, affixes []domain.Affix, roll int32) domain.Stack {
	st := domain.NewStack(d, 1)
	st.UID = 700000 + int64(d.ID)
	for i, a := range affixes {
		st.RolledAffixes[i] = domain.InstanceAffix{Affix: a}
	}
	st.RolledAffixCount = uint8(len(affixes))
	if roll > 0 {
		st.Endgame = domain.EndgameItemState{UniqueID: int32(d.ID), UniqueRollBP: roll, UniqueContentVersion: domain.UniqueContentVersion}
	}
	return st
}
func rebalanceV6Scene(t *testing.T, in rebalanceV6Input, tc rebalanceV6Case, st domain.Stack, seed int64) (*Scene, *entity.Entity, *entity.Entity, *fakeSink, *time.Time) {
	s := newTestScene(t, nil)
	s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	sink := join(t, s, 1, 100, "V6真实战斗", 100, 100)
	p := s.players[1]
	ch := p.Player.Char
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	// Exact original SQL game_combat_timing_rule (000000_init-0.sql:4680).
	s.combatTiming = domain.CombatTimingRule{MeleeHitDelayMS: 350, RangedBasicSpeedPXPerSec: 800, CastInterruptBaseBP: 4000, CastInterruptMinBP: 500}
	s.rng = rand.New(rand.NewSource(seed))
	s.items, s.skills, s.statuses = in.items, in.skills, in.statuses
	st = rebalanceV6Invest(t, in, st)
	ch.Race, ch.Level, ch.SkillAllocationVersion = tc.race, 54, domain.SkillFormulaVersion
	ch.EmploymentKnown, ch.Employed, ch.SkillPointsKnown = true, true, true
	ch.Base = rebalanceV6Base(t, in.items[tc.id], tc.race)
	ch.Skills = domain.Learned{}
	var learn func(domain.SkillID)
	learn = func(id domain.SkillID) {
		if ch.Skills[id] > 0 {
			return
		}
		ident, _ := domain.V5SkillIdentity(id)
		for _, pre := range ident.Prerequisites {
			learn(pre)
		}
		ch.Skills[id] = 1
	}
	if tc.skill != 0 {
		learn(tc.skill)
		ch.Skills[tc.skill] = 20
	}
	ch.SkillPoints = 53
	for _, r := range ch.Skills {
		ch.SkillPoints -= r
	}
	if err := domain.ValidateSkillAllocation(54, int32(tc.race)+1, ch.Skills, ch.SkillPoints); err != nil {
		t.Fatal(err)
	}
	p.Player.EndgameGrowthV5, p.Player.EndgameSkillBehavior, p.Player.EndgameSkillVariants = true, true, true
	p.Player.Worn = domain.NewEquipSet()
	if in.common != nil {
		p.Player.Worn = in.common[tc.race].Clone()
	}
	p.Player.Worn.Set(domain.EquipSlot(in.items[tc.id].Equip.Slot), st)
	s.refreshStats(p)
	if s.functionalWorn(p).Count() != p.Player.Worn.Count() {
		t.Fatal("inactive equipment", tc.id, ch.Base)
	}
	p.HP = p.MaxHP
	// Equal initial MP, no potions and no invented natural regeneration. This
	// budget is independently derived from the naked character for both arms.
	p.MP = min(p.MaxMP, domain.DeriveFromBase(ch.Base).MaxMP)
	if tc.pressure && !in.fullHealth {
		p.HP = max(int32(1), p.MaxHP*28/100)
	}
	m := putMonster(s, 900, 1000000000, domain.Stats{Def: 80, MDef: 60}, 120, 100)
	m.Level = 54
	if tc.id == 1810 {
		m.Monster.Kind = domain.MonsterElite
		m.Status = domain.NewStatusSet()
		m.Status.Apply(domain.StatusDef{ID: 999998, DurationSec: 120, BlockHarmful: true}, 0, m.ID)
	}
	sink.take()
	return s, p, m, sink, &now
}
func rebalanceV6Run(t *testing.T, in rebalanceV6Input, tc rebalanceV6Case, st domain.Stack, seed int64) rebalanceV6Result {
	s, p, m, sink, now := rebalanceV6Scene(t, in, tc, st, seed)
	out := rebalanceV6Result{}
	nextSkillTry := 0
	for n := 0; n < 600; n++ {
		end := now.Add(100 * time.Millisecond)
		for resolved := 0; ; resolved++ {
			at, ok := s.nextTimedCombatAt()
			if !ok || at.After(end) {
				break
			}
			if resolved >= 100 {
				t.Fatal("unbounded timed event chain")
			}
			if at.Before(*now) {
				at = *now
			}
			*now = at
			if !s.hitFrame(at) {
				t.Fatal("timed phase panicked")
			}
		}
		*now = end
		s.tick++
		s.stepStatus()
		s.stepEquipmentAuras()
		s.stepAstralRuneRegeneration()
		if p.Alive() {
			out.aliveTicks++
			if p.HP*100 <= p.MaxHP*30 {
				out.lowTicks++
			}
		}
		// Incoming blows every two seconds, plus three seconds of movement in
		// each ten-second cycle. Neither side receives post-death attacks.
		moving := tc.pressure && n%100 >= 70
		if tc.pressure && n%20 == 19 && p.Alive() {
			damage := in.pressureDamage
			if damage == 0 {
				damage = 30
			}
			m.Stats.MinAtk, m.Stats.MaxAtk, m.Stats.Hit = damage, damage, 10000
			s.onAttack(Attack{ID: m.ID, Target: p.ID, Blow: combat.Blow{GuaranteedHit: true}})
		}
		before := p.MP
		if !moving && p.Alive() {
			if tc.skill == 0 {
				s.onAttack(Attack{ID: p.ID, Target: m.ID})
			} else {
				if n >= nextSkillTry && p.Player.SkillReadyAt(tc.skill, s.tick) && !s.playerPreparingSkill(p.ID) {
					s.onUseSkill(UseSkill{ID: p.ID, Target: m.ID, Skill: tc.skill})
					if p.MP == before {
						nextSkillTry = n + 10
					}
				}
				if tc.race == domain.Assassin {
					s.onAttack(Attack{ID: p.ID, Target: m.ID})
				}
			}
		}
		out.mpSpent += int64(max(int32(0), before-p.MP))
		s.stepCasts()
		s.stepCombat()
		s.flush()
		for _, e := range sink.take() {
			switch e := e.(type) {
			case event.DamageDealt:
				if e.Src == p.ID && e.Dst == m.ID {
					if e.Flag.Has(event.DamageMiss) {
						out.misses++
					} else {
						out.hits++
						out.damage += int64(e.Amount)
						if e.SkillID == int32(tc.skill) && tc.skill != 0 {
							out.skillDamage += int64(e.Amount)
						}
						if n < 60 {
							out.burst += int64(e.Amount)
						}
					}
				}
			case event.SkillCastChanged:
				if e.Caster == p.ID && e.Phase == event.SkillCastChant {
					out.casts++
				}
			case event.Rejected:
				out.rejected++
			}
		}
		if tc.id == 1810 && m.Status != nil && !canAct(m) {
			t.Fatal("hard-control immune elite was controlled")
		}
	}
	out.endHP, out.endMP = p.HP, p.MP
	if out.damage != int64(m.MaxHP-m.HP) {
		t.Fatalf("event damage disagrees with actual HP loss: %d vs %d", out.damage, m.MaxHP-m.HP)
	}
	return out
}

// Enumerate all beneficial native top-roll combinations with distinct attribute
// families and the real five-natural-line quota (fixed lines consume it).
func rebalanceV6Yellows(in rebalanceV6Input, tc rebalanceV6Case) [][]domain.Affix {
	d := in.items[tc.id]
	by := map[int32][]domain.Affix{}
	relevant := map[int32]bool{domain.AttrSTR: true, domain.AttrHit: true, domain.AttrDEX: true, domain.AttrAtkSpeed: true, domain.AttrMinAtk: true, domain.AttrMaxAtk: true, domain.AttrAtk: true}
	if tc.race >= domain.Healer {
		relevant = map[int32]bool{domain.AttrINT: true, domain.AttrMAtk: true}
	}
	for _, r := range in.pools {
		if r.Tier == d.Equip.ResourceLevel && r.Type == d.Equip.Type && relevant[r.Attr] {
			by[r.Attr] = append(by[r.Attr], domain.Affix{Attr: r.Attr, Mode: r.Mode, Value: r.Max * r.Sign})
		}
	}
	ids := []int{}
	for id := range by {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	var out [][]domain.Affix
	var visit func(int, []domain.Affix)
	visit = func(i int, a []domain.Affix) {
		if i == len(ids) {
			if len(a) != min(len(ids), 5-len(d.Equip.Affixes)-in.sockets) {
				return
			}
			out = append(out, append([]domain.Affix(nil), a...))
			return
		}
		visit(i+1, a)
		if len(a) < 5-len(d.Equip.Affixes)-in.sockets {
			for _, v := range by[int32(ids[i])] {
				visit(i+1, append(a, v))
			}
		}
	}
	visit(0, nil)
	return out
}
func TestEquipmentRebalanceV6SingleTarget60Seconds(t *testing.T) {
	in := rebalanceV6Inputs(t)
	for _, tc := range []rebalanceV6Case{{1510, domain.Swordsman, 0, false}, {1310, domain.Warlock, 10401, false}, {1610, domain.Warrior, 0, true}, {1810, domain.Assassin, 10219, false}} {
		tc := tc
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			d := in.items[tc.id]
			var best []domain.Affix
			var score int64
			for _, a := range rebalanceV6Yellows(in, tc) {
				r := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, a, 0), 17)
				if r.damage > score {
					score = r.damage
					best = a
				}
			}
			if score == 0 {
				t.Fatal("yellow fixture produced no actual damage")
			}
			t.Logf("native yellow best legal affixes: %+v", best)
			for _, roll := range []int32{8000, 10000, 12000} {
				var y, u int64
				ratios := []float64{}
				for _, seed := range []int64{11, 29, 47, 83, 101} {
					a := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, best, 0), seed)
					b := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, nil, roll), seed)
					y += a.damage
					u += b.damage
					ratios = append(ratios, float64(b.damage)/float64(a.damage))
					t.Logf("roll=%d seed=%d yellow=%+v unique=%+v", roll, seed, a, b)
				}
				ratio := float64(u) / float64(y)
				t.Logf("single-target60s roll=%d ratio=%.4f seeds=%v", roll, ratio, ratios)
				if ratio < 3 {
					t.Errorf("godlike actual single-target damage %.4fx < 3x (roll %d); fix catalog, never weaken threshold", ratio, roll)
				}
			}
		})
	}
}

type rebalanceV6Upgrade struct {
	Native                                                               bool
	Position, Tier, RefineLevel, Durable, AttackWear, HitWear, DeathWear int32
	RawRefine                                                            []domain.Affix
	Source, RefineSource                                                 string
}

func rebalanceV6Invest(t *testing.T, in rebalanceV6Input, st domain.Stack) domain.Stack {
	t.Helper()
	d := in.items[st.Item]
	if in.refine {
		st.RefineLevel = in.upgrades[st.Item].RefineLevel
	}
	if in.sockets > domain.EquipmentSocketCapacity(d, st) {
		t.Fatal("socket budget exceeded", st.Item)
	}
	st.SocketCount = uint8(in.sockets)
	for n := 0; n < in.sockets; n++ {
		st.Sockets[n] = in.card
		st.SocketUIDs[n] = st.UID*10 + int64(n+1)
		st.SocketCards[n] = in.items[in.card].CardDefaults
	}
	if domain.EquipmentAttributeCount(d, st) > domain.MaxEquipmentAttributes {
		t.Fatal("over-budget yellow", st)
	}
	return st
}

// A fully equipped shared yellow background. Every selected base is native,
// level <=54, naked-stat/legal-profession eligible, and both arms wear the exact
// same objects. This is a documented reproducible strong loadout, not a claim
// that one greedy loadout is the global optimum across all possible builds.
func rebalanceV6CommonGear(t *testing.T, in rebalanceV6Input, tc rebalanceV6Case) *domain.EquipSet {
	b := rebalanceV6Base(t, in.items[tc.id], tc.race)
	chosen := map[domain.EquipSlot]domain.ItemDef{}
	ids := []int{}
	for id := range in.upgrades {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		d := in.items[domain.ItemID(id)]
		e := d.Equip
		slot := domain.EquipSlot(e.Slot)
		if slot == domain.SlotWeapon || slot == domain.SlotTwoHand || slot == domain.SlotShield {
			continue
		}
		if !e.Need.AllowsProfession(tc.race) || e.Need.Meet(54, b, 0) != domain.RejectNone {
			continue
		}
		old, ok := chosen[slot]
		if !ok || e.LevelReq > old.Equip.LevelReq {
			chosen[slot] = d
		}
	}
	worn := domain.NewEquipSet()
	for slot, d := range chosen {
		// All offensive families available on ordinary accessories fit within four
		// lines in this source; the remaining line is spent on HP rather than an
		// invented sixth offensive affix. Mode choice is explicit in the report.
		priorities := []int32{domain.AttrAtkSpeed, domain.AttrAtk, domain.AttrSTR, domain.AttrHit, domain.AttrDEX, domain.AttrMaxHP, domain.AttrVIT}
		if tc.race >= domain.Healer {
			priorities = []int32{domain.AttrMAtk, domain.AttrINT, domain.AttrSPI, domain.AttrMaxMP, domain.AttrMaxHP, domain.AttrVIT}
		}
		a := []domain.Affix{}
		for _, attr := range priorities {
			var best domain.Affix
			found := false
			for _, r := range in.pools {
				if r.Tier != d.Equip.ResourceLevel || r.Type != d.Equip.Type || r.Attr != attr {
					continue
				}
				candidate := domain.Affix{Attr: r.Attr, Mode: r.Mode, Value: r.Max * r.Sign}
				if !found || candidate.Mode > best.Mode || candidate.Mode == best.Mode && candidate.Value > best.Value {
					best, found = candidate, true
				}
			}
			if found && len(a) < 5-len(d.Equip.Affixes)-in.sockets {
				a = append(a, best)
			}
		}
		st := rebalanceV6Invest(t, in, rebalanceV6Stack(d, a, 0))
		worn.Set(slot, st)
	}
	if worn.Count() < 4 {
		t.Fatal("too few eligible common yellow pieces", tc.race, worn.Count())
	}
	return worn
}
func TestEquipmentRebalanceV6InvestedSingleTarget(t *testing.T) {
	for _, profile := range []struct {
		name                    string
		sockets                 int
		physicalCard, magicCard domain.ItemID
		enforce                 bool
	}{{"refine10_no_cards", 0, 0, 0, true}, {"stage54_drop_one_card_per_piece", 1, 7694, 7373, true}, {"unrestricted_five_top_cards_stress", 5, 10944, 10945, false}} {
		t.Run(profile.name, func(t *testing.T) {
			in := rebalanceV6Inputs(t)
			in.refine = true
			in.sockets = profile.sockets
			for _, tc := range []rebalanceV6Case{{1510, domain.Swordsman, 0, false}, {1310, domain.Warlock, 10401, false}, {1610, domain.Warrior, 0, true}, {1810, domain.Assassin, 10219, false}} {
				tc := tc
				t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
					in := in
					t.Parallel()
					in.card = profile.physicalCard
					if tc.race >= domain.Healer {
						in.card = profile.magicCard
					}
					in.common = map[domain.Race]*domain.EquipSet{tc.race: rebalanceV6CommonGear(t, in, tc)}
					d := in.items[tc.id]
					in.common[tc.race].Each(func(slot domain.EquipSlot, st domain.Stack) {
						t.Logf("common slot=%d item=%d refine=%d holes=%d card=%d natural=%v", slot, st.Item, st.RefineLevel, st.SocketCount, in.card, st.RolledAffixes[:st.RolledAffixCount])
					})
					var best []domain.Affix
					score := int64(-1)
					for _, a := range rebalanceV6Yellows(in, tc) {
						r := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, a, 0), 17)
						if r.damage > score {
							score, best = r.damage, a
						}
					}
					var y, u rebalanceV6Result
					ratios := []float64{}
					for seed := int64(1); seed <= 20; seed++ {
						a := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, best, 0), seed*37)
						b := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, nil, 8000), seed*37)
						if a.damage <= 0 {
							t.Fatal("zero actual denominator", seed)
						}
						ratios = append(ratios, float64(b.damage)/float64(a.damage))
						y.burst += a.burst
						y.casts += a.casts
						y.endHP += a.endHP
						y.endMP += a.endMP
						y.damage += a.damage
						u.burst += b.burst
						u.casts += b.casts
						u.endHP += b.endHP
						u.endMP += b.endMP
						u.damage += b.damage
						y.skillDamage += a.skillDamage
						u.skillDamage += b.skillDamage
						y.mpSpent += a.mpSpent
						u.mpSpent += b.mpSpent
						y.hits += a.hits
						u.hits += b.hits
						y.misses += a.misses
						u.misses += b.misses
						y.aliveTicks += a.aliveTicks
						u.aliveTicks += b.aliveTicks
						y.lowTicks += a.lowTicks
						u.lowTicks += b.lowTicks
					}
					ratio := float64(u.damage) / float64(y.damage)
					sort.Float64s(ratios)
					t.Logf("20 seed ratio distribution min=%.4f median=%.4f max=%.4f all=%v", ratios[0], ratios[len(ratios)/2], ratios[len(ratios)-1], ratios)
					t.Logf("%s item=%d singleTargetRatio=%.4f yellow=%+v unique=%+v affixes=%+v commonPieces=%d", profile.name, tc.id, ratio, y, u, best, in.common[tc.race].Count())
					if y.damage <= 0 {
						t.Fatal("zero denominator")
					}
					if profile.enforce && ratio < 3 {
						t.Errorf("invested strong-yellow target %.4fx below 3x", ratio)
					}
				})
			}
		})
	}
}

// Ordinary PvE monsters currently have no elemental-resistance field. These
// are deliberately named settlement-layer cases, not 60-second PvE simulations.
func TestEquipmentRebalanceV6ResistanceSettlementAndImmunity(t *testing.T) {
	in := rebalanceV6Inputs(t)
	tc := rebalanceV6Case{1310, domain.Warlock, 10401, false}
	s, p, _, _, _ := rebalanceV6Scene(t, in, tc, rebalanceV6Stack(in.items[1310], nil, 8000), 31)
	pierce := s.equipmentOffense(p).Elements[domain.ElementIndex("fire")].PierceBP
	for _, raw := range []int32{0, 6000, 9000} {
		yellow := domain.ElementResistanceDamage(10000, raw, 0, false)
		unique := domain.ElementResistanceDamage(10000, raw, pierce, false)
		want := int32(int64(10000) * (10000 - int64(min(int32(7500), max(int32(-5000), raw-min(int32(3000), pierce))))) / 10000)
		if unique != want {
			t.Fatal("raw overcap resistance incorrectly discarded", raw, unique, want)
		}
		t.Logf("SETTLEMENT ONLY rawResistBP=%d pierceBP=%d yellow=%d unique=%d resistanceChannelRatio=%.4f", raw, pierce, yellow, unique, float64(unique)/float64(yellow))
	}
	tc = rebalanceV6Case{1810, domain.Assassin, 10219, false}
	s, p, m, _, now := rebalanceV6Scene(t, in, tc, rebalanceV6Stack(in.items[1810], nil, 8000), 31)
	s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{10219: {}}}}
	before := p.MP
	s.onUseSkill(UseSkill{ID: p.ID, Target: m.ID, Skill: 10219})
	for n := 0; n < 600; n++ {
		*now = now.Add(100 * time.Millisecond)
		s.tick++
		s.stepCasts()
		s.stepCombat()
	}
	if m.HP != m.MaxHP || p.MP >= before {
		t.Fatal("whole-skill immunity bypassed or cast never admitted", m.HP, p.MP, before)
	}
}
func TestEquipmentRebalanceV6SkillCardAgreesWithAcceptedCast(t *testing.T) {
	in := rebalanceV6Inputs(t)
	for _, tc := range []rebalanceV6Case{{1310, domain.Warlock, 10401, false}, {1810, domain.Assassin, 10219, false}} {
		s, p, m, _, _ := rebalanceV6Scene(t, in, tc, rebalanceV6Stack(in.items[tc.id], nil, 8000), 31)
		ranks, err := s.skillRanksForDisplay(p)
		if err != nil {
			t.Fatal(err)
		}
		view := ranks[tc.skill]
		level, _ := s.skillLevelForUse(p, tc.skill)
		d, ok := s.skills.Get(tc.skill, level)
		if !ok {
			t.Fatal("missing actual level")
		}
		s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
		if view.EffectiveRank != strconv.Itoa(int(level)) || view.Contract == nil || view.Contract.Current == nil {
			t.Fatal("card rank disagrees with cast", view, level)
		}
		card := view.Contract.Current
		if card.MPCost != d.MPCost || card.PrepareMS != d.PrepareMS || card.CooldownMS != d.CooldownMS {
			t.Fatal("card timing/resource disagreement", card, d)
		}
		expected := skillFractionText(d.Effect.DamagePct, d.Effect.DamagePctNumerator, d.Effect.DamagePctDenominator) + "%"
		if !strings.Contains(strings.Join(view.CurrentEffectLines, "\n"), expected) {
			t.Fatal("card damage percentage missing actual rank", expected, view.CurrentEffectLines)
		}
		before := p.MP
		s.onUseSkill(UseSkill{ID: p.ID, Target: m.ID, Skill: tc.skill})
		if len(s.casts) != 1 || before-p.MP != d.MPCost {
			t.Fatal("accepted cast diverged from card")
		}
		cast := s.casts[0].def
		if cast.Effect.DamagePctNumerator != d.Effect.DamagePctNumerator || cast.Effect.DamagePctDenominator != d.Effect.DamagePctDenominator || cast.Effect.DamagePct != d.Effect.DamagePct {
			t.Fatal("accepted damage snapshot disagrees with card")
		}
	}
}

func TestEquipmentRebalanceV6InvestedWarlockDiagnostic(t *testing.T) {
	in := rebalanceV6Inputs(t)
	in.refine = true
	tc := rebalanceV6Case{1310, domain.Warlock, 10401, false}
	in.common = map[domain.Race]*domain.EquipSet{tc.race: rebalanceV6CommonGear(t, in, tc)}
	for _, roll := range []int32{0, 8000} {
		t.Log("begin", roll)
		r := rebalanceV6Run(t, in, tc, rebalanceV6Stack(in.items[tc.id], nil, roll), 37)
		t.Log("result", roll, r)
	}
}

func TestEquipmentRebalanceV6HealerEffectiveHealingAndMP(t *testing.T) {
	in := rebalanceV6Inputs(t)
	// Guarded Mend lives on a level-59 crown. This support fixture is therefore
	// explicitly level 60, separate from the four level-54 weapon comparisons.
	for _, scenario := range []struct {
		count     int
		threshold int32
	}{{1, 0}, {3, 0}, {1, 70}, {3, 70}} {
		count := scenario.count
		for _, unique := range []bool{false, true} {
			t.Run(fmt.Sprintf("patients%d/missingThreshold%d/unique%v", count, scenario.threshold, unique), func(t *testing.T) {
				s := newTestScene(t, nil)
				s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
				sink := join(t, s, 1, 100, "药师缺血脚本", 100, 100)
				p := s.players[1]
				ch := p.Player.Char
				now := time.Unix(1800000000, 0)
				s.now = func() time.Time { return now }
				s.rng = rand.New(rand.NewSource(37))
				s.items, s.skills, s.statuses = in.items, in.skills, in.statuses
				ch.Race, ch.Level, ch.SkillAllocationVersion = domain.Healer, 60, domain.SkillFormulaVersion
				ch.EmploymentKnown, ch.Employed, ch.SkillPointsKnown = true, true, true
				ch.Skills = domain.Learned{10301: 20}
				ch.SkillPoints = 39
				ch.Base = domain.StartingBase(domain.Healer)
				for level := int32(2); level <= 60; level++ {
					ch.Base = ch.Base.Add(domain.GrowthAt(domain.Healer, level))
				}
				needSPI := max(int32(0), in.items[2056].Equip.Need.Base.SPI-ch.Base.SPI)
				ch.Base.SPI += needSPI
				ch.Base.INT += 59*domain.FreePointsPerLevel - needSPI
				if err := domain.ValidateSkillAllocation(60, 4, ch.Skills, ch.SkillPoints); err != nil {
					t.Fatal(err)
				}
				d := in.items[2056]
				affixes := []domain.Affix{}
				for _, attr := range []int32{domain.AttrMaxMP, domain.AttrSPI, domain.AttrINT, domain.AttrMaxHP, domain.AttrVIT} {
					for _, r := range in.pools {
						if r.Tier == d.Equip.ResourceLevel && r.Type == d.Equip.Type && r.Attr == attr && len(affixes) < 5-len(d.Equip.Affixes) {
							affixes = append(affixes, domain.Affix{Attr: r.Attr, Value: r.Max * r.Sign, Mode: r.Mode})
							break
						}
					}
				}
				if unique {
					affixes = nil
				}
				roll := int32(0)
				if unique {
					roll = 8000
				}
				st := rebalanceV6Stack(d, affixes, roll)
				p.Player.Worn = domain.NewEquipSet()
				p.Player.Worn.Set(domain.EquipSlot(d.Equip.Slot), st)
				p.Player.EndgameGrowthV5, p.Player.EndgameSkillBehavior, p.Player.EndgameSkillVariants = true, true, true
				p.Player.Party = 77
				s.refreshStats(p)
				if s.functionalWorn(p).Count() != 1 {
					t.Fatal("support crown inactive")
				}
				p.HP = p.MaxHP
				p.MP = min(p.MaxMP, domain.DeriveFromBase(ch.Base).MaxMP)
				patients := []*entity.Entity{}
				enemies := []*entity.Entity{}
				for i := 0; i < count; i++ {
					id := domain.EntityID(10 + i)
					join(t, s, id, int64(200+i), "患者", 110+float64(i*20), 100)
					q := s.players[id]
					q.Player.Party = 77
					q.HP, q.MaxHP = 2500, 5000
					q.Stats.MaxHP = 5000
					patients = append(patients, q)
					m := putMonster(s, domain.EntityID(910+i), 10000000, domain.Stats{MinAtk: 40, MaxAtk: 40, AtkSpeedMS: 100, Hit: 10000}, 120+float64(i*20), 100)
					enemies = append(enemies, m)
				}
				var healing, spent, incoming int64
				casts := 0
				nextTry := 0
				sink.take()
				for n := 0; n < 600; n++ {
					now = now.Add(100 * time.Millisecond)
					s.tick++
					s.stepStatus()
					for i, q := range patients {
						if q.Alive() {
							before := q.HP
							s.onAttack(Attack{ID: enemies[i].ID, Target: q.ID, Blow: combat.Blow{GuaranteedHit: true}})
							s.stepCombat()
							incoming += int64(before - q.HP)
						}
					}
					var target *entity.Entity
					for _, q := range patients {
						if q.Alive() && q.HP < q.MaxHP && q.HP*100 <= q.MaxHP*(100-scenario.threshold) && (target == nil || q.HP < target.HP) {
							target = q
						}
					}
					if target != nil && n >= nextTry && p.Player.SkillReadyAt(10301, s.tick) && !s.playerPreparingSkill(p.ID) {
						before := p.MP
						s.onUseSkill(UseSkill{ID: p.ID, Target: target.ID, Skill: 10301})
						spent += int64(before - p.MP)
						if before == p.MP {
							nextTry = n + 10
						}
					}
					s.stepCasts()
					s.flush()
					for _, e := range sink.take() {
						switch e := e.(type) {
						case event.HealDone:
							if e.Src == p.ID {
								healing += int64(e.Amount)
							}
						case event.SkillCastChanged:
							if e.Caster == p.ID && e.Phase == event.SkillCastChant {
								casts++
							}
						}
					}
				}
				alive := 0
				hp := int64(0)
				for _, q := range patients {
					if q.Alive() {
						alive++
					}
					hp += int64(q.HP)
				}
				if healing <= 0 || spent <= 0 {
					t.Fatal("healing fixture did not cast", healing, spent)
				}
				if healing != hp-int64(count*2500)+incoming {
					t.Fatal("effective heal event/HP accounting mismatch", healing, hp, incoming)
				}
				t.Logf("level60 patients=%d missingThreshold=%d unique=%v effectiveHealing=%d paidMP=%d effectiveHealingPerMP=%.4f casts=%d survivors=%d finalPatientHP=%d incomingActualHPLoss=%d", count, scenario.threshold, unique, healing, spent, float64(healing)/float64(spent), casts, alive, hp, incoming)
			})
		}
	}
}

func TestEquipmentRebalanceV6WarriorFullHealthAndLethalPressure(t *testing.T) {
	in := rebalanceV6Inputs(t)
	in.refine = true
	in.sockets = 1
	in.card = 7694
	tc := rebalanceV6Case{1610, domain.Warrior, 0, true}
	in.common = map[domain.Race]*domain.EquipSet{tc.race: rebalanceV6CommonGear(t, in, tc)}
	d := in.items[1610]
	best := []domain.Affix{{Attr: 19, Value: 20}, {Attr: 25, Mode: 1, Value: -10}, {Attr: 31, Value: 60}, {Attr: 47, Mode: 1, Value: 39}}
	for _, full := range []bool{false, true} {
		for _, pressure := range []int32{30, 100} {
			in.fullHealth = full
			in.pressureDamage = pressure
			var yd, ud int64
			for _, seed := range []int64{37, 74, 111, 148, 185} {
				y := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, best, 0), seed)
				u := rebalanceV6Run(t, in, tc, rebalanceV6Stack(d, nil, 8000), seed)
				yd += y.damage
				ud += u.damage
				if pressure == 100 && !full && (y.endHP > 0 || u.endHP > 0 || y.aliveTicks >= 600 || u.aliveTicks >= 600) {
					t.Fatal("lethal pressure failed to stop actual living combat", y, u)
				}
				t.Logf("fullStart=%v incomingPer2s=%d seed=%d yellow=%+v unique=%+v", full, pressure, seed, y, u)
			}
			t.Logf("warrior same movement and incoming script fullStart=%v incoming=%d singleTargetRatio=%.4f", full, pressure, float64(ud)/float64(yd))
		}
	}
}

func TestEquipmentRebalanceV6WarlockResourceSpecializationScope(t *testing.T) {
	in := rebalanceV6Inputs(t)
	tc := rebalanceV6Case{1310, domain.Warlock, 10401, false}
	s, p, _, _, _ := rebalanceV6Scene(t, in, tc, rebalanceV6Stack(in.items[1310], nil, 8000), 37)
	for _, id := range []domain.SkillID{10401, 10409} {
		before, ok := s.skills.Get(id, 20)
		if !ok {
			t.Fatal("missing spell", id)
		}
		after := before
		s.applyUniqueSkillModifiers(p, &after)
		if id == 10401 {
			if after.MPCost >= before.MPCost || after.PrepareMS >= before.PrepareMS {
				t.Fatal("advertised specialization not applied", before.MPCost, after.MPCost, before.PrepareMS, after.PrepareMS)
			}
		} else if after.MPCost != before.MPCost || after.PrepareMS != before.PrepareMS {
			t.Fatal("fire specialization leaked to ice", before, after)
		}
	}
}
