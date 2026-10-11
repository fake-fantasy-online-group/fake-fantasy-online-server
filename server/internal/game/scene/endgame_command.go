package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

type EndgameRequest struct { ID domain.EntityID; Request domain.EndgameRequest }
func (EndgameRequest) CmdName() string { return "endgame" }
