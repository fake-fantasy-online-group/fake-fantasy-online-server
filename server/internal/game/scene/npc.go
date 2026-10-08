package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	npcChatterMinGap  = 40
	npcChatterMaxGap  = 90
	npcChatterShowSec = 5
)

// pushNpcChatter 用当前场景里的真实 NPC 实体构造完整配置。每条都显式带 NPC 名，
// 客户端会优先命中专属池，不再退回跨 NPC 的随机全局台词。
func (s *Scene) pushNpcChatter(who domain.EntityID) {
	lines := make([]event.NpcChatterLine, 0)
	for _, entity := range s.entities {
		if entity.Kind != domain.KindNPC || entity.NPC == nil || entity.Name == "" || entity.NPC.Greeting == "" {
			continue
		}
		lines = append(lines, event.NpcChatterLine{
			NPC: entity.Name, Role: entity.NPC.Role, Text: entity.NPC.Greeting,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].NPC != lines[j].NPC {
			return lines[i].NPC < lines[j].NPC
		}
		if lines[i].Text != lines[j].Text {
			return lines[i].Text < lines[j].Text
		}
		return lines[i].Role < lines[j].Role
	})
	lines = compactNpcChatter(lines)
	s.emitTo(who, event.NpcChatter{
		Who: who, Enabled: true,
		MinGap: npcChatterMinGap, MaxGap: npcChatterMaxGap, ShowSec: npcChatterShowSec,
		Lines: lines,
	})
}

func compactNpcChatter(lines []event.NpcChatterLine) []event.NpcChatterLine {
	if len(lines) < 2 {
		return lines
	}
	out := lines[:1]
	for _, line := range lines[1:] {
		last := out[len(out)-1]
		if line.NPC == last.NPC && line.Role == last.Role && line.Text == last.Text {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (s *Scene) npcGreeting(name string) string {
	for _, entity := range s.entities {
		if entity.Kind == domain.KindNPC && entity.NPC != nil && entity.Name == name {
			return entity.NPC.Greeting
		}
	}
	return ""
}
