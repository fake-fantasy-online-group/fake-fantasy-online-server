package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const (
	maxGMCommandBytes = 512
	gmCommandTimeout  = 3 * time.Second
)

const gmHelpText = "GM命令：\n" +
	"/gm help\n" +
	"/effect <技能ID> [self|pet]\n/ownersync\n" +
	"/addexp <经验>\n" +
	"/addmoney <铜币>\n" +
	"/addgold <金币>\n" +
	"/addsilver <银币>\n" +
	"/addhonor <名誉>\n" +
	"/item <物品ID> [数量]\n" +
	"/takemoney <铜币>\n" +
	"/takegold <金币>\n" +
	"/takesilver <银币>\n" +
	"/takehonor <名誉>\n" +
	"/takeitem <物品ID> [数量]\n" +
	"/max\n" +
	"/god [on|off|status]\n" +
	"/oneshot [on|off|status]\n" +
	"/goto <地图ID> [X Y]\n" +
	"/where\n" +
	"/online\n" +
	"/inspect <角色名> (WIZARD)\n" +
	"/givemoney <角色名> <铜币> (WIZARD)\n" +
	"/giveitem <角色名> <物品ID> [数量] (WIZARD)\n" +
	"/mute <角色名> <分钟> (WIZARD)\n" +
	"/unmute <角色名> (WIZARD)\n" +
	"/announce <内容> (WIZARD)\n" +
	"/kick <角色名> (ARCH)\n" +
	"/ban <角色名> [分钟] (ADMIN)\n" +
	"/unban <角色名> (ADMIN)"

type gmAdminAction uint8

const (
	gmAdminNone gmAdminAction = iota
	gmAdminAnnounce
	gmAdminKick
	gmAdminBan
	gmAdminUnban
	gmAdminMute
	gmAdminUnmute
)

type parsedGMCommand struct {
	skill        domain.SkillID
	effectTarget uint8
	name         string
	action       scene.GMAction
	amount       int64
	item         domain.ItemID
	count        int32
	to           domain.SceneID
	at           domain.Pos
	help         bool
	online       bool
	minimum      store.GMLevel
	admin        gmAdminAction
	text         string
	target       string
	remote       bool
	duration     time.Duration
	state        scene.GMStateMode
}

