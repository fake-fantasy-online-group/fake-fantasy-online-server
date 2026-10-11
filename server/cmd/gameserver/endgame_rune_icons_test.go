package main

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestEndgameRuneIconsFollowVerifiedMonsterCollectibles(t *testing.T) {
	// The SQL provenance test proves monster -> drop -> collectible -> small_map.
	// This pins the runtime lookup, including the user's wolf and earthworm cases.
	verified := map[int]int32{
		2:  4332, // 三头龙 -> 龙牙
		4:  4141, // 兔王 -> 兔尾巴 (兔毛 aliases 猴皮, sprite 4136)
		5:  4121, // 狐妖 -> 火灵珠
		6:  4102, // 小虎怪 -> 老虎皮
		9:  4313, // 牛头 -> 牛角
		11: 4304, // 山鼠 -> 山鼠尾巴
		13: 4132, // 小野猪 -> 野猪牙
		15: 4115, // 山狼 -> 狼皮
		16: 4131, // 小黑狗 -> 小狗牙
		20: 4136, // 小猴子 -> 猴皮
		25: 4314, // 马面 -> 马面具
		26: 4344, // 鹿角面具 -> 鹿角面具
		27: 4116, // 碧蛇 -> 蛇胆
		28: 4338, // 蚯蚓 -> 泥块
	}
	for ordinal, want := range verified {
		id := domain.FirstAstralRune + domain.ItemID(ordinal-1)
		if got := endgameItemIcon(id); got != want {
			t.Errorf("rune %d: got sprite %d, want verified collectible %d", id, got, want)
		}
	}
}

func TestEndgameRuneIconsUseLabelledRelatedCollectibles(t *testing.T) {
	// Related native collectibles are permitted for the remaining mansions.
	// Their separate provenance status must not imply an exact-species drop.
	related := map[int]int32{
		1: 26018, 3: 4134, 7: 4122, 8: 4118, 10: 4158, 12: 4117,
		14: 4112, 17: 4345, 18: 4142, 19: 39439, 21: 4350,
		22: 4137, 23: 27653, 24: 3583,
	}
	for ordinal, want := range related {
		id := domain.FirstAstralRune + domain.ItemID(ordinal-1)
		if got := endgameItemIcon(id); got != want {
			t.Errorf("related-collectible rune %d: got sprite %d, want %d", id, got, want)
		}
	}
}
