package scene

import (
	"fmt"
	"sort"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func (s *Scene) trialNotice(p *entity.Entity, text string) {
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
}
func (s *Scene) trialClaimNPC(p *entity.Entity) bool {
	for _, e := range s.trial.Entrances {
		if e.ClaimMap == s.id.MapID && s.npcOnMap(e.ClaimNPC) != "" && p.Player.QuestNPC == e.ClaimNPC {
			return true
		}
	}
	return false
}
func (s *Scene) trialEntryNPC(p *entity.Entity) bool {
	for _, e := range s.trial.Entrances {
		if e.EntryMap == s.id.MapID && s.npcOnMap(e.EntryNPC) != "" && p.Player.QuestNPC == e.EntryNPC {
			return true
		}
	}
	return false
}
func (s *Scene) onTrialAction(cmd TrialAction) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || s.trial == nil || !p.Alive() {
		return
	}
	switch cmd.Action {
	case "trialtask":
		s.acceptTrial(p)
	case "trialclaim":
		s.completeTrial(p)
	case "trialgo":
		s.travelToTrialGuard(p)
	case "trial":
		if !s.trialEntryNPC(p) {
			s.trialNotice(p, "请在对应的妖魔结界守卫处进入试炼。")
			return
		}
		ids := s.trialMapIDs()
		s.onEnterDungeon(EnterDungeon{ID: p.ID, MapID: ids[0]})
	case "trialleave":
		if s.trial.IsMap(s.id.MapID) && s.dungeon != nil {
			s.onTeleport(Teleport{ID: p.ID, To: domain.SceneID{MapID: s.dungeon.Exit.MapID}, At: s.dungeon.Exit})
		}
	}
}
func (s *Scene) acceptTrial(p *entity.Entity) {
	ch := p.Player.Char
	r := s.trial
	if !s.trialClaimNPC(p) {
		s.trialNotice(p, "请在四村试炼小仙处领取试炼任务。")
		return
	}
	if ch.Level < r.MinLevel || ch.Level > r.MaxLevel {
		s.trialNotice(p, fmt.Sprintf("试炼开放等级为%d～%d级。", r.MinLevel, r.MaxLevel))
		return
	}
	if ch.Quests.Active(r.Quest) {
		s.trialNotice(p, "你已经领取了试炼任务。")
		return
	}
	if ch.Quests == nil {
		ch.Quests = domain.QuestLog{}
	}
	ch.Quests[r.Quest] = domain.QuestEntry{State: domain.QuestActive}
	ch.Trial.MonsterLevel = 0
	p.Player.MarkDirty()
	s.emitTo(p.ID, event.QuestAccepted{Who: p.ID, Quest: int32(r.Quest), Name: "试炼"})
	s.pushQuestLog(p)
	s.pushQuestOffers(p, p.Player.QuestNPC)
	s.trialNotice(p, "已领取试炼任务，请前往妖魔结界守卫。")
}
func (s *Scene) completeTrial(p *entity.Entity) {
	ch := p.Player.Char
	r := s.trial
	if !s.trialClaimNPC(p) {
		s.trialNotice(p, "请回四村试炼小仙处领取奖励。")
		return
	}
	entry, ok := ch.Quests[r.Quest]
	if !ok || entry.State != domain.QuestDeliver || ch.Trial.MonsterLevel <= 0 {
		s.trialNotice(p, "尚未完成试炼，请消灭副本内全部怪物。")
		return
	}
	day := domain.DayOf(s.now().In(time.FixedZone("Asia/Shanghai", 8*60*60)))
	progress := ch.Trial
	if progress.Day != day {
		progress.Day = day
		progress.DailyCompleted = 0
	}
	special := progress.DailyCompleted == int32(len(r.Multipliers))-1
	reward := domain.QuestReward{Exp: r.Experience(ch.Level, ch.Trial.MonsterLevel, progress, day, s.levels)}
	// Check worst-case inventory capacity before rolling; repeated bag-full requests
	// cannot reroll the daily prize or consume a completion.
	if special {
		reward.Items = []domain.RewardItem{{Item: r.Pill, Qty: 1}, {Item: r.Fragment, Qty: r.RewardMax}}
	}
	if !s.canHoldReward(p, reward, 0, 0) {
		s.trialNotice(p, "请留出容纳试炼奖励的背包空间。")
		return
	}
	if special {
		reward.Items = reward.Items[:1]
		if s.rng.Intn(100) < int(r.RewardChance) {
			reward.Items = append(reward.Items, domain.RewardItem{Item: r.Fragment, Qty: r.RewardMin + int32(s.rng.Intn(int(r.RewardMax-r.RewardMin+1)))})
		}
	}
	planned, _, ok := s.planQuestItems(p.Player.Bag, nil, nil, reward.Items)
	if !ok {
		s.trialNotice(p, "背包无法容纳奖励。")
		return
	}
	p.Player.Bag = planned
	progress.DailyCompleted++
	progress.Cycle = (progress.Cycle + 1) % int32(len(r.Multipliers))
	progress.MonsterLevel = 0
	ch.Trial = progress
	ch.Quests.Finish(r.Quest)
	// Trial percentages are the complete reward; exp cards must not multiply them again.
	s.grantExpExact(p, reward.Exp)
	p.Player.MarkDirty()
	s.pushInventory(p)
	def := domain.QuestDef{ID: r.Quest, Name: "试炼"}
	s.emitTo(p.ID, s.questCompletedEvent(p.ID, def, reward, ""))
	s.pushQuestLog(p)
	s.pushQuestOffers(p, p.Player.QuestNPC)
	s.trialNotice(p, fmt.Sprintf("试炼完成，获得%d经验；今日已完成%d次。", reward.Exp, progress.DailyCompleted))
}
func (s *Scene) trialQuestLog(p *entity.Entity, list []domain.ActiveQuest) []domain.ActiveQuest {
	if s.trial == nil {
		return list
	}
	ch := p.Player.Char
	q, ok := ch.Quests[s.trial.Quest]
	if !ok || q.State == domain.QuestFinished {
		return list
	}
	active := domain.ActiveQuest{ID: s.trial.Quest, Total: 2, To: "妖魔结界守卫", Text: "请由队长先进入妖魔结界，消灭全部怪物。", Progress: fmt.Sprintf("本轮第%d/6次", ch.Trial.Cycle+1)}
	if q.State == domain.QuestDeliver {
		active.Step = 1
		active.Ready = true
		active.To = "试炼小仙"
		active.Text = "已消灭全部妖魔，回四村试炼小仙领取奖励。"
	}
	return append(list, active)
}
func (s *Scene) trialMapIDs() []int32 {
	var ids []int32
	for id := range s.trial.Maps {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// prepareTrial runs only in the destination actor, before its first player enters.
func (s *Scene) prepareTrial(cmd Enter) {
	if !s.trial.IsMap(s.id.MapID) || s.trialLevel != 0 {
		return
	}
	level := s.router.trialLevelOf(s.id)
	if level < s.trial.MinLevel || level > s.trial.MaxLevel {
		return
	} // closed until authorized entry assigns a level
	s.trialLevel = level
	var normal, boss []domain.TrialSpawn
	for _, p := range s.trial.Maps[s.id.MapID] {
		d := s.trial.Monsters[level][p.Candidates[0]]
		if d.Kind == domain.MonsterBoss {
			boss = append(boss, p)
		} else {
			normal = append(normal, p)
		}
	}
	defs := spawn.MapDefs{}
	var pts []domain.SpawnPoint
	choose := func(pool []domain.TrialSpawn, count int32) {
		perm := s.rng.Perm(len(pool))
		for _, index := range perm[:int(count)] {
			p := pool[index]
			id := p.Candidates[s.rng.Intn(len(p.Candidates))]
			d := s.trial.Monsters[level][id]
			if d.Kind == domain.MonsterBoss {
				d = s.trial.Monsters[level+s.trial.BossOffset][id]
			}
			defs[id] = d
			p.Point.Monster = id
			pts = append(pts, p.Point)
		}
	}
	choose(normal, s.trial.NormalCount)
	choose(boss, s.trial.BossCount)
	s.defs = defs
	s.spawner, _ = spawn.New(pts, defs, s.alloc)
	s.populate()
}
func (s *Scene) trialRemaining() int {
	n := 0
	for _, m := range s.monsters {
		if m.Alive() && m.Monster != nil && m.Monster.Summoner == 0 {
			n++
		}
	}
	return n
}
func (s *Scene) updateTrialClear() {
	if !s.trial.IsMap(s.id.MapID) || s.trialLevel == 0 || s.trialCleared {
		return
	}
	remaining := s.trialRemaining()
	if remaining == 0 {
		s.trialCleared = true
		s.router.finishTrial(s.id)
	}
	for _, p := range s.players {
		if remaining > 0 && remaining <= 10 {
			s.emitTo(p.ID, event.NewbieTextNotice{Who: p.ID, Text: fmt.Sprintf("结界里的妖魔还剩余%d只！", remaining)})
		}
		ch := p.Player.Char
		q, ok := ch.Quests[s.trial.Quest]
		if !ok || q.State != domain.QuestActive {
			continue
		}
		if remaining == 0 {
			q.State = domain.QuestDeliver
			ch.Quests[s.trial.Quest] = q
			ch.Trial.MonsterLevel = s.trialLevel
			p.Player.MarkDirty()
			s.trialNotice(p, "妖魔已全部消灭！试炼任务完成，请回试炼小仙领取奖励。")
			s.pushQuestLog(p)
			s.emitTo(p.ID, event.NewbieTextNotice{Who: p.ID, Text: "您和队友已经消灭结界里的所在妖魔，现在可以离开结界，到城里的试炼大仙处领取试炼奖励！"})
		}
	}
}
func (s *Scene) notifyDungeonOpened(ch *domain.Character, pid domain.PartyID) {
	if s.dungeon == nil || pid == 0 || s.dungeonParty == nil || s.dungeonNotify == nil {
		return
	}
	roster := s.dungeonParty(domain.CharID(ch.ID))
	for _, name := range roster.Members {
		if name != ch.Name {
			s.dungeonNotify(name, "队长已开启副本「"+s.dungeon.Name+"」，请从副本入口进入。")
		}
	}
}

// The native trialgo menu buys travel to the village's wilderness guard.
func (s *Scene) travelToTrialGuard(p *entity.Entity) {
	if !s.trialClaimNPC(p) || s.router == nil || s.mapLoads[p.ID] != nil {
		return
	}
	for _, entrance := range s.trial.Entrances {
		if entrance.ClaimMap != s.id.MapID {
			continue
		}
		money, err := inventoryMoney(p.Player.Char.Money)
		if err != nil || money < entrance.Price {
			s.trialNotice(p, "银币不足，无法传送。")
			return
		}
		to := domain.SceneID{MapID: entrance.EntryMap}
		if _, err := s.router.Ensure(to); err != nil {
			s.trialNotice(p, "暂时无法前往妖魔结界守卫，请稍后再试。")
			return
		}
		oldMoney := p.Player.Char.Money
		p.Player.Char.Money = moneyFromInventory(money - entrance.Price)
		if err := s.pushInventory(p); err != nil {
			p.Player.Char.Money = oldMoney
			return
		}
		p.Player.MarkDirty()
		if !s.onTeleport(Teleport{ID: p.ID, To: to, At: entrance.EntryPos}) {
			p.Player.Char.Money = oldMoney
			_ = s.pushInventory(p)
		}
		return
	}
}
