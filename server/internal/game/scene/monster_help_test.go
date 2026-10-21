package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestHelpRejectsOtherSpeciesAndAcceptsExplicitGroup(t *testing.T) {
	s, m, o := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	other := defs[m.Monster.TypeID]
	other.ID = 1002
	defs[other.ID] = other
	o.Monster.TypeID = other.ID
	s.aggro(m, 1)
	if o.Monster.Target != 0 {
		t.Fatal("recruited different species")
	}
	caller := defs[m.Monster.TypeID]
	caller.HelpGroup = "reviewed-test-species"
	other.HelpGroup = caller.HelpGroup
	defs[caller.ID] = caller
	defs[other.ID] = other
	s.tick += 10
	s.aggro(m, 1)
	if o.Monster.Target != 1 {
		t.Fatal("explicit species group ignored")
	}
}
func TestHelpRadiusAndTemporaryPursuit(t *testing.T) {
	s, m, o := behaviorFixture(t)
	s.aoi.Move(o, domain.Pos{MapID: 14, X: m.Pos.X + 800, Y: m.Pos.Y})
	s.aggro(m, 1)
	if o.Monster.Target != 1 {
		t.Fatal("800 pixel edge not recruited")
	}
	if aiParams(o).TraceDist != 600 || s.monsterAIParams(o).TraceDist <= 800 {
		t.Fatal("arrival budget missing or global trace changed")
	}
	before := o.Pos.X
	s.stepAI()
	if o.Monster.Target != 1 || o.Pos.X >= before {
		t.Fatal("distant ally dropped instead of approaching")
	}
	s.tick = o.Monster.HelpChaseUntil
	if s.monsterAIParams(o).TraceDist != 600 {
		t.Fatal("arrival budget did not expire")
	}
	s.dropTarget(o)
	if o.Monster.HelpChaseUntil != 0 || o.Monster.HelpTraceDist != 0 {
		t.Fatal("help budget survived disengage")
	}
}
func TestHelpConfirmedHitSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag event.DamageFlag
		hp   int32
		want bool
	}{
		{"miss", event.DamageMiss, 1, false}, {"fatal", event.DamageFatal, 0, false}, {"dead", 0, 0, false}, {"zero-damage-hit", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, o := behaviorFixture(t)
			m.HP = tc.hp
			s.aggroAfterHit(m, 1, event.DamageDealt{Flag: tc.flag})
			if tc.name == "miss" && m.Monster.Target != 1 {
				t.Fatal("miss incorrectly removed basic retaliation")
			}
			if (o.Monster.Target == 1) != tc.want {
				t.Fatal("incorrect hit eligibility")
			}
		})
	}
}
func TestHelpDoesNotCascadeOrStealTarget(t *testing.T) {
	s, m, o := behaviorFixture(t)
	third := *o
	md := *o.Monster
	third.Monster = &md
	third.ID++
	third.Pos.X = m.Pos.X + 1000
	s.entities[third.ID] = &third
	s.monsters[third.ID] = &third
	s.aoi.Enter(&third)
	s.aggro(m, 1)
	if o.Monster.Target != 1 || third.Monster.Target != 0 {
		t.Fatal("help cascaded beyond caller radius")
	}
	o.Monster.Target = 33
	s.tick += 10
	s.aggro(m, 1)
	if o.Monster.Target != 33 {
		t.Fatal("stole existing fight")
	}
}
func TestHelpOriginalDialogueAndProtection(t *testing.T) {
	s, m, o := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	d := defs[m.Monster.TypeID]
	delete(defs, d.ID)
	d.ID = 1033
	defs[d.ID] = d
	m.Monster.TypeID = d.ID
	o.Monster.TypeID = d.ID
	s.monsterDialogs = domain.NewMonsterDialogs([]domain.MonsterDialog{{RowNo: 135, Monster: 1033, Scene: 0, Group: 2, Text: "兄弟们，一起上！"}, {RowNo: 138, Monster: 1033, Scene: 4, Group: 1, Text: "我会生气的哦！"}})
	s.aggro(m, 1)
	if m.Monster.HelpSpeechUntil != 50 {
		t.Fatal("caller did not speak original help line")
	}
	s.monsterSpeak(m, domain.MonsterSceneBeHit)
	if m.Monster.SpokenScenes&(1<<domain.MonsterSceneBeHit) != 0 {
		t.Fatal("ordinary line overwrote help")
	}
	o.Monster.Target = 0
	s.tick = 10
	s.aggro(m, 1)
	if m.Monster.HelpSpeechUntil != 50 {
		t.Fatal("repeated help refreshed bubble")
	}
	o.Monster.Target = 0
	s.tick = 50
	s.aggro(m, 1)
	if m.Monster.HelpSpeechUntil != 100 {
		t.Fatal("later help was blocked by once-per-life speech")
	}
	s.initializeMonster(m)
	if m.Monster.HelpSpeechUntil != 0 || m.Monster.HelpChaseUntil != 0 {
		t.Fatal("help runtime survived initialization")
	}
}

