package domain

// TransformSourceStatus updates a specific already-admitted source-owned status
// without reapplying overlay, resetting the tick clock, or replenishing shields.
func (s *StatusSet) TransformSourceStatus(id StatusID, source EntityID, fn func(Active) Active) {
	if s == nil || fn == nil {
		return
	}
	list := s.act[id]
	for i := range list {
		if list[i].Source == source {
			list[i] = fn(list[i])
		}
	}
	s.act[id] = list
}
