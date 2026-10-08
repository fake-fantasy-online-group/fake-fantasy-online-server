package session

import (
	"context"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const (
	accountResultLock     uint8 = 1
	accountResultUnlock   uint8 = 2
	accountResultPassword uint8 = 3
	accountResultSecurity uint8 = 4
	accountResultOK       uint8 = 0
	accountResultFailed   uint8 = 1
)

func (s *Session) accountForSecurity() (*store.Account, bool) {
	s.mu.Lock()
	if s.account == nil || (s.stage != StageAuthed && s.stage != StageInGame) {
		s.mu.Unlock()
		return nil, false
	}
	cached := *s.account
	s.mu.Unlock()
	current, err := s.deps.Store.AccountByName(context.Background(), cached.Username)
	if err != nil {
		s.log.Error("复核账号凭据失败", "acc", cached.Username, "err", err)
		return nil, false
	}
	if current == nil || current.ID != cached.ID || current.PassHash != cached.PassHash ||
		current.SecHash != cached.SecHash || current.Banned || current.BannedUntil.After(time.Now()) {
		s.revokeAuthentication("账号凭据或封禁状态已变化")
		return nil, false
	}
	s.mu.Lock()
	valid := s.account != nil && s.account.ID == cached.ID &&
		(s.stage == StageAuthed || s.stage == StageInGame)
	s.mu.Unlock()
	return current, valid
}

func validAccountSecret(value string, maxBytes int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= maxBytes && !hasControl(value)
}

func (s *Session) sendAccountResult(kind, code uint8, message string) {
	s.sink.Send(protocol.AccountSecurityResult(kind, code, message, 0, 0, 0))
}

func (s *Session) onChangePassword(req protocol.Request) {
	account, ok := s.accountForSecurity()
	if !ok {
		s.sendAccountResult(accountResultPassword, accountResultFailed, "账号尚未登录")
		return
	}
	if !validAccountSecret(req.S1, maxPasswordBytes) || !validAccountSecret(req.S2, maxPasswordBytes) {
		s.sendAccountResult(accountResultPassword, accountResultFailed, "旧密码或新密码格式无效")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PassHash), []byte(req.S1)) != nil {
		s.sendAccountResult(accountResultPassword, accountResultFailed, "旧密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.S2), bcrypt.DefaultCost)
	updated := false
	if err == nil {
		updated, err = s.deps.Store.CompareAndSwapAccountCredentials(context.Background(), account.ID,
			account.PassHash, account.SecHash, string(hash), account.SecHash)
	}
	if err != nil {
		s.log.Error("修改账号密码失败", "acc", account.Username, "err", err)
		s.sendAccountResult(accountResultPassword, accountResultFailed, "服务器错误")
		return
	}
	if !updated {
		s.sendAccountResult(accountResultPassword, accountResultFailed, "账号凭据已变化，请重新登录")
		s.revokeAuthentication("修改密码时账号凭据已变化")
		return
	}
	s.mu.Lock()
	if s.account != nil && s.account.ID == account.ID {
		s.account.PassHash = string(hash)
	}
	s.mu.Unlock()
	s.sendAccountResult(accountResultPassword, accountResultOK, "密码修改成功")
}

func (s *Session) onChangeSecurityCode(req protocol.Request) {
	account, ok := s.accountForSecurity()
	if !ok {
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "账号尚未登录")
		return
	}
	if !validAccountSecret(req.S1, maxPasswordBytes) ||
		(account.SecHash != "" && !validAccountSecret(req.S2, maxSecurityBytes)) ||
		!validAccountSecret(req.S3, maxSecurityBytes) {
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "密码或安全码格式无效")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PassHash), []byte(req.S1)) != nil {
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "账号密码错误")
		return
	}
	if account.SecHash != "" && bcrypt.CompareHashAndPassword([]byte(account.SecHash), []byte(req.S2)) != nil {
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "旧安全码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.S3), bcrypt.DefaultCost)
	updated := false
	if err == nil {
		updated, err = s.deps.Store.CompareAndSwapAccountCredentials(context.Background(), account.ID,
			account.PassHash, account.SecHash, account.PassHash, string(hash))
	}
	if err != nil {
		s.log.Error("修改账号安全码失败", "acc", account.Username, "err", err)
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "服务器错误")
		return
	}
	if !updated {
		s.sendAccountResult(accountResultSecurity, accountResultFailed, "账号凭据已变化，请重新登录")
		s.revokeAuthentication("修改安全码时账号凭据已变化")
		return
	}
	s.mu.Lock()
	if s.account != nil && s.account.ID == account.ID {
		s.account.SecHash = string(hash)
	}
	s.mu.Unlock()
	s.sendAccountResult(accountResultSecurity, accountResultOK, "安全码修改成功")
	s.sink.Send(protocol.AccountSecuritySeed(true, 0))
}

func (s *Session) onSetItemLock(req protocol.Request) {
	account, ok := s.accountForSecurity()
	kind := accountResultUnlock
	if req.Flag == 1 {
		kind = accountResultLock
	}
	fail := func(message string) {
		s.sink.Send(protocol.AccountSecurityResult(kind, accountResultFailed, message,
			req.Tab, req.Slot, domain.ItemID(req.ID32)))
	}
	if !ok {
		fail("账号尚未登录")
		return
	}
	s.mu.Lock()
	stage := s.stage
	s.mu.Unlock()
	if stage != StageInGame {
		fail("角色尚未进入游戏")
		return
	}
	if req.Flag == 0 {
		if account.SecHash == "" {
			fail("账号尚未设置安全码")
			return
		}
		if !validAccountSecret(req.S1, maxSecurityBytes) ||
			bcrypt.CompareHashAndPassword([]byte(account.SecHash), []byte(req.S1)) != nil {
			fail("安全码错误")
			return
		}
	}
	s.postToScene(scene.SetItemLock{ID: s.entityID(), Tab: req.Tab, Slot: int32(clientBagSlot(req.Tab, int(req.Slot))),
		Item: domain.ItemID(req.ID32), On: req.Flag == 1})
}
