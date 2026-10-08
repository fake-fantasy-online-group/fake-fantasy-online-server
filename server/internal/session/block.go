package session

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const maxBlockEntries = int(^uint16(0))

func (s *Session) blockStore() (store.BlockStore, bool) {
	value, ok := s.deps.Store.(store.BlockStore)
	return value, ok
}

func (s *Session) loadBlocks(ctx context.Context, charID int64) ([]domain.BlockEntry, error) {
	value, ok := s.blockStore()
	if !ok {
		return nil, nil
	}
	return value.LoadBlocks(ctx, charID)
}

func (s *Session) loadBlockSet(ctx context.Context, charID int64) (map[domain.CharID]uint8, error) {
	entries, err := s.loadBlocks(ctx, charID)
	if err != nil {
		return nil, err
	}
	set := make(map[domain.CharID]uint8, len(entries))
	for _, entry := range entries {
		set[entry.Char] = entry.Scope
	}
	return set, nil
}

func (s *Session) blocksFrom(from domain.CharID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, blocked := s.blocks[from]
	return blocked
}

func controlBlocks(control online.Control, from domain.CharID) bool {
	value, ok := control.(online.BlockControl)
	return ok && value.BlocksFrom(from)
}

func (s *Session) targetBlocks(target, from domain.CharID) bool {
	_, control, ok := s.controlFor(target)
	return ok && controlBlocks(control, from)
}

func (s *Session) refreshBlockList() {
	s.mu.Lock()
	char, inGame := s.char, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || char == nil {
		return
	}
	entries, err := s.loadBlocks(context.Background(), char.ID)
	if err != nil {
		s.log.Error("刷新屏蔽名单失败", "char", char.ID, "err", err)
		return
	}
	set := make(map[domain.CharID]uint8, len(entries))
	views := make([]protocol.BlockView, 0, len(entries))
	for _, entry := range entries {
		set[entry.Char] = entry.Scope
		views = append(views, protocol.BlockView{CharID: int64(entry.Char), Name: entry.Name, Scope: entry.Scope})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].CharID < views[j].CharID })
	s.mu.Lock()
	if s.stage == StageInGame && s.char != nil && s.char.ID == char.ID {
		s.blocks = set
	}
	s.mu.Unlock()
	if packet := protocol.BlockList(views); packet != nil {
		s.sink.Send(packet)
	}
}

func (s *Session) onBlockRequest(req protocol.Request) {
	s.mu.Lock()
	me, inGame := s.char, s.stage == StageInGame
	s.mu.Unlock()
	blocks, ok := s.blockStore()
	if !inGame || me == nil || !ok {
		return
	}
	ctx := context.Background()
	switch req.Kind {
	case protocol.ReqBlockAdd:
		name := strings.TrimSpace(req.S1)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 30 {
			s.sendGMText("屏蔽失败：角色名格式不正确")
			return
		}
		target, err := s.deps.Store.CharByName(ctx, name)
		if err != nil || target == nil {
			s.sendGMText("屏蔽失败：角色不存在")
			return
		}
		if target.ID == me.ID {
			s.sendGMText("屏蔽失败：不能屏蔽自己")
			return
		}
		s.mu.Lock()
		_, exists := s.blocks[domain.CharID(target.ID)]
		count := len(s.blocks)
		s.mu.Unlock()
		if !exists && count >= maxBlockEntries {
			s.sendGMText("屏蔽失败：名单已满")
			return
		}
		if err := blocks.UpsertBlock(ctx, me.ID, target.ID, req.U8); err != nil {
			s.log.Error("写屏蔽关系失败", "char", me.ID, "target", target.ID, "err", err)
			s.sendGMText("屏蔽失败：服务端错误")
			return
		}
		s.sendGMText("已屏蔽：" + target.Name)
	case protocol.ReqBlockDel:
		if err := blocks.RemoveBlock(ctx, me.ID, req.CharID); err != nil {
			s.log.Error("删屏蔽关系失败", "char", me.ID, "target", req.CharID, "err", err)
			s.sendGMText("取消屏蔽失败：服务端错误")
			return
		}
	}
	s.refreshBlockList()
}