// onGMCommand 只认白名单。0x1006 还承载 pk/arena/trial 等普通玩法菜单命令；
// 不在白名单里的文本保持原先的“尚未接线”语义，不能因为接了 GM 就误伤它们。
func (s *Session) onGMCommand(raw string) {
	switch raw {
	case "trialtask", "trialgo", "trialleave", "trialclaim", "trial":
		s.postToScene(scene.TrialAction{ID: s.entityID(), Action: raw})
		return
	}
	if len(raw) == 0 || len(raw) > maxGMCommandBytes {
		s.log.Warn("GM/菜单命令长度非法", "bytes", len(raw))
		return
	}

	parsed, recognized, prefixed, err := s.parseGMCommand(raw)
	s.mu.Lock()
	account, char, stage, id, sc := s.account, s.char, s.stage, s.entity, s.scene
	s.mu.Unlock()
	if !recognized {
		// 只有显式写了 /gm 前缀的未知命令才回提示。直接文本可能属于普通菜单，
		// 给它回“GM 命令错误”会改变现有客户端行为。
		if prefixed && account != nil && account.GMLevel >= store.GMApp {
			s.sendGMText("未知 GM 命令，请输入 /gm help")
		}
		s.log.Debug("0x1006 命令未由 GM 白名单接管", "command", firstField(raw))
		return
	}

	if account == nil || account.GMLevel < store.GMApp {
		s.sendGMText("权限不足：当前账号不是 GM")
		s.auditGM(account, char, parsed.name, raw, false, "permission denied")
		return
	}
	if account.GMLevel < parsed.minimum {
		message := fmt.Sprintf("权限不足：/%s 需要 %s 权限", parsed.name, parsed.minimum.String())
		s.sendGMText(message)
		s.auditGM(account, char, parsed.name, raw, false, message)
		return
	}
	if stage != StageInGame || char == nil || id == 0 {
		s.sendGMText("GM 命令只能在角色进入游戏后使用")
		s.auditGM(account, char, parsed.name, raw, false, "not in game")
		return
	}
	if err != nil {
		s.sendGMText("参数错误：" + err.Error())
		s.auditGM(account, char, parsed.name, raw, false, err.Error())
		return
	}
	if parsed.help {
		s.sendGMText(gmHelpText)
		s.auditGM(account, char, parsed.name, raw, true, "help")
		return
	}
	if parsed.online {
		s.sendGMText(formatOnline(s.deps.Online))
		s.auditGM(account, char, parsed.name, raw, true, "online snapshot")
		return
	}
	if parsed.admin != gmAdminNone {
		ok, message := s.executeGMAdmin(parsed, account, char)
		s.sendGMText(message)
		s.auditGM(account, char, parsed.name, raw, ok, message)
		return
	}
	if s.deps.Router == nil {
		s.sendGMText("服务器场景路由不可用")
		s.auditGM(account, char, parsed.name, raw, false, "router unavailable")
		return
	}
	targetID, targetScene := id, sc
	if parsed.remote {
		if s.deps.Online == nil {
			s.sendGMText("在线索引不可用")
			s.auditGM(account, char, parsed.name, raw, false, "online registry unavailable")
			return
		}
		loc, ok := s.deps.Online.FindByName(parsed.target)
		if !ok {
			message := "目标角色不在线：" + parsed.target
			s.sendGMText(message)
			s.auditGM(account, char, parsed.name, raw, false, message)
			return
		}
		targetID, targetScene = loc.Entity, loc.Scene
	}

	replies := make(chan scene.GMResult, 1)
	cmd := scene.GMCommand{
		ID: targetID, Action: parsed.action, Amount: parsed.amount,
		Item: parsed.item, Count: parsed.count, To: parsed.to, At: parsed.at,
		State: parsed.state, Skill: parsed.skill, EffectTarget: parsed.effectTarget,
		Reply: replies,
	}
	if !s.deps.Router.Post(targetScene, cmd) {
		s.sendGMText("GM 命令投递失败")
		s.auditGM(account, char, parsed.name, raw, false, "scene post failed")
		return
	}

	select {
	case result := <-replies:
		s.sendGMText(result.Message)
		s.auditGM(account, char, parsed.name, raw, result.OK, result.Message)
	case <-time.After(gmCommandTimeout):
		s.sendGMText("GM 命令执行超时")
		s.auditGM(account, char, parsed.name, raw, false, "timeout")
	}
}

