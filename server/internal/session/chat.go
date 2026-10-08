package session

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

const maxChatRunes = 120

// onChat按PostgreSQL规则路由聊天；分享先解析真实背包，喇叭先提交道具消耗。
func (s *Session) onChat(req protocol.Request) {
	now := time.Now()
	s.mu.Lock()
	stage, char, entity, sceneID := s.stage, s.char, s.entity, s.scene
	mutedUntil := s.mutedUntil
	if !mutedUntil.IsZero() && !now.Before(mutedUntil) {
		s.mutedUntil = time.Time{}
		mutedUntil = time.Time{}
	}
	s.mu.Unlock()
	if stage != StageInGame || char == nil || entity == 0 {
		return
	}
	if !mutedUntil.IsZero() {
		remaining := time.Until(mutedUntil).Round(time.Second)
		if remaining < time.Second {
			remaining = time.Second
		}
		s.sendGMText(fmt.Sprintf("你已被禁言，剩余 %s", remaining))
		return
	}
	message := strings.TrimSpace(req.S1)
	s.log.Info("收到频道聊天", "channel", req.U8, "shares", len(req.Shares),
		"runes", utf8.RuneCountInString(message))
	if message == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > maxChatRunes {
		s.sendGMText("聊天内容必须为 1..120 个字符")
		return
	}
	rule, known := s.deps.ChatRules[req.U8]
	if !known {
		s.sendGMText(fmt.Sprintf("未知聊天频道：%d", req.U8))
		return
	}
	var hornTier, hornSkin uint8
	if req.U8 == 5 {
		d, ok := s.deps.Items[domain.ItemID(req.ID32)]
		if !ok || d.HornTier == 0 || req.Flag != d.HornSkin {
			s.sendGMText("喇叭或外观无效")
			return
		}
		hornTier, hornSkin = d.HornTier, d.HornSkin
	} else if req.ID32 != 0 || req.Flag != 0 {
		s.sendGMText("普通聊天不能携带喇叭参数")
		return
	}
	if s.deps.Online == nil {
		return
	}
	var resolvedShares []domain.ChatShare
	if len(req.Shares) > int(rule.MaxShares) {
		s.sendGMText("聊天分享数量超过频道上限")
		return
	}
	if len(req.Shares) > 0 {
		refs := make([]scene.ChatShareRef, 0, len(req.Shares))
		for _, ref := range req.Shares {
			refs = append(refs, scene.ChatShareRef{Kind: ref.Kind, Tab: ref.Tab, Slot: int32(clientBagSlotIfBag(ref.Kind, ref.Tab, int(ref.Slot)))})
		}
		reply := make(chan scene.ChatShareResult, 1)
		if !s.deps.Router.Post(sceneID, scene.ResolveChatShares{ID: entity, Refs: refs, Reply: reply}) {
			s.sendGMText("分享物品失败：角色场景不可用")
			return
		}
		select {
		case result := <-reply:
			if result.Reason != "" {
				s.sendGMText("分享物品失败：" + result.Reason)
				return
			}
			resolvedShares = result.Shares
		case <-time.After(2 * time.Second):
			s.sendGMText("分享物品超时，请重试")
			return
		}
	}
	s.mu.Lock()
	if s.lastChat == nil {
		s.lastChat = make(map[uint8]time.Time)
	}
	last := s.lastChat[req.U8]
	cooldown := time.Duration(rule.CooldownMS) * time.Millisecond
	if cooldown > 0 && now.Sub(last) < cooldown {
		remaining := (cooldown - now.Sub(last)).Round(100 * time.Millisecond)
		s.mu.Unlock()
		s.sendGMText(fmt.Sprintf("发言过快，请等待 %s", remaining))
		return
	}
	s.lastChat[req.U8] = now
	s.mu.Unlock()

	if hornTier > 0 {
		if err := s.consumeHorn(domain.ItemID(req.ID32), hornSkin); err != nil {
			s.sendGMText(err.Error())
			return
		}
	}
	chat := online.ChatMessage{
		Sender: entity, Name: char.Name, HornTier: hornTier, HornSkin: hornSkin,
		// 0x8039 的该字节只被证明是气泡主题选择器；协议证据明确不能把它
		// 解释为性别。领域尚无气泡主题状态时使用中性主题 0，不能拿外观字段代填。
		Theme:   0,
		Channel: req.U8, Text: message, Shares: resolvedShares,
	}
	var partyMembers map[domain.CharID]struct{}
	var familyMembers map[domain.CharID]struct{}
	if rule.Scope == domain.ChatScopeParty {
		if s.deps.Party == nil {
			s.sendGMText("队伍频道暂不可用")
			return
		}
		partyID := s.deps.Party.PartyOf(domain.CharID(char.ID))
		partyState, ok := s.deps.Party.Get(partyID)
		if partyID == 0 || !ok {
			s.sendGMText("你当前没有队伍")
			return
		}
		partyMembers = make(map[domain.CharID]struct{}, len(partyState.Members))
		for _, member := range partyState.Members {
			partyMembers[member.Char] = struct{}{}
		}
	}
	if rule.Scope == domain.ChatScopeFamily {
		familyStore, ok := s.familyStore()
		if !ok {
			s.sendGMText("家族频道暂不可用")
			return
		}
		members, err := familyStore.FamilyMemberIDs(context.Background(), char.ID)
		if err != nil || len(members) == 0 {
			s.sendGMText("你当前没有家族")
			return
		}
		familyMembers = make(map[domain.CharID]struct{}, len(members))
		for _, member := range members {
			familyMembers[member] = struct{}{}
		}
	}
	for _, recipient := range s.deps.Online.Recipients() {
		switch rule.Scope {
		case domain.ChatScopeMap:
			if recipient.Location.Scene != sceneID {
				continue
			}
		case domain.ChatScopeParty:
			if _, ok := partyMembers[recipient.Location.Char]; !ok {
				continue
			}
		case domain.ChatScopeWorld:
		case domain.ChatScopeFamily:
			if _, ok := familyMembers[recipient.Location.Char]; !ok {
				continue
			}
		default:
			continue
		}
		if controlBlocks(recipient.Control, domain.CharID(char.ID)) {
			continue
		}
		if delivery, ok := recipient.Control.(online.ChatControl); ok {
			delivery.DeliverChat(chat)
		}
	}
}
