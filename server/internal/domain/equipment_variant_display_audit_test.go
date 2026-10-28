package domain

import (
	"strings"
	"testing"
)

func TestEquipmentVariantDisplayIgnoresLegacyPolicyDescriptions(t *testing.T) {
	for _, r := range testEndgameCatalogData().Variants {
		before := r
		r.Description = "旧说明：不连锁引爆；每目标仅一份；满血不存量；需新版客户端；换装不刷新剩余冷却。"
		v := r.Variant()
		for _, bad := range []string{"旧说明", "不连锁", "每目标仅", "不存量", "新版客户端", "换装", "不附加状态", "不震退"} {
			if strings.Contains(v.Description, bad) {
				t.Errorf("%s policy leak: %s", r.ID, v.Description)
			}
		}
		if v.Description == "" || v.SkillID != before.SkillID || v.Behavior != before.Behavior {
			t.Fatal("variant identity changed", v)
		}
		// Existing retirement of geometric target caps is independent of wording.
		params := before.Variant().Params
		if v.Params != params {
			t.Fatal("description altered mechanics", v)
		}
	}
}