func (s *Session) parseGMCommand(raw string) (parsedGMCommand, bool, bool, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/"))
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return parsedGMCommand{}, false, false, nil
	}
	prefixed := strings.EqualFold(fields[0], "gm")
	if prefixed {
		fields = fields[1:]
		if len(fields) == 0 {
			fields = []string{"help"}
		}
	}
	name := strings.ToLower(fields[0])
	args := fields[1:]
	parsed := parsedGMCommand{name: name, count: 1, minimum: store.GMApp}

	switch name {
	case "help", "gmhelp":
		if len(args) != 0 {
			return parsed, true, prefixed, fmt.Errorf("用法：/gm help")
		}
		parsed.help = true
		return parsed, true, prefixed, nil
	case "effect":
		if len(args) < 1 || len(args) > 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/effect <技能ID> [self|pet]")
		}
		id, err := parsePositive(args[0], 1<<31-1)
		if err != nil {
			return parsed, true, prefixed, err
		}
		parsed.action, parsed.skill = scene.GMPlayEffect, domain.SkillID(id)
		if len(args) == 2 {
			if args[1] == "pet" {
				parsed.effectTarget = 1
			} else if args[1] != "self" {
				return parsed, true, prefixed, fmt.Errorf("目标必须为self或pet")
			}
		}
		return parsed, true, prefixed, nil
	case "ownersync":
		if len(args) != 0 {
			return parsed, true, prefixed, fmt.Errorf("用法：/ownersync")
		}
		parsed.action = scene.GMOwnerSync
		return parsed, true, prefixed, nil
	case "addexp":
		value, err := onePositiveInt64(args, 1_000_000_000, "/addexp <经验>")
		parsed.action, parsed.amount = scene.GMAddExp, value
		return parsed, true, prefixed, err
	case "addmoney":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/addmoney <铜币>")
		parsed.action, parsed.amount = scene.GMAddMoney, value
		return parsed, true, prefixed, err
	case "addgold":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/addgold <金币>")
		parsed.action, parsed.amount = scene.GMAddGold, value
		return parsed, true, prefixed, err
	case "addsilver":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/addsilver <银币>")
		parsed.action, parsed.amount = scene.GMAddSilver, value
		return parsed, true, prefixed, err
	case "addhonor":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/addhonor <名誉>")
		parsed.action, parsed.amount = scene.GMAddHonor, value
		return parsed, true, prefixed, err
	case "takemoney":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/takemoney <铜币>")
		parsed.action, parsed.amount = scene.GMTakeMoney, value
		return parsed, true, prefixed, err
	case "takegold":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/takegold <金币>")
		parsed.action, parsed.amount = scene.GMTakeGold, value
		return parsed, true, prefixed, err
	case "takesilver":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/takesilver <银币>")
		parsed.action, parsed.amount = scene.GMTakeSilver, value
		return parsed, true, prefixed, err
	case "takehonor":
		value, err := onePositiveInt64(args, 1_000_000_000_000, "/takehonor <名誉>")
		parsed.action, parsed.amount = scene.GMTakeHonor, value
		return parsed, true, prefixed, err
	case "item":
		parsed.action = scene.GMAddItem
		if len(args) < 1 || len(args) > 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/item <物品ID> [数量]")
		}
		item, err := parsePositive(args[0], 1<<31-1)
		if err != nil {
			return parsed, true, prefixed, fmt.Errorf("物品ID必须是正整数")
		}
		parsed.item = domain.ItemID(item)
		if len(args) == 2 {
			count, countErr := parsePositive(args[1], 10_000)
			if countErr != nil {
				return parsed, true, prefixed, fmt.Errorf("数量必须在 1..10000")
			}
			parsed.count = int32(count)
		}
		return parsed, true, prefixed, nil
	case "takeitem":
		parsed.action = scene.GMTakeItem
		if len(args) < 1 || len(args) > 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/takeitem <物品ID> [数量]")
		}
		item, err := parsePositive(args[0], 1<<31-1)
		if err != nil {
			return parsed, true, prefixed, fmt.Errorf("物品ID必须是正整数")
		}
		parsed.item = domain.ItemID(item)
		if len(args) == 2 {
			count, countErr := parsePositive(args[1], 10_000)
			if countErr != nil {
				return parsed, true, prefixed, fmt.Errorf("数量必须在 1..10000")
			}
			parsed.count = int32(count)
		}
		return parsed, true, prefixed, nil
	case "max":
		parsed.action = scene.GMMaxResources
		if len(args) != 0 {
			return parsed, true, prefixed, fmt.Errorf("用法：/max")
		}
		return parsed, true, prefixed, nil
	case "god", "oneshot":
		if name == "god" {
			parsed.action = scene.GMGod
		} else {
			parsed.action = scene.GMOneShot
		}
		state, stateErr := parseGMState(args, "/"+name+" [on|off|status]")
		parsed.state = state
		return parsed, true, prefixed, stateErr
	case "goto", "teleport":
		parsed.action = scene.GMTeleport
		if len(args) != 1 && len(args) != 3 {
			return parsed, true, prefixed, fmt.Errorf("用法：/goto <地图ID> [X Y]")
		}
		mapID, mapErr := parsePositive(args[0], 1<<31-1)
		if mapErr != nil {
			return parsed, true, prefixed, fmt.Errorf("地图ID必须是正整数")
		}
		if s.deps.GMLanding == nil {
			return parsed, true, prefixed, fmt.Errorf("服务器未配置 GM 地图落点")
		}
		landing, ok := s.deps.GMLanding(int32(mapID))
		if !ok {
			return parsed, true, prefixed, fmt.Errorf("地图 %d 没有已验证的安全落点", mapID)
		}
		if len(args) == 3 {
			x, xErr := strconv.ParseInt(args[1], 10, 32)
			y, yErr := strconv.ParseInt(args[2], 10, 32)
			if xErr != nil || yErr != nil || x < 0 || y < 0 {
				return parsed, true, prefixed, fmt.Errorf("X/Y 必须是非负整数")
			}
			landing.X, landing.Y = float64(x), float64(y)
		}
		landing.MapID = int32(mapID)
		parsed.to, parsed.at = domain.SceneID{MapID: int32(mapID)}, landing
		return parsed, true, prefixed, nil
	case "where":
		parsed.action = scene.GMWhere
		if len(args) != 0 {
			return parsed, true, prefixed, fmt.Errorf("用法：/where")
		}
		return parsed, true, prefixed, nil
	case "online":
		if len(args) != 0 {
			return parsed, true, prefixed, fmt.Errorf("用法：/online")
		}
		parsed.online = true
		return parsed, true, prefixed, nil
	case "inspect":
		parsed.action, parsed.minimum, parsed.remote = scene.GMInspect, store.GMWizard, true
		parsed.target = strings.TrimSpace(strings.Join(args, " "))
		if parsed.target == "" {
			return parsed, true, prefixed, fmt.Errorf("用法：/inspect <角色名>")
		}
		return parsed, true, prefixed, nil
	case "givemoney":
		parsed.action, parsed.minimum, parsed.remote = scene.GMAddMoney, store.GMWizard, true
		if len(args) < 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/givemoney <角色名> <铜币>")
		}
		value, valueErr := parsePositive(args[len(args)-1], 1_000_000_000_000)
		if valueErr != nil {
			return parsed, true, prefixed, fmt.Errorf("铜币必须在 1..1000000000000")
		}
		parsed.target = strings.TrimSpace(strings.Join(args[:len(args)-1], " "))
		parsed.amount = value
		return parsed, true, prefixed, nil
	case "giveitem":
		parsed.action, parsed.minimum, parsed.remote = scene.GMAddItem, store.GMWizard, true
		if len(args) < 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/giveitem <角色名> <物品ID> [数量]")
		}
		itemAt := len(args) - 1
		if len(args) >= 3 {
			if count, countErr := parsePositive(args[len(args)-1], 10_000); countErr == nil {
				if _, itemErr := parsePositive(args[len(args)-2], 1<<31-1); itemErr == nil {
					parsed.count = int32(count)
					itemAt--
				}
			}
		}
		item, itemErr := parsePositive(args[itemAt], 1<<31-1)
		if itemErr != nil {
			return parsed, true, prefixed, fmt.Errorf("物品ID必须是正整数")
		}
		parsed.target = strings.TrimSpace(strings.Join(args[:itemAt], " "))
		if parsed.target == "" {
			return parsed, true, prefixed, fmt.Errorf("用法：/giveitem <角色名> <物品ID> [数量]")
		}
		parsed.item = domain.ItemID(item)
		return parsed, true, prefixed, nil
	case "announce":
		parsed.admin, parsed.minimum = gmAdminAnnounce, store.GMWizard
		parsed.text = strings.TrimSpace(strings.Join(args, " "))
		if parsed.text == "" || len(parsed.text) > 300 {
			return parsed, true, prefixed, fmt.Errorf("公告内容必须为 1..300 字节")
		}
		return parsed, true, prefixed, nil
	case "mute":
		parsed.admin, parsed.minimum = gmAdminMute, store.GMWizard
		if len(args) < 2 {
			return parsed, true, prefixed, fmt.Errorf("用法：/mute <角色名> <分钟>")
		}
		minutes, minuteErr := parsePositive(args[len(args)-1], 10_080)
		if minuteErr != nil {
			return parsed, true, prefixed, fmt.Errorf("禁言分钟必须在 1..10080")
		}
		parsed.target = strings.TrimSpace(strings.Join(args[:len(args)-1], " "))
		parsed.duration = time.Duration(minutes) * time.Minute
		return parsed, true, prefixed, nil
	case "unmute":
		parsed.admin, parsed.minimum = gmAdminUnmute, store.GMWizard
		parsed.target = strings.TrimSpace(strings.Join(args, " "))
		if parsed.target == "" {
			return parsed, true, prefixed, fmt.Errorf("用法：/unmute <角色名>")
		}
		return parsed, true, prefixed, nil
	case "kick":
		parsed.admin, parsed.minimum = gmAdminKick, store.GMArch
		parsed.target = strings.TrimSpace(strings.Join(args, " "))
		if parsed.target == "" {
			return parsed, true, prefixed, fmt.Errorf("用法：/kick <角色名>")
		}
		return parsed, true, prefixed, nil
	case "ban", "unban":
		parsed.minimum = store.GMAdmin
		if name == "ban" {
			parsed.admin = gmAdminBan
			if len(args) >= 2 {
				last := args[len(args)-1]
				if _, numericErr := strconv.ParseInt(last, 10, 64); numericErr == nil {
					minutes, minuteErr := parsePositive(last, 525_600)
					if minuteErr != nil {
						return parsed, true, prefixed, fmt.Errorf("封禁分钟必须在 1..525600")
					}
					parsed.duration = time.Duration(minutes) * time.Minute
					args = args[:len(args)-1]
				}
			}
		} else {
			parsed.admin = gmAdminUnban
		}
		parsed.target = strings.TrimSpace(strings.Join(args, " "))
		if parsed.target == "" {
			if name == "ban" {
				return parsed, true, prefixed, fmt.Errorf("用法：/ban <角色名> [分钟]")
			}
			return parsed, true, prefixed, fmt.Errorf("用法：/unban <角色名>")
		}
		return parsed, true, prefixed, nil
	default:
		return parsed, false, prefixed, nil
	}
}