func TestHelpResponderUsesOwnOriginalLineAndOneBubble(t *testing.T) {
	s, m, o := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	d := defs[m.Monster.TypeID]
	delete(defs, d.ID)
	d.ID = 1031
	defs[d.ID] = d
	m.Monster.TypeID = d.ID
	o.Monster.TypeID = d.ID
	s.monsterDialogs = domain.NewMonsterDialogs([]domain.MonsterDialog{{RowNo: 116, Monster: 1031, Scene: 0, Group: 1, Text: "哥们！俺来啦！"}})
	s.outbox = nil
	s.aggro(m, 1)
	var speeches []event.MonsterSpoke
	for _, p := range s.outbox {
		if v, ok := p.ev.(event.MonsterSpoke); ok {
			speeches = append(speeches, v)
		}
	}
	if len(speeches) != 1 || speeches[0].Who != o.ID || speeches[0].Text != "哥们！俺来啦！" {
		t.Fatalf("wrong responder speech: %+v", speeches)
	}
	if m.Monster.HelpSpeechUntil != 0 {
		t.Fatal("victim said response line")
	}
}
func TestHelpDisabledDoesNotRecruit(t *testing.T) {
	s, m, o := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	d := defs[m.Monster.TypeID]
	d.CallHelp = false
	defs[d.ID] = d
	s.aggro(m, 1)
	if o.Monster.Target != 0 {
		t.Fatal("enabled help on a non-calling monster")
	}
}

func TestHelpZeroRadiusRemainsDisabled(t *testing.T) {
	s, m, o := behaviorFixture(t)
	m.Monster.AI.HelpRadius = 0
	s.aggro(m, 1)
	if o.Monster.Target != 0 {
		t.Fatal("zero radius must stay disabled")
	}
}
func TestHelpLevelDoesNotDefineSpecies(t *testing.T) {
	s, m, o := behaviorFixture(t)
	defs := s.defs.(spawn.MapDefs)
	d := defs[m.Monster.TypeID]
	d.Level = 61
	defs[d.ID] = d
	s.aggro(m, 1)
	if o.Monster.Target != 1 {
		t.Fatal("same species excluded by level")
	}
}
func TestHelpNilDialogsPreservesExistingGameplay(t *testing.T) {
	s, m, o := behaviorFixture(t)
	s.monsterDialogs = nil
	s.aggro(m, 1)
	if o.Monster.Target != 1 {
		t.Fatal("missing original dialogue disabled gameplay")
	}
}

func TestHelpBubbleSurvivesDisengageAndNewPlayerGreeting(t *testing.T) {
	s, m, other := behaviorFixture(t)
	s.monsterDialogs = domain.NewMonsterDialogs([]domain.MonsterDialog{{RowNo: 1, Monster: m.Monster.TypeID, Scene: domain.MonsterSceneNormal, Text: "ordinary greeting"}})
	m.Monster.HelpSpeechUntil = 50
	other.Monster.HelpSpeechUntil = 50
	m.Monster.Target = 1
	s.dropTarget(m)
	s.outbox = nil
	s.tick = 20
	s.sendIdleMonsterGreetingTo(s.entities[1])
	if len(s.outbox) != 0 {
		t.Fatal("arrival greeting overwrote active help bubble")
	}
	s.tick = 50
	s.sendIdleMonsterGreetingTo(s.entities[1])
	if len(s.outbox) != 1 {
		t.Fatal("normal greeting remained suppressed after help expired")
	}
}
