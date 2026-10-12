package scene

import (
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func TestOriginalControlExtensionChangesResistanceRollNotDurationOrDamage(t *testing.T) {
	var seed int64
	for ; ; seed++ {
		roll := rand.New(rand.NewSource(seed)).Intn(100)
		if roll >= 52 && roll < 90 {
			break
		}
	}
	for _, extra := range []int32{0, 20} {
		s := newTestScene(t, nil)
		join(t, s, 1, 100, "caster", 100, 100)
		first := domain.StatusDef{ID: domain.StatusStun, Level: 11, Type: domain.StatusBad, TypeKnown: true, DurationSec: 6, Control: domain.ControlStun}
		fifth := first
		fifth.Level, fifth.DurationSec = 15, 10
		s.statuses = statusTable(first, fifth)
		target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
		target.Stats.StatusResist = 90
		s.entities[target.ID] = target
		def := domain.SkillDef{ID: 10027, Level: 5 + extra, Statuses: []domain.StatusApplication{{ID: domain.StatusStun, Level: 15, ProgressionFirstLevel: 11, ProgressionRanks: extra}}}
		s.rng = rand.New(rand.NewSource(seed))
		if !s.applySkill(s.players[1], target, def, 0, false) {
			t.Fatal("status-only skill failed")
		}
		applied := false
		if target.Status != nil {
			target.Status.Each(func(a domain.Active) {
				if a.Def.ID == domain.StatusStun {
					applied = true
					if a.Def.DurationSec != 15 || a.Def.ControlPower != extra-5 {
						t.Fatal("control duration/power mismatch", a.Def)
					}
				}
			})
		}
		if applied != (extra > 0) || target.HP != 100 {
			t.Fatalf("extra ranks %d: applied=%t, HP=%d", extra, applied, target.HP)
		}
	}
}

func TestOriginalControlPressureCannotBypassHardBlocks(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s := newTestScene(t, nil)
		join(t, s, 1, 100, "caster", 100, 100)
		target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}, Status: domain.NewStatusSet()}
		s.entities[target.ID] = target
		if blocked {
			target.Status.Apply(domain.StatusDef{ID: domain.StatusSpiritLock, Level: 1, DurationSec: 10, BlockHarmful: true}, s.tick, target.ID)
		}
		def := domain.StatusDef{ID: domain.StatusStun, Level: 5, Type: domain.StatusBad, TypeKnown: true, DurationSec: 9, Control: domain.ControlStun, ControlPower: 100000}
		if got := s.applyStatusDef(target, def, 1); got == blocked {
			t.Fatalf("hard block=%t, applied=%t", blocked, got)
		}
	}
}

func TestOriginalControlFocusDurationRespectsExtendedCap(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "caster", 100, 100)
	first := domain.StatusDef{ID: domain.StatusStun, Level: 11, Type: domain.StatusBad, TypeKnown: true, DurationSec: 6, Control: domain.ControlStun}
	fifth := first
	fifth.Level, fifth.DurationSec = 15, 10
	s.statuses = statusTable(first, fifth)
	for _, extra := range []int32{0, 1, 20} {
		target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 100, MaxHP: 100, Monster: &entity.Monster{}}
		s.entities[target.ID] = target
		def := domain.SkillDef{ID: 10027, Level: 5 + extra, Statuses: []domain.StatusApplication{{ID: domain.StatusStun, Level: 15, ProgressionFirstLevel: 11, ProgressionRanks: extra}}}
		s.applySkill(s.players[1], target, def, 100, false)
		wantDuration := int32(15)
		if extra == 0 {
			wantDuration = 20 // Legacy rank5 modifier behavior remains exact.
		}
		if target.Status == nil || !target.Status.Has(domain.StatusStun) {
			t.Fatal("unresisted control did not apply", extra)
		}
		target.Status.Each(func(a domain.Active) {
			if a.Def.DurationSec != wantDuration {
				t.Fatal("focus escaped high-rank duration cap", extra, a.Def.DurationSec, wantDuration)
			}
		})
	}
}

func TestOriginalStatusShieldDowngradePreservesSpentPoolAndExpiry(t *testing.T) {
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "healer", 100, 100)
	source := s.players[1]
	source.Player.Char.Race = domain.Healer
	source.Player.Char.Level = 100
	source.Player.Char.Skills = domain.Learned{10320: 6}
	s.skills = v5TestSkillTable(10320, 4, domain.SkillEffect{})
	s.statuses = domain.StatusTable{}
	for rank := int32(1); rank <= 5; rank++ {
		key := domain.SkillKey{ID: 10320, Level: rank}
		row := s.skills[key]
		row.Statuses = []domain.StatusApplication{{ID: domain.StatusDamageShield, Level: rank}}
		s.skills[key] = row
		s.statuses[domain.StatusKey{ID: domain.StatusDamageShield, Level: rank}] = domain.StatusDef{
			ID: domain.StatusDamageShield, Level: rank, Type: domain.StatusGood, TypeKnown: true,
			DamageShield: 500 + 200*rank, DurationSec: 50 + 10*rank,
		}
	}
	def, ok := s.skills.Get(10320, 6)
	if !ok {
		t.Fatal("extended shield skill unresolved")
	}
	def.SnapshotRawRanks = domain.Learned{10320: 6}
	def.SnapshotHardRanks = domain.Learned{10320: 6}
	if !s.applySkill(source, source, def, 0, false) {
		t.Fatal("shield not applied")
	}
	readShield := func() domain.Active {
		var shield domain.Active
		source.Status.Each(func(a domain.Active) {
			if a.Def.ID == domain.StatusDamageShield {
				shield = a
			}
		})
		return shield
	}
	cast := readShield()
	if cast.Def.DamageShield != 1700 || cast.ShieldLeft != 1700 {
		t.Fatal("rank6 shield failed to resolve", cast)
	}
	if damage, _ := source.Status.MitigateDamage(false, 300); damage != 0 {
		t.Fatal("shield did not absorb damage", damage)
	}
	source.Player.Char.Skills[10320] = 5
	s.refreshSkillPersistentEffects(source)
	down := readShield()
	if down.Def.DamageShield != 1500 || down.ShieldLeft != 1200 || down.ExpireAt > cast.ExpireAt || down.Def.SkillSnapshotRank != 5 {
		t.Fatal("downgrade refreshed/refilled shield", cast, down)
	}
	source.Player.Char.Skills[10320] = 7
	s.refreshSkillPersistentEffects(source)
	up := readShield()
	if up.Def.DamageShield != down.Def.DamageShield || up.ShieldLeft != down.ShieldLeft || up.ExpireAt != down.ExpireAt {
		t.Fatal("later equipment/rank gain increased old cast", down, up)
	}
}