type gmSessionControl struct {
	sink Sink
	sess *Session
}

func (c gmSessionControl) Notify(text string) {
	if c.sink == nil {
		return
	}
	if pkt := protocol.GMCommandText(text); pkt != nil {
		c.sink.Send(pkt)
	}
}

func (c gmSessionControl) Disconnect() {
	if c.sink != nil {
		c.sink.Close()
	}
}

func (c gmSessionControl) SetMutedUntil(until time.Time) {
	if c.sess == nil {
		return
	}
	c.sess.mu.Lock()
	c.sess.mutedUntil = until
	c.sess.mu.Unlock()
}

func (c gmSessionControl) MutedUntil() time.Time {
	if c.sess == nil {
		return time.Time{}
	}
	c.sess.mu.Lock()
	defer c.sess.mu.Unlock()
	return c.sess.mutedUntil
}

func (c gmSessionControl) DeliverChat(message online.ChatMessage) {
	if c.sink == nil {
		return
	}
	shares := make([]protocol.ChatShareView, 0, len(message.Shares))
	for _, share := range message.Shares {
		shares = append(shares, protocol.ChatShareView{
			Kind: share.Kind, ItemID: int32(share.Item), Quality: share.Quality,
			Name: share.Name, Desc: share.Desc, Pet: share.Pet,
			PPAiUsed: share.PPAiUsed, PPAiCap: share.PPAiCap,
			PPShUsed: share.PPShUsed, PPShCap: share.PPShCap,
		})
	}
	var pkt []byte
	if message.HornTier > 0 {
		pkt = protocol.HornChat(int32(message.Sender), message.Name, message.Text, message.HornTier, message.HornSkin, shares)
	} else {
		pkt = protocol.ChannelChat(int32(message.Sender), message.Name, message.Theme, message.Channel, message.Text, shares...)
	}
	if pkt != nil {
		c.sink.Send(pkt)
	}
	if message.Channel == 0 {
		if pkt := protocol.EntityBubble(int32(message.Sender), message.Text); pkt != nil {
			c.sink.Send(pkt)
		}
	}
}

