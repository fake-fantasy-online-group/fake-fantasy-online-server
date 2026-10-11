package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

type EndgameMessage struct { Who domain.EntityID; Message domain.EndgameResponse }
func (e EndgameMessage) Subject() domain.EntityID { return e.Who }
