package domain

type HairChangeMode uint8

const (
	HairColor HairChangeMode = iota // 客户端 mode=0，修改 hair（发色）
	HairStyle                       // 客户端 mode=1，修改 head（发型）
)

func (m HairChangeMode) Valid() bool { return m == HairColor || m == HairStyle }

type HairOption struct {
	Mode     HairChangeMode
	ID       uint8
	Money    int64
	DyeItem  ItemID
	DyeCount int32
}

type HairRules map[HairChangeMode]map[uint8]HairOption

func (r HairRules) Get(mode HairChangeMode, id uint8) (HairOption, bool) {
	options := r[mode]
	option, ok := options[id]
	return option, ok
}