func (c gmSessionControl) RefreshSystemRequests() {
	if c.sess != nil {
		c.sess.refreshSystemRequests()
	}
}

func (c gmSessionControl) RefreshFriends() {
	if c.sess != nil {
		c.sess.refreshFriendList()
	}
}

func (c gmSessionControl) BlocksFrom(from domain.CharID) bool {
	return c.sess != nil && c.sess.blocksFrom(from)
}

func (c gmSessionControl) RefreshBlocks() {
	if c.sess != nil {
		c.sess.refreshBlockList()
	}
}

func (c gmSessionControl) DeliverGossip(from, text string) {
	if c.sink != nil {
		c.sink.Send(protocol.PrivateChat(from, text))
	}
}

func (c gmSessionControl) RequestNewbieTip(id int32) {
	if c.sess != nil {
		c.sess.postToScene(scene.RequestNewbieTip{ID: c.sess.entityID(), Tip: id})
	}
}

func (c gmSessionControl) RefreshMailIndicator() {
	if c.sess != nil {
		c.sess.refreshMailIndicator()
	}
}

func (c gmSessionControl) RefreshFamily() {
	if c.sess != nil {
		c.sess.refreshFamily()
	}
}

func (c gmSessionControl) RefreshFamilyPositions() {
	if c.sess != nil {
		c.sess.refreshFamilyPositions()
	}
}

