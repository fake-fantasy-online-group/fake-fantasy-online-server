package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const (
	gmReportListMax   = 30
	gmReportRecentMax = 10
)

// executeGMReport handles /reportallow, /reportdeny, /reportlist and /reports.
// The whitelist lives only in memory: changes apply to the next /report of an
// online player immediately and are cleared on restart.
func (s *Session) executeGMReport(parsed parsedGMCommand, account *store.Account) (bool, string) {
	logs := s.deps.ClientLogs
	if logs == nil {
		return false, "服务器未启用日志上报（需 -client-log-dir）"
	}
	if parsed.admin == gmAdminReportList {
		list := logs.List()
		if len(list) == 0 {
			return true, "日志上报白名单为空（重启后需重新添加）"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "日志上报白名单 %d 个账号：", len(list))
		for i, e := range list {
			if i == gmReportListMax {
				fmt.Fprintf(&b, "\n…另有 %d 个", len(list)-i)
				break
			}
			fmt.Fprintf(&b, "\n账号%d %s 由%s %s", e.AccountID, e.CharName, e.By, e.At.Format("01-02 15:04"))
			if e.Note != "" {
				b.WriteString(" 备注:" + e.Note)
			}
		}
		return true, b.String()
	}
	accountID, name, msg := s.resolveGMCharAccount(parsed.target)
	if accountID == 0 {
		return false, msg
	}
	switch parsed.admin {
	case gmAdminReportAllow:
		by := ""
		if account != nil {
			by = account.Username
		}
		if logs.Allow(ClientLogAllow{AccountID: accountID, CharName: name, Note: parsed.text, By: by, At: time.Now()}) {
			return true, fmt.Sprintf("已开通日志上报：%s（账号%d），玩家下次 /report 即生效", name, accountID)
		}
		return true, fmt.Sprintf("已更新日志上报白名单：%s（账号%d）", name, accountID)
	case gmAdminReportDeny:
		if logs.Deny(accountID) {
			return true, fmt.Sprintf("已关闭日志上报：%s（账号%d）", name, accountID)
		}
		return false, fmt.Sprintf("该账号未开通日志上报：%s（账号%d）", name, accountID)
	case gmAdminReports:
		ctx, cancel := context.WithTimeout(context.Background(), gmCommandTimeout)
		defer cancel()
		reports, err := logs.Recent(ctx, accountID, gmReportRecentMax)
		if err != nil {
			s.log.Error("GM 查询客户端日志失败", "accountID", accountID, "err", err)
			return false, "读取日志上报记录失败"
		}
		if len(reports) == 0 {
			return true, fmt.Sprintf("%s（账号%d）没有14天内的日志上报", name, accountID)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s（账号%d）最近 %d 条日志上报：", name, accountID, len(reports))
		for _, r := range reports {
			fmt.Fprintf(&b, "\n%s %s %s %s %d分钟 %dKB", r.ReportNo, r.CreatedAt.Local().Format("01-02 15:04"),
				r.CharName, r.Level, r.RangeMinutes, (r.SizeBytes+1023)/1024)
			if r.Truncated {
				b.WriteString(" 已截断")
			}
			if r.Note != "" {
				b.WriteString(" " + r.Note)
			}
		}
		return true, b.String()
	}
	return false, "未登记的 GM 管理动作"
}

// resolveGMCharAccount maps a character name to its account: online index
// first, then storage (offline characters can be whitelisted too).
func (s *Session) resolveGMCharAccount(name string) (int64, string, string) {
	if s.deps.Online != nil {
		if loc, _, ok := s.deps.Online.ControlByName(name); ok && loc.Account > 0 {
			return loc.Account, loc.Name, ""
		}
	}
	if s.deps.Store == nil {
		return 0, "", "目标角色不在线：" + name
	}
	ctx, cancel := context.WithTimeout(context.Background(), gmCommandTimeout)
	defer cancel()
	char, err := s.deps.Store.CharByName(ctx, name)
	if errors.Is(err, domain.ErrCharNotFound) || (err == nil && char == nil) {
		return 0, "", "目标角色不存在：" + name
	}
	if err != nil {
		s.log.Error("GM 查询角色失败", "target", name, "err", err)
		return 0, "", "读取目标角色失败"
	}
	return char.AccountID, char.Name, ""
}
