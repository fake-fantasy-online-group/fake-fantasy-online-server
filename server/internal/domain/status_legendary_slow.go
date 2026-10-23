package domain

// ShortenLongestSlow removes pct percent of the remaining duration of one
// effective, finite movement slow. Roots, stuns, expired and suspended effects
// are not slows. Ties are stable by status ID and then source ID.
func (s *StatusSet) ShortenLongestSlow(now Tick, pct int32) bool {
	if s == nil || pct <= 0 || s.SuppressesHarmfulAt(now) {
		return false
	}
	if pct > 100 {
		pct = 100
	}
	var picked StatusID
	index := -1
	var longest Tick
	var source EntityID
	for id, list := range s.act {
		for i, a := range list {
			if !a.Def.Harmful() || a.Def.BlockMove || a.Def.Control != ControlNone || a.ExpireAt == 0 || a.ExpireAt <= now {
				continue
			}
			slow := false
			for _, affix := range a.Def.Affixes {
				if affix.Attr == AttrMoveSpeed && affix.Value < 0 {
					slow = true
					break
				}
			}
			if !slow {
				continue
			}
			remaining := a.ExpireAt - now
			if index < 0 || remaining > longest || remaining == longest && (id < picked || id == picked && a.Source < source) {
				picked, index, longest, source = id, i, remaining, a.Source
			}
		}
	}
	if index < 0 {
		return false
	}
	// Split the multiply to avoid overflowing a permanent-sized Tick.
	keep := uint64(100 - pct)
	left := Tick(uint64(longest)/100*keep + (uint64(longest)%100*keep+99)/100)
	if left == longest {
		return false
	}
	list := s.act[picked]
	if left == 0 {
		list = append(list[:index], list[index+1:]...)
		if len(list) == 0 {
			delete(s.act, picked)
		} else {
			s.act[picked] = list
		}
		return true
	}
	list[index].ExpireAt = now + left
	s.act[picked] = list
	return true
}
