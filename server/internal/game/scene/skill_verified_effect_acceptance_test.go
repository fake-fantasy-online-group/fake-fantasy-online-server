package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func verifiedEffectScene(t *testing.T, id domain.SkillID, rank int32) (*Scene, *entity.Entity, *entity.Entity, domain.SkillDef) {
	t.Helper()
	s := newTestScene(t, nil)
	join(t, s, 1, 100, "效果验收", 100, 100)
	p := s.players[1]
	ident, _ := domain.V5SkillIdentity(id)
	p.Player.Char.Race = domain.Race(ident.Profession - 1)
	p.Player.Char.Level = 100
	p.Player.Char.Skills = domain.Learned{id: rank}
	s.skills, s.statuses = nativeRestoredSceneFixtures(t)
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 1 << 20, MAtk: 100}
	p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
	base := domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Def: 100, MDef: 100, Hit: 100, MaxHP: 100000, AtkSpeedMS: 1000, MoveSpeed: 100}
	s.defs = spawn.MapDefs{338: domain.MonsterDef{ID: 338, HP: 100000, Stats: base}}
	dst := putMonster(s, 900, 100000, base, 130, 100)
	def, ok := s.skills.Get(id, rank)
	if !ok {
		t.Fatal("missing", id, rank)
	}
	def.SnapshotRawRanks = p.Player.Char.Skills.Clone()
	def.SnapshotHardRanks = p.Player.Char.Skills.Clone()
	return s, p, dst, def
}

func verifiedActive(t *testing.T, target *entity.Entity, id domain.StatusID) domain.Active {
	t.Helper()
	var found bool
	var out domain.Active
	target.Status.Each(func(a domain.Active) {
		if a.Def.ID == id {
			out = a
			found = true
		}
	})
	if !found {
		t.Fatalf("missing applied runtime status %d", id)
	}
	return out
}

// Exercise real applySkill -> status resolution -> entity-stat recalculation for
// all five original ranks. Checking only a populated Statuses slice cannot pass.
func TestVerifiedOriginalEffectsReachRuntimeAll55Ranks(t *testing.T) {
	for _, id := range []domain.SkillID{10009, 10103, 10109, 10111, 10113, 10122, 10124, 10219, 10221, 10224, 10325} {
		for rank := int32(1); rank <= 5; rank++ {
			t.Run(fmt.Sprintf("%d/%d", id, rank), func(t *testing.T) {
				s, p, dst, def := verifiedEffectScene(t, id, rank)
				if !s.applySkill(p, dst, def, 0, false) {
					t.Fatal("actual skill effect failed")
				}
				var duration int32
				var status domain.StatusID
				switch id {
				case 10009:
					status, duration = 10009, 15
					if dst.Stats.Def != 80-10*rank {
						t.Fatal("defense debuff inert", dst.Stats)
					}
				case 10103:
					status, duration = domain.StatusBlind, 15
					if dst.Stats.Hit != 75-5*rank {
						t.Fatal("hit debuff inert", dst.Stats)
					}
				case 10109:
					status, duration = 1019, 10
					if dst.Stats.Def != 160+20*rank || canMove(dst) || !canAct(dst) || !canCast(dst) {
						t.Fatal("defense/movement-only tradeoff", dst.Stats)
					}
				case 10111:
					status, duration = 10111, 5+rank
					if canMove(dst) || !canAct(dst) || !canCast(dst) {
						t.Fatal("root changed attack/cast permissions")
					}
				case 10113:
					status, duration = domain.StatusSilence, 5+rank
					if canCast(dst) || !canMove(dst) || !canAct(dst) {
						t.Fatal("seal did not restrict only skills")
					}
				case 10122:
					status, duration = domain.StatusChaos, 9+rank
					before := dst.Stats.Def
					if s.applyStatusDef(dst, domain.StatusDef{ID: 9999, Level: 1, Type: domain.StatusGood, TypeKnown: true, DurationSec: 30, Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: 100}}}, p.ID) || dst.Stats.Def != before {
						t.Fatal("chaos did not reject real beneficial application")
					}
				case 10124:
					status, duration = 1019, 30
					delta := 7 + 3*rank
					if dst.Stats.Def != 100+delta || dst.Stats.MinAtk != 100-delta || dst.Stats.MaxAtk != 100-delta {
						t.Fatal("mountain defense/attack tradeoff", dst.Stats)
					}
				case 10219:
					status, duration = domain.StatusStun, 4+rank
					if canMove(dst) || canAct(dst) || canCast(dst) {
						t.Fatal("stun ineffective")
					}
				case 10221:
					status, duration = 10111, 5
					if canMove(dst) || !canAct(dst) || !canCast(dst) {
						t.Fatal("snipe root became incapacitation")
					}
				case 10224:
					status = domain.StatusDampen
					if dst.Stats.MDef != 100 || dst.Stats.MAtk >= 100 {
						t.Fatal("smoke retained invented magic-defense loss or lost magic-attack loss", dst.Stats)
					}
				case 10325:
					status, duration = domain.StatusSlow, 30
					slow := 25 + 5*rank
					if dst.Stats.PlayerAttackSpeedPercent() != 100-slow || dst.Stats.AtkSpeedMS != (100000+(100-slow)-1)/(100-slow) || dst.Stats.MoveSpeed != 100 {
						t.Fatal("shackle must reduce attack speed without invented movement slow", dst.Stats)
					}
				}
				active := verifiedActive(t, dst, status)
				if duration > 0 && (active.Def.DurationSec != duration || active.ExpireAt-s.tick != domain.Ticks(int(duration)*1000)) {
					t.Fatal("runtime duration mismatch", active, duration)
				}
				if (id == 10109 || id == 10111 || id == 10221) && active.Def.Immobile != 1 {
					t.Fatal("movement lock missing client immobile flag", active.Def)
				}
			})
		}
	}
}

