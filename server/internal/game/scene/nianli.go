package scene

import (
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 原作说明给出的在线恢复口径：每 10 分钟恢复 4 点念力。用逻辑帧计时，
// 因此服务器暂停或玩家离线都不会偷算恢复时间。
const (
	nianliRecoveryEvery  = domain.Tick((10 * time.Minute) / (domain.TickMS * time.Millisecond))
	nianliRecoveryAmount = 4
)

func (s *Scene) stepNianli() {
	for _, p := range s.players {
		if p.Player == nil || !p.Player.NianliRecoveryDue(s.tick, nianliRecoveryEvery) {
			continue
		}
		ch := p.Player.Char
		current := ch.EffectiveNianli()
		if current >= domain.DefaultNianli {
			continue
		}
		ch.SetNianli(current + nianliRecoveryAmount)
		p.Player.MarkDirty()
		s.emitTo(p.ID, s.attributeSnapshot(p))
	}
}
