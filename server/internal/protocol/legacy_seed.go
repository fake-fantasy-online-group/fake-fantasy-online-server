package protocol

import "encoding/binary"

// ParseLegacyStatBlock 解开旧种子 JSON 里的 statBlockHex。
//
// 这不是线上额外协议：它只是 0x8004 角色记录尾部的历史存储形式，字段依次为
// 六个外观 U16、攻击表现、属性数组和 Honor。新数据不再生成这种块。
func ParseLegacyStatBlock(b []byte) (ap ApView, attrs []int32, honor int64, ok bool) {
	if len(b) < 18 {
		return ap, nil, 0, false
	}
	u16 := func(i int) uint16 { return binary.LittleEndian.Uint16(b[i:]) }
	ap = ApView{
		Body: u16(0), Cap: u16(2), Backpack: u16(4),
		WeaponR: u16(6), WeaponL: u16(8), Face: u16(10),
		AtkVariant: b[12], WeaponCType: b[13], AtkDist: u16(14),
	}
	n := int(u16(16))
	if len(b) < 18+n*4 {
		return ap, nil, 0, false
	}
	attrs = make([]int32, n)
	for i := range attrs {
		attrs[i] = int32(binary.LittleEndian.Uint32(b[18+i*4:]))
	}
	if rest := b[18+n*4:]; len(rest) >= 8 {
		honor = int64(binary.LittleEndian.Uint64(rest))
	}
	return ap, attrs, honor, true
}
