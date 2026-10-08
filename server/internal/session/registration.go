package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const (
	maxAccountBytes  = 36
	maxPasswordBytes = 72 // bcrypt 只处理前 72 字节；超长必须明确拒绝，不能静默截断。
	maxSecurityBytes = 72
	maxInviteBytes   = 64
)

// onRegister 是正式客户端 0x105d 的账号创建入口。注册只落账号，不改变当前
// Session 的认证阶段；客户端收到 0x8009 成功结果后会返回登录流程。
func (s *Session) onRegister(req protocol.Request) {
	s.mu.Lock()
	stage := s.stage
	s.mu.Unlock()
	if stage != StageConnected {
		s.sink.Send(protocol.RegisterResult(1, "当前连接已经登录"))
		return
	}
	if message := validateRegistration(req.S1, req.S2, req.S3, req.S4); message != "" {
		s.sink.Send(protocol.RegisterResult(1, message))
		return
	}

	passHash, err := bcrypt.GenerateFromPassword([]byte(req.S2), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("生成注册密码哈希失败", "acc", req.S1, "err", err)
		s.sink.Send(protocol.RegisterResult(1, "服务器错误"))
		return
	}
	secHash, err := bcrypt.GenerateFromPassword([]byte(req.S3), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("生成安全码哈希失败", "acc", req.S1, "err", err)
		s.sink.Send(protocol.RegisterResult(1, "服务器错误"))
		return
	}

	_, err = s.deps.Store.CreateRegisteredAccount(
		context.Background(), req.S1, string(passHash), string(secHash),
	)
	if errors.Is(err, store.ErrDup) {
		s.sink.Send(protocol.RegisterResult(1, "账号已存在"))
		return
	}
	if err != nil {
		s.log.Error("注册账号落库失败", "acc", req.S1, "err", err)
		s.sink.Send(protocol.RegisterResult(1, "服务器错误"))
		return
	}

	s.log.Info("账号注册成功", "acc", req.S1)
	s.sink.Send(protocol.RegisterResult(0, "注册成功，请登录"))
}

// onResetPassword 处理标题页 0x105e。账号不存在与安全码错误使用同一消息，
// 避免把账号枚举能力暴露给未认证连接。
func (s *Session) onResetPassword(req protocol.Request) {
	s.mu.Lock()
	stage := s.stage
	s.mu.Unlock()
	if stage != StageConnected {
		s.sink.Send(protocol.ResetPasswordResult(1, "当前连接已经登录"))
		return
	}
	if req.S1 == "" || !utf8.ValidString(req.S1) || len(req.S1) > maxAccountBytes ||
		req.S2 == "" || !utf8.ValidString(req.S2) || len(req.S2) > maxSecurityBytes || hasControl(req.S2) ||
		req.S3 == "" || !utf8.ValidString(req.S3) || len(req.S3) > maxPasswordBytes || hasControl(req.S3) {
		s.sink.Send(protocol.ResetPasswordResult(1, "账号、安全码或新密码格式无效"))
		return
	}
	ctx := context.Background()
	account, err := s.deps.Store.AccountByName(ctx, req.S1)
	if err != nil {
		s.log.Error("重置密码查询账号失败", "acc", req.S1, "err", err)
		s.sink.Send(protocol.ResetPasswordResult(1, "服务器错误"))
		return
	}
	if account == nil || bcrypt.CompareHashAndPassword([]byte(account.SecHash), []byte(req.S2)) != nil {
		s.sink.Send(protocol.ResetPasswordResult(1, "账号或安全码错误"))
		return
	}
	passHash, err := bcrypt.GenerateFromPassword([]byte(req.S3), bcrypt.DefaultCost)
	updated := false
	if err == nil {
		updated, err = s.deps.Store.CompareAndSwapAccountCredentials(ctx, account.ID,
			account.PassHash, account.SecHash, string(passHash), account.SecHash)
	}
	if err != nil {
		s.log.Error("重置密码落库失败", "acc", req.S1, "err", err)
		s.sink.Send(protocol.ResetPasswordResult(1, "服务器错误"))
		return
	}
	if !updated {
		s.sink.Send(protocol.ResetPasswordResult(1, "账号凭据已变化，请重试"))
		return
	}
	if s.deps.AccountSessions != nil {
		s.deps.AccountSessions.RevokeCredentials(account.ID, account.PassHash, "密码已重置，请重新登录")
	} else {
		s.revokeAccountSessions(account.ID, "密码已重置，请重新登录")
	}
	s.sink.Send(protocol.ResetPasswordResult(0, "密码重置成功"))
}

func validateRegistration(account, password, security, invite string) string {
	if account == "" {
		return "请输入账号"
	}
	if !utf8.ValidString(account) || len(account) > maxAccountBytes ||
		strings.TrimSpace(account) != account || hasSpaceOrControl(account) {
		return fmt.Sprintf("账号须为 1-%d 字节且不能包含空白字符", maxAccountBytes)
	}
	if password == "" {
		return "请输入密码"
	}
	if !utf8.ValidString(password) || len(password) > maxPasswordBytes || hasControl(password) {
		return fmt.Sprintf("密码不能超过 %d 字节且不能包含控制字符", maxPasswordBytes)
	}
	if security == "" {
		return "请输入安全码"
	}
	if !utf8.ValidString(security) || len(security) > maxSecurityBytes || hasControl(security) {
		return fmt.Sprintf("安全码不能超过 %d 字节且不能包含控制字符", maxSecurityBytes)
	}
	if !utf8.ValidString(invite) || len(invite) > maxInviteBytes || hasControl(invite) {
		return fmt.Sprintf("邀请码不能超过 %d 字节且不能包含控制字符", maxInviteBytes)
	}
	return ""
}

func hasSpaceOrControl(value string) bool {
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// checkPassword 同时承担旧开发账号的无停机迁移：历史 pass_hash 为空时，首次
// 使用非空密码登录会原地写入 bcrypt；已有合法 bcrypt 的账号必须严格校验。
// 非空但不是 bcrypt 的值视为损坏数据并严格失败，不能留下空密码旁路。
func (s *Session) checkPassword(ctx context.Context, account *store.Account, password string) (bool, error) {
	if _, err := bcrypt.Cost([]byte(account.PassHash)); err == nil {
		return bcrypt.CompareHashAndPassword([]byte(account.PassHash), []byte(password)) == nil, nil
	}
	if account.PassHash != "" {
		return false, fmt.Errorf("账号 %d 的密码哈希格式无效", account.ID)
	}
	if password == "" {
		return false, nil
	}
	passHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	updated, err := s.deps.Store.CompareAndSwapAccountCredentials(ctx, account.ID,
		account.PassHash, account.SecHash, string(passHash), account.SecHash)
	if err != nil {
		return false, err
	}
	if !updated {
		return false, nil
	}
	account.PassHash = string(passHash)
	s.log.Info("旧账号已升级密码哈希", "acc", account.Username)
	return true, nil
}
