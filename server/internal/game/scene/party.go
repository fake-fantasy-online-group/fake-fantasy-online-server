package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 组队在场景里的那一半。
//
// **场景不管名册。** 谁在队里、谁是队长、有哪些邀请，全在 `game/party` 里，
// 由会话层操作 —— 因为队伍跨场景，而场景只拥有自己这张图上的实体。
//
// 场景只做两件事，两件都只需要比较实体身上抄来的那个 `PartyID`：
//
//	选目标   药师的治愈术只能落在队友身上（不组队时只能给自己）
//	分经验   击杀的经验在**同场景**的队友之间分
//
// 判队友是比两个整数，没有锁、没有跨 goroutine 访问，帧循环干净。

// onSetParty 刷新一个玩家的队伍号。
func (s *Scene) onSetParty(cmd SetParty) {
	p, ok := s.players[cmd.ID]
	if !ok {
		return
	}
	if p.Player.Party == cmd.Party {
		return
	}
	p.Player.Party = cmd.Party
	s.updateDungeonEligibility(p)
	s.emitTo(p.ID, event.PartyChanged{Who: p.ID, Party: uint32(cmd.Party)})
	s.log.Debug("队伍变更", "char", p.Name, "队伍", cmd.Party)
}

func (s *Scene) onRefreshParty(cmd RefreshParty) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil {
		s.emitTo(p.ID, event.PartyChanged{Who: p.ID, Party: uint32(p.Player.Party)})
	}
}

// sameParty 报告两个实体是不是队友。
//
// **0 号队伍不算队友** —— 没组队的人 PartyID 都是 0，
// 漏掉这个判断的话全服所有散人会互相成为队友，药师能给任何路人加血。
func sameParty(a, b *entity.Entity) bool {
	if a.Player == nil || b.Player == nil {
		return false
	}
	return a.Player.Party != 0 && a.Player.Party == b.Player.Party
}

// partyMembersInScene 找出本场景内的队友（含自己）。
//
// 跨场景的队友拿不到经验：他们不在这张图上，本场景连他们的实体都没有。
func (s *Scene) partyMembersInScene(p *entity.Entity) []*entity.Entity {
	out := []*entity.Entity{p}
	if p.Player == nil || p.Player.Party == 0 {
		return out
	}
	for _, o := range s.players {
		if o.ID == p.ID || !o.Alive() {
			continue
		}
		if !sameParty(p, o) {
			continue
		}
		out = append(out, o)
	}
	return out
}

// splitKillExp 把一次击杀的经验分给队伍。
//
// 返回 true 表示分掉了（调用方就不要再给击杀者结算一遍）。
//
// 顺序有讲究：**先按等级差筛人，再按人数算加成**。
// 反过来的话，一个满级号带四个小号，加成照拿而小号一分钱拿不到 ——
// 那就成了"挂四个小号给自己加 40% 经验"。
//
// monster 用于对**每个成员自己的等级**应用跨级等级差惩罚：
// 满级号带小号打低级怪，满级号按自己的高等级被惩罚，小号不受牵连。
func (s *Scene) splitKillExp(killer *entity.Entity, exp int64, monster domain.MonsterID) bool {
	if killer.Player == nil || killer.Player.Party == 0 {
		return false
	}
	members := s.partyMembersInScene(killer)

	eligible := make([]*entity.Entity, 0, len(members))
	for _, m := range members {
		if domain.CanShareExp(killer.Player.Char.Level, m.Player.Char.Level) {
			eligible = append(eligible, m)
		}
	}
	if len(eligible) <= 1 {
		return false // 就他一个够格, 走单人那条路
	}

	each := domain.PartyExpShare(exp, len(eligible))
	for _, m := range eligible {
		gain := s.expAfterPenalty(each, m.Player.Char.Level, monster)
		s.grantExp(m, gain)
		// 宠物那份跟着主人走 —— 队友的宠物也在打, 没道理不给
		s.grantPetExp(m, gain)
	}
	s.log.Debug("组队分经验", "队伍", killer.Player.Party,
		"人数", len(eligible), "每人", each, "原始", exp)
	return true
}
