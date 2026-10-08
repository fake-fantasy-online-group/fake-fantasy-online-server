package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"

// onSetResting 接收客户端已经完成的坐下/起身动作。表现由客户端本地切换，
// 场景只维护权威的恢复资格和首跳时刻。
func (s *Scene) onSetResting(cmd SetResting) {
	p, ok := s.players[cmd.ID]
	if !ok || p.Player == nil {
		s.log.Debug("忽略坐下状态：角色不在当前场景", "entity", cmd.ID, "on", cmd.On)
		return
	}
	if !cmd.On || !p.Alive() {
		p.Player.StopResting()
		s.log.Debug("坐下回血停止", "char", p.Name, "on", cmd.On, "alive", p.Alive())
		return
	}
	every := s.rest.IntervalTicks()
	if every == 0 {
		s.log.Warn("忽略坐下状态：回血配置未启用", "char", p.Name)
		return
	}
	p.Player.SetResting(true, s.tick, every)
	s.log.Debug("坐下回血开始", "char", p.Name, "间隔tick", every,
		"百分比", s.rest.HealMaxHPPct, "固定值", s.rest.HealFlat)
	s.requestNewbieTip(p.ID, 29)
}

// stepRest 在固定场景帧上结算坐下回血。满血时仍维持周期相位，但不发送
// 0 治疗飘字；之后受伤会在下一次正常周期点恢复。
func (s *Scene) stepRest() {
	every := s.rest.IntervalTicks()
	if every == 0 {
		return
	}
	for _, p := range s.players {
		if p.Player == nil {
			continue
		}
		if !p.Alive() {
			p.Player.StopResting()
			continue
		}
		if !p.Player.RestHealDue(s.tick, every) || p.HP >= p.MaxHP {
			continue
		}
		amount := s.rest.HealAmount(p.MaxHP)
		if amount <= 0 {
			continue
		}
		ev := combat.Restore(p, p, amount, 0)
		if ev.Amount <= 0 {
			continue
		}
		s.emit(ev)
		s.emitTo(p.ID, s.attributeSnapshot(p))
		s.log.Debug("坐下回血", "char", p.Name, "恢复", ev.Amount, "当前", ev.DstHP,
			"上限", p.MaxHP)
	}
}
