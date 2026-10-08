package domain

import "testing"

// seqIntn 是确定性伪随机源，保证测试不依赖 math/rand 的实现细节。
func seqIntn() func(int64) int64 {
	state := uint64(0x9E3779B97F4A7C15)
	return func(n int64) int64 {
		state = state*6364136223846793005 + 1442695040888963407
		return int64((state >> 33) % uint64(n))
	}
}

// colorRollTable 造一张最小矩阵：默认档保留 60/20/10/5/3/1，普通档只配到 2 条。
// 数值池给 6 个不同 attr，避免“候选不足”掩盖条数上限。
func colorRollTable() EquipmentRollTable {
	key := EquipmentRollKey{Level: 1, Type: 101}
	options := make([]EquipmentRollOption, 0, 6)
	for i := 1; i <= 6; i++ {
		options = append(options, EquipmentRollOption{
			CardID: int32(5000 + i),
			Affix:  Affix{Attr: int32(i * 2), Value: 10, Mode: ModeAbsolute},
			Weight: 100,
		})
	}
	return EquipmentRollTable{
		Counts: map[ColorProfile][]WeightedAffixCount{
			DefaultColorProfile: {{0, 60}, {1, 20}, {2, 10}, {3, 5}, {4, 3}, {5, 1}},
			2:                   {{0, 70}, {1, 20}, {2, 10}},
		},
		Options: map[EquipmentRollKey][]EquipmentRollOption{key: options},
	}
}

func colorRollDef() ItemDef {
	return ItemDef{ID: 1001, Equip: &EquipDef{ResourceLevel: 1, Type: 101}}
}

// 普通档只配了 0/1/2 三档，任何一次抽取都不得超过 2 条（天蓝上限）。
func TestRollOrdinaryProfileCapsAtTwoAffixes(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	next := seqIntn()
	seen := map[int]bool{}
	for i := 0; i < 4000; i++ {
		rolled := table.Roll(def, 2, next)
		if len(rolled) > 2 {
			t.Fatalf("普通档抽出 %d 条属性，超过 2 条上限", len(rolled))
		}
		seen[len(rolled)] = true
	}
	if !seen[0] || !seen[1] || !seen[2] {
		t.Fatalf("普通档应能抽到 0/1/2 条，实得 %v", seen)
	}
}

// 默认档（精英/BOSS）保持原有分布，仍能出到 5 条。
func TestRollDefaultProfileReachesFiveAffixes(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	next := seqIntn()
	max := 0
	for i := 0; i < 6000; i++ {
		if n := len(table.Roll(def, DefaultColorProfile, next)); n > max {
			max = n
		}
	}
	if max != MaxEquipmentAttributes {
		t.Fatalf("默认档最多只出到 %d 条，应能出到 %d 条", max, MaxEquipmentAttributes)
	}
}

// 档位缺失时回退默认档，避免没带档位的实体（测试夹具、旧回放）静默产出白板。
func TestRollUnknownProfileFallsBackToDefault(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	if got := len(table.CountsFor(99)); got != len(table.Counts[DefaultColorProfile]) {
		t.Fatalf("未知档位应回退默认档，实得 %d 档权重", got)
	}
	next := seqIntn()
	max := 0
	for i := 0; i < 6000; i++ {
		if n := len(table.Roll(def, 99, next)); n > max {
			max = n
		}
	}
	if max <= 2 {
		t.Fatalf("未知档位回退后最多只出到 %d 条，没有走默认档", max)
	}
}

// 同一件装备不重复抽同一个 attr_id。
func TestRollDoesNotRepeatAttr(t *testing.T) {
	table := colorRollTable()
	def := colorRollDef()
	next := seqIntn()
	for i := 0; i < 2000; i++ {
		rolled := table.Roll(def, DefaultColorProfile, next)
		used := make(map[int32]struct{}, len(rolled))
		for _, affix := range rolled {
			if _, dup := used[affix.Affix.Attr]; dup {
				t.Fatalf("attr %d 重复出现: %+v", affix.Affix.Attr, rolled)
			}
			used[affix.Affix.Attr] = struct{}{}
		}
	}
}

// 权重按总份额归一化，最后一份必须能被抽到（intn 返回 total-1 的边界）。
func TestWeightedAffixCountBoundary(t *testing.T) {
	counts := []WeightedAffixCount{{0, 70}, {1, 20}, {2, 10}}
	if got := weightedAffixCount(counts, func(int64) int64 { return 0 }); got != 0 {
		t.Fatalf("第一份应为 0 条，实得 %d", got)
	}
	if got := weightedAffixCount(counts, func(n int64) int64 { return n - 1 }); got != 2 {
		t.Fatalf("最后一份应为 2 条，实得 %d", got)
	}
	if got := weightedAffixCount(counts, func(n int64) int64 { return 89 }); got != 1 {
		t.Fatalf("第 90 份（70+20）应为 1 条，实得 %d", got)
	}
}
