package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/ai"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
)

const monsterHelpRadius = 800.0
const monsterHelpCooldown domain.Tick = 10
const monsterHelpChaseTicks domain.Tick = 150
const monsterHelpSpeechTicks domain.Tick = 50

// Original ov_dialog identities. Caller and responder roles are distinct:
// never put an arriving companion's words in the victim's mouth.
type monsterHelpDialog struct{ caller, responder int32 }

var monsterHelpDialogs = map[domain.MonsterID]monsterHelpDialog{
	1031: {responder: 116}, 1032: {responder: 124}, 1033: {caller: 135}, 1035: {responder: 142},
	1041: {responder: 150}, 1043: {caller: 164}, 1065: {caller: 200}, 1094: {responder: 312},
	1120: {caller: 410}, 1128: {responder: 442}, 1131: {responder: 454}, 1133: {responder: 468},
	1134: {responder: 474}, 1135: {responder: 479}, 1163: {responder: 600}, 1164: {caller: 607},
	1172: {caller: 656}, 1173: {responder: 663}, 1174: {responder: 671}, 1203: {responder: 698}, 1209: {caller: 738},
}

func sameHelpSpecies(a, b domain.MonsterDef) bool {
	return a.ID == b.ID || a.HelpGroup != "" && a.HelpGroup == b.HelpGroup
}
func (s *Scene) aggroAfterHit(victim *entity.Entity, attacker domain.EntityID, hit event.DamageDealt) {
	s.aggroWithHelp(victim, attacker, !hit.Flag.Has(event.DamageMiss) && !hit.Flag.Has(event.DamageFatal))
}
func (s *Scene) monsterAIParams(m *entity.Entity) ai.Params {
	p := aiParams(m)
	// Dungeon aggression is scene policy, not shared template data. Dialogue
	// availability controls speech only. Keep props/non-attacking entities and
	// native-passive monsters in the open world unchanged.
	if (s.dungeon != nil || s.challengeMonsterScene()) && m.Monster.Kind.Hostile() && !m.Monster.NoAttack {
		p.Aggressive = true
	}
	if s.tick < m.Monster.HelpChaseUntil && m.Monster.HelpTraceDist > p.TraceDist {
		p.TraceDist = m.Monster.HelpTraceDist
	}
	return p
}
func (s *Scene) sayMonsterHelp(m *entity.Entity, row int32) bool {
	if row <= 0 || m == nil || !m.Alive() || m.Monster == nil {
		return false
	}
	text := s.monsterDialogs.RowLine(m.Monster.TypeID, row)
	if text == "" {
		return false
	}
	// Repeated assistance during this still-visible bubble does not resend it.
	if s.tick < m.Monster.HelpSpeechUntil {
		return true
	}
	m.Monster.HelpSpeechUntil = s.tick + monsterHelpSpeechTicks
	s.emit(event.MonsterSpoke{Who: m.ID, Text: text})
	return true
}

// Help scans around the victim, never recursively. It does not change idle
// acquisition or permanently enlarge normal pursuit. Missing original lines
// never become invented dialogue or disable existing help gameplay.
func (s *Scene) callMonsterHelp(victim *entity.Entity, attacker domain.EntityID) {
	if victim == nil || !victim.Alive() || victim.Monster == nil || s.defs == nil || victim.Monster.NoAttack || victim.Monster.AI.HelpRadius <= 0 {
		return
	}
	def, ok := s.defs.Def(victim.Monster.TypeID)
	if !ok || !def.CallHelp || !def.Kind.Hostile() || s.tick < victim.Monster.HelpReadyAt {
		return
	}
	a := s.entities[attacker]
	if a == nil || a.Kind != domain.KindPlayer || !a.Alive() || entityInvisible(a) || s.entityMapLoading(a) || !s.equipmentCombatReady(a) || a.Player != nil && a.Player.Protected(s.tick) {
		return
	}
	victim.Monster.HelpReadyAt = s.tick + monsterHelpCooldown
	var allies []*entity.Entity
	s.aoi.AroundRadius(victim.Pos, monsterHelpRadius, func(other *entity.Entity) {
		if other == nil || other.ID == victim.ID || !other.Alive() || other.Monster == nil || other.Monster.NoAttack || !other.Monster.Kind.Hostile() || other.Monster.Target != 0 || sqDist(victim.Pos, other.Pos) > monsterHelpRadius*monsterHelpRadius {
			return
		}
		od, found := s.defs.Def(other.Monster.TypeID)
		if !found || !sameHelpSpecies(def, od) {
			return
		}
		if !s.straightPathWalkable(victim.Pos, other.Pos) {
			return
		}
		allies = append(allies, other)
	})
	if len(allies) == 0 {
		return
	}
	spoken := s.sayMonsterHelp(victim, monsterHelpDialogs[victim.Monster.TypeID].caller)
	var speaker *entity.Entity
	for _, other := range allies {
		other.Monster.Target = attacker
		other.Monster.Roaming = false
		other.Monster.HelpChaseUntil = s.tick + monsterHelpChaseTicks
		other.Monster.HelpTraceDist = math.Min(2*monsterHelpRadius, math.Sqrt(sqDist(other.Pos, a.Pos))+100)
		row := monsterHelpDialogs[other.Monster.TypeID].responder
		if !spoken && row > 0 && s.monsterDialogs.RowLine(other.Monster.TypeID, row) != "" && (speaker == nil || other.ID < speaker.ID) {
			speaker = other
		}
	}
	if speaker != nil {
		s.sayMonsterHelp(speaker, monsterHelpDialogs[speaker.Monster.TypeID].responder)
	}
}
