package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"

func (s *Scene) onRenamePlayer(cmd RenamePlayer) {
	ok := false
	if p := s.players[cmd.ID]; p != nil && p.Player != nil && p.Player.Char != nil && cmd.Name != "" {
		p.Name = cmd.Name
		p.Player.Char.Name = cmd.Name
		p.Player.MarkDirty()
		s.emit(event.PlayerRenamed{Who: p.ID, Name: cmd.Name})
		ok = true
	}
	if cmd.Done != nil {
		cmd.Done <- ok
	}
}