func TestVerifiedShackleActualPlayerAndMonsterAttackIntervals(t *testing.T) {
	for _, rank := range []int32{1, 2, 3, 4, 5, 20, 100} {
		t.Run(fmt.Sprint(rank), func(t *testing.T) {
			s, src, monster, def := verifiedEffectScene(t, 10325, rank)
			join(t, s, 2, 200, "枷锁目标", 120, 100)
			p := s.players[2]
			s.refreshEntityStats(p)
			before := p.Stats.PlayerAttackSpeedPercent()
			if before != 100 {
				t.Fatal("unexpected baseline", before)
			}
			if !s.applySkill(src, p, def, 0, false) || !s.applySkill(src, monster, def, 0, false) {
				t.Fatal("shackle application failed")
			}
			speed := int64(max(50, 100-(25+5*rank)))
			for _, ranged := range []bool{false, true} {
				p.Look.AtkDist = 100
				ms := int64(1000)
				if ranged {
					p.Look.AtkDist = 200
					ms = 500
				}
				want := domain.Tick(ms * 100 / speed / domain.TickMS)
				p.DidBasicAttack(100)
				if p.ReadyToAttack(100+want-1) || !p.ReadyToAttack(100+want) {
					t.Fatal("player actual admission interval mismatch", rank, ranged, want)
				}
				if want <= domain.Tick(ms/domain.TickMS) {
					t.Fatal("shackle did not slow attack admission")
				}
			}
			// This restored state reduces attack frequency, without changing any
			// existing equipment interval-percent semantics.
			expectedMS := int32((100000 + speed - 1) / speed)
			for refresh := 0; refresh < 3; refresh++ {
				s.refreshEntityStats(monster)
				if monster.Stats.AtkSpeedMS != expectedMS {
					t.Fatal("shackle interval compounded or wrong", rank, refresh, monster.Stats.AtkSpeedMS, expectedMS)
				}
			}
			want := domain.Ticks(int(expectedMS))
			monster.DidBasicAttack(100)
			if monster.ReadyToAttack(100+want-1) || !monster.ReadyToAttack(100+want) || want <= 10 {
				t.Fatal("monster actual interval mismatch", rank, want)
			}
			t.Logf("rank%d player speed%d%% monster interval%dms", rank, p.Stats.PlayerAttackSpeedPercent(), monster.Stats.AtkSpeedMS)
		})
	}
}

func TestVerifiedRootSourceRefreshOnlyDownwardAndReleases(t *testing.T) {
	s, p, dst, def := verifiedEffectScene(t, 10111, 5)
	if !s.applySkill(p, dst, def, 0, false) {
		t.Fatal("root failed")
	}
	old := verifiedActive(t, dst, 10111)
	p.Player.Char.Skills[10111] = 20
	s.refreshSkillPersistentEffects(p)
	same := verifiedActive(t, dst, 10111)
	if same.Def.DurationSec != old.Def.DurationSec || same.ExpireAt != old.ExpireAt {
		t.Fatal("source gain strengthened accepted root", old, same)
	}
	p.Player.Char.Skills[10111] = 1
	s.refreshSkillPersistentEffects(p)
	down := verifiedActive(t, dst, 10111)
	if down.Def.DurationSec != 6 || down.Def.SkillSnapshotRank != 1 || canMove(dst) {
		t.Fatal("source downgrade not honored", down)
	}
	delete(p.Player.Char.Skills, 10111)
	s.refreshSkillPersistentEffects(p)
	if !canMove(dst) {
		t.Fatal("lost source retained movement lock")
	}
}

func TestVerifiedHighRootCannotBypassCategoricalImmunity(t *testing.T) {
	s, p, dst, def := verifiedEffectScene(t, 10111, 100)
	if !s.applyStatusDef(dst, domain.StatusDef{ID: domain.StatusSpiritLock, Level: 1, Type: domain.StatusGood, TypeKnown: true, DurationSec: 60, BlockHarmful: true}, dst.ID) {
		t.Fatal("immunity application failed")
	}
	s.applySkill(p, dst, def, 0, false)
	if !canMove(dst) {
		t.Fatal("high root pierced categorical harmful immunity")
	}
	dst.Status.Each(func(a domain.Active) {
		if a.Def.ID == 10111 {
			t.Fatal("immune target acquired root", a)
		}
	})
}
