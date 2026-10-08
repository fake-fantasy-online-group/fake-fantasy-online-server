package protocol

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// 0x8048 uses integer speed/time fields, not F32.
func Announcements(p domain.AnnouncementPool, immediate bool) []byte {
	if len(p.Lines) > 65535 || p.Speed <= 0 || p.IntervalSec <= 0 || p.PeriodMin <= 0 {
		return nil
	}
	w := NewW(0x8048).U8(b2u8(immediate)).U16(uint16(len(p.Lines)))
	for _, l := range p.Lines {
		if !wireString(l.Text) {
			return nil
		}
		w.I32(l.ID).Str(l.Text)
	}
	return w.I32(p.Speed).I32(p.IntervalSec).U8(b2u8(p.Enabled)).I32(p.PeriodMin).Bytes()
}
func EntityOwner(id, owner domain.EntityID) []byte {
	if id == 0 {
		return nil
	}
	return NewW(0x8043).I32(int32(id)).I32(int32(owner)).Bytes()
}
func EntityEffect(target uint8, effect string) []byte {
	if target > 1 || effect == "" || !wireString(effect) {
		return nil
	}
	return NewW(0x800d).U8(target).Str(effect).Bytes()
}

// Native 1.5.8: 0x8074 opens HornDialog; 0x8071 is counter state.
func OpenHorn(item domain.ItemID, tier, skin uint8) []byte {
	if item <= 0 || tier == 0 {
		return nil
	}
	return NewW(0x8074).I32(int32(item)).U8(tier).U8(skin).Bytes()
}
func appendShares(w *W, shares []ChatShareView) bool {
	if len(shares) > 255 {
		return false
	}
	w.U8(uint8(len(shares)))
	for _, v := range shares {
		if !appendChatShare(w, v) {
			return false
		}
	}
	return true
}

func appendChatShare(w *W, v ChatShareView) bool {
	if !wireString(v.Name) || !wireString(v.Desc) || (v.Kind == 1) != (v.Pet != nil) {
		return false
	}
	w.U8(v.Kind).I32(v.ItemID).U8(v.Quality).Str(v.Name).Str(v.Desc).
		U8(b2u8(v.Pet != nil))
	if v.Pet != nil && !appendPetItem(w, v.Pet) {
		return false
	}
	w.I32(v.PPAiUsed).I32(v.PPAiCap).I32(v.PPShUsed).I32(v.PPShCap)
	return true
}
func HornChat(sender int32, name, text string, tier, skin uint8, shares []ChatShareView) []byte {
	if sender <= 0 || tier == 0 || !wireString(name) || !wireString(text) {
		return nil
	}
	w := NewW(0x8039).I32(sender).Str(name).U8(0).U8(5).Str(text)
	if !appendShares(w, shares) {
		return nil
	}
	return w.U8(tier).U8(skin).Bytes()
}