func (c gmSessionControl) RefreshApprentice() {
	if c.sess != nil {
		c.sess.refreshApprentice()
	}
}

func (c gmSessionControl) OpenTrade(other domain.EntityID, otherName string) {
	if c.sink != nil {
		c.sink.Send(protocol.TradeOpen(other, otherName))
	}
}

func (c gmSessionControl) UpdateTrade(state online.TradeState) {
	if c.sink == nil {
		return
	}
	convert := func(items []online.TradeItem) []protocol.TradeItemView {
		out := make([]protocol.TradeItemView, len(items))
		for i, item := range items {
			out[i] = protocol.TradeItemView{
				ID: item.ID, Count: item.Count, Name: item.Name,
				Info: item.Info, Quality: item.Quality, Pet: item.Pet,
			}
		}
		return out
	}
	if pkt := protocol.TradeState(protocol.TradeStateView{
		MyLocked: state.MyLocked, OtherLocked: state.OtherLocked,
		MyConfirmed: state.MyConfirmed, OtherConfirmed: state.OtherConfirmed,
		MyMoney: state.MyMoney, OtherMoney: state.OtherMoney,
		MyItems: convert(state.MyItems), OtherItems: convert(state.OtherItems),
	}); pkt != nil {
		c.sink.Send(pkt)
	}
}

func (c gmSessionControl) EndTrade(color uint8, text string) {
	if c.sink != nil {
		c.sink.Send(protocol.TradeEnd(color, text))
	}
}

