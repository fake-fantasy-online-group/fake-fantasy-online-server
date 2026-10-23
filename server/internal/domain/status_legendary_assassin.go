package domain

// ConsumeLegendaryStealthTrap changes only the existing native stealth instance.
// It creates no independent buff, clock, or restorable external charge ledger.
func (s *StatusSet) ConsumeLegendaryStealthTrap() bool {
	if s == nil {
		return false
	}
	for id, list := range s.act {
		for i := range list {
			d := &list[i].Def
			if d.Invisible && d.LegendaryStealthTrapCharges > 0 {
				d.LegendaryStealthTrapCharges--
				s.act[id] = list
				return true
			}
		}
	}
	return false
}

func (s *StatusSet) MarkLegendaryPoisonSpread(source EntityID) bool {
	if s == nil {
		return false
	}
	list := s.act[StatusPoison]
	for i := range list {
		d := &list[i].Def
		if list[i].Source == source && d.ClassSetPoison && !d.LegendaryPoisonCopy && !d.LegendaryPoisonSpread {
			d.LegendaryPoisonSpread = true
			s.act[StatusPoison] = list
			return true
		}
	}
	return false
}
