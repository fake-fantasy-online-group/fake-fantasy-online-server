package domain

import "strconv"

func legacyCubeRecipe(id int32, target Stack, items map[ItemID]ItemDef) (EndgameRecipeView, bool) {
	r := EndgameRecipeView{ID: id, Money: "0", TargetRequired: true}
	add := func(id ItemID, n int32) {
		r.Materials = append(r.Materials, EndgameMaterialView{ItemID: int32(id), Name: items[id].Name, Count: n})
	}
	fee := func(n int64) { r.Money = strconv.FormatInt(n, 10) }
	switch id {
	case CubeSalvageNormal:
		r.Name = "解器归元"
		r.Operation = "salvage.normal"
		r.Description = "消耗普通可染色装备，获得星髓；孔内卡原样返还，换形与装备灵随装备消失。"
	case CubeSalvageUnique:
		r.Name = "仙遗化韵"
		r.Operation = "salvage.unique"
		r.Description = "消耗仙遗，获得星髓与八仙余韵；孔内卡原样返还，换形与装备灵随装备消失。"
	case CubeSalvageRune:
		r.Name = "星篆归尘"
		r.Operation = "salvage.rune"
		r.TargetRequired = false
		r.Description = "消耗所选一枚背包星篆，获得4星髓。"
	case CubeOpenSocket:
		r.Name = "开窍引星"
		r.Operation = "socket.open"
		r.Description = "仅限无孔装备，随机开出1至真实容量的孔数，各孔数等概率；已有孔不可重开。"
		add(CubeStarMarrow, 4)
		fee(5000)
	case CubeExtractRunes:
		r.Name = "归星还篆"
		r.Operation = "socket.extract"
		r.Description = "原样取回全部孔内卡，保留空孔；背包不足则整笔不执行。"
		add(CubeStarMarrow, 4)
		add(CubePurifiedDust, 2)
		fee(10000)
	case CubeReorderRunes:
		r.Name = "移星换斗"
		r.Operation = "socket.reorder"
		r.Description = "按所选顺序重排已镶满的全部星篆，保留每枚身份与属性。"
		add(CubeStarMarrow, 2)
		add(CubePurifiedDust, 1)
		fee(5000)
	case CubeInscribeArray:
		r.Name = "布宿成阵"
		r.Operation = "socket.array"
		r.Description = "将所选星篆依次放入全部空孔，须完全匹配一项宿阵的孔数、顺序与部位。"
		add(CubeStarMarrow, 6)
		add(CubePurifiedDust, 3)
		fee(15000)
	case CubeAwakenUnique:
		r.Name = "淬器成真"
		r.Operation = "unique.awaken"
		r.Description = "唤醒此基底唯一对应的仙遗；主体属性固定，少数词条浮动；原有加工、孔内卡与换形保留。"
		add(CubeStarMarrow, 120)
		add(CubePurifiedDust, 30)
		fee(100000)
	case CubeReforgeUnique:
		r.Name = "返本重铸"
		r.Operation = "unique.reforge"
		r.Description = "仅重铸明确浮动的词条，保证与当前可见数值不同，可能降低；固定属性、原有养成与仙遗身份保留。"
		add(CubeStarMarrow, 40)
		add(CubePurifiedDust, 25)
		add(CubeImmortalEcho, 4)
		fee(100000)
	case CubeTemperUnique:
		r.Name = "炼真守一"
		r.Operation = "unique.temper"
		r.Description = "提高少数浮动词条的共享品质，确保可见数值提升；已满值不可使用，固定属性与原有养成不变。"
		add(CubeStarMarrow, 60)
		add(CubePurifiedDust, 40)
		add(CubeImmortalEcho, 6)
		fee(200000)
	default:
		if rune, ok := cubeRecipeRune(id, CubeInscribeRuneBase); ok {
			d, exists := items[rune]
			if !exists {
				return r, false
			}
			r.Name = "凝髓成篆·" + d.Name
			r.Operation = "rune.inscribe"
			r.TargetRequired = false
			r.OutputItemID = int32(rune)
			r.Description = "凝成一枚指定星篆。"
			add(CubeStarMarrow, 30)
			add(CubePurifiedDust, 10)
			fee(30000)
		} else if rune, ok := cubeRecipeRune(id, CubeConvertRuneBase); ok {
			d, exists := items[rune]
			if !exists {
				return r, false
			}
			r.Name = "同象易宿·" + d.Name
			r.Operation = "rune.convert"
			r.TargetRequired = false
			r.OutputItemID = int32(rune)
			r.Description = "消耗三枚相同星篆，换得指定的同象异宿星篆。"
			add(CubeStarMarrow, 6)
			add(CubePurifiedDust, 3)
			fee(10000)
		} else {
			return r, false
		}
	}
	return r, true
}

// Recipe IDs are stable protocol identifiers. This helper is only used by tests
// and compatibility checks; production execution dispatches the configured op.
func cubeRecipeRune(id, base int32) (ItemID, bool) {
	n := id - base - 1
	return FirstAstralRune + ItemID(n), n >= 0 && n < 28
}
