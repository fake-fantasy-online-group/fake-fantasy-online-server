package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func appendPetItem(w *W, p *domain.PetItemInfo) bool {
	if p == nil || len(p.Skills) > 255 || !wireString(p.Name) || !wireString(p.Prename) || !wireString(p.Species) {
		return false
	}
	w.I32(p.Model).I32(p.Slot).Str(p.Name).Str(p.Prename).Str(p.Species).U8(p.Gender).U8(p.Habit).U8(b2u8(p.Opened)).I32(p.Level).
		I32(p.Base.STR).I32(p.Base.INT).I32(p.Base.VIT).I32(p.Base.AGI).I32(p.Base.DEX).I32(p.Base.SPI).
		I32(p.Starve).I32(p.Trust).I32(p.FreePoints).I32(p.UsedPoints).U8(uint8(len(p.Skills)))
	for _, sk := range p.Skills {
		if !wireString(sk.Name) {
			return false
		}
		w.Str(sk.Name).I32(sk.Level)
	}
	w.U8(b2u8(p.Bound))
	return true
}
func appendTradePets(w *W, items []TradeItemView) bool {
	n := 0
	for _, v := range items {
		if v.Pet != nil {
			n++
		}
	}
	if n > 255 {
		return false
	}
	w.U8(uint8(n))
	for i, v := range items {
		if v.Pet != nil {
			w.U16(uint16(i))
			if !appendPetItem(w, v.Pet) {
				return false
			}
		}
	}
	return true
}
func appendTradePetPP(w *W, items []TradeItemView) {
	n := 0
	for _, v := range items {
		if v.Pet != nil {
			n++
		}
	}
	w.U8(uint8(n))
	for i, v := range items {
		if v.Pet != nil {
			w.U16(uint16(i)).I32(v.Pet.PPAiUsed).I32(v.Pet.PPAiCap).I32(0).I32(0)
		}
	}
}
