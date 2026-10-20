package scene

import (
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// statusVisual projects existing artwork without changing the runtime status,
// its ID, icon, description, mechanics or lifetime. Legacy self clients already
// resolve the original alias table, but remote actors require the final key.
// Only the recovered statuses missing from that self table need a wire title
// change: 0x8017 shares its Name field between the HUD and aura lookup.
func statusVisual(def domain.StatusDef) (effect, buffName string, position uint8) {
	effect, buffName, position = def.Name, def.Name, statusPositionMode(def)
	switch def.ID {
	case 10009:
		if def.Name == "破防之枪" {
			effect, buffName = "防御下降", "防御下降"
		}
	case 10012:
		// Same incoming physical/magic damage operators as native 降幅 16–20.
		if def.Name == "破甲" {
			effect, buffName = "降幅", "降幅"
		}
	case 10018, 12034:
		if def.Name == "冲锋" {
			effect, buffName = "加速", "加速"
		}
	case 10019, 10427:
		if def.Name == "全力投掷" || def.Name == "冰系掌握" {
			effect, buffName = "减速", "减速"
		}
	case 10115:
		// Same attack increase/defense decrease pair as native 强化 11–15.
		if def.Name == "火字诀" {
			effect, buffName = "强化", "强化"
		}
	case 10303:
		// Exact HP-per-second and duration series of native 补充生命 12–16.
		if def.Name == "回春术" {
			effect, buffName = "补血", "补血"
		}
	}
	// Existing client aliases, not newly inferred skill-cast artwork. Keep their
	// established self HUD titles; only the remote effect key needs resolving.
	switch effect {
	case "春风", "补充生命":
		effect = "补血"
	case "补充法力":
		effect = "补魔法"
	case "魔镜术":
		effect = "魔法反弹"
	case "瞬发":
		effect = "鹰眼"
	case "燃烧":
		effect = "燃烧1"
	case "仙佑符":
		effect = "TFX074_40"
	case "灵聚之术符":
		effect = "TFX7011_40"
	case "神返咒符":
		effect = "TFX7012_40"
	case "砺金之咒符", "坚甲之术符", "灵护之术符", "御法之术符":
		effect = "TFX028_40"
	case "裂刃枪法符":
		effect = "TFX068_40"
	case "野性之赐符":
		effect = "TFX7015_40"
	case "心与剑随符":
		effect = "TFX056_40"
	case "凌虚步法符":
		effect = "TFX7018_40"
	case "灵印法符":
		effect = "TFX1082_30"
	}
	// Clients retain an aura instance while any row uses its key. A shared
	// resource therefore needs one stable anchor, independent of row ordering.
	switch effect {
	case "加速", "减速", "补血", "补魔法", "鹰眼", "燃烧1":
		position = 0
	case "防御下降", "降幅", "强化", "魔法反弹":
		position = domain.StatusPositionOrbit
	default:
		if strings.HasPrefix(effect, "TFX") {
			position = 3
		}
	}
	return
}

// statusWorldEffects coalesces only the name-keyed visual channel, never the
// independent Buff/icon/status rows. Its deadline is the last contributor's
// deadline, not the last row's deadline. Equipment keys stay separate.
func statusWorldEffects(statuses []statusPresentation) []event.EntityStatusEffect {
	var out []event.EntityStatusEffect
	index := make(map[string]int, len(statuses))
	for _, status := range statuses {
		effect := status.effect
		if effect.Effect == "" {
			continue
		}
		if i, ok := index[effect.Effect]; ok {
			if effect.DurationMS > out[i].DurationMS {
				out[i].DurationMS = effect.DurationMS
			}
			continue
		}
		index[effect.Effect] = len(out)
		out = append(out, effect)
	}
	return out
}
