package session

import (
	"context"
	"errors"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func (s *Session) onRenameCharacter(newName string) {
	s.mu.Lock()
	ch, entityID, sceneID, inGame := s.char, s.entity, s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || ch == nil || entityID == 0 {
		return
	}
	if err := domain.ValidateCharacterName(newName); err != nil {
		s.sink.Send(protocol.RenameCharacterResult(10, createCharacterMessage(err), newName))
		return
	}
	if newName == ch.Name {
		s.sink.Send(protocol.RenameCharacterResult(0, "角色名未改变", newName))
		return
	}
	renamer, ok := s.deps.Store.(store.CharacterRenamer)
	if !ok {
		s.log.Error("存储未实现角色改名")
		s.sink.Send(protocol.RenameCharacterResult(1, "服务器不支持角色改名", newName))
		return
	}
	if err := renamer.RenameCharacter(context.Background(), ch.ID, newName); err != nil {
		code, message := int32(1), "角色改名失败"
		if errors.Is(err, domain.ErrNameTaken) {
			code, message = 10, "角色名已被占用"
		}
		s.sink.Send(protocol.RenameCharacterResult(code, message, newName))
		return
	}

	done := make(chan bool, 1)
	posted := s.deps.Router != nil && s.deps.Router.PostLifecycle(sceneID, scene.RenamePlayer{
		ID: entityID, Name: newName, Done: done,
	})
	if !posted {
		s.log.Error("改名已落库但场景同步失败", "char", ch.ID, "name", newName)
		s.sink.Send(protocol.RenameCharacterResult(0, "改名成功，请重新登录", newName))
		s.sink.Close()
		return
	}
	select {
	case ok := <-done:
		if !ok {
			s.log.Error("改名已落库但场景实体不存在", "char", ch.ID, "entity", entityID)
			s.sink.Send(protocol.RenameCharacterResult(0, "改名成功，请重新登录", newName))
			s.sink.Close()
			return
		}
	case <-time.After(3 * time.Second):
		s.log.Error("改名场景同步超时", "char", ch.ID, "entity", entityID)
		s.sink.Send(protocol.RenameCharacterResult(0, "改名成功，请重新登录", newName))
		s.sink.Close()
		return
	}

	charID := domain.CharID(ch.ID)
	if s.deps.Online != nil {
		if loc, found := s.deps.Online.Find(charID); found && loc.Entity == entityID {
			loc.Name = newName
			s.deps.Online.Enter(loc)
		}
	}
	if s.deps.Party != nil {
		s.deps.Party.Rename(charID, newName)
		if pid := s.deps.Party.PartyOf(charID); pid != 0 {
			if p, found := s.deps.Party.Get(pid); found {
				for _, member := range p.Members {
					s.pushPartyRefresh(member.Char)
				}
			}
		}
	}
	if s.deps.Requests != nil {
		s.deps.Requests.Rename(charID, newName)
	}
	if s.deps.Online != nil {
		for _, recipient := range s.deps.Online.Recipients() {
			if delivery, ok := recipient.Control.(online.SocialControl); ok {
				delivery.RefreshFriends()
			}
		}
	}
	s.sink.Send(protocol.RenameCharacterResult(0, "角色改名成功", newName))
}