func (s *Session) executeGMAdmin(parsed parsedGMCommand, account *store.Account,
	actor *domain.Character) (bool, string) {
	if s.deps.Online == nil {
		return false, "在线索引不可用"
	}
	switch parsed.admin {
	case gmAdminAnnounce:
		recipients := s.deps.Online.Recipients()
		message := "【系统公告】" + parsed.text
		for _, recipient := range recipients {
			if c, ok := recipient.Control.(online.AnnouncementControl); ok {
				c.Announce(parsed.text)
			} else {
				recipient.Control.Notify(message)
			}
		}
		return true, fmt.Sprintf("公告已发送给 %d 名在线角色", len(recipients))
	case gmAdminKick:
		loc, control, ok := s.deps.Online.ControlByName(parsed.target)
		if !ok {
			return false, "目标角色不在线：" + parsed.target
		}
		if actor != nil && loc.Char == domain.CharID(actor.ID) {
			return false, "不能踢出当前 GM 角色"
		}
		control.Disconnect()
		return true, "已踢出角色：" + loc.Name
	case gmAdminMute, gmAdminUnmute:
		loc, control, ok := s.deps.Online.ControlByName(parsed.target)
		if !ok {
			return false, "目标角色不在线：" + parsed.target
		}
		moderation, ok := control.(online.ModerationControl)
		if !ok {
			return false, "目标会话不支持禁言控制"
		}
		if parsed.admin == gmAdminUnmute {
			moderation.SetMutedUntil(time.Time{})
			return true, "已解除禁言：" + loc.Name
		}
		until := time.Now().Add(parsed.duration)
		moderation.SetMutedUntil(until)
		return true, fmt.Sprintf("已禁言 %s：%d 分钟", loc.Name, int(parsed.duration/time.Minute))
	case gmAdminBan, gmAdminUnban:
		ctx, cancel := context.WithTimeout(context.Background(), gmCommandTimeout)
		defer cancel()
		if parsed.admin == gmAdminBan && parsed.duration > 0 {
			loc, _, ok := s.deps.Online.ControlByName(parsed.target)
			if !ok {
				return false, "目标角色不在线：" + parsed.target
			}
			if account != nil && loc.Account == account.ID {
				return false, "不能封禁当前 GM 账号"
			}
			timed, ok := s.deps.Store.(store.TimedBanStore)
			if !ok {
				return false, "存储不支持定时封禁"
			}
			until := time.Now().Add(parsed.duration)
			if err := timed.SetAccountBannedUntil(ctx, loc.Account, until); err != nil {
				s.log.Error("GM 修改定时封禁失败", "target", loc.Name,
					"accountID", loc.Account, "until", until, "err", err)
				return false, "修改定时封禁状态失败"
			}
			disconnected := s.revokeAccountSessions(loc.Account, "账号已被封禁")
			return true, fmt.Sprintf("已封禁角色 %s 所属账号 %d 分钟，并断开 %d 个在线角色",
				loc.Name, int(parsed.duration/time.Minute), disconnected)
		}
		target, err := s.deps.Store.CharByName(ctx, parsed.target)
		if errors.Is(err, domain.ErrCharNotFound) {
			return false, "目标角色不存在：" + parsed.target
		}
		if err != nil {
			s.log.Error("GM 查询封禁目标失败", "target", parsed.target, "err", err)
			return false, "读取目标角色失败"
		}
		if target == nil {
			return false, "目标角色不存在：" + parsed.target
		}
		if account != nil && target.AccountID == account.ID {
			return false, "不能封禁或解封当前 GM 账号"
		}
		banned := parsed.admin == gmAdminBan
		if err := s.deps.Store.SetAccountBanned(ctx, target.AccountID, banned); err != nil {
			s.log.Error("GM 修改封禁状态失败", "target", target.Name,
				"accountID", target.AccountID, "banned", banned, "err", err)
			return false, "修改封禁状态失败"
		}
		if !banned {
			return true, "已解封角色所属账号：" + target.Name
		}
		disconnected := s.revokeAccountSessions(target.AccountID, "账号已被封禁")
		return true, fmt.Sprintf("已封禁角色 %s 所属账号，并断开 %d 个在线角色",
			target.Name, disconnected)
	default:
		return false, "未登记的 GM 管理动作"
	}
}

func disconnectAccount(registry *online.Registry, accountID int64) int {
	disconnected := 0
	for _, recipient := range registry.Recipients() {
		if recipient.Location.Account != accountID {
			continue
		}
		recipient.Control.Disconnect()
		disconnected++
	}
	return disconnected
}

func formatOnline(registry *online.Registry) string {
	if registry == nil {
		return "在线索引不可用"
	}
	locations := registry.Snapshot()
	sort.Slice(locations, func(i, j int) bool { return locations[i].Name < locations[j].Name })
	const maxNames = 20
	n := len(locations)
	shown := locations
	if len(shown) > maxNames {
		shown = shown[:maxNames]
	}
	parts := make([]string, 0, len(shown))
	for _, loc := range shown {
		parts = append(parts, fmt.Sprintf("%s(Lv%d,地图%d)",
			loc.Name, loc.Profile.Level, loc.Scene.MapID))
	}
	message := fmt.Sprintf("当前在线 %d 人", n)
	if len(parts) != 0 {
		message += "：" + strings.Join(parts, "、")
	}
	if n > len(shown) {
		message += fmt.Sprintf("……另有 %d 人", n-len(shown))
	}
	return message
}

func onePositiveInt64(args []string, max int64, usage string) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("用法：%s", usage)
	}
	value, err := parsePositive(args[0], max)
	if err != nil {
		return 0, fmt.Errorf("数值必须在 1..%d", max)
	}
	return value, nil
}

func parsePositive(raw string, max int64) (int64, error) {
	base := 10
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		base = 0
	}
	value, err := strconv.ParseInt(raw, base, 64)
	if err != nil || value <= 0 || value > max {
		return 0, fmt.Errorf("out of range")
	}
	return value, nil
}

func parseGMState(args []string, usage string) (scene.GMStateMode, error) {
	if len(args) == 0 {
		return scene.GMStateQuery, nil
	}
	if len(args) != 1 {
		return scene.GMStateQuery, fmt.Errorf("用法：%s", usage)
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return scene.GMStateEnable, nil
	case "off":
		return scene.GMStateDisable, nil
	case "status":
		return scene.GMStateQuery, nil
	default:
		return scene.GMStateQuery, fmt.Errorf("用法：%s", usage)
	}
}

func (s *Session) sendGMText(text string) {
	if pkt := protocol.GMCommandText(text); pkt != nil {
		s.sink.Send(pkt)
	}
}

func (s *Session) auditGM(account *store.Account, char *domain.Character, command, raw string,
	ok bool, result string) {
	var accountName, charName, level string
	if account != nil {
		accountName, level = account.Username, account.GMLevel.String()
	}
	if char != nil {
		charName = char.Name
	}
	s.log.Info("GM命令审计", "account", accountName, "char", charName, "gmLevel", level,
		"command", command, "args", commandArgs(raw), "ok", ok, "result", result)
}

func commandArgs(raw string) string {
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/")))
	if len(fields) > 0 && strings.EqualFold(fields[0], "gm") {
		fields = fields[1:]
	}
	if len(fields) <= 1 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

func firstField(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (c gmSessionControl) DeliverGossipShares(from, text string, shares []domain.ChatShare) {
	if c.sink == nil {
		return
	}
	v := make([]protocol.ChatShareView, 0, len(shares))
	for _, s := range shares {
		v = append(v, protocol.ChatShareView{
			Kind: s.Kind, ItemID: int32(s.Item), Quality: s.Quality, Name: s.Name, Desc: s.Desc,
			Pet: s.Pet, PPAiUsed: s.PPAiUsed, PPAiCap: s.PPAiCap,
			PPShUsed: s.PPShUsed, PPShCap: s.PPShCap,
		})
	}
	if p := protocol.PrivateChat(from, text, v...); p != nil {
		c.sink.Send(p)
	}
}
func (c gmSessionControl) Announce(text string) {
	if c.sink == nil || c.sess == nil {
		return
	}
	p := c.sess.deps.Announcements
	p.Lines = []domain.AnnouncementLine{{ID: 0, Text: text}}
	p.Enabled = true
	if b := protocol.Announcements(p, true); b != nil {
		c.sink.Send(b)
	}
}
